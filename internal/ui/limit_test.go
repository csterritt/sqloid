// Scripted Bubble Tea coverage for Issue #18 Task 5: the Limit field's
// universal-entry flow driven end-to-end through Update. The entered
// representation stays distinct builder state; the QueryBuilder owns parsing
// and the exact invalid reason, which the field bar shows verbatim. Empty
// input means the unbounded logical result; cancel restores the prior value
// untouched; Backspace clears the whole value.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// limitFocused drives a fresh model through SELECT on users with the Limit
// field focused, using the shared projection catalog.
func limitFocused(t *testing.T) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: projectionUICatalog()}, key('s'),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("setup table selection failed")
	}
	m.setFocus(findField(t, m, limitFieldLabel))
	return m
}

// typeRunes appends one typed-runes key per rune.
func typeRunes(m Model, s string) Model {
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}).(Model)
	return m
}

// TestLimitEnterOpensPromptSeededWithPriorValue requires Enter to open the
// universal entry seeded byte-for-byte with the currently entered text.
func TestLimitEnterOpensPromptSeededWithPriorValue(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt == nil {
		t.Fatal("Enter on Limit did not open the value prompt")
	}
	if m.ValuePrompt.Opener != limitFieldLabel {
		t.Fatalf("opener=%q, want %q", m.ValuePrompt.Opener, limitFieldLabel)
	}
	m = typeRunes(m, "5")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // commit 5
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // reopen
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "5" {
		t.Fatalf("reopened prompt=%v, want seeded \"5\"", m.ValuePrompt)
	}
	m = typeRunes(m, "9")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)   // cancel the 59 draft
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // reopen
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "5" {
		t.Fatalf("post-cancel prompt=%v, want prior \"5\" restored", m.ValuePrompt)
	}
}

// TestLimitSubmitAcceptsCanonicalRange requires submitted integers to store
// exactly and render canonically, with no extra parameters.
func TestLimitSubmitAcceptsCanonicalRange(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = typeRunes(m, "07") // leading zeros are tolerated at entry
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt != nil {
		t.Fatal("submit left the prompt open")
	}
	if v, ok := m.QB.LimitValue(); !ok || v != 7 {
		t.Fatalf("LimitValue()=(%d,%v), want (7,true)", v, ok)
	}
	if got := m.QB.LimitInput(); got != "07" {
		t.Fatalf("LimitInput()=%q, want the entered representation verbatim", got)
	}
	if got := m.Fields[findField(t, m, limitFieldLabel)].Content; got != "07" {
		t.Fatalf("content=%q, want %q", got, "07")
	}
	if focus := m.Fields[m.Focus].Label; focus != limitFieldLabel {
		t.Fatalf("focus=%v, want %q kept", focus, limitFieldLabel)
	}
}

// TestLimitInvalidShowsExactReasonVerbatim requires a rejected submission to
// keep the entered text, show the QueryBuilder's exact reason verbatim, and
// report the first-invalid contract with no accepted value.
func TestLimitInvalidShowsExactReasonVerbatim(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = typeRunes(m, "-3")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.QB.LimitInput() != "-3" {
		t.Fatalf("LimitInput()=%q, want the invalid text preserved", m.QB.LimitInput())
	}
	if _, ok := m.QB.LimitValue(); ok {
		t.Fatal("invalid input produced an accepted value")
	}
	issue, invalid := m.QB.FirstInvalidIssue()
	if !invalid || issue.Field != qb.FieldIdentityLimit || issue.Reason != qb.LimitInvalidReason {
		t.Fatalf("first-invalid=%+v, want Limit/%s", issue, qb.LimitInvalidReason)
	}
	want := "-3 — " + qb.LimitInvalidReason
	if got := m.Fields[findField(t, m, limitFieldLabel)].Content; got != want {
		t.Fatalf("content=%q, want %q", got, want)
	}
	if view := m.View(); !strings.Contains(view, qb.LimitInvalidReason) {
		t.Fatal("rendered view lacks the exact invalid reason")
	}
}

// TestLimitEscRestoresPriorValue requires Esc to discard the open draft and
// leave the previously committed representation untouched.
func TestLimitEscRestoresPriorValue(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = typeRunes(m, "9")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // commit 9
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // reopen seeded "9"
	m = typeRunes(m, "9")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model) // cancel the 99 draft
	if m.ValuePrompt != nil {
		t.Fatal("cancel left the prompt open")
	}
	if m.QB.LimitInput() != "9" {
		t.Fatalf("LimitInput()=%q, want the prior \"9\" restored", m.QB.LimitInput())
	}
	if focus := m.Fields[m.Focus].Label; focus != limitFieldLabel {
		t.Fatalf("focus=%v, want %q", focus, limitFieldLabel)
	}
}

// TestLimitBackspaceClearsWholeValue requires Backspace/Delete on the focused
// base Limit field to remove the whole value — entered text and any accepted
// integer — after which revision works normally.
func TestLimitBackspaceClearsWholeValue(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = typeRunes(m, "9")
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = backspaceAndDelete(m)
	if m.QB.LimitInput() != "" {
		t.Fatalf("LimitInput()=%q after clear, want empty", m.QB.LimitInput())
	}
	if _, ok := m.QB.LimitValue(); ok {
		t.Fatal("clear left an accepted value")
	}
	issue, invalid := m.QB.FirstInvalidIssue()
	if invalid {
		t.Fatalf("cleared query reported %+v", issue)
	}
}

// TestLimitEmptyMeansUnbounded requires submitting nothing to commit the
// unbounded logical result: no accepted value, no invalid report, and no
// LIMIT clause in the rendered SQL.
func TestLimitEmptyMeansUnbounded(t *testing.T) {
	m := limitFocused(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.QB.LimitInput() != "" {
		t.Fatalf("LimitInput()=%q, want empty", m.QB.LimitInput())
	}
	if _, ok := m.QB.LimitValue(); ok {
		t.Fatal("empty input produced an accepted value")
	}
	if issue, invalid := m.QB.FirstInvalidIssue(); invalid {
		t.Fatalf("empty Limit reported %+v", issue)
	}
	if sql := m.QB.SelectSQL(); strings.Contains(sql, "LIMIT") {
		t.Fatalf("SelectSQL()=%q, want no LIMIT clause", sql)
	}
}
