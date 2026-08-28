// Scripted Bubble Tea model coverage for Issue #16 Tasks 3–4: Backspace and
// Delete from the base Column(s) field remove exactly the latest committed
// projection entry, walk backward through named entries and the bare sentinel,
// and no-op when empty — while Backspace/Delete inside a focused Column(s)
// search or scroll-only aggregate popup stay governed by the reusable popup
// contract. Projection rules are never duplicated here; they come from the
// QueryBuilder transitions covered by internal/querybuilder/projection_test.go.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// backspaceAndDelete runs both editing keys through Update so each scripted
// press exercises the same transition twice per state.
func backspaceAndDelete(m Model) Model {
	m = drive(m, tea.KeyMsg{Type: tea.KeyBackspace}).(Model)
	return drive(m, tea.KeyMsg{Type: tea.KeyDelete}).(Model)
}

// columnsEntryState asserts the committed entries, field-bar content, and
// exact focus ownership of the base Column(s) field.
func assertColumnsFocused(t *testing.T, m Model) {
	t.Helper()
	if m.Popup != nil {
		t.Fatalf("unexpected open popup %+v", *m.Popup)
	}
	if m.Fields[m.Focus].Label != columnsFieldLabel {
		t.Fatalf("focus=%v, want the base %v field", m.Fields[m.Focus].Label, columnsFieldLabel)
	}
}

// commitCountStarThenAggregates drives a fresh model through SELECT on users,
// commits the bare COUNT(*) sentinel plus id(Min) and email(Value), then
// closes the reopened popup to return to the base Column(s) context.
func commitCountStarThenAggregates(t *testing.T) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: projectionUICatalog()}, key('s')).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'), key('r'),
		tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyTab}).(Model)
	// Open Column(s); Down moves off `*` onto COUNT(*) and commits it directly,
	// which reopens Column(s) with only named candidates.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.QB.ProjectionEmpty() {
		t.Fatal("setup did not commit the sentinel")
	}
	for i, agg := range []int{2, 0} { // id → Min; then email highlighted → Value
		if i == 1 {
			m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model) // move to email
		}
		m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // routes named column
		if m.Popup == nil || m.Popup.Mode != PopupScrollOnly {
			t.Fatalf("setup step %d: aggregate popup=%+v, want scroll-only", i, m.Popup)
		}
		for d := 0; d < agg; d++ {
			m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
		}
		m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // completes aggregate
	}
	entries := m.QB.ProjectionEntries()
	want := []qb.ProjectionEntry{
		{Kind: qb.ProjectionCountStar},
		{Kind: qb.ProjectionColumn, Column: "id", Aggregate: qb.AggMin},
		{Kind: qb.ProjectionColumn, Column: "email"},
	}
	if !projectionEntriesEqual(entries, want) {
		t.Fatalf("setup entries=%v, want %v", entries, want)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	assertColumnsFocused(t, m)
	return m
}

// TestBackspaceDeletesRemoveExactlyLatestWalkingBackward requires repeated
// presses from the base Column(s) field to remove one latest entry per press,
// walking backward through named entries and the bare sentinel with order
// preserved and focus left on Column(s); an empty state is an unchanged no-op.
func TestBackspaceDeletesRemoveExactlyLatestWalkingBackward(t *testing.T) {
	m := commitCountStarThenAggregates(t)
	want := [][]string{ // field-bar content after each backspace+delete round
		{"COUNT(*)"},
		{},
		{},
		{}, // empty-state round: a true unchanged no-op
	}
	for i, wantLabels := range want {
		m = backspaceAndDelete(m)
		assertColumnsFocused(t, m)
		idx := findField(t, m, columnsFieldLabel)
		if got := m.Fields[idx].Content; got != joinLabels(wantLabels) {
			t.Fatalf("round %d: Column(s) content=%q, want %q", i, got, joinLabels(wantLabels))
		}
	}
}

// joinLabels joins display labels the way projectionEntryLabels does.
func joinLabels(labels []string) string {
	out := ""
	for i, l := range labels {
		if i > 0 {
			out += ", "
		}
		out += l
	}
	return out
}

// TestRemovingSoleWildcardEmptiesAndRestoresEmptyCandidates requires removing
// the sole wildcard through base-field Backspace/Delete to empty the
// projection entirely, after which reopening Column(s) shows the Issue #15
// wildcard-first and COUNT(*)-second candidate sequence.
func TestRemovingSoleWildcardEmptiesAndRestoresEmptyCandidates(t *testing.T) {
	m := openColumnsPopup(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // accept highlighted *
	if entries := m.QB.ProjectionEntries(); len(entries) != 1 || entries[0].Kind != qb.ProjectionWildcard {
		t.Fatalf("setup entries=%v, want sole wildcard", entries)
	}
	m = backspaceAndDelete(m)
	assertColumnsFocused(t, m)
	if !m.QB.ProjectionEmpty() || m.Fields[findField(t, m, columnsFieldLabel)].Content != "" {
		t.Fatalf("wildcard removal left entries=%v content=%q",
			m.QB.ProjectionEntries(), m.Fields[findField(t, m, columnsFieldLabel)].Content)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // reopen Column(s)
	if m.Popup == nil || !m.Popup.Open() || m.Popup.Opener != columnsFieldLabel {
		t.Fatalf("reopened popup=%+v, want searchable Column(s)", m.Popup)
	}
	got := visibleCandidates(m)
	wantFirst := qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}.Key()
	wantSecond := qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key()
	if len(got) < 2 || got[0] != wantFirst || got[1] != wantSecond {
		t.Errorf("emptied candidates=%v, want %q first then %q second", got, wantFirst, wantSecond)
	}
}

// TestPopupEditingKeysKeepPopupContract requires Backspace/Delete inside a
// focused Column(s) search or the scroll-only aggregate popup to remain
// governed by the popup contract — editing search text without deleting any
// committed entry, closing, reopening, reordering, or corrupting the open
// popup.
func TestPopupEditingKeysKeepPopupContract(t *testing.T) {
	m := commitCountStarThenAggregates(t)
	entriesBefore := m.QB.ProjectionEntries()
	labelsBefore := m.Fields[findField(t, m, columnsFieldLabel)].Content

	// Searchable Column(s) popup: printable search narrows, backspace restores.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || m.Popup.Mode != PopupSearchable {
		t.Fatalf("popup=%+v, want searchable Column(s)", m.Popup)
	}
	m = drive(m, key('e')).(Model)
	if got := m.Popup.Search; got != "e" {
		t.Fatalf("search=%q after typing e, want e", got)
	}
	filtered := len(visibleCandidates(m))
	if filtered >= len(m.Popup.candidates) {
		t.Fatal("typing e did not narrow the candidate list")
	}
	m = backspaceAndDelete(m) // inside the popup this edits search only
	if got := m.Popup.Search; got != "" {
		t.Errorf("search=%q after in-popup backspace, want restored %q", got, "")
	}
	if len(visibleCandidates(m)) <= filtered {
		t.Error("in-popup backspace did not restore the full candidate list")
	}
	if !m.Popup.Open() || m.Popup.Opener != columnsFieldLabel {
		t.Errorf("popup=%+v changed by editing keys", *m.Popup)
	}
	if !projectionEntriesEqual(m.QB.ProjectionEntries(), entriesBefore) ||
		labelsBefore != m.Fields[findField(t, m, columnsFieldLabel)].Content {
		t.Error("in-popup backspace/delete removed or corrupted committed entries")
	}

	// Scroll-only aggregate popup: editing keys delete nothing there either.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || m.Popup.Mode != PopupScrollOnly {
		t.Fatalf("aggregate popup=%+v, want scroll-only", m.Popup)
	}
	aggLabels := visibleCandidates(m)
	if len(aggLabels) != 6 {
		t.Fatalf("aggregate choices=%v, want six", aggLabels)
	}
	m = backspaceAndDelete(m)
	if m.Popup == nil || m.Popup.Mode != PopupScrollOnly || m.Popup.Opener != columnsFieldLabel {
		t.Fatalf("scroll-only popup corrupted by editing keys: %+v", m.Popup)
	}
	if got := visibleCandidates(m); !equalStrings(got, aggLabels) {
		t.Errorf("scroll-only candidates=%v, want unchanged %v", got, aggLabels)
	}
	if !projectionEntriesEqual(m.QB.ProjectionEntries(), entriesBefore) {
		t.Error("scroll-only backspace/delete removed committed entries")
	}
}

// findField returns the index of the labeled field bar entry.
func findField(t *testing.T, m Model, label string) int {
	t.Helper()
	for i, f := range m.Fields {
		if f.Label == label {
			return i
		}
	}
	t.Fatalf("no %q field", label)
	return 0
}

// projectionEntriesEqual compares two ordered slices of committed entries.
func projectionEntriesEqual(a, b []qb.ProjectionEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalStrings reports whether two strings slices match exactly.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
