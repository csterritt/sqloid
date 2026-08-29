package querybuilder

// UpdateSQL renders the complete UPDATE statement with safely quoted table and
// column atoms, ordered assignments, and the optional committed WHERE clause.
// It returns empty unless the authoritative runnable report accepts the state.
func (q QueryBuilder) UpdateSQL() string {
	if q.command != CommandUpdate || !q.RunnableReport().Runnable {
		return ""
	}
	assignments := make([]string, 0, len(q.sets))
	for _, assignment := range q.sets {
		value := "?"
		if assignment.choice == SetChoiceNull {
			value = "NULL"
		}
		assignments = append(assignments, quoteIdentifierAtom(assignment.Column)+" = "+value)
	}
	statement := "UPDATE " + quoteIdentifierAtom(q.table) + " SET " + joinSQLList(assignments)
	if predicate := q.WherePredicate(); predicate.State() == WhereComplete {
		statement += " WHERE " + predicate.SQL()
	}
	return statement
}

// UpdateParams returns fresh bound parameters in placeholder order: submitted
// Value assignments in SET order, skipping SQL NULL, followed by WHERE value.
// It returns nil unless the UPDATE state is runnable.
func (q QueryBuilder) UpdateParams() []any {
	if q.command != CommandUpdate || !q.RunnableReport().Runnable {
		return nil
	}
	params := make([]any, 0, len(q.sets)+1)
	for _, assignment := range q.sets {
		if value, ok := assignment.SubmittedValue(); ok {
			params = append(params, value.ParamValue())
		}
	}
	params = append(params, q.WhereParams()...)
	if len(params) == 0 {
		return nil
	}
	return params
}
