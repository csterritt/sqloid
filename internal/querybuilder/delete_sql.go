// DELETE command state and SQL generation (Issue #38): DELETE composes the
// selected write-eligible table identity, Issue #17's shared optional WHERE
// predicate, Issue #14's canonical identifier quoting and parameter binding,
// and Issue #19's authoritative runnable evaluator. It adds no state of its
// own and performs no database work; the destructive preparation and
// execution stages arrive later.

package querybuilder

// DeleteSQL renders the complete DELETE statement with one safely quoted
// table atom and, when a predicate is committed, its complete WHERE clause
// appended exactly once. An absent WHERE is valid and renders the bare
// unqualified form that targets every row. It returns empty unless the
// authoritative runnable report accepts the state.
func (q QueryBuilder) DeleteSQL() string {
	if q.command != CommandDelete || !q.RunnableReport().Runnable {
		return ""
	}
	statement := "DELETE FROM " + quoteIdentifierAtom(q.table)
	if predicate := q.WherePredicate(); predicate.State() == WhereComplete {
		statement += " WHERE " + predicate.SQL()
	}
	return statement
}

// DeleteParams returns fresh bound parameters in placeholder order: exactly
// the committed predicate's parameter — none for no WHERE and for the
// IS NULL / IS NOT NULL operators, one universally parsed value otherwise.
// It returns nil unless the DELETE state is runnable.
func (q QueryBuilder) DeleteParams() []any {
	if q.command != CommandDelete || !q.RunnableReport().Runnable {
		return nil
	}
	return q.WhereParams()
}
