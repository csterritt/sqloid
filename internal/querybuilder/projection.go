// SELECT projection state for the wildcard and COUNT(*) path (Issue #15):
// candidate derivation, direct wildcard/sentinel transitions, named-column
// continuation to aggregate selection, and removal. The model consumes typed
// visible column metadata from internal/schema and stays UI-independent: no
// popup behavior, no rendering, and no SQL construction, per the Builder
// lifecycle decision and the Query Grammar of Notes/PRD-sqloid.md.
//
// Candidate identity is the Kind/Column pair, deliberately separate from
// Display text so a real column named `*` or `COUNT(*)` can never collide
// with the synthetic wildcard or sentinel identities. Aggregates exist only
// on named-column identities — MIN(*), MAX(*), AVG(*), and SUM(*) are not
// representable by construction. Entries commit in insertion order (Issue
// #16): exact repeated named pairs are rejected as no-ops, the sentinel
// duplicates defensively, and wildcard selection atomically replaces the
// whole list, staying sole until removal empties the projection.

package querybuilder

import "github.com/chris/sqloid/internal/schema"

// ProjectionKind classifies one projection identity.
type ProjectionKind int

const (
	// ProjectionWildcard is the sole wildcard `*` identity.
	ProjectionWildcard ProjectionKind = iota + 1
	// ProjectionCountStar is the synthetic bare `COUNT(*)` sentinel identity.
	ProjectionCountStar
	// ProjectionColumn is a named column identity carrying Column.
	ProjectionColumn
)

// Aggregate names the aggregate applied to a projected named column,
// reusing the typed choice from the SQL atoms layer (sql_atoms.go); the zero
// value is the plain nonaggregate entry, exported here as AggregateValue.
// There is no aggregate-on-wildcard value, so wildcard aggregates cannot be
// built.
const (
	// AggregateValue is the zero aggregate: the raw column values projected
	// without any SQL function.
	AggregateValue = Aggregate(0)
)

// ProjectionCandidate is one offered Column(s) choice: an identity (Kind plus
// optional Column) separate from its display label.
type ProjectionCandidate struct {
	Kind   ProjectionKind
	Column string // declared name when Kind == ProjectionColumn; "" otherwise
}

// Key returns a stable string encoding of the candidate identity for callers
// that need flat IDs (such as popup candidates). Synthetic wildcard/sentinel
// identities use reserved prefixes so they can never collide with any
// declared column name.
func (c ProjectionCandidate) Key() string {
	switch c.Kind {
	case ProjectionWildcard:
		return "wildcard:*"
	case ProjectionCountStar:
		return "count-star:(*)"
	default:
		return "column:" + c.Column
	}
}

// Display returns the human-facing label of the candidate. Two candidates may
// share a Display while remaining distinct identities.
func (c ProjectionCandidate) Display() string {
	switch c.Kind {
	case ProjectionWildcard:
		return "*"
	case ProjectionCountStar:
		return "COUNT(*)"
	default:
		return c.Column
	}
}

// ProjectionEntry is one committed projection item in selection order.
type ProjectionEntry struct {
	Kind      ProjectionKind
	Column    string    // declared name when Kind == ProjectionColumn
	Aggregate Aggregate // AggregateValue for plain columns; zero otherwise
}

// ProjectionOutcome is the result of one projection acceptance: the next
// builder snapshot, whether the Column(s) popup should reopen, and — for a
// named column that must continue to aggregate selection — the pending
// named identity awaiting its aggregate choice.
type ProjectionOutcome struct {
	Builder          QueryBuilder         // snapshot after applying the transition
	ReopenColumns    bool                 // reopen the Column(s) popup (direct commits and aggregate completion)
	PendingAggregate *ProjectionCandidate // non-nil when a named column awaits its aggregate choice
}

// projectionReady reports whether projection transitions apply at all:
// only a SELECT with a selected table owns a projection.
func (q QueryBuilder) projectionReady() bool {
	return q.command == CommandSelect && q.tableSet
}

// selectedColumns lists the visible (non-hidden) columns of the selected
// object in Schema order; nil when nothing eligible is selected.
func (q QueryBuilder) selectedColumns() []schema.Column {
	obj := q.findObject(q.table)
	if obj == nil {
		return nil
	}
	out := make([]schema.Column, 0, len(obj.Columns))
	for _, col := range obj.Columns {
		if !col.Hidden {
			out = append(out, col)
		}
	}
	return out
}

// ProjectionCandidates derives the deterministic Column(s) choices for the
// current state. An empty projection offers wildcard `*` first (the popup's
// default highlight), synthetic bare COUNT(*) second, then every visible
// column in Schema order. Once any entry exists both synthetic identities
// disappear, leaving only named columns. A committed wildcard leaves nothing.
// The returned slice is fresh; callers may mutate it freely.
func (q QueryBuilder) ProjectionCandidates() []ProjectionCandidate {
	if !q.projectionReady() || len(q.projection) == 1 && q.projection[0].Kind == ProjectionWildcard {
		return nil
	}
	cols := q.selectedColumns()
	var out []ProjectionCandidate
	if len(q.projection) == 0 {
		out = append(out,
			ProjectionCandidate{Kind: ProjectionWildcard},
			ProjectionCandidate{Kind: ProjectionCountStar})
	}
	for _, col := range cols {
		out = append(out, ProjectionCandidate{Kind: ProjectionColumn, Column: col.Name})
	}
	return out
}

// projectionHasWildcard reports whether any committed entry is the dedicated
// wildcard identity. The wildcard is always the sole entry by construction:
// selection replaces the whole list atomically and nothing may append beside
// it until removal empties the projection again.
func (q QueryBuilder) projectionHasWildcard() bool {
	for _, e := range q.projection {
		if e.Kind == ProjectionWildcard {
			return true
		}
	}
	return false
}

// AcceptProjection applies one accepted candidate (Issue #16 ordered rules).
// A named column never commits here: it hands back the chosen identity as
// PendingAggregate for the aggregate-selection step while leaving builder
// state unchanged; a malformed empty-name identity routes nowhere. Wildcard
// selection — from any prior state, including fully populated ones — replaces
// the whole list with its dedicated sole identity atomically. Bare COUNT(*)
// appends only while the projection is empty; a repeated sentinel transition
// outside the conditional UI path is an unchanged no-op. Duplicate accepts
// return the receiver unchanged without changing focus-transition data.
func (q QueryBuilder) AcceptProjection(c ProjectionCandidate) ProjectionOutcome {
	switch c.Kind {
	case ProjectionWildcard:
		replacement := q
		replacement.projection = []ProjectionEntry{{Kind: ProjectionWildcard}}
		replacement.focus = FieldColumns
		return ProjectionOutcome{Builder: replacement}
	case ProjectionCountStar:
		if !q.ProjectionEmpty() {
			return ProjectionOutcome{Builder: q}
		}
		next := q.appendEntry(ProjectionEntry{Kind: ProjectionCountStar})
		return ProjectionOutcome{Builder: next, ReopenColumns: true}
	case ProjectionColumn:
		if c.Column == "" {
			return ProjectionOutcome{Builder: q}
		}
		pending := c
		return ProjectionOutcome{Builder: q, PendingAggregate: &pending}
	default:
		return ProjectionOutcome{Builder: q}
	}
}

// CompleteProjectionAggregate finishes a pending named-column acceptance by
// appending the (column, aggregate) entry, focusing Column(s), and reopening
// the popup. The column must be a visible declared name and the aggregate one
// of the six supported values; anything else — including any attempt to
// aggregate a wildcard — leaves the builder unchanged with reopen unset.
// A pending aggregate never completes onto an existing wildcard: exclusivity
// leaves it alone until removal empties the projection, per Issue #16.
func (q QueryBuilder) CompleteProjectionAggregate(column string, agg Aggregate) ProjectionOutcome {
	if !q.projectionReady() || agg > AggSum || q.projectionHasWildcard() {
		return ProjectionOutcome{Builder: q}
	}
	found := false
	for _, col := range q.selectedColumns() {
		if col.Name == column {
			found = true
			break
		}
	}
	if !found {
		return ProjectionOutcome{Builder: q}
	}
	entry := ProjectionEntry{Kind: ProjectionColumn, Column: column}
	if agg != AggregateValue {
		entry.Aggregate = agg
	}
	for _, e := range q.projection {
		if e == entry {
			// Exact repeated (column, aggregate) pairs are rejected outright:
			// no reorder, no replace, no focus-transition change.
			return ProjectionOutcome{Builder: q}
		}
	}
	next := q.appendEntry(entry)
	return ProjectionOutcome{Builder: next, ReopenColumns: true}
}

// RemoveProjection removes the committed entry at index, ignoring indexes
// outside the current entries. Use this seam until Issue #16 adds ordered
// editing.
func (q QueryBuilder) RemoveProjection(index int) QueryBuilder {
	if index < 0 || index >= len(q.projection) {
		return q
	}
	next := q
	next.projection = make([]ProjectionEntry, 0, len(q.projection)-1)
	next.projection = append(next.projection, q.projection[:index]...)
	next.projection = append(next.projection, q.projection[index+1:]...)
	return next
}

// appendEntry copies the receiver, appends one entry, and focuses Column(s)
// where that field lives.
func (q QueryBuilder) appendEntry(e ProjectionEntry) QueryBuilder {
	next := q
	next.projection = append(append([]ProjectionEntry(nil), q.projection...), e)
	next.focus = FieldColumns
	return next
}

// RemoveLatestProjection removes the most recently committed projection entry,
// returning a new snapshot without mutating the receiver's slices (Issue #16):
// one entry per call, walking backward through named entries and the bare
// sentinel while every earlier entry keeps its identity and insertion order.
// Removing the last remaining entry — including a sole wildcard — empties the
// projection entirely so the Issue #15 empty-projection candidate sequence
// reappears. Removal never changes focus; an empty projection is an exact
// unchanged no-op.
func (q QueryBuilder) RemoveLatestProjection() QueryBuilder {
	if len(q.projection) == 0 {
		return q
	}
	return q.RemoveProjection(len(q.projection) - 1)
}

// ProjectionEntries returns the committed entries in insertion order as a
// fresh slice.
func (q QueryBuilder) ProjectionEntries() []ProjectionEntry {
	out := make([]ProjectionEntry, len(q.projection))
	copy(out, q.projection)
	return out
}

// ProjectionEmpty reports whether no projection entry exists yet.
func (q QueryBuilder) ProjectionEmpty() bool { return len(q.projection) == 0 }
