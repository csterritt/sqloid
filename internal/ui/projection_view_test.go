// Focused rendering assertions for the Issue #15 projection popups: exact
// candidate ordering and highlight markers, the search-less scroll-only
// aggregate variant, and the absence of any unsupported aggregate-on-wildcard
// choice. Pure text rendering checks on top of the Issue #12 popup
// presentation contract; no interaction logic here.

package ui

import (
	"slices"
	"strings"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// columnsCandidateRows installs a searchable Column(s) popup carrying the
// given QueryBuilder candidates and returns its rendered bordered box.
func columnsCandidateRows(t *testing.T, cands []qb.ProjectionCandidate) string {
	t.Helper()
	m := baseSized(t)
	var pcs []PopupCandidate
	for _, c := range cands {
		pcs = append(pcs, PopupCandidate{ID: c.Key(), Display: c.Display()})
	}
	m.installPopup(NewSearchablePopup(columnsFieldLabel, pcs), nil)
	return RenderPopup(m.Popup, 60, 12)
}

// rowsIn extracts ordered candidate-row texts from a rendered popup box,
// reading each line's interior between the box borders. Highlighted rows
// keep their content after stripping the "> " caret marker.
func rowsIn(view string) []string {
	var out []string
	for _, line := range strings.Split(view, "\n") {
		start := strings.Index(line, "\u2502")
		if start < 0 {
			continue // top/bottom border rows own no candidate text
		}
		parts := strings.Split(line[start:], "\u2502")
		if len(parts) < 3 {
			continue
		}
		inner := strings.TrimSpace(parts[1])
		inner = strings.TrimPrefix(inner, popupSelectedPrefix)
		if inner == "" || strings.HasPrefix(inner, popupSearchPrompt) ||
			inner == NoMatchesMessage {
			continue
		}
		out = append(out, inner)
	}
	return out
}

// TestRenderEmptyProjectionOrderPinsWildcardHighlight requires the empty
// Column(s) rendering to show `*` as the highlighted first row, bare COUNT(*)
// second, and named rows after both.
func TestRenderEmptyProjectionOrderPinsWildcardHighlight(t *testing.T) {
	view := columnsCandidateRows(t, []qb.ProjectionCandidate{
		{Kind: qb.ProjectionWildcard},
		{Kind: qb.ProjectionCountStar},
		{Kind: qb.ProjectionColumn, Column: "id"},
	})
	rows := rowsIn(view)
	want := []string{"*", "COUNT(*)", "id"}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows=%v (%s), want %v", rows, view, want)
	}
	if !strings.Contains(view, popupSelectedPrefix+"*") {
		t.Errorf("wildcard row not default-highlighted:\n%s", view)
	}
}

// TestRenderAggregatePopupIsScrollOnlyWithoutSearch requires the aggregate
// popup to render every choice in order without any Search: line and without
// any aggregate-on-wildcard option.
func TestRenderAggregatePopupIsScrollOnlyWithoutSearch(t *testing.T) {
	var pcs []PopupCandidate
	for _, name := range []string{"Value", "Count", "Min", "Max", "Avg", "Sum"} {
		pcs = append(pcs, PopupCandidate{ID: name, Display: name})
	}
	m := baseSized(t)
	m.installPopup(NewScrollOnlyPopup(columnsFieldLabel, pcs), nil)
	view := RenderPopup(m.Popup, 60, 12)
	if strings.Contains(view, "Search:") {
		t.Errorf("scroll-only aggregate popup rendered a search input:\n%s", view)
	}
	if got := rowsIn(view); !slices.Equal(got, []string{"Value", "Count", "Min", "Max", "Avg", "Sum"}) {
		t.Errorf("aggregate rows=%v (%s), want Value/Count/Min/Max/Avg/Sum in order", got, view)
	}
	for _, bad := range []string{"MIN(*)", "MAX(*)", "AVG(*)", "SUM(*)"} {
		if strings.Contains(view, bad) {
			t.Errorf("unsupported wildcard aggregate %q rendered:\n%s", bad, view)
		}
	}
}

// TestRenderSentinelLikeColumnsKeepDistinctRows requires real columns named
// `*` and `COUNT(*)` to render under their own rows below the synthetic pair.
func TestRenderSentinelLikeColumnsKeepDistinctRows(t *testing.T) {
	view := columnsCandidateRows(t, []qb.ProjectionCandidate{
		{Kind: qb.ProjectionWildcard},
		{Kind: qb.ProjectionCountStar},
		{Kind: qb.ProjectionColumn, Column: "*"},
		{Kind: qb.ProjectionColumn, Column: "COUNT(*)"},
	})
	rows := rowsIn(view)
	want := []string{"*", "COUNT(*)", "*", "COUNT(*)"}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows=%v (%s), want %v with the synthetic pair first", rows, view, want)
	}
}
