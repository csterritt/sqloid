// Pure whole-column horizontal layout for the result grid (Issue #29), per
// the UI Behavior and Module Design sections of Notes/PRD-sqloid.md. The
// horizontal position of the grid is exactly one first-visible output-column
// index: every layout pass recomputes the visible columns and their widths
// from the current deduplicated columns, rendered cell display widths, and
// available grid width, starting at that index. There is deliberately no
// character or byte offset anywhere in this arithmetic, so no intra-cell
// horizontal scrolling can exist; a single oversized column is capped and
// ellipsized within the available cell area instead. Bubble Tea key handling
// stays with the model seams; this file is pure and terminal-free.

package ui

import (
	"github.com/mattn/go-runewidth"
)

// gridSeparatorWidth is the exact display width of the grid separator
// (" | ") joined between rendered grid cells.
const gridSeparatorWidth = len(" | ")

// gridVisibleLayout is the outcome of one pure horizontal layout pass. First
// is the clamped first-visible output-column index, Widths holds one display
// width per visible column starting at First, and Total is the total number
// of deduplicated output columns. Count is derived as len(Widths). No offset
// beyond the column index exists anywhere in the layout.
type gridVisibleLayout struct {
	First  int
	Widths []int
	Total  int
}

// visibleGridLayout computes the visible output columns and their display
// widths for the given deduplicated header names, rendered cell texts per
// row, available grid width, and first-visible output-column index. An
// invalid index is clamped; the first visible column is always included with
// its width capped to the available cell area (its oversized cells ellipsize
// during rendering), and every later column joins only when it fits
// completely, grid separator included. The cumulative rendered-width
// invariant holds for every accepted column: sum(widths) +
// (visible column count - 1) * gridSeparatorWidth <= availWidth, with no
// separator counted before the first column.
func visibleGridLayout(names []string, cells [][]string, availWidth int, first int) gridVisibleLayout {
	total := len(names)
	l := gridVisibleLayout{First: clampFirstColumn(first, total), Total: total}
	if total == 0 {
		return l
	}
	if availWidth < 1 {
		availWidth = 1 // fitGridCell floors at one cell; keep the contract aligned
	}
	naturals := naturalGridWidths(names, cells)
	used := 0
	for i := l.First; i < total; i++ {
		w := naturals[i]
		if i == l.First {
			// Cap an oversized first column to the available cell area; the
			// cells ellipsize within it. No intra-cell offset is produced.
			if w > availWidth {
				w = availWidth
			}
		} else if used+gridSeparatorWidth+w > availWidth {
			// No room for another complete column: stop packing.
			break
		}
		l.Widths = append(l.Widths, w)
		// After the first visible column, every accepted column contributes
		// both its grid separator and its own width to the cumulative used
		// width, preserving the rendered-width invariant across all accepted
		// columns. No separator is counted before the first column.
		if i == l.First {
			used += w
		} else {
			used += gridSeparatorWidth + w
		}
	}
	return l
}

// horizontalStep moves a first-visible output-column index by delta whole
// columns, reporting whether the move was accepted. Presses at the first
// (delta < 0) or last (delta > 0) boundary — and any navigation with zero or
// one output columns — are no-ops.
func horizontalStep(first, total, delta int) (int, bool) {
	next := first + delta
	if total <= 1 || next < 0 || next >= total {
		return first, false
	}
	return next, true
}

// clampFirstColumn normalizes a first-visible output-column index after
// column or width changes: a valid index is preserved unchanged and an
// invalid one is clamped to the nearest valid boundary, with empty results
// collapsing to zero.
func clampFirstColumn(first, total int) int {
	if total <= 0 || first < 0 {
		return 0
	}
	if first >= total {
		return total - 1
	}
	return first
}

// naturalGridWidths computes per-column natural display widths from the
// rendered header names and cell texts — the widest Unicode display width of
// each column's cells, header included — capped at gridColumnCap.
func naturalGridWidths(names []string, cells [][]string) []int {
	widths := make([]int, len(names))
	for i, n := range names {
		widths[i] = runewidth.StringWidth(n)
	}
	for _, row := range cells {
		for i, c := range row {
			if i < len(widths) {
				if w := runewidth.StringWidth(c); w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	for i, w := range widths {
		if w > gridColumnCap {
			widths[i] = gridColumnCap
		}
	}
	return widths
}
