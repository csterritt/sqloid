// Reduced-help and immediate-quit coverage for Issue #46, per the Global Key
// Precedence and Context/Action matrix decisions in Notes/PRD-sqloid.md. In
// both health terminals `?` opens a reduced help listing only the in-memory
// history selection, help dismissal, and the immediate quit keys — never an
// execution, refresh, paging, rerun, cancellation, or any other database
// suggestion (save/export integration stays owned by Issues #48/#49). From
// the primary message, either history selection, and the reduced help, `q`
// and Ctrl+C exit immediately with status 1, bypass confirmation, and
// schedule no cancellation, cleanup, connection, or delayed command.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// helpTerminalFixture enters both health terminal variants (populated or
// empty histories) ready for help/quit coverage, returning at the primary
// message view (any entry-time result selection dismissed with Esc).
func helpTerminalFixture(t *testing.T, want TerminalState, empty bool) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
	t.Helper()
	var err error
	if want == TerminalReplaced {
		err = replacedHealthErr()
	} else {
		err = deletedHealthErr()
	}
	m, sel, count, page, refresh := healthTerminalModel(t, err, want)
	m = termKey(t, m, "esc")
	if empty {
		m.ResultHistory = history.NewResultStore()
		m.History = history.NewStore()
		m.resultHistoryMode = false
		m.resultHistoryCursorID = 0
		m.resultHistoryView = nil
	}
	return m, sel, count, page, refresh
}

// TestHealthTerminalReducedHelpListsOnlyInMemoryActions proves `?` opens a
// reduced help whose contents cover only the actions actually available in
// the terminal states, with no database suggestion.
func TestHealthTerminalReducedHelpListsOnlyInMemoryActions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  TerminalState
		empty bool
	}{
		{name: "deleted populated", want: TerminalDeleted, empty: false},
		{name: "replaced populated", want: TerminalReplaced, empty: false},
		{name: "deleted empty", want: TerminalDeleted, empty: true},
		{name: "replaced empty", want: TerminalReplaced, empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, sel, count, page, refresh := helpTerminalFixture(t, tc.want, tc.empty)
			m = termKey(t, m, "?")
			if !m.terminalHelpOpen {
				t.Fatal("? did not open the reduced help")
			}
			view := m.View()
			for _, want := range []string{
				"Reduced help",
				"Ctrl+P / Ctrl+N",
				"Ctrl+E / Ctrl+Y",
				"Esc",
				"quit immediately (status 1)",
			} {
				if !strings.Contains(view, want) {
					t.Errorf("reduced help lacks %q:\n%s", want, view)
				}
			}
			for _, forbidden := range []string{
				"refresh", "Refresh", "rerun", "Rerun",
				"cancel command", "Cancel command",
				"Execute", "execute", "page results",
				"Select a command",
			} {
				if strings.Contains(view, forbidden) {
					t.Errorf("reduced help suggests %q — no database suggestion may appear:\n%s", forbidden, view)
				}
			}
			// The exact terminal message stays the primary view line.
			wantMessage := DeletedSessionEndedMessage
			if tc.want == TerminalReplaced {
				wantMessage = ReplacedSessionEndedMessage
			}
			if !strings.HasPrefix(view, wantMessage) {
				t.Errorf("help view does not lead with the exact terminal message:\n%s", view)
			}
			// Esc dismisses help and restores the message.
			m = termKey(t, m, "esc")
			if m.terminalHelpOpen {
				t.Fatal("esc did not dismiss the help")
			}
			if view := m.View(); !strings.Contains(view, wantMessage) {
				t.Errorf("help dismissal lost the terminal message:\n%s", view)
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "reduced help")
		})
	}
}

// TestHealthTerminalImmediateQuitFromEveryContext proves q and Ctrl+C exit
// immediately with status 1 from the primary message, selected history, and
// reduced help — bypassing confirmation and scheduling no work.
func TestHealthTerminalImmediateQuitFromEveryContext(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  TerminalState
		empty bool
	}{
		{name: "deleted populated", want: TerminalDeleted, empty: false},
		{name: "replaced populated", want: TerminalReplaced, empty: false},
		{name: "deleted empty", want: TerminalDeleted, empty: true},
		{name: "replaced empty", want: TerminalReplaced, empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// From the primary message.
			m, sel, count, page, refresh := helpTerminalFixture(t, tc.want, tc.empty)
			m = termKey(t, m, "esc")
			pressQuit(t, m, "q")
			pressQuit(t, m, "ctrl+c")
			requireNoDatabaseWork(t, sel, count, page, refresh, "quit from primary")

			// From selected query history (populated only: empty history is a
			// no-op fallback covered elsewhere).
			if !tc.empty {
				m, sel, count, page, refresh = helpTerminalFixture(t, tc.want, tc.empty)
				m = termKey(t, m, "ctrl+p")
				if !m.historyMode {
					t.Fatal("setup: query-history browsing did not open")
				}
				pressQuit(t, m, "q")
				requireNoDatabaseWork(t, sel, count, page, refresh, "quit from query history")

				// From selected result history.
				m, sel, count, page, refresh = helpTerminalFixture(t, tc.want, tc.empty)
				m = termKey(t, m, "ctrl+e")
				if !m.resultHistoryMode {
					t.Fatal("setup: result-history browsing did not open")
				}
				pressQuit(t, m, "ctrl+c")
				requireNoDatabaseWork(t, sel, count, page, refresh, "quit from result history")
			}

			// From the reduced help.
			m, sel, count, page, refresh = helpTerminalFixture(t, tc.want, tc.empty)
			m = termKey(t, m, "?")
			if !m.terminalHelpOpen {
				t.Fatal("setup: reduced help did not open")
			}
			pressQuit(t, m, "q")
			pressQuit(t, m, "ctrl+c")
			requireNoDatabaseWork(t, sel, count, page, refresh, "quit from help")
		})
	}
}

// TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing asserts the
// immediate quit never opens the ordinary quit confirmation and leaves no
// cancellation, cleanup, or delayed command behind.
func TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing(t *testing.T) {
	m, _, _, _, _ := helpTerminalFixture(t, TerminalDeleted, false)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	nm := next.(Model)
	if cmd == nil {
		t.Fatal("q produced no exit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("q command produced %T, want tea.Quit", msg)
	}
	if nm.ExitStatus() != 1 {
		t.Errorf("exit status = %d, want 1", nm.ExitStatus())
	}
	if nm.quitConfirm || nm.quitSuspended != nil || nm.quitWaitWrite {
		t.Error("q scheduled a confirmation overlay or wait state")
	}
	if nm.CancelCommand != nil || nm.firstPageCancel != nil || nm.countCancel != nil || nm.pageRequestCancel != nil {
		t.Error("q scheduled a cancellation request")
	}
	if nm.inFlightNotice != "" {
		t.Error("q left in-flight feedback behind")
	}
}

// TestHealthTerminalHelpKeepsDatabaseWorkBlocked reasserts that while the
// reduced help is open every normally database-capable key remains blocked.
func TestHealthTerminalHelpKeepsDatabaseWorkBlocked(t *testing.T) {
	m, sel, count, page, refresh := helpTerminalFixture(t, TerminalReplaced, false)
	m = termKey(t, m, "?")
	for _, k := range []string{"enter", "s", "u", "d", "i", "x", "r", "pgup", "pgdown", "ctrl+w"} {
		next := termKey(t, m, k)
		if !next.terminalHelpOpen || next.terminalState != TerminalReplaced {
			t.Fatalf("key %q escaped the help overlay (open=%v state=%v)", k, next.terminalHelpOpen, next.terminalState)
		}
		m = next
	}
	requireNoDatabaseWork(t, sel, count, page, refresh, "help-open keys")
}
