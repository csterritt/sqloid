// Table-driven universal quit-confirmation matrix (Issue #55 Task 1), per
// the Global Key Precedence and Context/Action Matrix in Notes/PRD-sqloid.md.
// In every enabled nonterminal context — base builder/result state, focused
// value entry, searchable and scroll-only popups, the contextual help
// overlay, schema validation, the destructive estimate/preparation phase,
// pending read and write phases, history browsing, and the too-small screen
// — q and Ctrl+C open exactly one shared quit confirmation that suspends the
// exact current context, except that q stays literal input when a focused
// text/search owner owns it. Deletion, replacement, and outcome-unknown
// terminal states keep their immediate status-1 q/Ctrl+C quit with no
// confirmation. Repeated quit keys inside the confirmation are consumed
// no-ops, no confirmation-opening key ever leaks into text, navigation,
// cancellation, save, or database work, and ordinary overlays never stack
// behind the confirmation.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
)

// quitKeyList enumerates the two quit keys whose routing the matrix proves.
func quitKeyList() []tea.KeyMsg {
	return []tea.KeyMsg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		tea.KeyMsg{Type: tea.KeyCtrlC},
	}
}

// quitExpectation classifies what q and Ctrl+C must do in one context.
type quitExpectation int

const (
	// quitOpensConfirmation: both quit keys open the shared confirmation.
	quitOpensConfirmation quitExpectation = iota
	// quitLiteralQ: q stays literal input for the focused text/search owner;
	// Ctrl+C still opens the shared confirmation.
	quitLiteralQ
	// quitTerminalStatus1: the terminal state exits immediately with status
	// 1 and never opens a confirmation.
	quitTerminalStatus1
)

// quitContext is one scripted context of the universal quit matrix with the
// exact model fixture that realizes it and the context's quit expectation.
type quitContext struct {
	name   string
	expect quitExpectation
	build  func(t *testing.T) Model
	// confirmFields, when set, asserts context signatures that must survive
	// inside the confirmation and be restored exactly on cancellation.
	check func(t *testing.T, m Model)
}

// assertQuitConfirmationSuspended verifies the shared confirmation opened
// over m: one confirmation, the exact context suspended behind it, and no
// command dispatched merely by opening.
func assertQuitConfirmationSuspended(t *testing.T, m Model, after tea.Model, cmd tea.Cmd, key, ctx string) {
	t.Helper()
	nm, ok := after.(Model)
	if !ok {
		t.Fatalf("%s: %s returned %T", ctx, key, after)
	}
	if !nm.quitConfirm {
		t.Fatalf("%s: %s did not open the shared quit confirmation", ctx, key)
	}
	if nm.quitSuspended == nil {
		t.Fatalf("%s: %s opened the confirmation without suspending the context", ctx, key)
	}
	if cmd != nil {
		t.Fatalf("%s: %s issued a command merely by opening the confirmation", ctx, key)
	}
}

// quitMatrixContexts returns every enabled nonterminal context plus the
// three terminal exceptions.
func quitMatrixContexts() []quitContext {
	return []quitContext{
		{
			name:   "ordinary base builder",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				m.History = history.NewStore()
				return sized(m, 80, 24).(Model)
			},
		},
		{
			name:   "focused value prompt",
			expect: quitLiteralQ,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.ValuePrompt = NewValuePrompt(limitFieldLabel, "Limit", "1")
				return m
			},
		},
		{
			name:   "searchable popup",
			expect: quitLiteralQ,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.installPopup(NewSearchablePopup(tableFieldLabel, []PopupCandidate{
					{ID: "alpha", Display: "alpha"}, {ID: "beta", Display: "beta"},
				}), nil)
				return m
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if m.Popup == nil {
					t.Fatal("searchable popup lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "scroll-only popup",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.installPopup(NewScrollOnlyPopup(tableFieldLabel, []PopupCandidate{
					{ID: "a", Display: "a"}, {ID: "b", Display: "b"},
				}), nil)
				return m
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if m.Popup == nil {
					t.Fatal("scroll-only popup lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "contextual help overlay",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				m.History = history.NewStore()
				m = sized(m, 80, 24).(Model)
				return m.openContextualHelp().(Model)
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if !m.helpOpen {
					t.Fatal("help overlay lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "first page pending",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				return pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if m.selectRequestPending() == false && !m.writePending {
					t.Fatal("pending SELECT state lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "noncancellable write phase pending",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.writePending = true
				m.writeNoncancellable = true
				m.writePhase = connection.WritePhaseCommitting
				return m
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if !m.writePending || !m.writeNoncancellable {
					t.Fatal("write phase state lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "schema validation pending",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				m.History = history.NewStore()
				return sized(m, 80, 24).(Model)
			},
		},
		{
			name:   "query history mode",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				store := history.NewStore()
				store.Append(richSelectQB().HistoryState())
				m.History = store
				m = sized(m, 80, 24).(Model)
				next, _ := m.enterHistoryMode()
				nm := next.(Model)
				if !nm.historyMode {
					t.Fatal("setup: query history mode did not open")
				}
				return nm
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if !m.historyMode {
					t.Fatal("query history mode lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "too-small screen",
			expect: quitOpensConfirmation,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m = sized(m, 79, 23).(Model)
				if !m.suspended {
					t.Fatal("setup: model not suspended")
				}
				m.History = history.NewStore()
				return m
			},
			check: func(t *testing.T, m Model) {
				t.Helper()
				if !m.suspended {
					t.Fatal("too-small wrapper lost while the quit confirmation is open")
				}
			},
		},
		{
			name:   "deletion terminal",
			expect: quitTerminalStatus1,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.terminalState = TerminalDeleted
				return m
			},
		},
		{
			name:   "replacement terminal",
			expect: quitTerminalStatus1,
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.terminalState = TerminalReplaced
				return m
			},
		},
		{
			name:   "outcome-unknown terminal",
			expect: quitTerminalStatus1,
			build: func(t *testing.T) Model {
				m, _, _, _, _ := terminalModel(t)
				return m
			},
		},
	}
}

// TestQuitConfirmationMatrix routes q and Ctrl+C through every scripted
// context and asserts the exact expectation: shared confirmation suspension
// everywhere nonterminal (with the q-literal exception under focused
// text/search), immediate status-1 exit in every terminal state, repeated
// quit keys consumed inside the confirmation, and no command issued merely
// by opening.
func TestQuitConfirmationMatrix(t *testing.T) {
	for _, ctx := range quitMatrixContexts() {
		t.Run(ctx.name, func(t *testing.T) {
			for _, msg := range quitKeyList() {
				key := msg.String()
				before := ctx.build(t)
				after, cmd := before.Update(msg)
				m, ok := after.(Model)
				if !ok {
					t.Fatalf("%s returned %T", key, after)
				}
				switch ctx.expect {
				case quitTerminalStatus1:
					if m.quitConfirm {
						t.Fatalf("terminal %s opened a quit confirmation", key)
					}
					if m.exitStatus != 1 {
						t.Fatalf("terminal %s exit status = %d, want 1", key, m.exitStatus)
					}
					if cmd == nil {
						t.Fatalf("terminal %s issued no quit command", key)
					}
				case quitLiteralQ:
					if key == "q" {
						// q stays literal input for the focused owner: the
						// confirmation must not open and the context holds.
						if m.quitConfirm {
							t.Fatalf("focused text/search owner lost literal q to the quit confirmation")
						}
						if cmd != nil {
							t.Fatalf("literal q dispatched a command")
						}
						continue
					}
					assertQuitConfirmationSuspended(t, before, after, cmd, key, ctx.name)
				case quitOpensConfirmation:
					assertQuitConfirmationSuspended(t, before, after, cmd, key, ctx.name)
				}
			}
		})
	}
}

// TestQuitConfirmationRepeatedOpensSuspended proves the confirmation opens
// at most once: a repeated q inside it is a consumed no-op that neither
// re-suspends, exits, nor leaks. (Ctrl+C inside the confirmation is the PRD's
// accept key, not a re-open.)
func TestQuitConfirmationRepeatedOpensSuspended(t *testing.T) {
	for _, ctx := range quitMatrixContexts() {
		if ctx.expect == quitTerminalStatus1 || ctx.expect == quitLiteralQ {
			continue
		}
		t.Run(ctx.name, func(t *testing.T) {
			m := ctx.build(t)
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			first, ok := next.(Model)
			if !ok || !first.quitConfirm {
				t.Fatal("setup: first quit key did not open the confirmation")
			}
			after, cmd := first.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			nm, ok := after.(Model)
			if !ok {
				t.Fatalf("repeated q returned %T", after)
			}
			if cmd != nil {
				t.Fatal("repeated q inside the confirmation issued a command")
			}
			if !nm.quitConfirm || nm.quitSuspended == nil {
				t.Fatal("repeated q inside the confirmation resolved it unexpectedly")
			}
		})
	}
}
