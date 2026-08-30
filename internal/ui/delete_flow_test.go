// Scripted Bubble Tea coverage for Issue #38 Task 3: DELETE command/table
// selection and its optional shared WHERE flow. DELETE reuses Issue #17's
// popup/value behavior, Issue #19's Enter gating, and the global context
// precedence in Notes/PRD-sqloid.md unchanged. Runnable no-WHERE and
// complete-predicate DELETE states emit only the established pre-execution
// handoff — never a direct internal/connection write execution and never a
// history append — while incomplete stages consume Enter with first-invalid
// focus, the QueryBuilder reason, and no preparation identity.

package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// withHistory wires a fresh history store so tests can prove no append.
func withHistory(m Model) Model {
	m.History = history.NewStore()
	return m
}

func deleteUIModel() Model {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandDelete).SelectTable("users")
	return focusField(withHistory(modelWithQB(q)), commandFieldLabel)
}

// deleteInvalidModel returns a base-context model over one incomplete DELETE
// predicate stage built through the QueryBuilder transitions themselves.
func deleteInvalidModel(stage string) Model {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandDelete).SelectTable("users")
	next, ok := q.StartWhere("note")
	if !ok {
		panic("StartWhere(note) rejected")
	}
	if stage == "awaiting-value" {
		draft, ok := next.WhereDraft().ChooseOperator(qb.OpEq)
		if !ok {
			panic("ChooseOperator(=) rejected")
		}
		next = next.ApplyWhereDraft(draft)
	}
	return focusField(withHistory(modelWithQB(next)), commandFieldLabel)
}

func TestDeleteTableSelectionOffersOnlyWriteEligibleObjects(t *testing.T) {
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: whereUICatalog()}, key('d')).(Model)
	if got := m.QB.Command(); got != qb.CommandDelete {
		t.Fatalf("command = %v, want DELETE", got)
	}
	if m.Fields[m.Focus].Label != tableFieldLabel {
		t.Fatalf("focus = %q, want Table", m.Fields[m.Focus].Label)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || m.Popup.Mode != PopupSearchable {
		t.Fatal("Table Enter did not open the searchable popup")
	}
	visible := m.Popup.Visible()
	if len(visible) != 1 || visible[0].ID != "users" {
		t.Fatalf("eligible DELETE tables = %#v, want only users (no view)", visible)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, selected := m.QB.SelectedTable(); !selected {
		t.Fatal("table was not selected")
	}
	hasWhere := false
	for _, f := range m.Fields {
		if f.Label == whereFieldLabel {
			hasWhere = true
		}
	}
	if !hasWhere {
		t.Fatal("DELETE did not render its optional Where field")
	}
}

func TestDeleteNoWhereEnterHandsOffToPreparation(t *testing.T) {
	m := deleteUIModel()
	next, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if enterCmd == nil {
		t.Fatal("runnable no-WHERE DELETE emitted no pre-execution seam")
	}
	m = next.(Model)
	opened, _ := m.Update(enterCmd())
	m = opened.(Model)
	if !m.validating {
		t.Fatal("seam did not open the pre-execution validation workflow")
	}
	if m.History.Len() != 0 {
		t.Fatalf("runnable Enter appended %d history entries", m.History.Len())
	}
}

func TestDeletePredicateFlowsCompleteAndHandOff(t *testing.T) {
	tests := []struct {
		name       string
		colDowns   int
		opToken    string
		value      string
		wantParams []any
	}{
		{"value equality", 0, "=", "42", []any{int64(42)}},
		{"LIKE verbatim wildcards", 0, "LIKE", `%a_b%`, []any{"%a_b%"}},
		{"IS NULL bypasses value input", 0, "IS NULL", "", nil},
		{"IS NOT NULL bypasses value input", 0, "IS NOT NULL", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := beginWhere(withHistory(whereFocusedAt('d')))
			m = chooseColumnAndOperator(m, tt.colDowns, tt.opToken)
			if tt.opToken == "IS NULL" || tt.opToken == "IS NOT NULL" {
				if m.ValuePrompt != nil {
					t.Fatal("null operator opened a value prompt")
				}
			} else {
				if m.ValuePrompt == nil {
					t.Fatal("value-taking operator opened no value prompt")
				}
				m = submitValueText(m, tt.value)
			}
			if m.Popup != nil || m.ValuePrompt != nil {
				t.Fatal("completed predicate did not restore the base context")
			}
			if m.Fields[m.Focus].Label != whereFieldLabel {
				t.Fatalf("focus = %q, want Where", m.Fields[m.Focus].Label)
			}
			report := m.QB.RunnableReport()
			if !report.Runnable {
				t.Fatalf("RunnableReport() = %+v, want runnable", report)
			}
			if got := m.QB.WhereParams(); !reflect.DeepEqual(got, tt.wantParams) {
				t.Fatalf("WhereParams() = %#v, want %#v", got, tt.wantParams)
			}
			// The focused Where field's own opener consumes Enter locally; move
			// to a non-opener base field before exercising the runnable gate.
			m = focusField(m, commandFieldLabel)
			next, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if enterCmd == nil {
				t.Fatal("runnable DELETE emitted no pre-execution seam")
			}
			m = next.(Model)
			opened, _ := m.Update(enterCmd())
			m = opened.(Model)
			if !m.validating {
				t.Fatal("seam did not open the validation workflow")
			}
			if m.History.Len() != 0 {
				t.Fatalf("runnable Enter appended %d history entries", m.History.Len())
			}
		})
	}
}

func TestDeletePredicateEscRestoresExactStateAndFocus(t *testing.T) {
	m := beginWhere(withHistory(whereFocusedAt('d')))
	m = chooseColumnAndOperator(m, 0, "=")
	m = submitValueText(m, "42")
	before := m.QB.HistoryState()

	// Revisit: the column popup, the operator popup, and the value prompt all
	// restore the prior staged choice exactly.
	m = beginWhere(m)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || m.Popup.Mode != PopupScrollOnly {
		t.Fatal("revisiting did not reopen the operator popup")
	}
	if highlighted, ok := m.Popup.Highlighted(); !ok || highlighted.ID != "=" {
		t.Fatalf("restored operator highlight = (%+v, %v)", highlighted, ok)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "42" {
		t.Fatalf("revisiting value = %#v, want exact restoration of 42", m.ValuePrompt)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if m.Fields[m.Focus].Label != whereFieldLabel {
		t.Fatalf("cancel restored focus to %q, want Where", m.Fields[m.Focus].Label)
	}
	if !m.QB.HistoryState().Equal(before) {
		t.Fatal("cancellation changed committed predicate state")
	}

	// A fresh visit restores the same exact choices all the way to completion.
	m = beginWhere(m)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if highlighted, ok := m.Popup.Highlighted(); !ok || highlighted.ID != "=" {
		t.Fatalf("fresh-visit operator highlight = (%+v, %v)", highlighted, ok)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "42" {
		t.Fatalf("fresh-visit value = %#v, want exact restoration of 42", m.ValuePrompt)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt != nil || m.Popup != nil {
		t.Fatal("re-commit left a stage open")
	}
	if !m.QB.HistoryState().Equal(before) {
		t.Fatal("re-commit changed committed predicate state")
	}
}

func TestDeleteWholeValueClearingPreservesChoice(t *testing.T) {
	m := beginWhere(withHistory(whereFocusedAt('d')))
	m = chooseColumnAndOperator(m, 0, "=")
	m = submitValueText(m, "42")
	before := m.QB.HistoryState()

	m = beginWhere(m)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "42" {
		t.Fatalf("revisiting value = %#v, want exact restoration of 42", m.ValuePrompt)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyDelete}).(Model)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if !m.QB.HistoryState().Equal(before) {
		t.Fatal("clearing disturbed the committed state")
	}
}

func TestIncompleteDeleteEnterTargetsWhereWithoutPreparation(t *testing.T) {
	tests := []struct {
		name  string
		stage string
	}{
		{"column chosen, no operator", "column-chosen"},
		{"operator chosen, no value", "awaiting-value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := deleteInvalidModel(tc.stage)
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatal("invalid DELETE Enter emitted a command")
			}
			m = next.(Model)
			if m.Fields[m.Focus].Label != whereFieldLabel {
				t.Fatalf("focus = %q, want Where", m.Fields[m.Focus].Label)
			}
			if !strings.Contains(m.View(), qb.ReasonIncompletePrompt) {
				t.Fatal("reason was not rendered")
			}
			if m.validationAttempt != 0 {
				t.Fatalf("invalid Enter advanced preparation identity to %d", m.validationAttempt)
			}
			if m.History.Len() != 0 {
				t.Fatalf("invalid Enter appended %d history entries", m.History.Len())
			}
		})
	}
}

func TestDeleteHigherPrecedenceContextsConsumeEnter(t *testing.T) {
	t.Run("open popup", func(t *testing.T) {
		m := beginWhere(withHistory(whereFocusedAt('d')))
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("popup context emitted a command")
		}
		m = next.(Model)
		if m.Popup == nil || m.Popup.Mode != PopupScrollOnly {
			t.Fatal("Enter inside the Where popup did not advance its own selection")
		}
	})
	t.Run("focused value input", func(t *testing.T) {
		m := beginWhere(withHistory(whereFocusedAt('d')))
		m = chooseColumnAndOperator(m, 0, "=")
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("value-entry context emitted a command")
		}
		m = next.(Model)
		if m.ValuePrompt != nil {
			t.Fatal("Enter inside value entry did not submit locally")
		}
		if value, ok := m.QB.WherePredicate().SubmittedValue(); !ok || value.Text != "" {
			t.Fatalf("submitted value = (%+v, %v), want empty TEXT", value, ok)
		}
	})
	t.Run("quit confirmation overlay", func(t *testing.T) {
		m := deleteUIModel()
		overlay := m.openQuitConfirmation().(Model)
		if !m.quitConfirm {
			t.Fatal("quit confirmation did not open")
		}
		next, cmd := overlay.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("quit confirmation consumed no Enter")
		}
		if _, isSeam := cmd().(PreExecutionRequestedMsg); isSeam {
			t.Fatal("overlay context emitted the runnable seam")
		}
		if _, quit := cmd().(tea.QuitMsg); !quit {
			t.Fatal("overlay Enter did not confirm the quit")
		}
		_ = next
	})
	t.Run("request pending", func(t *testing.T) {
		m := deleteUIModel()
		m.refreshPending = true
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("pending-request context emitted a command")
		}
		if !next.(Model).refreshPending {
			t.Fatal("Enter disturbed the pending request")
		}
	})
	t.Run("too small terminal", func(t *testing.T) {
		m := deleteUIModel()
		before := m.QB.HistoryState()
		m = m.resize(MinWidth-1, MinHeight-1)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("too-small context emitted a command")
		}
		if !next.(Model).suspended {
			t.Fatal("too-small context left suspension")
		}
		if !next.(Model).QB.HistoryState().Equal(before) {
			t.Fatal("Enter behind the too-small message changed builder state")
		}
	})
}
