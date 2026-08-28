// Catalog-to-tracer composition seam, per Issue #10: selects one object from
// an already-built catalog snapshot and renders its safely identifier-quoted
// hardcoded SELECT *. This seam exists only to de-risk the Connection ↔
// Schema ↔ UI integration stack; Issue #22 must replace it entirely rather
// than extend it into the production query path. No builder, revalidation,
// paging, count, history, cancellation, or write behavior lives here.

package schema

import (
	"fmt"
	"strings"
)

// TracerError marks composition failures of the disposable tracer: the chosen
// object name is not present in the supplied catalog snapshot. Terminal
// wording and recovery stay owned by later issues.
type TracerError struct {
	Name string // rejected object name, as requested by the caller
}

// Error returns lower-case diagnostic text naming the missing object without
// claiming any classification beyond rejection itself.
func (e *TracerError) Error() string {
	return fmt.Sprintf("%q: not present in the refreshed schema catalog", e.Name)
}

// ChooseTracerTarget returns the cataloged object named name from cat,
// rejecting names absent from the refreshed snapshot so execution never runs
// against stale or unvalidated identifiers. Any catalog-selected object kind
// (ordinary table, virtual table, or view) is acceptable because the tracer
// only ever executes a SELECT. The returned Object remains immutably owned by
// the catalog.
func ChooseTracerTarget(cat *Catalog, name string) (*Object, error) {
	for _, obj := range cat.Objects {
		if obj.Name == name {
			return obj, nil
		}
	}
	return nil, &TracerError{Name: name}
}

// SelectAllSQL renders the tracer's one hardcoded statement against obj:
// SELECT * with no projection, predicate, ordering, limit, or parameters, so
// nothing user-entered can reach SQL text. Only the table name appears as an
// identifier, quoted by doubling embedded double quotes so even unusual
// cataloged names execute safely.
func SelectAllSQL(obj *Object) string {
	return fmt.Sprintf("SELECT * FROM %s", quoteIdentifier(obj.Name))
}

// quoteIdentifier renders s as a SQLite double-quoted identifier.
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
