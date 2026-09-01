// Pure INSERT statement generation (Issue #39 Task 4), per the INSERT
// handling decision in Notes/PRD-sqloid.md. The complete prompt state is
// traversed in authoritative schema prompt order: Value columns contribute a
// `?` placeholder and one bound parameter each; NULL columns stay in both
// lists as the SQL keyword NULL with no parameter; Default/Omit columns are
// absent from both lists. When every prompted column is omitted the exact
// `INSERT INTO <quoted table> DEFAULT VALUES` form is emitted with no
// parameters and no empty parentheses. Incomplete, zero-insertable-column,
// or stale-prompt state (Issue #67: a stored prompt whose column is dropped,
// hidden, generated, or otherwise no longer insertable) renders nothing: the
// authoritative runnable report gates both functions before any stored prompt
// is traversed, so no partial SQL, stale identifier, or former bound value
// ever escapes. Identifier quoting reuses Issue #14's atom-by-atom quoting
// and parameters reuse the universal Value binding; no hidden-input
// synthesis, default inference, or AUTOINCREMENT special-casing exists.

package querybuilder

// InsertSQL renders the complete INSERT statement with safely quoted table
// and column atoms, ordered columns, and per-choice values. It returns empty
// unless the authoritative runnable report accepts the state.
func (q QueryBuilder) InsertSQL() string {
	if q.command != CommandInsert || !q.RunnableReport().Runnable {
		return ""
	}
	columns := make([]string, 0, len(q.inserts))
	values := make([]string, 0, len(q.inserts))
	for _, c := range q.inserts {
		switch c.choice {
		case InsertChoiceValue:
			columns = append(columns, quoteIdentifierAtom(c.Column))
			values = append(values, "?")
		case InsertChoiceNull:
			columns = append(columns, quoteIdentifierAtom(c.Column))
			values = append(values, "NULL")
		default:
			// Default/Omit: absent from both lists entirely.
		}
	}
	if len(columns) == 0 {
		return "INSERT INTO " + quoteIdentifierAtom(q.table) + " DEFAULT VALUES"
	}
	return "INSERT INTO " + quoteIdentifierAtom(q.table) +
		" (" + joinSQLList(columns) + ") VALUES (" + joinSQLList(values) + ")"
}

// InsertParams returns fresh bound parameters in placeholder order: the
// submitted Value choices in schema prompt order, skipping SQL NULL and
// Default/Omit columns. It returns nil when nothing binds or the INSERT
// state is not runnable.
func (q QueryBuilder) InsertParams() []any {
	if q.command != CommandInsert || !q.RunnableReport().Runnable {
		return nil
	}
	var params []any
	for _, c := range q.inserts {
		if value, ok := c.SubmittedValue(); ok {
			params = append(params, value.ParamValue())
		}
	}
	return params
}
