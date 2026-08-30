// Destructive-preparation statement construction (Issue #40), per the
// Estimate SQL and modal decision in Notes/PRD-sqloid.md. The canonical
// standalone rendered UPDATE/DELETE SQL reuses Issues #37/#38's executable
// structured state and renders every Value through Issue #14's sole shared
// typed-literal renderer and identifier atoms — the modal owns no serializer.
// The independent estimate request is exactly `SELECT COUNT(*) FROM <quoted
// target> [WHERE <identical predicate>]`, binding only the shared predicate's
// parameters in predicate order and never any UPDATE SET value.

package querybuilder

// UpdateRenderedSQL renders the canonical standalone UPDATE statement with
// safely quoted table and column atoms and every submitted SET Value
// serialized through the shared typed-literal renderer: INTEGER/REAL/TEXT
// literals exactly as parsed, TEXT quote-doubling, typed TEXT `NULL` as a
// quoted text literal, and the SQL-NULL assignment choice plus null-operator
// predicates as keywords. It returns empty unless the authoritative runnable
// report accepts the state.
func (q QueryBuilder) UpdateRenderedSQL() string {
	if q.command != CommandUpdate || !q.RunnableReport().Runnable {
		return ""
	}
	assignments := make([]string, 0, len(q.sets))
	for _, assignment := range q.sets {
		rendered, ok := renderedSetAssignment(assignment)
		if !ok {
			return ""
		}
		assignments = append(assignments, rendered)
	}
	statement := "UPDATE " + quoteIdentifierAtom(q.table) + " SET " + joinSQLList(assignments)
	if clause := q.renderedWhereClause(); clause != "" {
		statement += " " + clause
	}
	return statement
}

// DeleteRenderedSQL renders the canonical standalone DELETE statement with
// one safely quoted table atom and, when a predicate is committed, its
// complete WHERE clause with the bound value serialized through the shared
// typed-literal renderer. It returns empty unless the authoritative runnable
// report accepts the state.
func (q QueryBuilder) DeleteRenderedSQL() string {
	if q.command != CommandDelete || !q.RunnableReport().Runnable {
		return ""
	}
	statement := "DELETE FROM " + quoteIdentifierAtom(q.table)
	if clause := q.renderedWhereClause(); clause != "" {
		statement += " " + clause
	}
	return statement
}

// EstimateSQL renders the independent matching-target estimate request
// exactly as `SELECT COUNT(*) FROM <quoted target> [WHERE <identical
// predicate>]`. The predicate is the shared committed WHERE clause verbatim
// — the same '?' placeholder SQL as the executable statements — so one
// binding seam serves both requests. It never contains UPDATE SET fragments.
// It returns empty unless the destructive state is runnable.
func (q QueryBuilder) EstimateSQL() string {
	switch q.command {
	case CommandUpdate, CommandDelete:
	default:
		return ""
	}
	if !q.RunnableReport().Runnable {
		return ""
	}
	statement := "SELECT COUNT(*) FROM " + quoteIdentifierAtom(q.table)
	if predicate := q.WherePredicate(); predicate.State() == WhereComplete {
		statement += " WHERE " + predicate.SQL()
	}
	return statement
}

// EstimateParams returns the estimate statement's bound parameters: exactly
// the committed predicate's values in predicate order — none for a no-WHERE
// statement or an IS NULL / IS NOT NULL predicate — and never any UPDATE SET
// parameter. It returns nil unless the destructive state is runnable.
func (q QueryBuilder) EstimateParams() []any {
	switch q.command {
	case CommandUpdate, CommandDelete:
	default:
		return nil
	}
	if !q.RunnableReport().Runnable {
		return nil
	}
	return q.WhereParams()
}

// renderedSetAssignment renders one UPDATE SET assignment with the shared
// atoms: the SQL NULL choice stays the keyword, and a submitted Value choice
// serializes through the shared literal renderer. A value that cannot render
// safely reports false so the whole statement renders empty.
func renderedSetAssignment(assignment SetAssignment) (string, bool) {
	quoted := quoteIdentifierAtom(assignment.Column)
	if assignment.Choice() == SetChoiceNull {
		return quoted + " = NULL", true
	}
	value, ok := assignment.SubmittedValue()
	if !ok {
		return "", false
	}
	literal, err := RenderSQLLiteral(value.Literal())
	if err != nil {
		return "", false
	}
	return quoted + " = " + literal, true
}

// renderedWhereClause renders the committed predicate's WHERE clause with the
// bound value serialized through the shared literal renderer; null-operator
// predicates render their keywords. Anything short of a complete predicate —
// or a value that cannot render safely — renders empty.
func (q QueryBuilder) renderedWhereClause() string {
	predicate := q.WherePredicate()
	if predicate.State() != WhereComplete {
		return ""
	}
	token, err := predicate.op.SQLToken()
	if err != nil {
		// Completed predicates only exist with operator tokens validated at
		// construction; an unrenderable completion stays silent.
		return ""
	}
	quoted := quoteIdentifierAtom(predicate.col.Name)
	if !predicate.op.TakesValue() {
		return "WHERE " + quoted + " " + token
	}
	literal, err := RenderSQLLiteral(predicate.value.Literal())
	if err != nil {
		return ""
	}
	return "WHERE " + quoted + " " + token + " " + literal
}
