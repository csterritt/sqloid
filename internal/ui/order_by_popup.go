// ORDER BY popup integration for Issue #18: the builder's Order By field
// opens a searchable popup whose candidates come from the QueryBuilder's
// context-derived ordering rules, so only table columns or grouped columns
// and selected aggregate expressions are ever offered. The UI never encodes
// candidate eligibility: the QueryBuilder owns identity, defaults, and SQL,
// while this layer owns exact focus, acceptance, cancellation, base-field
// Up/Down direction toggling, and whole-value clearing.

package ui

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// orderByFieldLabel is the field-bar label of the ORDER BY field; it also
// names the popup opener identity so accept/cancel restore that exact focus.
const orderByFieldLabel = "Order By"

// orderByFocused reports whether the field bar currently has Order By
// focused, guarding against suspension and open overlays like tableFocused.
func (m *Model) orderByFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == orderByFieldLabel
}

// beginOrderByPopup installs the fresh searchable ORDER BY popup over the
// focused Order By field whenever it opens. Candidates derive from
// QueryBuilder.OrderByCandidates so context-valid expressions — grouped
// columns and selected aggregates in grouped contexts, plain table columns
// otherwise — follow the builder's own state. No database work is issued.
func (m *Model) beginOrderByPopup() {
	if !m.orderByFocused() {
		return
	}
	m.installPopup(NewSearchablePopup(orderByFieldLabel, orderByPopupCandidates(m.QB)),
		orderByAcceptHook)
	m.Popup.SetViewportHeight(columnsPopupViewport)
}

// orderByPopupCandidates converts the builder's typed ORDER BY candidates
// into popup rows: identity is the candidate Key encoding, display is the
// candidate label, so equal labels (a column beside its own aggregate, or a
// literal column named like an aggregate call) keep distinct identities.
func orderByPopupCandidates(q qb.QueryBuilder) []PopupCandidate {
	cands := q.OrderByCandidates()
	out := make([]PopupCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, PopupCandidate{ID: c.Key, Display: c.Display})
	}
	return out
}

// orderByAcceptHook commits one accepted candidate through the builder's
// AcceptOrderBy transition — atomic replacement with the ASC default — and
// leaves the popup closed with focus restored to the Order By field. A key
// rejected by the builder never changes any state.
func orderByAcceptHook(m *Model, id string) {
	next, ok := m.QB.AcceptOrderBy(id)
	m.applyBuilder(next)
	for i := range m.Fields {
		if m.Fields[i].Label == orderByFieldLabel {
			m.setFocus(i)
			break
		}
	}
	if !ok {
		// Rejected acceptance: reopen unchanged so the flow can continue.
		m.beginOrderByPopup()
	}
}

// toggleOrderDirectionInBaseField applies the base Order By field's Up/Down
// toggle when a selection is committed; the closed ASC/DESC pair flips
// deterministically without opening the popup or moving popup selection.
// With nothing committed there is no direction to flip, so Up/Down fall
// through to ordinary focus navigation.
func toggleOrderDirectionInBaseField(m *Model) {
	if _, _, ok := m.QB.OrderBySelection(); ok {
		m.applyBuilder(m.QB.ToggleOrderDirection())
		refocusField(m, orderByFieldLabel)
	}
}

// clearOrderByField removes the whole committed ORDER BY selection from the
// focused base Order By field; an empty selection is unchanged.
func clearOrderByField(m *Model) {
	m.applyBuilder(m.QB.ClearOrderBy())
	refocusField(m, orderByFieldLabel)
}

// refocusField keeps focus on the labeled base field after an applyBuilder
// whose builder focus pointer points elsewhere (e.g. an older downstream
// requirement), so base-field edits never relocate the user's cursor.
func refocusField(m *Model, label string) {
	for i := range m.Fields {
		if m.Fields[i].Label == label {
			m.setFocus(i)
			return
		}
	}
}

// orderByFieldContent renders the committed selection as
// `expression DIRECTION`, or empty when nothing is committed or the stored
// identity is stale against the current grouping/projection context.
func orderByFieldContent(q qb.QueryBuilder) string {
	cand, dir, ok := q.OrderBySelection()
	if !ok {
		return ""
	}
	token, err := dir.SQLToken()
	if err != nil {
		return ""
	}
	return cand.Display + " " + token
}
