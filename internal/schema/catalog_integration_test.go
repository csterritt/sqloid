//go:build unix

// SQLite-backed integration coverage for the catalog contract, per Issue #9
// Tasks 1 and 3: real fixtures containing ordinary tables, a virtual fts5
// table (with its shadow tables), views, a WITHOUT ROWID table, declared
// rowid-alias columns, generated columns, SQLite-owned objects, and D1's
// _cf_METADATA prove cataloging from main.sqlite_master only, accurate
// object kinds, PRAGMA schema_version capture, deterministic eligible
// results, and both required exclusions through the Connection request
// boundary.

package schema_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/chris/sqloid/internal/connection"
	schema "github.com/chris/sqloid/internal/schema"
)

// buildFixtureDatabase creates a SQLite database covering every object kind
// and exclusion the catalog must handle, including tables SQLite generates
// on Sqloid's behalf (AUTOINCREMENT's sqlite_sequence, fts5 shadow tables).
func buildFixtureDatabase(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening fixture %q: %v", path, err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '')`,
		`CREATE VIRTUAL TABLE album_notes_fts USING fts5(title)`,
		`CREATE TABLE kv_no_rowid (code TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID`,
		`CREATE TABLE shadowed_rowid (rowid TEXT, n INTEGER)`,
		`CREATE VIEW recent AS SELECT id, title FROM albums WHERE id > 0`,
		`CREATE TABLE big_auto (id INTEGER PRIMARY KEY AUTOINCREMENT)`,
		`CREATE TABLE "_cf_METADATA" (k TEXT, v TEXT)`,
		`CREATE TABLE generated_mix (a INTEGER, b INTEGER GENERATED ALWAYS AS (a*2), c INTEGER GENERATED ALWAYS AS (a*3))`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("creating fixture (%s): %v", stmt, err)
		}
	}
}

// TestCatalogIntegrationOpen reads the full fixture catalog through
// connection.Open and asserts every observable contract per object.
func TestCatalogIntegrationOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-fixture.db")
	buildFixtureDatabase(t, path)

	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer db.Close()

	// The exclusions must be meaningful: the fixture genuinely contains the
	// objects that must never surface.
	for _, reserved := range []string{"sqlite_sequence", "_cf_METADATA"} {
		var n int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM main.sqlite_master WHERE name = ?", reserved).Scan(&n); err != nil || n != 1 {
			t.Fatalf("fixture sanity: reserved object %q present = %d (err %v), want 1", reserved, n, err)
		}
	}

	var directVersion int64
	if err := db.SQL.QueryRow("PRAGMA schema_version").Scan(&directVersion); err != nil {
		t.Fatalf("reading direct schema version: %v", err)
	}

	cat, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	if cat.Version != directVersion {
		t.Errorf("catalog version = %d, want direct PRAGMA schema_version %d", cat.Version, directVersion)
	}

	var names []string
	for _, obj := range cat.Objects {
		names = append(names, obj.Name)
	}
	wantNames := []string{
		"album_notes_fts",
		"album_notes_fts_config",
		"album_notes_fts_content",
		"album_notes_fts_data",
		"album_notes_fts_docsize",
		"album_notes_fts_idx",
		"albums",
		"big_auto",
		"generated_mix",
		"kv_no_rowid",
		"recent",
		"shadowed_rowid",
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("cataloged objects = %v, want %v", names, wantNames)
	}

	byName := map[string]*schema.Object{}
	for _, obj := range cat.Objects {
		byName[obj.Name] = obj
	}

	for _, tc := range []struct {
		name       string
		wantKind   schema.ObjectKind
		wantRowid  schema.RowidCapability
		wantShadow bool
	}{
		{name: "albums", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
		{name: "album_notes_fts", wantKind: schema.KindVirtualTable, wantRowid: schema.RowidNotApplicable},
		{name: "album_notes_fts_config", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidWithout},
		{name: "album_notes_fts_content", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
		{name: "album_notes_fts_data", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
		{name: "album_notes_fts_docsize", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
		{name: "album_notes_fts_idx", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidWithout},
		{name: "kv_no_rowid", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidWithout},
		{name: "shadowed_rowid", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas, wantShadow: true},
		{name: "recent", wantKind: schema.KindView, wantRowid: schema.RowidNotApplicable},
		{name: "big_auto", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
		{name: "generated_mix", wantKind: schema.KindOrdinaryTable, wantRowid: schema.RowidHas},
	} {
		obj, ok := byName[tc.name]
		if !ok {
			t.Errorf("object %q missing from catalog", tc.name)
			continue
		}
		if obj.Kind != tc.wantKind {
			t.Errorf("%s kind = %s, want %s", tc.name, obj.Kind, tc.wantKind)
		}
		if obj.Rowid != tc.wantRowid {
			t.Errorf("%s rowid = %s, want %s", tc.name, obj.Rowid, tc.wantRowid)
		}
		if obj.RowidShadowed != tc.wantShadow {
			t.Errorf("%s rowid shadowed = %v, want %v", tc.name, obj.RowidShadowed, tc.wantShadow)
		}
	}
}

// TestCatalogIntegrationEligibilityAndColumns pins write eligibility, SELECT
// -only views, virtual-table hidden columns, and generated-column
// insertability against the real fixture.
func TestCatalogIntegrationEligibilityAndColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-fixture.db")
	buildFixtureDatabase(t, path)

	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer db.Close()

	cat, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	byName := map[string]*schema.Object{}
	for _, obj := range cat.Objects {
		byName[obj.Name] = obj
	}

	// Eligible tables are exactly the ordinary and virtual objects; the view
	// is never write eligible and reports zero insertable columns.
	var eligible []string
	for _, obj := range cat.Objects {
		switch {
		case obj.WriteEligible:
			if obj.Kind == schema.KindView {
				t.Errorf("view %s reported write eligible", obj.Name)
			}
			eligible = append(eligible, obj.Name)
		case obj.Kind == schema.KindView:
			if obj.InsertableCount != 0 {
				t.Errorf("view %s insertable count = %d, want 0 (SELECT-only)", obj.Name, obj.InsertableCount)
			}
			for _, col := range obj.Columns {
				if col.Insertable {
					t.Errorf("view %s column %s insertable, want SELECT-only", obj.Name, col.Name)
				}
			}
		default:
			t.Errorf("object %s neither write eligible nor a view", obj.Name)
		}
	}
	if !reflect.DeepEqual(eligible, []string{
		"album_notes_fts", "album_notes_fts_config", "album_notes_fts_content",
		"album_notes_fts_data", "album_notes_fts_docsize", "album_notes_fts_idx",
		"albums", "big_auto", "generated_mix", "kv_no_rowid", "shadowed_rowid",
	}) {
		t.Errorf("write-eligible objects = %v", eligible)
	}

	// Virtual-table columns: the declared fts5 column stays insertable while
	// the module's hidden columns do not.
	fts := byName["album_notes_fts"]
	var visible, hidden []string
	for _, col := range fts.Columns {
		if col.Hidden {
			hidden = append(hidden, col.Name)
		} else {
			visible = append(visible, col.Name)
		}
	}
	if !reflect.DeepEqual(visible, []string{"title"}) {
		t.Errorf("fts visible columns = %v, want [title]", visible)
	}
	if len(hidden) == 0 {
		t.Errorf("fts hidden columns = %v, want the module's hidden columns", hidden)
	}
	for _, col := range fts.Columns {
		if col.Insertable != (col.Name == "title") {
			t.Errorf("fts column %s insertable = %v", col.Name, col.Insertable)
		}
	}

	// Ordinary-table columns keep declared types as metadata and stay
	// insertable; generated columns of an ordinary table are hidden and
	// noninsertable.
	albums := byName["albums"]
	if got, want := len(albums.Columns), 2; got != want {
		t.Fatalf("albums column count = %d, want %d", got, want)
	}
	if albums.Columns[0].Name != "id" || albums.Columns[0].DeclaredType != "INTEGER" || !albums.Columns[0].Insertable {
		t.Errorf("albums id column = %+v, want INTEGER insertable", albums.Columns[0])
	}
	if albums.Columns[1].Name != "title" || albums.Columns[1].DeclaredType != "TEXT" || !albums.Columns[1].Insertable {
		t.Errorf("albums title column = %+v, want TEXT insertable", albums.Columns[1])
	}
	gen := byName["generated_mix"]
	if gen.InsertableCount != 1 || gen.Columns[0].Name != "a" || !gen.Columns[0].Insertable {
		t.Errorf("generated_mix insertable = %d (%+v), want only column a", gen.InsertableCount, gen.Columns)
	}
	for _, col := range gen.Columns[1:] {
		if !col.Hidden || col.Insertable {
			t.Errorf("generated column %+v, want hidden and noninsertable", col)
		}
	}
	if gen.Columns[1].DeclaredType != "INTEGER" {
		t.Errorf("generated column declared type = %q, want INTEGER metadata passthrough", gen.Columns[1].DeclaredType)
	}
}

// TestCatalogIntegrationDeterministicAndRefresh pins that repeated reads
// produce identical catalogs and that a dropped object raises the schema
// version and disappears from a refreshed catalog.
func TestCatalogIntegrationDeterministicAndRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-fixture.db")
	buildFixtureDatabase(t, path)

	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer db.Close()

	cat, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("first ReadCatalog result = %+v, want success", res)
	}
	second, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("second ReadCatalog result = %+v, want success", res)
	}
	if !reflect.DeepEqual(cat, second) {
		t.Errorf("repeated read produced a different catalog:\n%+v\n%+v", cat, second)
	}

	before := cat.Version
	var names []string
	for _, obj := range cat.Objects {
		names = append(names, obj.Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("empty catalog before refresh")
	}

	if _, err := db.SQL.Exec(`DROP TABLE big_auto`); err != nil {
		t.Fatalf("dropping fixture table: %v", err)
	}

	refreshed, res := db.ReadCatalog(t.Context())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("refreshed ReadCatalog result = %+v, want success", res)
	}
	if refreshed.Version <= before {
		t.Errorf("refreshed version = %d, want greater than %d after DDL", refreshed.Version, before)
	}
	for _, obj := range refreshed.Objects {
		if obj.Name == "big_auto" {
			t.Errorf("dropped object still present in refreshed catalog")
		}
	}
}
