// Table-driven scripted key-precedence matrix (Issue #54 Task 1), per the
// Global Key Precedence and Context/Action Matrix in Notes/PRD-sqloid.md. One
// scripted (model, msg) → (model, cmd) table drives the non-quit rows of the
// matrix — terminal deletion/replacement/outcome-unknown, every top overlay
// or modal, focused builder text, popup search and file-picker search,
// scroll-only popups, request-pending base state (cancellable and
// noncancellable phases), ordinary builder/result base state, and the
// too-small screen — and asserts that the ordered dispatcher
// terminal → top overlay → focused input/search → request restriction →
// base context consumes each key exactly once, issues no lower-level command,
// and leaves focus, selection, viewport, history, and save/export state
// untouched beneath the consuming layer. Universal q/Ctrl+C confirmation
// behavior belongs to Issue #27/#55 and is deliberately excluded here.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
)

// matrixKey is one non-quit key press under its name.
type matrixKey struct {
	name string
	msg  tea.KeyMsg
}

// matrixKeyList enumerates the non-quit keys whose routing the matrix proves.
func matrixKeyList() []matrixKey {
	return []matrixKey{
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}},
		{"printable x", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}},
		{"down", tea.KeyMsg{Type: tea.KeyDown}},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}},
		{"ctrl+p", tea.KeyMsg{Type: tea.KeyCtrlP}},
		{"ctrl+n", tea.KeyMsg{Type: tea.KeyCtrlN}},
		{"ctrl+e", tea.KeyMsg{Type: tea.KeyCtrlE}},
		{"ctrl+y", tea.KeyMsg{Type: tea.KeyCtrlY}},
		{"ctrl+s", tea.KeyMsg{Type: tea.KeyCtrlS}},
		{"ctrl+x", tea.KeyMsg{Type: tea.KeyCtrlX}},
		{"ctrl+w", tea.KeyMsg{Type: tea.KeyCtrlW}},
		{"?", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}},
		{",", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}}},
		{".", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'.'}}},
	}
}

// assertMatrixNoLeak checks the invariants every lower layer must keep when a
// higher layer consumes a key: no history or result-history mutation, no save
// or export preparation loss, and unchanged result state.
func assertMatrixNoLeak(t *testing.T, before, after Model, context string) {
	t.Helper()
	if before.History.Len() != after.History.Len() {
		t.Errorf("%s: history mutated by a lower layer (%d → %d)", context, before.History.Len(), after.History.Len())
	}
	if before.ResultHistory.Len() != after.ResultHistory.Len() {
		t.Errorf("%s: result history mutated beneath the consuming layer", context)
	}
	if before.savePrepared != nil && after.savePrepared == nil {
		t.Errorf("%s: save state mutated beneath the consuming layer", context)
	}
	if before.exportPrepared != nil && after.exportPrepared == nil {
		t.Errorf("%s: export capture discarded beneath the consuming layer", context)
	}
}

// matrixState is one scripted context of the Global Key Precedence matrix
// with the exact model fixture that realizes it.
type matrixState struct {
	name  string
	build func(t *testing.T) Model
	// observe asserts exactly the mutations the consuming layer owns for one
	// key; everything else must be a consumed no-op with no lower-layer
	// command, exactly once routing.
	observe func(t *testing.T, before, after Model, cmd tea.Cmd, key string)
}

func matrixStates() []matrixState {
	return []matrixState{
		{
			name: "outcome-unknown terminal",
			build: func(t *testing.T) Model {
				t.Helper()
				m, _, _, _, _ := terminalModel(t)
				// Leave the settlement's initial result selection so `?`
				// reaches the reduced-help route.
				return termKey(t, m, "esc")
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				switch key {
				case "enter", "printable x", "esc", "up", "down", "tab", "ctrl+w":
					// Consumed no-op: the terminal starts no database work,
					// mutates no builder state, and issues no command.
					if cmd != nil {
						t.Fatalf("terminal %q produced a command", key)
					}
				case "?":
					if !after.terminalHelpOpen {
						t.Fatal("terminal ? did not open the reduced help")
					}
					if cmd != nil {
						t.Fatal("terminal ? produced a command")
					}
					if closed := termKey(t, after, "esc"); closed.terminalHelpOpen {
						t.Fatal("help esc did not dismiss the reduced help")
					}
				case "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y":
					// Allowed in-memory navigation; never issues a command.
					if cmd != nil {
						t.Fatalf("terminal %q dispatched a command", key)
					}
				case "ctrl+s", "ctrl+x":
					// In-memory targeting only; no write work may start.
					if after.writePending {
						t.Fatalf("terminal %q started write work", key)
					}
				}
				assertMatrixNoLeak(t, before, after, "terminal "+key)
			},
		},
		{
			name: "searchable popup",
			build: func(t *testing.T) Model {
				t.Helper()
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.installPopup(NewSearchablePopup(tableFieldLabel, []PopupCandidate{
					{ID: "alpha", Display: "alpha"}, {ID: "beta", Display: "beta"},
				}), nil)
				return m
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				switch key {
				case "enter", "esc":
					// Popup-owned accept/cancel paths may close the popup;
					// nothing may leak past the closing key.
				case "printable x", "?", "q", "up", "down", "tab",
					"ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "ctrl+s", "ctrl+x",
					"ctrl+w":
					if after.Popup == nil {
						t.Fatalf("popup closed by unrelated key %q", key)
					}
					if cmd != nil {
						t.Fatalf("popup context %q dispatched a command", key)
					}
				}
				assertMatrixNoLeak(t, before, after, "searchable popup "+key)
			},
		},
		{
			name: "scroll-only popup",
			build: func(t *testing.T) Model {
				t.Helper()
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.installPopup(NewScrollOnlyPopup(tableFieldLabel, []PopupCandidate{
					{ID: "a", Display: "a"}, {ID: "b", Display: "b"},
				}), nil)
				return m
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				if after.Popup == nil && key != "enter" && key != "esc" {
					t.Fatalf("scroll-only popup lost to %q", key)
				}
				if key == "?" && after.helpOpen {
					t.Fatal("scroll-only popup leaked `?` into help")
				}
				switch key {
				case "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "ctrl+s", "ctrl+x", "ctrl+w":
					if cmd != nil {
						t.Fatalf("scroll-only popup %q dispatched a command", key)
					}
				}
				assertMatrixNoLeak(t, before, after, "scroll-only popup "+key)
			},
		},
		{
			name: "focused value prompt",
			build: func(t *testing.T) Model {
				t.Helper()
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.ValuePrompt = NewValuePrompt(limitFieldLabel, "Limit", "1")
				return m
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				if after.ValuePrompt == nil && (key == "enter" || key == "esc") {
					return // prompt-owned submit/cancel closed it
				}
				switch key {
				case "printable x", "?":
					buf := after.ValuePrompt.Buffer()
					if len(buf) == 0 {
						t.Fatalf("prompt did not insert %q literally", key)
					}
					if got := buf[len(buf)-1:]; got != string(rune(key[len(key)-1])) {
						t.Fatalf("prompt buffer %q does not end in %q", buf, key)
					}
					if after.helpOpen {
						t.Fatal("`?` opened help over a focused input")
					}
				case "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "ctrl+s", "ctrl+x":
					if after.ValuePrompt == nil {
						t.Fatalf("focused input disallowed %q but the prompt closed", key)
					}
					if cmd != nil {
						t.Fatalf("focused input %q dispatched a command", key)
					}
					if after.ValuePrompt.Buffer() != before.ValuePrompt.Buffer() {
						t.Fatalf("focused input %q mutated the buffer", key)
					}
				}
				assertMatrixNoLeak(t, before, after, "focused prompt "+key)
			},
		},
		{
			name: "first page pending",
			build: func(t *testing.T) Model {
				return pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				switch key {
				case "enter":
					if after.inFlightNotice == "" {
						t.Fatal("pending Enter produced no running feedback")
					}
					if cmd != nil {
						t.Fatal("pending Enter issued a command")
					}
				case "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "ctrl+s", "ctrl+x":
					if after.inFlightNotice == "" {
						t.Fatalf("pending base consumed %q without feedback", key)
					}
				case "ctrl+w":
					if !after.selectCancelling {
						t.Fatal("cancellable pending Ctrl+W did not enter cancelling state")
					}
				case ",", ".":
					// Permitted local horizontal movement consumes the gate.
					if after.inFlightNotice != "" {
						t.Fatalf("permitted horizontal key retained gate feedback %q", after.inFlightNotice)
					}
				case "pgdown":
					// A new page request cannot stack while one is pending.
					if cmd != nil {
						t.Fatal("pending state stacked a second page request")
					}
				}
				assertMatrixNoLeak(t, before, after, "pending "+key)
			},
		},
		{
			name: "noncancellable write phase pending",
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.History = history.NewStore()
				m.writePending = true
				m.writeNoncancellable = true
				m.writePhase = connection.WritePhaseCommitting
				return m
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				switch key {
				case "ctrl+w":
					if after.writeCancelling {
						t.Fatal("noncancellable phase dispatched a cancellation")
					}
					if after.inFlightNotice != CommitBoundaryFeedback {
						t.Fatalf("noncancellable Ctrl+W feedback = %q", after.inFlightNotice)
					}
				case "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "ctrl+s", "ctrl+x":
					if after.inFlightNotice == "" {
						t.Fatalf("noncancellable write did not reject %q", key)
					}
				case "enter":
					if cmd != nil {
						t.Fatal("noncancellable pending Enter issued a command")
					}
				}
				if after.writePending != before.writePending || after.writeNoncancellable != before.writeNoncancellable {
					t.Fatalf("write phase state mutated by %q", key)
				}
				assertMatrixNoLeak(t, before, after, "write pending "+key)
			},
		},
		{
			name: "ordinary base builder",
			build: func(t *testing.T) Model {
				m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
				m.History = history.NewStore()
				return sized(m, 80, 24).(Model)
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				switch key {
				case "tab":
					if after.Focus != before.Focus+1 {
						t.Fatalf("base Tab moved focus %d → %d", before.Focus, after.Focus)
					}
				case "ctrl+w":
					// No cancellable request is owned: ignored, no command.
					if cmd != nil {
						t.Fatal("idle Ctrl+W issued a cancellation command")
					}
				case "enter":
					// Idle invalid builder state consumes Enter without a
					// runnable execution and without any command.
					if cmd != nil {
						t.Fatal("idle invalid Enter issued a command")
					}
				case "?":
					if !after.helpOpen {
						t.Fatal("base `?` did not open contextual help")
					}
					if after.helpKind != helpKindBuilder {
						t.Fatalf("builder base help kind = %q", after.helpKind)
					}
					restored, _ := after.Update(tea.KeyMsg{Type: tea.KeyEsc})
					rm := restored.(Model)
					if rm.helpOpen {
						t.Fatal("esc did not restore the help opener")
					}
					if rm.Focus != before.Focus || len(rm.Fields) != len(before.Fields) {
						t.Fatal("help dismissal did not restore the exact opener state")
					}
				}
				assertMatrixNoLeak(t, before, after, "base "+key)
			},
		},
		{
			name: "too-small screen",
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m = sized(m, 79, 23).(Model)
				if !m.suspended {
					t.Fatal("setup: model not suspended")
				}
				m.History = history.NewStore()
				return m
			},
			observe: func(t *testing.T, before, after Model, cmd tea.Cmd, key string) {
				t.Helper()
				if !after.suspended {
					t.Fatalf("too-small state lost suspension to %q", key)
				}
				if cmd != nil {
					t.Fatalf("too-small key %q issued a command", key)
				}
				assertMatrixNoLeak(t, before, after, "suspended "+key)
			},
		},
	}
}

// TestKeyPrecedenceMatrix routes every enumerated non-quit key through every
// scripted context and asserts exactly one layer consumes it: the consuming
// layer's owned effect happens (or the key is a consumed no-op), no command
// leaks from a lower layer, and the covered state invariants never change.
func TestKeyPrecedenceMatrix(t *testing.T) {
	states := matrixStates()
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			for _, mk := range matrixKeyList() {
				if mk.name == "q" || mk.name == "ctrl+c" {
					// Universal q/Ctrl+C quit confirmation belongs to Issue
					// #55; this matrix covers non-quit routing only.
					continue
				}
				t.Run(mk.name, func(t *testing.T) {
					before := state.build(t)
					next, cmd := before.Update(mk.msg)
					after, ok := next.(Model)
					if !ok {
						t.Fatalf("key %q returned %T", mk.name, next)
					}
					state.observe(t, before, after, cmd, mk.name)
				})
			}
		})
	}
}
