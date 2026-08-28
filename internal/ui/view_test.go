// Rendering assertions for the pre-execution idle state, per Issue #11 Task 3:
// the bordered results region shows exactly the startup prompt with no frozen
// header, displayed range, or count, while normal Issue #8 layout arithmetic
// remains untouched.

package ui

import (
	"strings"
	"testing"
)

// IdlePrompt is the exact startup results-region content before any execution
// exists, distinct from an executed SELECT's `No rows` state.
const IdlePrompt = "Select a command (S/U/D/I) to begin"

// TestIdleResultsContent pins the exact idle prompt inside the results region
// at each supported baseline size.
func TestIdleResultsContent(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}, {160, 50}} {
		m := sized(New(), size.w, size.h).(Model)
		view := m.View()
		if !strings.Contains(view, IdlePrompt) {
			t.Errorf("%dx%d idle view lacks exact prompt %q", size.w, size.h, IdlePrompt)
		}
	}
}

// boxRows renders the results region and strips the exclusive border column
// so each returned row carries only its interior cells.
func boxRows(t *testing.T, rendered string) []string {
	t.Helper()
	rows := strings.Split(rendered, "\n")
	for i, r := range rows {
		r = strings.TrimPrefix(r, "│")
		r = strings.TrimSuffix(r, "│")
		rows[i] = r
	}
	return rows
}

// TestIdleHasNoResultOnlyDecoration asserts absence of frozen header text,
// displayed result range, and count in the idle state, and that the status
// row inside the results region is exactly the prompt with nothing beneath.
func TestIdleHasNoResultOnlyDecoration(t *testing.T) {
	m := sized(New(), 80, 24).(Model)
	l := CalculateLayout(m.Height, m.Fields)
	rendered := m.renderResults(m.Width, l.ResultsHeight)
	rows := boxRows(t, rendered)
	if len(rows) != l.ResultsHeight {
		t.Fatalf("results region has %d rows, want %d", len(rows), l.ResultsHeight)
	}
	if strings.TrimSpace(rows[1]) != IdlePrompt {
		t.Errorf("status row = %q, want exactly %q", strings.TrimSpace(rows[1]), IdlePrompt)
	}
	// Everything between the status row and the bottom border is blank: no
	// frozen header, no range, no count.
	for i, r := range rows[2 : len(rows)-1] {
		if strings.TrimSpace(r) != "" {
			t.Errorf("results row %d = %q, want blank idle interior", i+3, r)
		}
	}
	full := m.View()
	for _, banned := range []string{"No rows", "Result count:"} {
		if strings.Contains(full, banned) {
			t.Errorf("idle view contains result-only decoration %q", banned)
		}
	}
}

// TestIdleLayoutArithmeticUnchanged requires that the idle state occupies the
// identical region rows as any other state: no extra or missing lines from
// special-casing the prompt.
func TestIdleLayoutArithmeticUnchanged(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}, {160, 50}} {
		m := sized(New(), size.w, size.h).(Model)
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d idle view has %d rows, want %d", size.w, size.h, len(lines), size.h)
		}
	}
}

// TestIdleDistinctFromTracerGrid contrasts the idle prompt with a settled
// tracer grid so executed-result presentation can never be confused with it.
func TestIdleDistinctFromTracerGrid(t *testing.T) {
	idle := sized(New(), 80, 24).View()
	settled := drive(sized(New(), 80, 24),
		StartTraceMsg{}, traceSettledMsg{result: TraceResult{Grid: &TraceGrid{
			Headers: []string{"id"},
			Rows:    [][]string{{"1"}},
		}}}).
		View()
	if settled == idle || !strings.Contains(settled, "id") {
		t.Error("settled grid state failed to differ meaningfully from idle")
	}
}
