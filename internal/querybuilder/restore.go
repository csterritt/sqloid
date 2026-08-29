// Builder restoration from a normalized history state (Issues #20 and #35):
// one pure seam that rebuilds a QueryBuilder from a stored HistoryState
// through the canonical immutable transitions, so every complete builder
// field — command, table identity, ordered projection entries, WHERE
// column/operator/entered value/bound type, GROUP BY order, ORDER BY
// expression/direction, and Limit — is reproduced byte-for-byte. The input
// state is never mutated and the source history store is untouched: callers
// receive a fresh snapshot they may edit freely.

package querybuilder

import (
	"strconv"

	"github.com/chris/sqloid/internal/schema"
)

// RestoreBuilder reconstructs a QueryBuilder from a normalized HistoryState
// against the given catalog, returning (builder, true). It returns (q, false)
// when a stored identity cannot resolve — an unselected state, a table that
// is not currently eligible for the stored command, or a WHERE/SET/INSERT
// column no longer eligible — so a caller never installs a partially
// restored builder. A restored builder satisfies HistoryState().Equal(state)
// whenever every stored identity resolves against the catalog.
func RestoreBuilder(state HistoryState, catalog *schema.Catalog) (QueryBuilder, bool) {
	q := NewQuery().RefreshSchema(catalog)
	if catalog == nil {
		return QueryBuilder{}, false
	}
	if !state.Command.Selected() || !state.TableSet || state.Table == "" {
		return QueryBuilder{}, false
	}
	q = q.SelectCommand(state.Command)
	q = q.SelectTable(state.Table)
	if _, ok := q.SelectedTable(); !ok || q.command != state.Command {
		return QueryBuilder{}, false
	}
	q, ok := restoreProjection(q, state)
	if !ok {
		return QueryBuilder{}, false
	}
	q, ok = restoreWhere(q, state)
	if !ok {
		return QueryBuilder{}, false
	}
	for _, g := range state.Groups {
		next, ok := q.AcceptGroupColumn(g)
		if !ok {
			return QueryBuilder{}, false
		}
		q = next
	}
	if state.OrderSet {
		next, ok := q.AcceptOrderBy(state.OrderExpression)
		if !ok {
			return QueryBuilder{}, false
		}
		q = next.SetOrderDirection(state.OrderDirection)
	}
	if state.LimitHas {
		q = q.SetLimitInput(strconv.FormatInt(state.LimitValue, 10))
	} else {
		q = q.SetLimitInput("")
	}
	q, ok = restoreSets(q, state)
	if !ok {
		return QueryBuilder{}, false
	}
	q, ok = restoreInserts(q, state)
	if !ok {
		return QueryBuilder{}, false
	}
	return q, true
}

// restoreProjection rebuilds the ordered projection entries. The fresh
// SELECT default (a sole wildcard) is cleared first, then each stored entry
// is re-committed in order.
func restoreProjection(q QueryBuilder, state HistoryState) (QueryBuilder, bool) {
	if state.Command != CommandSelect {
		return q, true
	}
	for len(q.projection) > 0 {
		q = q.RemoveLatestProjection()
	}
	for _, e := range state.Projection {
		switch e.Kind {
		case ProjectionWildcard:
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
		case ProjectionCountStar:
			if !q.ProjectionEmpty() {
				return QueryBuilder{}, false
			}
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
			if got := q.ProjectionEntries(); len(got) != 1 || got[0].Kind != ProjectionCountStar {
				return QueryBuilder{}, false
			}
		case ProjectionColumn:
			out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: e.Column})
			if out.PendingAggregate == nil {
				return QueryBuilder{}, false
			}
			next := out.Builder.CompleteProjectionAggregate(e.Column, e.Aggregate)
			got := next.Builder.ProjectionEntries()
			if len(got) != len(q.ProjectionEntries())+1 {
				return QueryBuilder{}, false
			}
			q = next.Builder
		default:
			return QueryBuilder{}, false
		}
	}
	return q, true
}

// restoreWhere rebuilds the committed predicate through the guided draft
// flow, so the operator, parsed value, and exact entered representation are
// reproduced verbatim.
func restoreWhere(q QueryBuilder, state HistoryState) (QueryBuilder, bool) {
	if !state.WhereSet {
		return q, true
	}
	next, ok := q.StartWhere(state.WhereColumn)
	if !ok {
		return QueryBuilder{}, false
	}
	draft, ok := next.WhereDraft().ChooseOperator(state.WhereOperator)
	if !ok {
		return QueryBuilder{}, false
	}
	next = next.ApplyWhereDraft(draft)
	if state.WhereHasValue {
		draft, ok = draft.SubmitValue(state.WhereEntered)
		if !ok {
			return QueryBuilder{}, false
		}
		next = next.ApplyWhereDraft(draft)
	}
	q, ok = next.CommitWhereDraft()
	if !ok {
		return QueryBuilder{}, false
	}
	return q, true
}

// restoreSets rebuilds the ordered UPDATE SET assignments, reproducing each
// choice and — for Value choices — the exact submitted representation.
func restoreSets(q QueryBuilder, state HistoryState) (QueryBuilder, bool) {
	if len(state.Sets) == 0 {
		return q, true
	}
	for _, s := range state.Sets {
		next, ok := q.AcceptSetColumn(s.Column)
		if !ok {
			return QueryBuilder{}, false
		}
		q = next
		q, ok = q.ChooseSetAssignment(s.Column, s.Choice)
		if !ok {
			return QueryBuilder{}, false
		}
		if s.HasValue {
			q, ok = q.SubmitSetValue(s.Column, s.Entered)
			if !ok {
				return QueryBuilder{}, false
			}
		}
	}
	return q, true
}

// restoreInserts rebuilds the per-column INSERT prompt states in declared
// order, reproducing each choice and exact submitted value representation.
func restoreInserts(q QueryBuilder, state HistoryState) (QueryBuilder, bool) {
	if len(state.Inserts) == 0 {
		return q, true
	}
	q = q.BeginInsertPrompts()
	for _, c := range state.Inserts {
		next, ok := q.ChooseInsertColumn(c.Column, c.Choice)
		if !ok {
			return QueryBuilder{}, false
		}
		q = next
		if c.HasValue {
			q, ok = q.SubmitInsertValue(c.Column, c.Entered)
			if !ok {
				return QueryBuilder{}, false
			}
		}
	}
	return q, true
}
