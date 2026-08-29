// Scripted (model, msg) → (model, cmd) coverage for the whole-column
// horizontal scrolling bindings (Issue #29 Tasks 3–4), per the Builder and
// Display Interaction and Global Key Precedence and Context/Action Matrix
// sections of Notes/PRD-sqloid.md. Shift+Page Down and `.` move the
// first-visible output-column index exactly one column forward and
// Shift+Page Up and `,` exactly one column back, regardless of how many
// columns fit; boundary presses are consumed as no-ops; accepted moves issue
// no database command and leave request counts unchanged; movement stays
// local while SELECT first-page, later-page, or count work is in flight
// through the controllable fake executor seams; and higher-precedence
// contexts (terminal, quit confirmation, overlay, focused input/search,
// too-small screen) consume the keys according to their owning context.
// Reuses the pure layout assertions from horizontal_layout_test.go.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// fiveColumnPage is a wide settled result: five narrow output columns so the
// default 80-column terminal fits them all and a press can move exactly one
// whole column at any index.
func fiveColumnPage() *result.Page {
	cols := []string{"c1", "c2", "c3", "c4", "c5"}
	rows := [][]result.Value{
		{result.NewInteger(1), result.NewInteger(2), result.NewInteger(3), result.NewInteger(4), result.NewInteger(5)},
	}
	return &result.Page{Columns: cols, Rows: rows}
}

// settleColumnPage wires and settles one successful execution of page as the
// displayed first page.
func settleColumnPage(t *testing.T, page *result.Page) Model {
	t.Helper()
	return resultModel(t, page, nil)
}

// dotKey and commaKey build the portable printable horizontal bindings.
func dotKey() tea.KeyMsg   { return key('.') }
func commaKey() tea.KeyMsg { return key(',') }

// rawShiftPageMsg is a deterministic stand-in for the unknown-CSI message the
// terminal driver reports for the raw xterm shift+Page Down/Up sequences
// (ESC[6;2~ and ESC[5;2~). It carries the same String representation the
// production bridge matches on.
type rawShiftPageMsg struct{ down bool }

func (x rawShiftPageMsg) String() string {
	if x.down {
		return "?CSI[54 59 50 126]?"
	}
	return "?CSI[53 59 50 126]?"
}

// TestShiftPageBridgeFromRawCSI requires the xterm shift+Page CSI sequences,
// which bubbletea reports as unknown CSI messages instead of KeyMsgs, to
// bridge onto the same one-column bindings.
func TestShiftPageBridgeFromRawCSI(t *testing.T) {
	m := settleColumnPage(t, fiveColumnPage())
	next, cmd := pressKey(m, rawShiftPageMsg{down: true})
	if cmd != nil || next.firstColumn != 1 {
		t.Fatalf("raw shift+Page Down bridge = (firstColumn %d, cmd %v), want exactly one column forward", next.firstColumn, cmd)
	}
	next, cmd = pressKey(next, rawShiftPageMsg{down: false})
	if cmd != nil || next.firstColumn != 0 {
		t.Fatalf("raw shift+Page Up bridge = (firstColumn %d, cmd %v), want exactly one column back", next.firstColumn, cmd)
	}
}

// TestHorizontalKeysMoveExactlyOneColumn presses each of the four bindings
// and requires an exact one-column move per accepted press.
func TestHorizontalKeysMoveExactlyOneColumn(t *testing.T) {
	bindings := []struct {
		name  string
		key   tea.Msg
		delta int
	}{
		{"`.`", dotKey(), 1},
		{"Shift+Page Down", ShiftPageMsg{Down: true}, 1},
		{"`,`", commaKey(), -1},
		{"Shift+Page Up", ShiftPageMsg{Down: false}, -1},
	}
	for _, b := range bindings {
		t.Run(b.name, func(t *testing.T) {
			m := settleColumnPage(t, fiveColumnPage())
			if b.delta < 0 {
				// Retreat only makes sense from a column beyond the first;
				// step forward once so the retreat move is accepted.
				m = drive(m, dotKey()).(Model)
			}
			next, cmd := pressKey(m, b.key)
			if cmd != nil {
				t.Fatalf("%s issued a command for a local horizontal move", b.name)
			}
			want := b.delta
			if b.delta < 0 {
				want = 0 // one whole column back from index 1
			}
			if next.firstColumn != want {
				t.Fatalf("%s moved first column to %d, want exactly %d", b.name, next.firstColumn, want)
			}
			// Repeating the press keeps moving exactly one column at a time,
			// with the boundary pressing consumed as a no-op.
			next, _ = pressKey(next, b.key)
			wantAfterRepeat := 2 * b.delta
			if b.delta < 0 {
				wantAfterRepeat = 0 // the second retreat is a boundary no-op
			}
			if next.firstColumn != wantAfterRepeat {
				t.Fatalf("second %s moved first column to %d, want exactly %d", b.name, next.firstColumn, wantAfterRepeat)
			}
		})
	}
}

// TestHorizontalMovementRendersNewColumns reuses the pure layout seam: after
// an accepted move the grid renders starting at the new first-visible
// column, with widths recomputed from that column. At a 100-column terminal
// only two of the five 32-wide columns fit, so moving one column hides c1
// and reveals c3.
func TestHorizontalMovementRendersNewColumns(t *testing.T) {
	page := &result.Page{
		Columns: []string{
			"c1-32-char-wide-header-padding-x!",
			"c2-32-char-wide-header-padding-x!",
			"c3-32-char-wide-header-padding-x!",
			"c4-32-char-wide-header-padding-x!",
			"c5-32-char-wide-header-padding-x!",
		},
		Rows: [][]result.Value{
			{result.NewInteger(1), result.NewInteger(2), result.NewInteger(3), result.NewInteger(4), result.NewInteger(5)},
		},
	}
	m := sized(settleColumnPage(t, page), 100, 24).(Model)
	before := m.View()
	if !strings.Contains(before, "c1-32") || strings.Contains(before, "c3-32") {
		t.Fatalf("setup: initial view shows the wrong columns\n%s", before)
	}
	m = sized(drive(m, dotKey()), 100, 24).(Model)
	after := m.View()
	if strings.Contains(after, "c1-32") {
		t.Errorf("moved view still renders the first column:\n%s", after)
	}
	if !strings.Contains(after, "c3-32") {
		t.Errorf("moved view does not render the newly visible column:\n%s", after)
	}
}

func TestHorizontalKeysNoOpAtBoundaries(t *testing.T) {
	m := settleColumnPage(t, fiveColumnPage())

	// At the first column both retreat keys are consumed without effect.
	next, cmd := pressKey(m, commaKey())
	if cmd != nil || next.firstColumn != 0 {
		t.Fatalf("`,` at the first column = (%d, cmd %v), want a no-op", next.firstColumn, cmd)
	}
	next, cmd = pressKey(m, ShiftPageMsg{Down: false})
	if cmd != nil || next.firstColumn != 0 {
		t.Fatalf("Shift+Page Up at the first column = (%d, cmd %v), want a no-op", next.firstColumn, cmd)
	}

	// At the last column both advance keys are consumed without effect.
	last := next
	for last.firstColumn != len(last.Result.Page.Columns)-1 {
		last, _ = pressKey(last, dotKey())
	}
	next, cmd = pressKey(last, dotKey())
	if cmd != nil || next.firstColumn != last.firstColumn {
		t.Fatalf("`.` at the last column = (%d, cmd %v), want a no-op", next.firstColumn, cmd)
	}
	next, cmd = pressKey(last, ShiftPageMsg{Down: true})
	if cmd != nil || next.firstColumn != last.firstColumn {
		t.Fatalf("Shift+Page Down at the last column = (%d, cmd %v), want a no-op", next.firstColumn, cmd)
	}
}

func TestHorizontalMovementIssuesNoDatabaseCommand(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, &fakeSelectExecutor{page: fiveColumnPage()}, pageExec)

	before := pageExec.issued
	next, cmd := pressKey(m, dotKey())
	if cmd != nil {
		t.Fatal("horizontal move returned a command")
	}
	if next.firstColumn != 1 {
		t.Fatalf("horizontal move landed at %d, want 1", next.firstColumn)
	}
	if pageExec.issued != before {
		t.Fatalf("horizontal move dispatched page requests: %d -> %d", before, pageExec.issued)
	}
	if next.pagePending || next.firstPagePending || next.countPendingFlag {
		t.Fatal("horizontal move claimed a request slot")
	}
}

func TestHorizontalMovementStaysLocalWhileRequestsPending(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) Model
	}{
		{"first page pending", func(t *testing.T) Model {
			return pendingFirstPage(t, &fakeSelectExecutor{page: fiveColumnPage()})
		}},
		{"later page pending", func(t *testing.T) Model {
			return pendingLaterPage(t, &fakeSelectExecutor{page: fiveColumnPage()}, &fakePageExecutor{rowsShown: 11})
		}},
		{"count pending", func(t *testing.T) Model {
			return pendingCountOnly(t, &fakeSelectExecutor{page: fiveColumnPage()}, &fakeCountExecutor{total: 7})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.build(t)
			for _, k := range []tea.Msg{dotKey(), commaKey(), ShiftPageMsg{Down: true}, ShiftPageMsg{Down: false}} {
				next, cmd := pressKey(m, k)
				if cmd != nil {
					t.Fatalf("horizontal key %v returned a command while %s pending", k, tc.name)
				}
				if next.inFlightNotice != "" {
					t.Fatalf("horizontal key %v produced gate feedback %q; movement must stay local", k, next.inFlightNotice)
				}
			}
			// With displayed columns the move is genuinely local. While the
			// first page itself is in flight there are no output columns yet,
			// so the press is a consumed no-op by the boundary arithmetic.
			if len(m.outputColumnNames()) > 0 {
				moved, _ := pressKey(m, dotKey())
				if moved.firstColumn != m.firstColumn+1 {
					t.Fatalf("`.` while %s pending did not move one column locally: %d -> %d",
						tc.name, m.firstColumn, moved.firstColumn)
				}
			} else if moved, _ := pressKey(m, dotKey()); moved.firstColumn != 0 {
				t.Fatalf("`.` moved the first column without output columns: %d", moved.firstColumn)
			}
		})
	}
}

func TestHorizontalKeysConsumedByHigherPrecedenceContexts(t *testing.T) {
	t.Run("quit confirmation", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		q := m.openQuitConfirmation()
		next, _ := pressKey(q.(Model), dotKey())
		if !next.quitConfirm {
			t.Fatal("`.` leaked out of the quit confirmation")
		}
		if next.firstColumn != 0 {
			t.Fatalf("quit confirmation moved the first column to %d", next.firstColumn)
		}
	})
	t.Run("popup overlay", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		m.installPopup(NewScrollOnlyPopup("column(s)", []PopupCandidate{{ID: "a", Display: "a"}}), nil)
		next, _ := pressKey(m, dotKey())
		if next.Popup == nil {
			t.Fatal("`.` closed the popup overlay")
		}
		if next.firstColumn != 0 {
			t.Fatalf("popup overlay moved the first column to %d", next.firstColumn)
		}
	})
	t.Run("focused input", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		m.ValuePrompt = NewValuePrompt("where", "where", "")
		next, _ := pressKey(m, dotKey())
		if next.firstColumn != 0 {
			t.Fatalf("focused input moved the first column to %d", next.firstColumn)
		}
		if got := next.ValuePrompt.Buffer(); got != "." {
			t.Fatalf("focused input buffer = %q, want the typed `.` consumed as input", got)
		}
	})
	t.Run("too-small screen", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		small := sized(drive(m), 79, 23).(Model)
		if !small.suspended {
			t.Fatal("setup: 79x23 did not suspend the shell")
		}
		next, _ := pressKey(small, dotKey())
		if !next.suspended || next.firstColumn != 0 {
			t.Fatalf("too-small screen handled `.`: suspended=%v firstColumn=%d", next.suspended, next.firstColumn)
		}
	})
	t.Run("terminal state", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		m.terminalState = TerminalReplaced
		next, _ := pressKey(m, dotKey())
		if next.firstColumn != 0 {
			t.Fatalf("terminal state moved the first column to %d", next.firstColumn)
		}
	})
}

func TestHorizontalClampsFirstColumnOnResize(t *testing.T) {
	t.Run("valid index preserved across widths", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		m = drive(m, dotKey(), dotKey()).(Model)
		m = sized(drive(m), 120, 24).(Model)
		if m.firstColumn != 2 {
			t.Fatalf("resize preserved firstColumn = %d, want 2", m.firstColumn)
		}
		m = sized(drive(m), 80, 24).(Model)
		if m.firstColumn != 2 {
			t.Fatalf("second resize preserved firstColumn = %d, want 2", m.firstColumn)
		}
	})
	t.Run("invalid index clamped to the last column on resize", func(t *testing.T) {
		m := settleColumnPage(t, fiveColumnPage())
		m.firstColumn = 9 // e.g. columns disappeared from an older view
		m = sized(drive(m), 80, 24).(Model)
		if m.firstColumn != 4 {
			t.Fatalf("resize clamped firstColumn = %d, want 4", m.firstColumn)
		}
	})
	t.Run("single-column result clamps to zero", func(t *testing.T) {
		page := &result.Page{Columns: []string{"only"}, Rows: [][]result.Value{{result.NewInteger(1)}}}
		m := resultModel(t, page, nil)
		m.firstColumn = 3
		m = sized(drive(m), 80, 24).(Model)
		if m.firstColumn != 0 {
			t.Fatalf("single-column result clamped firstColumn = %d, want 0", m.firstColumn)
		}
	})
	t.Run("empty columns clamp to zero", func(t *testing.T) {
		m := resultModel(t, &result.Page{Columns: nil, Rows: nil}, nil)
		m.firstColumn = 3
		m = sized(drive(m), 80, 24).(Model)
		if m.firstColumn != 0 {
			t.Fatalf("empty result clamped firstColumn = %d, want 0", m.firstColumn)
		}
	})
}
