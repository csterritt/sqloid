// History-ready execution state (Issue #20): the canonical normalized
// representation of one QueryBuilder execution snapshot, together with its
// normalized equality, consumed by the internal/history store and append
// policy. The state preserves every field the PRD's History decision declares
// significant for consecutive comparison — command, stable table identity,
// ordered projection entries, WHERE presence/column/operator/entered
// value/parsed bound type, GROUP BY order, ORDER BY expression/direction,
// Limit empty-versus-accepted-number, and ordered UPDATE assignments and
// INSERT choices with their exact submitted values and entered
// representations — and deliberately omits every transient UI-owned state:
// focus, popup/input cursors, inline errors, layout, and request identities.
//
// Entered representation, parsed bound type, structural choice, column order,
// projection order, and group order are significant even when rendered SQL or
// bound database values could match, because history restoration must
// reproduce the user's own bytes and order exactly (Issues #20 and #35).

package querybuilder

// HistoryProjectionEntry is one normalized projection item in commit order.
type HistoryProjectionEntry struct {
	Kind      ProjectionKind
	Column    string    // declared column name when Kind == ProjectionColumn
	Aggregate Aggregate // AggregateValue for plain columns; zero otherwise
}

// HistorySetAssignment is one normalized UPDATE SET assignment in SET order.
// HasValue distinguishes a real submission (with its exact entered
// representation and parsed bound payload) from an unsubmitted Value choice
// and from the SQL-NULL choice, which never carries a parameter.
type HistorySetAssignment struct {
	Column   string
	Choice   SetChoice
	HasValue bool
	Value    Value  // parsed submission when HasValue
	Entered  string // exact entered text when HasValue
}

// HistoryInsertColumn is one normalized INSERT per-column prompt state in
// declared order, mirroring HistorySetAssignment's submission rules.
type HistoryInsertColumn struct {
	Column   string
	Choice   InsertChoice
	HasValue bool
	Value    Value  // parsed submission when HasValue
	Entered  string // exact entered text when HasValue
}

// HistoryState is one immutable, history-ready execution snapshot. Every
// slice it carries is freshly allocated by HistoryState, so callers may hold
// or mutate their copy without ever touching builder state; the History
// package additionally deep-copies on retention per its own ownership rules.
// The zero value is a valid empty unselected snapshot.
type HistoryState struct {
	Command  Command
	Table    string // selected object name exactly as cataloged
	TableSet bool

	Projection []HistoryProjectionEntry // committed entries in insertion order

	WhereSet      bool
	WhereColumn   string   // committed predicate's column name when WhereSet
	WhereOperator Operator // committed operator when WhereSet (zero only defensively)

	WhereHasValue bool   // a submitted value exists on the completed predicate
	WhereValue    Value  // parsed submission when WhereHasValue
	WhereEntered  string // exact entered representation when WhereHasValue

	Groups []string // committed GROUP BY names in acceptance order

	OrderSet        bool
	OrderExpression string // committed candidate Key identity
	OrderDirection  Direction

	LimitHas   bool  // an integer was accepted (false = empty/unbounded)
	LimitValue int64 // accepted integer when LimitHas

	Sets    []HistorySetAssignment // committed UPDATE assignments in SET order
	Inserts []HistoryInsertColumn  // INSERT prompt states in declared order
}

// HistoryState returns the canonical normalized execution snapshot of this
// builder: only fields significant for query-history comparison and
// restoration, with every slice freshly allocated. Draft WHERE state, focus,
// catalogs, and all other transient UI-owned state are excluded; a completed
// committed predicate contributes its exact column, operator, parsed value,
// bound type, and entered representation. This never mutates the receiver.
func (q QueryBuilder) HistoryState() HistoryState {
	state := HistoryState{
		Command:  q.command,
		Table:    q.table,
		TableSet: q.tableSet,
	}
	state.Projection = make([]HistoryProjectionEntry, len(q.projection))
	for i, e := range q.projection {
		state.Projection[i] = HistoryProjectionEntry{Kind: e.Kind, Column: e.Column, Aggregate: e.Aggregate}
	}
	if q.whereSet {
		state.WhereSet = true
		state.WhereColumn = q.where.col.Name
		state.WhereOperator = q.where.op
		if v, ok := q.where.SubmittedValue(); ok {
			state.WhereHasValue = true
			state.WhereValue = v
			state.WhereEntered, _ = q.where.Entered()
		}
	}
	state.Groups = append([]string(nil), q.groups...)
	if q.orderSet {
		state.OrderSet = true
		state.OrderExpression = q.orderKey
		state.OrderDirection = q.orderDir
	}
	if q.limitHas {
		state.LimitHas = true
		state.LimitValue = q.limitVal
	}
	state.Sets = make([]HistorySetAssignment, len(q.sets))
	for i, a := range q.sets {
		entry := HistorySetAssignment{Column: a.Column, Choice: a.choice}
		if v, ok := a.SubmittedValue(); ok {
			entry.HasValue = true
			entry.Value = v
			entry.Entered, _ = a.Entered()
		}
		state.Sets[i] = entry
	}
	state.Inserts = make([]HistoryInsertColumn, len(q.inserts))
	for i, c := range q.inserts {
		entry := HistoryInsertColumn{Column: c.Column, Choice: c.choice}
		if v, ok := c.SubmittedValue(); ok {
			entry.HasValue = true
			entry.Value = v
			entry.Entered, _ = c.Entered()
		}
		state.Inserts[i] = entry
	}
	return state
}

// Equal reports whether two history states are normalized-equal: every
// significant field and every slice element (in order) matches exactly,
// including entered representations, parsed kinds, structural choices, and
// ordering. Rendered SQL equality is irrelevant: two states are equal only
// when their significant representations are.
func (s HistoryState) Equal(other HistoryState) bool {
	if s.Command != other.Command || s.Table != other.Table || s.TableSet != other.TableSet {
		return false
	}
	if len(s.Projection) != len(other.Projection) {
		return false
	}
	for i := range s.Projection {
		if s.Projection[i] != other.Projection[i] {
			return false
		}
	}
	if s.WhereSet != other.WhereSet || s.WhereColumn != other.WhereColumn || s.WhereOperator != other.WhereOperator {
		return false
	}
	if s.WhereHasValue != other.WhereHasValue || s.WhereValue != other.WhereValue || s.WhereEntered != other.WhereEntered {
		return false
	}
	if len(s.Groups) != len(other.Groups) {
		return false
	}
	for i := range s.Groups {
		if s.Groups[i] != other.Groups[i] {
			return false
		}
	}
	if s.OrderSet != other.OrderSet || s.OrderExpression != other.OrderExpression || s.OrderDirection != other.OrderDirection {
		return false
	}
	// Limit compares only the empty-versus-accepted-number distinction:
	// entered representations such as "5" and "05" are transient entry detail
	// once the same integer is accepted, per the History decision.
	if s.LimitHas != other.LimitHas || s.LimitValue != other.LimitValue {
		return false
	}
	if len(s.Sets) != len(other.Sets) {
		return false
	}
	for i := range s.Sets {
		if s.Sets[i] != other.Sets[i] {
			return false
		}
	}
	if len(s.Inserts) != len(other.Inserts) {
		return false
	}
	for i := range s.Inserts {
		if s.Inserts[i] != other.Inserts[i] {
			return false
		}
	}
	return true
}
