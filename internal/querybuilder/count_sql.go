// Complete-SELECT result count construction (Issue #24), per the Paging
// consistency decision in Notes/PRD-sqloid.md. The count statement wraps the
// same safely quoted SELECT statement this snapshot renders — including any
// user LIMIT inside the subquery, so rows beyond the Limit are irrelevant to
// completeness — and never implies a table count or a pre-Limit count. Bound
// parameters and their order are exactly the SELECT's, unchanged.
package querybuilder

// CountSQL renders the exact count statement for the complete SELECT result:
// `SELECT COUNT(*) FROM (<SelectSQL()>)`. An empty result means the snapshot
// is not a runnable SELECT — the authoritative RunnableReport gates the whole
// SELECT renderer family through SelectSQL (Issue #66) — never a partially
// valid statement; validity and renderability stay exactly those of SelectSQL
// itself.
func (q QueryBuilder) CountSQL() string {
	base := q.SelectSQL()
	if base == "" {
		return ""
	}
	return "SELECT COUNT(*) FROM (" + base + ")"
}

// CountParams returns the count statement's bound parameters, which are
// exactly the complete SELECT's ordered parameters unchanged: only the WHERE
// predicate contributes values, and parameter order must stay identical so
// one execution seam can serve both requests. It returns nil unless the
// authoritative RunnableReport accepts the SELECT state (Issue #66).
func (q QueryBuilder) CountParams() []any {
	return q.SelectParams()
}
