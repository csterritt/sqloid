// First-page SELECT execution coverage (Issue #22 Tasks 3–4): the production
// first-page method on the Connection boundary executes exactly one bound
// SELECT through the shared RunRequest rules (Issue #6 lifecycle, Issue #7
// health classification) on a dedicated leased connection and eagerly scans
// original driver labels plus typed SQLite values — NULL, INTEGER, REAL,
// TEXT, and byte-exact BLOBs — converting rows once into internal/result
// without string coercion. Query errors are ordinary typed failures; the
// request lifecycle and health classification stay owned by RunRequest.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/chris/sqloid/internal/result"
)

// createMixedDatabase builds a fixture with one row of every SQLite storage
// class, including invalid UTF-8 TEXT and binary BLOB payloads, so typed scan
// behavior is observable end to end.
func createMixedDatabase(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", path, err)
	}
	defer db.Close()
	const ddl = `
CREATE TABLE mix (id INTEGER PRIMARY KEY, r REAL, t TEXT, b BLOB);
INSERT INTO mix VALUES (1, 1.5, 'plain', x'00ABFF');
INSERT INTO mix VALUES (2, -0.25, cast(x'626164E08080' AS TEXT), x'');
CREATE TABLE empty_table (id INTEGER);
`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("creating mixed fixture: %v", err)
	}
}

func openMixed(t *testing.T) *DB {
	t.Helper()

	path := t.TempDir() + "/mix.db"
	createMixedDatabase(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunFirstPageTypedRowsAndLabels(t *testing.T) {
	db := openMixed(t)

	page, res := db.RunFirstPage(context.Background(), `SELECT id, r, t, b FROM "mix" ORDER BY id`, nil)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v, health %v), want success", res.Outcome, res.Err, res.Health)
	}
	if page == nil {
		t.Fatal("success returned no page")
	}
	wantCols := []string{"id", "r", "t", "b"}
	if len(page.Columns) != len(wantCols) {
		t.Fatalf("columns = %q, want %q", page.Columns, wantCols)
	}
	for i, want := range wantCols {
		if page.Columns[i] != want {
			t.Errorf("columns[%d] = %q, want %q", i, page.Columns[i], want)
		}
	}
	if len(page.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(page.Rows))
	}

	first := page.Rows[0]
	if first[0].Kind != result.KindInteger || first[0].Int != 1 {
		t.Errorf("INTEGER cell = %+v, want Integer 1", first[0])
	}
	if first[1].Kind != result.KindReal || first[1].Float != 1.5 {
		t.Errorf("REAL cell = %+v, want Real 1.5", first[1])
	}
	if first[2].Kind != result.KindText || first[2].Str != "plain" {
		t.Errorf("TEXT cell = %+v, want Text plain", first[2])
	}
	if first[3].Kind != result.KindBlob || !equalBytes(first[3].Bytes, []byte{0x00, 0xAB, 0xFF}) {
		t.Errorf("BLOB cell = %+v, want exact bytes 00ABFF", first[3])
	}

	second := page.Rows[1]
	if second[0].Int != 2 {
		t.Errorf("second INTEGER = %+v, want 2", second[0])
	}
	if second[2].Kind != result.KindText || second[2].Str == "bad\xE0\x80\x80" {
		t.Errorf("invalid UTF-8 TEXT not decoded: %q", second[2].Str)
	}
	if decoded, _ := result.DecodeText("bad\xE0\x80\x80"); second[2].Str != decoded {
		t.Errorf("TEXT cell = %q, want decoded %q", second[2].Str, decoded)
	}
	if !page.InvalidUTF {
		t.Error("invalid UTF-8 TEXT did not set page metadata")
	}
	if second[3].Kind != result.KindBlob || len(second[3].Bytes) != 0 {
		t.Errorf("empty BLOB = %+v, want zero-byte BLOB", second[3])
	}
}

func TestRunFirstPageNullsTyped(t *testing.T) {
	db := openMixed(t)

	page, res := db.RunFirstPage(context.Background(), "SELECT NULL, 7, 2.5, 'text'", nil)
	if res.Outcome != OutcomeSuccess || page == nil {
		t.Fatalf("outcome = %v, page %v", res.Outcome, page)
	}
	cells := page.Rows[0]
	if cells[0].Kind != result.KindNull {
		t.Errorf("NULL cell = %+v, want KindNull", cells[0])
	}
	if cells[1].Kind != result.KindInteger || cells[1].Int != 7 {
		t.Errorf("INTEGER cell = %+v, want Integer 7", cells[1])
	}
	if cells[2].Kind != result.KindReal || cells[2].Float != 2.5 {
		t.Errorf("REAL cell = %+v, want Real 2.5", cells[2])
	}
	if cells[3].Kind != result.KindText || cells[3].Str != "text" {
		t.Errorf("TEXT cell = %+v, want Text text", cells[3])
	}
}

func TestRunFirstPageZeroRows(t *testing.T) {
	db := openMixed(t)

	page, res := db.RunFirstPage(context.Background(), `SELECT id FROM "empty_table"`, nil)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success", res.Outcome, res.Err)
	}
	if page == nil {
		t.Fatal("zero-row success returned no page")
	}
	if len(page.Rows) != 0 {
		t.Errorf("rows = %d, want 0", len(page.Rows))
	}
	if len(page.Columns) != 1 || page.Columns[0] != "id" {
		t.Errorf("columns = %q, want [id]", page.Columns)
	}
	if page.InvalidUTF {
		t.Error("zero-row page reported invalid UTF-8")
	}
}

func TestRunFirstPageQueryErrorOrdinaryFailure(t *testing.T) {
	db := openMixed(t)

	// A post-validation DDL race (the target table was dropped between
	// validation and the page request) is an ordinary query error, not a
	// special path.
	page, res := db.RunFirstPage(context.Background(), `SELECT id FROM "dropped_table"`, nil)
	if page != nil {
		t.Error("failed page request returned a page")
	}
	if res.Outcome != OutcomeFailed {
		t.Errorf("outcome = %v, want failed", res.Outcome)
	}
	if res.Err == nil {
		t.Fatal("failure carried no error")
	}
	if !strings.Contains(res.Err.Error(), "no such table") {
		t.Errorf("error text %q does not name the driver cause", res.Err.Error())
	}
	if res.Health != nil {
		t.Errorf("ordinary query error misclassified as health failure %v", res.Health)
	}
}

func TestRunFirstPageBindsParamsInOrder(t *testing.T) {
	db := openMixed(t)

	page, res := db.RunFirstPage(context.Background(), `SELECT id FROM "mix" WHERE t = ?`, []any{"plain"})
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success", res.Outcome, res.Err)
	}
	if len(page.Rows) != 1 || page.Rows[0][0].Int != 1 {
		t.Errorf("rows = %+v, want exactly id 1", page.Rows)
	}
}

// TestRunFirstPagePrecancelledContext pins the boundary classification for
// a request whose context is already cancelled when issued: no page is
// returned, the outcome is cancelled at this boundary (Issue #60: pre-lease
// cancellation classifies as the existing cancelled outcome), and the
// cancellation cause stays inspectable through errors.Is. (Mid-flight
// cancellation classification is owned and proven by the Issue #6 request
// lifecycle; Ctrl+W routing for SELECT pages arrives with later issues.)
func TestRunFirstPagePrecancelledContext(t *testing.T) {
	db := openMixed(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, res := db.RunFirstPage(ctx, `SELECT id FROM "mix"`, nil)
	if page != nil {
		t.Error("cancelled request returned a page")
	}
	if res.Outcome != OutcomeCancelled {
		t.Errorf("outcome = %v, want cancelled", res.Outcome)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("cancelled error %v does not unwrap to context.Canceled", res.Err)
	}
}

func TestRunFirstPageScanTypedWithoutStringCoercion(t *testing.T) {
	db := openMixed(t)

	// A REAL 1.5 must arrive as KindReal, and an INTEGER 1 as KindInteger:
	// identical-looking values must never collapse through strings.
	page, res := db.RunFirstPage(context.Background(), `SELECT CAST(1 AS REAL), CAST(1 AS INTEGER)`, nil)
	if res.Outcome != OutcomeSuccess || page == nil {
		t.Fatalf("outcome = %v, page %v", res.Outcome, page)
	}
	if page.Rows[0][0].Kind != result.KindReal || page.Rows[0][0].Float != 1 {
		t.Errorf("REAL cell = %+v, want Real 1", page.Rows[0][0])
	}
	if page.Rows[0][1].Kind != result.KindInteger || page.Rows[0][1].Int != 1 {
		t.Errorf("INTEGER cell = %+v, want Integer 1", page.Rows[0][1])
	}
	if page.Rows[0][0].Display() != "1.0" || page.Rows[0][1].Display() != "1" {
		t.Errorf("display tokens = %q %q, want \"1.0\" \"1\"", page.Rows[0][0].Display(), page.Rows[0][1].Display())
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
