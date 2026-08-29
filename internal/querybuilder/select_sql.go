// SELECT SQL rendering (Issue #18): one deterministic, safely quoted SELECT
// statement assembled from committed builder state per the Query Grammar in
// Notes/PRD-sqloid.md. Identifiers come from the Schema and quote as single
// atoms; aggregates, directions, and LIMIT values render only through closed
// typed tokens or parsed integers, so no arbitrary user text can reach the
// statement. Bound parameters stay separate through SelectParams.

package querybuilder

import (
	"strings"

	"github.com/chris/sqloid/internal/schema"
)

// SelectSQL renders the current snapshot's SELECT statement exactly: quoted
// projection over the quoted table, then WHERE, GROUP BY (commit order),
// ORDER BY (single committed expression with direction), and LIMIT in grammar
// order. An empty result means the snapshot cannot yet be rendered at all —
// command, table, or any required piece missing — never a partially valid
// query; validity stays the job of FirstInvalidIssue.
func (q QueryBuilder) SelectSQL() string {
	parts, ok := renderSelectCore(q, false)
	if !ok {
		return ""
	}
	if limit := renderLimit(q); limit != "" {
		parts = append(parts, limit)
	}
	return strings.Join(parts, " ")
}

// renderSelectCore renders the shared SELECT prefix: the quoted projection
// over the quoted table, then WHERE, GROUP BY (commit order), and ORDER BY.
// It appends the implicit `ORDER BY rowid` fallback only when allowRowid
// holds and rowidFallbackEligible confirms the single eligible case — an
// ordinary rowid table with no declared rowid shadow, no aggregate or GROUP
// shape, and no user ORDER BY. A false second return means the snapshot
// cannot be rendered at all, never a partially valid query.
func renderSelectCore(q QueryBuilder, allowRowid bool) ([]string, bool) {
	if q.command != CommandSelect || !q.tableSet {
		return nil, false
	}
	projection := q.renderProjection()
	if projection == "" {
		return nil, false
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
			return nil, false // unreachable by construction; refuse unsafe text
		}
		expr := cand.sqlExpr()
		if expr == "" {
			return nil, false // unresolved aggregate identity: refuse rather than guess
		}
		parts = append(parts, "ORDER BY "+expr+" "+token)
	} else if allowRowid && rowidFallbackEligible(q) {
		parts = append(parts, "ORDER BY rowid")
	}
	return parts, true
}

// rowidFallbackEligible reports whether this snapshot is the single shape the
// Issue #25 ordering policy allows an implicit `ORDER BY rowid` on: a plain
// (non-aggregate, ungrouped) SELECT over an ordinary rowid table with no
// declared rowid, _rowid_, or oid shadow. Every excluded object or query kind
// stays unordered: views, virtual tables, WITHOUT ROWID tables, shadowed
// rowid aliases, aggregate-only and grouped queries, and ties in general have
// no implied stability.
func rowidFallbackEligible(q QueryBuilder) bool {
	if len(q.groups) > 0 || hasAggregateEntry(q) {
		return false
	}
	obj := q.findObject(q.table)
	if obj == nil {
		return false
	}
	return obj.Kind == schema.KindOrdinaryTable && obj.Rowid == schema.RowidHas && !obj.RowidShadowed
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
