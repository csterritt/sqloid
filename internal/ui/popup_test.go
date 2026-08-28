// Pure popup-state tests for the reusable searchable and scroll-only
// candidate lists, per Issue #12 Task 1: case-insensitive subsequence
// matching, empty/no-match states, viewport scrolling at both boundaries, and
// deterministic highlight/viewport reset whenever the search text changes.
// Independent of QueryBuilder fields and database access; no later column,
// grouping, aggregate, or operator behavior is prescribed here.

package ui

import (
	"slices"
	"testing"
)

func candidates(displays ...string) []PopupCandidate {
	out := make([]PopupCandidate, len(displays))
	for i, d := range displays {
		out[i] = PopupCandidate{ID: d, Display: d}
	}
	return out
}

func ids(p *Popup) []string {
	var out []string
	for _, c := range p.Visible() {
		out = append(out, c.ID)
	}
	return out
}

// TestSubsequenceMatchingIsCaseInsensitive pins the exact matching rule on
// which every searchable popup relies: lower-cased runes of the query appear
// in display order anywhere inside the lower-cased display text.
func TestSubsequenceMatchingIsCaseInsensitive(t *testing.T) {
	cases := []struct {
		query   string
		display string
		want    bool
	}{
		{"", "anything", true},
		{"users", "USERS", true},
		{"usr", "Users", true},          // subsequence across gaps
		{"ures", "Users", false},        // wrong order
		{"logs", "_cf_METADATA", false}, // absent runes
		{"s", "", false},
	}
	for _, tc := range cases {
		if got := matchesSubsequence(tc.query, tc.display); got != tc.want {
			t.Errorf("matchesSubsequence(%q, %q)=%v, want %v", tc.query, tc.display, got, tc.want)
		}
	}
}

// TestEmptySearchShowsAllCandidatesInSourceOrder requires an empty search to
// present every candidate, including repeated case variants with distinct
// identities, in the original source order.
func TestEmptySearchShowsAllCandidatesInSourceOrder(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("Logs", "LOGS", "Events"))
	got := ids(p)
	want := []string{"Logs", "LOGS", "Events"}
	if !slices.Equal(got, want) {
		t.Errorf("visible=%v, want %v", got, want)
	}
	if p.MatchCount() != 3 || p.NoMatch() {
		t.Errorf("matchCount=%d noMatch=%v, want 3/false with empty search", p.MatchCount(), p.NoMatch())
	}
}

// TestNonmatchingSearchKeepsPopupOpenWithExactNoMatches covers the no-match
// contract: the popup stays open, exposes zero matches, and reports the
// exact `no matches` state rather than closing or inventing entries.
func TestNonmatchingSearchKeepsPopupOpenWithExactNoMatches(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("alpha", "beta"))
	p.SetSearch("zzz")
	if !p.Open() {
		t.Fatal("nonmatching search closed the popup, want open")
	}
	if p.MatchCount() != 0 || !p.NoMatch() {
		t.Errorf("matchCount=%d noMatch=%v, want 0/true", p.MatchCount(), p.NoMatch())
	}
	if msgs := p.StatusMessages(); !slices.Equal(msgs, []string{NoMatchesMessage}) {
		t.Errorf("statusMessages=%v, want exactly [%q]", msgs, NoMatchesMessage)
	}
}

// TestEmptyCandidatesAlwaysReportNoMatches requires empty candidate data to be
// a permanent no-match open state regardless of the search text.
func TestEmptyCandidatesAlwaysReportNoMatches(t *testing.T) {
	for _, q := range []string{"", "a"} {
		p := NewScrollOnlyPopup("Aggregates", nil)
		p.SetSearch(q)
		if !p.Open() || !p.NoMatch() || p.MatchCount() != 0 {
			t.Errorf("empty candidates q=%q: open=%v noMatch=%v count=%d",
				q, p.Open(), p.NoMatch(), p.MatchCount())
		}
	}
}

// TestSearchFiltersPreservingSourceOrder exercises subsequence filtering with
// repeated case variants and requires the source ordering to survive.
func TestSearchFiltersPreservingSourceOrder(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("Orders", "order_items", "Customers", "invoice_logs"))
	p.SetSearch("rr")
	want := []string{"Orders", "order_items"}
	if got := ids(p); !slices.Equal(got, want) {
		t.Errorf("'rr' -> %v, want %v", got, want)
	}
	p.SetSearch("ilog")
	if got := ids(p); !slices.Equal(got, []string{"invoice_logs"}) {
		t.Errorf("'ilog' -> %v, want [invoice_logs]", got)
	}
	p.SetSearch("")
	if got := ids(p); len(got) != 4 {
		t.Errorf("cleared search restored %d items, want all four", len(got))
	}
}

// TestSearchChangeResetsHighlightAndViewportDeterministically requires every
// actual search-text change to snap the highlight back to the first visible
// result and the viewport back to its top, including changes made while
// scrolled deep into a long list.
func TestSearchChangeResetsHighlightAndViewportDeterministically(t *testing.T) {
	long := []PopupCandidate{}
	for i := 0; i < 30; i++ {
		name := "row_02d_" + string(rune('a'+i%26))
		long = append(long, PopupCandidate{ID: name, Display: name})
	}
	p := NewSearchablePopup("Column", long)
	p.SetViewportHeight(5)
	for i := 0; i < 20; i++ {
		p.Down()
	}
	if p.highlightIndex != 20 || p.viewportTop != 16 {
		t.Fatalf("setup: highlight=%d top=%d, want 20/16", p.highlightIndex, p.viewportTop)
	}
	p.AppendSearchRune('0') // appending is itself a search-text change
	for i := 0; i < 4; i++ {
		p.Down()
	}
	p.BackspaceSearch() // clearing back to empty resets again
	if p.highlightIndex != 0 || p.viewportTop != 0 {
		t.Errorf("cleared-search reset gave highlight=%d top=%d, want 0/0", p.highlightIndex, p.viewportTop)
	}
	for i := 0; i < 3; i++ {
		p.Down()
	}
	p.SetSearch("row")
	if p.highlightIndex != 0 || p.viewportTop != 0 {
		t.Errorf("new-search reset gave highlight=%d top=%d, want 0/0", p.highlightIndex, p.viewportTop)
	}
	// Appending a rune is itself a search change and must reset again.
	for i := 0; i < 10; i++ {
		p.Down()
	}
	p.AppendSearchRune('2')
	if p.highlightIndex != 0 || p.viewportTop != 0 {
		t.Errorf("append reset gave highlight=%d top=%d, want 0/0", p.highlightIndex, p.viewportTop)
	}
}

// TestAppendAndBackspaceMutateSearchText pins the incremental editing surface
// used by keyboard input on searchable popups.
func TestAppendAndBackspaceMutateSearchText(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("Users", "Roles"))
	p.AppendSearchRune('r')
	p.AppendSearchRune('S')
	if p.Search != "rS" {
		t.Fatalf("search=%q after appends, want rS", p.Search)
	}
	if got := ids(p); !slices.Equal(got, []string{"Users", "Roles"}) {
		t.Errorf("rS visible=%v, want case-insensitive rs matches in source order", got)
	}
	p.BackspaceSearch()
	if p.Search != "r" {
		t.Fatalf("backspace left search=%q, want r", p.Search)
	}
}

// TestViewportScrollingAtBothBoundaries drives a list longer than the
// viewport across both extremes: Down stops at the last item with the
// viewport pinned to the bottom, Up stops at the first with the viewport
// pinned to the top, and the highlighted item always stays visible.
func TestViewportScrollingAtBothBoundaries(t *testing.T) {
	items := candidates("a", "b", "c", "d", "e", "f")
	p := NewScrollOnlyPopup("Operators", items)
	p.SetViewportHeight(2)
	for i := 0; i < 50; i++ {
		p.Down()
	}
	if p.highlightIndex != 5 || p.viewportTop != 4 {
		t.Errorf("bottom boundary highlight=%d top=%d, want 5/4", p.highlightIndex, p.viewportTop)
	}
	for i := 0; i < 50; i++ {
		p.Up()
	}
	if p.highlightIndex != 0 || p.viewportTop != 0 {
		t.Errorf("top boundary highlight=%d top=%d, want 0/0", p.highlightIndex, p.viewportTop)
	}
}

// TestHighlightedItemAlwaysVisibleAfterStep requires stepping to scroll the
// viewport exactly when the highlight leaves it, keeping the highlight within
// the visible window after each single move.
func TestHighlightedItemAlwaysVisibleAfterStep(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("t1", "t2", "t3", "t4", "t5", "t6"))
	p.SetViewportHeight(3)
	for step := 0; step < 6; step++ {
		p.Down()
		if p.highlightIndex < p.viewportTop || p.highlightIndex >= p.viewportTop+3 {
			t.Fatalf("step %d: highlight=%d outside viewport [%d,%d)",
				step, p.highlightIndex, p.viewportTop, p.viewportTop+3)
		}
	}
	for step := 0; step < 6; step++ {
		p.Up()
		if p.highlightIndex < p.viewportTop || p.highlightIndex >= p.viewportTop+3 {
			t.Fatalf("up step %d: highlight=%d outside viewport [%d,%d)",
				step, p.highlightIndex, p.viewportTop, p.viewportTop+3)
		}
	}
}

// TestScrollOnlyVariantIgnoresSearchRequiresAllItems pins the scroll-only
// contract: no search input exists, printable attempts never filter, and the
// full source list remains navigable.
func TestScrollOnlyVariantIgnoresSearchRequiresAllItems(t *testing.T) {
	p := NewScrollOnlyPopup("Operator", candidates("=", "!=", "<", ">"))
	p.SetSearch("=")
	p.AppendSearchRune('=')
	if p.Search != "" {
		t.Errorf("scroll-only popup accumulated search %q, want empty", p.Search)
	}
	if got := ids(p); !slices.Equal(got, []string{"=", "!=", "<", ">"}) {
		t.Errorf("scroll-only visible=%v, want unfiltered source order", got)
	}
	if p.Down(); p.highlightIndex != 1 {
		t.Errorf("scroll-only Down gave highlight=%d, want 1", p.highlightIndex)
	}
}

// TestSearchableWithoutViewportShowsEverythingUnwindowed pins that a popup
// with no explicit viewport height behaves as fully visible with static top.
func TestSearchableWithoutViewportShowsEverythingUnwindowed(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("a", "b", "c"))
	p.Down()
	p.Down()
	if p.highlightIndex != 2 || p.viewportTop != 0 {
		t.Errorf("unwindowed highlight=%d top=%d, want 2/0", p.highlightIndex, p.viewportTop)
	}
}

// TestSingleSelectEnterAcceptsHighlightedCandidate pins single-select Enter:
// it yields exactly one accepted commit carrying the highlighted candidate
// ID, and Enter over empty/no-match lists is ignored.
func TestSingleSelectEnterAcceptsHighlightedCandidate(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("users", "logs"))
	p.Down()
	if r := p.Enter(); r.Outcome != EnterAccepted || r.ID != "logs" {
		t.Errorf("Enter gave %+v, want Accepted/logs", r)
	}
	empty := NewSearchablePopup("Table", candidates())
	if r := empty.Enter(); r.Outcome != EnterNone {
		t.Errorf("Enter on empty candidates gave %+v, want None", r)
	}
	varied := NewSearchablePopup("Table", candidates("users"))
	varied.SetSearch("zz")
	if r := varied.Enter(); r.Outcome != EnterNone || !varied.Open() {
		t.Errorf("Enter under no-match gave %+v open=%v, want None/open", r, varied.Open())
	}
}

// TestMultiSelectEnterAddsReopensAndDeduplicates pins multi-select Enter:
// each press adds a nonduplicate completion and keeps the popup open for
// another choice; repeating a completed candidate changes nothing.
func TestMultiSelectEnterAddsReopensAndDeduplicates(t *testing.T) {
	p := NewMultiSearchablePopup("Columns", candidates("a", "b"))
	if r := p.Enter(); r.Outcome != EnterAdded || r.ID != "a" {
		t.Fatalf("first Enter gave %+v, want Added/a", r)
	}
	if !p.Open() {
		t.Fatal("multi-select Enter closed the popup, want reopened for another choice")
	}
	p.Down()
	if r := p.Enter(); r.Outcome != EnterAdded || r.ID != "b" {
		t.Errorf("second Enter gave %+v, want Added/b", r)
	}
	p.Up() // back to "a", already completed
	if r := p.Enter(); r.Outcome != EnterDuplicate || r.ID != "a" {
		t.Errorf("repeat Enter gave %+v, want Duplicate/a", r)
	}
	want := []string{"a", "b"}
	if got := p.Completed(); !slices.Equal(got, want) {
		t.Errorf("completed=%v, want %v insertion order without duplicates", got, want)
	}
}

// TestMultiSelectSearchChangeKeepsCompletedRequiresReset pins that completed
// multi-selections survive any filtering (including temporary no-match
// searches) while highlight/viewport reset rules still apply to navigation.
func TestMultiSelectSearchChangeKeepsCompletedRequiresReset(t *testing.T) {
	p := NewMultiSearchablePopup("Columns", candidates("alpha", "beta"))
	p.Enter()
	p.SetSearch("zzz")
	if got := p.Completed(); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("completed during no-match filter=%v, want [alpha]", got)
	}
	p.SetSearch("")
	if p.highlightIndex != 0 || p.viewportTop != 0 {
		t.Errorf("reset broken after completed selection: highlight=%d top=%d", p.highlightIndex, p.viewportTop)
	}
}

// TestEscDiscardsUnfinishedWorkPreservesCompleted pins the cancel path: Esc
// discards unfinished work and reports closed, yet Completed remains intact
// for the caller to preserve across reopening.
func TestEscDiscardsUnfinishedWorkPreservesCompleted(t *testing.T) {
	p := NewMultiSearchablePopup("Columns", candidates("a", "b", "c"))
	p.Down()
	p.Enter() // completes "b"
	p.SetSearch("xyz")
	p.Esc()
	if p.Open() {
		t.Error("Esc left the popup open")
	}
	if got := p.Completed(); !slices.Equal(got, []string{"b"}) {
		t.Errorf("completed after Esc=%v, want [b] preserved", got)
	}
}

// TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight requires Enter
// to accept whatever row the caret points at after independent scrolling and
// filtering journeys, never silently reverting to a stale item.
func TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight(t *testing.T) {
	p := NewSearchablePopup("Column", candidates("c1", "c2", "c3", "c4", "c5"))
	p.SetViewportHeight(2)
	for i := 0; i < 10; i++ {
		p.Down()
	}
	if r := p.Enter(); r.Outcome != EnterAccepted || r.ID != "c5" {
		t.Errorf("scroll-path Enter gave %+v, want Accepted/c5", r)
	}
	p.SetSearch("3")
	if r := p.Enter(); r.Outcome != EnterAccepted || r.ID != "c3" {
		t.Errorf("filter-path Enter gave %+v, want Accepted/c3", r)
	}
}
