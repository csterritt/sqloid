// Table-driven INSERT insertability and INTEGER PRIMARY KEY hinting contract
// tests, per Issue #39 Task 1: every visible insertable column of an
// ordinary or virtual table appears exactly once in schema order with no
// AUTOINCREMENT-based, nullable-based, or default-based skip; hidden and
// generated columns are excluded regardless of declared type or defaults;
// and exactly one omission hint shape exists — the single-column INTEGER
// PRIMARY KEY rowid alias of a has-rowid table — while similar declared
// types and non-primary INTEGER columns never receive it. Fixtures here are
// synthetic so every rule is observable without SQLite; integration tests
// prove the same contract against a real database.

package schema

import (
	"reflect"
	"testing"
)

// insertableCatalog builds one synthetic two-table fixture: an ordinary
// table mixing visible, hidden, and generated columns with unusual quoted
// names, and a virtual fts5-style table whose hidden inputs must never
// prompt.
func insertableCatalog() *Catalog {
	return BuildCatalog(Input{
		Version: 7,
		Master: []MasterRow{
			{Name: `"mix ""ed"`, Type: "table", SQL: `CREATE TABLE "mix ""ed" (a INTEGER PRIMARY KEY AUTOINCREMENT, b TEXT DEFAULT 'x', c, d INTEGER GENERATED ALWAYS AS (a*2), e TEXT HIDDEN)`},
			{Name: "notes_fts", Type: "table", SQL: "CREATE VIRTUAL TABLE notes_fts USING fts5(title)"}, // fts5: title is hidden in table_xinfo
			{Name: "all_hidden", Type: "table", SQL: "CREATE TABLE all_hidden (g INTEGER GENERATED ALWAYS AS (1))"},
		},
		Columns: map[string][]ColumnRow{
			`"mix ""ed"`: {
				{Name: "a", DeclaredType: "INTEGER", PrimaryKey: 1},
				{Name: "b", DeclaredType: "TEXT"},
				{Name: "c"},
				{Name: "d", DeclaredType: "INTEGER", Hidden: 2}, // generated: stored virtual column
				{Name: "e", DeclaredType: "TEXT", Hidden: 3},    // module-style hidden input column
			},
			"notes_fts": {
				{Name: "title", Hidden: 1},
			},
			"all_hidden": {
				{Name: "g", DeclaredType: "INTEGER", Hidden: 2},
			},
		},
	})
}

func objByName(t *testing.T, cat *Catalog, name string) *Object {
	t.Helper()
	for _, o := range cat.Objects {
		if o.Name == name {
			return o
		}
	}
	t.Fatalf("object %q missing from catalog", name)
	return nil
}

// TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce proves schema-order
// prompting over every visible insertable column with hidden and generated
// exclusion, no declared-type or nullable/default/AUTOINCREMENT-based skip,
// and zero insertable columns for all-hidden tables and fully hidden virtual
// tables.
func TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce(t *testing.T) {
	cat := insertableCatalog()

	t.Run("ordinary mixed table", func(t *testing.T) {
		obj := objByName(t, cat, `"mix ""ed"`)
		var got []string
		for _, col := range obj.Columns {
			if col.Insertable {
				got = append(got, col.Name)
			}
		}
		// The AUTOINCREMENT primary key, the nullable defaulted TEXT column,
		// and the untyped column all prompt exactly once, in schema order;
		// the generated and hidden columns never do.
		want := []string{"a", "b", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("insertable columns = %q, want %q", got, want)
		}
		if obj.InsertableCount != len(want) {
			t.Fatalf("InsertableCount = %d, want %d", obj.InsertableCount, len(want))
		}
	})

	t.Run("virtual table hidden input", func(t *testing.T) {
		obj := objByName(t, cat, "notes_fts")
		if obj.InsertableCount != 0 {
			t.Fatalf("fts5 fixture InsertableCount = %d, want 0 (title is a hidden module column in table_xinfo)", obj.InsertableCount)
		}
	})

	t.Run("all-hidden table", func(t *testing.T) {
		obj := objByName(t, cat, "all_hidden")
		if obj.InsertableCount != 0 {
			t.Fatalf("all-hidden InsertableCount = %d, want 0", obj.InsertableCount)
		}
	})
}

// TestInsertOmissionHintExactness proves the hint appears exactly once per
// catalog shape: only the single-column INTEGER PRIMARY KEY rowid alias of a
// has-rowid table receives "(auto-assigned if omitted)". WITHOUT ROWID
// tables, multi-column keys, similar declared types, virtual tables, and
// non-primary INTEGER columns never receive it, and the hint never changes
// insertability or ordering.
func TestInsertOmissionHintExactness(t *testing.T) {
	cat := BuildCatalog(Input{
		Version: 1,
		Master: []MasterRow{
			{Name: "plain", Type: "table", SQL: "CREATE TABLE plain (id INTEGER PRIMARY KEY, n INTEGER, s INT, big BIGINT, other INT)"},
			{Name: "wr", Type: "table", SQL: "CREATE TABLE wr (id INTEGER PRIMARY KEY, txt TEXT PRIMARY KEY) WITHOUT ROWID"},
			{Name: "composite", Type: "table", SQL: "CREATE TABLE composite (a INTEGER, b INTEGER, PRIMARY KEY (a, b))"},
			{Name: "nonprimary", Type: "table", SQL: "CREATE TABLE nonprimary (id INTEGER, k INTEGER UNIQUE)"},
			{Name: "virtual_pk", Type: "table", SQL: "CREATE VIRTUAL TABLE virtual_pk USING fts5(id)"},
		},
		Columns: map[string][]ColumnRow{
			"plain": {
				{Name: "id", DeclaredType: "INTEGER", PrimaryKey: 1},
				{Name: "n", DeclaredType: "INTEGER"},
				{Name: "s", DeclaredType: "INT"},
				{Name: "big", DeclaredType: "BIGINT"},
				{Name: "other", DeclaredType: "int"},
			},
			"wr": {
				{Name: "id", DeclaredType: "INTEGER", PrimaryKey: 1},
				{Name: "txt", DeclaredType: "TEXT", PrimaryKey: 1},
			},
			"composite": {
				{Name: "a", DeclaredType: "INTEGER", PrimaryKey: 1},
				{Name: "b", DeclaredType: "INTEGER", PrimaryKey: 2},
			},
			"nonprimary": {
				{Name: "id", DeclaredType: "INTEGER"},
				{Name: "k", DeclaredType: "INTEGER"},
			},
			"virtual_pk": {
				{Name: "id", Hidden: 1},
			},
		},
	})

	wantHints := map[string]map[string]string{
		"plain":      {"id": InsertOmissionHint, "n": "", "s": "", "big": "", "other": ""},
		"wr":         {"id": "", "txt": ""},
		"composite":  {"a": "", "b": ""},
		"nonprimary": {"id": "", "k": ""},
		"virtual_pk": {"id": ""},
	}
	for _, objName := range []string{"plain", "wr", "composite", "nonprimary", "virtual_pk"} {
		obj := objByName(t, cat, objName)
		for _, col := range obj.Columns {
			want, probed := wantHints[objName][col.Name]
			if !probed {
				t.Fatalf("fixture column %s.%s lacks an expectation", objName, col.Name)
			}
			got, ok := obj.InsertHint(col)
			if want == "" {
				if ok {
					t.Fatalf("%s.%s unexpectedly received the omission hint %q", objName, col.Name, got)
				}
				continue
			}
			if !ok || got != want {
				t.Fatalf("%s.%s hint = (%q, %v), want exactly %q", objName, col.Name, got, ok, want)
			}
		}
	}
}

// TestInsertHintCaseInsensitiveExactInteger proves the declared-type check
// is exact INTEGER case-insensitively: quoting styles do not matter to the
// metadata, and the quoted unusual column name still receives the hint.
func TestInsertHintCaseInsensitiveExactInteger(t *testing.T) {
	cat := BuildCatalog(Input{
		Version: 3,
		Master: []MasterRow{
			{Name: `"q ""t"`, Type: "table", SQL: `CREATE TABLE "q ""t" ("i ""d" integer primary key)`},
		},
		Columns: map[string][]ColumnRow{
			`"q ""t"`: {{Name: `i "d`, DeclaredType: "integer", PrimaryKey: 1}},
		},
	})
	obj := objByName(t, cat, `"q ""t"`)
	if len(obj.Columns) != 1 {
		t.Fatalf("column count = %d, want 1", len(obj.Columns))
	}
	got, ok := obj.InsertHint(obj.Columns[0])
	if !ok || got != InsertOmissionHint {
		t.Fatalf("quoted INTEGER PRIMARY KEY hint = (%q, %v), want %q", got, ok, InsertOmissionHint)
	}
}
