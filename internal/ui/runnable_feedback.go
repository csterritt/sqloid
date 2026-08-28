// Authoritative runnable feedback in the TUI (Issue #19 Tasks 5–6): base
// -context Enter consults the UI-independent QueryBuilder runnable report
// after the global precedence contexts (open popups, value prompts, pending
// requests, stale-refresh flow, suspension) have had their say. Invalid data
// consumes Enter: focus moves to the report's typed first-invalid field in
// visual order and the exact reason renders inline — with no validation,
// estimation, execution, or history command. Runnable data emits only the
// pre-execution lifecycle seam consumed by later schema-validation and
// destructive-preparation issues; this issue never executes SQL.
//
// Fields that own their own editing opener (popups and universal value entry)
// keep consuming Enter locally, preserving the Issue #12–#18 openers; the one
// authoritative exception is the Limit field with nonempty invalid committed
// text, where the PRD requires Enter to display the exact reason instead of
// reopening universal entry.

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// setFieldLabel and insertFieldLabel are the field-bar labels of the UPDATE
// SET assignments field and the INSERT per-column prompts field; they are
// also the focus targets for the report's write-command field identities.
const (
	setFieldLabel    = "Set"
	insertFieldLabel = "Insert"
)

// PreExecutionRequestedMsg is the pre-execution lifecycle seam: emitted only
// when Enter lands on runnable data in an idle supported base context. Later
// issues route it into cancellable schema validation and the write
// preparation workflow; nothing executes directly in response.
type PreExecutionRequestedMsg struct{}

// requestPreExecution returns the seam command for runnable base-context
// Enter presses.
func requestPreExecution() tea.Msg { return PreExecutionRequestedMsg{} }

// focusedFieldHasOpener reports whether the currently focused base field owns
// an Enter-driven editing opener (a popup or universal value entry) that
// consumes Enter locally before any runnable-gate consultation.
func (m *Model) focusedFieldHasOpener() bool {
	if m.suspended || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	switch m.Fields[m.Focus].Label {
	case tableFieldLabel, columnsFieldLabel, whereFieldLabel,
		groupByFieldLabel, orderByFieldLabel, limitFieldLabel:
		return true
	default:
		return false
	}
}

// handleBaseEnter applies the authoritative runnable gate to one Enter press
// that reached the idle base context. The returned command is non-nil only on
// the runnable-data seam or a field opener's issued work.
func (m *Model) handleBaseEnter() tea.Cmd {
	// Pending requests and the stale-schema retry flow consume Enter with no
	// runnable action (PRD key precedence: Enter ignored while a request is
	// in flight).
	if m.refreshPending || m.ActiveCancellable || m.schemaStale {
		return nil
	}
	report := m.QB.RunnableReport()
	// Authoritative exception: a focused Limit field holding nonempty invalid
	// committed text never reopens universal entry. When Limit is the report's
	// first invalid field the exact reason is already rendered in its content;
	// otherwise the report governs and focus moves to the earlier invalid
	// field.
	if m.limitFocused() {
		if text := m.QB.LimitInput(); text != "" {
			if _, ok := m.QB.LimitValue(); !ok {
				if !report.Runnable && report.Field != qb.RunFieldLimit {
					m.focusRunnableField(report.Field)
					m.showRunnableReason(report.Reason)
					return nil
				}
				return nil
			}
		}
	}
	if m.focusedFieldHasOpener() {
		// The field's own editing opener consumes Enter locally regardless of
		// the report, preserving Issue #12–#18 opener behavior.
		return m.openPopupCmd(tea.KeyMsg{Type: tea.KeyEnter})
	}
	if !report.Runnable {
		m.focusRunnableField(report.Field)
		m.showRunnableReason(report.Reason)
		return nil
	}
	// Runnable data: emit only the pre-execution seam; no execution starts
	// within this issue.
	return requestPreExecution
}

// runnableFieldLabel maps a report field identity onto its exact visual field
// -bar label, including the future UPDATE/INSERT write prompt targets.
func runnableFieldLabel(f qb.RunField) string {
	switch f {
	case qb.RunFieldCommand:
		return commandFieldLabel
	case qb.RunFieldTable:
		return tableFieldLabel
	case qb.RunFieldProjection:
		return columnsFieldLabel
	case qb.RunFieldSetAssignments:
		return setFieldLabel
	case qb.RunFieldInsertColumns:
		return insertFieldLabel
	case qb.RunFieldWhere:
		return whereFieldLabel
	case qb.RunFieldGroupBy:
		return groupByFieldLabel
	case qb.RunFieldOrderBy:
		return orderByFieldLabel
	case qb.RunFieldLimit:
		return limitFieldLabel
	default:
		return commandFieldLabel
	}
}

// focusRunnableField moves focus to the exact visual target of the report's
// typed field; a field the current builder does not render is ignored so
// stale reports can never land focus outside the field bar.
func (m *Model) focusRunnableField(f qb.RunField) {
	label := runnableFieldLabel(f)
	for i := range m.Fields {
		if m.Fields[i].Label == label {
			m.setFocus(i)
			return
		}
	}
}

// showRunnableReason renders the report's reason verbatim beside the focused
// field's content. The display is transient: the next applyBuilder rebuilds
// the field bar from the authoritative snapshot, which removes superseded
// feedback whenever the focused field is corrected or cleared.
func (m *Model) showRunnableReason(reason string) {
	if m.Focus < 0 || m.Focus >= len(m.Fields) {
		return
	}
	f := m.Fields[m.Focus]
	content := strings.TrimRight(f.Content, " ")
	if content == "" {
		f.Content = reason
	} else {
		f.Content = content + " — " + reason
	}
	m.Fields[m.Focus] = f
}

// setFieldContent renders the committed UPDATE SET assignments in selection
// order: submitted Value choices show the placeholder atom, NULL choices show
// the SQL keyword, and every incomplete choice is marked.
func setFieldContent(q qb.QueryBuilder) string {
	parts := make([]string, 0, len(q.SetAssignments()))
	for _, a := range q.SetAssignments() {
		switch a.Choice() {
		case qb.SetChoiceValue:
			if _, submitted := a.SubmittedValue(); submitted {
				parts = append(parts, a.Column+" = ?")
			} else {
				parts = append(parts, a.Column+" = ? (incomplete)")
			}
		case qb.SetChoiceNull:
			parts = append(parts, a.Column+" = NULL")
		default:
			parts = append(parts, a.Column+" (incomplete)")
		}
	}
	return strings.Join(parts, ", ")
}

// insertFieldContent renders the INSERT per-column prompts in declared order,
// including the incomplete marker for prompts still awaiting a choice.
func insertFieldContent(q qb.QueryBuilder) string {
	parts := make([]string, 0, len(q.InsertColumns()))
	for _, c := range q.InsertColumns() {
		name := c.Choice().String()
		if c.Choice() == qb.InsertChoiceNone {
			name = "incomplete"
		}
		parts = append(parts, c.Column+": "+name)
	}
	return strings.Join(parts, ", ")
}
