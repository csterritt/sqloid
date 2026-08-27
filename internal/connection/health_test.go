//go:build unix

// Request-boundary database identity checks for Issue #7 and the Session
// health requirements in Notes/PRD-sqloid.md: startup records the validated
// target's device and inode on Linux/macOS, the original path is verified
// immediately before every request and before any newly opened or
// replacement physical connection is admitted for use, deletions (including
// rename-away) and same-path replacements yield typed outcomes, in-place
// same-inode mutation follows ordinary SQLite behavior, a raced replacement
// plus request error classifies terminal immediately while a successful
// result stands until the next boundary, and an entire phased write receives
// exactly one pre-BEGIN check with none between statement execution and
// COMMIT. Terminal UI wording is owned by Issue #46 and is asserted nowhere.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// mustOpen opens path through the shared opener and registers cleanup.
func mustOpen(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// replacePath creates a fresh, valid SQLite database at an adjacent temporary
// name and renames it over path so that path keeps its name but gains a new
// inode. tmpfs/ext4 rarely reuse inode numbers while old files remain known,
// but the retry loop guarantees the identifiers truly differ.
func replacePath(t *testing.T, path string, recordedIno uint64) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		fresh := path + ".fresh"
		createDatabase(t, fresh)
		if _, ino, err := statIdentity(fresh); err == nil && ino == recordedIno {
			os.Remove(fresh)
			continue
		}
		if err := os.Rename(fresh, path); err != nil {
			t.Fatalf("renaming replacement over %q: %v", path, err)
		}
		return
	}
	t.Fatalf("could not manufacture a replacement with a distinct inode for %q", path)
}

// TestOpenRecordsStartupDeviceAndInode pins that the opener records the
// device and inode of the successfully validated target, so every later
// request-boundary comparison has a trustworthy reference.
func TestOpenRecordsStartupDeviceAndInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.db")
	createDatabase(t, path)

	db := mustOpen(t, path)
	wantDev, wantIno, err := statIdentity(path)
	if err != nil {
		t.Fatalf("statting %q: %v", path, err)
	}
	if db.startDev != wantDev || db.startIno != wantIno {
		t.Errorf("recorded identity = (%d, %d), want current stat = (%d, %d)", db.startDev, db.startIno, wantDev, wantIno)
	}
}

// TestVerifyHealthClassifications is the table-driven typed-outcome contract:
// absence (deletion and rename-away alike) classifies as deletion with a
// preserved unwrappable cause, same-path replacement with different device or
// inode classifies as replacement, and in-place mutation retaining both
// identifiers is accepted as ordinary SQLite behavior.
func TestVerifyHealthClassifications(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(t *testing.T, db *DB, path string)
		wantKind  HealthKind // 0 means nil error expected
		wantCause bool       // underlying cause must be preserved and unwrappable
	}{
		{
			name:   "unchanged target passes",
			mutate: func(t *testing.T, db *DB, path string) {},
		},
		{
			name: "deletion reports typed absence",
			mutate: func(t *testing.T, db *DB, path string) {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
			wantKind:  HealthDeleted,
			wantCause: true,
		},
		{
			name: "rename-away reports typed absence",
			mutate: func(t *testing.T, db *DB, path string) {
				if err := os.Rename(path, path+".moved"); err != nil {
					t.Fatal(err)
				}
			},
			wantKind:  HealthDeleted,
			wantCause: true,
		},
		{
			name: "same-path replacement with new inode reports replacement",
			mutate: func(t *testing.T, db *DB, path string) {
				replacePath(t, path, db.startIno)
			},
			wantKind: HealthReplaced,
		},
		{
			name: "in-place same-inode mutation follows ordinary SQLite behavior",
			mutate: func(t *testing.T, db *DB, path string) {
				outside, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer outside.Close()
				if _, err := outside.Exec("INSERT INTO t VALUES (2, 'mutated in place')"); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "classify.db")
			createDatabase(t, path)
			db := mustOpen(t, path)

			tt.mutate(t, db, path)
			err := db.VerifyHealth()

			switch {
			case tt.wantKind == 0 && err != nil:
				t.Fatalf("VerifyHealth() = %v, want nil", err)
			case tt.wantKind != 0 && err == nil:
				t.Fatalf("VerifyHealth() = nil, want kind %s", tt.wantKind)
			}
			if tt.wantKind == 0 {
				return
			}
			var he *HealthError
			if !errors.As(err, &he) {
				t.Fatalf("VerifyHealth() = %T %v, want *HealthError", err, err)
			}
			if he.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", he.Kind, tt.wantKind)
			}
			if he.Path != path {
				t.Errorf("Path = %q, want %q", he.Path, path)
			}
			if tt.wantCause && he.Cause == nil {
				t.Error("underlying cause was dropped; typed outcomes must preserve causes")
			}
			if tt.wantCause && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("cause %v does not unwrap to fs.ErrNotExist", he.Cause)
			}
		})
	}
}

// TestLeaseVerifiesIdentityBeforeConnectionIsUsed proves the guard covers
// newly opened and replacement physical connections: no connection obtained
// through Lease is handed to a caller for use until the original path's
// identity matches startup. Deletion and replacement both stop admission;
// wording remains typed, never terminal UI copy.
func TestLeaseVerifiesIdentityBeforeConnectionIsUsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lease-guard.db")
	createDatabase(t, path)
	db := mustOpen(t, path)

	healthy, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("Lease on unchanged target: %v", err)
	}
	healthy.Release(context.Background())

	for _, tc := range []struct {
		name     string
		mutate   func(t *testing.T, db *DB, path string)
		wantKind HealthKind
	}{
		{name: "deleted file refuses lease", mutate: func(t *testing.T, db *DB, path string) { os.Remove(path) }, wantKind: HealthDeleted},
		{name: "replacement refuses lease", mutate: func(t *testing.T, db *DB, path string) { replacePath(t, path, db.startIno) }, wantKind: HealthReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(t, db, path)
			lease, err := db.Lease(context.Background())
			if err == nil {
				lease.Release(context.Background())
				t.Fatal("Lease admitted a connection despite changed target identity")
			}
			var he *HealthError
			if !errors.As(err, &he) || he.Kind != tc.wantKind {
				t.Fatalf("Lease error = %v (%T), want *HealthError kind %s", err, err, tc.wantKind)
			}
		})
	}
}

// rawPointer captures the underlying driver connection pointer so tests can
// compare physical identity of pooled connections.
func rawPointer(conn *sql.Conn) any {
	var p any
	_ = conn.Raw(func(driverConn any) error { p = driverConn; return nil })
	return p
}

// TestRunRequestExercisesReusableBoundaries drives the reusable request-bound
// entry point with the request shapes later features build on: a schema-style
// read, a count-style read, an estimate-style read, and two concurrent pooled
// reads served by distinct physical connections.
func TestRunRequestExercisesReusableBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundaries.db")
	createDatabase(t, path)
	db := mustOpen(t, path)
	ctx := context.Background()

	tests := []struct {
		name string
		op   func(ctx context.Context, conn *sql.Conn) error
	}{
		{name: "schema-style read", op: func(ctx context.Context, conn *sql.Conn) error {
			var version int64
			return conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version)
		}},
		{name: "count-style read", op: func(ctx context.Context, conn *sql.Conn) error {
			var n int64
			return conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM t").Scan(&n)
		}},
		{name: "estimate-style read", op: func(ctx context.Context, conn *sql.Conn) error {
			var v string
			if err := conn.QueryRowContext(ctx, "SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
				return err
			}
			if v != "one" {
				t.Errorf("estimate-style read got %q, want %q", v, "one")
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := db.identityChecks.Load()
			result := db.RunRequest(ctx, tt.op)
			if result.Outcome != OutcomeSuccess {
				t.Fatalf("Outcome = %s, Err = %v, want success", result.Outcome, result.Err)
			}
			if result.Err != nil || result.Health != nil {
				t.Errorf("successful request carried Err = %v, Health = %v", result.Err, result.Health)
			}
			if got := db.identityChecks.Load() - before; got < 1 {
				t.Errorf("request performed %d identity checks, want at least the one boundary check", got)
			}
		})
	}

	t.Run("concurrent pooled requests use distinct checked connections", func(t *testing.T) {
		type captured struct{ pointer any }
		pointers := make(chan captured, 2)
		results := make(chan RequestResult, 2)
		// Rendezvous barrier: neither operation proceeds past it until both
		// leases are held, proving the pool served both concurrently.
		var arrive sync.WaitGroup
		arrive.Add(2)
		gate := make(chan struct{})
		go func() { arrive.Wait(); close(gate) }()
		run := func() {
			results <- db.RunRequest(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
				pointers <- captured{pointer: rawPointer(conn)}
				arrive.Done()
				<-gate
				var n int64
				return conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM t").Scan(&n)
			})
		}
		go run()
		go run()

		seen := map[any]bool{}
		for i := 0; i < 2; i++ {
			res := <-results
			if res.Outcome != OutcomeSuccess {
				t.Fatalf("concurrent request %d outcome = %s, Err = %v, want success", i, res.Outcome, res.Err)
			}
			p := (<-pointers).pointer
			if seen[p] {
				t.Error("both concurrent pooled requests ran on the same physical connection")
			}
			seen[p] = true
		}
	})
}

// TestRunRequestBlocksChangedTargetBeforeWork proves the boundary precedes
// all database work: when the original path is deleted or replaced before a
// request, its operation never starts and the typed classification names the
// reason.
func TestRunRequestBlocksChangedTargetBeforeWork(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(t *testing.T, db *DB, path string)
		wantKind HealthKind
	}{
		{name: "deleted", mutate: func(t *testing.T, db *DB, path string) { os.Remove(path) }, wantKind: HealthDeleted},
		{name: "replaced", mutate: func(t *testing.T, db *DB, path string) { replacePath(t, path, db.startIno) }, wantKind: HealthReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "blocked.db")
			createDatabase(t, path)
			db := mustOpen(t, path)
			tc.mutate(t, db, path)

			opRan := false
			before := db.identityChecks.Load()
			result := db.RunRequest(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
				opRan = true
				return nil
			})

			if opRan {
				t.Error("operation ran although the target had changed before the boundary")
			}
			if result.Outcome != OutcomeFailed {
				t.Errorf("Outcome = %s, want failed", result.Outcome)
			}
			if result.Health == nil || result.Health.Kind != tc.wantKind {
				t.Fatalf("Health = %v, want typed kind %s", result.Health, tc.wantKind)
			}
			if got := db.identityChecks.Load() - before; got != 1 {
				t.Errorf("boundary performed %d identity checks, want exactly 1", got)
			}

			// Every subsequent boundary keeps blocking before further work.
			after := db.RunRequest(context.Background(), func(ctx context.Context, conn *sql.Conn) error { return nil })
			if after.Health == nil || after.Health.Kind != tc.wantKind {
				t.Errorf("next boundary Health = %v, want kind %s", after.Health, tc.wantKind)
			}
		})
	}
}

// TestPostErrorReclassificationPrecedence pins the race contract: a raced
// identity change discovered after a failing request immediately reclassifies
// the result as deletion or replacement ahead of any ordinary SQLite error
// handling.
func TestPostErrorReclassificationPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(t *testing.T, db *DB, path string)
		wantKind HealthKind
	}{
		{name: "raced deletion beats ordinary error", mutate: func(t *testing.T, db *DB, path string) { os.Remove(path) }, wantKind: HealthDeleted},
		{name: "raced replacement beats ordinary error", mutate: func(t *testing.T, db *DB, path string) { replacePath(t, path, db.startIno) }, wantKind: HealthReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "post-error.db")
			createDatabase(t, path)
			db := mustOpen(t, path)

			ctx := context.Background()
			started := make(chan struct{})
			release := make(chan struct{})
			results := make(chan RequestResult, 1)
			go func() {
				results <- db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
					close(started)
					<-release
					// A real SQLite error: the statement names no existing table.
					var n int
					return conn.QueryRowContext(ctx, "SELECT v FROM definitely_missing_table").Scan(&n)
				})
			}()

			// Barrier: the replacement happens strictly after the request's
			// successful precheck and while the request is in flight.
			<-started
			tc.mutate(t, db, path)
			close(release)

			result := <-results
			if result.Outcome != OutcomeFailed {
				t.Errorf("Outcome = %s, want failed", result.Outcome)
			}
			if result.Health == nil || result.Health.Kind != tc.wantKind {
				t.Fatalf("post-error Health = %v, want typed kind %s taking precedence", result.Health, tc.wantKind)
			}
			if result.Err == nil {
				t.Error("original request error was discarded; classification must preserve underlying causes alongside the typed outcome")
			}
		})
	}

	t.Run("ordinary failure without identity change preserves SQLite cause", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plain-error.db")
		createDatabase(t, path)
		db := mustOpen(t, path)

		result := db.RunRequest(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
			var n int
			return conn.QueryRowContext(ctx, "SELECT v FROM definitely_missing_table").Scan(&n)
		})
		if result.Outcome != OutcomeFailed || result.Err == nil {
			t.Fatalf("result = %+v, want failed with the SQLite cause retained", result)
		}
		if result.Health != nil {
			t.Errorf("Health = %v, want nil for an ordinary failure", result.Health)
		}
	})
}

// TestRacedReplacementThenSuccessStands pins that a replacement occurring
// after a request's successful precheck does not discard a result the request
// legitimately produced; the next boundary detects the replacement before any
// further database work begins.
func TestRacedReplacementThenSuccessStands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race-success.db")
	createDatabase(t, path)
	db := mustOpen(t, path)
	ctx := context.Background()

	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan RequestResult, 1)
	go func() {
		results <- db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
			close(started)
			<-release
			var v string
			return conn.QueryRowContext(ctx, "SELECT v FROM t WHERE id = 1").Scan(&v)
		})
	}()

	<-started
	replacePath(t, path, db.startIno)
	close(release)

	result := <-results
	if result.Outcome != OutcomeSuccess {
		t.Fatalf("Outcome = %s, Err = %v; a successful result must stand despite the raced replacement", result.Outcome, result.Err)
	}
	if result.Health != nil {
		t.Errorf("Health = %v on a settled successful request; the next boundary owns detection", result.Health)
	}

	next := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
		t.Error("next request performed work before detecting the replacement")
		return nil
	})
	if next.Outcome != OutcomeFailed || next.Health == nil || next.Health.Kind != HealthReplaced {
		t.Fatalf("next result = %+v, want failed with typed replacement before work", next)
	}
}

// TestPhasedWriteReceivesExactlyOnePreBEGINCheck proves the entire phased
// write transaction is one request: exactly one identity check occurs before
// BEGIN and none between statement execution and COMMIT, for both committed
// and rolled-back phases.
func TestPhasedWriteReceivesExactlyOnePreBEGINCheck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		commit bool
	}{
		{name: "committed phase", commit: true},
		{name: "rolled-back phase", commit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "phased-write.db")
			createDatabase(t, path)
			db := mustOpen(t, path)
			ctx := context.Background()

			before := db.identityChecks.Load()
			samples := make([]int64, 3)
			result := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer tx.Rollback()                   // no-op after Commit; safe either way
				samples[0] = db.identityChecks.Load() // immediately after BEGIN

				if _, err := tx.ExecContext(ctx, "INSERT INTO t VALUES (2, 'two')"); err != nil {
					return err
				}
				samples[1] = db.identityChecks.Load() // between statement and COMMIT

				if tc.commit {
					return tx.Commit()
				}
				return tx.Rollback()
			})
			samples[2] = db.identityChecks.Load()

			if result.Outcome != OutcomeSuccess {
				t.Fatalf("phased write Outcome = %s, Err = %v, want success", result.Outcome, result.Err)
			}
			if got := db.identityChecks.Load() - before; got != 1 {
				t.Errorf("whole write received %d identity checks, want exactly 1 pre-BEGIN check", got)
			}
			for i, s := range samples {
				if s != before+1 {
					t.Errorf("check counter sample %d = %d, want no check inside the transaction (base %d + 1)", i, s, before)
				}
			}

			var n int64
			if err := db.SQL.QueryRow("SELECT COUNT(*) FROM t").Scan(&n); err != nil {
				t.Fatalf("verifying transaction outcome: %v", err)
			}
			if tc.commit && n != 2 {
				t.Errorf("row count after commit = %d, want 2", n)
			}
			if !tc.commit && n != 1 {
				t.Errorf("row count after rollback = %d, want 1", n)
			}
		})
	}
}
