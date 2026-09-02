// Frozen-grid rendering coverage for the first production result (Issue #22
// Tasks 5–6), per the Builder and Display Interaction, Grid rendering/cache,
// Invalid UTF-8 TEXT, and Output names decisions in Notes/PRD-sqloid.md.
// Tests consume only internal/result fixtures — no database access — and
// forbid grid-local name or value formatting: every display token comes from
// the shared seam. The grid pins deduplicated frozen headers, the absolute
// inclusive displayed range, typed cell distinctions, the persistent
// invalid-UTF warning, and the exact `No rows` executed-empty state.

package ui

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/chris/sqloid/internal/result"
)

// contextErr returns the ordinary driver cause used by error-path fixtures.
func contextErr() error { return errors.New("no such table: users") }

// resultModel returns a supported-size model wired with a successful executor
// carrying the given page, driven through validation, execution start, and
// settlement — the exact production route for one first page.
func resultModel(t *testing.T, page *result.Page, err error) Model {
	t.Helper()

	exec := &fakeSelectExecutor{page: page, err: err}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	settled := execCmd()
	return asModel(execModel.Update(settled))
}

func TestResultGridFrozenDeduplicatedHeader(t *testing.T) {
	page := &result.Page{
		Columns: []string{"COUNT(*)", "COUNT(*)", "id", "id"},
		Rows: [][]result.Value{{
			result.NewInteger(3), result.NewInteger(5), result.NewInteger(1), result.NewText("a"),
		}},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	// Full-set deduplication across the header, rendered once, frozen.
	if !strings.Contains(view, "COUNT(*) | COUNT(*)_2 | id | id_2") {
		t.Errorf("frozen header missing or not deduplicated:\n%s", view)
	}
	if strings.Contains(m.renderResults(80, 18), "tracer") {
		t.Error("stale tracer content leaked into the result view")
	}
}

func TestResultGridAbsoluteRangeStatus(t *testing.T) {
	page := &result.Page{
		Columns: []string{"id", "v"},
		Rows: [][]result.Value{
			{result.NewInteger(1), result.NewText("one")},
			{result.NewInteger(2), result.NewText("two")},
			{result.NewInteger(3), result.NewText("three")},
		},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	// Absolute inclusive logical range, not page-relative indexes.
	if !strings.Contains(view, "rows 1-3") {
		t.Errorf("view missing absolute range status \"rows 1-3\":\n%s", view)
	}
	if strings.Contains(view, "rows 0-") {
		t.Errorf("status shows a page-relative range:\n%s", view)
	}
}

func TestResultGridTypedCellDistinctions(t *testing.T) {
	page := &result.Page{
		Columns: []string{"n", "r", "t", "b", "z"},
		Rows: [][]result.Value{{
			result.NewInteger(1), result.NewReal(1), result.NewText("1.0"),
			result.NewBlob([]byte{0x01, 0x02, 0x03}), result.NewNull(),
		}},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	for _, want := range []string{"1", "1.0", "[BLOB 3 bytes]"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing typed cell %q:\n%s", want, view)
		}
	}
	// NULL renders through the shared designated token.
	if !strings.Contains(view, result.NullDisplay) {
		t.Errorf("view missing NULL display:\n%s", view)
	}
	// Backing bytes stay raw and untouched.
	if string(page.Rows[0][3].Bytes) != "\x01\x02\x03" {
		t.Errorf("BLOB bytes mutated by rendering: %x", page.Rows[0][3].Bytes)
	}
}

func TestResultGridNonFiniteRealTokens(t *testing.T) {
	page := &result.Page{
		Columns: []string{"r", "t"},
		Rows: [][]result.Value{
			{result.NewReal(math.Inf(1)), result.NewText("Inf")},
			{result.NewReal(math.Inf(-1)), result.NewText("-Inf")},
			{result.NewReal(math.NaN()), result.NewText("NaN")},
			{result.NewReal(1), result.NewText("1.0")},
		},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	for _, want := range []string{"Inf", "-Inf", "NaN"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing non-finite REAL token %q:\n%s", want, view)
		}
	}
	// Finite REALs keep the Issue #22 shortest-round-trip token.
	if !strings.Contains(view, "1.0") {
		t.Errorf("view missing finite REAL token \"1.0\":\n%s", view)
	}
	// Identical-looking TEXT keeps its type and verbatim value; REALs keep
	// their float64 backing value.
	if page.Rows[0][1].Kind != result.KindText || page.Rows[0][1].Str != "Inf" {
		t.Errorf("TEXT \"Inf\" value altered: (%d, %q)", page.Rows[0][1].Kind, page.Rows[0][1].Str)
	}
	if page.Rows[0][0].Kind != result.KindReal || !math.IsInf(page.Rows[0][0].Float, 1) {
		t.Errorf("REAL +Inf backing value altered: (%d, %v)", page.Rows[0][0].Kind, page.Rows[0][0].Float)
	}
	if len(m.Result.Page.Rows) != 4 {
		t.Errorf("row count = %d, want 4", len(m.Result.Page.Rows))
	}
	// Exact per-row grid tokens, rendered through the shared seam only.
	wantRows := [][]string{
		{"Inf", "Inf"},
		{"-Inf", "-Inf"},
		{"NaN", "NaN"},
		{"1.0", "1.0"},
	}
	for i, row := range m.Result.Page.Rows {
		cells := gridCellTexts(row)
		for j, want := range wantRows[i] {
			if cells[j] != want {
				t.Errorf("grid cell (%d,%d) = %q, want %q", i, j, cells[j], want)
			}
		}
	}
}

func TestResultGridVisibleControlCharacters(t *testing.T) {
	page := &result.Page{
		Columns: []string{"t"},
		Rows:    [][]result.Value{{result.NewText("a\tb\nc")}},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	if !strings.Contains(view, "a"+result.TabSymbol+"b"+result.NewlineSymbol+"c") {
		t.Errorf("view does not render visible tab/newline symbols:\n%s", view)
	}
	if strings.Contains(view, "a\tb") || strings.Contains(view, "a\nb") {
		t.Errorf("raw control characters leaked into the grid:\n%s", view)
	}
}

func TestResultGridInvalidUTFWarningPersistent(t *testing.T) {
	decoded, _ := result.DecodeText("bad\xE0\x80\x80")
	page := &result.Page{
		Columns:    []string{"t", "n"},
		Rows:       [][]result.Value{{result.NewText(decoded), result.NewInteger(2)}},
		InvalidUTF: true,
	}
	m := resultModel(t, page, nil)
	view := m.View()

	if !strings.Contains(view, decoded) {
		t.Errorf("view missing the U+FFFD replacement result:\n%s", view)
	}
	if !strings.Contains(view, result.UTFWarning) {
		t.Errorf("view missing the persistent invalid-UTF warning:\n%s", view)
	}
	// No extra row or column was added for the warning: both cells of the
	// single row still render on one grid line, and the row count is 1.
	if len(m.Result.Page.Rows) != 1 || len(m.Result.Page.Rows[0]) != 2 {
		t.Errorf("warning altered tabular data: %+v", m.Result.Page.Rows)
	}
	if !strings.Contains(view, "rows 1-1") {
		t.Errorf("view missing the single-row absolute range:\n%s", view)
	}
}

func TestResultGridExactNoRows(t *testing.T) {
	page := &result.Page{Columns: []string{"id"}, Rows: nil}
	m := resultModel(t, page, nil)
	view := m.View()

	if !strings.Contains(view, "No rows") {
		t.Errorf("view missing exact \"No rows\" for an executed empty SELECT:\n%s", view)
	}
	if strings.Contains(view, "rows 1-") || strings.Contains(view, "rows 0-") {
		t.Errorf("empty result shows a misleading data range:\n%s", view)
	}
	// Distinct from the pre-execution startup prompt.
	if strings.Contains(view, "Select a command (S/U/D/I) to begin") {
		t.Errorf("executed-empty state shows the startup prompt:\n%s", view)
	}
}

func TestStartupPromptDistinctFromNoRows(t *testing.T) {
	// Pre-execution startup: exactly the prompt, no header, no range, and no
	// "No rows" anywhere.
	m := sized(New(), 80, 24).(Model)
	view := m.View()
	if !strings.Contains(view, "Select a command (S/U/D/I) to begin") {
		t.Errorf("startup prompt missing:\n%s", view)
	}
	if strings.Contains(view, "No rows") || strings.Contains(view, "rows 1-") {
		t.Errorf("startup view shows executed-empty state:\n%s", view)
	}
}

func TestResultErrorReplacesIdleContent(t *testing.T) {
	m := resultModel(t, nil, contextErr())
	view := m.View()
	if !strings.Contains(view, "no such table: users") {
		t.Errorf("ordinary execution error not rendered:\n%s", view)
	}
	if strings.Contains(view, "rows 1-") {
		t.Errorf("error state shows a data range:\n%s", view)
	}
}

// renderGridLines renders the visible header and one line per data row through
// visibleGridLayout and renderGridRow, returning the header line and data
// lines. It mirrors the composition in renderResultPage without snapshotting
// the surrounding shell borders or status line.
func renderGridLines(t *testing.T, names []string, cells [][]string, availWidth, first int) (string, []string) {
	t.Helper()
	layout := visibleGridLayout(names, cells, availWidth, first)
	if len(layout.Widths) == 0 {
		return "", nil
	}
	visibleNames := names[layout.First : layout.First+len(layout.Widths)]
	header := renderGridRow(visibleNames, layout.Widths)
	lines := []string{header}
	for _, row := range cells {
		visible := row[layout.First : layout.First+len(layout.Widths)]
		lines = append(lines, renderGridRow(visible, layout.Widths))
	}
	return header, lines[1:]
}

// assertRenderedFits requires the header and every data row rendered for the
// given layout to have a terminal display width no greater than the supplied
// grid row width, with header and data column counts aligned to the layout.
func assertRenderedFits(t *testing.T, names []string, cells [][]string, availWidth, first int) {
	t.Helper()
	header, dataLines := renderGridLines(t, names, cells, availWidth, first)
	layout := visibleGridLayout(names, cells, availWidth, first)
	if len(layout.Widths) == 0 {
		if header != "" || len(dataLines) != 0 {
			t.Fatalf("empty layout rendered header %q or %d data lines", header, len(dataLines))
		}
		return
	}
	if hw := runewidth.StringWidth(header); hw > availWidth {
		t.Errorf("header display width = %d, exceeds grid row width %d: %q", hw, availWidth, header)
	}
	for i, line := range dataLines {
		if dw := runewidth.StringWidth(line); dw > availWidth {
			t.Errorf("data row %d display width = %d, exceeds grid row width %d: %q", i, dw, availWidth, line)
		}
	}
	// Header and data column counts stay aligned to the layout: each rendered
	// line joins exactly len(layout.Widths) cells padded to their widths.
	wantCells := len(layout.Widths)
	if parts := strings.Split(header, " | "); len(parts) != wantCells {
		t.Errorf("header has %d joined cells, want %d: %q", len(parts), wantCells, header)
	}
	for i, line := range dataLines {
		if parts := strings.Split(line, " | "); len(parts) != wantCells {
			t.Errorf("data row %d has %d joined cells, want %d: %q", i, len(parts), wantCells, line)
		}
	}
}

// TestResultGridRenderedWidthFits proves joined headers and data rows rendered
// through visibleGridLayout and renderGridRow fit the supplied grid row width
// across scrolling and oversized-column cases, using terminal display width
// rather than byte length. Multiple narrow columns are rendered at exact-fit
// and overflow boundaries; the first-visible index is moved one column at a
// time; a single oversized first column is capped and ellipsized; and a
// regression proves a column whose width plus its separator fits exactly is
// not unnecessarily omitted.
func TestResultGridRenderedWidthFits(t *testing.T) {
	// Three width-2 columns: exact fit is 2+sep+2+sep+2 = 12.
	threeNames := []string{"ab", "cd", "ef"}
	threeCells := [][]string{{"x", "y", "z"}, {"1", "2", "3"}}
	// Four width-2 columns: exact fit is 2+sep+2+sep+2+sep+2 = 17.
	fourNames := []string{"ab", "cd", "ef", "gh"}
	fourCells := [][]string{{"x", "y", "z", "w"}, {"1", "2", "3", "4"}}

	tests := []struct {
		name       string
		names      []string
		cells      [][]string
		availWidth int
		first      int
		wantCols   int
	}{
		{
			name:       "three columns exact fit renders all columns within width",
			names:      threeNames,
			cells:      threeCells,
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      0,
			wantCols:   3,
		},
		{
			name:       "three columns one cell below excludes the overflowing column",
			names:      threeNames,
			cells:      threeCells,
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 - 1,
			first:      0,
			wantCols:   2,
		},
		{
			name:       "four columns exact fit renders all columns within width",
			names:      fourNames,
			cells:      fourCells,
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2,
			first:      0,
			wantCols:   4,
		},
		{
			name:       "four columns one cell below excludes the overflowing column",
			names:      fourNames,
			cells:      fourCells,
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2 - 1,
			first:      0,
			wantCols:   3,
		},
		{
			name:       "oversized first column is capped and ellipsized within width",
			names:      []string{"very-long-header", "ab", "cd"},
			cells:      [][]string{{"very-long-cell-value", "x", "y"}},
			availWidth: 10,
			first:      0,
			wantCols:   1,
		},
		{
			name:       "column whose width plus separator fits exactly is not omitted",
			names:      threeNames,
			cells:      threeCells,
			availWidth: 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2, // exact fit
			first:      0,
			wantCols:   3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layout := visibleGridLayout(tc.names, tc.cells, tc.availWidth, tc.first)
			if len(layout.Widths) != tc.wantCols {
				t.Fatalf("visible column count = %d, want %d (widths %v)",
					len(layout.Widths), tc.wantCols, layout.Widths)
			}
			assertRenderedFits(t, tc.names, tc.cells, tc.availWidth, tc.first)
		})
	}

	// Move the first-visible index one column at a time across four columns
	// at a width that fits three columns from index 0 (2+sep+2+sep+2 = 12).
	// Every shifted start must render header and data within the grid row
	// width with aligned column counts.
	t.Run("shifted first-visible index one column at a time", func(t *testing.T) {
		availWidth := 2 + gridSeparatorWidth + 2 + gridSeparatorWidth + 2
		for first := 0; first < len(fourNames); first++ {
			layout := visibleGridLayout(fourNames, fourCells, availWidth, first)
			if first == len(fourNames)-1 {
				if len(layout.Widths) != 1 {
					t.Fatalf("first=%d visible column count = %d, want 1 (last column only)",
						first, len(layout.Widths))
				}
			} else if len(layout.Widths) > 3 {
				t.Fatalf("first=%d visible column count = %d, want at most 3",
					first, len(layout.Widths))
			}
			assertRenderedFits(t, fourNames, fourCells, availWidth, first)
		}
	})

	// The oversized capped first column ellipsizes to exactly the available
	// cell area with no follower and no off-screen content.
	t.Run("oversized first column ellipsizes to the available cell area", func(t *testing.T) {
		names := []string{"very-long-header", "ab", "cd"}
		cells := [][]string{{"very-long-cell-value", "x", "y"}}
		header, dataLines := renderGridLines(t, names, cells, 10, 0)
		if hw := runewidth.StringWidth(header); hw != 10 {
			t.Errorf("header display width = %d, want exactly 10: %q", hw, header)
		}
		if dw := runewidth.StringWidth(dataLines[0]); dw != 10 {
			t.Errorf("data row display width = %d, want exactly 10: %q", dw, dataLines[0])
		}
		if !strings.HasSuffix(header, gridEllipsis) {
			t.Errorf("header not ellipsized: %q", header)
		}
		if !strings.HasSuffix(dataLines[0], gridEllipsis) {
			t.Errorf("data row not ellipsized: %q", dataLines[0])
		}
	})
}
