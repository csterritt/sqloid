// Bubble Tea tests for the disposable tracer's minimal rendering contract,
// per Issue #10 Tasks 3 and 4: typed tracer success and failure messages are
// fed into the Update loop without opening any database, and View renders a
// minimal bordered results grid or a basic non-crashing error state inside
// the Issue #8 responsive shell. The tracer stays visibly isolated for
// mandatory replacement by Issue #22; no Connection logic, driver type, SQL,
// paging, count, history, validation, or recovery behavior may appear here.

package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sizeAndDraw(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	sized, ok := next.(Model)
	if !ok {
		t.Fatalf("WindowSizeMsg returned %T, want ui.Model", next)
	}
	return sized
}

// driveTrace drives a full tracer round trip through Update only: the start
// message with an injected executor enters through Update, the returned
// command carries the blocking work, its completion message re-enters
// Update, and the final model is returned. startCount observes how often the
// injected executor ran.
func driveTrace(t *testing.T, w, h int, execute func(context.Context) TraceResult, startCount *int) Model {
	t.Helper()
	m := sizeAndDraw(t, New(), w, h)

	next, cmd := m.Update(StartTraceMsg{
		Execute: func(ctx context.Context) TraceResult {
			if startCount != nil {
				*startCount++
			}
			return execute(ctx)
		},
	})
	var ok bool
	m, ok = next.(Model)
	if !ok {
		t.Fatalf("StartTraceMsg returned %T, want ui.Model", next)
	}
	if cmd == nil {
		t.Fatal("StartTraceMsg produced no command to run the executor")
	}
	msg := cmd()
	next, _ = m.Update(msg)
	final, ok := next.(Model)
	if !ok {
		t.Fatalf("completion message returned %T, want ui.Model", next)
	}
	return final
}

func successGrid() TraceResult {
	return TraceResult{Grid: &TraceGrid{
		Headers: []string{"id", "v"},
		Rows:    [][]string{{"1", "one"}, {"2", "two"}},
	}}
}

// TestTracerRendersBorderedGridWithHeadersAndRows pins the minimal grid
// contract: column headers plus rows render inside the existing bordered
// results region at the normal shell layout arithmetic for 80x24, the
// executor runs exactly once, and the builder bar remains untouched.
func TestTracerRendersBorderedGridWithHeadersAndRows(t *testing.T) {
	started := 0
	m := driveTrace(t, 80, 24, func(context.Context) TraceResult { return successGrid() }, &started)
	if started != 1 {
		t.Errorf("executor ran %d times, want exactly 1", started)
	}
	view := m.View()

	for _, want := range []string{"id", "one", "two"} {
		if !strings.Contains(view, want) {
			t.Errorf("rendered view does not contain %q:\n%s", want, view)
		}
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Errorf("view has %d lines, want exactly the terminal height 24 (layout intact)", len(lines))
	}
	if !strings.Contains(view, "─") || !strings.Contains(view, "│") {
		t.Error("tracer output is not drawn inside a border")
	}
	if !strings.Contains(view, "Command:") {
		t.Error("builder bar missing while tracer renders")
	}
	// Tracer state must stay isolated from builder fields rather than leaking
	// rows or headers into the builder bar.
	for _, f := range m.Fields {
		if strings.Contains(f.Content, "one") || strings.Contains(f.Content, "two") {
			t.Errorf("tracer row data leaked into builder field %q", f.Label)
		}
	}
}

// TestTracerErrorStateRendersWithoutCrashing pins the basic error state: a
// failed execution shows its typed error text without panicking and without
// claiming any unsupported recovery behavior.
func TestTracerErrorStateRendersWithoutCrashing(t *testing.T) {
	const errText = "could not trace SELECT * FROM \"t\": no such table: t"
	m := driveTrace(t, 80, 24, func(context.Context) TraceResult { return TraceResult{Err: errText} }, nil)
	view := m.View()

	if !strings.Contains(view, errText) {
		t.Errorf("view does not contain the error text %q:\n%s", errText, view)
	}
	lower := strings.ToLower(view)
	for _, claim := range []string{"page 1", "row count", "history", "retry", "recovered"} {
		if strings.Contains(lower, claim) {
			t.Errorf("tracer output %q claims unsupported feature %q", lower, claim)
		}
	}
}

// TestTracerUnsetStartMsgNilExecutorIsSafe proves a zero-value start message
// never panics before any execution exists.
func TestTracerUnsetStartMsgNilExecutorIsSafe(t *testing.T) {
	m := sizeAndDraw(t, New(), 80, 24)
	next, cmd := m.Update(StartTraceMsg{Execute: nil})
	final := next.(Model)
	if cmd == nil {
		t.Fatal("StartTraceMsg must always produce a completion command")
	}
	msg := cmd() // nil-executor completion still settles safely
	next, _ = final.Update(msg)
	if _, ok := next.(Model); !ok {
		t.Fatalf("nil-executor completion returned %T, want ui.Model", next)
	}
}
