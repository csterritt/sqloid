// Demo program for Issue #70 walkthrough. Mirrors the pure layout and
// rendering arithmetic of internal/ui/horizontal_layout.go and
// internal/ui/results_grid.go so the cumulative separator invariant can be
// calculated and rendered without importing the internal package.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
)

const gridSeparatorWidth = len(" | ")
const gridEllipsis = "…"

func naturalWidths(names []string, cells [][]string) []int {
	w := make([]int, len(names))
	for i, n := range names {
		w[i] = runewidth.StringWidth(n)
	}
	for _, row := range cells {
		for i, c := range row {
			if i < len(w) {
				if d := runewidth.StringWidth(c); d > w[i] {
					w[i] = d
				}
			}
		}
	}
	return w
}

func clampFirst(first, total int) int {
	if total <= 0 || first < 0 {
		return 0
	}
	if first >= total {
		return total - 1
	}
	return first
}

func layout(names []string, cells [][]string, avail, first int) (int, []int) {
	total := len(names)
	if total == 0 {
		return clampFirst(first, total), nil
	}
	if avail < 1 {
		avail = 1
	}
	nat := naturalWidths(names, cells)
	f := clampFirst(first, total)
	used := 0
	var widths []int
	for i := f; i < total; i++ {
		w := nat[i]
		if i == f {
			if w > avail {
				w = avail
			}
		} else if used+gridSeparatorWidth+w > avail {
			break
		}
		widths = append(widths, w)
		if i == f {
			used += w
		} else {
			used += gridSeparatorWidth + w
		}
	}
	return f, widths
}

func invariant(widths []int) int {
	s := 0
	for _, w := range widths {
		s += w
	}
	return s + (len(widths)-1)*gridSeparatorWidth
}

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

func showBoundaries(label string, names []string, cells [][]string, avs []int) {
	fmt.Printf("--- %s ---\n", label)
	for _, av := range avs {
		f, w := layout(names, cells, av, 0)
		inv := invariant(w)
		fits := "fits"
		if inv > av {
			fits = "OVERFLOWS"
		}
		fmt.Printf("availWidth=%2d first=%d widths=%v invariant=%2d %s (<= %d)\n", av, f, w, inv, fits, av)
	}
}

func main() {
	cmd := "all"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "three":
		showBoundaries("three width-2 columns", []string{"ab", "cd", "ef"}, [][]string{{"x", "y", "z"}}, []int{12, 11, 13})
	case "four-unicode":
		showBoundaries("four width-2 columns", []string{"ab", "cd", "ef", "gh"}, [][]string{{"x", "y", "z", "w"}}, []int{17, 16, 18})
		showBoundaries("unicode columns (4,4,1)", []string{"広告", "広告", "x"}, [][]string{{"a", "b", "c"}}, []int{15, 14, 16})
	case "render":
		names := []string{"ab", "cd", "ef"}
		cells := [][]string{{"x", "y", "z"}, {"1", "2", "3"}}
		for _, av := range []int{12, 11} {
			f, w := layout(names, cells, av, 0)
			visN := names[f : f+len(w)]
			header := renderGridRow(visN, w)
			fmt.Printf("availWidth=%d cols=%d header=%q width=%d\n", av, len(w), header, runewidth.StringWidth(header))
			for i, row := range cells {
				vis := row[f : f+len(w)]
				line := renderGridRow(vis, w)
				fmt.Printf("  row %d: %q width=%d\n", i, line, runewidth.StringWidth(line))
			}
		}
	case "shift":
		fourNames := []string{"ab", "cd", "ef", "gh"}
		fourCells := [][]string{{"x", "y", "z", "w"}}
		av := 12
		fmt.Printf("--- shifted first-visible index (availWidth=%d) ---\n", av)
		for first := 0; first < len(fourNames); first++ {
			f, w := layout(fourNames, fourCells, av, first)
			visN := fourNames[f : f+len(w)]
			header := renderGridRow(visN, w)
			fmt.Printf("first=%d cols=%d widths=%v header=%q width=%d\n", first, len(w), w, header, runewidth.StringWidth(header))
		}
		fmt.Println("--- oversized first column capped/ellipsized (availWidth=10) ---")
		bigNames := []string{"very-long-header", "ab", "cd"}
		bigCells := [][]string{{"very-long-cell-value", "x", "y"}}
		f, w := layout(bigNames, bigCells, 10, 0)
		visN := bigNames[f : f+len(w)]
		header := renderGridRow(visN, w)
		dataRow := renderGridRow(bigCells[0][f:f+len(w)], w)
		fmt.Printf("cols=%d widths=%v\n", len(w), w)
		fmt.Printf("header=%q width=%d ellipsized=%v\n", header, runewidth.StringWidth(header), strings.HasSuffix(header, gridEllipsis))
		fmt.Printf("data  =%q width=%d ellipsized=%v\n", dataRow, runewidth.StringWidth(dataRow), strings.HasSuffix(dataRow, gridEllipsis))
	case "regress":
		f, w := layout([]string{"ab"}, [][]string{{"x"}}, 5, 0)
		fmt.Printf("one-column: first=%d widths=%v (no separator, count=%d)\n", f, w, len(w))
		f, w = layout([]string{"ab", "cd", "ef"}, [][]string{{"x", "y", "z"}}, 12, 0)
		fmt.Printf("exact-fit regression: first=%d widths=%v count=%d (final column kept, not omitted)\n", f, w, len(w))
	default:
		fmt.Println("usage: demo_layout.go [three|four-unicode|render|shift|regress]")
		os.Exit(1)
	}
}
