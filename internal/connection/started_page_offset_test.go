// Issue #71 started later-page offset contract: StartPage receives the
// requested nonzero logical offset and passes it to the shared scanner so
// oversized-value failures report the one-based absolute logical result
// position (offset + page-relative index + 1), not a page-relative position.
// The same logical offset is rendered into PageSQL's LIMIT/OFFSET range and
// passed separately to the execution boundary. Offset-zero controls for
// StartFirstPage and StartPage, cancellation and ordinary-error controls, and
// assertions that the supplied execution offset matches the SQL request's
// intended range are all covered here. Page-envelope (KindPage) failures are
// a cache-layer concept tested in internal/resultcache; the connection layer
// produces only value-limit (KindValue) failures.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// createOffsetFixture builds a fixture with smallRows one-byte BLOB rows
// followed by one oversized BLOB row at absolute position smallRows+1, so
// later-page requests with nonzero offsets can verify value-limit failure
// positions are absolute (offset + page-relative index + 1). The fixture is
// written through a plain modernc connection with default limits so the
// oversized value can exist; it is read through the limited DB.
func createOffsetFixture(t *testing.T, path string, smallRows int) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE objs (id INTEGER PRIMARY KEY, b BLOB)`); err != nil {
		t.Fatalf("creating fixture table: %v", err)
	}
	stmt, err := db.Prepare(`INSERT INTO objs (id, b) VALUES (?, ?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := 1; i <= smallRows; i++ {
		if _, err := stmt.Exec(int64(i), []byte{byte(i)}); err != nil {
			t.Fatalf("insert small row %d: %v", i, err)
		}
	}
	// The oversized row at absolute position smallRows+1.
	oversized := make([]byte, int(sqlMaxLengthBytes)+1)
	if _, err := stmt.Exec(int64(smallRows+1), oversized); err != nil {
		t.Fatalf("insert oversized row: %v", err)
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("close stmt: %v", err)
	}
}

// pageSQLWithRange renders a simple page statement over the fixture with the
// exact LIMIT/OFFSET range, matching QueryBuilder's PageSQL contract.
func pageSQLWithRange(limit, offset int64) string {
	return "SELECT id, b FROM objs ORDER BY id LIMIT " +
		strconv.FormatInt(limit, 10) + " OFFSET " + strconv.FormatInt(offset, 10)
}

// TestStartPageValueFailureAbsolutePositionWithNonZeroOffset proves the core
// Issue #71 contract: StartPage receives the requested logical offset and
// passes it to the shared scanner, so an oversized value at page-relative
// index i on a request with offset O fails typed at the one-based absolute
// position O + i + 1 with the exact shared message and no partial failing row.
// Several nonzero OFFSET values exercise both first and later page-relative
// rows.
func TestStartPageValueFailureAbsolutePositionWithNonZeroOffset(t *testing.T) {
	const smallRows = 3 // oversized row is at absolute position 4

	cases := []struct {
		name        string
		offset      int64
		limit       int64
		relativeIdx int64 // 0-based page-relative index of the oversized row
	}{
		{"first relative row at offset 3", 3, 1, 0},
		{"second relative row at offset 2", 2, 2, 1},
		{"third relative row at offset 1", 1, 3, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/offset_fixture.db"
			createOffsetFixture(t, path, smallRows)
			db, err := Open(path)
			if err != nil {
				t.Fatalf("Open error = %v", err)
			}
			t.Cleanup(func() { db.Close() })

			statement := pageSQLWithRange(tc.limit, tc.offset)
			started := db.StartPage(context.Background(), statement, nil, tc.offset)
			page, res := started.Wait()

			if res.Outcome != OutcomeFailed {
				t.Fatalf("outcome = %v, want failed", res.Outcome)
			}
			var failure *result.LimitFailure
			if res.Err == nil || !errors.As(res.Err, &failure) {
				t.Fatalf("err = %v, want typed *result.LimitFailure", res.Err)
			}
			wantPos := tc.offset + tc.relativeIdx + 1
			if failure.Kind != result.KindValue || failure.Position != wantPos {
				t.Fatalf("failure = %+v, want {KindValue, position %d}", failure, wantPos)
			}
			wantMsg := "result value exceeds the 64 MiB v1 limit at row " + strconv.FormatInt(wantPos, 10)
			if got := res.Err.Error(); got != wantMsg {
				t.Fatalf("err text = %q, want %q", got, wantMsg)
			}
			if res.Health != nil {
				t.Fatalf("health = %v, want nil (an oversized value is an ordinary typed failure)", res.Health)
			}

			// No partial failing row: the oversized row contributes nothing.
			// Earlier complete rows are retained with exact bytes; when the
			// oversized row is the first in the page (relativeIdx 0), the
			// driver may surface SQLITE_TOOBIG at execution time before any
			// row is scanned, so the partial page is nil or empty.
			if page != nil && int64(len(page.Rows)) != tc.relativeIdx {
				t.Fatalf("retained rows = %d, want %d complete leading rows", len(page.Rows), tc.relativeIdx)
			}
			if page != nil {
				for i, row := range page.Rows {
					if row[0].Int != int64(i+1)+tc.offset {
						t.Fatalf("retained row %d id = %d, want %d", i, row[0].Int, int64(i+1)+tc.offset)
					}
				}
			}
		})
	}
}

// TestStartPageOffsetZeroControlMatchesFirstPage proves that StartPage with
// offset zero behaves exactly like the first page: the oversized value at the
// fixture's last row fails at the same absolute position as RunFirstPage.
func TestStartPageOffsetZeroControlMatchesFirstPage(t *testing.T) {
	const smallRows = 3 // oversized row is at absolute position 4

	path := t.TempDir() + "/offset_zero.db"
	createOffsetFixture(t, path, smallRows)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	statement := pageSQLWithRange(smallRows+1, 0)
	started := db.StartPage(context.Background(), statement, nil, 0)
	page, res := started.Wait()

	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	var failure *result.LimitFailure
	if !errors.As(res.Err, &failure) || failure.Kind != result.KindValue || failure.Position != 4 {
		t.Fatalf("failure = %v (%v), want {KindValue, position 4}", failure, res.Err)
	}
	if got := res.Err.Error(); got != "result value exceeds the 64 MiB v1 limit at row 4" {
		t.Fatalf("err text = %q, want the exact shared message", got)
	}
	if page == nil || len(page.Rows) != smallRows {
		t.Fatalf("retained rows = %d, want %d complete leading rows", len(page.Rows), smallRows)
	}
}

// TestStartFirstPageOffsetZeroControlUnchanged proves StartFirstPage keeps its
// fixed offset-zero behavior: the oversized value at position 4 fails at
// position 4, exactly as before Issue #71.
func TestStartFirstPageOffsetZeroControlUnchanged(t *testing.T) {
	const smallRows = 3

	path := t.TempDir() + "/first_page_zero.db"
	createOffsetFixture(t, path, smallRows)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	started := db.StartFirstPage(context.Background(), pageSQLWithRange(smallRows+1, 0), nil)
	page, res := started.Wait()

	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	var failure *result.LimitFailure
	if !errors.As(res.Err, &failure) || failure.Kind != result.KindValue || failure.Position != 4 {
		t.Fatalf("failure = %v (%v), want {KindValue, position 4}", failure, res.Err)
	}
	if page == nil || len(page.Rows) != smallRows {
		t.Fatalf("retained rows = %d, want %d complete leading rows", len(page.Rows), smallRows)
	}
}

// TestStartPageOrdinaryErrorControlWithOffset proves StartPage with a nonzero
// offset still returns ordinary query errors with the cause preserved, not a
// typed limit failure.
func TestStartPageOrdinaryErrorControlWithOffset(t *testing.T) {
	db := openMixed(t)

	started := db.StartPage(context.Background(), `SELECT * FROM "mix" WHERE no_such_column = 1`, nil, 2)
	_, res := started.Wait()

	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no such column") {
		t.Fatalf("err = %v, want the driver cause preserved", res.Err)
	}
	var failure *result.LimitFailure
	if errors.As(res.Err, &failure) {
		t.Fatalf("ordinary query error classified as *result.LimitFailure: %+v", failure)
	}
}

// TestStartPagePreLeaseCancellationControlWithOffset proves StartPage with a
// nonzero offset still settles as cancelled when the context is cancelled
// before the lease is acquired, with no page returned.
func TestStartPagePreLeaseCancellationControlWithOffset(t *testing.T) {
	db := openMixed(t)

	// Hold both pool connections so the third request queues for a lease.
	hold1, _ := db.Lease(context.Background())
	hold2, _ := db.Lease(context.Background())
	t.Cleanup(func() {
		hold1.Release(context.Background())
		hold2.Release(context.Background())
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-lease cancellation

	started := db.StartPage(ctx, `SELECT id FROM "mix"`, nil, 5)
	page, res := started.Wait()

	if res.Outcome != OutcomeCancelled {
		t.Fatalf("outcome = %v, want cancelled", res.Outcome)
	}
	if page != nil {
		t.Fatalf("cancelled pre-lease page returned rows, want nil")
	}
}

// TestStartPageOffsetMatchesSQLRange proves the supplied execution offset
// matches the SQL request's intended OFFSET range: for several nonzero
// offsets, the failure position equals the SQL's OFFSET plus the page-relative
// index plus one, confirming the same logical offset reaches the scanner that
// QueryBuilder rendered into the statement.
func TestStartPageOffsetMatchesSQLRange(t *testing.T) {
	const smallRows = 3 // oversized row at absolute position 4

	for _, offset := range []int64{1, 2, 3} {
		t.Run("offset_"+strconv.FormatInt(offset, 10), func(t *testing.T) {
			path := t.TempDir() + "/range_check.db"
			createOffsetFixture(t, path, smallRows)
			db, err := Open(path)
			if err != nil {
				t.Fatalf("Open error = %v", err)
			}
			t.Cleanup(func() { db.Close() })

			limit := int64(smallRows+1) - offset
			statement := pageSQLWithRange(limit, offset)
			started := db.StartPage(context.Background(), statement, nil, offset)
			_, res := started.Wait()

			if res.Outcome != OutcomeFailed {
				t.Fatalf("outcome = %v, want failed", res.Outcome)
			}
			var failure *result.LimitFailure
			if !errors.As(res.Err, &failure) {
				t.Fatalf("err = %v, want typed *result.LimitFailure", res.Err)
			}
			// The SQL's OFFSET equals the supplied execution offset, so the
			// absolute position is offset + (smallRows+1 - offset - 1) + 1 =
			// smallRows + 1 = 4, regardless of the offset value.
			wantPos := int64(smallRows + 1)
			if failure.Position != wantPos {
				t.Fatalf("offset %d: failure position = %d, want %d (the SQL OFFSET + relative index + 1)",
					offset, failure.Position, wantPos)
			}
		})
	}
}
