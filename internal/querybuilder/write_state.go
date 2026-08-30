// Typed immutable write state: Issue #37's complete UPDATE SET assignments and
// Issue #19's forward-compatible INSERT per-column choices used by the
// authoritative runnable report until the INSERT flow lands in Issue #39.
//
// Choices are structural, never booleans: UPDATE offers exactly
// {Value, NULL} per SET column (Default/Omit is never offered), and INSERT
// offers {Value, NULL, Default/Omit} per insertable column, where an
// all-omit state is valid and later emits DEFAULT VALUES. A Value choice is
// complete only after one universal submission; entered empty text counts as
// submitted empty TEXT, and a typed `NULL` submission stays TEXT — structurally
// distinct from the SQL-NULL choice, which binds no parameter.

package querybuilder

import "github.com/chris/sqloid/internal/schema"

// SetChoice classifies one UPDATE SET assignment choice.
type SetChoice int

const (
	// SetChoiceNone is the incomplete state before any Value/NULL choice.
	SetChoiceNone SetChoice = iota
	// SetChoiceValue binds one universally parsed submitted value.
	SetChoiceValue
	// SetChoiceNull emits the SQL keyword NULL and binds no parameter.
	SetChoiceNull
)

// String renders the human-facing choice name used in tests and diagnostics.
func (c SetChoice) String() string {
	switch c {
	case SetChoiceValue:
		return "Value"
	case SetChoiceNull:
		return "NULL"
	default:
		return "none"
	}
}

// SetAssignment is one UPDATE SET column together with its typed choice
// state. The zero value is the incomplete state; construct through the
// QueryBuilder transitions below.
type SetAssignment struct {
	Column string // declared SET column name exactly as accepted

	choice    SetChoice
	value     Value  // parsed submission when choice == SetChoiceValue and submitted
	input     string // exact entered representation of the last submission
	submitted bool   // distinguishes a real submission from pending entry
}

// Choice reports the assignment's typed choice.
func (a SetAssignment) Choice() SetChoice { return a.choice }

// SubmittedValue reports the parsed submission for a submitted Value choice;
// NULL choices and unsubmitted entries never report one.
func (a SetAssignment) SubmittedValue() (Value, bool) {
	if a.choice != SetChoiceValue || !a.submitted {
		return Value{}, false
	}
	return a.value, true
}

// Entered reports the exact entered text behind a submitted Value choice.
func (a SetAssignment) Entered() (string, bool) {
	if a.choice != SetChoiceValue || !a.submitted {
		return "", false
	}
	return a.input, true
}

// SetCandidates returns the refreshed visible columns eligible for UPDATE SET
// assignment, in schema order. The returned slice is fresh.
func (q QueryBuilder) SetCandidates() []schema.Column {
	if q.command != CommandUpdate || !q.tableSet {
		return nil
	}
	return q.selectedColumns()
}

// SetAssignments returns the committed SET assignments in selection order as
// a fresh slice; callers may mutate it freely.
func (q QueryBuilder) SetAssignments() []SetAssignment {
	out := make([]SetAssignment, len(q.sets))
	copy(out, q.sets)
	return out
}

// WithSetAssignments installs an arbitrary assignment slice as the committed
// SET state, without eligibility filtering. The runnable and restoration tests
// use this defensive seam to represent malformed states — including duplicate
// SET columns — that the guided flow itself never constructs.
func (q QueryBuilder) WithSetAssignments(as []SetAssignment) QueryBuilder {
	next := q
	next.sets = append([]SetAssignment(nil), as...)
	return next
}

// AcceptSetColumn appends one visible column of the selected table as a new
// incomplete SET assignment. It requires UPDATE with a selected table and a
// name among the current visible columns; anything else is rejected.
func (q QueryBuilder) AcceptSetColumn(name string) (QueryBuilder, bool) {
	if q.command != CommandUpdate || !q.tableSet || name == "" {
		return q, false
	}
	for _, assignment := range q.sets {
		if assignment.Column == name {
			return q, false
		}
	}
	for _, col := range q.SetCandidates() {
		if col.Name == name {
			next := q
			next.sets = append(append([]SetAssignment(nil), q.sets...), SetAssignment{Column: name})
			return next, true
		}
	}
	return q, false
}

// ChooseSetAssignment installs choice on every assignment naming column,
// discarding any earlier choice or submission state. An unknown choice value
// or a column without a matching assignment is rejected unchanged.
func (q QueryBuilder) ChooseSetAssignment(column string, choice SetChoice) (QueryBuilder, bool) {
	if choice != SetChoiceValue && choice != SetChoiceNull {
		return q, false
	}
	found := false
	next := q
	next.sets = append([]SetAssignment(nil), q.sets...)
	for i := range next.sets {
		if next.sets[i].Column == column {
			next.sets[i].choice = choice
			next.sets[i].value, next.sets[i].input, next.sets[i].submitted = Value{}, "", false
			found = true
		}
	}
	if !found {
		return q, false
	}
	return next, true
}

// SubmitSetValue records one universal submission for the first unsubmitted
// Value-choice assignment naming column. Empty text completes as empty TEXT;
// a typed `NULL` stays TEXT, distinct from the SQL-NULL choice. Submissions
// onto NULL choices or already-submitted entries are ignored.
func (q QueryBuilder) SubmitSetValue(column, text string) (QueryBuilder, bool) {
	next := q
	next.sets = append([]SetAssignment(nil), q.sets...)
	for i := range next.sets {
		a := &next.sets[i]
		if a.Column == column && a.choice == SetChoiceValue && !a.submitted {
			a.value = ParseValue(text)
			a.input = text
			a.submitted = true
			return next, true
		}
	}
	return q, false
}

// InsertChoice classifies one INSERT per-column prompt choice.
type InsertChoice int

const (
	// InsertChoiceNone is the incomplete state before any choice.
	InsertChoiceNone InsertChoice = iota
	// InsertChoiceValue binds one universally parsed submitted value.
	InsertChoiceValue
	// InsertChoiceNull binds an explicit SQL NULL.
	InsertChoiceNull
	// InsertChoiceOmit excludes the column from the statement; an all-omit
	// state is valid and later emits DEFAULT VALUES.
	InsertChoiceOmit
)

// String renders the human-facing choice name used in tests and diagnostics.
func (c InsertChoice) String() string {
	switch c {
	case InsertChoiceValue:
		return "Value"
	case InsertChoiceNull:
		return "NULL"
	case InsertChoiceOmit:
		return "Default/Omit"
	default:
		return "none"
	}
}

// InsertColumn is one INSERT prompt column with its typed choice state.
type InsertColumn struct {
	Column string // declared insertable column name

	choice    InsertChoice
	value     Value  // parsed submission when choice == InsertChoiceValue and submitted
	input     string // exact entered representation of the last submission
	submitted bool   // distinguishes a real submission from pending entry
}

// Choice reports the column's typed choice.
func (c InsertColumn) Choice() InsertChoice { return c.choice }

// Entered reports the exact entered text behind a submitted Value choice.
func (c InsertColumn) Entered() (string, bool) {
	if c.choice != InsertChoiceValue || !c.submitted {
		return "", false
	}
	return c.input, true
}

// SubmittedValue reports the parsed submission for a submitted Value choice;
// NULL, Default/Omit, and unsubmitted entries never report one.
func (c InsertColumn) SubmittedValue() (Value, bool) {
	if c.choice != InsertChoiceValue || !c.submitted {
		return Value{}, false
	}
	return c.value, true
}

// InsertableColumns lists the insertable columns of the selected object in
// declared order: visible (non-hidden) columns of a write-eligible object.
// The returned slice is fresh; callers may mutate it freely.
func (q QueryBuilder) InsertableColumns() []schema.Column {
	obj := q.findObject(q.table)
	if q.command != CommandInsert || !q.tableSet || obj == nil {
		return nil
	}
	out := make([]schema.Column, 0, len(obj.Columns))
	for _, col := range obj.Columns {
		if col.Insertable {
			out = append(out, col)
		}
	}
	return out
}

// InsertColumns returns the per-column prompt states in declared order as a
// fresh slice; an empty result means prompts have not been begun.
func (q QueryBuilder) InsertColumns() []InsertColumn {
	out := make([]InsertColumn, len(q.inserts))
	copy(out, q.inserts)
	return out
}

// BeginInsertPrompts seeds one incomplete prompt per insertable column of the
// selected table in declared order. Repeated calls are no-ops, and a table
// with zero insertable columns never yields prompts — the runnable report
// blocks that state instead.
func (q QueryBuilder) BeginInsertPrompts() QueryBuilder {
	if q.command != CommandInsert || !q.tableSet || len(q.inserts) > 0 {
		return q
	}
	next := q
	for _, col := range q.InsertableColumns() {
		next.inserts = append(next.inserts, InsertColumn{Column: col.Name})
	}
	return next
}

// ChooseInsertColumn installs choice on the prompt naming column, discarding
// any earlier choice or submission state. An unknown choice value or a column
// without a prompt is rejected unchanged.
func (q QueryBuilder) ChooseInsertColumn(column string, choice InsertChoice) (QueryBuilder, bool) {
	if choice < InsertChoiceValue || choice > InsertChoiceOmit {
		return q, false
	}
	found := false
	next := q
	for i := range next.inserts {
		if next.inserts[i].Column == column {
			next.inserts[i].choice = choice
			next.inserts[i].value, next.inserts[i].input, next.inserts[i].submitted = Value{}, "", false
			found = true
		}
	}
	if !found {
		return q, false
	}
	return next, true
}

// SubmitInsertValue records one universal submission for the unsubmitted
// Value-choice prompt naming column. Empty text completes as empty TEXT; a
// typed `NULL` stays TEXT, distinct from the SQL-NULL choice.
func (q QueryBuilder) SubmitInsertValue(column, text string) (QueryBuilder, bool) {
	next := q
	for i := range next.inserts {
		c := &next.inserts[i]
		if c.Column == column && c.choice == InsertChoiceValue && !c.submitted {
			c.value = ParseValue(text)
			c.input = text
			c.submitted = true
			return next, true
		}
	}
	return q, false
}

// InsertPromptHint reports the exact omission hint for the INSERT prompt
// naming column, delegating to the selected object's schema metadata: the
// hint appears only on the single-column INTEGER PRIMARY KEY rowid alias of
// a has-rowid table, never from UI type inference. A column without a
// prompt state or without the hint reports false. The hint annotates the
// prompt only; it never changes the offered {Value, NULL, Default/Omit}
// choices, selects omission automatically, or alters any behavior.
func (q QueryBuilder) InsertPromptHint(column string) (string, bool) {
	if q.command != CommandInsert || !q.tableSet {
		return "", false
	}
	if _, found := q.insertPrompt(column); !found {
		return "", false
	}
	obj := q.findObject(q.table)
	if obj == nil {
		return "", false
	}
	for _, col := range obj.Columns {
		if col.Name == column {
			return obj.InsertHint(col)
		}
	}
	return "", false
}
