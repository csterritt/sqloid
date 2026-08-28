//go:build unix

// SQLite-backed integration coverage for the tracer boundary, per Issue #10
// Task 1: one catalog-selected eligible table flows through Schema and
// Connection into typed column names, rows, and errors without any UI
// database logic. Proves typed value transport, a safe unusual identifier,
// and a basic SQLite failure only; no builder, validation, paging, count,
// history, cancellation, or write behavior may be implied.

package schema_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/connection"
	schema "github.com/chris/sqloid/internal/schema"
)

// openTracerFixture builds a small fixture containing an ordinary table with
// mixed INTEGER/TEXT values plus a table whose cataloged name embeds a double
// quote, then opens it through Connection's validated startup boundary.
func openTracerFixture(t *testing.T) (*connection.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tracer.db")

	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening fixture %q: %v", path, err)
	}
	defer writer.Close()
	statements := []string{
		`CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO albums VALUES (1, 'one'), (2, 'two')`,
		`CREATE TABLE "odd ""name" (a INTEGER)`,
		`INSERT INTO "odd ""name" VALUES (7)`,
	}
	for _, stmt := range statements {
		if _, err := writer.Exec(stmt); err != nil {
			t.Fatalf("creating tracer fixture (%s): %v", stmt, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing fixture writer: %v", err)
	}

	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

// readFixtureCatalog performs one successful catalog read so tests always
// compose from Schema's validated snapshot rather than assumptions.
func readFixtureCatalog(t *testing.T, db *connection.DB) *schema.Catalog {
	t.Helper()
	cat, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess || cat == nil {
		t.Fatalf("ReadCatalog result = %+v, want success with catalog", res)
	}
	return cat
}

// TestTracerBoundaryTypedRows pins that an ordinary cataloged table executes
// as one hardcoded quoted SELECT * and returns typed columns and rows: values
// must keep their driver types (int64 for INTEGER, string for TEXT) so the UI
// can compose deterministic rendering later without re-deriving anything.
func TestTracerBoundaryTypedRows(t *testing.T) {
	db, _ := openTracerFixture(t)
	cat := readFixtureCatalog(t, db)

	obj, err := schema.ChooseTracerTarget(cat, "albums")
	if err != nil {
		t.Fatalf("ChooseTracerTarget(\"albums\") error = %v", err)
	}
	out, tres := db.RunTraceSelectAll(t.Context(), obj)
	if tres.Outcome != connection.OutcomeSuccess || tres.Err != nil {
		t.Fatalf("RunTraceSelectAll result = %+v, want success", tres)
	}
	if out == nil {
		t.Fatal("successful trace returned no result")
	}
	if len(out.Columns) != 2 || out.Columns[0] != "id" || out.Columns[1] != "title" {
		t.Errorf("columns = %v, want [id title]", out.Columns)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(out.Rows))
	}
	if out.Rows[0][0] != int64(1) {
		t.Errorf("rows[0][0] = %#v of type %T, want int64 1", out.Rows[0][0], out.Rows[0][0])
	}
	if out.Rows[0][1] != "one" {
		t.Errorf("rows[0][1] = %#v of type %T, want string \"one\"", out.Rows[0][1], out.Rows[0][1])
	}
}

// TestTracerBoundaryUnusualIdentifier proves an embedded double quote in the
// catalog name is identifier-quoted safely rather than interpolated raw.
func TestTracerBoundaryUnusualIdentifier(t *testing.T) {
	db, _ := openTracerFixture(t)
	cat := readFixtureCatalog(t, db)

	obj, err := schema.ChooseTracerTarget(cat, `odd "name`)
	if err != nil {
		t.Fatalf("ChooseTracerTarget error = %v", err)
	}
	out, tres := db.RunTraceSelectAll(t.Context(), obj)
	if tres.Outcome != connection.OutcomeSuccess || tres.Err != nil {
		t.Fatalf("RunTraceSelectAll result = %+v, want success", tres)
	}
	if len(out.Rows) != 1 || out.Rows[0][0] != int64(7) {
		t.Errorf("rows = %v, want [[7]] with int64 value", out.Rows)
	}
}

// TestTracerBoundaryBasicFailure proves a basic SQLite failure surfaces as a
// failed RequestResult carrying the wrapped cause instead of crashing.
func TestTracerBoundaryBasicFailure(t *testing.T) {
	db, path := openTracerFixture(t)
	cat := readFixtureCatalog(t, db)

	obj, err := schema.ChooseTracerTarget(cat, "albums")
	if err != nil {
		t.Fatalf("ChooseTracerTarget error = %v", err)
	}
	// Drop the underlying table after selection so execution alone fails:
	// exactly a basic query failure at the request boundary.
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening fixture for drop: %v", err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`DROP TABLE albums`); err != nil {
		t.Fatalf("dropping fixture table: %v", err)
	}

	out, tres := db.RunTraceSelectAll(t.Context(), obj)
	if tres.Outcome != connection.OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", tres.Outcome)
	}
	if tres.Err == nil {
		t.Error("failed outcome carried no cause")
	} else if !strings.Contains(tres.Err.Error(), "albums") && !strings.Contains(strings.ToLower(tres.Err.Error()), "select") {
		t.Errorf("error %q neither names the failing object nor its failing statement", tres.Err.Error())
	}
	if out != nil {
		t.Errorf("failed trace returned non-nil result %v", out)
	}
}
