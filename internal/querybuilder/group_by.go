// SELECT GROUP BY state and validation (Issue #18): assisted multi-selection
// of visible table columns in stable commit order, duplicate and stale-identity
// rejection, the complete grouping validity matrix, and deterministic quoted
// SQL generation. The selection stays UI-independent; popup behavior lives in
// internal/ui, per the Builder lifecycle decision in Notes/PRD-sqloid.md.

package querybuilder

// Exact user-facing invalid reasons reported through InvalidIssue.Reason for
// grouping problems. Tests assert these wordings verbatim.
const (
	// MixedAggregationNeedsGroupReason reports a mixed aggregate/nonaggregate
	// projection whose nonaggregate side is not fully grouped, with or without
	// an existing GROUP BY leaving one out.
	MixedAggregationNeedsGroupReason = "every nonaggregate projected column must be grouped"
	// WildcardGroupedByReason reports any GROUP BY beside a wildcard projection.
	WildcardGroupedByReason = "the wildcard cannot be used together with GROUP BY"
	// StaleGroupColumnReason reports a committed group naming a column that no
	// longer exists among the selected object's visible columns after a refresh.
	StaleGroupColumnReason = "a grouped column no longer exists"
	// StaleOrderByExpressionReason reports a committed ORDER BY whose expression
	// is no longer offered among the current context candidates.
	StaleOrderByExpressionReason = "the ordered expression is no longer offered"
)

// FirstInvalidIssue returns the first blocking problem found in this snapshot
// for the runnable-state contract: grouping rules first, then ORDER BY and
// LIMIT as later milestones add them. ok is false when every checked rule is
// satisfied; later milestones extend the covered set in fixed order.
func (q QueryBuilder) FirstInvalidIssue() (InvalidIssue, bool) {
	if issue, invalid := validateGrouping(q); invalid {
		return issue, true
	}
	if issue, invalid := validateOrderBy(q); invalid {
		return issue, true
	}
	if issue, invalid := validateLimit(q); invalid {
		return issue, true
	}
	return InvalidIssue{}, false
}

// validateGrouping applies the PRD grouping matrix to the current snapshot:
// wildcard and GROUP BY never coexist; a nonempty GROUP BY requires every
// nonaggregate projected column grouped while extra groups stay permitted; a
// mixed aggregate/nonaggregate projection without GROUP BY is invalid; an
// all-aggregate projection or bare COUNT(*) stays valid either way; and every
// committed group must still name a currently visible column.
func validateGrouping(q QueryBuilder) (InvalidIssue, bool) {
	groups := q.groups
	plain := false // some named nonaggregate column is projected
	for _, e := range q.projection {
		switch {
		case e.Kind == ProjectionWildcard:
			if len(groups) > 0 {
				return InvalidIssue{FieldIdentityGroupBy, WildcardGroupedByReason}, true
			}
		case e.Kind == ProjectionColumn && e.Aggregate == AggregateValue:
			plain = true
		}
	}
	if len(groups) > 0 {
		if plain {
			for _, e := range q.projection {
				if e.Kind != ProjectionColumn || e.Aggregate != AggregateValue {
					continue
				}
				found := false
				for _, g := range groups {
					if g == e.Column {
						found = true
						break
					}
				}
				if !found {
					return InvalidIssue{FieldIdentityGroupBy, MixedAggregationNeedsGroupReason}, true
				}
			}
		}
		for _, g := range groups {
			stale := true
			for _, col := range q.selectedColumns() {
				if col.Name == g {
					stale = false
					break
				}
			}
			if stale {
				return InvalidIssue{FieldIdentityGroupBy, StaleGroupColumnReason}, true
			}
		}
	} else if plain && hasAggregateEntry(q) {
		return InvalidIssue{FieldIdentityGroupBy, MixedAggregationNeedsGroupReason}, true
	}
	return InvalidIssue{}, false
}

// hasAggregateEntry reports whether the projection contains any aggregated
// named column or the synthetic bare COUNT(*) sentinel.
func hasAggregateEntry(q QueryBuilder) bool {
	for _, e := range q.projection {
		if e.Kind == ProjectionCountStar || (e.Kind == ProjectionColumn && e.Aggregate > AggregateValue) {
			return true
		}
	}
	return false
}

// GroupByCandidates lists the still-uncommitted GROUP BY choices derived from
// the currently eligible table's visible columns in Schema order. Already
// committed columns are excluded so the assisted multi-selection can never
// offer a duplicate. The returned slice is fresh; callers may mutate it freely.
func (q QueryBuilder) GroupByCandidates() []string {
	if !q.projectionReady() {
		return nil
	}
	committed := make(map[string]bool, len(q.groups))
	for _, g := range q.groups {
		committed[g] = true
	}
	var out []string
	for _, col := range q.selectedColumns() {
		if !committed[col.Name] {
			out = append(out, col.Name)
		}
	}
	return out
}

// AcceptGroupColumn appends one visible column name to the committed GROUP BY
// list, preserving insertion order exactly as accepted — deliberately not
// Schema order — because the user's own selection order is significant for
// rendering and history comparison. An unknown, hidden, empty, or exactly
// duplicated identity is rejected as an immutable no-op carrying false, so a
// stale or foreign identity can never become SQL.
func (q QueryBuilder) AcceptGroupColumn(name string) (QueryBuilder, bool) {
	if !q.projectionReady() || name == "" {
		return q, false
	}
	visible := false
	for _, col := range q.selectedColumns() {
		if col.Name == name {
			visible = true
			break
		}
	}
	if !visible {
		return q, false
	}
	for _, g := range q.groups {
		if g == name {
			return q, false // exact duplicate: immutable no-op
		}
	}
	next := q
	next.groups = append(append([]string(nil), q.groups...), name)
	return next, true
}

// RemoveLatestGroup deletes the most recently accepted GROUP BY column; an
// empty selection is unchanged. This backs the base Group By field's removal
// seam, mirroring RemoveLatestProjection for the Column(s) field.
func (q QueryBuilder) RemoveLatestGroup() QueryBuilder {
	if len(q.groups) == 0 {
		return q
	}
	next := q
	next.groups = make([]string, 0, len(q.groups)-1)
	next.groups = append(next.groups, q.groups[:len(q.groups)-1]...)
	return next
}

// GroupByEntries returns the committed group column names in selection order
// as a fresh slice; callers may mutate it freely.
func (q QueryBuilder) GroupByEntries() []string {
	out := make([]string, len(q.groups))
	copy(out, q.groups)
	return out
}

// renderProjection renders the SELECT list per SnapshotRendering semantics:
// a sole wildcard renders as *, an empty projection expands to every visible
// column in Schema order, the bare sentinel renders COUNT(*), aggregated named
// entries render TOKEN("column"), and plain named entries render the quoted
// column atom.
func (q QueryBuilder) renderProjection() string {
	if len(q.projection) == 1 && q.projection[0].Kind == ProjectionWildcard {
		return "*"
	}
	entries := q.ProjectionEntries()
	if len(entries) == 0 {
		cols := q.selectedColumns()
		entries = make([]ProjectionEntry, 0, len(cols))
		for _, c := range cols {
			entries = append(entries, ProjectionEntry{Kind: ProjectionColumn, Column: c.Name})
		}
		if len(entries) == 0 {
			return ""
		}
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		switch e.Kind {
		case ProjectionCountStar:
			parts = append(parts, "COUNT(*)")
		default:
			atom := quoteIdentifierAtom(e.Column)
			if e.Aggregate > AggregateValue {
				token, err := e.Aggregate.SQLToken()
				if err != nil {
					continue // unreachable by construction; skip rather than emit text
				}
				parts = append(parts, token+"("+atom+")")
				continue
			}
			parts = append(parts, atom)
		}
	}
	return joinSQLList(parts)
}

// joinSQLList comma-joins rendered SQL atoms with single spaces.
func joinSQLList(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
