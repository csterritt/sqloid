// SELECT SQL rendering (Issue #18): one deterministic, safely quoted SELECT
// statement assembled from committed builder state per the Query Grammar in
// Notes/PRD-sqloid.md. Identifiers come from the Schema and quote as single
// atoms; aggregates, directions, and LIMIT values render only through closed
// typed tokens or parsed integers, so no arbitrary user text can reach the
// statement. Bound parameters stay separate through SelectParams.

package querybuilder

import "strings"

// SelectSQL renders the current snapshot's SELECT statement exactly: quoted
// projection over the quoted table, then WHERE, GROUP BY (commit order),
// ORDER BY (single committed expression with direction), and LIMIT in grammar
// order. An empty result means the snapshot cannot yet be rendered at all —
// command, table, or any required piece missing — never a partially valid
// query; validity stays the job of FirstInvalidIssue.
func (q QueryBuilder) SelectSQL() string {
	if q.command != CommandSelect || !q.tableSet {
		return ""
	}
	projection := q.renderProjection()
	if projection == "" {
		return ""
	}
	parts := []string{"SELECT " + projection + " FROM " + quoteIdentifierAtom(q.table)}
	if pred := q.WherePredicate(); pred.State() == WhereComplete {
		parts = append(parts, "WHERE "+pred.SQL())
	}
	if groups := q.GroupByEntries(); len(groups) > 0 {
		atoms := make([]string, 0, len(groups))
		for _, g := range groups {
			atoms = append(atoms, quoteIdentifierAtom(g))
		}
		parts = append(parts, "GROUP BY "+joinSQLList(atoms))
	}
	if cand, dir, ok := q.OrderBySelection(); ok {
		token, err := dir.SQLToken()
		if err != nil {
			return "" // unreachable by construction; refuse unsafe text
		}
		expr := cand.sqlExpr()
		if expr == "" {
			return "" // unresolved aggregate identity: refuse rather than guess
		}
		parts = append(parts, "ORDER BY "+expr+" "+token)
	}
	if limit := renderLimit(q); limit != "" {
		parts = append(parts, limit)
	}
	return strings.Join(parts, " ")
}

// SelectParams returns this snapshot's bound parameter values in deterministic
// order — currently only the completed WHERE predicate's single value when it
// takes one. Projection, grouping, ordering, and LIMIT contribute no
// parameters: identifiers are quoted atoms and the limit is a literal integer.
func (q QueryBuilder) SelectParams() []any {
	if q.command != CommandSelect || !q.tableSet {
		return nil
	}
	return q.WherePredicate().Params()
}
