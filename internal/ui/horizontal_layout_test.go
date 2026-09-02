// Pure whole-column horizontal layout coverage for the result grid
// (Issue #29 Tasks 1–2), per the UI Behavior, Module Design, Testing
// Decisions, and exact-horizontal-units sections of Notes/PRD-sqloid.md.
// The seam consumes only deduplicated output columns, rendered cell values,
// the available grid width, and a first-visible output-column index. Every
// pass recomputes visible columns and widths starting at the selected index,
// caps and ellipsizes a single oversized column to the available cell area,
// and exposes no character or byte offset that could permit intra-cell
// horizontal scrolling. Resize clamping is likewise pure: a valid
// first-visible index is preserved across column or width changes and an
// invalid one is clamped to the nearest valid boundary.

package ui

import (
	"testing"
)

func TestVisibleGridLayout(t *testing.T) {
	tests := []struct {
		name       string
		names      []string
		cells      [][]string
		availWidth int
		first      int
		wantFirst  int
		wantWidths []int
	}{
		{
			name:       "all columns fit on a wide terminal",
			names:      []string{"id", "name"},
			cells:      [][]string{{"1", "alice"}, {"2", "bob"}},
			availWidth: 40,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 5}, // "alice" is the widest "name" cell
		},
		{
			name:       "narrow terminal fits only the first column",
			names:      []string{"id", "name"},
			cells:      [][]string{{"1", "alice"}},
			availWidth: 8,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2},
		},
		{
			name:       "layout restarts at the selected first-visible index",
			names:      []string{"id", "name"},
			cells:      [][]string{{"1", "alice"}, {"2", "bob"}},
			availWidth: 40,
			first:      1,
			wantFirst:  1,
			wantWidths: []int{5},
		},
		{
			name:       "multiple columns fit and widths are recomputed per pass",
			names:      []string{"a", "bb", "ccc"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 13,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{1, 2, 3},
		},
		{
			name:       "exact-fit boundary packs the final column",
			names:      []string{"ab", "cd"},
			cells:      [][]string{{"x", "y"}},
			availWidth: 2 + gridSeparatorWidth + 2,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2},
		},
		{
			name:       "no room for another complete column excludes it",
			names:      []string{"ab", "cd"},
			cells:      [][]string{{"x", "y"}},
			availWidth: 2 + gridSeparatorWidth + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2},
		},
		{
			name:       "unicode display widths count double-width runes",
			names:      []string{"広告", "x"},
			cells:      [][]string{{"a", "b"}},
			availWidth: 40,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{4, 1},
		},
		{
			name:       "unicode columns pack by display width, not rune count",
			names:      []string{"広告", "x"},
			cells:      [][]string{{"a", "b"}},
			availWidth: 4 + gridSeparatorWidth + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{4, 1},
		},
		{
			name:       "oversized first column is capped to the available cell area",
			names:      []string{"long-column-header-that-will-not-fit", "x"},
			cells:      [][]string{{"also a very long cell value", "y"}},
			availWidth: 10,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{10},
		},
		{
			name:       "a column capped at the grid cap still packs following columns",
			names:      []string{"a-column-header-thirty-three-chars-wide!!", "y"},
			cells:      [][]string{{"x", "y"}},
			availWidth: 40,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{gridColumnCap, 1}, // 32 + sep + 1 = 36 fits in 40
		},
		{
			name:       "first index below zero clamps to the first column",
			names:      []string{"id", "v"},
			cells:      [][]string{{"1", "a"}},
			availWidth: 40,
			first:      -1,
			wantFirst:  0,
			wantWidths: []int{2, 1},
		},
		{
			name:       "first index beyond the last column clamps to it",
			names:      []string{"id", "v"},
			cells:      [][]string{{"1", "a"}},
			availWidth: 40,
			first:      5,
			wantFirst:  1,
			wantWidths: []int{1},
		},
		{
			name:       "empty results yield a zero layout at any index",
			names:      nil,
			cells:      nil,
			availWidth: 40,
			first:      7,
			wantFirst:  0,
			wantWidths: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := visibleGridLayout(tc.names, tc.cells, tc.availWidth, tc.first)
			if l.First != tc.wantFirst {
				t.Errorf("First = %d, want %d", l.First, tc.wantFirst)
			}
			if l.Total != len(tc.names) {
				t.Errorf("Total = %d, want %d", l.Total, len(tc.names))
			}
			if len(l.Widths) != len(tc.wantWidths) {
				t.Fatalf("Widths = %v, want %v", l.Widths, tc.wantWidths)
			}
			for i, w := range tc.wantWidths {
				if l.Widths[i] != w {
					t.Errorf("Widths[%d] = %d, want %d", i, l.Widths[i], w)
				}
			}
			// Reapplying the returned layout is idempotent: the layout is a
			// pure function of the index and the current widths, with no
			// stored intra-cell offset that could drift between passes.
			again := visibleGridLayout(tc.names, tc.cells, tc.availWidth, l.First)
			if again.First != l.First || len(again.Widths) != len(l.Widths) {
				t.Fatalf("recomputation drifted: first %d/%d widths %v/%v", l.First, again.First, l.Widths, again.Widths)
			}
			for i := range l.Widths {
				if again.Widths[i] != l.Widths[i] {
					t.Errorf("recomputed Widths[%d] = %d, want %d", i, again.Widths[i], l.Widths[i])
				}
			}
		})
	}
}

// TestVisibleGridLayoutCumulativeSeparators enforces the cumulative
// rendered-width invariant for layouts with three or more narrow columns:
// the sum of returned cell widths plus one gridSeparatorWidth per adjacent
// pair never exceeds the available width. The existing two-column cases in
// TestVisibleGridLayout cannot expose a missing cumulative separator, so
// these table-driven cases cover exact-fit, one-display-cell-below, and
// one-display-cell-above boundaries for three, four, and Unicode-width
// columns, plus empty/one-column controls, invalid/clamped first indices,
// an oversized first column, and shifted starts. A fitting final column
// remains and an overflowing one is excluded.
func TestVisibleGridLayoutCumulativeSeparators(t *testing.T) {
	tests := []struct {
		name       string
		names      []string
		cells      [][]string
		availWidth int
		first      int
		wantFirst  int
		wantWidths []int
	}{
		// Three width-2 columns: exact fit is 2+sep+2+sep+2 = 12.
		{
			name:       "three columns exact fit keeps the final column",
			names:      []string{"ab", "cd", "ef"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2},
		},
		{
			name:       "three columns one cell below exact fit excludes the final column",
			names:      []string{"ab", "cd", "ef"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 - 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2},
		},
		{
			name:       "three columns one cell above exact fit keeps the final column",
			names:      []string{"ab", "cd", "ef"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2},
		},
		// Four width-2 columns: exact fit is 2+sep+2+sep+2+sep+2 = 17.
		{
			name:       "four columns exact fit keeps the final column",
			names:      []string{"ab", "cd", "ef", "gh"},
			cells:      [][]string{{"x", "y", "z", "w"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2, 2},
		},
		{
			name:       "four columns one cell below exact fit excludes the final column",
			names:      []string{"ab", "cd", "ef", "gh"},
			cells:      [][]string{{"x", "y", "z", "w"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 - 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2},
		},
		{
			name:       "four columns one cell above exact fit keeps the final column",
			names:      []string{"ab", "cd", "ef", "gh"},
			cells:      [][]string{{"x", "y", "z", "w"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2, 2},
		},
		// Three Unicode-width columns (4,4,1): exact fit is 4+sep+4+sep+1 = 15.
		{
			name:       "unicode columns exact fit keeps the final column",
			names:      []string{"広告", "広告", "x"},
			cells:      [][]string{{"a", "b", "c"}},
			availWidth: 4 + gridSeparatorWidth + 4 + gridSeparatorWidth + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{4, 4, 1},
		},
		{
			name:       "unicode columns one cell below exact fit excludes the final column",
			names:      []string{"広告", "広告", "x"},
			cells:      [][]string{{"a", "b", "c"}},
			availWidth: 4 + gridSeparatorWidth + 4 + gridSeparatorWidth + 1 - 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{4, 4},
		},
		{
			name:       "unicode columns one cell above exact fit keeps the final column",
			names:      []string{"広告", "広告", "x"},
			cells:      [][]string{{"a", "b", "c"}},
			availWidth: 4 + gridSeparatorWidth + 4 + gridSeparatorWidth + 1 + 1,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{4, 4, 1},
		},
		// Shifted start: first-visible index 1 of four columns leaves three
		// visible columns (cd, ef, gh), exact fit 2+sep+2+sep+2 = 12.
		{
			name:       "shifted start one below exact fit excludes the final visible column",
			names:      []string{"ab", "cd", "ef", "gh"},
			cells:      [][]string{{"x", "y", "z", "w"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 - 1,
			first:      1,
			wantFirst:  1,
			wantWidths: []int{2, 2},
		},
		{
			name:       "shifted start exact fit keeps all three visible columns",
			names:      []string{"ab", "cd", "ef", "gh"},
			cells:      [][]string{{"x", "y", "z", "w"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      1,
			wantFirst:  1,
			wantWidths: []int{2, 2, 2},
		},
		// Oversized first column caps to the available cell area with no follower.
		{
			name:       "oversized first column caps and excludes every follower",
			names:      []string{"very-long-header", "ab", "cd"},
			cells:      [][]string{{"very-long-cell-value", "x", "y"}},
			availWidth: 10,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{10},
		},
		// One-column control: no separators.
		{
			name:       "one column control has no separator",
			names:      []string{"ab"},
			cells:      [][]string{{"x"}},
			availWidth: 5,
			first:      0,
			wantFirst:  0,
			wantWidths: []int{2},
		},
		// Empty control.
		{
			name:       "empty results control yields a zero layout",
			names:      nil,
			cells:      nil,
			availWidth: 40,
			first:      7,
			wantFirst:  0,
			wantWidths: nil,
		},
		// Invalid first index below zero clamps to the first column.
		{
			name:       "negative first index clamps to first column and packs three columns",
			names:      []string{"ab", "cd", "ef"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      -3,
			wantFirst:  0,
			wantWidths: []int{2, 2, 2},
		},
		// First index beyond the last column clamps to it.
		{
			name:       "first index beyond last clamps to the last column",
			names:      []string{"ab", "cd", "ef"},
			cells:      [][]string{{"x", "y", "z"}},
			availWidth: 40,
			first:      9,
			wantFirst:  2,
			wantWidths: []int{2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := visibleGridLayout(tc.names, tc.cells, tc.availWidth, tc.first)
			if l.First != tc.wantFirst {
				t.Errorf("First = %d, want %d", l.First, tc.wantFirst)
			}
			if l.Total != len(tc.names) {
				t.Errorf("Total = %d, want %d", l.Total, len(tc.names))
			}
			if len(l.Widths) != len(tc.wantWidths) {
				t.Fatalf("Widths = %v, want %v", l.Widths, tc.wantWidths)
			}
			for i, w := range tc.wantWidths {
				if l.Widths[i] != w {
					t.Errorf("Widths[%d] = %d, want %d", i, l.Widths[i], w)
				}
			}
			// The cumulative rendered-width invariant: the sum of returned
			// cell widths plus one gridSeparatorWidth per adjacent pair never
			// exceeds the available width. Empty results and a single column
			// have no separators.
			if len(l.Widths) > 0 {
				rendered := 0
				for _, w := range l.Widths {
					rendered += w
				}
				rendered += (len(l.Widths) - 1) * gridSeparatorWidth
				if rendered > tc.availWidth {
					t.Errorf("cumulative rendered width = %d, exceeds availWidth %d (widths %v)",
						rendered, tc.availWidth, l.Widths)
				}
			}
		})
	}
}

// TestVisibleGridLayoutExposesNoIntraCellOffset requires the capped oversized
// column to occupy exactly the available cell area with nothing left over:
// there is no character or byte offset in the layout through which content
// inside a cell could ever scroll.
func TestVisibleGridLayoutExposesNoIntraCellOffset(t *testing.T) {
	names := []string{"very-long-header", "x"}
	cells := [][]string{{"very-long-cell-value", "y"}}
	l := visibleGridLayout(names, cells, 12, 0)
	if l.Widths[0] != 12 {
		t.Errorf("capped first column width = %d, want exactly the available cell area 12", l.Widths[0])
	}
	if len(l.Widths) != 1 {
		t.Fatalf("capped column left room for %d columns, want 1", len(l.Widths))
	}
}

func TestHorizontalStepBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		first      int
		total      int
		delta      int
		want       int
		wantAccept bool
	}{
		{"advance from the first column", 0, 3, 1, 1, true},
		{"advance within the columns", 1, 3, 1, 2, true},
		{"advance at the last column is a no-op", 2, 3, 1, 2, false},
		{"retreat from the last column", 2, 3, -1, 1, true},
		{"retreat at the first column is a no-op", 0, 3, -1, 0, false},
		{"single column never moves", 0, 1, 1, 0, false},
		{"single column never retreats", 0, 1, -1, 0, false},
		{"no columns never moves", 0, 0, 1, 0, false},
		{"no columns never retreats", 0, 0, -1, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := horizontalStep(tc.first, tc.total, tc.delta)
			if got != tc.want || ok != tc.wantAccept {
				t.Errorf("horizontalStep(%d, %d, %d) = (%d, %v), want (%d, %v)",
					tc.first, tc.total, tc.delta, got, ok, tc.want, tc.wantAccept)
			}
		})
	}
}

func TestClampFirstColumnOnResize(t *testing.T) {
	tests := []struct {
		name  string
		first int
		total int
		want  int
	}{
		{"valid first index preserved", 0, 3, 0},
		{"valid last index preserved", 2, 3, 2},
		{"valid middle index preserved", 1, 4, 1},
		{"index beyond a shrunken column set clamps to the last", 5, 3, 2},
		{"index reduced to one column clamps to it", 2, 1, 0},
		{"negative index clamps to the first", -1, 3, 0},
		{"empty results clamp to zero", 3, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampFirstColumn(tc.first, tc.total); got != tc.want {
				t.Errorf("clampFirstColumn(%d, %d) = %d, want %d", tc.first, tc.total, got, tc.want)
			}
		})
	}
}
