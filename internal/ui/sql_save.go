// Ctrl+S save targeting inside the UI (Issue #48), per the Query save
// targeting decision in Notes/PRD-sqloid.md. Target resolution is a pure
// in-memory step: the viewed historical result's query is resolved through
// its backing immutable history entry (never through visible text), the
// current builder contributes only its Issue #19 authoritative runnable
// verdict, and the last actual execution resolves through the stable
// query-history entry recorded at its actual execution start. Resolution
// starts no validation, schema refresh, connection, or database work; a
// failed resolution shows exactly `no runnable query to save` and never
// opens a picker or serializes anything. On success the immutable complete
// query state is recorded as the prepared save target handed to the picker
// flow owned by later issues; this issue implements no filesystem behavior.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// NoRunnableQueryFeedback is the exact inline feedback for a Ctrl+S press
// with no resolvable save target; no picker opens and nothing serializes.
const NoRunnableQueryFeedback = "no runnable query to save"

// viewedResultSaveQuery resolves the currently viewed historical result's
// query through its backing immutable history entry: the result entry's
// stable query-history association must still resolve against the retained
// store. Visible text is never inspected. A false result leaves the next
// ordinary priority to apply.
func (m Model) viewedResultSaveQuery() (qb.HistoryState, bool) {
	if !m.resultHistoryMode || m.resultHistoryView == nil || m.ResultHistory == nil {
		return qb.HistoryState{}, false
	}
	entry, ok := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	if !ok || entry.QueryEntryID == 0 {
		return qb.HistoryState{}, false
	}
	if m.History == nil {
		return qb.HistoryState{}, false
	}
	e, ok := m.History.Lookup(entry.QueryEntryID)
	if !ok {
		// The associated entry was evicted: the association no longer
		// resolves in memory, so the next candidate applies.
		return qb.HistoryState{}, false
	}
	return e.State, true
}

// lastExecutionSaveQuery resolves the last actual execution's immutable
// query state through its recorded stable query-history entry.
func (m Model) lastExecutionSaveQuery() (qb.HistoryState, bool) {
	if m.lastExecQueryEntryID == 0 || m.History == nil {
		return qb.HistoryState{}, false
	}
	e, ok := m.History.Lookup(m.lastExecQueryEntryID)
	if !ok {
		return qb.HistoryState{}, false
	}
	return e.State, true
}

// sqlSaveInput collects the immutable in-memory Ctrl+S candidates. Ordinary
// states contribute the viewed-result query, the builder with its runnable
// verdict, and the last execution; terminal states contribute only the
// Ctrl+P/N selection and the last execution. Nothing here validates,
// refreshes schema, opens a connection, or issues database work.
func (m Model) sqlSaveInput() export.SQLSaveInput {
	var in export.SQLSaveInput
	in.Terminal = m.terminalState != TerminalNone
	if !in.Terminal {
		if state, ok := m.viewedResultSaveQuery(); ok {
			in.ViewedResultQuery = &state
		}
	}
	if report := m.QB.RunnableReport(); report.Runnable {
		state := m.QB.HistoryState()
		in.Builder = &state
		in.BuilderRunnable = true
	}
	if m.terminalState != TerminalNone && m.historyMode && m.History != nil {
		// Terminal-only priority: the Ctrl+P/N-selected immutable query.
		if e, ok := m.History.Lookup(m.historyCursorID); ok {
			in.TerminalSelection = &e.State
		}
	}
	if state, ok := m.lastExecutionSaveQuery(); ok {
		in.LastExecution = &state
	}
	return in
}

// handleSQLSaveKey resolves one Ctrl+S press entirely in memory. A failed
// resolution records exactly NoRunnableQueryFeedback, opens no picker, and
// prepares nothing; a successful resolution records the immutable complete
// query state and opens the Issue #52 destination picker at the process
// working directory, without starting validation, serialization, or any
// database work.
func (m Model) handleSQLSaveKey() (Model, tea.Cmd) {
	target, err := export.ResolveSQLSaveTarget(m.sqlSaveInput())
	if err != nil {
		m.saveNotice = export.ErrNoRunnableQuery.Error()
		m.savePrepared = nil
		return m, nil
	}
	m.saveNotice = ""
	m.savePrepared = &target
	return m, m.openPicker(pickerFlowSave, filepicker.FormatSQL)
}
