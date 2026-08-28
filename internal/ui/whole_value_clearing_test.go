// Scripted Bubble Tea coverage for the general completed whole-value clearing
// contract (Issue #19 Task 3): Backspace/Delete on focused base fields clears
// the entire entered representation, parsed value, and submission marker
// atomically; already empty fields are unchanged no-ops; keys inside an open
// popup or value prompt keep that context's editing behavior.

package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// clearKeyMsgs returns both whole-value clearing key messages.
func clearKeyMsgs() []tea.Msg {
	return []tea.Msg{tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyDelete}}
}

// modelWithQB returns a supported-size model whose builder state is q, with
// the field bar rebuilt from it.
func modelWithQB(q qb.QueryBuilder) Model {
	m := sized(New(), 80, 24).(Model)
	m.QB = q
	m.applyBuilder(q)
	return m
}

// validSelectQB returns runnable SELECT state with a completed WHERE value.
func validSelectQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandSelect).SelectTable("users")
	q = q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
	return completeWhereQB(q, "x")
}

// validDeleteQB returns runnable DELETE state with a completed WHERE value.
func validDeleteQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandDelete).SelectTable("users")
	return completeWhereQB(q, "x")
}

// completeWhereQB commits a `=` predicate over the name column through the
// guided transitions.
func completeWhereQB(q qb.QueryBuilder, text string) qb.QueryBuilder {
	next, ok := q.StartWhere("email")
	if !ok {
		panic("setup: StartWhere failed")
	}
	draft, ok := next.WhereDraft().ChooseOperator(qb.OpEq)
	if !ok {
		panic("setup: ChooseOperator failed")
	}
	next = next.ApplyWhereDraft(draft)
	draft, ok = draft.SubmitValue(text)
	if !ok {
		panic("setup: SubmitValue failed")
	}
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		panic("setup: CommitWhereDraft failed")
	}
	return next
}

// focusField moves model focus to the named field label.
func focusField(m Model, label string) Model {
	for i := range m.Fields {
		if m.Fields[i].Label == label {
			m.setFocus(i)
			return m
		}
	}
	panic("setup: field label missing: " + label)
}

// TestBackspaceAndDeleteClearCompletedWhereValue requires both clearing keys
// on the focused base Where field to remove the whole submitted value while
// preserving the selected column and operator as an incomplete draft, with no
// command returned.
func TestBackspaceAndDeleteClearCompletedWhereValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		qb   qb.QueryBuilder
	}{
		{"select", validSelectQB()},
		{"delete", validDeleteQB()},
	} {
		for _, keyType := range []tea.KeyType{tea.KeyBackspace, tea.KeyDelete} {
			m := modelWithQB(tc.qb)
			m = focusField(m, whereFieldLabel)
			next, cmd := m.Update(tea.KeyMsg{Type: keyType})
			if cmd != nil {
				t.Errorf("%s/%v: clearing returned a command %v", tc.name, keyType, cmd)
			}
			got := next.(Model)
			if got.QB.HasWhere() {
				t.Errorf("%s/%v: cleared WHERE still committed", tc.name, keyType)
			}
			if !got.QB.WhereDrafting() {
				t.Fatalf("%s/%v: clearing did not reopen the WHERE draft", tc.name, keyType)
			}
			draft := got.QB.WhereDraft()
			if col, ok := draft.Column(); !ok || col.Name != "email" {
				t.Errorf("%s/%v: column identity = (%v,%v), want name preserved",
					tc.name, keyType, col.Name, ok)
			}
			if op, ok := draft.ChosenOperator(); !ok || op != qb.OpEq {
				t.Errorf("%s/%v: operator = (%v,%v), want = preserved", tc.name, keyType, op, ok)
			}
			if _, ok := draft.SubmittedValue(); ok {
				t.Errorf("%s/%v: cleared draft still reports a submission", tc.name, keyType)
			}
			if report := got.QB.RunnableReport(); report.Runnable ||
				report.Field != qb.RunFieldWhere || report.Reason != qb.ReasonIncompletePrompt {
				t.Errorf("%s/%v: resulting report = %+v, want blocked at WHERE",
					tc.name, keyType, report)
			}
		}
	}
}

// TestClearingEmptyWhereFieldIsNoOp requires already absent or already empty
// whole fields to be exact no-ops with no focus or command side effect.
func TestClearingEmptyWhereFieldIsNoOp(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandSelect).SelectTable("users")
	q = q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
	m := modelWithQB(q)
	m = focusField(m, whereFieldLabel)
	for _, msg := range clearKeyMsgs() {
		next, cmd := m.Update(msg)
		if cmd != nil {
			t.Fatalf("empty Where clearing returned a command %v", cmd)
		}
		got := next.(Model)
		if !reflect.DeepEqual(got.QB, m.QB) {
			t.Fatalf("empty Where clearing changed builder state")
		}
		if got.Focus != m.Focus || got.Fields[got.Focus].Label != whereFieldLabel {
			t.Fatalf("empty Where clearing moved focus")
		}
	}
}

// TestClearingLimitWithKeysRequiresClearingToRestoreUnboundedValidity clears
// valid and invalid nonempty Limit text with both keys and asserts the
// unbounded valid result; an already empty Limit is an exact no-op.
func TestClearingLimitWithKeysRequiresClearingToRestoreUnboundedValidity(t *testing.T) {
	for _, text := range []string{"5", "abc"} {
		q := validSelectQB().SetLimitInput(text)
		for _, keyType := range []tea.KeyType{tea.KeyBackspace, tea.KeyDelete} {
			m := modelWithQB(q)
			m = focusField(m, limitFieldLabel)
			next, cmd := m.Update(tea.KeyMsg{Type: keyType})
			if cmd != nil {
				t.Fatalf("clearing %q with %v returned a command", text, keyType)
			}
			got := next.(Model)
			if got.QB.LimitInput() != "" {
				t.Errorf("clearing %q with %v left entered text %q", text, keyType, got.QB.LimitInput())
			}
			if _, ok := got.QB.LimitValue(); ok {
				t.Errorf("clearing %q with %v left an accepted value", text, keyType)
			}
			if report := got.QB.RunnableReport(); !report.Runnable {
				t.Errorf("cleared Limit report = %+v, want valid unbounded state", report)
			}
			if got.Fields[got.Focus].Label != limitFieldLabel {
				t.Errorf("clearing moved focus to %q", got.Fields[got.Focus].Label)
			}
		}
	}
	q := validSelectQB().SetLimitInput("")
	m := modelWithQB(q)
	m = focusField(m, limitFieldLabel)
	for _, msg := range clearKeyMsgs() {
		next, _ := m.Update(msg)
		if !reflect.DeepEqual(next.(Model).QB, q) {
			t.Fatalf("clearing an already empty Limit changed state")
		}
	}
}

// TestPopupAndPromptKeysRetainTheirEditingContext requires keys inside an
// open popup or focused value prompt to keep that context's editing behavior
// rather than triggering base-field clearing.
func TestPopupAndPromptKeysRetainTheirEditingContext(t *testing.T) {
	m := modelWithQB(validSelectQB())
	m = focusField(m, whereFieldLabel)
	m.installPopup(NewSearchablePopup(whereFieldLabel, whereColumnCandidates(m.QB)), whereColumnAcceptHook)
	popupSearchBefore := m.Popup.Search
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	got := next.(Model)
	if got.Popup == nil || !got.Popup.Open() {
		t.Fatal("Backspace closed the open popup instead of editing its search")
	}
	if got.QB.HasWhere() != m.QB.HasWhere() || got.QB.WhereDrafting() != m.QB.WhereDrafting() {
		t.Error("Backspace inside a popup mutated builder state")
	}
	_ = popupSearchBefore

	// A focused value prompt edits its buffer; Backspace never clears the
	// whole committed field underneath.
	m2 := modelWithQB(validSelectQB())
	m2 = focusField(m2, limitFieldLabel)
	m2.ValuePrompt = NewValuePrompt(limitFieldLabel, "row limit", m2.QB.LimitInput())
	next2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyDelete})
	got2 := next2.(Model)
	if got2.ValuePrompt == nil {
		t.Fatal("Delete closed the value prompt")
	}
	if got2.QB.LimitInput() != m2.QB.LimitInput() {
		t.Error("Delete inside a value prompt cleared the committed Limit")
	}
}

// TestProjectionRemovalUnaffectedByWholeValueClearing pins that Issue #16's
// remove-latest projection behavior is untouched: Backspace on the base
// Column(s) field still removes one entry rather than whole-value clearing.
func TestProjectionRemovalUnaffectedByWholeValueClearing(t *testing.T) {
	m := modelWithQB(validSelectQB())
	m = focusField(m, columnsFieldLabel)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	got := next.(Model)
	if !got.QB.ProjectionEmpty() {
		t.Fatal("Backspace on Column(s) did not remove the wildcard entry")
	}
	if got.QB.HasWhere() != m.QB.HasWhere() {
		t.Error("Backspace on Column(s) disturbed the committed WHERE")
	}
}

// TestClearingCorrectedReasonsVanish requires the inline runnable reason to
// disappear once the focused field is corrected via any builder transition.
func TestClearingCorrectedReasonsVanish(t *testing.T) {
	m := modelWithQB(validSelectQB().SetLimitInput("abc"))
	m = focusField(m, limitFieldLabel)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if !strings.Contains(got.Fields[got.Focus].Content, qb.LimitInvalidReason) {
		t.Fatal("setup: reason not shown after invalid Enter")
	}
	corrected := modelWithQB(got.QB.ClearLimitValue())
	corrected = focusField(corrected, limitFieldLabel)
	if strings.Contains(corrected.Fields[corrected.Focus].Content, qb.LimitInvalidReason) {
		t.Fatal("stale reason survived the correcting transition")
	}
}
