// Column(s) popup integration for Issue #15: the builder's Column(s) field
// opens a searchable single-select popup whose candidates come from the
// QueryBuilder projection rules (Issue #15). The QueryBuilder decides — via
// AcceptProjection/CompleteProjectionAggregate outcomes — whether acceptance
// commits the wildcard or bare COUNT(*) directly and reopens Column(s), or
// routes a named column into the scroll-only Value/Count/Min/Max/Avg/Sum
// aggregate popup. Candidate identity stays typed through ProjectionCandidate
// keys; the UI never synthesizes aggregate-on-wildcard choices and never
// duplicates projection rules.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// columnsPopupViewport caps how many column candidates are visible at once;
// a longer visible-column list scrolls within this window.
const columnsPopupViewport = 8

// columnsFieldLabel is the field-bar label of the SELECT Column(s) field; it
// also names the popup opener identity for both column and aggregate popups.
const columnsFieldLabel = "Column(s)"

// columnsFocused reports whether the field bar currently has Column(s)
// focused, guarding against suspension and open popups like tableFocused.
func (m *Model) columnsFocused() bool {
	if m.suspended || m.Popup != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == columnsFieldLabel
}

// beginColumnsPopup installs the fresh searchable single-select Column(s)
// popup over the focused Column(s) field whenever it opens. Candidates derive
// from QueryBuilder.ProjectionCandidates so conditional wildcard/sentinel
// visibility follows the builder's own state. No database work is issued.
func (m *Model) beginColumnsPopup() tea.Cmd {
	if !m.columnsFocused() {
		return nil
	}
	m.installPopup(NewSearchablePopup(columnsFieldLabel, projectionPopupCandidates(m.QB)),
		columnsAcceptHook)
	m.Popup.SetViewportHeight(columnsPopupViewport)
	return nil
}

// reopenColumnsPopup returns exact UI focus to the Column(s) field — the same
// opener any earlier popup was opened by, whatever its current index — and
// reopens the popup fresh with deterministic reset semantics per the reusable
// Issue #12 contract: search cleared, highlight on the first visible
// candidate, viewport at its top.
func (m *Model) reopenColumnsPopup() {
	if m.Popup != nil {
		m.Popup = nil
		m.popupAccept = nil
	}
	for i := range m.Fields {
		if m.Fields[i].Label == columnsFieldLabel {
			m.setFocus(i)
			break
		}
	}
	m.beginColumnsPopup()
}

// projectionPopupCandidates converts the builder's typed projection
// candidates into popup rows: identity is the candidate Key encoding, display
// is the candidate label, so a real column named `*` or `COUNT(*)` keeps its
// distinct identity even when its text collides with a synthetic row.
func projectionPopupCandidates(q qb.QueryBuilder) []PopupCandidate {
	cands := q.ProjectionCandidates()
	out := make([]PopupCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, PopupCandidate{ID: c.Key(), Display: c.Display()})
	}
	return out
}

// columnsAcceptHook commits one accepted Column(s) candidate. Wildcard and
// bare COUNT(*) commit directly through the builder transition; a reopened
// popup follows only when that transition requested it. A named column never
// commits here: it hands the chosen identity to the scroll-only aggregate
// step instead.
func columnsAcceptHook(m *Model, id string) {
	candidate, ok := lookupCandidate(m.QB, id)
	if !ok {
		return
	}
	if candidate.Kind == qb.ProjectionColumn {
		openAggregatePopup(m, candidate.Column)
		return
	}
	next := m.QB.AcceptProjection(candidate)
	reopen := next.ReopenColumns
	m.applyBuilder(next.Builder)
	if reopen {
		m.reopenColumnsPopup()
	}
}

// lookupCandidate resolves an accepted popup ID back to the typed candidate it
// named among the builder's current projection candidates; unknown IDs are
// rejected rather than guessed at.
func lookupCandidate(q qb.QueryBuilder, id string) (qb.ProjectionCandidate, bool) {
	for _, c := range q.ProjectionCandidates() {
		if c.Key() == id {
			return c, true
		}
	}
	return qb.ProjectionCandidate{}, false
}

// aggregateChoices are the six fixed names shown for every named column, in
// their stable presentation order.
var aggregateChoices = []struct {
	name string
	agg  qb.Aggregate
}{
	{"Value", qb.AggregateValue},
	{"Count", qb.AggCount},
	{"Min", qb.AggMin},
	{"Max", qb.AggMax},
	{"Avg", qb.AggAvg},
	{"Sum", qb.AggSum},
}

// openAggregatePopup installs the scroll-only Value/Count/Min/Max/Avg/Sum
// popup for one accepted named column. Only real column identities ever reach
// this path; no wildcard or sentinel flow can construct it.
func openAggregatePopup(m *Model, column string) {
	pcs := make([]PopupCandidate, 0, len(aggregateChoices))
	for _, ch := range aggregateChoices {
		pcs = append(pcs, PopupCandidate{ID: ch.name, Display: ch.name})
	}
	m.installPopup(NewScrollOnlyPopup(columnsFieldLabel, pcs),
		func(mm *Model, id string) {
			for _, ch := range aggregateChoices {
				if ch.name != id {
					continue
				}
				next := mm.QB.CompleteProjectionAggregate(column, ch.agg)
				mm.applyBuilder(next.Builder)
				if next.ReopenColumns {
					mm.reopenColumnsPopup()
				}
				return
			}
		})
	m.Popup.SetViewportHeight(columnsPopupViewport)
}

// projectionEntryLabels renders committed entries in order for the field bar:
// the bare labels `*` and `COUNT(*)`, plain column names, and
// `column(AGGREGATE)` for aggregated entries, comma-joined. Empty for an
// empty projection.
func projectionEntryLabels(entries []qb.ProjectionEntry) string {
	labels := make([]string, 0, len(entries))
	for _, e := range entries {
		switch e.Kind {
		case qb.ProjectionWildcard:
			labels = append(labels, "*")
		case qb.ProjectionCountStar:
			labels = append(labels, "COUNT(*)")
		default:
			label := e.Column
			if agg := e.Aggregate; agg > 0 && agg <= qb.AggSum {
				token, err := agg.SQLToken()
				if err == nil {
					label += "(" + token + ")"
				}
			}
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	out := labels[0]
	for _, l := range labels[1:] {
		out += ", " + l
	}
	return out
}
