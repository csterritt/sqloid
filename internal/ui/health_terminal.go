// Health-terminal workflow inside the UI (Issue #46), per the Session health
// and Global Key Precedence decisions in Notes/PRD-sqloid.md. Issue #7's
// typed request-boundary classifications — deletion and same-path
// replacement — are consumed here as typed *connection.HealthError values,
// never by parsing driver text, and mapped onto the two terminal states whose
// exact user-facing messages are owned by this package (the health layer
// carries no terminal copy). Entry transitions atomically only after no
// transaction or driver work remains pending; selection, navigation, and
// rendering thereafter never issue a request. Save/export integration in
// these terminals remains owned by Issues #48/#49.

package ui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/result"
)

// healthTerminalFor maps one typed Issue #7 health classification onto its
// terminal state. It inspects only the typed *connection.HealthError kind —
// never error text — so a decoy error string can never select a terminal.
func healthTerminalFor(err error) (TerminalState, bool) {
	var he *connection.HealthError
	if !errors.As(err, &he) {
		return TerminalNone, false
	}
	switch he.Kind {
	case connection.HealthReplaced:
		return TerminalReplaced, true
	case connection.HealthDeleted:
		return TerminalDeleted, true
	default:
		return TerminalNone, false
	}
}

// endSelectIntoHealthTerminal finalizes the active SELECT exactly once
// without appending a health-failed snapshot entry — the database is gone or
// unproven, so no truthful entry can be constructed — then enters the mapped
// terminal state. It reports whether the classification was health-typed.
func (m *Model) endSelectIntoHealthTerminal(err error) bool {
	state, ok := healthTerminalFor(err)
	if !ok {
		return false
	}
	m.suppressFinalizedAppend = true
	m.finalizeActiveSelect()
	m.suppressFinalizedAppend = false
	m.enterTerminal(state)
	return true
}

// healthTerminalHelpLines are the reduced help contents shared by both
// health terminals: only the in-memory history selection, help dismissal,
// and the immediate quit are available. No database suggestion appears.
var healthTerminalHelpLines = outcomeUnknownHelpLines

// selectedHealthTerminalEntry resolves the currently selected result entry
// for the health terminal view: the stable-ID selection while browsing,
// otherwise nothing (empty histories keep the selection empty). The boolean
// is false when no backing entry exists; no entry is ever synthesized.
func (m Model) selectedHealthTerminalEntry() (history.ResultEntry, bool) {
	if m.resultHistoryMode && m.resultHistoryView != nil {
		return m.ResultHistory.Lookup(m.resultHistoryCursorID)
	}
	return history.ResultEntry{}, false
}

// healthTerminalMessage returns the exact primary message of the current
// terminal state.
func healthTerminalMessage(state TerminalState) string {
	if state == TerminalReplaced {
		return ReplacedSessionEndedMessage
	}
	return DeletedSessionEndedMessage
}

// terminalHistorySQL renders the currently selected query-history state's
// executed SQL for the terminal view.
func terminalHistorySQL(q qb.QueryBuilder) string {
	switch q.Command() {
	case qb.CommandUpdate:
		return q.UpdateSQL()
	case qb.CommandDelete:
		return q.DeleteSQL()
	case qb.CommandInsert:
		return q.InsertSQL()
	default:
		return q.SelectSQL()
	}
}

// renderHealthTerminal draws the health terminal view. The exact primary
// message is always the first line and, while nothing is selected, the whole
// view. With a stable-backed result entry selected, its immutable projection
// renders below the message; selected query-history states render their
// complete executed SQL. The reduced help renders only its own actions.
// Rendering is deterministic and never issues a request.
func (m Model) renderHealthTerminal() string {
	message := healthTerminalMessage(m.terminalState)
	nothingSelected := !m.historyMode && !m.resultHistoryMode && !m.terminalHelpOpen
	if nothingSelected {
		return message
	}
	lines := []string{message, ""}
	if m.historyMode {
		lines = append(lines, "Selected query history: "+terminalHistorySQL(m.QB))
		lines = append(lines, "")
	}
	if e, ok := m.selectedHealthTerminalEntry(); ok {
		if e.Kind == history.KindTabular && m.resultHistoryView != nil {
			_, rowLines := renderResultContent(m.resultHistoryView, result.CountState{}, false, false, false, m.Width-resultsBorderRows, m.firstColumn)
			lines = append(lines, rowLines...)
		} else {
			lines = append(lines, e.Summary)
			if e.SQL != "" {
				lines = append(lines, "SQL: "+e.SQL)
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines,
		"Ctrl+P / Ctrl+N query history · Ctrl+E / Ctrl+Y result history · ? help · q or Ctrl+C quits (status 1)",
	)
	if m.terminalHelpOpen {
		lines = append(lines, "", healthTerminalHelpLines[0])
		lines = append(lines, healthTerminalHelpLines[1:]...)
		lines = append(lines, "", terminalHelpClosingLine)
	}
	return strings.Join(lines, "\n")
}

// handleTerminalHealthKey dispatches one key inside either health terminal
// state. The reduced in-memory key set is exactly the Issue #45 terminal
// set: Ctrl+P/N and Ctrl+E/Y navigate the immutable history stores locally,
// ? toggles the reduced help, and q/Ctrl+C exit immediately with status 1.
// Every other key is consumed as an inert no-op — the database-action gate
// stays authoritative before any command can be built.
func (m Model) handleTerminalHealthKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleTerminalOutcomeUnknownKey(msg)
}
