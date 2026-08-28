// Popup rendering tests for Issue #12 Task 1: exact presentation of the
// search-input line, highlighted versus plain candidate rows, the `no
// matches` state, viewport windowing, and the scroll-only variant without a
// search input. Overlay composition must draw over existing regions without
// reflowing them.

package ui

import (
	"strings"
	"testing"
)

// TestRenderSearchablePopupShowsSearchLineAndRows pins the searchable layout:
// one `Search: <text>_` line above the candidate rows, "> " prefix on the
// highlighted row, "  " on the others.
func TestRenderSearchablePopupShowsSearchLineAndRows(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("users", "logs", "orders"))
	out := RenderPopup(p, 30, 8)
	if !strings.Contains(out, "Search: _") {
		t.Errorf("rendered popup lacks empty-search input line:\n%s", out)
	}
	if !strings.Contains(out, "> users") || !strings.Contains(out, "  logs") || !strings.Contains(out, "  orders") {
		t.Errorf("rendered popup lacks highlighted/plain candidate rows:\n%s", out)
	}
}

// TestRenderPopupNoMatchesState requires an exhausted filter to render exactly
// `no matches` in place of any candidate row while remaining visibly open.
func TestRenderPopupNoMatchesState(t *testing.T) {
	p := NewSearchablePopup("Table", candidates("users"))
	p.SetSearch("zzz")
	out := RenderPopup(p, 30, 6)
	if !strings.Contains(out, "no matches") {
		t.Errorf("rendered popup lacks exact no-matches text:\n%s", out)
	}
	if strings.Contains(out, "users") {
		t.Errorf("no-match view still lists stale candidates:\n%s", out)
	}
}

// TestRenderPopupReflectsSearchChangeReset pins that rendering follows the
// deterministic highlight reset after a search change: the previously
// scrolled highlight is gone and the new first match carries the caret.
func TestRenderPopupReflectsSearchChangeReset(t *testing.T) {
	long := []PopupCandidate{}
	for _, n := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		long = append(long, PopupCandidate{ID: n, Display: n})
	}
	p := NewSearchablePopup("Column", long)
	p.SetViewportHeight(3)
	for i := 0; i < 5; i++ {
		p.Down()
	}
	if got := RenderPopup(p, 30, 8); !strings.Contains(got, "> zeta") {
		t.Fatalf("scrolled view lost highlight:\n%s", got)
	}
	p.SetSearch("gam")
	got := RenderPopup(p, 30, 6)
	if !strings.Contains(got, "> gamma") || !strings.Contains(got, "Search: gam_") {
		t.Errorf("view after search change shows:\n%s\nwant highlighted gamma with Search: gam_", got)
	}
}

// TestRenderPopupWindowRespectsViewportHeight requires the rendered candidate
// block to expose exactly the visible window of a longer list, in order.
func TestRenderPopupWindowRespectsViewportHeight(t *testing.T) {
	items := candidates("c1", "c2", "c3", "c4", "c5")
	p := NewScrollOnlyPopup("Aggregates", items)
	p.SetViewportHeight(2)
	p.Down()
	p.Down()
	out := RenderPopup(p, 30, 4)
	for _, shown := range []string{"  c2", "> c3"} {
		if !strings.Contains(out, shown) {
			t.Errorf("windowed view lacks %q:\n%s", shown, out)
		}
	}
	for _, hidden := range []string{"c1", "c4"} {
		if strings.Contains(out, hidden) {
			t.Errorf("windowed view leaked off-screen item %q:\n%s", hidden, out)
		}
	}
}

// TestRenderScrollOnlyPopupHasNoSearchInput pins that scroll-only variants
// never present a search-input modality.
func TestRenderScrollOnlyPopupHasNoSearchInput(t *testing.T) {
	p := NewScrollOnlyPopup("Operator", candidates("=", "<"))
	if out := RenderPopup(p, 20, 4); strings.Contains(out, "Search:") {
		t.Errorf("scroll-only variant rendered a search input:\n%s", out)
	}
}

// TestComposeOverlayNeverReflowsBase pins the overlay composition contract:
// the overlay is spliced over the base at its coordinates and every untouched
// base row keeps its exact previous content and total base dimensions.
func TestComposeOverlayNeverReflowsBase(t *testing.T) {
	base := strings.Join([]string{
		"row0",
		"row1",
		"row2",
	}, "\n")
	overlay := strings.Join([]string{"XXXXXXXX", "YYYYYYYY"}, "\n")
	got := composeOverlay(base, overlay, 1, 0)
	want := strings.Join([]string{"row0", "XXXXXXXX", "YYYYYYYY"}, "\n")
	if got != want {
		t.Errorf("composed overlay=%q, want %q (no reflow)", got, want)
	}
	full := strings.Repeat("a", 10)
	if inside := composeOverlay(full, full, 0, 0); len(strings.Split(inside, "\n")) != 1 {
		t.Errorf("single-line base changed height under full overlay: %q", inside)
	}
}
