// Disposable Bubble Tea composition path for the Issue #10 tracer: messages
// that start one hardcoded SELECT * trace, the isolated tracer state they
// settle into, and the command boundary that keeps all blocking database
// work inside tea.Cmd functions via an injected Schema/Connection-facing
// executor. This package never opens a database, runs SQL, or touches driver
// types itself; composition wires real executors in. The entire path exists
// only to de-risk the integration stack — Issue #22 must replace it rather
// than extend it into the production query path.

package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// TraceGrid is the tracer's rendered data: column headers plus row cell text
// in order. It carries no paging, count, or history metadata because none
// exists at this milestone.
type TraceGrid struct {
	Headers []string
	Rows    [][]string
}

// TraceResult is the typed completion of one disposable trace execution.
// Exactly one of Grid or Err is meaningful: Grid is non-nil precisely when
// the execution succeeded, and Err holds the failure text otherwise.
type TraceResult struct {
	Grid *TraceGrid
	Err  string
}

// StartTraceMsg begins one disposable tracer round trip. Execute performs the
// Schema/Connection-facing work and returns the typed completion; it always
// runs inside a returned tea.Cmd, never directly in Update or View, and may
// block until the execution settles. A nil Execute yields a completion with
// neither grid nor error rather than panicking.
type StartTraceMsg struct {
	Execute func(ctx context.Context) TraceResult
}

// traceSettledMsg carries the executor's settled completion back through
// Update. Unexported: only this package produces it.
type traceSettledMsg struct{ result TraceResult }

// TraceView is the model's complete tracer state. It is deliberately a single
// isolated field — not woven into builder fields or shell regions — so Issue
// #22 can remove and replace it wholesale.
type TraceView struct {
	Grid    *TraceGrid // non-nil on success
	Err     string     // non-empty failure text on error
	Settled bool       // true once a completion message was applied
}

// SettledTracer reports whether tracer state exists and has completed.
func (m Model) SettledTracer() bool {
	return m.Trace != nil && m.Trace.Settled
}

// handleStartTrace turns a start message into the command that runs the
// injected executor. Executing here (not in Update) keeps every blocking
// database call off the Update goroutine per Bubble Tea rules.
func handleStartTrace(msg StartTraceMsg) tea.Msg {
	if msg.Execute == nil {
		return traceSettledMsg{}
	}
	return traceSettledMsg{result: msg.Execute(context.Background())}
}

// applyTraceResult stores the settled completion as fresh, fully owned
// isolated tracer state, replacing any previous trace outright.
func (m Model) applyTraceResult(result TraceResult) Model {
	m.Trace = &TraceView{Settled: true, Grid: result.Grid, Err: result.Err}
	return m
}
