// Outcome-unknown terminal key handling (Issue #45), per the Global Key
// Precedence and Context/Action matrix in Notes/PRD-sqloid.md. Terminal
// rules take precedence over every other context: only the in-memory
// query-history (Ctrl+P/N) and result-history (Ctrl+E/Y) selection, the
// reduced help, its dismissal, and the immediate quit are available. Every
// database-starting action is suppressed before any command can be built —
// no execution, validation, estimation, refresh, paging, rerun, or other
// request is ever issued — and selection always resolves through the
// immutable backing store by stable ID.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleTerminalOutcomeUnknownKey dispatches one key inside the
// outcome-unknown terminal state. It returns no command ever: navigation is
// purely local over the retained immutable history stores, help toggling is
// a state flag, and every other key is consumed as an inert no-op.
func (m Model) handleTerminalOutcomeUnknownKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// Issue #45: the terminal quit takes precedence over confirmation,
		// help, and history contexts and exits immediately with status 1. No
		// transaction or driver work remains pending (terminal entry settled
		// it), so nothing is cancelled, cleaned up, or delayed: the only
		// command is the quit itself, which the runner maps onto status 1.
		m.quitConfirm = false
		m.quitSuspended = nil
		m.quitWaitWrite = false
		m.terminalHelpOpen = false
		m.historyMode = false
		m.resultHistoryMode = false
		m.exitStatus = 1
		return m, tea.Quit
	case "ctrl+p":
		if !m.historyMode {
			next, _ := m.enterHistoryMode()
			nm := next.(Model)
			nm.adjustScroll()
			return nm, nil
		}
		return m.historyStep(false)
	case "ctrl+n":
		if !m.historyMode {
			next, _ := m.enterHistoryMode()
			nm := next.(Model)
			nm.adjustScroll()
			return nm, nil
		}
		return m.historyStep(true)
	case "ctrl+e":
		if !m.resultHistoryMode {
			m.enterTerminalResultHistory()
		} else {
			m.terminalResultHistoryStep(false)
		}
		m.adjustScroll()
		return m, nil
	case "ctrl+y":
		if !m.resultHistoryMode {
			m.enterTerminalResultHistory()
		} else {
			m.terminalResultHistoryStep(true)
		}
		m.adjustScroll()
		return m, nil
	case "ctrl+s":
		// Issue #48: terminal save targeting resolves only from the
		// Ctrl+P/N-selected immutable query or the last actual execution —
		// never from builder or viewed-result candidates — entirely in
		// memory, with the exact no-target feedback and no picker.
		next, cmd := m.handleSQLSaveKey()
		return next, cmd
	case "?":
		// Reduced help opens only outside history browsing; its contents list its contents list
		// just the actions available in this terminal state.
		if !m.historyMode && !m.resultHistoryMode {
			m.terminalHelpOpen = !m.terminalHelpOpen
		}
		m.adjustScroll()
		return m, nil
	case "esc":
		// Esc dismisses the top in-memory layer: the help, then history
		// browsing. It never dismisses the terminal state itself.
		if m.terminalHelpOpen {
			m.terminalHelpOpen = false
		} else if m.historyMode {
			m.exitHistoryMode()
		} else if m.resultHistoryMode {
			m.exitResultHistoryMode()
		}
		m.adjustScroll()
		return m, nil
	}
	// Every other key — including Enter and every printable input — is
	// consumed with no state change: the terminal state starts no database
	// work and leaves no editable builder path.
	m.adjustScroll()
	return m, nil
}

// enterTerminalResultHistory selects the newest retained result entry —
// which is the outcome-unknown entry appended at settlement — entirely in
// memory. It never finalizes anything, never appends, and is a no-op when
// nothing is retained; no entry is synthesized.
func (m *Model) enterTerminalResultHistory() {
	m.exitHistoryMode()
	m.terminalHelpOpen = false
	if m.ResultHistory == nil || m.ResultHistory.Len() == 0 {
		return
	}
	e, ok := m.ResultHistory.Newest()
	if !ok {
		return
	}
	m.resultHistoryMode = true
	m.resultHistoryCursorID = e.ID
	m.resultHistoryNotice = ""
	m.projectSelectedHistoryEntry()
}

// terminalResultHistoryStep moves the selection one entry older (newer ==
// false, Ctrl+E) or newer (newer == true, Ctrl+Y) through the stable-ID
// primitives. Unlike ordinary browsing, the newest boundary never exits
// result-history mode here — the terminal result view is the session's only
// home — so boundary presses are deterministic no-ops.
func (m *Model) terminalResultHistoryStep(newer bool) {
	if m.ResultHistory == nil {
		return
	}
	e, ok := m.ResultHistory.NewerThan(m.resultHistoryCursorID)
	if !newer {
		e, ok = m.ResultHistory.OlderThan(m.resultHistoryCursorID)
	}
	if !ok {
		return
	}
	m.resultHistoryCursorID = e.ID
	m.resultHistoryNotice = ""
	m.projectSelectedHistoryEntry()
}
