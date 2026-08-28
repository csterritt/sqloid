// SELECT ORDER BY state (Issue #18): one context-derived candidate identity,
// immutable single-selection with atomic replacement, and a closed ASC/DESC
// direction that defaults to ASC on every fresh selection. Candidates come
// only from committed projection and GROUP BY state; popup behavior stays in
// internal/ui per the Builder lifecycle decision.

package querybuilder

// orderExprKind classifies one ORDER BY expression identity.
type orderExprKind int

const (
	orderExprColumn    orderExprKind = iota + 1 // plain grouped/table column
	orderExprAggregate                          // aggregated projected column
	orderExprCountStar                          // the synthetic bare COUNT(*) sentinel
)

// OrderByCandidate is one offered ORDER BY expression: a stable Key separate
// from its Display label so equal labels (and duplicate-looking expressions
// such as a column beside its own aggregate) keep distinct identities. Keys
// use reserved prefixes mirroring ProjectionCandidate.Key so a real column
// named like another candidate can never collide.
type OrderByCandidate struct {
	Key     string
	Display string

	kind orderExprKind
	col  string    // declared column name for column/aggregate identities
	agg  Aggregate // aggregate token source for aggregate identities
}

// SQL renders this candidate's exact expression atom: quoted identifiers and
// fixed aggregate tokens only, never display text or arbitrary user input.
func (c OrderByCandidate) sqlExpr() string {
	switch c.kind {
	case orderExprAggregate:
		token, err := c.agg.SQLToken()
		if err != nil {
			return "" // unreachable by construction; empty rather than unsafe
		}
		return token + "(" + quoteIdentifierAtom(c.col) + ")"
	case orderExprCountStar:
		return "COUNT(*)"
	default:
		return quoteIdentifierAtom(c.col)
	}
}

// OrderByCandidates derives the deterministic ORDER BY choices for the current
// snapshot. Ordinary ungrouped SELECTs without aggregates offer every visible
// table column in Schema order. Aggregate or grouped queries offer exactly the
// committed GROUP BY columns (commit order), then selected aggregate
// expressions (projection order), then bare COUNT(*) when selected — never an
// unselected aggregate, an ungrouped nonaggregate column, the wildcard, or
// arbitrary text. The returned slice is fresh; callers may mutate it freely.
func (q QueryBuilder) OrderByCandidates() []OrderByCandidate {
	if !q.projectionReady() {
		return nil
	}
	if len(q.groups) == 0 && !hasAggregateEntry(q) {
		var out []OrderByCandidate
		for _, col := range q.selectedColumns() {
			out = append(out, OrderByCandidate{
				Key:     orderByKey(orderExprColumn, col.Name, AggregateValue),
				Display: col.Name,
				kind:    orderExprColumn,
				col:     col.Name,
			})
		}
		return out
	}
	candidates := make([]OrderByCandidate, 0, len(q.groups)+len(q.projection))
	for _, g := range q.groups {
		candidates = append(candidates, OrderByCandidate{
			Key:     orderByKey(orderExprColumn, g, AggregateValue),
			Display: g,
			kind:    orderExprColumn,
			col:     g,
		})
	}
	for _, e := range q.projection {
		if e.Kind != ProjectionColumn || e.Aggregate <= AggregateValue {
			continue
		}
		if token, err := e.Aggregate.SQLToken(); err == nil {
			candidates = append(candidates, OrderByCandidate{
				Key:     orderByKey(orderExprAggregate, e.Column, e.Aggregate),
				Display: token + "(" + e.Column + ")",
				kind:    orderExprAggregate,
				col:     e.Column,
				agg:     e.Aggregate,
			})
		}
	}
	for _, e := range q.projection {
		if e.Kind == ProjectionCountStar {
			candidates = append(candidates, OrderByCandidate{
				Key:     orderByKey(orderExprCountStar, "", AggregateValue),
				Display: "COUNT(*)",
				kind:    orderExprCountStar,
			})
		}
	}
	return dedupeCandidates(candidates)
}

// dedupeCandidates drops repeated keys defensively while preserving first-seen
// order; by construction duplicates cannot occur, but grouped state and
// projection state both feed this list across milestones.
func dedupeCandidates(in []OrderByCandidate) []OrderByCandidate {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, c := range in {
		if seen[c.Key] {
			continue
		}
		seen[c.Key] = true
		out = append(out, c)
	}
	return out
}

// orderByKey encodes one ordered-expression identity: role prefix plus column
// name plus aggregate token when aggregated.
func orderByKey(kind orderExprKind, column string, agg Aggregate) string {
	switch kind {
	case orderExprAggregate:
		token := ""
		if tok, err := agg.SQLToken(); err == nil {
			token = tok
		}
		return "order-aggregate:" + column + ":" + token
	case orderExprCountStar:
		return "order-count-star"
	default:
		return "order-column:" + column
	}
}

// AcceptOrderBy commits key as the sole ORDER BY expression, replacing any
// existing selection atomically and resetting direction to the ASC default.
// A key outside the current candidates — including stale Schema identities,
// aggregates absent from the projection, the wildcard, or arbitrary text — is
// rejected unchanged and reports false.
func (q QueryBuilder) AcceptOrderBy(key string) (QueryBuilder, bool) {
	for _, c := range q.OrderByCandidates() {
		if c.Key == key {
			next := q
			next.orderKey, next.orderDir, next.orderSet = key, DirAsc, true
			return next, true
		}
	}
	return q, false
}

// ClearOrderBy removes the committed selection entirely; a whole-value seam
// used by base-field removal and downstream clearing.
func (q QueryBuilder) ClearOrderBy() QueryBuilder {
	next := q
	next.orderKey, next.orderDir, next.orderSet = "", 0, false
	return next
}

// SetOrderDirection installs dir over the committed selection when dir is a
// valid closed choice; any other value leaves the builder unchanged.
func (q QueryBuilder) SetOrderDirection(dir Direction) QueryBuilder {
	if _, err := dir.SQLToken(); err != nil || !q.orderSet {
		return q
	}
	next := q
	next.orderDir = dir
	return next
}

// ToggleOrderDirection flips a committed selection between the two closed
// directions; with nothing committed it is an unchanged no-op. This backs the
// base-field Up/Down toggle owned by the UI layer.
func (q QueryBuilder) ToggleOrderDirection() QueryBuilder {
	switch q.orderDir {
	case DirAsc:
		return q.SetOrderDirection(DirDesc)
	case DirDesc:
		return q.SetOrderDirection(DirAsc)
	default:
		return q
	}
}

// OrderBySelection reports the committed expression as its typed candidate
// resolved against the current candidates, plus the effective direction. The
// boolean is false when nothing is committed or the stored identity is stale
// relative to the current grouping/projection context.
func (q QueryBuilder) OrderBySelection() (OrderByCandidate, Direction, bool) {
	if !q.orderSet {
		return OrderByCandidate{}, 0, false
	}
	for _, c := range q.OrderByCandidates() {
		if c.Key == q.orderKey {
			dir := q.orderDir
			if _, err := dir.SQLToken(); err != nil {
				dir = DirAsc // defensive default; never persisted invalid
			}
			return c, dir, true
		}
	}
	return OrderByCandidate{}, 0, false
}

// validateOrderBy flags a committed ORDER BY whose expression is no longer
// offered among the context candidates — for example after a refresh removed
// the grouped column — so stale ordering can never run or render.
func validateOrderBy(q QueryBuilder) (InvalidIssue, bool) {
	if !q.orderSet {
		return InvalidIssue{}, false
	}
	if _, dir, ok := q.OrderBySelection(); ok {
		if _, err := dir.SQLToken(); err == nil {
			return InvalidIssue{}, false
		}
	}
	return InvalidIssue{FieldIdentityOrderBy, StaleOrderByExpressionReason}, true
}
