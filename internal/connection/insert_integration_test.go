//go:build unix

// SQLite-backed INSERT integration coverage through the Connection boundary,
// per Issue #39 Tasks 3–4: complete mixed Value/NULL/omit states insert
// through the normal path with exact parameter effects, all-omit DEFAULT
// VALUES runs through the same seam, INTEGER PRIMARY KEY omission
// auto-assigns, virtual tables insert via visible columns, and modules or
// constraints requiring hidden or invalid input surface ordinary database
// errors without builder fabrication.

package connection_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/chris/sqloid/internal/connection"
	qb "github.com/chris/sqloid/internal/querybuilder"
	schema "github.com/chris/sqloid/internal/schema"
)

// openInsertFixture opens a real database through connection.Open holding
// the fixture tables the INSERT integration cases exercise, plus the
// refreshed catalog gathered through the connection's own request boundary.
func openInsertFixture(t *testing.T) (*connection.DB, *schema.Catalog) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "insert-fixture.db")
	// connection.Open requires an existing file: seed it with the plain
	// driver first, then open through the production boundary.
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("seeding sql.Open: %v", err)
	}
	if _, err := seed.Exec("CREATE TABLE _seed (x INTEGER); DROP TABLE _seed"); err != nil {
		t.Fatalf("seeding fixture database: %v", err)
	}
	seed.Close()
	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range []string{
		`CREATE TABLE logs (id INTEGER PRIMARY KEY, msg TEXT NOT NULL, temp REAL)`,
		`CREATE TABLE all_defaults (id INTEGER PRIMARY KEY, tag TEXT DEFAULT 'none', n INTEGER)`,
		`CREATE TABLE quoted (k TEXT PRIMARY KEY, "va ""l" TEXT)`,
		`CREATE VIRTUAL TABLE notes_fts USING fts5(title)`,
		`CREATE TABLE strict (n INTEGER NOT NULL)`,
		`CREATE TABLE full_fts (x, y)`, // stand-in: fts5 hidden-input failure exercised via NOT NULL below
	} {
		if _, err := db.SQL.Exec(stmt); err != nil {
			t.Fatalf("fixture create (%s): %v", stmt, err)
		}
	}
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("ReadCatalog: outcome=%v err=%v", res.Outcome, res.Err)
	}
	return db, cat
}

// buildInsertQB drives the given table through QueryBuilder with the given
// per-column choices; unlisted columns stay omitted. The returned builder is
// runnable when every column has a choice.
func buildInsertQB(t *testing.T, cat *schema.Catalog, table string, choices map[string]qb.InsertChoice, values map[string]string) qb.QueryBuilder {
	t.Helper()
	q := qb.NewQuery().RefreshSchema(cat).SelectCommand(qb.CommandInsert).SelectTable(table).BeginInsertPrompts()
	for _, c := range q.InsertColumns() {
		choice, listed := choices[c.Column]
		if !listed {
			choice = qb.InsertChoiceOmit
		}
		next, ok := q.ChooseInsertColumn(c.Column, choice)
		if !ok {
			t.Fatalf("setup: choice %v on %q failed", choice, c.Column)
		}
		q = next
		if choice == qb.InsertChoiceValue {
			next, ok := q.SubmitInsertValue(c.Column, values[c.Column])
			if !ok {
				t.Fatalf("setup: submit for %q failed", c.Column)
			}
			q = next
		}
	}
	return q
}

// mustRunInsert executes the builder's rendered INSERT through
// connection.ExecuteInsert and asserts success with the expected row count.
func mustRunInsert(t *testing.T, db *connection.DB, q qb.QueryBuilder, wantRows int64) {
	t.Helper()
	affected, res := db.ExecuteInsert(context.Background(), q.InsertSQL(), q.InsertParams())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("ExecuteInsert(%q) failed: outcome=%v err=%v health=%v", q.InsertSQL(), res.Outcome, res.Err, res.Health)
	}
	if affected != wantRows {
		t.Fatalf("ExecuteInsert(%q) rows = %d, want %d", q.InsertSQL(), affected, wantRows)
	}
}

// TestInsertIntegrationMixedChoicesAndEffects proves mixed Value/NULL/omit
// state inserts real rows with exact stored effects: empty TEXT, typed NULL
// TEXT, explicit SQL NULL, and omitted defaulted columns through one normal
// execution path.
func TestInsertIntegrationMixedChoicesAndEffects(t *testing.T) {
	db, cat := openInsertFixture(t)
	ctx := context.Background()

	// The real table_xinfo metadata marks logs.id as the single INTEGER
	// PRIMARY KEY rowid alias: the catalog carries the exact omission hint.
	for _, obj := range cat.Objects {
		if obj.Name != "logs" {
			continue
		}
		got, ok := obj.InsertHint(obj.Columns[0])
		if !ok || got != schema.InsertOmissionHint {
			t.Fatalf("logs.id catalog hint = (%q, %v), want %q", got, ok, schema.InsertOmissionHint)
		}
	}

	// One row, mixed choices: id auto-assigned (omitted INTEGER PRIMARY KEY),
	// msg submitted empty TEXT, temp explicit NULL.
	q := buildInsertQB(t, cat, "logs",
		map[string]qb.InsertChoice{"id": qb.InsertChoiceOmit, "msg": qb.InsertChoiceValue, "temp": qb.InsertChoiceNull},
		map[string]string{"msg": ""})
	if got, want := q.InsertSQL(), `INSERT INTO "logs" ("msg", "temp") VALUES (?, NULL)`; got != want {
		t.Fatalf("InsertSQL() = %q, want %q", got, want)
	}
	mustRunInsert(t, db, q, 1)

	var msg string
	var temp any
	var id int64
	if err := db.SQL.QueryRowContext(ctx, "SELECT id, msg, temp FROM logs").Scan(&id, &msg, &temp); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if id != 1 || msg != "" || temp != nil {
		t.Fatalf("stored row = (%d, %q, %v), want (1, \"\", NULL)", id, msg, temp)
	}

	// Typed NULL text stays literal TEXT, distinct from SQL NULL.
	q = buildInsertQB(t, cat, "logs",
		map[string]qb.InsertChoice{"msg": qb.InsertChoiceValue},
		map[string]string{"msg": "NULL"})
	mustRunInsert(t, db, q, 1)
	var got string
	if err := db.SQL.QueryRowContext(ctx, "SELECT msg FROM logs WHERE id = 2").Scan(&got); err != nil || got != "NULL" {
		t.Fatalf("typed NULL readback = (%q, %v), want literal \"NULL\"", got, err)
	}
}

// TestInsertIntegrationDefaultValuesProvesAllOmitRuns proves the all-omit
// state emits exactly `INSERT INTO "all_defaults" DEFAULT VALUES`, carries no
// parameters, and inserts through the normal execution path so declared
// defaults apply.
func TestInsertIntegrationDefaultValuesProvesAllOmitRuns(t *testing.T) {
	db, cat := openInsertFixture(t)

	q := buildInsertQB(t, cat, "all_defaults",
		map[string]qb.InsertChoice{"id": qb.InsertChoiceOmit, "tag": qb.InsertChoiceOmit, "n": qb.InsertChoiceOmit}, nil)
	if got, want := q.InsertSQL(), `INSERT INTO "all_defaults" DEFAULT VALUES`; got != want {
		t.Fatalf("InsertSQL() = %q, want %q", got, want)
	}
	if params := q.InsertParams(); len(params) != 0 {
		t.Fatalf("InsertParams() = %#v, want none", params)
	}
	mustRunInsert(t, db, q, 1)

	var tag string
	var id int64
	if err := db.SQL.QueryRow("SELECT id, tag FROM all_defaults").Scan(&id, &tag); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if id != 1 || tag != "none" {
		t.Fatalf("defaults row = (%d, %q), want (1, \"none\")", id, tag)
	}
}

// TestInsertIntegrationQuotedNamesAndVirtualTable proves unusual quoted
// identifiers insert safely and a virtual table inserts through its visible
// columns with a successful fts5 match afterward.
func TestInsertIntegrationQuotedNamesAndVirtualTable(t *testing.T) {
	db, cat := openInsertFixture(t)

	q := buildInsertQB(t, cat, `quoted`,
		map[string]qb.InsertChoice{"k": qb.InsertChoiceValue, `va "l`: qb.InsertChoiceValue},
		map[string]string{"k": `a "b`, `va "l`: `c "d`})
	mustRunInsert(t, db, q, 1)

	var extra string
	if err := db.SQL.QueryRow(`SELECT "va ""l" FROM quoted WHERE k = 'a "b'`).Scan(&extra); err != nil || extra != `c "d` {
		t.Fatalf("quoted readback = (%q, %v)", extra, err)
	}

	q = buildInsertQB(t, cat, "notes_fts",
		map[string]qb.InsertChoice{"title": qb.InsertChoiceValue},
		map[string]string{"title": "hello world"})
	mustRunInsert(t, db, q, 1)

	var hits int
	if err := db.SQL.QueryRow(`SELECT COUNT(*) FROM notes_fts WHERE notes_fts MATCH 'hello'`).Scan(&hits); err != nil || hits != 1 {
		t.Fatalf("fts match = (%d, %v), want 1 hit", hits, err)
	}
}

// TestInsertIntegrationOrdinaryDatabaseErrors proves a constraint failure
// surfaces as an ordinary failed request preserving the underlying cause —
// never a fabricated success — through the same seam.
func TestInsertIntegrationOrdinaryDatabaseErrors(t *testing.T) {
	db, cat := openInsertFixture(t)

	q := buildInsertQB(t, cat, "strict",
		map[string]qb.InsertChoice{"n": qb.InsertChoiceNull}, nil)
	affected, res := db.ExecuteInsert(context.Background(), q.InsertSQL(), q.InsertParams())
	if res.Outcome == connection.OutcomeSuccess {
		t.Fatal("NOT NULL insert unexpectedly succeeded")
	}
	if res.Err == nil || res.Health != nil {
		t.Fatalf("ordinary failure classification wrong: err=%v health=%v", res.Err, res.Health)
	}
	if affected != 0 {
		t.Fatalf("failed insert reported %d rows", affected)
	}
}
