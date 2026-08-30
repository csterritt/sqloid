// Package schema describes the objects and columns of a database's main
// schema independently of any UI or driver concern, per Issue #9 and the
// Schema scope and Schema metadata decisions in Notes/PRD-sqloid.md. It owns
// the catalog types (object kinds, rowid capabilities, column metadata) and
// the deterministic rules that turn authoritative SQLite metadata rows into
// them; internal/connection gathers those rows through its request boundary,
// so this package never touches the SQLite driver and never depends on
// internal/ui or internal/querybuilder. Declared column types are carried as
// metadata only: nothing here adds type-specific input behavior.
package schema

import (
	"fmt"
	"strings"
)

// ObjectKind identifies which kind of named object an Object describes.
type ObjectKind int

const (
	// KindOrdinaryTable is a plain rowid-capable or WITHOUT ROWID table.
	KindOrdinaryTable ObjectKind = iota + 1
	// KindVirtualTable is a table backed by a virtual-table module such as
	// fts5, declared through CREATE VIRTUAL TABLE.
	KindVirtualTable
	// KindView is a stored SELECT defined by CREATE VIEW; views are always
	// SELECT-only for Sqloid's purposes.
	KindView
)

// String renders the human-facing name of the object kind used in tests and
// diagnostics.
func (k ObjectKind) String() string {
	switch k {
	case KindOrdinaryTable:
		return "ordinary-table"
	case KindVirtualTable:
		return "virtual-table"
	case KindView:
		return "view"
	default:
		return fmt.Sprintf("ObjectKind(%d)", int(k))
	}
}

// RowidCapability classifies whether a rowid column may be addressed on an
// object, per the Schema metadata decision {has-rowid, without-rowid,
// not-applicable}.
type RowidCapability int

const (
	// RowidApplicable has no zero-value meaning: every object receives an
	// explicit capability. It exists to keep the iota alignment explicit.
	RowidApplicable = iota
	// RowidHas means the object supports ORDER BY / addressing by rowid,
	// including the case of an undeclared alias column.
	RowidHas
	// RowidWithout means the object is a WITHOUT ROWID table or otherwise
	// cannot be ordered by rowid but still accepts writes.
	RowidWithout
	// RowidNotApplicable means rowid ordering never applies to this kind of
	// object, such as views and virtual tables.
	RowidNotApplicable
)

// String renders the human-facing name of the capability used in tests and
// diagnostics.
func (c RowidCapability) String() string {
	switch c {
	case RowidHas:
		return "has-rowid"
	case RowidWithout:
		return "without-rowid"
	case RowidNotApplicable:
		return "not-applicable"
	default:
		return fmt.Sprintf("RowidCapability(%d)", int(c))
	}
}

// Column is one declared column of a cataloged object. Insertability comes
// from PRAGMA table_xinfo: hidden and generated columns are never insertable,
// and no column of a SELECT-only object (a view) is insertable. DeclaredType
// is passed through verbatim as metadata; it deliberately influences nothing
// else because v1 universal value entry has no type-specific behavior — the
// sole exception is the INTEGER PRIMARY KEY omission hint, which derives
// from declared metadata, never from type-based input filtering.
type Column struct {
	Name         string // identifier exactly as declared in the cataloged object
	DeclaredType string // declared affinity/type text, empty when untyped
	Hidden       bool   // true when table_xinfo marks the column hidden or generated
	Insertable   bool   // true when INSERT may target the column explicitly
	PrimaryKey   int    // raw table_xinfo pk value; 0 when not part of the primary key
}

// InsertOmissionHint is the exact user-facing hint attached to exactly one
// prompt shape: an insertable column that is the single-column INTEGER
// PRIMARY KEY rowid alias of a has-rowid table. Omitting that column lets
// SQLite auto-assign the rowid, so the builder surfaces the hint verbatim
// without changing the offered choices or any behavior.
const InsertOmissionHint = "(auto-assigned if omitted)"

// Object is one cataloged main-schema object together with everything the
// builder needs about its shape. Objects are owned immutably by their Catalog;
// Columns preserves declared order (cid ascending from table_xinfo).
type Object struct {
	Name            string          // object name from main.sqlite_master
	Kind            ObjectKind      // ordinary/virtual/view classification
	WriteEligible   bool            // true only for ordinary and virtual tables; views stay SELECT-only
	Rowid           RowidCapability // whether rowid can address this object
	RowidShadowed   bool            // true when a declared column occupies the rowid alias slot
	InsertableCount int             // number of explicitly insertable columns; zero marks the non-runnable case
	PrimaryKeyCount int             // number of primary-key columns; 1 with INTEGER PRIMARY KEY marks the rowid alias
	Columns         []Column        // declared-order column records
}

// InsertHint reports the exact omission hint for one insertable column of
// this object: InsertOmissionHint only when the column is the single-column
// INTEGER PRIMARY KEY rowid alias of a has-rowid table — declared type
// exactly INTEGER (case-insensitive), pk slot 1, no other key columns, and
// the table keeps its rowid. WITHOUT ROWID tables, multi-column keys,
// similar declared types (INT, BIGINT, UINT64…), virtual tables, and
// non-primary INTEGER columns never receive the hint. All other columns and
// insertability facts are unaffected.
func (o *Object) InsertHint(col Column) (string, bool) {
	if o.Rowid != RowidHas || o.PrimaryKeyCount != 1 || col.PrimaryKey != 1 || !col.Insertable {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSpace(col.DeclaredType), "INTEGER") {
		return "", false
	}
	return InsertOmissionHint, true
}

// Catalog is the refreshed snapshot of the main schema returned after every
// successful refresh. Version is the PRAGMA schema_version read during the
// same request as the object rows, and Objects lists every eligible object in
// ascending case-sensitive name order so identical contents produce identical
// catalogs regardless of sqlite_master enumeration order.
type Catalog struct {
	Version int64
	Objects []*Object
}
