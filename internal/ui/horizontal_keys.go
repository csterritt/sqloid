// Whole-column horizontal result-grid scrolling (Issue #29), per the
// Builder and Display Interaction and Global Key Precedence and
// Context/Action Matrix sections of Notes/PRD-sqloid.md. The horizontal
// position of the grid is exactly one first-visible output-column index on
// the Model; Shift+Page Down and `.` advance it exactly one whole column and
// Shift+Page Up and `,` retreat it exactly one, regardless of how many
// columns fit. Boundary presses are consumed without any state change or
// command, and horizontal movement is purely local: it never dispatches a
// database request and stays available while any SELECT page or count request
// is pending. Higher-precedence contexts (terminal, quit confirmation,
// overlay, focused input/search, too-small screen) consume the keys before
// base handling, exactly like every other base action.

package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// ShiftPageMsg is the model-facing message for the terminal Shift+Page
// Down/Up keys. Real terminals send the xterm sequences ESC[6;2~ and
// ESC[5;2~, which the Bubble Tea input reader reports as unknown CSI
// messages rather than KeyMsgs; the bridge in shiftPageDirection converts
// those representations (and accepts this message directly) so the
// Issue #29 bindings stay testable without naming any unexported type.
// Down selects Shift+Page Down; false selects Shift+Page Up.
type ShiftPageMsg struct {
	Down bool
}

// shiftPageDirection reports the whole-column delta (±1) requested by a
// Shift+Page message, bridged either from the synthetic ShiftPageMsg or from
// the raw unknown-CSI representation, or 0, false when msg is neither.
func shiftPageDirection(msg tea.Msg) (int, bool) {
	switch m := msg.(type) {
	case ShiftPageMsg:
		if m.Down {
			return 1, true
		}
		return -1, true
	default:
		if s, ok := msg.(fmt.Stringer); ok {
			switch s.String() {
			case "?CSI[54 59 50 126]?": // xterm ESC[6;2~ — Shift+Page Down
				return 1, true
			case "?CSI[53 59 50 126]?": // xterm ESC[5;2~ — Shift+Page Up
				return -1, true
			}
		}
		return 0, false
	}
}

// handleHorizontalKey moves the first-visible output-column index by delta
// whole columns (Issue #29). Only the index changes: accepted moves issue no
// database command, never touch request ownership, and remain local while
// any SELECT page or count request is pending. Boundary presses are consumed
// as no-ops through the pure horizontalStep arithmetic.
func (m *Model) handleHorizontalKey(delta int) {
	if next, accepted := horizontalStep(m.firstColumn, len(m.outputColumnNames()), delta); accepted {
		m.firstColumn = next
	}
}

// outputColumnNames returns the current deduplicated output column names, or
// nil when no result is displayed — the only horizontal extent the grid has.
func (m Model) outputColumnNames() []string {
	if m.Result == nil || m.Result.Page == nil {
		return nil
	}
	return m.Result.Page.HeaderNames()
}

// clampFirstColumnModel normalizes the first-visible output-column index
// after column or width changes: a valid index is preserved and an invalid
// one is clamped to the nearest valid output-column boundary, including
// empty and single-column results. Resize paths call this after the viewport
// generation bump.
func (m *Model) clampFirstColumnModel() {
	m.firstColumn = clampFirstColumn(m.firstColumn, len(m.outputColumnNames()))
}
