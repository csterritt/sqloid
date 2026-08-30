// Immediate terminal quit coverage for Issue #45, per the Global Key
// Precedence matrix in Notes/PRD-sqloid.md: in the outcome-unknown terminal
// state `q` and Ctrl+C override every other context and exit immediately
// with process status 1 — no quit confirmation overlay, no cancellation
// request, no cleanup command, no database request, no delayed settlement,
// and no state restoration — because terminal entry already guarantees no
// transaction or driver work remains pending. Covered from the primary
// outcome-unknown view, selected query/result history, and the reduced
// help, with populated and empty histories and repeated keys.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// pressQuit feeds one quit key and asserts the immediate status-1 exit:
// the command is exactly tea.Quit, the model's exit status is 1, and no
// confirmation overlay, cancellation handle, or suspended state survives.
func pressQuit(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if key == "ctrl+c" {
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	}
	if next == nil {
		t.Fatal("quit key produced no model")
	}
	nm := next.(Model)
	if cmd == nil {
		t.Fatalf("key %q in the terminal state produced no exit command", key)
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("key %q exit command produced %T, want tea.Quit", key, msg)
	}
	if got := nm.ExitStatus(); got != 1 {
		t.Fatalf("exit status after %q = %d, want 1", key, got)
	}
	if nm.quitConfirm || nm.quitSuspended != nil {
		t.Errorf("key %q scheduled a quit confirmation overlay", key)
	}
	if nm.CancelCommand != nil || nm.writeCancel != nil {
		t.Errorf("key %q scheduled a cancellation request", key)
	}
	if nm.terminalHelpOpen || nm.historyMode || nm.resultHistoryMode {
		t.Errorf("key %q left terminal subview state behind instead of quitting", key)
	}
	return nm
}

func TestTerminalQuitFromPrimaryView(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "esc") // back to the primary outcome-unknown view
	m = pressQuit(t, m, "q")
	// A repeated key re-arms the same immediate exit; no second state change.
	pressQuit(t, m, "q")
	pressQuit(t, m, "ctrl+c")
}

func TestTerminalQuitFromQueryHistory(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "ctrl+p")
	if !m.historyMode {
		t.Fatal("setup: query-history browsing did not open")
	}
	pressQuit(t, m, "q")
}

func TestTerminalQuitFromResultHistory(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "ctrl+e")
	if !m.resultHistoryMode {
		t.Fatal("setup: result-history browsing did not open")
	}
	pressQuit(t, m, "ctrl+c")
}

func TestTerminalQuitFromHelp(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "esc")
	m = termKey(t, m, "?")
	if !m.terminalHelpOpen {
		t.Fatal("setup: reduced help did not open")
	}
	pressQuit(t, m, "q")
}

func TestTerminalQuitWithEmptyHistories(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "esc")
	m.History = history.NewStore()
	m.ResultHistory = history.NewResultStore()
	pressQuit(t, m, "q")
	pressQuit(t, m, "ctrl+c")
}
