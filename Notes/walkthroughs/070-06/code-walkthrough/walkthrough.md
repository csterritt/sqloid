# Issue #070 Code Walkthrough: Cumulative Separator-Aware Horizontal Grid Packing

*2026-09-02T13:53:27Z by Showboat 0.6.1*
<!-- showboat-id: 43d34bcd-a8a2-4c14-ae3c-707273980a57 -->

Issue #70 (Notes/tasks/070-count-grid-separators-in-layout.md, Notes/PRD-sqloid.md §horizontal-scroll, §visual invariants, §UI Module Design, §manual matrix, §Testing Decisions) corrects the cumulative separator accounting in visibleGridLayout so three-or-more-column layouts no longer accept a column whose separator plus width overflows the available grid width. The invariant is sum(column widths) + (visible column count - 1) * gridSeparatorWidth <= available width, with no separator counted before the first column. This walkthrough demonstrates three-or-more-column exact-fit, one-below, and one-above layouts and calculates the cumulative separator invariant from returned widths; renders matching headers/data to show no wrapping or off-screen output and no unnecessary omission; scrolls through shifted first-column indices; and shows one-column, Unicode, and oversized cap/ellipsis controls. It closes with the focused automated-test output. The standalone demo program demo_layout.go mirrors the pure layout and rendering arithmetic so the invariant can be calculated and rendered without importing the internal package.

## The corrected cumulative separator accounting

visibleGridLayout (internal/ui/horizontal_layout.go) packs whole columns starting at the clamped first-visible index. The first visible column is always included with its width capped to the available cell area. After the first visible column, every accepted column contributes both gridSeparatorWidth and its own width to the running used total, so the next column is included only when its separator plus its width still fit. The fix is the branch that adds gridSeparatorWidth + w for non-first columns instead of only w.

```bash
sed -n '33,80p' internal/ui/horizontal_layout.go
```

```output
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
```

## Three-column exact-fit, one-below, and one-above layouts

Three width-2 columns (ab, cd, ef) have an exact-fit width of 2 + sep + 2 + sep + 2 = 12. At exact fit (availWidth=12) all three columns are kept; one display cell below (availWidth=11) the final column is excluded; one above (availWidth=13) all three remain. The invariant sum(widths) + (count-1)*sep is calculated from the returned widths and must never exceed availWidth.

```bash
go run Notes/walkthroughs/070-06/code-walkthrough/demo_layout.go three
```

```output
--- three width-2 columns ---
availWidth=12 first=0 widths=[2 2 2] invariant=12 fits (<= 12)
availWidth=11 first=0 widths=[2 2] invariant= 7 fits (<= 11)
availWidth=13 first=0 widths=[2 2 2] invariant=12 fits (<= 13)
```

## Four-column and Unicode-width boundaries

Four width-2 columns (ab, cd, ef, gh) have an exact-fit width of 2 + sep + 2 + sep + 2 + sep + 2 = 17. One cell below (16) excludes the final column; one above (18) keeps all four. Three Unicode-width columns (広告=4, 広告=4, x=1) have an exact-fit width of 4 + sep + 4 + sep + 1 = 15; one cell below (14) excludes the final column; one above (16) keeps all three. The invariant holds in every case.

```bash
go run Notes/walkthroughs/070-06/code-walkthrough/demo_layout.go four-unicode
```

```output
--- four width-2 columns ---
availWidth=17 first=0 widths=[2 2 2 2] invariant=17 fits (<= 17)
availWidth=16 first=0 widths=[2 2 2] invariant=12 fits (<= 16)
availWidth=18 first=0 widths=[2 2 2 2] invariant=17 fits (<= 18)
--- unicode columns (4,4,1) ---
availWidth=15 first=0 widths=[4 4 1] invariant=15 fits (<= 15)
availWidth=14 first=0 widths=[4 4] invariant=11 fits (<= 14)
availWidth=16 first=0 widths=[4 4 1] invariant=15 fits (<= 16)
```

## Rendered headers and data fit the grid row width

renderGridRow (internal/ui/results_grid.go) pads or caps each cell to its column width and joins with exactly the grid separator. Rendering the three-column exact-fit and one-below layouts produces joined header and data lines whose terminal display width (runewidth.StringWidth) never exceeds the supplied grid row width, with no wrapping or off-screen output. At one cell below, the final column is omitted rather than clipped, and header/data column counts stay aligned.

```bash
go run Notes/walkthroughs/070-06/code-walkthrough/demo_layout.go render
```

```output
availWidth=12 cols=3 header="ab | cd | ef" width=12
  row 0: "x  | y  | z " width=12
  row 1: "1  | 2  | 3 " width=12
availWidth=11 cols=2 header="ab | cd" width=7
  row 0: "x  | y " width=7
  row 1: "1  | 2 " width=7
```

## Shifted first-column indices and oversized cap/ellipsis

Scrolling the first-visible index one column at a time across four columns at a width that fits three columns (availWidth=12) keeps every shifted layout within the grid row width. A single oversized first column is capped to the available cell area (10) and ellipsized with the grid ellipsis, with no follower and no off-screen content; the rendered header and data row both have display width exactly 10.

```bash
go run Notes/walkthroughs/070-06/code-walkthrough/demo_layout.go shift
```

```output
--- shifted first-visible index (availWidth=12) ---
first=0 cols=3 widths=[2 2 2] header="ab | cd | ef" width=12
first=1 cols=3 widths=[2 2 2] header="cd | ef | gh" width=12
first=2 cols=2 widths=[2 2] header="ef | gh" width=7
first=3 cols=1 widths=[2] header="gh" width=2
--- oversized first column capped/ellipsized (availWidth=10) ---
cols=1 widths=[10]
header="very-long…" width=10 ellipsized=true
data  ="very-long…" width=10 ellipsized=true
```

## One-column control and no-unnecessary-omission regression

A one-column layout has no separator and is never omitted. The regression case proves a column whose width plus its separator fits exactly is not unnecessarily omitted: at the exact-fit width (12) the third column (width 2 + separator 3 = 5) fits exactly in the remaining 10 cells after the first column, and after the second column the remaining 5 cells fit separator+width exactly, so the third column is kept.

```bash
go run Notes/walkthroughs/070-06/code-walkthrough/demo_layout.go regress
```

```output
one-column: first=0 widths=[2] (no separator, count=1)
exact-fit regression: first=0 widths=[2 2 2] count=3 (final column kept, not omitted)
```

## Focused automated-test output

The focused tests for Issue #70 are TestVisibleGridLayoutCumulativeSeparators (pure layout invariant) and TestResultGridRenderedWidthFits (rendered header/data width integration). Both pass against the corrected visibleGridLayout.

```bash
go test ./internal/ui/ -run '^TestVisibleGridLayoutCumulativeSeparators$|^TestResultGridRenderedWidthFits$' -count=1 -v
```

```output
=== RUN   TestVisibleGridLayoutCumulativeSeparators
=== RUN   TestVisibleGridLayoutCumulativeSeparators/three_columns_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/three_columns_one_cell_below_exact_fit_excludes_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/three_columns_one_cell_above_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/four_columns_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/four_columns_one_cell_below_exact_fit_excludes_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/four_columns_one_cell_above_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/unicode_columns_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/unicode_columns_one_cell_below_exact_fit_excludes_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/unicode_columns_one_cell_above_exact_fit_keeps_the_final_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/shifted_start_one_below_exact_fit_excludes_the_final_visible_column
=== RUN   TestVisibleGridLayoutCumulativeSeparators/shifted_start_exact_fit_keeps_all_three_visible_columns
=== RUN   TestVisibleGridLayoutCumulativeSeparators/oversized_first_column_caps_and_excludes_every_follower
=== RUN   TestVisibleGridLayoutCumulativeSeparators/one_column_control_has_no_separator
=== RUN   TestVisibleGridLayoutCumulativeSeparators/empty_results_control_yields_a_zero_layout
=== RUN   TestVisibleGridLayoutCumulativeSeparators/negative_first_index_clamps_to_first_column_and_packs_three_columns
=== RUN   TestVisibleGridLayoutCumulativeSeparators/first_index_beyond_last_clamps_to_the_last_column
--- PASS: TestVisibleGridLayoutCumulativeSeparators (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/three_columns_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/three_columns_one_cell_below_exact_fit_excludes_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/three_columns_one_cell_above_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/four_columns_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/four_columns_one_cell_below_exact_fit_excludes_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/four_columns_one_cell_above_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/unicode_columns_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/unicode_columns_one_cell_below_exact_fit_excludes_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/unicode_columns_one_cell_above_exact_fit_keeps_the_final_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/shifted_start_one_below_exact_fit_excludes_the_final_visible_column (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/shifted_start_exact_fit_keeps_all_three_visible_columns (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/oversized_first_column_caps_and_excludes_every_follower (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/one_column_control_has_no_separator (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/empty_results_control_yields_a_zero_layout (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/negative_first_index_clamps_to_first_column_and_packs_three_columns (0.00s)
    --- PASS: TestVisibleGridLayoutCumulativeSeparators/first_index_beyond_last_clamps_to_the_last_column (0.00s)
=== RUN   TestResultGridRenderedWidthFits
=== RUN   TestResultGridRenderedWidthFits/three_columns_exact_fit_renders_all_columns_within_width
=== RUN   TestResultGridRenderedWidthFits/three_columns_one_cell_below_excludes_the_overflowing_column
=== RUN   TestResultGridRenderedWidthFits/four_columns_exact_fit_renders_all_columns_within_width
=== RUN   TestResultGridRenderedWidthFits/four_columns_one_cell_below_excludes_the_overflowing_column
=== RUN   TestResultGridRenderedWidthFits/oversized_first_column_is_capped_and_ellipsized_within_width
=== RUN   TestResultGridRenderedWidthFits/column_whose_width_plus_separator_fits_exactly_is_not_omitted
=== RUN   TestResultGridRenderedWidthFits/shifted_first-visible_index_one_column_at_a_time
=== RUN   TestResultGridRenderedWidthFits/oversized_first_column_ellipsizes_to_the_available_cell_area
--- PASS: TestResultGridRenderedWidthFits (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/three_columns_exact_fit_renders_all_columns_within_width (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/three_columns_one_cell_below_excludes_the_overflowing_column (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/four_columns_exact_fit_renders_all_columns_within_width (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/four_columns_one_cell_below_excludes_the_overflowing_column (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/oversized_first_column_is_capped_and_ellipsized_within_width (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/column_whose_width_plus_separator_fits_exactly_is_not_omitted (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/shifted_first-visible_index_one_column_at_a_time (0.00s)
    --- PASS: TestResultGridRenderedWidthFits/oversized_first_column_ellipsizes_to_the_available_cell_area (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.011s
```

## References

- Issue #70: Notes/tasks/070-count-grid-separators-in-layout.md
- PRD: Notes/PRD-sqloid.md (horizontal-scroll, visual invariants, UI Module Design, manual matrix, Testing Decisions)
- Implementation: internal/ui/horizontal_layout.go (visibleGridLayout), internal/ui/results_grid.go (renderGridRow, renderResultPage)
- Tests: internal/ui/horizontal_layout_test.go (TestVisibleGridLayoutCumulativeSeparators), internal/ui/results_grid_test.go (TestResultGridRenderedWidthFits)
- Wiki: Notes/wiki/whole-column-horizontal-scrolling.md
- Demo artifact: demo_layout.go (this directory)
