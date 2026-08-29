package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

func updateUIModel() Model {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	return focusField(modelWithQB(q), setFieldLabel)
}

func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("Update(%T) returned unexpected command", msg)
	}
	return next.(Model)
}

func TestUpdatePromptFlowSelectsUniqueColumnsAndCompletesAssignments(t *testing.T) {
	m := updateModel(t, updateUIModel(), tea.KeyMsg{Type: tea.KeyEnter})
	if m.Popup == nil || !m.Popup.Multi || m.Popup.Mode != PopupSearchable {
		t.Fatal("Set Enter did not open the searchable multi-select popup")
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.QB.SetAssignments(); len(got) != 1 || got[0].Column != "id" {
		t.Fatalf("first accepted SET columns = %#v", got)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.QB.SetAssignments(); len(got) != 1 {
		t.Fatalf("duplicate acceptance changed SET assignments: %#v", got)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.QB.SetAssignments(); len(got) != 2 || got[0].Column != "id" || got[1].Column != "email" {
		t.Fatalf("accepted SET order = %#v", got)
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Popup == nil || m.Popup.Multi || m.Popup.Mode != PopupScrollOnly {
		t.Fatal("closing SET selection did not open the first assignment choice")
	}
	if visible := m.Popup.Visible(); !reflect.DeepEqual(visible, []PopupCandidate{{ID: "Value", Display: "Value"}, {ID: "NULL", Display: "NULL"}}) {
		t.Fatalf("SET choices = %#v", visible)
	}
	if view := m.View(); !strings.Contains(view, "Column: id") || !strings.Contains(view, "> Value") || !strings.Contains(view, "NULL") {
		t.Fatalf("SET choice view does not identify its column and exact choices:\n%s", view)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt == nil {
		t.Fatal("Value choice did not open universal entry")
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("42")})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Popup == nil {
		t.Fatal("first Value submission did not advance to the second assignment")
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Popup != nil || m.ValuePrompt != nil || m.Fields[m.Focus].Label != whereFieldLabel {
		t.Fatalf("completed assignments did not continue to Where: focus=%q", m.Fields[m.Focus].Label)
	}
	assignments := m.QB.SetAssignments()
	if value, ok := assignments[0].SubmittedValue(); !ok || value.Kind != qb.KindInteger || value.Int != 42 {
		t.Fatalf("first assignment value = (%+v, %v)", value, ok)
	}
	if assignments[1].Choice() != qb.SetChoiceNull {
		t.Fatalf("second assignment choice = %v, want NULL", assignments[1].Choice())
	}
	if got := m.QB.UpdateParams(); !reflect.DeepEqual(got, []any{int64(42)}) {
		t.Fatalf("UpdateParams() = %#v", got)
	}
}

func TestUpdatePromptRevisionRestoresChoiceTextAndBoundType(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, _ = q.AcceptSetColumn("email")
	q, _ = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	q, _ = q.SubmitSetValue("email", "NULL")
	m := focusField(modelWithQB(q), setFieldLabel)

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Popup == nil || !m.Popup.Multi {
		t.Fatal("revision did not reopen SET selection")
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if highlighted, ok := m.Popup.Highlighted(); !ok || highlighted.ID != "Value" {
		t.Fatalf("restored choice highlight = (%+v, %v)", highlighted, ok)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "NULL" || m.ValuePrompt.Cursor() != 4 {
		t.Fatalf("restored value prompt = %#v", m.ValuePrompt)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	assignment := m.QB.SetAssignments()[0]
	value, ok := assignment.SubmittedValue()
	if !ok || assignment.Choice() != qb.SetChoiceValue || value.Kind != qb.KindText || value.Text != "NULL" {
		t.Fatalf("cancel changed restored assignment: choice=%v value=(%+v,%v)", assignment.Choice(), value, ok)
	}
}

func TestUpdateAssignmentNavigationAndChoiceRevision(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	for i, column := range []string{"id", "email", "note"} {
		q, _ = q.AcceptSetColumn(column)
		choice := qb.SetChoiceValue
		if i == 1 {
			choice = qb.SetChoiceNull
		}
		q, _ = q.ChooseSetAssignment(column, choice)
		if choice == qb.SetChoiceValue {
			q, _ = q.SubmitSetValue(column, column)
		}
	}
	before := q.HistoryState()
	m := focusField(modelWithQB(q), setFieldLabel)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if highlighted, ok := m.Popup.Highlighted(); !ok || highlighted.ID != "NULL" {
		t.Fatalf("Tab did not restore second assignment choice: (%+v, %v)", highlighted, ok)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.setCursor != 1 {
		t.Fatalf("Shift+Tab assignment cursor = %d, want 1", m.setCursor)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Fields[m.Focus].Label != setFieldLabel {
		t.Fatalf("choice cancellation restored focus to %q, want Set", m.Fields[m.Focus].Label)
	}
	if !m.QB.HistoryState().Equal(before) {
		t.Fatal("choice-popup navigation or cancellation changed assignment state")
	}

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.setCursor != 2 {
		t.Fatalf("Down assignment cursor = %d, want 2", m.setCursor)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.setCursor != 1 {
		t.Fatalf("Up assignment cursor = %d, want 1", m.setCursor)
	}
}

func TestUpdateChoiceCanChangeValueToNullAndBack(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, _ = q.AcceptSetColumn("email")
	q, _ = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	q, _ = q.SubmitSetValue("email", "old")
	m := focusField(modelWithQB(q), setFieldLabel)

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	assignment := m.QB.SetAssignments()[0]
	if assignment.Choice() != qb.SetChoiceNull {
		t.Fatalf("choice = %v, want NULL", assignment.Choice())
	}
	if _, ok := assignment.SubmittedValue(); ok {
		t.Fatal("Value-to-NULL revision retained a bound value")
	}

	m = focusField(m, setFieldLabel)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt == nil || m.ValuePrompt.Buffer() != "" {
		t.Fatalf("NULL-to-Value prompt = %#v, want fresh empty entry", m.ValuePrompt)
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	assignment = m.QB.SetAssignments()[0]
	value, ok := assignment.SubmittedValue()
	if !ok || value.Kind != qb.KindText || value.Text != "" {
		t.Fatalf("empty TEXT revision = (%+v, %v)", value, ok)
	}
}

func TestInvalidUpdateEnterTargetsFirstIncompleteAssignment(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, _ = q.AcceptSetColumn("id")
	q, _ = q.ChooseSetAssignment("id", qb.SetChoiceNull)
	q, _ = q.AcceptSetColumn("email")
	q, _ = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	m := focusField(modelWithQB(q), commandFieldLabel)
	m.setCursor = 0
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("invalid UPDATE Enter emitted a command")
	}
	got := next.(Model)
	if got.Fields[got.Focus].Label != setFieldLabel || got.setCursor != 1 {
		t.Fatalf("invalid UPDATE target = (%q, %d), want Set assignment 1", got.Fields[got.Focus].Label, got.setCursor)
	}
}

func TestUpdateWholeValueClearingUsesCurrentAssignment(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).SelectCommand(qb.CommandUpdate).SelectTable("users")
	for _, column := range []string{"id", "email"} {
		q, _ = q.AcceptSetColumn(column)
		q, _ = q.ChooseSetAssignment(column, qb.SetChoiceValue)
		q, _ = q.SubmitSetValue(column, column)
	}
	m := focusField(modelWithQB(q), setFieldLabel)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDelete})
	assignments := m.QB.SetAssignments()
	if _, ok := assignments[0].SubmittedValue(); !ok {
		t.Fatal("clearing second assignment disturbed first")
	}
	if _, ok := assignments[1].SubmittedValue(); ok || assignments[1].Choice() != qb.SetChoiceValue {
		t.Fatal("clearing did not preserve second Value choice while dropping its submission")
	}
	if report := m.QB.RunnableReport(); report.Runnable || report.Field != qb.RunFieldSetAssignments {
		t.Fatalf("cleared assignment report = %+v", report)
	}
}
