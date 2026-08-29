// Frozen-header result-grid rendering for the first production SELECT page
// (Issue #22), per the Grid rendering/cache and Output names decisions in
// Notes/PRD-sqloid.md. Every display token — deduplicated header names,
// typed cell tokens, visible control-character symbols, the BLOB placeholder,
// and the invalid-UTF warning — comes from the internal/result seam; no
// UI-private formatting exists here. Rendering reads the settled ResultView
// without re-fetching, paging, counting, or horizontally scrolling (later
// issues own those contracts).

package ui

import (
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/chris/sqloid/internal/result"
)

// gridColumnCap is the designated maximum terminal-cell width of any single
// result column at this milestone; longer cells are capped and ellipsized
// without intra-cell scrolling, keeping every column visible.
const gridColumnCap = 32

// gridEllipsis is the exact ellipsis appended to capped cells.
const gridEllipsis = "…"

// renderResultPage renders the settled first page as the results content:
// the status/count line, the frozen deduplicated header, and one line per
// row. An executed-empty SELECT renders exactly `No rows` with no data range.
// The status range is the page's absolute logical range (offset from the
// paging state, Issue #25): rows offset+1 through offset+row count. Column
// widths derive from the complete visible rows (complete-row sizing) and
// apply uniformly to header and cells so columns stay aligned.
func renderResultPage(p *result.Page, offset int64, count result.CountState, loading, running, cancelling bool) (status string, content []string) {
	names := p.HeaderNames()
	var rangeStatus string
	if len(p.Rows) == 0 {
		// Executed-empty state: exact `No rows`, no misleading data range.
		if p.InvalidUTF {
			rangeStatus = result.UTFWarning
		}
		rangeStatus = joinStatusParts(count.Header(), runningText(running), cancellingText(cancelling), loadingText(loading), rangeStatus)
		return rangeStatus, []string{"No rows"}
	}
	rangeStatus = "rows " + strconv.Itoa(int(offset+1)) + "-" +
		strconv.Itoa(int(offset+int64(len(p.Rows))))
	if p.InvalidUTF {
		rangeStatus += " — " + result.UTFWarning
	}
	if count.Status != 0 || loading || running || cancelling {
		// Issue #24: the exact count wording (`Result count: N`,
		// `Result count: N (after Limit M)`, or `Count unavailable`) leads the
		// status/count line; Issue #25 appends the exact loading feedback while
		// a page request is pending. Issue #27 adds `Running…` while the
		// first-page request is in flight and the `cancelling…` handoff after a
		// Ctrl+W request. Neither replaces or clamps the independently displayed
		// absolute range.
		rangeStatus = joinStatusParts(count.Header(), runningText(running), cancellingText(cancelling), loadingText(loading), rangeStatus)
	}
	widths := gridColumnWidths(names, p.Rows)
	lines := []string{resultsHeaderStyle.Render(renderGridRow(names, widths))}
	for _, row := range p.Rows {
		lines = append(lines, renderGridRow(gridCellTexts(row), widths))
	}
	return rangeStatus, lines
}

// loadingText returns the exact loading feedback when a page request is
// pending, or empty text otherwise so joinStatusParts can skip it.
func loadingText(loading bool) string {
	if loading {
		return PageLoadingIndicator
	}
	return ""
}

// runningText returns the exact `Running…` feedback while a first-page
// request is in flight, or empty text otherwise (Issue #27).
func runningText(running bool) string {
	if running {
		return SelectRunningIndicator
	}
	return ""
}

// cancellingText returns the exact `cancelling…` handoff from a Ctrl+W
// cancellation request until settlement, or empty text otherwise (Issue
// #27). It mirrors the established validation wording without changing it.
func cancellingText(cancelling bool) string {
	if cancelling {
		return SelectCancellingIndicator
	}
	return ""
}

// joinStatusParts composes the status/count line from its independent parts,
// skipping empty ones, joined by the designated separator.
func joinStatusParts(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " — ")
}

// gridCellTexts renders one row's typed values through the shared seam only.
func gridCellTexts(row []result.Value) []string {
	cells := make([]string, len(row))
	for i, v := range row {
		cells[i] = v.Display()
	}
	return cells
}

// gridColumnWidths computes per-column display widths as the natural width of
// the widest rendered cell (header included), capped at gridColumnCap.
func gridColumnWidths(names []string, rows [][]result.Value) []int {
	widths := make([]int, len(names))
	for i, n := range names {
		widths[i] = runewidth.StringWidth(n)
	}
	for _, row := range rows {
		for i, v := range gridCellTexts(row) {
			if i < len(widths) {
				if w := runewidth.StringWidth(v); w > widths[i] {
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

// renderGridRow pads or caps each cell to its column width and joins the row
// with the grid separator.
func renderGridRow(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = fitGridCell(cell, w)
	}
	return strings.Join(parts, " | ")
}

// fitGridCell truncates an oversized cell to w terminal cells with the grid
// ellipsis (never splitting visible glyphs) and pads short cells to w.
func fitGridCell(cell string, w int) string {
	if w < 1 {
		w = 1
	}
	if runewidth.StringWidth(cell) > w {
		if w > runewidth.StringWidth(gridEllipsis) {
			cell = runewidth.Truncate(cell, w-runewidth.StringWidth(gridEllipsis), "") + gridEllipsis
		} else {
			cell = runewidth.Truncate(cell, w, "")
		}
	}
	pad := w - runewidth.StringWidth(cell)
	return cell + strings.Repeat(" ", pad)
}

// renderResultContent splits the settled result view into its status line
// and body content, routing ordinary execution errors to the ordinary
// result-error boundary exactly like successful pages. loading adds the
// exact Issue #25 page-loading feedback to the status line while the one
// paged-page request is pending; it never changes the displayed rows.
func renderResultContent(v *ResultView, count result.CountState, loading, running, cancelling bool) (status string, content []string) {
	if v.Err != nil {
		return "", []string{v.Err.Error()}
	}
	if v.Page == nil {
		return "", nil
	}
	return renderResultPage(v.Page, v.Offset, count, loading, running, cancelling)
}
