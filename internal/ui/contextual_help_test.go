// Contextual `?` routing and required help-content tests (Issue #54 Tasks
// 3 and 5), per the Global Key Precedence and Context/Action Matrix in
// Notes/PRD-sqloid.md. Focused text/search inputs insert `?` literally at the
// cursor without opening help; eligible base contexts open one nonstacking
// contextual help overlay from an exact opener snapshot restored on Esc;
// overlays and the too-small screen consume `?` as a no-op; repeated `?`
// never stacks help. The exact semantic contents of the WHERE SQL-NULL
// guidance, the independent limited-result count help, and the reduced
// terminal help (available in-memory actions only, no database suggestion)
// are asserted verbatim.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

func qKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}} }

func escKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

// baseHelpFixture returns an ordinary base builder model with an empty
// history store and the Command field focused.
func baseHelpFixture(t *testing.T) Model {
	t.Helper()
	m := selectModel(&fakeVersionReader{}, &fakeRefresher{})
	m.History = history.NewStore()
	return sized(m, 80, 24).(Model)
}

// resultHelpFixture settles one first page so the displayed result carries
// the independent count state its help must explain.
func resultHelpFixture(t *testing.T) Model {
	t.Helper()
	return settledFirstPage(t, &fakeSelectExecutor{page: threeRowPage()},
		&fakePageExecutor{rowsShown: 11})
}

// TestQuestionMarkInsertsLiterallyInFocusedInputs drives `?` into every
// focused text/search component and asserts one literal character at the
// current cursor, no help opened, and nothing else changed.
func TestQuestionMarkInsertsLiterallyInFocusedInputs(t *testing.T) {
	t.Run("universal value prompt with mid-buffer cursor", func(t *testing.T) {
		m := baseHelpFixture(t)
		m.ValuePrompt = NewValuePrompt(limitFieldLabel, "Limit", "12")
		// Left once so insertion is observable mid-buffer.
		m.ValuePrompt.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
		next, _ := m.Update(qKey())
		pm := next.(Model)
		if pm.ValuePrompt == nil {
			t.Fatal("help opened over a focused input and closed the prompt")
		}
		if got := pm.ValuePrompt.Buffer(); got != "1?2" {
			t.Fatalf("buffer = %q, want the literal `?` inserted at the cursor", got)
		}
		if pm.ValuePrompt.Cursor() != 2 {
			t.Fatalf("cursor = %d, want just after the inserted `?`", pm.ValuePrompt.Cursor())
		}
		if pm.helpOpen {
			t.Fatal("focused `?` opened help instead of inserting")
		}
	})

	t.Run("searchable popup search", func(t *testing.T) {
		m := sized(New(), 80, 24).(Model)
		m.installPopup(NewSearchablePopup(tableFieldLabel, []PopupCandidate{
			{ID: "alpha", Display: "alpha"}, {ID: "beta", Display: "beta"},
		}), nil)
		next, _ := m.Update(qKey())
		pm := next.(Model)
		if pm.Popup == nil {
			t.Fatal("popup closed by `?`")
		}
		if pm.Popup.Search != "?" {
			t.Fatalf("search = %q, want one literal `?`", pm.Popup.Search)
		}
		if pm.helpOpen {
			t.Fatal("`?` opened help from popup search")
		}
	})

	t.Run("picker filename input", func(t *testing.T) {
		f := pickerNewFakeFS()
		f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
		_, p := savePickerModel(t, f)
		p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
		next, _ := p.Update(qKey())
		a := next.(Model)
		if !a.pickerOpen {
			t.Fatal("picker closed by `?` in filename focus")
		}
		if got := a.picker.Filename(); !strings.Contains(got, "?") {
			t.Fatalf("filename = %q, want a literal `?`", got)
		}
		if a.helpOpen {
			t.Fatal("`?` opened help from picker filename focus")
		}
	})
}

// TestQuestionMarkOpensNonstackingContextualHelp drives `?` from eligible
// base contexts, asserts the typed context classification, that repeated `?`
// never stacks help, and Esc-exact opener restoration.
func TestQuestionMarkOpensNonstackingContextualHelp(t *testing.T) {
	openers := []struct {
		name     string
		build    func(t *testing.T) Model
		wantKind string
	}{
		{"builder base", func(t *testing.T) Model { return baseHelpFixture(t) }, helpKindBuilder},
		{"where field focused", func(t *testing.T) Model {
			return whereFocusedAt('s')
		}, helpKindWhere},
		{"settled result view", func(t *testing.T) Model {
			return resultHelpFixture(t)
		}, helpKindResult},
	}
	for _, tc := range openers {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.build(t)
			next, _ := before.Update(qKey())
			after := next.(Model)
			if !after.helpOpen {
				t.Fatalf("%s: `?` did not open help", tc.name)
			}
			if after.helpKind != tc.wantKind {
				t.Fatalf("%s help kind = %q, want %q", tc.name, after.helpKind, tc.wantKind)
			}
			if after.helpOpener == nil {
				t.Fatalf("%s opened help without an opener snapshot", tc.name)
			}

			// Repeated `?` never stacks help: the same overlay stays open,
			// the same opener snapshot is retained, and no extra state
			// appears. The snapshot is shared by value, so a second press
			// must keep pointing at the identical immutable opener copy.
			twice, _ := after.Update(qKey())
			tm := twice.(Model)
			if !tm.helpOpen {
				t.Fatal("repeated `?` closed the help overlay")
			}
			if tm.helpOpener != after.helpOpener {
				t.Fatal("repeated `?` minted a second opener snapshot instead of keeping one overlay")
			}
			if tm.helpKind != tc.wantKind {
				t.Fatalf("repeated `?` changed help kind to %q", tm.helpKind)
			}

			// Esc restores the exact opener state: focus, scroll, first
			// visible column, builder snapshot, viewport offset, and selected
			// history entries — nothing rebuilt from rendered text.
			restored, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
			rm := restored.(Model)
			if rm.helpOpen || rm.helpOpener != nil || rm.helpKind != "" {
				t.Fatal("esc did not close the help overlay atomically")
			}
			if rm.Focus != before.Focus || rm.Scroll != before.Scroll || rm.firstColumn != before.firstColumn {
				t.Fatalf("%s: opener focus/scroll/viewport not restored exactly", tc.name)
			}
			if rm.pageOffset != before.pageOffset {
				t.Fatalf("%s: page offset changed by help", tc.name)
			}
			if rm.historyMode != before.historyMode || rm.historyCursorID != before.historyCursorID ||
				rm.resultHistoryMode != before.resultHistoryMode || rm.resultHistoryCursorID != before.resultHistoryCursorID {
				t.Fatalf("%s: selected history changed by help", tc.name)
			}
			if len(rm.Fields) != len(before.Fields) {
				t.Fatalf("%s: builder fields rebuilt from rendered text", tc.name)
			}
			for i := range rm.Fields {
				if rm.Fields[i] != before.Fields[i] {
					t.Fatalf("%s: builder field %q changed by help", tc.name, before.Fields[i].Label)
				}
			}
		})
	}
}

// TestQuestionMarkIsNoOpInOverlaysAndTooSmall asserts `?` is consumed as a
// no-op wherever no search input is focused: over a modal, the quit
// confirmation, a scroll-only popup, and the too-small screen.
func TestQuestionMarkIsNoOpInOverlays(t *testing.T) {
	t.Run("preparation modal", func(t *testing.T) {
		m := settledPreparation(t, prepUpdateQB(true), &prepFakeEstimator{result: EstimateResult{Total: 3}}, EstimateResult{Total: 3})
		next, _ := m.Update(qKey())
		a := next.(Model)
		if !a.prepOpen || a.helpOpen {
			t.Fatal("`?` disturbed the preparation modal")
		}
	})
	t.Run("scroll-only popup", func(t *testing.T) {
		m := sized(New(), 80, 24).(Model)
		m.installPopup(NewScrollOnlyPopup(tableFieldLabel, []PopupCandidate{
			{ID: "a", Display: "a"},
		}), nil)
		next, _ := m.Update(qKey())
		a := next.(Model)
		if a.Popup == nil || a.Popup.Open() == false {
			t.Fatal("scroll-only popup disturbed by `?`")
		}
		if a.helpOpen {
			t.Fatal("`?` opened help over a scroll-only popup")
		}
	})
	t.Run("quit confirmation", func(t *testing.T) {
		m := baseHelpFixture(t)
		qm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		q := qm.(Model)
		next, _ := q.Update(qKey())
		a := next.(Model)
		if !a.quitConfirm || a.helpOpen {
			t.Fatal("`?` in quit confirmation opened help or leaked")
		}
		// Esc restores the suspended base context, not help.
		rm, _ := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
		rm2 := rm.(Model)
		if rm2.quitConfirm || rm2.helpOpen {
			t.Fatal("quit cancel did not restore the exact suspended context")
		}
	})
	t.Run("too-small screen", func(t *testing.T) {
		m := sized(baseHelpFixture(t), 79, 23).(Model)
		next, _ := m.Update(qKey())
		a := next.(Model)
		if !a.suspended {
			t.Fatal("too-small state lost suspension")
		}
		if a.helpOpen || a.terminalHelpOpen {
			t.Fatal("`?` opened any help while undersized")
		}
	})
}

// TestQuestionMarkNeverLeaksToLowerBaseAction opens help from a base result
// and asserts the dismissal key is not applied beneath the closing overlay:
// Esc restores the result instead of dismissing it as an error.
func TestQuestionMarkNeverLeaksIntoLowerBaseAction(t *testing.T) {
	m := resultHelpFixture(t)
	if m.Result == nil {
		t.Fatal("setup: no settled result")
	}
	_ = m
	hm := resultHelpFixture(t)
	opened, _ := hm.Update(qKey())
	h := opened.(Model)
	if !h.helpOpen {
		t.Fatal("result base `?` did not open help")
	}
	restored, _ := h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := restored.(Model)
	if rm.helpOpen {
		t.Fatal("esc did not close help")
	}
	if rm.Result == nil {
		t.Fatal("esc leaked into the base error-dismissal action beneath help")
	}
}

// TestRequiredWhereHelpContent asserts the exact WHERE SQL-NULL semantics of
// the contextual help opened from a WHERE value/operator context.
func TestRequiredWhereHelpContent(t *testing.T) {
	m := whereFocusedAt('s')
	next, _ := m.Update(qKey())
	after := next.(Model)
	if !after.helpOpen || after.helpKind != helpKindWhere {
		t.Fatal("WHERE-focused `?` did not open the WHERE help")
	}
	lines := after.helpLines()
	view := after.View()
	for _, required := range []string{
		"A typed token spelled NULL binds as literal TEXT",
		"use the IS NULL or IS NOT NULL operator",
		"Ordinary comparisons and LIKE do not match rows where the column",
		"actually holds NULL.",
	} {
		if !strings.Contains(view, required) && !containsLine(lines, required) {
			t.Errorf("WHERE help lacks %q:\n%s", required, view)
		}
	}
}

// TestRequiredResultCountHelpContent asserts the result-count help semantics
// for a result with independent count state.
func TestRequiredResultCountHelpContent(t *testing.T) {
	m := resultHelpFixture(t)
	next, _ := m.Update(qKey())
	after := next.(Model)
	if !after.helpOpen || after.helpKind != helpKindResult {
		t.Fatal("result `?` did not open the result-count help")
	}
	lines := after.helpLines()
	if !containsLine(lines, "including your Limit") {
		t.Error("result help does not state the count covers the Limit")
	}
	if !containsLine(lines, "not a table count and not a pre-Limit row count") {
		t.Error("result help lacks the not-a-table/pre-Limit-count semantics")
	}
	if !containsLine(lines, "independent autocommit read") || !strings.Contains(strings.Join(lines, "\n"), "drift") {
		t.Error("result help lacks the independent autocommit drift semantics")
	}
	if !containsLine(lines, "never clamps fetched pages or the retained result cache") {
		t.Error("result help lacks the no-clamping statement")
	}
}

// TestReducedTerminalHelpListsOnlyInMemoryActions asserts, for populated and
// empty histories, that the outcome-unknown terminal help lists exactly its
// available in-memory actions — history selection, save, export rule, help
// dismissal, and status-1 quit — with no database suggestion, and preserves
// the exact opener selection while open.
func TestReducedTerminalHelpContentAndOpenerState(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "populated histories"
		if empty {
			name = "empty histories"
		}
		t.Run(name, func(t *testing.T) {
			m, sel, count, page, refresh := helpTerminalFixture(t, TerminalDeleted, empty)
			m = termKey(t, m, "esc") // leave the settlement's initial selection
			before := m
			opened := termKey(t, m, "?")
			if !openState(opened) {
				t.Fatal("terminal ? did not open the reduced help")
			}
			view := opened.View()
			for _, want := range []string{
				"Ctrl+P / Ctrl+N",
				"Ctrl+E / Ctrl+Y",
				"Ctrl+S",
				"Ctrl+X",
				"non-tabular",
				"Esc",
				"quit immediately (status 1)",
			} {
				if !strings.Contains(view, want) {
					t.Errorf("reduced help lacks %q:\n%s", want, view)
				}
			}
			for _, forbidden := range []string{
				"refresh", "rerun", "cancel", "cancel command",
				"Execute", "execute", "page results", "Select a command",
				"Enter validates",
			} {
				if strings.Contains(view, forbidden) {
					t.Errorf("terminal help suggests %q — no database suggestion may appear:\n%s", forbidden, view)
				}
			}
			// Opening help preserved the exact opener selection state.
			if opened.resultHistoryCursorID != m.resultHistoryCursorID || opened.historyCursorID != before.historyCursorID {
				t.Error("terminal help did not preserve the exact selected history")
			}
			_ = sel
			_ = count
			_ = page
			_ = refresh
		})
	}
}

// openState reports whether the reduced terminal help is open.
func openState(m Model) bool { return m.terminalHelpOpen }

// containsLine reports whether any help line equals or contains the needle.
func containsLine(lines []string, needle string) bool {
	for _, l := range lines {
		if strings.Contains(l, needle) {
			return true
		}
	}
	return false
}
