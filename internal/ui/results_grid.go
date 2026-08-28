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
// the status/count line (absolute inclusive displayed range plus the
// persistent invalid-UTF warning where applicable), the frozen deduplicated
// header, and one line per row. An executed-empty SELECT renders exactly
// `No rows` with no data range. Column widths derive from the complete
// visible rows (complete-row sizing) and apply uniformly to header and cells
// so columns stay aligned.
func renderResultPage(p *result.Page) (status string, content []string) {
	names := p.HeaderNames()
	if len(p.Rows) == 0 {
		// Executed-empty state: exact `No rows`, no misleading data range.
		if p.InvalidUTF {
			status = result.UTFWarning
		}
		return status, []string{"No rows"}
	}
	status = "rows 1-" + strconv.Itoa(len(p.Rows))
	if p.InvalidUTF {
		status += " — " + result.UTFWarning
	}
	widths := gridColumnWidths(names, p.Rows)
	lines := []string{resultsHeaderStyle.Render(renderGridRow(names, widths))}
	for _, row := range p.Rows {
		lines = append(lines, renderGridRow(gridCellTexts(row), widths))
	}
	return status, lines
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
// result-error boundary exactly like successful pages.
func renderResultContent(v *ResultView) (status string, content []string) {
	if v.Err != nil {
		return "", []string{v.Err.Error()}
	}
	if v.Page == nil {
		return "", nil
	}
	return renderResultPage(v.Page)
}
