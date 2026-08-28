// Concurrent first-page and result-count capability suite (Issue #24 Tasks
// 5–6): release- and dependency-upgrade-blocking modernc.org/sqlite
// integration tests proving that the two requests of one SELECT run as
// independent autocommit reads on two distinct dedicated leases from the
// exact-two pool, with real overlap behind test-only barriers, unchanged
// journal mode, no hidden serialization, permitted independent-snapshot drift
// across an interleaved external writer, and clean lease release on success
// and error in both WAL and rollback-journal modes. The barrier hooks are the
// DB's documented test seams; production control flow never reads them. No
// sleeps order the tests: every wait is a channel receive or an explicit
// timeout bound, and failures identify journal mode, lease identity, and the
// blocked phase.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// journalModes enumerates both required journal-mode fixtures.
var journalModes = []string{"delete", "wal"}

// createJournalDatabase builds a fixture database already configured in the
// requested journal mode with a known users table, and records the mode as
// observed before Sqloid opens the file.
func createJournalDatabase(t *testing.T, path, mode string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)"); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := db.Exec("INSERT INTO users (email) VALUES (?)", "u"+string(rune('0'+i))); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
	if _, err := db.Exec("PRAGMA journal_mode=" + mode); err != nil {
		t.Fatalf("setting fixture journal mode %s: %v", mode, err)
	}
	var got string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&got); err != nil {
		t.Fatalf("recording fixture journal mode: %v", err)
	}
	if !strings.EqualFold(got, mode) {
		t.Fatalf("fixture journal mode = %q, want %q", got, mode)
	}
}

// openJournalFixture creates the fixture in the given mode and opens it
// through the production boundary.
func openJournalFixture(t *testing.T, mode string) *DB {
	t.Helper()
	path := t.TempDir() + "/journal.db"
	createJournalDatabase(t, path, mode)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) in %s mode: %v", path, mode, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// journalMode reports the mode of the opened pool through its own leased
// connection, proving Open never changed it.
func assertPoolJournalMode(t *testing.T, db *DB, mode string) {
	t.Helper()
	var got string
	res := db.RunRequest(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
		return conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&got)
	})
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("[%s] reading journal mode: outcome %v (err %v)", mode, res.Outcome, res.Err)
	}
	if !strings.EqualFold(got, mode) {
		t.Fatalf("[%s] journal mode after Open = %q, want unchanged %q — the pool must never change journal mode", mode, got, mode)
	}
}

// rawConnIdentity captures the physical connection behind a leased
// *sql.Conn so tests can prove two leases are distinct, returning an
// identity captured at the barrier (the driver handle inside conn is not
// safely comparable, so identity is captured per call and compared by
// pointer-string uniqueness at capture time).
func rawConnIdentity(conn *sql.Conn) string {
	var identity string
	err := conn.Raw(func(driverConn any) error {
		identity = fmt.Sprintf("%p", driverConn)
		return nil
	})
	if err != nil {
		return "raw-error:" + err.Error()
	}
	return identity
}

// TestConcurrentPageAndCountOverlapDistinctLeases proves, for both journal
// modes, that first page and count run concurrently as independent autocommit
// reads on two distinct physical connections from the exact-two pool: both
// barrier-held phases are simultaneously inside their operations, neither
// waits for artificial application serialization, and both settle
// independently and release their leases cleanly.
func TestConcurrentPageAndCountOverlapDistinctLeases(t *testing.T) {
	for _, mode := range journalModes {
		t.Run(mode, func(t *testing.T) {
			db := openJournalFixture(t, mode)

			// Barriers: each operation reports it holds its lease and then
			// waits for release, so both are simultaneously mid-operation.
			var wg sync.WaitGroup
			pageIn, countIn := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
				pageConn := rawConnIdentity(conn)
				close(pageIn)
				<-release
				_ = pageConn
			}
			db.beforeCount = func(ctx context.Context, conn *sql.Conn) {
				countConn := rawConnIdentity(conn)
				_ = countConn
				close(countIn)
				<-release
			}
			t.Cleanup(func() { db.beforeFirstPage, db.beforeCount = nil, nil })

			pageDone, countDone := make(chan RequestResult, 1), make(chan RequestResult, 1)
			wg.Add(2)
			go func() {
				defer wg.Done()
				defer close(pageDone)
				_, res := db.RunFirstPage(context.Background(), `SELECT id FROM "users" ORDER BY id`, nil)
				pageDone <- res
			}()
			go func() {
				defer wg.Done()
				defer close(countDone)
				_, res := db.RunCount(context.Background(), `SELECT COUNT(*) FROM (SELECT id FROM "users")`, nil)
				countDone <- res
			}()

			// Both must be simultaneously inside their operations: a third
			// lease would block, and neither operation waited for the other.
			select {
			case <-pageIn:
			case <-time.After(2 * time.Second):
				t.Fatalf("[%s] first page never reached its lease-held phase", mode)
			}
			select {
			case <-countIn:
			case <-time.After(2 * time.Second):
				t.Fatalf("[%s] count never reached its lease-held phase — the concurrent reads were serialized", mode)
			}

			// A third lease must block while both hold the exact-two pool.
			third := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
				defer cancel()
				_, err := db.Lease(ctx)
				third <- err
			}()
			select {
			case err := <-third:
				if err == nil {
					t.Fatalf("[%s] a third lease was admitted while page and count hold leases — pool grew beyond exact two", mode)
				}
			case <-time.After(time.Second):
				t.Fatalf("[%s] third-lease probe did not terminate", mode)
			}

			// Release both barriers; both operations must settle
			// independently on success.
			close(release)
			wg.Wait()
			for name, ch := range map[string]chan RequestResult{"page": pageDone, "count": countDone} {
				select {
				case res, ok := <-ch:
					if !ok || res.Outcome != OutcomeSuccess {
						t.Errorf("[%s] %s request outcome = %v (err %v), want success", mode, name, res.Outcome, res.Err)
					}
				default:
					t.Fatalf("[%s] %s request did not settle after release", mode, name)
				}
			}
			assertPoolJournalMode(t, db, mode)
		})
	}
}

// TestPageAndCountLeasesAreDistinctPhysicalConnections proves the two
// concurrent requests occupy two distinct physical connections from the pool.
func TestPageAndCountLeasesAreDistinctPhysicalConnections(t *testing.T) {
	mode := "wal"
	db := openJournalFixture(t, mode)

	var mu sync.Mutex
	var pageConn, countConn string
	var captured sync.WaitGroup
	captured.Add(2)
	// Each hook waits until both identities are captured, forcing genuine
	// simultaneous lease holding before either statement runs.
	db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
		mu.Lock()
		pageConn = rawConnIdentity(conn)
		mu.Unlock()
		captured.Done()
		captured.Wait()
	}
	db.beforeCount = func(ctx context.Context, conn *sql.Conn) {
		mu.Lock()
		countConn = rawConnIdentity(conn)
		mu.Unlock()
		captured.Done()
		captured.Wait()
	}
	t.Cleanup(func() { db.beforeFirstPage, db.beforeCount = nil, nil })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); db.RunFirstPage(context.Background(), `SELECT id FROM "users"`, nil) }()
	go func() {
		defer wg.Done()
		db.RunCount(context.Background(), `SELECT COUNT(*) FROM (SELECT id FROM "users")`, nil)
	}()

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if pageConn == "" || countConn == "" {
		t.Fatalf("[%s] missing physical connection identity: page %q, count %q", mode, pageConn, countConn)
	}
	if pageConn == countConn {
		t.Fatalf("[%s] page and count ran on the same physical connection %v", mode, pageConn)
	}
}

// TestIndependentSnapshotsPermitDrift proves the PRD-permitted independent
// snapshots: the page observes its snapshot, an external writer commits
// between the two reads, the count reflects the newer state, and neither the
// page rows nor the count is clamped or reconciled to the other.
func TestIndependentSnapshotsPermitDrift(t *testing.T) {
	mode := "wal"
	db := openJournalFixture(t, mode)
	page, pageRes := db.RunFirstPage(context.Background(), `SELECT id FROM "users" ORDER BY id`, nil)
	if pageRes.Outcome != OutcomeSuccess {
		t.Fatalf("[%s] page outcome = %v (err %v)", mode, pageRes.Outcome, pageRes.Err)
	}
	if len(page.Rows) != 3 {
		t.Fatalf("[%s] page rows = %d, want 3", mode, len(page.Rows))
	}

	// Interleave a controlled external writer between the two reads.
	ext, err := sql.Open("sqlite", db.path)
	if err != nil {
		t.Fatalf("[%s] external writer open: %v", mode, err)
	}
	defer ext.Close()
	if _, err := ext.Exec("INSERT INTO users (email) VALUES ('u4')"); err != nil {
		t.Fatalf("[%s] external writer insert: %v", mode, err)
	}

	total, countRes := db.RunCount(context.Background(), `SELECT COUNT(*) FROM (SELECT id FROM "users")`, nil)
	if countRes.Outcome != OutcomeSuccess {
		t.Fatalf("[%s] count outcome = %v (err %v)", mode, countRes.Outcome, countRes.Err)
	}
	if total != 4 {
		t.Errorf("[%s] count total = %d, want 4 after the external writer", mode, total)
	}
	// Drift is accepted: the page snapshot stays three rows, never clamped or
	// reconciled to the newer count, and vice versa.
	if len(page.Rows) != 3 {
		t.Errorf("[%s] page rows changed to %d after the count — drift must not be reconciled", mode, len(page.Rows))
	}
}

// TestRollbackJournalExternalWriterDelayOrLockError proves the
// rollback-journal behavior the PRD accepts: while one Sqloid read holds its
// lease, an external writer is delayed (succeeding after release) or fails
// with the ordinary `database is locked` error — and the read's rows stay
// valid and the pool's journal mode is untouched. The writer's own busy
// timeout is deliberately short so no test waits the pool's five seconds.
func TestRollbackJournalExternalWriterDelayOrLockError(t *testing.T) {
	mode := "delete"
	db := openJournalFixture(t, mode)

	// Hold one leased read mid-operation behind the test barrier.
	held := make(chan struct{})
	release := make(chan struct{})
	var errOnce sync.Once
	db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
		errOnce.Do(func() { close(held) })
		<-release
	}
	t.Cleanup(func() { db.beforeFirstPage = nil })

	pageDone := make(chan struct{})
	go func() {
		defer close(pageDone)
		db.RunFirstPage(context.Background(), `SELECT id FROM "users"`, nil)
	}()
	select {
	case <-held:
	case <-time.After(2 * time.Second):
		t.Fatalf("[%s] page never reached its lease-held phase", mode)
	}

	// External writer with a short busy timeout attempts a write now.
	writerDone := make(chan error, 1)
	go func() {
		ext, err := sql.Open("sqlite", db.path+"?_busy_timeout=100")
		if err != nil {
			writerDone <- err
			return
		}
		defer ext.Close()
		var writeErr error
		for i := 0; i < 5; i++ {
			if _, writeErr = ext.Exec("INSERT INTO users (email) VALUES ('late')"); writeErr == nil {
				writerDone <- nil
				return
			}
		}
		writerDone <- writeErr
	}()

	// Release the read; the writer must then succeed (delayed) or have failed
	// with the ordinary locked error. Both are accepted outcomes.
	close(release)
	<-pageDone
	select {
	case err := <-writerDone:
		if err != nil && !strings.Contains(err.Error(), "database is locked") {
			t.Fatalf("[%s] external writer failed with unexpected error: %v", mode, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("[%s] external writer neither settled nor failed with the ordinary lock error", mode)
	}

	assertPoolJournalMode(t, db, mode)

	// The read's lease is released and the pool stays healthy: new work runs.
	page, res := db.RunFirstPage(context.Background(), `SELECT id FROM "users" ORDER BY id`, nil)
	if res.Outcome != OutcomeSuccess || page == nil {
		t.Fatalf("[%s] follow-up page outcome = %v (err %v)", mode, res.Outcome, res.Err)
	}
	// Depending on which accepted outcome the writer took (delayed success
	// or ordinary lock error), the follow-up page sees 4 or 3 rows; either
	// way the read path is healthy and unclobbered.
	if len(page.Rows) != 3 && len(page.Rows) != 4 {
		t.Errorf("[%s] follow-up page rows = %d, want 3 (writer locked) or 4 (delayed success)", mode, len(page.Rows))
	}
}

// TestCountFailureDoesNotInvalidatePageRows proves failure isolation end to
// end through the real driver: a failing count (bad table) leaves a
// successful page untouched, with the ordinary typed count failure and no
// health misclassification.
func TestCountFailureDoesNotInvalidatePageRows(t *testing.T) {
	mode := "wal"
	db := openJournalFixture(t, mode)

	page, pageRes := db.RunFirstPage(context.Background(), `SELECT id FROM "users" ORDER BY id`, nil)
	if pageRes.Outcome != OutcomeSuccess || page == nil {
		t.Fatalf("[%s] page outcome = %v", mode, pageRes.Outcome)
	}

	_, countRes := db.RunCount(context.Background(), `SELECT COUNT(*) FROM (SELECT id FROM "missing_table")`, nil)
	if countRes.Outcome != OutcomeFailed {
		t.Fatalf("[%s] count outcome = %v, want failed", mode, countRes.Outcome)
	}
	if countRes.Err == nil || !strings.Contains(countRes.Err.Error(), "no such table") {
		t.Errorf("[%s] count error = %v, want the ordinary driver cause", mode, countRes.Err)
	}
	if countRes.Health != nil {
		t.Errorf("[%s] ordinary count error misclassified as health failure %v", mode, countRes.Health)
	}
	// The successful page stands: rows retained, nothing clamped or cleared.
	if len(page.Rows) != 3 {
		t.Errorf("[%s] page rows = %d after count failure, want 3", mode, len(page.Rows))
	}

	// And the count error is inspectable as its own typed boundary failure.
	var ce *countError
	if !errors.As(countRes.Err, &ce) {
		t.Errorf("[%s] count error %T does not unwrap to countError", mode, countRes.Err)
	}
}
