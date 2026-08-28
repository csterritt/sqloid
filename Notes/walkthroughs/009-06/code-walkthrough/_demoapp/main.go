// Walkthrough helper for Issue #9's code walkthrough: creates the schema
// fixture database ("create <path>") or prints the typed catalog contract
// ("show <path>"). Lives under Notes/walkthroughs so production source trees
// stay untouched; ignored by ./... builds because no Go file exists elsewhere
// in the tree that references it and this directory carries its own package.
package main

import (
	"context"

	"database/sql"
	"fmt"
	"os"
	"reflect"

	"github.com/chris/sqloid/internal/connection"
	_ "modernc.org/sqlite"
)

// schemaVersion reads PRAGMA schema_version directly from a raw handle.
func schemaVersion(db *sql.DB) (int64, bool) {
	var v int64
	err := db.QueryRow("PRAGMA schema_version").Scan(&v)
	return v, err == nil
}

func main() {
	cmd, path := os.Args[1], os.Args[2]
	switch cmd {
	case "create":
		db, err := sql.Open("sqlite", path)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		for _, stmt := range []string{
			`CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '')`,
			`CREATE VIRTUAL TABLE album_notes_fts USING fts5(title)`,
			`CREATE TABLE kv_no_rowid (code TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID`,
			`CREATE TABLE shadowed_rowid (rowid TEXT, n INTEGER)`,
			`CREATE VIEW recent AS SELECT id, title FROM albums WHERE id > 0`,
			`CREATE TABLE big_auto (id INTEGER PRIMARY KEY AUTOINCREMENT)`,
			`CREATE TABLE "_cf_METADATA" (k TEXT, v TEXT)`,
			`CREATE TABLE generated_mix (a INTEGER, b INTEGER GENERATED ALWAYS AS (a*2), c INTEGER GENERATED ALWAYS AS (a*3))`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				panic(err)
			}
		}

	case "columns":
		db, err := connection.Open(path)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != connection.OutcomeSuccess {
			panic(res.Err)
		}
		for _, name := range []string{"albums", "album_notes_fts", "kv_no_rowid", "shadowed_rowid", "recent", "generated_mix"} {
			for _, obj := range cat.Objects {
				if obj.Name != name {
					continue
				}
				for _, col := range obj.Columns {
					fmt.Printf("%-16s %-16s type=%-9q hidden=%-5v insertable=%v\n", obj.Name, col.Name, col.DeclaredType, col.Hidden, col.Insertable)
				}
			}
		}

	case "determinism":
		db, err := connection.Open(path)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != connection.OutcomeSuccess {
			panic(res.Err)
		}
		again, res2 := db.ReadCatalog(context.Background())
		if res2.Outcome != connection.OutcomeSuccess {
			panic(res2.Err)
		}
		fmt.Printf("schema_version = %d; repeated read deep-equal: %v\n", cat.Version, reflect.DeepEqual(cat, again))

	case "drop-refresh":
		db, err := sql.Open("sqlite", path)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		before, ok := schemaVersion(db)
		if !ok {
			panic("no version")
		}
		if _, err := db.Exec(`DROP TABLE big_auto`); err != nil {
			panic(err)
		}
		cdb, err := connection.Open(path)
		if err != nil {
			panic(err)
		}
		defer cdb.Close()
		cat, res := cdb.ReadCatalog(context.Background())
		if res.Outcome != connection.OutcomeSuccess {
			panic(res.Err)
		}
		present := false
		for _, obj := range cat.Objects {
			if obj.Name == "big_auto" {
				present = true
			}
		}
		fmt.Printf("version before = %d after DDL refresh = %d big_auto present = %v\n", before, cat.Version, present)

	case "show":
		db, err := connection.Open(path)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != connection.OutcomeSuccess {
			panic(res.Err)
		}
		fmt.Printf("schema_version = %d\n", cat.Version)
		fmt.Printf("cataloged objects: %d\n\n", len(cat.Objects))
		for _, obj := range cat.Objects {
			fmt.Printf("%-24s %-15s write=%-5v rowid=%-14s shadow=%-5v insertableCols=%d\n",
				obj.Name, obj.Kind, obj.WriteEligible, obj.Rowid, obj.RowidShadowed, obj.InsertableCount)
		}
	}
}
