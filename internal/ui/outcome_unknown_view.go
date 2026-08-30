// Outcome-unknown terminal view (Issue #45), per the Writes and commit
// boundary and Global Key Precedence decisions in Notes/PRD-sqloid.md. The
// terminal result view renders only the currently selected immutable
// non-tabular entry — always resolved by stable ID from the result-history
// store, never synthesized and never rendered through a missing backing
// entry — plus the reduced help of this terminal state. The view never
// claims the database was committed, rolled back, or untouched.

package ui

import (
	"strings"

	"github.com/chris/sqloid/internal/history"
)

// OutcomeUnknownHeading is the exact first line of the outcome-unknown
// terminal view: it states the outcome could not be proven without claiming
// any resolution.
const OutcomeUnknownHeading = "Outcome unknown — the write's final state could not be proven"

// outcomeUnknownHelpLines are the reduced help contents of the terminal
// state: only the in-memory history actions, help dismissal, and the
// immediate quit are available here. No database suggestion is included.
var outcomeUnknownHelpLines = []string{
	"Reduced help — only these actions are available:",
	"",
	"Ctrl+P / Ctrl+N   select an older/newer query from history",
	"Ctrl+E / Ctrl+Y   select an older/newer result from history",
	"Esc               dismiss help",
	"q or Ctrl+C       quit immediately (status 1)",
}

// selectedOutcomeUnknownEntry resolves the currently selected result entry
// for the terminal view: the stable-ID selection while browsing, otherwise
// the newest retained entry. The boolean is false only when nothing is
// retained; no entry is ever synthesized.
func (m Model) selectedOutcomeUnknownEntry() (history.ResultEntry, bool) {
	if m.resultHistoryMode {
		return m.ResultHistory.Lookup(m.resultHistoryCursorID)
	}
	return m.ResultHistory.Newest()
}

// renderOutcomeUnknownTerminal draws the full terminal view for the
// outcome-unknown state: the heading, the selected entry's summary, SQL,
// and rows-affected line, and the reduced help when open. Rendering is
// deterministic for a given model and dimensions.
func (m Model) renderOutcomeUnknownTerminal() string {
	lines := []string{OutcomeUnknownHeading, ""}
	if e, ok := m.selectedOutcomeUnknownEntry(); ok {
		lines = append(lines, e.Summary, "SQL: "+e.SQL)
		if e.RowsAffected > 0 {
			lines = append(lines, "Rows affected reported by the statement: this does not prove persistence")
		}
	}
	lines = append(lines,
		"",
		"Ctrl+P / Ctrl+N query history · Ctrl+E / Ctrl+Y result history · ? help · q or Ctrl+C quits (status 1)",
	)
	if m.terminalHelpOpen {
		lines = append(lines, "", outcomeUnknownHelpLines[0])
		lines = append(lines, outcomeUnknownHelpLines[1:]...)
	}
	return strings.Join(lines, "\n")
}
