// Table popup integration, Issue #12 Task 4: the builder's Table field is
// the first end-to-end searchable single-select popup consumer. Candidates
// come from the refreshed Schema catalog already held by QueryBuilder,
// filtered by its eligibility rules — the UI never duplicates them. Enter
// commits accepted object identity through QueryBuilder transitions and
// restores the exact opener focus; Esc closes unchanged.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
)

// tablePopupViewport caps how many table candidates are visible at once; a
// longer eligible list scrolls within this window.
const tablePopupViewport = 8

// beginTablePopup installs the fresh searchable single-select Table popup
// over the focused Table field whenever it opens, and issues the mandatory
// fresh main-schema catalog request (Issue #13) whose settled result later
// replaces the offered candidates. Eligibility filtering follows the
// currently selected command through QueryBuilder's own rule set. The
// returned command performs the request; nil when nothing is wired.
func (m *Model) beginTablePopup() tea.Cmd {
	if !m.tableFocused() {
		return nil
	}
	m.installPopup(NewSearchablePopup(tableFieldLabel, popupCandidates(m.QB.EligibleTables())),
		func(mm *Model, id string) {
			// Commit identity through the builder transition; applyBuilder
			// re-renders fields from the authoritative snapshot.
			mm.applyBuilder(mm.QB.SelectTable(id))
		})
	m.Popup.SetViewportHeight(tablePopupViewport)
	return m.issueRefresh()
}

// popupCandidates converts refreshed schema objects into popup candidates:
// identity is exactly the cataloged object name, so acceptance commits that
// name back through QueryBuilder without reinterpreting eligibility here.
func popupCandidates(objs []*schema.Object) []PopupCandidate {
	out := make([]PopupCandidate, 0, len(objs))
	for _, o := range objs {
		out = append(out, PopupCandidate{ID: o.Name, Display: o.Name})
	}
	return out
}

// tableFocused reports whether the field bar currently has Table focused.
func (m *Model) tableFocused() bool {
	if m.suspended || m.Popup != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == tableFieldLabel
}

// openPopupCmd installs the context-appropriate popup when msg opens it and
// returns the popup's issued work as a tea.Cmd — the mandatory fresh schema
// catalog request for the Table popup, nil otherwise.
func (m *Model) openPopupCmd(msg tea.KeyMsg) tea.Cmd {
	if msg.Type != tea.KeyEnter {
		return nil
	}
	if m.columnsFocused() {
		return m.beginColumnsPopup()
	}
	if m.groupByFocused() {
		m.beginGroupByPopup()
		return nil
	}
	if m.orderByFocused() {
		m.beginOrderByPopup()
		return nil
	}
	if m.limitFocused() {
		m.beginLimitPrompt()
		return nil
	}
	if m.whereFocused() {
		m.beginWhereEdit()
		return nil
	}
	return m.beginTablePopup()
}
