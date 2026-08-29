package ui

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

const setPopupViewport = 8

var setChoices = []struct {
	name   string
	choice qb.SetChoice
}{
	{"Value", qb.SetChoiceValue},
	{"NULL", qb.SetChoiceNull},
}

func (m *Model) setFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == setFieldLabel
}

func (m *Model) clampSetCursor() {
	assignments := m.QB.SetAssignments()
	if len(assignments) == 0 {
		m.setCursor = 0
		return
	}
	if m.setCursor < 0 {
		m.setCursor = 0
	}
	if m.setCursor >= len(assignments) {
		m.setCursor = len(assignments) - 1
	}
}

func (m *Model) moveSetCursor(delta int) bool {
	if !m.setFocused() || len(m.QB.SetAssignments()) == 0 {
		return false
	}
	m.setCursor += delta
	m.clampSetCursor()
	return true
}

func (m *Model) beginSetEdit() {
	if m.setFocused() {
		m.beginSetSelection()
	}
}

func (m *Model) beginSetSelection() {
	rows := make([]PopupCandidate, 0, len(m.QB.SetCandidates()))
	for _, column := range m.QB.SetCandidates() {
		rows = append(rows, PopupCandidate{ID: column.Name, Display: column.Name})
	}
	m.installPopup(NewMultiSearchablePopup(setFieldLabel, rows), setColumnAccepted)
	m.Popup.SetViewportHeight(setPopupViewport)
}

func setColumnAccepted(m *Model, column string) {
	next, ok := m.QB.AcceptSetColumn(column)
	if !ok {
		return
	}
	m.applyBuilder(next)
	refocusField(m, setFieldLabel)
	m.setCursor = len(next.SetAssignments()) - 1
}

func (m *Model) finishSetSelection() {
	assignments := m.QB.SetAssignments()
	if len(assignments) == 0 {
		refocusField(m, setFieldLabel)
		return
	}
	m.openSetChoice(firstIncompleteSetIndex(assignments, m.setCursor))
}

func firstIncompleteSetIndex(assignments []qb.SetAssignment, fallback int) int {
	for i, assignment := range assignments {
		if assignment.Choice() == qb.SetChoiceNone {
			return i
		}
		if assignment.Choice() == qb.SetChoiceValue {
			if _, ok := assignment.SubmittedValue(); !ok {
				return i
			}
		}
	}
	return fallback
}

func (m *Model) setChoiceStatus() []string {
	assignments := m.QB.SetAssignments()
	if m.Popup == nil || m.Popup.Opener != setFieldLabel || m.Popup.Multi || m.setCursor < 0 || m.setCursor >= len(assignments) {
		return nil
	}
	return []string{"Column: " + assignments[m.setCursor].Column}
}

func (m *Model) openSetChoice(index int) {
	assignments := m.QB.SetAssignments()
	if index < 0 || index >= len(assignments) {
		return
	}
	m.setCursor = index
	rows := make([]PopupCandidate, 0, len(setChoices))
	for _, choice := range setChoices {
		rows = append(rows, PopupCandidate{ID: choice.name, Display: choice.name})
	}
	m.installPopup(NewScrollOnlyPopup(setFieldLabel, rows), setChoiceAccepted)
	m.Popup.SetViewportHeight(setPopupViewport)
	for i, choice := range setChoices {
		if choice.choice == assignments[index].Choice() {
			for range i {
				m.Popup.Down()
			}
			break
		}
	}
}

func setChoiceAccepted(m *Model, id string) {
	assignments := m.QB.SetAssignments()
	if m.setCursor < 0 || m.setCursor >= len(assignments) {
		return
	}
	column := assignments[m.setCursor].Column
	for _, option := range setChoices {
		if option.name != id {
			continue
		}
		if option.choice == qb.SetChoiceValue {
			seed := ""
			if entered, ok := assignments[m.setCursor].Entered(); ok {
				seed = entered
			}
			m.ValuePrompt = NewValuePrompt(setFieldLabel, column+" = Value", seed)
			return
		}
		next, ok := m.QB.ChooseSetAssignment(column, qb.SetChoiceNull)
		if ok {
			m.applyBuilder(next)
			refocusField(m, setFieldLabel)
			m.advanceSetChoice()
		}
		return
	}
}

func (m *Model) setValueAccepted(text string) {
	assignments := m.QB.SetAssignments()
	if m.setCursor < 0 || m.setCursor >= len(assignments) {
		return
	}
	column := assignments[m.setCursor].Column
	next, ok := m.QB.ChooseSetAssignment(column, qb.SetChoiceValue)
	if !ok {
		return
	}
	next, ok = next.SubmitSetValue(column, text)
	if !ok {
		return
	}
	m.applyBuilder(next)
	refocusField(m, setFieldLabel)
	m.advanceSetChoice()
}

func (m *Model) advanceSetChoice() {
	if m.setCursor+1 < len(m.QB.SetAssignments()) {
		m.openSetChoice(m.setCursor + 1)
		return
	}
	refocusField(m, whereFieldLabel)
}

func (m *Model) clearCurrentSetValue() {
	assignments := m.QB.SetAssignments()
	if m.setCursor < 0 || m.setCursor >= len(assignments) {
		return
	}
	m.applyBuilder(m.QB.ClearSetValue(assignments[m.setCursor].Column))
	refocusField(m, setFieldLabel)
}
