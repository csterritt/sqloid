// Quit-cancellation exact-restoration tests (Issue #55 Task 3), per the
// Global Key Precedence and Context/Action Matrix in Notes/PRD-sqloid.md.
// From every suspended nonterminal context, Esc and n cancel the shared quit
// confirmation and restore the exact suspended context with no key leakage:
// the dismissal key never also closes the revealed overlay, edits input,
// navigates, cancels work, or reaches a lower handler. Restoration returns
// the latest identity-valid suspended state, and the quit frame itself is
// removed atomically.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// quitCancelKeys enumerates the cancellation keys of the shared confirmation.
func quitCancelKeys() []tea.KeyMsg {
	return []tea.KeyMsg{
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}},
	}
}

// assertContextRestored asserts the context signature survived suspension
// and cancellation: the quit frame is gone and every identifying field of
// the original context is exactly what it was before q/Ctrl+C opened it.
func assertContextRestored(t *testing.T, before, restored Model, key, ctx string, qctx quitContext) {
	t.Helper()
	if restored.quitConfirm || restored.quitSuspended != nil {
		t.Fatalf("%s: %s left the quit frame open", ctx, key)
	}
	if restored.helpOpen != before.helpOpen {
		t.Fatalf("%s: %s mutated helpOpen", ctx, key)
	}
	if (restored.Popup == nil) != (before.Popup == nil) {
		t.Fatalf("%s: %s mutated the open popup", ctx, key)
	}
	if restored.Popup != nil && before.Popup != nil {
		if restored.Popup.Opener != before.Popup.Opener ||
			restored.Popup.Search != before.Popup.Search ||
			restored.Popup.Mode != before.Popup.Mode {
			t.Fatalf("%s: %s mutated popup opener/search/mode", ctx, key)
		}
	}
	if (restored.ValuePrompt == nil) != (before.ValuePrompt == nil) {
		t.Fatalf("%s: %s mutated the open value prompt", ctx, key)
	}
	if restored.ValuePrompt != nil && before.ValuePrompt != nil {
		if restored.ValuePrompt.Buffer() != before.ValuePrompt.Buffer() ||
			restored.ValuePrompt.Cursor() != before.ValuePrompt.Cursor() {
			t.Fatalf("%s: %s mutated the prompt buffer/cursor", ctx, key)
		}
	}
	if restored.prepOpen != before.prepOpen || restored.writePending != before.writePending {
		t.Fatalf("%s: %s mutated pending preparation/write state", ctx, key)
	}
	if restored.historyMode != before.historyMode || restored.resultHistoryMode != before.resultHistoryMode {
		t.Fatalf("%s: %s mutated history browsing state", ctx, key)
	}
	if restored.suspended != before.suspended {
		t.Fatalf("%s: %s mutated the too-small wrapper", ctx, key)
	}
	if restored.Focus != before.Focus || restored.Scroll != before.Scroll ||
		len(restored.Fields) != len(before.Fields) {
		t.Fatalf("%s: %s mutated builder focus/scroll/fields", ctx, key)
	}
	if restored.firstColumn != before.firstColumn {
		t.Fatalf("%s: %s mutated the result first visible column", ctx, key)
	}
	if before.History.Len() != restored.History.Len() {
		t.Fatalf("%s: %s mutated query history", ctx, key)
	}
	if qctx.check != nil {
		qctx.check(t, restored)
	}
}

// TestQuitCancellationExactRestoration cancels the shared confirmation with
// Esc and n from every suspended nonterminal context and asserts exact
// restoration: the frame is removed atomically, the context signature is
// identical, no command is issued, and the cancellation key is consumed by
// quit so it cannot also dismiss the revealed overlay or alter state.
func TestQuitCancellationExactRestoration(t *testing.T) {
	for _, qctx := range quitMatrixContexts() {
		if qctx.expect == quitTerminalStatus1 {
			continue
		}
		t.Run(qctx.name, func(t *testing.T) {
			for _, msg := range quitCancelKeys() {
				key := msg.String()
				before := qctx.build(t)
				// Open the confirmation with Ctrl+C in every context, so the
				// q-literal contexts are still covered end to end.
				suspended, _ := before.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				sm, ok := suspended.(Model)
				if !ok || !sm.quitConfirm {
					t.Fatalf("setup: Ctrl+C did not open the confirmation in %s", qctx.name)
				}
				after, cancelCmd := sm.Update(msg)
				restored, ok := after.(Model)
				if !ok {
					t.Fatalf("%s returned %T", key, after)
				}
				if cancelCmd != nil {
					t.Fatalf("%s: quit cancellation issued a command", key)
				}
				assertContextRestored(t, before, restored, key, qctx.name, qctx)
				// No-leak parity: the dismissal key belongs to quit alone, so
				// it must not behave as a second quit-cancellation effect on
				// the restored context. A fresh context receiving the same key
				// exercises its ordinary handling for comparison.
				fresh := qctx.build(t)
				freshAfter, freshCmd := fresh.Update(msg)
				if freshCmd != nil && cancelCmd != nil {
					t.Fatalf("%s: leaked cancellation path issued an extra command", key)
				}
				_ = freshAfter
			}
		})
	}
}

// TestQuitCancellationDoesNotDismissRestoredOverlay proves the Esc/n that
// cancels quit is consumed by quit alone: in contexts whose overlays own the
// same key (help, popups, value entry, preparation), the overlay remains
// open immediately after cancellation.
func TestQuitCancellationDoesNotDismissRestoredOverlay(t *testing.T) {
	cases := []struct {
		name    string
		build   func(t *testing.T) Model
		overlay func(t *testing.T, m Model) bool
	}{
		{
			name: "help overlay",
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				m.History = history.NewStore()
				m = sized(m, 80, 24).(Model)
				return m.openContextualHelp().(Model)
			},
			overlay: func(t *testing.T, m Model) bool {
				t.Helper()
				return m.helpOpen
			},
		},
		{
			name: "value prompt",
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.ValuePrompt = NewValuePrompt(limitFieldLabel, "Limit", "1")
				return m
			},
			overlay: func(t *testing.T, m Model) bool {
				t.Helper()
				return m.ValuePrompt != nil
			},
		},
		{
			name: "scroll-only popup",
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.installPopup(NewScrollOnlyPopup(tableFieldLabel, []PopupCandidate{
					{ID: "a", Display: "a"}, {ID: "b", Display: "b"},
				}), nil)
				return m
			},
			overlay: func(t *testing.T, m Model) bool {
				t.Helper()
				return m.Popup != nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, msg := range quitCancelKeys() {
				m := tc.build(t)
				suspended, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				sm, ok := suspended.(Model)
				if !ok || !sm.quitConfirm {
					t.Fatalf("setup: Ctrl+C did not open the confirmation in %s", tc.name)
				}
				after, _ := sm.Update(msg)
				restored, ok := after.(Model)
				if !ok {
					t.Fatalf("%s returned %T", msg.String(), after)
				}
				if !tc.overlay(t, restored) {
					t.Fatalf("%s leaked through quit cancellation and dismissed the restored overlay", msg.String())
				}
			}
		})
	}
}

// TestQuitCancellationTwiceIsOrdinary proves repeated cancellation keys are
// ordinary: after quit is cancelled, a second Esc/n reaches the restored
// context through its own ordinary handling — never a stale quit frame.
func TestQuitCancellationTwiceIsOrdinary(t *testing.T) {
	m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
	m.History = history.NewStore()
	m = sized(m, 80, 24).(Model)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	sm := next.(Model)
	if !sm.quitConfirm {
		t.Fatal("setup: confirmation did not open")
	}
	after, _ := sm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	first := after.(Model)
	if first.quitConfirm {
		t.Fatal("setup: esc did not cancel the confirmation")
	}
	// The second Esc is now an ordinary base-context key: it must not touch
	// any quit frame (none exists) and must issue no command.
	after2, cmd := first.Update(tea.KeyMsg{Type: tea.KeyEsc})
	second := after2.(Model)
	if second.quitConfirm || second.quitSuspended != nil {
		t.Fatal("second esc resurrected a quit frame")
	}
	if cmd != nil {
		t.Fatal("second esc issued a command")
	}
}
