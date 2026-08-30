// Catalog building: deterministic conversion of authoritative SQLite
// metadata rows (main.sqlite_master and PRAGMA table_xinfo) into the typed
// Catalog contract, per Issue #9. All eligibility, exclusion, kind, rowid,
// and insertability rules live here so they are pure, table-driven testable
// functions over plain data; internal/connection only gathers the rows.

package schema

import (
	"sort"
	"strings"
)

// MasterRow is one decoded main.sqlite_master row limited to what cataloging
// consumes: the object's name, its type column ("table" or "view"), and its
// original CREATE statement (empty for sqlite-internal views).
type MasterRow struct {
	Name string
	Type string
	SQL  string
}

// ColumnRow is one decoded PRAGMA table_xinfo row, restricted to the fields
// the catalog consumes. Hidden carries table_xinfo's raw value verbatim so
// virtual-table hidden columns and generated columns both classify as
// non-insertable without this package having to guess a driver-side split.
// PrimaryKey carries the raw pk column value verbatim: 0 for key-less
// columns and the 1-based key slot otherwise.
type ColumnRow struct {
	Name         string
	DeclaredType string
	Hidden       int
	PrimaryKey   int
}

// Input bundles everything BuildCatalog needs for one refresh: the schema
// version read in the same request as the master rows, the master rows
// themselves, and one table_xinfo row slice per object name. Input is owned
// by the caller; BuildCatalog never mutates it.
type Input struct {
	Version int64
	Master  []MasterRow

	// Columns maps an object name from Master to its declared columns in cid
	// order. Objects absent from the map report no columns.
	Columns map[string][]ColumnRow
}

// BuildCatalog converts gathered SQLite metadata into the typed Catalog:
// indexes and triggers are ignored, every other object is classified, both
// required exclusions are applied, and objects are sorted ascending by name
// for determinism regardless of input order.
func BuildCatalog(in Input) *Catalog {
	master := make([]MasterRow, len(in.Master))
	copy(master, in.Master)
	sort.Slice(master, func(i, j int) bool { return master[i].Name < master[j].Name })

	var cat Catalog
	cat.Version = in.Version
	for _, row := range master {
		if excluded(row.Name) {
			continue
		}
		obj, ok := buildObject(row, in.Columns[row.Name])
		if !ok {
			continue // index, trigger, or another type Sqloid does not catalog
		}
		cat.Objects = append(cat.Objects, obj)
	}
	return &cat
}

// excluded reports whether a main-schema object is deliberately invisible to
// Sqloid: everything SQLite reserves through the sqlite_ prefix and Cloudflare
// D1's internal _cf_METADATA bookkeeping table.
func excluded(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "sqlite_") || strings.EqualFold(name, "_cf_METADATA")
}

// buildObject classifies one master row into a fully populated Object. The
// second return is false for rows of types outside Sqloid's {table, view}
// scope. Virtual tables are those whose stored SQL declares them with CREATE
// VIRTUAL TABLE; shadow tables created beneath a virtual module arrive as
// ordinary tables and stay ordinary here.
func buildObject(row MasterRow, colRows []ColumnRow) (*Object, bool) {
	var obj Object
	obj.Name = row.Name
	switch strings.ToLower(strings.TrimSpace(row.Type)) {
	case "view":
		obj.Kind = KindView
		// Views are SELECT-only: never write-eligible, never addressable by
		// rowid, and therefore their rowid-shadow flag stays false even if a
		// projected output name happens to look like rowid.
		obj.WriteEligible = false
		obj.Rowid = RowidNotApplicable
		obj.RowidShadowed = false
	case "table":
		if isVirtualTableSQL(row.SQL) {
			obj.Kind = KindVirtualTable
			// Virtual-table rowid behavior is module-specific (fts5 exposes
			// docid, others may not support rowid at all), so v1 treats
			// virtual tables as explicitly not rowid-addressable; writes go
			// through visible columns only.
			obj.Rowid = RowidNotApplicable
			obj.WriteEligible = true
		} else {
			obj.Kind = KindOrdinaryTable
			obj.WriteEligible = true
			if isWithoutRowidSQL(row.SQL) {
				obj.Rowid = RowidWithout
			} else {
				obj.Rowid = RowidHas
			}
		}
		obj.RowidShadowed = shadowsRowid(colRows)
		for _, cr := range colRows {
			if cr.PrimaryKey > 0 {
				obj.PrimaryKeyCount++
			}
		}
	default:
		return nil, false
	}

	for _, cr := range colRows {
		col := Column{
			Name:         cr.Name,
			DeclaredType: cr.DeclaredType,
			Hidden:       cr.Hidden != 0,
			Insertable:   cr.Hidden == 0 && obj.WriteEligible,
			PrimaryKey:   cr.PrimaryKey,
		}
		if col.Insertable {
			obj.InsertableCount++
		}
		obj.Columns = append(obj.Columns, col)
	}
	return &obj, true
}

// isVirtualTableSQL reports whether the stored object SQL declares a virtual
// table, matched case-insensitively on its authoritative CREATE prefix.
func isVirtualTableSQL(sqlText string) bool {
	s := strings.TrimSpace(sqlText)
	const marker = "create virtual table"
	return len(s) >= len(marker) && strings.EqualFold(s[:len(marker)], marker)
}

// isWithoutRowidSQL reports whether a CREATE TABLE statement ends with the
// WITHOUT ROWID clause, which SQL grammar places at the very end of the
// statement, so detection is suffix-based: trailing whitespace, one final
// semicolon, and one trailing single-line comment are tolerated before the
// case-insensitive comparison.
func isWithoutRowidSQL(sqlText string) bool {
	s := stripStatementTail(sqlText)
	if s == "" {
		return false
	}
	const marker1, marker2 = "without", "rowid"
	words := strings.Fields(s)
	if len(words) < 2 {
		return false
	}
	return strings.EqualFold(words[len(words)-2], marker1) && strings.EqualFold(words[len(words)-1], marker2)
}

// stripStatementTail removes everything after a CREATE TABLE statement's last
// real token: trailing whitespace, one final semicolon, and trailing single-
// line comments, in any order. Only a comment segment containing no newline
// is cut, so interior or block content is never interpreted as a comment.
func stripStatementTail(s string) string {
	s = strings.TrimSpace(s)
	for {
		if strings.HasSuffix(s, ";") {
			s = strings.TrimSpace(s[:len(s)-1])
			continue
		}
		if i := strings.LastIndex(s, "--"); i >= 0 && !strings.ContainsAny(s[i+2:], "\r\n") {
			s = strings.TrimSpace(s[:i])
			continue
		}
		return s
	}
}

// rowidAliasNames lists, lower-cased, every identifier SQLite accepts as the
// implicit rowid alias. Declaring any of them shadows rowid addressing for a
// has-rowid table.
var rowidAliasNames = [...]string{"rowid", "_rowid_", "oid"}

// shadowsRowid reports whether any declared column occupies one of the three
// rowid alias names, compared case-insensitively because SQL identifiers are
// case-insensitive there too.
func shadowsRowid(colRows []ColumnRow) bool {
	for _, cr := range colRows {
		lower := strings.ToLower(cr.Name)
		for _, alias := range rowidAliasNames {
			if lower == alias {
				return true
			}
		}
	}
	return false
}
