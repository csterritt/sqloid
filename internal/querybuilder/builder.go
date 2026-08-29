// Package querybuilder stores the structured state of a Sqloid query as
// UI-independent data: the selected command, the selected table, refreshed
// eligible objects from the Schema catalog, downstream command-specific fields,
// and the next required builder field, per Issue #11 and the QueryBuilder
// Module Design in Notes/PRD-sqloid.md.
//
// Every transition is immutable: methods return a new QueryBuilder value and
// never mutate their receiver. The package consumes object kinds and write
// eligibility from internal/schema instead of duplicating catalog rules; it
// never imports internal/ui, renders no copy, or implements popup behavior;
// command-specific SQL renderers remain pure functions over this state.
package querybuilder
