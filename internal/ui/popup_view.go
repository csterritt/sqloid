// Popup presentation for Issue #12: deterministic text rendering of search
// input, highlighted and plain candidate rows, the exact `no matches` state,
// viewport windowing, and the scroll-only variant without a search input.
// Rendering performs no state mutation and is pure given the popup, width,
// and height.

package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Popup presentation prefixes. The caret prefix marks the highlighted row so
// focus stays readable with color disabled.
const (
	popupSelectedPrefix = "> "
	popupPlainPrefix    = "  "
	popupCursorRune     = "_"
	popupSearchPrompt   = "Search: "
)

var popupStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

// Popup border geometry shared with the overlay composer.
const (
	popupBorderRows = 2 // top and bottom border rows owned by the popup box
	popupBorderCols = 2 // left and right border columns
)

// RenderPopup renders the full bordered popup box: one `Search: <text>_`
// line for searchable variants (absent for scroll-only), any status lines
// such as the exact `no matches`, then the visible candidate window with
// `> ` on the highlighted row. width/height bound the box; every line is
// truncated to the interior width without splitting visible glyphs.
func RenderPopup(p *Popup, width, height int) string {
	lines := renderPopupLines(p, width)
	maxContent := height - popupBorderRows
	if height > 0 && len(lines) > maxContent {
		lines = lines[:maxContent]
	}
	w := popupInteriorWidth(p, width)
	return popupStyle.
		Width(w).
		Render(strings.Join(lines, "\n"))
}

// renderPopupLines produces the unbordered content lines of a popup, each
// already truncated to width cells.
func renderPopupLines(p *Popup, width int) []string {
	var lines []string
	if p.Mode == PopupSearchable {
		lines = append(lines, truncateCell(popupSearchPrompt+p.Search+popupCursorRune, width))
	}
	lines = append(lines, p.StatusMessages()...)
	if !p.NoMatch() {
		start := p.viewportTop
		end := start + p.viewHeight()
		if end > len(p.filtered) {
			end = len(p.filtered)
		}
		for i := start; i < end; i++ {
			c := p.candidates[p.filtered[i]]
			prefix := popupPlainPrefix
			if i == p.highlightIndex {
				prefix = popupSelectedPrefix
			}
			lines = append(lines, truncateCell(prefix+c.Display, width))
		}
	}
	return lines
}

// popupInteriorWidth sizes the box wide enough for its widest content line
// but never wider than width allows, accounting for the two border columns.
func popupInteriorWidth(p *Popup, width int) int {
	longest := 0
	for _, l := range renderPopupLines(p, maxInt(width, 1)) {
		if w := lipgloss.Width(l); w > longest {
			longest = w
		}
	}
	limit := width - popupBorderCols
	if limit < 1 {
		limit = 1
	}
	w := longest + 2 // breathing room around the widest row
	if w < 4 {
		w = 4
	}
	return minInt(w, limit)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncateCell clips s to at most width terminal cells, preserving whole
// runes so no glyph is split mid-sequence.
func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	return runewidth.Truncate(s, width, "")
}

// dropCellPrefix removes exactly width leading cells from s; partial wide
// glyphs are consumed whole so the remainder starts on a rune boundary.
func dropCellPrefix(s string, width int) string {
	for width > 0 && s != "" {
		r, size := utf8.DecodeRuneInString(s)
		width -= runewidth.RuneWidth(r)
		s = s[size:]
	}
	return s
}

// composeOverlay splices overlay onto base starting at row/col, drawing over
// existing regions without reflowing them: every line beyond the overlay's
// extent keeps its exact prior content. Negative coordinates leave the base
// unchanged.
func composeOverlay(base, overlay string, row, col int) string {
	if row < 0 || col < 0 {
		return base
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	for dy, ol := range overlayLines {
		y := row + dy
		if y >= len(baseLines) {
			break
		}
		bl := baseLines[y]
		left := truncateCell(bl, col)
		pad := col - runewidth.StringWidth(left)
		right := dropCellPrefix(bl, col+runewidth.StringWidth(ol))
		baseLines[y] = left + strings.Repeat(" ", pad) + ol + right
	}
	return strings.Join(baseLines, "\n")
}
