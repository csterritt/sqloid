// Reusable whole-value clearing transitions (Issue #19 Tasks 3–4): one
// immutable contract that removes an entire completed value — exact entered
// representation, parsed/bound type, and submission marker — atomically, while
// preserving the surrounding structural choices. Clearing a value-taking WHERE
// keeps its selected column and operator but reopens them as an incomplete
// awaiting-value draft; clearing Limit restores the valid unbounded state;
// clearing an UPDATE/INSERT Value keeps the Value choice and column identity
// but drops the submission. Already absent or empty whole fields are unchanged
// no-ops: no state moves, and the authoritative runnable report derives the
// resulting validity. These transitions produce no UI action and no history
// mutation by construction.

package querybuilder

// ClearWhereValue drops the submitted whole value of a completed, committed
// value-taking WHERE predicate, restoring its selected column and operator as
// an open awaiting-value draft so the predicate becomes incomplete while the
// structural choices survive. Absent predicates, null operators, predicates
// without a submission, and open drafts are unchanged no-ops.
func (q QueryBuilder) ClearWhereValue() QueryBuilder {
	if q.whereDrafting || !q.whereSet {
		return q
	}
	p := q.where
	if p.State() != WhereComplete || !p.op.TakesValue() {
		return q
	}
	draft := WherePredicate{state: WhereAwaitingValue, col: p.col, op: p.op}
	next := q
	next.where, next.whereSet = WherePredicate{}, false
	next.whereDrafting = true
	next.whereDraft = draft
	return next
}

// ClearLimitValue resets the whole Limit field to the valid unbounded state:
// both the entered representation and any accepted integer are removed.
// Clearing an already empty Limit returns an identical snapshot.
func (q QueryBuilder) ClearLimitValue() QueryBuilder {
	return q.SetLimitInput("")
}

// ClearSetValue drops the whole submitted value of the UPDATE Value choice
// naming column, keeping the Value choice and column identity selected but
// incomplete. A column with no submitted Value entry is an unchanged no-op.
func (q QueryBuilder) ClearSetValue(column string) QueryBuilder {
	next := q
	next.sets = append([]SetAssignment(nil), q.sets...)
	changed := false
	for i := range next.sets {
		a := &next.sets[i]
		if a.Column == column && a.choice == SetChoiceValue && a.submitted {
			a.value, a.input, a.submitted = Value{}, "", false
			changed = true
		}
	}
	if !changed {
		return q
	}
	return next
}

// ClearInsertValue drops the whole submitted value of the INSERT Value choice
// naming column, keeping the Value choice and column identity selected but
// incomplete. A column with no submitted Value entry is an unchanged no-op.
func (q QueryBuilder) ClearInsertValue(column string) QueryBuilder {
	next := q
	next.inserts = append([]InsertColumn(nil), q.inserts...)
	changed := false
	for i := range next.inserts {
		c := &next.inserts[i]
		if c.Column == column && c.choice == InsertChoiceValue && c.submitted {
			c.value, c.input, c.submitted = Value{}, "", false
			changed = true
		}
	}
	if !changed {
		return q
	}
	return next
}
