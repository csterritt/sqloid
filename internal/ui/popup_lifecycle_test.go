// Scripted Bubble Tea lifecycle coverage for Issue #12 Task 3: searchable
// text input updating filtering without leaking into builder shortcuts,
// single-select Enter accepting and closing, multi-select add/reopen, Esc
// preserving completed multi-selections, and exact opener focus restoration
// on both accept and cancel paths. Scroll-only variants are driven through
// the same generic popup routing. No Table-specific production wiring and no
// database access.

package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	testOpenerLabel   = "TestField"
	testPopupViewport = 4
)

func baseSized(t *testing.T) Model {
	t.Helper()
	return sized(New(), 80, 24).(Model)
}

// TestSingleSelectAcceptRestoresExactOpenerFocus drives a single-select popup
// installed over a non-Table opener field: Enter accepts the highlighted ID,
// closes the popup, and restores exactly the captured opener focus index.
func TestSingleSelectAcceptRestoresExactOpenerFocus(t *testing.T) {
	m := baseSized(t)
	opener := m.Focus // startup opener is the Command field
	accepted := ""
	m.installPopup(NewSearchablePopup(testOpenerLabel, candidates("alpha", "beta", "gamma")),
		func(mm *Model, id string) { accepted = id },
	)
	m.Popup.SetViewportHeight(testPopupViewport)

	next := drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if next.Popup != nil {
		t.Fatal("Enter left a popup open after acceptance")
	}
	if accepted != "beta" {
		t.Errorf("accepted %q, want beta", accepted)
	}
	if next.Focus != opener {
		t.Errorf("accept restored UI focus=%d, want opener %d", next.Focus, opener)
	}
	if next.searchRuneCount() != 0 {
		t.Errorf("typed keys leaked into search after accept: %d runes", next.searchRuneCount())
	}
}

func (m Model) searchRuneCount() int {
	if m.Popup == nil {
		return 0
	}
	return len([]rune(m.Popup.Search))
}

// TestSearchInputDoesNotLeakIntoBuilderShortcuts requires printable keys,
// including S/U/D/I and '?', to become popup search text while a searchable
// popup is open: commands are never selected, Tab does not move builder
// focus, space is search input, and Backspace edits the search query.
func TestSearchInputDoesNotLeakIntoBuilderShortcuts(t *testing.T) {
	m := baseSized(t)
	before := m.QB.Command()
	focusBefore := m.Focus
	m.installPopup(NewSearchablePopup(testOpenerLabel, candidates("users", "logs")), nil)
	m.Popup.SetViewportHeight(testPopupViewport)
	next := drive(m, key('s'), key('u'), tea.KeyMsg{Type: tea.KeyTab},
		key('?'), tea.KeyMsg{Type: tea.KeySpace}, tea.KeyMsg{Type: tea.KeyBackspace}, key('u')).(Model)
	if next.QB.Command() != before {
		t.Errorf("command mutated under popup: %v -> %v", before, next.QB.Command())
	}
	if next.Focus != focusBefore {
		t.Errorf("builder focus moved under popup: %d -> %d", focusBefore, next.Focus)
	}
	if next.Popup.Search != "su?u" {
		t.Errorf("search=%q, want \"su?u\" — every printable became popup search text\n(no ? literal/help split, tab consumed)", next.Popup.Search)
	}
}

// TestEscCancelRestoresExactOpenerFocus drives the cancel path through Update:
// Esc closes unchanged and restores the exact opener focus captured at open.
func TestEscCancelRestoresExactOpenerFocus(t *testing.T) {
	m := baseSized(t)
	opener := m.Focus
	m.installPopup(NewSearchablePopup(testOpenerLabel, candidates("a")), nil)
	next := drive(m, key('x'), tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if next.Popup != nil {
		t.Fatal("Esc did not close the popup")
	}
	if next.Focus != opener {
		t.Errorf("cancel restored focus=%d, want opener %d", next.Focus, opener)
	}
}

// TestMultiSelectAddReopenAndEscPreservation scripts a multi-select popup:
// Enter adds each highlighted candidate nonduplicately, keeps the popup open
// between choices, and Esc preserves only already completed selections.
func TestMultiSelectAddReopenAndEscPreservation(t *testing.T) {
	m := baseSized(t)
	m.installPopup(NewMultiSearchablePopup(testOpenerLabel, candidates("a", "b", "c")), nil)
	m.Popup.SetViewportHeight(testPopupViewport)
	m1 := drive(m, tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m1.Popup == nil || !m1.Popup.Open() {
		t.Fatal("multi-select Enter closed the popup")
	}
	want := []string{"a", "c"}
	if got := m1.Popup.Completed(); !slices.Equal(got, want) {
		t.Fatalf("completed=%v, want %v", got, want)
	}
	m2 := drive(m1, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m2.Popup != nil {
		t.Error("Esc left the popup object installed")
	}
}

// TestScrollOnlyVariantThroughUpdate drives the scroll-only variant through
// the same routing: printable keys do nothing, Up/Down navigate all items,
// and Enter accepts without ever accumulating search text.
func TestScrollOnlyVariantThroughUpdate(t *testing.T) {
	m := baseSized(t)
	m.installPopup(NewScrollOnlyPopup(testOpenerLabel, candidates("=", "!=", "<")), nil)
	m.Popup.SetViewportHeight(testPopupViewport)
	m1 := drive(m, key('='), tea.KeyMsg{Type: tea.KeyDown}).(Model)
	if m1.Popup == nil || m1.Popup.Search != "" {
		t.Fatalf("scroll-only accumulated search %q, want none", m1.Popup.Search)
	}
	if _, ok := m1.Popup.Highlighted(); !ok {
		t.Fatal("no highlight in scroll-only list")
	}
}

// End-to-end coverage for Issue #12 Task 4: the builder's Table field as the
// first searchable single-select popup consumer. Candidates follow refreshed
// Schema eligibility held by QueryBuilder; acceptance commits identity
// through its transitions and both paths restore exact opener focus.

// TestTableEnterOpensFreshSearchablePopup requires Enter on a focused Table
// field to open a fresh searchable popup listing exactly the current
// command's eligible objects, with empty search showing all of them.
func TestTableEnterOpensFreshSearchablePopup(t *testing.T) {
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: uiCatalog()}, key('s')).(Model)
	if m.Focus != 1 || m.Fields[m.Focus].Label != tableFieldLabel {
		t.Fatalf("setup: focus=%d field=%q, want Table", m.Focus, m.Fields[m.Focus].Label)
	}
	next := drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if next.Popup == nil || !next.Popup.Open() {
		t.Fatal("Enter on Table did not open a popup")
	}
	if next.Popup.Mode != PopupSearchable || next.Popup.Multi || next.Popup.Opener != tableFieldLabel {
		t.Errorf("popup=%+v, want searchable single-select opened by Table", *next.Popup)
	}
	names := ids(next.Popup)
	if !slices.Equal(names, []string{"logs_fts", "users", "vw_summary"}) {
		t.Errorf("SELECT candidates=%v, want every cataloged object in source order", names)
	}
	// Write commands see only write-eligible tables.
	wr := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: uiCatalog()}, key('u'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if got := ids(wr.Popup); !slices.Equal(got, []string{"logs_fts", "users"}) {
		t.Errorf("UPDATE candidates=%v, want write tables only", got)
	}
}

// TestTableAcceptCommitsIdentityAndRestoresOpener scripts the full accept
// path: search narrows the list, Enter commits the highlighted object name
// through the QueryBuilder transition, closes the popup, and lands back on
// the exact opener field (Table) that opened it.
func TestTableAcceptCommitsIdentityAndRestoresOpener(t *testing.T) {
	opener := 1 // Table holds UI focus index 1 after any command selection
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: uiCatalog()}, key('u')).(Model)
	if m.Focus != opener {
		t.Fatalf("setup opener=%d, want %d", m.Focus, opener)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'), key('r')).(Model)
	if got := ids(m.Popup); !slices.Equal(got, []string{"users"}) {
		t.Fatalf("'ser' narrowed to %v, want [users]", got)
	}
	next := drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	name, ok := next.QB.SelectedTable()
	if !ok || name != "users" {
		t.Errorf("accepted selection=(%q,%v), want users committed via builder", name, ok)
	}
	if next.Popup != nil {
		t.Error("accept left the popup installed")
	}
	if next.Focus != opener || next.Fields[next.Focus].Label != tableFieldLabel {
		t.Errorf("accept restored focus=%d (%q), want exact Table opener",
			next.Focus, next.Fields[next.Focus].Label)
	}
}

// TestTableEscClosesUnchangedAndRestoresExactOpener scripts the cancel path:
// Esc discards search and highlight without touching the builder and puts
// focus back on the very field that opened the popup.
func TestTableEscClosesUnchangedAndRestoresExactOpener(t *testing.T) {
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: uiCatalog()}, key('d')).(Model)
	beforeCmd := m.QB.Command()
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, key('v'), tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Popup != nil {
		t.Error("Esc left the popup installed")
	}
	if _, ok := m.QB.SelectedTable(); ok {
		t.Error("cancel path selected a table anyway")
	}
	if m.QB.Command() != beforeCmd {
		t.Errorf("command mutated through popup keys: %v", m.QB.Command())
	}
	if m.Focus != 1 || m.Fields[1].Label != tableFieldLabel {
		t.Errorf("Esc restored focus=%d (%q), want exact Table opener", m.Focus, m.Fields[1].Label)
	}
}

// TestTablePopupEmptyCatalogStaysOpenWithNoMatches drives Enter on a Table
// field against an unrefreshed catalog: the popup opens on empty candidate
// data, shows the exact no-match state, Enter is ignored, and Esc restores.
func TestTablePopupEmptyCatalogStaysOpenWithNoMatches(t *testing.T) {
	m := drive(sized(New(), 80, 24), key('s')).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || !m.Popup.NoMatch() || !m.Popup.Open() {
		t.Fatalf("empty catalog popup state wrong: open=%v noMatch=%v",
			m.Popup.Open(), m.Popup.NoMatch())
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Popup != nil || m.Focus != 1 {
		t.Errorf("post-Esc popup=%v focus=%d, want closed/1", m.Popup, m.Focus)
	}
}

// TestViewDrawsPopupOverResultsWithoutReflow pins the View-level overlay:
// with a popup open, the composed shell contains the popup's box while every
// region keeps its dimensions — status line, builder rows, footer intact.
func TestViewDrawsPopupOverResultsWithoutReflow(t *testing.T) {
	m := baseSized(t)
	before := m.View()
	m.installPopup(NewSearchablePopup(tableFieldLabel, candidates("alpha", "beta")), nil)
	withPopup := m.View()
	for _, want := range []string{"Search: _", "> alpha", "beta"} {
		if !strings.Contains(withPopup, want) {
			t.Errorf("view lacks popup element %q:\n%s", want, withPopup)
		}
	}
	if strings.Count(before, "\n") != strings.Count(withPopup, "\n") {
		t.Errorf("overlay reflowed the shell: before lines=%d after lines=%d",
			strings.Count(before, "\n"), strings.Count(withPopup, "\n"))
	}
}
