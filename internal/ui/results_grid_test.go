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
