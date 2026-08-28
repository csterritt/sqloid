// GROUP BY assisted multi-selection for Issue #18: the builder's Group By
// field opens a searchable popup whose candidates come from the QueryBuilder's
// grouping rules. Each accepted candidate commits immediately through
// AcceptGroupColumn and the popup reopens fresh with the remaining committed
// columns excluded, so the assisted selection can continue column by column.
// The UI never encodes grouping validity: the QueryBuilder owns eligibility,
// duplicate rejection, and SQL rendering, while this layer owns exact focus,
// acceptance, and cancellation.

package ui

import (
	"strings"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// groupByFieldLabel is the field-bar label of the GROUP BY field; it also
// names the popup opener identity so accept/cancel restore that exact focus.
const groupByFieldLabel = "Group By"

// groupByFocused reports whether the field bar currently has Group By
// focused, guarding against suspension and open overlays like tableFocused.
func (m *Model) groupByFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == groupByFieldLabel
}

// beginGroupByPopup installs the fresh searchable GROUP BY popup over the
// focused Group By field whenever it opens. Candidates derive from
// QueryBuilder.GroupByCandidates, which excludes committed columns so the
// multi-selection can never offer a duplicate. No database work is issued.
func (m *Model) beginGroupByPopup() {
	if !m.groupByFocused() {
		return
	}
	m.installPopup(NewSearchablePopup(groupByFieldLabel, groupByPopupCandidates(m.QB)),
		groupByAcceptHook)
	m.Popup.SetViewportHeight(columnsPopupViewport)
}

// reopenGroupByPopup returns exact UI focus to the Group By field and reopens
// the popup fresh with deterministic reset semantics per the reusable Issue
// #12 contract: search cleared, highlight on the first visible candidate,
// viewport at its top. Committed columns stay excluded through the builder.
func (m *Model) reopenGroupByPopup() {
	if m.Popup != nil {
		m.Popup = nil
		m.popupAccept = nil
	}
	for i := range m.Fields {
		if m.Fields[i].Label == groupByFieldLabel {
			m.setFocus(i)
			break
		}
	}
	m.beginGroupByPopup()
}

// groupByPopupCandidates converts the builder's typed GROUP BY choices into
// popup rows keyed by declared column name, in Schema order.
func groupByPopupCandidates(q qb.QueryBuilder) []PopupCandidate {
	names := q.GroupByCandidates()
	out := make([]PopupCandidate, 0, len(names))
	for _, n := range names {
		out = append(out, PopupCandidate{ID: n, Display: n})
	}
	return out
}

// groupByAcceptHook commits one accepted column through the builder's
// AcceptGroupColumn transition and reopens the popup for the next choice.
// A rejected acceptance — a duplicate or an identity no longer visible —
// reopens unchanged as an immutable no-op rather than committing anything.
func groupByAcceptHook(m *Model, id string) {
	next, ok := m.QB.AcceptGroupColumn(id)
	m.applyBuilder(next)
	if ok {
		m.reopenGroupByPopup()
		return
	}
	// Rejected acceptance: restore the popup exactly as before so the user
	// can pick a different candidate without losing the selection flow.
	m.beginGroupByPopup()
}

// removeLatestGroup deletes exactly one committed GROUP BY column — the most
// recently accepted — per press from the focused base Group By field,
// mirroring the Column(s) base-field removal contract.
func removeLatestGroup(m *Model) {
	m.applyBuilder(m.QB.RemoveLatestGroup())
	refocusField(m, groupByFieldLabel)
}

// groupByEntryLabels renders the committed GROUP BY columns in selection
// order, comma-joined; empty when none are committed.
func groupByEntryLabels(entries []string) string {
	return strings.Join(entries, ", ")
}
