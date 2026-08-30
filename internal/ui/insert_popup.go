// INSERT prompt popups (Issue #39 Tasks 5–6), mirroring the UPDATE SET flow:
// one shared scroll-only choice popup per insertable column in schema order,
// offering exactly {Value, NULL, Default/Omit}. Universal text entry opens
// only for Value, seeded with the exact prior entered representation on
// revision; NULL and Default/Omit complete immediately. The INTEGER PRIMARY
// KEY omission hint renders from QueryBuilder metadata — never UI type
// inference — and never changes behavior or pre-selects omission. Esc cancels
// with exact restoration; Tab/Shift+Tab move between prompts; Backspace/
// Delete on a completed Value clears the whole submission while keeping the
// Value choice. No prompt handling ever executes SQL, mutates history, or
// issues a connection request: execution remains behind the established
// pre-execution validation handoff.

package ui

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

const insertPopupViewport = 8

var insertChoices = []struct {
	name   string
	choice qb.InsertChoice
}{
	{"Value", qb.InsertChoiceValue},
	{"NULL", qb.InsertChoiceNull},
	{"Default/Omit", qb.InsertChoiceOmit},
}

func (m *Model) insertFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == insertFieldLabel
}

func (m *Model) clampInsertCursor() {
	columns := m.QB.InsertColumns()
	if len(columns) == 0 {
		m.insertCursor = 0
		return
	}
	if m.insertCursor < 0 {
		m.insertCursor = 0
	}
	if m.insertCursor >= len(columns) {
		m.insertCursor = len(columns) - 1
	}
}

// firstIncompleteInsertIndex returns the index of the first prompt still
// awaiting a choice or a Value submission, or fallback when all are complete.
func firstIncompleteInsertIndex(columns []qb.InsertColumn, fallback int) int {
	for i, c := range columns {
		if c.Choice() == qb.InsertChoiceNone {
			return i
		}
		if c.Choice() == qb.InsertChoiceValue {
			if _, ok := c.SubmittedValue(); !ok {
				return i
			}
		}
	}
	return fallback
}

// beginInsertEdit is the Insert field's Enter opener: with zero prompt states
// (a zero-insertable-column table) it shows the exact blocking reason
// instead of opening any popup; otherwise it opens the shared choice popup
// on the first incomplete column.
func (m *Model) beginInsertEdit() {
	columns := m.QB.InsertColumns()
	if len(columns) == 0 {
		m.showRunnableReason(qb.ReasonNoInsertableColumns)
		return
	}
	m.beginInsertChoice(firstIncompleteInsertIndex(columns, m.insertCursor))
}

// beginInsertChoice installs the shared scroll-only choice popup for the
// column at index, restoring the highlight onto the column's current choice
// so revision shows the exact prior state.
func (m *Model) beginInsertChoice(index int) {
	columns := m.QB.InsertColumns()
	if index < 0 || index >= len(columns) {
		return
	}
	m.insertCursor = index
	rows := make([]PopupCandidate, 0, len(insertChoices))
	for _, choice := range insertChoices {
		rows = append(rows, PopupCandidate{ID: choice.name, Display: choice.name})
	}
	m.installPopup(NewScrollOnlyPopup(insertFieldLabel, rows), insertChoiceAccepted)
	m.Popup.SetViewportHeight(insertPopupViewport)
	for i, choice := range insertChoices {
		if choice.choice == columns[index].Choice() {
			for range i {
				m.Popup.Down()
			}
			break
		}
	}
}

// insertChoiceStatus renders the popup status line identifying the prompted
// column, including the exact INTEGER PRIMARY KEY omission hint when — and
// only when — the schema metadata says the column auto-assigns on omission.
func (m *Model) insertChoiceStatus() []string {
	columns := m.QB.InsertColumns()
	if m.Popup == nil || m.Popup.Opener != insertFieldLabel || m.Popup.Multi || m.insertCursor < 0 || m.insertCursor >= len(columns) {
		return nil
	}
	line := "Column: " + columns[m.insertCursor].Column
	if hint, ok := m.QB.InsertPromptHint(columns[m.insertCursor].Column); ok {
		line += " " + hint
	}
	return []string{line}
}

func insertChoiceAccepted(m *Model, id string) {
	columns := m.QB.InsertColumns()
	if m.insertCursor < 0 || m.insertCursor >= len(columns) {
		return
	}
	column := columns[m.insertCursor].Column
	for _, option := range insertChoices {
		if option.name != id {
			continue
		}
		if option.choice == qb.InsertChoiceValue {
			// Universal entry opens only for Value, seeded with the exact
			// prior entered representation on revision.
			seed := ""
			if entered, ok := columns[m.insertCursor].Entered(); ok {
				seed = entered
			}
			m.ValuePrompt = NewValuePrompt(insertFieldLabel, column+" = Value", seed)
			return
		}
		next, ok := m.QB.ChooseInsertColumn(column, option.choice)
		if ok {
			m.applyBuilder(next)
			refocusField(m, insertFieldLabel)
			m.advanceInsertChoice()
		}
		return
	}
}

func (m *Model) insertValueAccepted(text string) {
	columns := m.QB.InsertColumns()
	if m.insertCursor < 0 || m.insertCursor >= len(columns) {
		return
	}
	column := columns[m.insertCursor].Column
	next, ok := m.QB.ChooseInsertColumn(column, qb.InsertChoiceValue)
	if !ok {
		return
	}
	next, ok = next.SubmitInsertValue(column, text)
	if !ok {
		return
	}
	m.applyBuilder(next)
	refocusField(m, insertFieldLabel)
	m.advanceInsertChoice()
}

// advanceInsertChoice opens the next column's choice popup in schema order,
// or returns focus to the Insert field once every prompt is complete.
func (m *Model) advanceInsertChoice() {
	if m.insertCursor+1 < len(m.QB.InsertColumns()) {
		m.beginInsertChoice(m.insertCursor + 1)
		return
	}
	refocusField(m, insertFieldLabel)
}

// clearCurrentInsertValue drops the whole submitted value of the column the
// insert cursor sits on, preserving its Value choice and column identity.
// Columns without a submitted Value are unchanged no-ops.
func (m *Model) clearCurrentInsertValue() {
	columns := m.QB.InsertColumns()
	if m.insertCursor < 0 || m.insertCursor >= len(columns) {
		return
	}
	m.applyBuilder(m.QB.ClearInsertValue(columns[m.insertCursor].Column))
	refocusField(m, insertFieldLabel)
}
