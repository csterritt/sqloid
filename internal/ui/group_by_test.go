// Scripted Bubble Tea coverage for Issue #18: the GROUP BY assisted
// multi-selection popup driven end-to-end through Update. Selection order is
// the user's own acceptance order; candidates exclude committed columns; the
// builder's AcceptGroupColumn rejection keeps a rejected identity an
// immutable no-op; Esc restores opener focus without committing.

package ui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// groupFocused drives a fresh model through SELECT on users with the Group By
// field focused, using the shared projection catalog.
func groupFocused(t *testing.T) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: projectionUICatalog()}, key('s'),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("setup table selection failed")
	}
	m.setFocus(findField(t, m, groupByFieldLabel))
	return m
}

// TestGroupByPopupSelectionOrderAndReopen requires each Enter to commit one
// column through the builder and reopen the popup with the remaining
// candidates, preserving the user's acceptance order in the field bar.
func TestGroupByPopupSelectionOrderAndReopen(t *testing.T) {
	m := groupFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, // open popup
		tea.KeyMsg{Type: tea.KeyDown},          // highlight email (second candidate)
		tea.KeyMsg{Type: tea.KeyEnter}).(Model) // commit email, reopen
	if m.Popup == nil || !m.Popup.Open() {
		t.Fatal("multi-selection popup did not reopen after a commit")
	}
	var got []string
	for _, c := range m.Popup.Visible() {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, []string{"id"}) {
		t.Fatalf("reopened candidates=%v, want only [id]", got)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // commit id, reopen
	if m.Popup == nil || !m.Popup.Open() {
		t.Fatal("popup closed while candidates remained")
	}
	if !m.Popup.NoMatch() && len(m.Popup.Visible()) != 0 {
		t.Fatalf("reopened candidates=%v, want none after full selection", m.Popup.Visible())
	}
	if got := m.Fields[findField(t, m, groupByFieldLabel)].Content; got != "email, id" {
		t.Fatalf("content=%q, want selection order %q", got, "email, id")
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Popup != nil {
		t.Fatal("Esc did not close the exhausted popup")
	}
	if m.Fields[m.Focus].Label != groupByFieldLabel {
		t.Fatalf("focus=%v, want %q restored", m.Fields[m.Focus].Label, groupByFieldLabel)
	}
}

// TestGroupByEscCancelsWithoutCommitting requires Esc on a fresh popup to
// close it and leave no group committed.
func TestGroupByEscCancelsWithoutCommitting(t *testing.T) {
	m := groupFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Popup != nil {
		t.Fatal("cancel left the popup open")
	}
	if entries := m.QB.GroupByEntries(); len(entries) != 0 {
		t.Fatalf("cancel committed %v", entries)
	}
	if m.Fields[m.Focus].Label != groupByFieldLabel {
		t.Fatalf("focus=%v, want %q restored", m.Fields[m.Focus].Label, groupByFieldLabel)
	}
}

// TestGroupByBackspaceRemovesLatest requires Backspace/Delete on the focused
// base Group By field to delete exactly the most recently accepted column.
func TestGroupByBackspaceRemovesLatest(t *testing.T) {
	m := groupFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}).(Model) // email
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // id
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyBackspace}).(Model) // remove id
	if got := m.QB.GroupByEntries(); !slices.Equal(got, []string{"email"}) {
		t.Fatalf("entries=%v after one Backspace, want [email]", got)
	}
	if got := m.Fields[findField(t, m, groupByFieldLabel)].Content; got != "email" {
		t.Fatalf("content=%q, want %q", got, "email")
	}
}
