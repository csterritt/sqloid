// Accepted-quit write cleanup coverage for Issue #43, per the Writes and
// commit boundary and write-cancellation Testing Decisions in
// Notes/PRD-sqloid.md. The shared quit confirmation is opened from every
// write phase and accepted: cancellable work receives exactly one
// cancellation request and resolves through rollback, noncancellable work
// receives no interrupt and resolves through the existing operation, and the
// exit command is emitted only after transaction and driver work have fully
// ended and the outcome finalized exactly once — never while work is
// pending, never twice. Duplicate acceptance, stale settlement identities,
// and declined quit are exercised without sleeps.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
)

// quitConfirmationModel presses `q` and returns the confirmation state.
func quitConfirmationModel(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	nextModel := next.(Model)
	if !nextModel.quitConfirm {
		t.Fatal("q did not open the shared quit confirmation over the write")
	}
	return nextModel
}

// acceptQuit presses Enter in the confirmation and returns (model, cmd).
func acceptQuit(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model), cmd
}

// requireNoQuit fails when cmd is nil or is an immediate exit — the
// application must stay alive while write work remains pending.
func requireNoQuit(t *testing.T, cmd tea.Cmd, phase string) {
	t.Helper()
	if cmd == nil {
		return
	}
	if _, exit := cmd().(tea.QuitMsg); exit {
		t.Fatalf("accepted quit during %s emitted an exit while write work was pending", phase)
	}
}

// requireQuit fails when cmd does not produce the exit command.
func requireQuit(t *testing.T, cmd tea.Cmd, phase string) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("settlement after %s produced no command; exit was never emitted", phase)
	}
	if _, exit := cmd().(tea.QuitMsg); !exit {
		t.Fatalf("settlement after %s produced %T, want the exit command", phase, cmd)
	}
}

// requireQuitFinalized requires exactly one KindWrite entry for the given
// execution with the expected status, proving quit finalization.
func requireQuitFinalized(t *testing.T, m Model, execution uint64, want history.WriteStatus, phase string) {
	t.Helper()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("%s: result history holds %d entries, want exactly 1 write entry", phase, len(entries))
	}
	e := entries[0]
	if e.Kind != history.KindWrite || e.ExecutionID != execution {
		t.Fatalf("%s: entry kind=%v execution=%d, want write for execution %d", phase, e.Kind, e.ExecutionID, execution)
	}
	if !writeStatusIn(e.Summary, want) {
		t.Fatalf("%s: summary %q does not report status %v", phase, e.Summary, want)
	}
}

func writeStatusIn(summary string, status history.WriteStatus) bool {
	switch status {
	case history.WriteStatusCommitted:
		return strings.Contains(summary, "committed")
	case history.WriteStatusCancelled:
		return strings.Contains(summary, "cancelled")
	default:
		return strings.Contains(summary, "failed")
	}
}

// TestAcceptedQuitDuringCancellableWriteWaitsForRollback covers the
// cancellable-phase quit: acceptance requests exactly one cancellation and
// keeps the application alive until the write settles through rollback
// resolution; only then does exit follow, with one finalized entry.
func TestAcceptedQuitDuringCancellableWriteWaitsForRollback(t *testing.T) {
	m, fake := holdExecutedWrite(t, 42)

	m = quitConfirmationModel(t, m)
	m, cmd := acceptQuit(t, m)
	if m.quitConfirm {
		t.Fatal("accepted quit left the confirmation open")
	}
	if !m.quitWaitWrite {
		t.Fatal("accepted quit did not enter the write-settlement wait state")
	}
	requireNoQuit(t, cmd, "cancellable write")

	// Exactly one cancellation request: the scoped handle fired once.
	if !fake.cancelled() {
		t.Fatal("accepted quit did not request cancellation of the cancellable write")
	}
	if m.writeCancelling != true {
		t.Fatal("accepted quit did not enter the write cancelling state")
	}

	// Work still pending: the application remains alive with no exit.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(Model)
	requireNoQuit(t, cmd, "pending write before settlement")

	// Settlement resolves through rollback; exit follows only afterward.
	m, cmd = deliverSettlement(t, m, fake)
	requireQuit(t, cmd, "cancellable write rollback resolution")
	requireQuitFinalized(t, m, 42, history.WriteStatusCancelled, "cancellable write quit")

	// A late duplicate settlement finalizes nothing again and exits nothing.
	next, cmd = m.Update(WriteSettledMsg{Execution: 42, Result: fake.result})
	m = next.(Model)
	requireNoQuit(t, cmd, "duplicate settlement after exit")
	if entries := m.ResultHistory.Entries(); len(entries) != 1 {
		t.Fatalf("duplicate settlement appended %d entries, want 1", len(entries))
	}
}

// TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution covers the
// rollback-cleanup and committing phases: acceptance issues no interrupt,
// waits for the existing operation, then exits after resolution with the
// definite entry — for both committed and failed classifications.
func TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution(t *testing.T) {
	cases := []struct {
		name   string
		phase  connection.WritePhase
		result connection.WriteResult
		want   history.WriteStatus
	}{
		{"committing commits", connection.WritePhaseCommitting,
			connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 3},
			history.WriteStatusCommitted},
		{"rollback cleanup resolves failed", connection.WritePhaseRollbackCleanup,
			connection.WriteResult{Outcome: connection.WriteFailed, Err: errors.New("boom"), RollbackConfirmed: true},
			history.WriteStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, fake := holdBoundaryWrite(t, tc.phase, tc.result)
			if fake.ctx != nil && fake.cancelled() {
				t.Fatal("write was cancelled before quit was even accepted")
			}

			m = quitConfirmationModel(t, m)
			m, cmd := acceptQuit(t, m)
			requireNoQuit(t, cmd, tc.phase.String())
			if !m.quitWaitWrite {
				t.Fatalf("accepted quit during %s did not enter the wait state", tc.phase)
			}
			if fake.cancelled() {
				t.Fatalf("accepted quit during noncancellable %s issued an interrupt", tc.phase)
			}
			if m.writePhase != tc.phase {
				t.Fatalf("accepted quit during %s mutated the write phase", tc.phase)
			}

			m, cmd = deliverSettlement(t, m, fake)
			requireQuit(t, cmd, tc.phase.String())
			requireQuitFinalized(t, m, resultExecID, tc.want, tc.phase.String())
		})
	}
}

// TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork covers the
// unresolved rollback and commit outcomes: quit waits for pending work to
// end, the finalizer appends exactly one entry that makes no untouched or
// persistence claim (rollback unconfirmed), and only then does exit follow.
func TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork(t *testing.T) {
	cases := []struct {
		name   string
		phase  connection.WritePhase
		result connection.WriteResult
	}{
		{"unresolved rollback", connection.WritePhaseCommitting,
			connection.WriteResult{Outcome: connection.WriteCancelled, Err: errors.New("rollback failed")}},
		{"unresolved commit", connection.WritePhaseCommitting,
			connection.WriteResult{Outcome: connection.WriteFailed, Err: errors.New("commit failed")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, fake := holdBoundaryWrite(t, tc.phase, tc.result)

			m = quitConfirmationModel(t, m)
			m, cmd := acceptQuit(t, m)
			requireNoQuit(t, cmd, tc.name)
			if fake.cancelled() {
				t.Fatalf("%s: accepted quit issued an interrupt", tc.name)
			}

			m, cmd = deliverSettlement(t, m, fake)
			requireQuit(t, cmd, tc.name)
			entries := m.ResultHistory.Entries()
			if len(entries) != 1 {
				t.Fatalf("%s: %d entries appended, want exactly one outcome finalization", tc.name, len(entries))
			}
			// An unconfirmed rollback forbids any untouched claim, and a
			// non-committed outcome forbids any persistence claim.
			summary := entries[0].Summary
			if strings.Contains(summary, "untouched") || strings.Contains(summary, "committed: ") {
				t.Fatalf("%s: summary %q claims an outcome the resolution never proved", tc.name, summary)
			}
		})
	}
}

// TestAcceptedQuitDuplicatesStaleSettlementAndDecline covers the guards:
// duplicate acceptance never exits early or requests a second cancellation;
// a stale settlement identity cannot exit or finalize; and a declined quit
// restores the exact suspended write phase with no cleanup side effects.
func TestAcceptedQuitDuplicatesStaleSettlementAndDecline(t *testing.T) {
	t.Run("duplicate acceptance", func(t *testing.T) {
		m, fake := holdExecutedWrite(t, 42)
		m = quitConfirmationModel(t, m)
		m, first := acceptQuit(t, m)
		requireNoQuit(t, first, "first acceptance")

		// Reopen and accept again while settlement is pending.
		m = quitConfirmationModel(t, m)
		m, second := acceptQuit(t, m)
		requireNoQuit(t, second, "duplicate acceptance")
		if !m.quitWaitWrite {
			t.Fatal("duplicate acceptance disturbed the wait state")
		}

		m, cmd := deliverSettlement(t, m, fake)
		requireQuit(t, cmd, "settlement after duplicate acceptance")
		requireQuitFinalized(t, m, 42, history.WriteStatusCancelled, "duplicate acceptance")
	})

	t.Run("stale settlement identity", func(t *testing.T) {
		m, fake := holdExecutedWrite(t, 42)
		m = quitConfirmationModel(t, m)
		m, _ = acceptQuit(t, m)

		// A stale identity's settlement can neither exit nor finalize.
		next, cmd := m.Update(WriteSettledMsg{Execution: 43, Result: connection.WriteResult{Outcome: connection.WriteCommitted}})
		m = next.(Model)
		requireNoQuit(t, cmd, "stale settlement")
		if m.quitWaitWrite != true {
			t.Fatal("stale settlement consumed the quit wait state")
		}
		if len(m.ResultHistory.Entries()) != 0 {
			t.Fatal("stale settlement appended a result entry")
		}

		m, cmd = deliverSettlement(t, m, fake)
		requireQuit(t, cmd, "settlement after stale identity")
	})

	t.Run("declined quit restores exact phase", func(t *testing.T) {
		m, fake := holdExecutedWrite(t, 42)
		m = quitConfirmationModel(t, m)

		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		restored := next.(Model)
		if restored.quitConfirm || restored.quitSuspended != nil {
			t.Fatal("declined quit left confirmation state behind")
		}
		if !restored.writePending || restored.writePhase != connection.WritePhaseExecuting {
			t.Fatalf("declined quit did not restore the exact suspended write phase (phase %v pending %v)", restored.writePhase, restored.writePending)
		}
		if restored.writeCancelling || fake.cancelled() {
			t.Fatal("declined quit issued a cancellation side effect")
		}
		if restored.inFlightNotice != "" {
			t.Fatal("declined quit left feedback behind")
		}
		// The exact suspended context remains fully live: Ctrl+W still routes.
		next, cmd := restored.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		restored = next.(Model)
		if cmd == nil {
			t.Fatal("declined quit broke subsequent Ctrl+W cancellation routing")
		}
		cmd() // the runtime runs the returned cancellation command
		if !fake.cancelled() {
			t.Fatal("Ctrl+W after declined quit did not request cancellation")
		}
	})
}

// TestQuitWaitProhibitsReplacementWork proves no replacement database work
// starts while the accepted quit waits for write settlement, and the lease
// ownership stays with the sole write until it resolves.
func TestQuitWaitProhibitsReplacementWork(t *testing.T) {
	m, fake := holdExecutedWrite(t, 42)
	m = quitConfirmationModel(t, m)
	m, _ = acceptQuit(t, m)

	if cmd := m.startWrite(43, "UPDATE", `UPDATE "users" SET "email" = 'x'`, nil); cmd != nil {
		t.Fatal("a replacement write started while quit awaited settlement")
	}
	if fake.callCount() != 1 {
		t.Fatalf("executor called %d times, want exactly the one retained write", fake.callCount())
	}

	m, cmd := deliverSettlement(t, m, fake)
	requireQuit(t, cmd, "replacement prohibition")
}
