// Guided WHERE predicate state (Issue #17): one reusable immutable
// column → operator → optional value flow shared unchanged by the SELECT,
// UPDATE, and DELETE consumers, per the Query Grammar and SQL safety decisions
// in Notes/PRD-sqloid.md.
//
// The predicate holds structurally distinct states rather than booleans:
// absent, column-chosen, awaiting-value, and complete. IS NULL / IS NOT NULL
// become complete immediately at operator selection and never carry a bound
// parameter; every other operator stays incomplete until universal value
// submission supplies exactly one parsed bound value. User-entered text —
// including typed `NULL`, empty input, and LIKE '%'/'_' wildcards — remains
// verbatim on the parameter list and never appears in rendered SQL.
//
// The QueryBuilder owns the guided flow: one committed completed predicate
// plus at most one in-progress draft (seeded from the commitment so revision
// can restore prior choices exactly), cleared together with all other
// downstream state whenever the command or table replaces or vanishes.

package querybuilder

import (
	"github.com/chris/sqloid/internal/schema"
)

// WhereState classifies one structurally distinct WHERE predicate stage.
type WhereState int

const (
	// WhereAbsent is the zero value: nothing entered yet. It is deliberately
	// meaningful so a freshly constructed predicate starts inert.
	WhereAbsent WhereState = iota
	// WhereColumnChosen means a column identity was accepted and an operator
	// selection is next.
	WhereColumnChosen
	// WhereAwaitingValue means a value-taking operator was chosen and the
	// predicate stays incomplete until a value is submitted.
	WhereAwaitingValue
	// WhereComplete means the predicate renders deterministically and binds
	// its parameters (none for the null operators).
	WhereComplete
)

// String renders the human-facing state name used in tests and diagnostics.
func (s WhereState) String() string {
	switch s {
	case WhereAbsent:
		return "absent"
	case WhereColumnChosen:
		return "column-chosen"
	case WhereAwaitingValue:
		return "awaiting-value"
	case WhereComplete:
		return "complete"
	default:
		return "WhereState(out-of-range)"
	}
}

// WherePredicate is one immutable guided predicate snapshot. Values move only
// through transitions, which always return a new value and never mutate their
// receiver; the zero value behaves as WhereAbsent.
type WherePredicate struct {
	state    WhereState    // structural stage; other fields live only in later stages
	col      schema.Column // schema-derived column identity once a column is chosen
	op       Operator      // closed typed operator choice once chosen
	value    Value         // universally parsed submission; only when complete on a value operator
	input    string        // exact entered representation of the last submission
	hasValue bool          // distinguishes a real submission inside the complete state
}

// AbsentWhere returns the initial empty predicate.
func AbsentWhere() WherePredicate { return WherePredicate{} }

// State reports the current structural stage.
func (p WherePredicate) State() WhereState {
	if p.state < WhereAbsent || p.state > WhereComplete {
		return WhereAbsent
	}
	return p.state
}

// Column reports the schema-derived column identity once a column is chosen.
func (p WherePredicate) Column() (schema.Column, bool) {
	if p.State() == WhereAbsent {
		return schema.Column{}, false
	}
	return p.col, true
}

// ChosenOperator reports the typed operator choice once an operator is chosen;
// the boolean is false while only a column has been picked.
func (p WherePredicate) ChosenOperator() (Operator, bool) {
	if p.State() < WhereAwaitingValue {
		return 0, false
	}
	return p.op, true
}

// SubmittedValue reports the universally parsed value of a completed
// value-taking predicate; null-operator completions never report one.
func (p WherePredicate) SubmittedValue() (Value, bool) {
	if !p.hasValue || p.State() != WhereComplete {
		return Value{}, false
	}
	return p.value, true
}

// Entered reports the exact entered text behind a completed value-taking
// predicate; byte-for-byte restoration of revisions depends on it.
func (p WherePredicate) Entered() (string, bool) {
	if !p.hasValue || p.State() != WhereComplete {
		return "", false
	}
	return p.input, true
}

// SelectColumn accepts one schema-derived column identity, starting (or
// restarting) the mid-selection stage and discarding any earlier operator or
// value state. An identity without a name is rejected unchanged so malformed
// data cannot seed a renderable predicate.
func (p WherePredicate) SelectColumn(col schema.Column) WherePredicate {
	if col.Name == "" {
		return p
	}
	next := WherePredicate{state: WhereColumnChosen, col: col}
	return next
}

// ChooseOperator applies one closed typed operator choice. Null operators —
// IS NULL and IS NOT NULL — complete immediately, emitting no placeholder and
// discarding any stale submitted value; every other operator moves to the
// awaiting-value stage and keeps waiting for exactly one submission. Unknown
// operators are rejected unchanged.
func (p WherePredicate) ChooseOperator(op Operator) (WherePredicate, bool) {
	if _, err := op.SQLToken(); err != nil {
		return p, false
	}
	next := WherePredicate{state: WhereColumnChosen, col: p.col}
	if !op.TakesValue() {
		next.state = WhereComplete
		next.op = op
		return next, true
	}
	next.op = op
	next.state = WhereAwaitingValue
	return next, true
}

// SubmitValue parses one user-entered token through the universal parser and,
// for an awaiting value-taking predicate, completes it with that exact parsed
// payload: typed `NULL`, empty input, and LIKE wildcards stay TEXT, preserving
// SQLite wildcard meaning byte-for-byte. Submissions outside the awaiting
// state or onto a no-value operator are ignored.
func (p WherePredicate) SubmitValue(text string) (WherePredicate, bool) {
	if p.State() != WhereAwaitingValue || !p.op.TakesValue() {
		return p, false
	}
	v := ParseValue(text)
	next := p
	next.state = WhereComplete
	next.value = v
	next.input = text // verbatim; restoring revision needs the original representation
	next.hasValue = true
	return next, true
}

// SQL renders the exact predicate fragment: a safely quoted column atom plus
// the fixed operator token, plus '?' wherever a bound value belongs. Anything
// short of the complete stage renders empty. Entered values are never
// interpolated into this text.
func (p WherePredicate) SQL() string {
	if p.State() != WhereComplete {
		return ""
	}
	token, err := p.op.SQLToken()
	if err != nil {
		// Completed predicates can only exist with operator tokens validated
		// at construction; an unrenderable completion stays silent instead of
		// ever emitting caller-controlled text.
		return ""
	}
	quoted := quoteIdentifierAtom(p.col.Name)
	if !p.op.TakesValue() {
		return quoted + " " + token
	}
	return quoted + " " + token + " ?"
}

// Params returns this predicate's bound parameters in deterministic order:
// none for IS NULL / IS NOT NULL or any incomplete state, otherwise exactly
// the single parsed submission at its concrete driver-facing type.
func (p WherePredicate) Params() []any {
	if p.State() != WhereComplete || !p.op.TakesValue() {
		return nil
	}
	return []any{p.value.ParamValue()}
}

// fixedOperators lists every v1 WHERE operator in presentation order,
// offered identically for every eligible column with no declared-type
// filtering.
var fixedOperators = []Operator{OpEq, OpNotEq, OpLt, OpLe, OpGt, OpGe, OpIsNull, OpIsNotNull, OpLike}

// whereReady reports whether the current command owns the optional WHERE
// predicate and a table is selected: exactly SELECT, UPDATE, and DELETE with
// a surviving table selection.
func (q QueryBuilder) whereReady() bool {
	switch q.command {
	case CommandSelect, CommandUpdate, CommandDelete:
	default:
		return false
	}
	return q.tableSet
}

// WhereReady reports whether a WHERE draft may begin in the current state.
func (q QueryBuilder) WhereReady() bool { return q.whereReady() }

// WhereCandidates lists the eligible column identities offered to the guided
// flow: every visible (non-hidden) column of the selected object in declared
// order, with no declared-type filtering. Empty when WHERE is unavailable.
// The returned slice is fresh; callers may mutate it freely.
func (q QueryBuilder) WhereCandidates() []schema.Column {
	obj := q.findObject(q.table)
	if !q.whereReady() || obj == nil {
		return nil
	}
	out := make([]schema.Column, 0, len(obj.Columns))
	for _, col := range obj.Columns {
		if !col.Hidden {
			out = append(out, col)
		}
	}
	return out
}

// FixedOperators returns the closed fixed operator set in deterministic
// presentation order. The returned slice is fresh; callers may mutate it
// freely.
func (q QueryBuilder) FixedOperators() []Operator {
	return append([]Operator(nil), fixedOperators...)
}

// HasWhere reports whether a completed WHERE predicate is committed to the
// builder.
func (q QueryBuilder) HasWhere() bool { return q.whereSet }

// WherePredicate returns the committed WHERE predicate, or AbsentWhere when
// none exists; drafts are visible only through WhereDraft.
func (q QueryBuilder) WherePredicate() WherePredicate {
	if !q.whereSet {
		return AbsentWhere()
	}
	return q.where
}

// WhereParams returns the committed predicate's bound parameters at this
// consumer boundary, or nil when no complete predicate exists. The slice is
// fresh on each call.
func (q QueryBuilder) WhereParams() []any {
	p := q.WherePredicate()
	params := p.Params()
	if params == nil {
		return nil
	}
	return append([]any(nil), params...)
}

// WhereDrafting reports whether a guided WHERE draft is in progress.
func (q QueryBuilder) WhereDrafting() bool { return q.whereDrafting }

// WhereDraft returns the in-progress draft, seeded from any prior commitment
// so revision restores its exact column, operator, entered text, and bound
// type. Without a draft it is AbsentWhere.
func (q QueryBuilder) WhereDraft() WherePredicate {
	if !q.whereDrafting {
		return AbsentWhere()
	}
	return q.whereDraft
}

// StartWhere accepts one eligible column name, opening the guided draft over
// the committed state so revisions restore prior choices exactly. It requires
// WHERE availability (SELECT, UPDATE, or DELETE with a selected table), no
// other open draft, and a name among the current visible columns; anything
// else leaves the builder unchanged.
func (q QueryBuilder) StartWhere(column string) (QueryBuilder, bool) {
	if !q.whereReady() || q.whereDrafting {
		return q, false
	}
	var chosen schema.Column
	found := false
	for _, col := range q.WhereCandidates() {
		if col.Name == column {
			chosen, found = col, true
			break
		}
	}
	if !found {
		return q, false
	}
	next := q
	next.whereDrafting = true
	if q.whereSet && q.where.col == chosen {
		// Revising the same column restores the prior operator, exact entered
		// text, and bound type wholesale.
		next.whereDraft = q.where
	} else {
		next.whereDraft = AbsentWhere().SelectColumn(chosen)
	}
	return next, true
}

// ApplyWhereDraft installs one transitioned draft snapshot, never mutating
// prior snapshots, while the guided flow stays open. Without an open draft it
// leaves the builder unchanged.
func (q QueryBuilder) ApplyWhereDraft(next WherePredicate) QueryBuilder {
	if !q.whereDrafting {
		return q
	}
	after := q
	after.whereDraft = next
	return after
}

// CancelWhereDraft discards the open draft — including any partial commits —
// leaving a previously completed predicate untouched for exact restoration.
// With no draft open it is an unchanged no-op.
func (q QueryBuilder) CancelWhereDraft() QueryBuilder {
	if !q.whereDrafting {
		return q
	}
	next := q
	next.whereDrafting = false
	next.whereDraft = WherePredicate{}
	return next
}

// CommitWhereDraft promotes the draft into the committed single-predicate
// state once it is structurally complete and still names a currently eligible
// column of the selected object; incomplete drafts commit nothing and keep
// the flow open. Committing also closes the draft.
func (q QueryBuilder) CommitWhereDraft() (QueryBuilder, bool) {
	draft := q.WhereDraft()
	if !q.whereDrafting || draft.State() != WhereComplete {
		return q, false
	}
	name, hasCol := draft.Column()
	if !hasCol {
		return q, false
	}
	eligible := false
	for _, col := range q.WhereCandidates() {
		if col.Name == name.Name && col == name {
			eligible = true
			break
		}
	}
	if !eligible {
		return q, false
	}
	next := q
	next.where = draft
	next.whereSet = true
	next.whereDrafting = false
	next.whereDraft = WherePredicate{}
	return next, true
}

// discardWhere drops both the committed predicate and any open draft together,
// used whenever downstream command-specific state is cleared wholesale.
func (q *QueryBuilder) discardWhere() {
	q.where = WherePredicate{}
	q.whereSet = false
	q.whereDrafting = false
	q.whereDraft = WherePredicate{}
}
