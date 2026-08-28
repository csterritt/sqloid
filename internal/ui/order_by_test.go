// Scripted Bubble Tea coverage for Issue #18 Task 3: the ORDER BY popup and
// base-field flow driven end-to-end through Update. Candidate eligibility is
// owned by the QueryBuilder — only context-valid table-column or
// selected-expression identities are ever offered — while this layer owns
// exact focus, acceptance, cancellation, ASC default, Up/Down direction
// toggling in the focused base Order By field, and whole-value clearing.
// Grouping rules are never duplicated in UI fixtures.

package ui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// orderFocused drives a fresh model through SELECT on users, applies the
// given committed builder state through applyBuilder, then focuses the Order
// By field. The builder transitions come from the authoritative QueryBuilder
// tests, so this fixture only establishes context.
func orderFocused(t *testing.T, prepare func(q qb.QueryBuilder) qb.QueryBuilder) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: projectionUICatalog()}, key('s'),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("setup table selection failed")
	}
	if prepare != nil {
		m.applyBuilder(prepare(m.QB))
	}
	m.setFocus(findField(t, m, orderByFieldLabel))
	return m
}

// groupedWithCount prepares a grouped SELECT with COUNT(id) projected and id
// grouped, so only the grouped column and the selected aggregate remain.
func groupedWithCount(q qb.QueryBuilder) qb.QueryBuilder {
	q = q.CompleteProjectionAggregate("id", qb.AggCount).Builder
	q, _ = q.AcceptGroupColumn("id")
	return q
}

// TestOrderByPopupUngroupedOffersTableColumns requires the ungrouped popup to
// present exactly the visible table columns in Schema order, keyed by their
// typed identities.
func TestOrderByPopupUngroupedOffersTableColumns(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || !m.Popup.Open() {
		t.Fatal("Enter on Order By did not open its popup")
	}
	if m.Popup.Opener != orderByFieldLabel {
		t.Fatalf("opener=%q, want %q", m.Popup.Opener, orderByFieldLabel)
	}
	var got []string
	for _, c := range m.Popup.Visible() {
		got = append(got, c.ID)
	}
	want := []string{"order-column:id", "order-column:email"}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates=%v, want %v", got, want)
	}
}

// TestOrderByPopupGroupedOffersOnlyGroupedAndSelectedAggregates requires the
// grouped context to present exactly the grouped column and the selected
// aggregate — never the ungrouped email column or the wildcard.
func TestOrderByPopupGroupedOffersOnlyGroupedAndSelectedAggregates(t *testing.T) {
	m := orderFocused(t, groupedWithCount)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	var got []string
	for _, c := range m.Popup.Visible() {
		got = append(got, c.ID)
	}
	want := []string{"order-column:id", "order-aggregate:id:COUNT"}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates=%v, want %v", got, want)
	}
}

// TestOrderByPopupGroupedIncludesBareCountStar requires the bare COUNT(*)
// sentinel to appear beside the grouped column in grouped contexts.
func TestOrderByPopupGroupedIncludesBareCountStar(t *testing.T) {
	m := orderFocused(t, func(q qb.QueryBuilder) qb.QueryBuilder {
		q = q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
		q, _ = q.AcceptGroupColumn("email")
		return q
	})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	var got []string
	for _, c := range m.Popup.Visible() {
		got = append(got, c.ID)
	}
	want := []string{"order-column:email", "order-count-star"}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates=%v, want %v", got, want)
	}
}

// TestOrderByAcceptCommitsAndRestoresFocus requires Enter to commit the
// highlighted candidate, close the popup, restore focus to the Order By
// field, and render the committed expression with its ASC default.
func TestOrderByAcceptCommitsAndRestoresFocus(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup != nil {
		t.Fatal("accept left the popup open")
	}
	if m.Fields[m.Focus].Label != orderByFieldLabel {
		t.Fatalf("focus=%v, want %v restored", m.Fields[m.Focus].Label, orderByFieldLabel)
	}
	_, dir, ok := m.QB.OrderBySelection()
	if !ok || dir != qb.DirAsc {
		t.Fatalf("selection direction=%v ok=%v, want ASC", dir, ok)
	}
	if got := m.Fields[findField(t, m, orderByFieldLabel)].Content; got != "email ASC" {
		t.Fatalf("content=%q, want %q", got, "email ASC")
	}
}

// TestOrderByEscCancelsWithoutCommitting requires Esc to close the popup,
// restore opener focus, and leave no selection behind.
func TestOrderByEscCancelsWithoutCommitting(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Popup != nil {
		t.Fatal("cancel left the popup open")
	}
	if m.Fields[m.Focus].Label != orderByFieldLabel {
		t.Fatalf("focus=%v, want %v restored", m.Fields[m.Focus].Label, orderByFieldLabel)
	}
	if _, _, ok := m.QB.OrderBySelection(); ok {
		t.Fatal("cancel committed a selection")
	}
}

// TestOrderByUpDownTogglesDirectionInBaseField requires Up/Down in the
// focused base Order By field with a committed selection to flip ASC/DESC
// deterministically without opening any popup or moving focus.
func TestOrderByUpDownTogglesDirectionInBaseField(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // id ASC
	focusBefore := m.Focus
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	if m.Popup != nil {
		t.Fatal("Down opened a popup")
	}
	if m.Focus != focusBefore {
		t.Fatalf("Down moved focus to %d", m.Focus)
	}
	if _, dir, _ := m.QB.OrderBySelection(); dir != qb.DirDesc {
		t.Fatalf("Down produced %v, want DESC", dir)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyUp}).(Model)
	if _, dir, _ := m.QB.OrderBySelection(); dir != qb.DirAsc {
		t.Fatalf("Up produced %v, want ASC", dir)
	}
	if got := m.Fields[findField(t, m, orderByFieldLabel)].Content; got != "id ASC" {
		t.Fatalf("content=%q, want %q", got, "id ASC")
	}
}

// TestOrderByUpDownUncommittedNavigates requires Up/Down without a committed
// selection to keep ordinary focus navigation.
func TestOrderByUpDownUncommittedNavigates(t *testing.T) {
	m := orderFocused(t, nil)
	before := m.Focus
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	if m.Popup != nil {
		t.Fatal("Down opened a popup without a selection")
	}
	if m.Focus == before {
		t.Fatal("Down did not navigate without a committed selection")
	}
	if _, _, ok := m.QB.OrderBySelection(); ok {
		t.Fatal("navigation committed a selection")
	}
}

// TestOrderByReplacementResetsToAsc requires a popup replacement to swap the
// committed expression atomically and reset the direction to the ASC default.
func TestOrderByReplacementResetsToAsc(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyDown}).(Model) // id ASC then toggled DESC
	if _, dir, _ := m.QB.OrderBySelection(); dir != qb.DirDesc {
		t.Fatalf("setup direction=%v, want DESC", dir)
	}
	// Reopen and accept the other candidate: replacement is atomic.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	_, dir, ok := m.QB.OrderBySelection()
	if !ok || dir != qb.DirAsc {
		t.Fatalf("replacement direction=%v ok=%v, want ASC default", dir, ok)
	}
	if got := m.Fields[findField(t, m, orderByFieldLabel)].Content; got != "email ASC" {
		t.Fatalf("content=%q, want %q", got, "email ASC")
	}
}

// TestOrderByBackspaceClearsWholeValue requires Backspace/Delete on the
// focused base Order By field to remove the whole committed selection, after
// which reselection works normally.
func TestOrderByBackspaceClearsWholeValue(t *testing.T) {
	m := orderFocused(t, nil)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = backspaceAndDelete(m)
	if _, _, ok := m.QB.OrderBySelection(); ok {
		t.Fatal("clear left a selection behind")
	}
	if got := m.Fields[findField(t, m, orderByFieldLabel)].Content; got != "" {
		t.Fatalf("content=%q after clear, want empty", got)
	}
	// Reselection after clearing commits fresh.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, _, ok := m.QB.OrderBySelection(); !ok {
		t.Fatal("reselection after clear did not commit")
	}
}
