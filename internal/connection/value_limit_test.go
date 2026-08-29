// Issue #31 connection-local oversized-value coverage: one value exceeding
// the connection-local 64 MiB SQLITE_LIMIT_LENGTH fails typed at the SQLite
// scan boundary with the exact shared message and the one-based absolute
// logical result position, the oversized row is never exposed or retained
// partially, and every earlier complete row keeps its exact bytes — BLOBs
// byte-for-byte. A boundary-sized value (exactly 64 MiB) succeeds.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// createOversizedValueDatabase builds a fixture with two BLOB rows: row 1 is
// exactly 64 MiB (the boundary size, which the connection must accept) and
// row 2 is 64 MiB + 1 byte, which exceeds the connection-local limit. The
// fixture is written through a plain modernc connection with default limits
// so the oversized value can exist at all; it is read through the limited DB.
func createOversizedValueDatabase(t *testing.T, path string, boundary, oversized int) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE blobs (id INTEGER PRIMARY KEY, b BLOB)`); err != nil {
		t.Fatalf("creating fixture table: %v", err)
	}
	stmt, err := db.Prepare(`INSERT INTO blobs (id, b) VALUES (?, ?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for _, spec := range []struct {
		id   int64
		size int
	}{
		{1, boundary},
		{2, oversized},
	} {
		payload := make([]byte, spec.size)
		for i := range payload {
			payload[i] = byte(spec.id)
		}
		if _, err := stmt.Exec(spec.id, payload); err != nil {
			t.Fatalf("insert row %d: %v", spec.id, err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("close stmt: %v", err)
	}
}

func TestRunFirstPageValueLimitTypedFailure(t *testing.T) {
	path := t.TempDir() + "/oversized.db"
	createOversizedValueDatabase(t, path, int(sqlMaxLengthBytes), int(sqlMaxLengthBytes)+1)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	page, res := db.RunFirstPage(context.Background(), `SELECT id, b FROM blobs ORDER BY id`, nil)
	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	var failure *result.LimitFailure
	if res.Err == nil || !errors.As(res.Err, &failure) {
		t.Fatalf("err = %v, want typed *result.LimitFailure", res.Err)
	}
	if failure.Kind != result.KindValue || failure.Position != 2 {
		t.Fatalf("failure = %+v, want {KindValue, position 2}", failure)
	}
	if got := res.Err.Error(); got != "result value exceeds the 64 MiB v1 limit at row 2" {
		t.Fatalf("err text = %q, want the exact shared message", got)
	}
	if res.Health != nil {
		t.Fatalf("health = %v, want nil (an oversized value is an ordinary typed failure)", res.Health)
	}

	// The earlier complete row is retained with exact BLOB bytes; the failing
	// row contributes nothing — no partial row, no bytes from row 2.
	if page == nil {
		t.Fatal("typed limit failure lost the earlier complete rows")
	}
	if len(page.Rows) != 1 {
		t.Fatalf("retained rows = %d, want only the complete leading row", len(page.Rows))
	}
	if page.Rows[0][0].Int != 1 {
		t.Fatalf("retained row id = %d, want 1", page.Rows[0][0].Int)
	}
	blob := page.Rows[0][1]
	if blob.Kind != result.KindBlob || len(blob.Bytes) != int(sqlMaxLengthBytes) {
		t.Fatalf("retained BLOB = kind %v len %d, want BLOB of exactly the boundary size", blob.Kind, len(blob.Bytes))
	}
	for i, b := range blob.Bytes {
		if b != 1 {
			t.Fatalf("BLOB byte %d = %#x, want 0x01 (exact bytes preserved)", i, b)
			break
		}
	}
}

func TestRunFirstPageBoundaryValueSucceeds(t *testing.T) {
	// A value of exactly 64 MiB sits at the connection-local limit and must
	// not be classified as oversized.
	path := t.TempDir() + "/boundary.db"
	createOversizedValueDatabase(t, path, int(sqlMaxLengthBytes), int(sqlMaxLengthBytes))
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	page, res := db.RunFirstPage(context.Background(), `SELECT id, b FROM blobs ORDER BY id`, nil)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success for boundary-sized values", res.Outcome, res.Err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("rows = %d, want both boundary rows", len(page.Rows))
	}
}

func TestExecutePageValueLimitNonFirstPagePosition(t *testing.T) {
	// On a non-first page the one-based absolute logical result position
	// includes the page's offset: the failing row is the first scanned row of
	// an OFFSET-1 page, so the failure names row 2.
	path := t.TempDir() + "/oversized_page.db"
	createOversizedValueDatabase(t, path, int(sqlMaxLengthBytes), int(sqlMaxLengthBytes)+1)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	page, res := db.ExecutePage(context.Background(), `SELECT id, b FROM blobs ORDER BY id LIMIT 1 OFFSET 1`, nil, 1)
	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	var failure *result.LimitFailure
	if !errors.As(res.Err, &failure) || failure.Kind != result.KindValue || failure.Position != 2 {
		t.Fatalf("failure = %v (%v), want {KindValue, position 2}", failure, res.Err)
	}
	if got := res.Err.Error(); got != "result value exceeds the 64 MiB v1 limit at row 2" {
		t.Fatalf("err text = %q, want the exact shared message", got)
	}
	if page != nil && len(page.Rows) != 0 {
		t.Fatalf("non-first oversized page retained %d rows, want none from the failing row", len(page.Rows))
	}
	if !strings.Contains(res.Err.Error(), "row 2") {
		t.Fatal("position missing")
	}
}
