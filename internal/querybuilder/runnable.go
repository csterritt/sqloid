// Authoritative runnable-state reporting (Issue #19): one pure, UI-independent
// evaluation of the Runnable-State Contract in Notes/PRD-sqloid.md. The report
// is either runnable or the first invalid typed field plus one specific reason,
// ordered by each command's visual builder order — never by validation
// implementation order. It reuses the grouping, ORDER BY, LIMIT, projection,
// and Schema identity contracts instead of duplicating validators, carries no
// UI action, and never starts validation, estimation, execution, or history
// append.
//
// The UPDATE SET assignment and INSERT per-column choice seams defined in
// write_state.go make all four command contracts representable now, so Issues
// #37 and #39 can adopt them unchanged.

package querybuilder

import "fmt"

// RunField is one typed runnable-report field target in visual builder order.
// The UI maps these onto its exact focus targets; the report itself knows
// nothing about rendering.
type RunField int

const (
	// RunFieldCommand is the top-level Command field.
	RunFieldCommand RunField = iota
	// RunFieldTable is the Table field.
	RunFieldTable
	// RunFieldProjection is the SELECT Column(s) field.
	RunFieldProjection
	// RunFieldSetAssignments is the UPDATE SET assignments field.
	RunFieldSetAssignments
	// RunFieldInsertColumns is the INSERT per-column prompts field.
	RunFieldInsertColumns
	// RunFieldWhere is the WHERE field.
	RunFieldWhere
	// RunFieldGroupBy is the GROUP BY field.
	RunFieldGroupBy
	// RunFieldOrderBy is the ORDER BY field.
	RunFieldOrderBy
	// RunFieldLimit is the Limit field.
	RunFieldLimit
)

// String renders the human-facing field name used in tests and diagnostics.
func (f RunField) String() string {
	switch f {
	case RunFieldCommand:
		return "Command"
	case RunFieldTable:
		return FieldIdentityTable
	case RunFieldProjection:
		return FieldIdentityColumns
	case RunFieldSetAssignments:
		return "SET"
	case RunFieldInsertColumns:
		return "INSERT"
	case RunFieldWhere:
		return "WHERE"
	case RunFieldGroupBy:
		return FieldIdentityGroupBy
	case RunFieldOrderBy:
		return FieldIdentityOrderBy
	case RunFieldLimit:
		return FieldIdentityLimit
	default:
		return "RunField(out-of-range)"
	}
}

// Exact user-facing runnable reasons reported through RunnableReport.Reason;
// tests assert these wordings verbatim.
const (
	// ReasonNoCommand reports a builder with no selected command.
	ReasonNoCommand = "select a command"
	// ReasonNoTable reports a selected command without a table.
	ReasonNoTable = "select a table"
	// ReasonStaleTable reports a selected table absent from the refreshed
	// catalog snapshot.
	ReasonStaleTable = "the selected table no longer exists"
	// ReasonNoProjection reports a SELECT with no committed projection entry.
	ReasonNoProjection = "select at least one column"
	// ReasonIncompletePrompt reports any open value prompt or incomplete
	// guided state: the common no-incomplete-value-prompt gate.
	ReasonIncompletePrompt = "complete the open value prompt"
	// ReasonStaleWhereColumn reports a committed WHERE naming a column that no
	// longer exists among the selected object's visible columns.
	ReasonStaleWhereColumn = "the where column no longer exists"
	// ReasonNoSetAssignments reports an UPDATE without any SET assignment.
	ReasonNoSetAssignments = "add at least one SET assignment"
	// ReasonDuplicateSetColumns reports an UPDATE whose SET columns repeat.
	ReasonDuplicateSetColumns = "SET columns must be unique"
	// ReasonStaleSetColumn reports an UPDATE assignment whose column is absent
	// from the selected table's refreshed visible columns.
	ReasonStaleSetColumn = "the SET column no longer exists"
	// ReasonNoInsertableColumns reports an INSERT onto a table with zero
	// insertable columns; the exact PRD wording.
	ReasonNoInsertableColumns = "table has no insertable columns"
	// ReasonIncompleteChoiceFmt reports an UPDATE SET assignment or INSERT
	// column whose {Value, NULL[, Default/Omit]} choice is still pending; %s
	// is the declared column name.
	ReasonIncompleteChoiceFmt = "complete the choice for column %s"
	// ReasonUnsubmittedValueFmt reports a Value choice whose universal entry
	// was never submitted; %s is the declared column name.
	ReasonUnsubmittedValueFmt = "submit a value for column %s"
)

// RunnableReport is one authoritative evaluation: runnable data, or the first
// invalid field in visual order with its specific reason. It carries no UI
// action by construction.
type RunnableReport struct {
	Runnable bool     // true when every prerequisite and gate holds
	Field    RunField // first invalid field in visual order; unused when Runnable
	Reason   string   // exact reason text asserted by tests and shown verbatim
}

// RunnableReport evaluates the full Runnable-State Contract: the selected
// command and refreshed-identifier common gates, then each command's own
// prerequisites in visual order. Incomplete value prompts (an open WHERE
// draft, pending choices, unsubmitted Value entries) block at their own field,
// preserving the visual-order rule even though the gate is described as
// common.
func (q QueryBuilder) RunnableReport() RunnableReport {
	if !q.command.Selected() {
		return RunnableReport{Field: RunFieldCommand, Reason: ReasonNoCommand}
	}
	name, ok := q.SelectedTable()
	if !ok {
		return RunnableReport{Field: RunFieldTable, Reason: ReasonNoTable}
	}
	if q.findObject(name) == nil {
		return RunnableReport{Field: RunFieldTable, Reason: ReasonStaleTable}
	}
	switch q.command {
	case CommandSelect:
		return q.reportSelect()
	case CommandUpdate:
		return q.reportUpdate()
	case CommandDelete:
		r, _ := q.reportWhere() // DELETE: only the optional WHERE remains
		return r
	case CommandInsert:
		return q.reportInsert()
	}
	return RunnableReport{Runnable: true}
}

// reportSelect evaluates a SELECT in visual order: projection, WHERE,
// grouping, ORDER BY, Limit — reusing the Issue #18 validators for the rules
// they already own.
func (q QueryBuilder) reportSelect() RunnableReport {
	if q.ProjectionEmpty() {
		return RunnableReport{Field: RunFieldProjection, Reason: ReasonNoProjection}
	}
	if r, invalid := q.reportWhere(); invalid {
		return r
	}
	if issue, invalid := validateGrouping(q); invalid {
		return RunnableReport{Field: RunFieldGroupBy, Reason: issue.Reason}
	}
	if issue, invalid := validateOrderBy(q); invalid {
		return RunnableReport{Field: RunFieldOrderBy, Reason: issue.Reason}
	}
	if issue, invalid := validateLimit(q); invalid {
		return RunnableReport{Field: RunFieldLimit, Reason: issue.Reason}
	}
	return RunnableReport{Runnable: true}
}

// reportUpdate evaluates an UPDATE in visual order: SET assignments, then the
// optional WHERE. Duplicate SET columns block; every assignment needs exactly
// one complete {Value, NULL} choice, and Value entries must be submitted.
func (q QueryBuilder) reportUpdate() RunnableReport {
	if len(q.sets) == 0 {
		return RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonNoSetAssignments}
	}
	eligible := make(map[string]bool, len(q.SetCandidates()))
	for _, column := range q.SetCandidates() {
		eligible[column.Name] = true
	}
	seen := make(map[string]bool, len(q.sets))
	for _, a := range q.sets {
		if !eligible[a.Column] {
			return RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonStaleSetColumn}
		}
		if seen[a.Column] {
			return RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonDuplicateSetColumns}
		}
		seen[a.Column] = true
	}
	for _, a := range q.sets {
		switch {
		case a.choice != SetChoiceValue && a.choice != SetChoiceNull:
			return RunnableReport{Field: RunFieldSetAssignments,
				Reason: fmt.Sprintf(ReasonIncompleteChoiceFmt, a.Column)}
		case a.choice == SetChoiceValue && !a.submitted:
			return RunnableReport{Field: RunFieldSetAssignments,
				Reason: fmt.Sprintf(ReasonUnsubmittedValueFmt, a.Column)}
		}
	}
	r, _ := q.reportWhere()
	return r
}

// reportInsert evaluates an INSERT in visual order: the zero-insertable-column
// block, then every per-column prompt. All-omit is valid; a missing prompt
// state is an incomplete choice, so prompts must be begun for runnable data.
func (q QueryBuilder) reportInsert() RunnableReport {
	insertable := q.InsertableColumns()
	if len(insertable) == 0 {
		return RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonNoInsertableColumns}
	}
	for _, col := range insertable {
		c, found := q.insertPrompt(col.Name)
		if !found || c.choice == InsertChoiceNone {
			return RunnableReport{Field: RunFieldInsertColumns,
				Reason: fmt.Sprintf(ReasonIncompleteChoiceFmt, col.Name)}
		}
		if c.choice == InsertChoiceValue && !c.submitted {
			return RunnableReport{Field: RunFieldInsertColumns,
				Reason: fmt.Sprintf(ReasonUnsubmittedValueFmt, col.Name)}
		}
	}
	return RunnableReport{Runnable: true}
}

// reportWhere evaluates the shared WHERE gate for SELECT, UPDATE, and DELETE:
// an open draft or a committed predicate that is not structurally complete
// blocks as the no-incomplete-value-prompt gate, and a committed predicate
// naming a vanished column blocks as a stale identifier.
func (q QueryBuilder) reportWhere() (RunnableReport, bool) {
	if q.whereDrafting {
		return RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt}, true
	}
	if q.whereSet {
		p := q.where
		if p.State() != WhereComplete {
			return RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt}, true
		}
		name, ok := p.Column()
		if !ok {
			return RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt}, true
		}
		for _, col := range q.WhereCandidates() {
			if col.Name == name.Name {
				return RunnableReport{Runnable: true}, false
			}
		}
		return RunnableReport{Field: RunFieldWhere, Reason: ReasonStaleWhereColumn}, true
	}
	return RunnableReport{Runnable: true}, false
}

// insertPrompt locates one committed prompt state by column name.
func (q QueryBuilder) insertPrompt(name string) (InsertColumn, bool) {
	for _, c := range q.inserts {
		if c.Column == name {
			return c, true
		}
	}
	return InsertColumn{}, false
}
