// Table-driven contract tests for main-schema cataloging, per Issue #9
// Tasks 1 and 3: BuildCatalog classifies ordinary tables, virtual tables,
// and views; applies the required sqlite_% and _cf_METADATA exclusions;
// reports schema version, write eligibility, rowid capability and declared
// shadowing; and derives column declared type, visibility, and insertability
// from table_xinfo rows. Fixtures here are synthetic so every rule is
// observable without SQLite; integration tests in this directory prove the
// same contract against a real database.

package schema

import (
	"reflect"
	"testing"
)

// TestBuildCatalogClassifiesObjects exercises kind detection, write
// eligibility, rowid capability/shadowing, and insertability derivation as
// one table-driven sweep over synthetic metadata.
func TestBuildCatalogClassifiesObjects(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  MasterRow
		cols []ColumnRow

		wantKind          ObjectKind
		wantWriteEligible bool
		wantRowid         RowidCapability
		wantShadowed      bool
		wantInsertable    []string // column names expected Insertable, in order
	}{
		{
			name:              "ordinary table is write eligible with rowid",
			row:               MasterRow{Name: "albums", Type: "table", SQL: "CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT)"},
			cols:              []ColumnRow{{Name: "id", DeclaredType: "INTEGER"}, {Name: "title", DeclaredType: "TEXT"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
			wantInsertable:    []string{"id", "title"},
		},
		{
			name:              "WITHOUT ROWID table reports rowid absence",
			row:               MasterRow{Name: "kv", Type: "table", SQL: "CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID"},
			cols:              []ColumnRow{{Name: "k", DeclaredType: "TEXT"}, {Name: "v", DeclaredType: "TEXT"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidWithout,
			wantInsertable:    []string{"k", "v"},
		},
		{
			name:              "WITHOUT ROWID tolerated after semicolon, spaces, comment, and odd case",
			row:               MasterRow{Name: "kv2", Type: "table", SQL: "create table kv2 (k TEXT PRIMARY KEY) WiThOuT   RoWId ; --  legacy fixture\n"},
			cols:              []ColumnRow{{Name: "k", DeclaredType: "TEXT"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidWithout,
			wantInsertable:    []string{"k"},
		},
		{
			name:              "virtual table is write eligible but never rowid-addressable",
			row:               MasterRow{Name: "notes_fts", Type: "table", SQL: "CREATE VIRTUAL TABLE notes_fts USING fts5(body)"},
			cols:              []ColumnRow{{Name: "body"}, {Name: "notes_fts", Hidden: 1}, {Name: "rank", Hidden: 1}},
			wantKind:          KindVirtualTable,
			wantWriteEligible: true,
			wantRowid:         RowidNotApplicable,
			wantShadowed:      false,
			wantInsertable:    []string{"body"},
		},
		{
			name:      "view is SELECT-only and never insertable",
			row:       MasterRow{Name: "recent", Type: "view", SQL: "CREATE VIEW recent AS SELECT id, title FROM albums"},
			cols:      []ColumnRow{{Name: "id", DeclaredType: "INTEGER"}, {Name: "title", DeclaredType: "TEXT"}},
			wantKind:  KindView,
			wantRowid: RowidNotApplicable,
		},
		{
			name:              "declared rowid column shadows rowid addressing",
			row:               MasterRow{Name: "shadowed_rowid", Type: "table", SQL: "CREATE TABLE shadowed_rowid (rowid TEXT, n INTEGER)"},
			cols:              []ColumnRow{{Name: "rowid", DeclaredType: "TEXT"}, {Name: "n", DeclaredType: "INTEGER"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
			wantShadowed:      true,
			wantInsertable:    []string{"rowid", "n"},
		},
		{
			name:              "declared _rowid_ column shadows rowid addressing",
			row:               MasterRow{Name: "shadowed_underscore", Type: "table", SQL: "CREATE TABLE shadowed_underscore (_rowid_ TEXT, n INTEGER)"},
			cols:              []ColumnRow{{Name: "_rowid_", DeclaredType: "TEXT"}, {Name: "n", DeclaredType: "INTEGER"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
			wantShadowed:      true,
			wantInsertable:    []string{"_rowid_", "n"},
		},
		{
			name:              "declared oid column shadows rowid addressing case-insensitively",
			row:               MasterRow{Name: "shadowed_oid", Type: "table", SQL: "CREATE TABLE shadowed_oid (OID TEXT, n INTEGER)"},
			cols:              []ColumnRow{{Name: "OID", DeclaredType: "TEXT"}, {Name: "n", DeclaredType: "INTEGER"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
			wantShadowed:      true,
			wantInsertable:    []string{"OID", "n"},
		},
		{
			name:              "generated columns are hidden and noninsertable",
			row:               MasterRow{Name: "gen_mix", Type: "table", SQL: "CREATE TABLE gen_mix (a INTEGER, b INTEGER GENERATED ALWAYS AS (a*2), c INTEGER GENERATED ALWAYS AS (a*3))"},
			cols:              []ColumnRow{{Name: "a", DeclaredType: "INTEGER"}, {Name: "b", DeclaredType: "INTEGER", Hidden: 2}, {Name: "c", DeclaredType: "INTEGER", Hidden: 3}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
			wantInsertable:    []string{"a"},
		},
		{
			name:              "table whose every column is hidden has zero insertable columns",
			row:               MasterRow{Name: "zero_ins", Type: "table", SQL: "CREATE TABLE zero_ins (a INTEGER, b INTEGER GENERATED ALWAYS AS (a*2))"},
			cols:              []ColumnRow{{Name: "a", Hidden: 1}, {Name: "b", DeclaredType: "INTEGER", Hidden: 3}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cat := BuildCatalog(Input{Version: 7, Master: []MasterRow{tc.row}, Columns: map[string][]ColumnRow{tc.row.Name: tc.cols}})
			objs := cat.Objects
			if len(objs) != 1 {
				t.Fatalf("catalog objects = %d, want exactly %q", len(objs), tc.row.Name)
			}
			obj := objs[0]
			if obj.Name != tc.row.Name {
				t.Errorf("object name = %q, want %q", obj.Name, tc.row.Name)
			}
			if obj.Kind != tc.wantKind {
				t.Errorf("object kind = %s, want %s", obj.Kind, tc.wantKind)
			}
			if obj.WriteEligible != tc.wantWriteEligible {
				t.Errorf("write eligible = %v, want %v", obj.WriteEligible, tc.wantWriteEligible)
			}
			if obj.Rowid != tc.wantRowid {
				t.Errorf("rowid capability = %s, want %s", obj.Rowid, tc.wantRowid)
			}
			if obj.RowidShadowed != tc.wantShadowed {
				t.Errorf("rowid shadowed = %v, want %v", obj.RowidShadowed, tc.wantShadowed)
			}
			if got, want := len(obj.Columns), len(tc.cols); got != want {
				t.Fatalf("column count = %d, want %d", got, want)
			}
			var insertable []string
			for _, col := range obj.Columns {
				if col.Insertable {
					insertable = append(insertable, col.Name)
				}
			}
			if len(tc.wantInsertable) == 0 && len(insertable) != 0 {
				t.Errorf("insertable columns = %v, want none", insertable)
			}
			if !reflect.DeepEqual(insertable, tc.wantInsertable) {
				t.Errorf("insertable columns = %v, want %v", insertable, tc.wantInsertable)
			}
			if tc.wantInsertable != nil && obj.InsertableCount != len(tc.wantInsertable) {
				t.Errorf("insertable count = %d, want %d", obj.InsertableCount, len(tc.wantInsertable))
			}
		})
	}
}

// TestBuildCatalogExclusionsAndIgnores pins that SQLite-owned objects and
// D1's _cf_METADATA never appear in any case form, while look-alike names
// stay visible and non-cataloged master types are ignored.
func TestBuildCatalogExclusionsAndIgnores(t *testing.T) {
	cat := BuildCatalog(Input{
		Version: 3,
		Master: []MasterRow{
			{Name: "sqlite_sequence", Type: "table", SQL: "CREATE TABLE sqlite_sequence(name,seq)"},
			{Name: "SQLITE_STAT1", Type: "table"},
			{Name: "Sqlite_Foo", Type: "table"},
			{Name: "_cf_METADATA", Type: "table", SQL: "CREATE TABLE \"_cf_METADATA\"(k,v)"},
			{Name: "_cf_metadata", Type: "table"},
			{Name: "user_data", Type: "table", SQL: "CREATE TABLE user_data (id INTEGER PRIMARY KEY)"},
			{Name: "sqlitextra", Type: "table", SQL: "CREATE TABLE sqlitextra (id INTEGER PRIMARY KEY)"},
			{Name: "idx_name", Type: "index"},
			{Name: "trg_touch", Type: "trigger"},
		},
		Columns: map[string][]ColumnRow{
			"user_data": {{Name: "id", DeclaredType: "INTEGER"}},
		},
	})

	var names []string
	for _, obj := range cat.Objects {
		names = append(names, obj.Name)
	}
	want := []string{"sqlitextra", "user_data"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("cataloged objects = %v, want %v", names, want)
	}
}

// TestBuildCatalogDeterministic pins that identical inputs always yield the
// identical ascending-name catalog regardless of sqlite_master order.
func TestBuildCatalogDeterministic(t *testing.T) {
	master := []MasterRow{
		{Name: "zeta", Type: "table", SQL: "CREATE TABLE zeta (id INTEGER PRIMARY KEY)"},
		{Name: "alpha", Type: "table", SQL: "CREATE TABLE alpha (id INTEGER PRIMARY KEY)"},
		{Name: "m_view", Type: "view", SQL: "CREATE VIEW m_view AS SELECT 1"},
	}
	cols := map[string][]ColumnRow{
		"zeta":   {{Name: "id", DeclaredType: "INTEGER"}},
		"alpha":  {{Name: "id", DeclaredType: "INTEGER"}},
		"m_view": {},
	}

	first := BuildCatalog(Input{Version: 9, Master: master, Columns: cols})
	second := BuildCatalog(Input{Version: 9, Master: master, Columns: cols})
	if !reflect.DeepEqual(first, second) {
		t.Errorf("rebuild produced different catalog:\n%+v\n%+v", first, second)
	}

	reversed := []MasterRow{master[1], master[2], master[0]}
	third := BuildCatalog(Input{Version: 9, Master: reversed, Columns: cols})
	if !reflect.DeepEqual(first, third) {
		t.Errorf("reversed input produced different catalog:\n%+v\n%+v", first, third)
	}
	var names []string
	for _, obj := range first.Objects {
		names = append(names, obj.Name)
	}
	if want := []string{"alpha", "m_view", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("object order = %v, want ascending %v", names, want)
	}
	if first.Version != 9 {
		t.Errorf("catalog version = %d, want 9 passthrough of PRAGMA schema_version", first.Version)
	}
}

// TestBuildCatalogColumnsPreserveDeclaredOrderAndType pins that column
// records keep table_xinfo order and carry declared type as pure metadata,
// including empty type text and untypeable kinds reporting no columns.
func TestBuildCatalogColumnsPreserveDeclaredOrderAndType(t *testing.T) {
	cat := BuildCatalog(Input{
		Version: 1,
		Master:  []MasterRow{{Name: "wide", Type: "table", SQL: "CREATE TABLE wide (b BLOB, a INTEGER, c)"}},
		Columns: map[string][]ColumnRow{
			"wide": {
				{Name: "b", DeclaredType: "BLOB"},
				{Name: "a", DeclaredType: "INTEGER"},
				{Name: "c"},
			},
		},
	})
	obj := cat.Objects[0]
	var got []struct {
		name string
		typ  string
	}
	for _, col := range obj.Columns {
		got = append(got, struct {
			name string
			typ  string
		}{col.Name, col.DeclaredType})
	}
	want := []struct {
		name string
		typ  string
	}{{"b", "BLOB"}, {"a", "INTEGER"}, {"c", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("columns = %+v, want declared order and types %+v", got, want)
	}
}

// TestBuildCatalogObjectWithoutColumns pins that objects absent from the
// Columns map catalog with zero columns rather than failing.
func TestBuildCatalogObjectWithoutColumns(t *testing.T) {
	cat := BuildCatalog(Input{
		Version: 1,
		Master:  []MasterRow{{Name: "orphan", Type: "table", SQL: "CREATE TABLE orphan (id INTEGER PRIMARY KEY)"}},
	})
	obj := cat.Objects[0]
	if len(obj.Columns) != 0 || obj.InsertableCount != 0 {
		t.Errorf("orphan columns = %+v, want none reported", obj.Columns)
	}
}

// TestObjectKindStrings pins the exact human-facing kind names used in
// diagnostics and UI copy decisions.
func TestObjectKindStrings(t *testing.T) {
	for kind, want := range map[ObjectKind]string{
		KindOrdinaryTable: "ordinary-table",
		KindVirtualTable:  "virtual-table",
		KindView:          "view",
	} {
		if got := kind.String(); got != want {
			t.Errorf("kind %d String() = %q, want %q", int(kind), got, want)
		}
	}
}

// TestRowidCapabilityStrings pins the exact rowid capability names from the
// PRD's Schema metadata decision for the three meaningful values, plus the
// diagnostic form of the zero (unset) sentinel and a representative unknown
// value. The zero value is reserved as an unset/unknown sentinel and never
// produced by BuildCatalog; an unknown value is likewise not produced but its
// diagnostic shape is locked so a future renumbering cannot silently change
// user-facing diagnostics.
func TestRowidCapabilityStrings(t *testing.T) {
	cases := []struct {
		cap  RowidCapability
		want string
	}{
		{RowidHas, "has-rowid"},
		{RowidWithout, "without-rowid"},
		{RowidNotApplicable, "not-applicable"},
		{0, "RowidCapability(0)"},
		{RowidCapability(99), "RowidCapability(99)"},
	}
	for _, tc := range cases {
		if got := tc.cap.String(); got != tc.want {
			t.Errorf("capability %d String() = %q, want %q", int(tc.cap), got, tc.want)
		}
	}
}

// TestBuildCatalogRowidClassificationsLocked pins that the four object shapes
// Sqloid catalogs — ordinary rowid table, WITHOUT ROWID table, virtual table,
// and view — keep their current kind, write eligibility, and rowid capability
// classifications. This is the behavioral safety net run before the
// RowidApplicable cleanup so the enum edit cannot move a classification.
func TestBuildCatalogRowidClassificationsLocked(t *testing.T) {
	cases := []struct {
		name              string
		row               MasterRow
		cols              []ColumnRow
		wantKind          ObjectKind
		wantWriteEligible bool
		wantRowid         RowidCapability
	}{
		{
			name:              "ordinary rowid table",
			row:               MasterRow{Name: "albums", Type: "table", SQL: "CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT)"},
			cols:              []ColumnRow{{Name: "id", DeclaredType: "INTEGER"}, {Name: "title", DeclaredType: "TEXT"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidHas,
		},
		{
			name:              "WITHOUT ROWID table",
			row:               MasterRow{Name: "kv", Type: "table", SQL: "CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID"},
			cols:              []ColumnRow{{Name: "k", DeclaredType: "TEXT"}, {Name: "v", DeclaredType: "TEXT"}},
			wantKind:          KindOrdinaryTable,
			wantWriteEligible: true,
			wantRowid:         RowidWithout,
		},
		{
			name:              "virtual table",
			row:               MasterRow{Name: "notes_fts", Type: "table", SQL: "CREATE VIRTUAL TABLE notes_fts USING fts5(body)"},
			cols:              []ColumnRow{{Name: "body"}, {Name: "notes_fts", Hidden: 1}, {Name: "rank", Hidden: 1}},
			wantKind:          KindVirtualTable,
			wantWriteEligible: true,
			wantRowid:         RowidNotApplicable,
		},
		{
			name:              "view",
			row:               MasterRow{Name: "recent", Type: "view", SQL: "CREATE VIEW recent AS SELECT id, title FROM albums"},
			cols:              []ColumnRow{{Name: "id", DeclaredType: "INTEGER"}, {Name: "title", DeclaredType: "TEXT"}},
			wantKind:          KindView,
			wantWriteEligible: false,
			wantRowid:         RowidNotApplicable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := BuildCatalog(Input{Version: 1, Master: []MasterRow{tc.row}, Columns: map[string][]ColumnRow{tc.row.Name: tc.cols}})
			obj := cat.Objects[0]
			if obj.Kind != tc.wantKind {
				t.Errorf("kind = %s, want %s", obj.Kind, tc.wantKind)
			}
			if obj.WriteEligible != tc.wantWriteEligible {
				t.Errorf("write eligible = %v, want %v", obj.WriteEligible, tc.wantWriteEligible)
			}
			if obj.Rowid != tc.wantRowid {
				t.Errorf("rowid capability = %s, want %s", obj.Rowid, tc.wantRowid)
			}
		})
	}
}
