// SQLite-backed boundary tests for the disposable tracer execution path, per
// Issue #10 Task 1: one hardcoded safe SELECT * chosen from the schema
// catalog executes through Connection's RunRequest boundary and returns typed
// column names, rows, or errors suitable for composition, with no UI database
// logic. Only this minimal execution contract is proven — no builder,
// validation, paging, count, history, cancellation, or write behavior exists
// at this milestone and none may be implied.

package connection

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// openTracerDatabase creates a fixture database holding one ordinary table
// plus a table whose name embeds a double quote, then opens it through the
// validated startup boundary.
func openTracerDatabase(t *testing.T) *DB {
	t.Helper()
	path := t.TempDir() + "/tracer.db"

	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening fixture %q: %v", path, err)
	}
	defer writer.Close()
	statements := []string{
		`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT); INSERT INTO t VALUES (1, 'one'), (2, 'two')`,
		`CREATE TABLE "odd ""name" (a INTEGER); INSERT INTO "odd ""name" VALUES (7)`,
	}
	for _, stmt := range statements {
		if _, err := writer.Exec(stmt); err != nil {
			t.Fatalf("creating tracer fixture (%s): %v", stmt, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing fixture writer: %v", err)
	}
	return mustOpen(t, path)
}

func TestRunTraceSelectAllReturnsTypedRows(t *testing.T) {
	db := openTracerDatabase(t)
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess || cat == nil {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	obj, err := schema.ChooseTracerTarget(cat, "t")
	if err != nil {
		t.Fatalf("ChooseTracerTarget error = %v", err)
	}

	out, tres := db.RunTraceSelectAll(context.Background(), obj)
	if tres.Outcome != OutcomeSuccess || tres.Err != nil {
		t.Fatalf("RunTraceSelectAll result = %+v, want success", tres)
	}
	if out == nil {
		t.Fatal("successful trace returned no result")
	}
	if len(out.Columns) != 2 || out.Columns[0] != "id" || out.Columns[1] != "v" {
		t.Errorf("columns = %v, want [id v]", out.Columns)
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

// TestRunTraceSelectAllQuotedIdentifier proves Connection executes exactly
// Schema's safely quoted SQL against an unusual cataloged identifier without
// revalidating or altering it.
func TestRunTraceSelectAllQuotedIdentifier(t *testing.T) {
	db := openTracerDatabase(t)
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess || cat == nil {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	obj, err := schema.ChooseTracerTarget(cat, `odd "name`)
	if err != nil {
		t.Fatalf("ChooseTracerTarget error = %v", err)
	}

	out, tres := db.RunTraceSelectAll(context.Background(), obj)
	if tres.Outcome != OutcomeSuccess || tres.Err != nil {
		t.Fatalf("RunTraceSelectAll result = %+v, want success", tres)
	}
	if out == nil || len(out.Rows) != 1 || out.Rows[0][0] != int64(7) {
		t.Errorf("result rows = %v, want [[7]] with int64 value", out)
	}
}

// TestRunTraceSelectAllBasicFailure proves a basic query failure settles as a
// failed RequestResult whose cause is preserved through wrapping.
func TestRunTraceSelectAllBasicFailure(t *testing.T) {
	db := openTracerDatabase(t)
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess || cat == nil {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	obj, err := schema.ChooseTracerTarget(cat, "t")
	if err != nil {
		t.Fatalf("ChooseTracerTarget error = %v", err)
	}

	missing := *obj
	missing.Name = "no_such_table"
	out, tres := db.RunTraceSelectAll(context.Background(), &missing)
	if tres.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", tres.Outcome)
	}
	if tres.Err == nil {
		t.Fatal("failed outcome carried no cause")
	} else if !strings.Contains(tres.Err.Error(), "no_such_table") {
		t.Errorf("error %q does not identify the failing object", tres.Err.Error())
	} else if strings.Contains(strings.ToLower(tres.Err.Error()), "terminal") {
		t.Errorf("error %q claims terminal classification owned elsewhere", tres.Err.Error())
	}
	if out != nil {
		t.Errorf("failed trace returned non-nil result %v", out)
	}
}
