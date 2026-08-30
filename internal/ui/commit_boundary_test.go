// Commit-boundary Ctrl+W routing coverage for Issue #43, per the Writes and
// commit boundary and the Global Key Precedence and Context/Action Matrix in
// Notes/PRD-sqloid.md. A barrier-held fake executor (no sleeps) holds the
// model's sole write in each phase: before the boundary, Ctrl+W requests
// cancellation exactly once against the active write execution; once
// rollback cleanup or committing has begun, Ctrl+W issues no context
// cancellation or interrupt, leaves the phase and work unchanged, and shows
// exactly `Commit in progress; cancellation is no longer available`. Repeated
// keys, phase regressions, and stale execution identities cannot cross the
// boundary backward, interrupt cleanup/commit, or start replacement work.

package ui

import (
	"context"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
)

// boundaryWriteFake is a controllable executor that delivers scripted
// phases, then blocks inside the write until released — the barrier that
// keeps the model's write deterministically pending without sleeps.
type boundaryWriteFake struct {
	mu      sync.Mutex
	calls   int
	ctx     context.Context
	phases  []connection.WritePhase
	result  connection.WriteResult
	entered chan struct{}
	release chan struct{}
}

func newBoundaryWriteFake(phases []connection.WritePhase, result connection.WriteResult) *boundaryWriteFake {
	return &boundaryWriteFake{
		phases:  phases,
		result:  result,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

// Write implements the WriteExecutor seam.
func (f *boundaryWriteFake) Write(ctx context.Context, execution uint64, sql string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	f.ctx = ctx
	for _, p := range f.phases {
		phase(connection.WritePhaseMsg{Execution: execution, Phase: p})
	}
	close(f.entered)
	<-f.release
	return f.result
}

func (f *boundaryWriteFake) cancelled() bool {
	return f.ctx != nil && f.ctx.Err() == context.Canceled
}

// callCount returns the executor invocation count observed so far.
func (f *boundaryWriteFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// startHeldWrite dispatches one held write at the given execution identity
// and drives its phase relays through Update, returning the model suspended
// exactly after the last scripted phase with settlement withheld.
func startHeldWrite(t *testing.T, m Model, fake *boundaryWriteFake, execution uint64) Model {
	t.Helper()
	cmd := m.startWrite(execution, "UPDATE", `UPDATE "users" SET "email" = 'new'`, nil)
	if cmd == nil {
		t.Fatal("held write dispatch produced no command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("write dispatch produced %T, want the two-command write batch", cmd())
	}
	settlementsCh := make(chan tea.Msg, 1)
	run, relay := batch[0], batch[1]
	go func() { settlementsCh <- run() }()
	<-fake.entered // the fake has buffered every scripted phase
	// Drive exactly one relay per scripted phase so the model consumes the
	// whole scripted phase history while settlement stays withheld behind
	// the barrier; the next relay only fires at release.
	for i := 0; i < len(fake.phases); i++ {
		msg, ok := relay().(connection.WritePhaseMsg)
		if !ok {
			t.Fatal("relay command delivered no phase message")
		}
		next, nextRelay := m.Update(msg)
		m = next.(Model)
		relay = nextRelay
	}
	return m
}

// deliverSettlement releases the held fake and applies its settled message,
// returning the model and the command Update produced at settlement.
func deliverSettlement(t *testing.T, m Model, fake *boundaryWriteFake) (Model, tea.Cmd) {
	t.Helper()
	fake.releaseOnce()
	next, cmd := m.Update(WriteSettledMsg{Execution: m.writeExecution, Result: fake.result})
	return next.(Model), cmd
}

func (f *boundaryWriteFake) releaseOnce() {
	select {
	case <-f.release:
	default:
		close(f.release)
	}
}

// holdExecutedWrite returns a model whose write is pending in the executing
// phase with a cancellable cancellation handle installed.
func holdExecutedWrite(t *testing.T, execution uint64) (Model, *boundaryWriteFake) {
	t.Helper()
	fake := newBoundaryWriteFake(
		[]connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting},
		connection.WriteResult{Outcome: connection.WriteCancelled, RollbackConfirmed: true},
	)
	m := New()
	m.Write = fake.Write
	t.Cleanup(fake.releaseOnce)
	return startHeldWrite(t, m, fake, execution), fake
}

// holdBoundaryWrite returns a model whose write is pending in the given
// noncancellable boundary phase.
func holdBoundaryWrite(t *testing.T, phase connection.WritePhase, result connection.WriteResult) (Model, *boundaryWriteFake) {
	t.Helper()
	fake := newBoundaryWriteFake(
		[]connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, phase},
		result,
	)
	m := New()
	m.Write = fake.Write
	t.Cleanup(fake.releaseOnce)
	return startHeldWrite(t, m, fake, resultExecID), fake
}

const resultExecID uint64 = 77

// TestWriteCtrlWCancelsOnceBeforeBoundary covers the pre-boundary rule:
// Ctrl+W in the cancellable executing phase dispatches the scoped
// cancellation exactly once against the active write, and repeated Ctrl+W
// deduplicates — no second cancellation request is issued.
func TestWriteCtrlWCancelsOnceBeforeBoundary(t *testing.T) {
	m, fake := holdExecutedWrite(t, 42)
	if !m.writePending || m.writePhase != connection.WritePhaseExecuting {
		t.Fatalf("held write phase = %v pending=%v, want executing/pending", m.writePhase, m.writePending)
	}
	if !m.ActiveCancellable || m.CancelCommand == nil {
		t.Fatal("cancellable write did not install its cancellation handle")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Ctrl+W in the cancellable executing phase dispatched no cancellation command")
	}
	if _, ok := cmd().(WriteCancelRequestedMsg); !ok {
		t.Fatalf("Ctrl+W produced %T, want a write cancellation request", cmd())
	}
	if !fake.cancelled() {
		t.Fatal("scoped cancellation never reached the active write execution")
	}
	if !m.writeCancelling {
		t.Fatal("Ctrl+W did not enter the visible cancelling state")
	}

	// Repeated keys are deduplicated: the cancellation request is issued
	// exactly once, so a fresh observer handle is never invoked again.
	cancels := 0
	m.CancelCommand = func() tea.Msg { cancels++; return WriteCancelRequestedMsg{} }
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("repeated Ctrl+W dispatched %v; pre-boundary cancellation must be requested once", cmd)
	}
	if cancels != 0 {
		t.Fatalf("repeated Ctrl+W issued %d extra cancellation requests, want 0", cancels)
	}
}

// TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback covers the irreversible
// boundary: once rollback cleanup or committing has begun, Ctrl+W issues no
// context cancellation, leaves the phase and pending work untouched, and
// shows exactly the boundary feedback — repeated keys included.
func TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback(t *testing.T) {
	results := map[connection.WritePhase]connection.WriteResult{
		connection.WritePhaseRollbackCleanup: {Outcome: connection.WriteFailed, Err: context.Canceled, RollbackConfirmed: true},
		connection.WritePhaseCommitting:      {Outcome: connection.WriteCommitted, RowsAffected: 3},
	}
	for phase, result := range results {
		t.Run(phase.String(), func(t *testing.T) {
			m, fake := holdBoundaryWrite(t, phase, result)
			if m.writePhase != phase || !m.writePending {
				t.Fatalf("held write phase = %v pending=%v, want %v/pending", m.writePhase, m.writePending, phase)
			}
			if m.writeNoncancellable != true {
				t.Fatal("boundary phase did not install the typed noncancellable state")
			}
			if m.ActiveCancellable || m.CancelCommand != nil {
				t.Fatalf("%s phase left a cancellation handle installed", phase)
			}

			for i := 0; i < 3; i++ {
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
				m = next.(Model)
				if cmd != nil {
					t.Fatalf("Ctrl+W during %s dispatched %v; the boundary is noncancellable", phase, cmd)
				}
				if got := m.inFlightNotice; got != CommitBoundaryFeedback {
					t.Fatalf("Ctrl+W during %s feedback = %q, want exactly %q", phase, got, CommitBoundaryFeedback)
				}
				if fake.cancelled() {
					t.Fatalf("Ctrl+W during %s issued a context cancellation", phase)
				}
				if m.writePhase != phase || !m.writePending || fake.ctx == nil {
					t.Fatalf("Ctrl+W during %s mutated the write work (phase %v pending %v)", phase, m.writePhase, m.writePending)
				}
			}
		})
	}
}

// TestWriteBoundaryResistsRegressionAndStaleIdentities covers the guards:
// after the boundary, a late beginning/executing phase for the same
// execution, a duplicate boundary phase, or a phase message from a stale
// execution identity cannot re-enable cancellation or change the boundary
// state, and no replacement write may start while the current one pends.
func TestWriteBoundaryResistsRegressionAndStaleIdentities(t *testing.T) {
	m, fake := holdBoundaryWrite(t, connection.WritePhaseCommitting,
		connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 3})

	// Regression attempt: the same execution claims to be executing again.
	m = apply(m, connection.WritePhaseMsg{Execution: resultExecID, Phase: connection.WritePhaseExecuting})
	if m.ActiveCancellable || m.CancelCommand != nil || !m.writeNoncancellable {
		t.Fatal("a regressed executing phase crossed the boundary backward and re-enabled cancellation")
	}
	// Duplicate boundary phase: equally inert.
	m = apply(m, connection.WritePhaseMsg{Execution: resultExecID, Phase: connection.WritePhaseCommitting})
	if !m.writeNoncancellable || m.writePhase != connection.WritePhaseCommitting {
		t.Fatal("a duplicate boundary phase mutated the boundary state")
	}
	// Stale identity: never touches the current boundary state.
	m = apply(m, connection.WritePhaseMsg{Execution: resultExecID + 1, Phase: connection.WritePhaseBeginning})
	if m.writePhase != connection.WritePhaseCommitting || m.writeNoncancellable != true {
		t.Fatal("a stale execution's phase message mutated the current write")
	}
	// Ctrl+W remains boundary feedback, never a cancellation.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(Model)
	if cmd != nil || m.inFlightNotice != CommitBoundaryFeedback {
		t.Fatalf("Ctrl+W after a regression attempt dispatched %v with notice %q", cmd, m.inFlightNotice)
	}
	// No replacement write may start while the boundary write pends.
	if cmd := m.startWrite(resultExecID+2, "UPDATE", `UPDATE "users" SET "email" = 'x'`, nil); cmd != nil {
		t.Fatal("a replacement write started during pending boundary work")
	}
	if fake.callCount() != 1 {
		t.Fatalf("executor called %d times, want exactly the one held write", fake.calls)
	}
}
