// Transactional write orchestration inside the UI (Issue #42), per the
// Writes and commit boundary and write-transaction decisions in
// Notes/PRD-sqloid.md. One confirmed UPDATE/DELETE or one dispatched runnable
// INSERT becomes exactly one actual write execution: the execution-start
// boundary exits either history first and appends the complete query state
// subject only to consecutive-identical suppression, then dispatches the
// sole write through the WriteExecutor seam. Connection's typed phase and
// outcome messages flow back through Update: phases are retained state only
// (post-boundary rendering stays with Issue #43), rollback-cleanup/committing
// phases make the write noncancellable, and settlement finalizes exactly one
// immutable non-tabular result entry carrying the retained executed SQL and
// the operation-appropriate summary. Duplicate, late, and stale messages are
// idempotent no-ops; unresolved rollback/commit outcomes remain Issue #45's.

package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// WriteExecutor performs one transactional write execution for the given
// execution identity and the safely rendered statement and ordered bound
// parameters. As phases progress it delivers each connection.WritePhaseMsg
// through phase (synchronously is fine; the command relays them), and it
// returns the resolved connection.WriteResult. It always runs inside a
// tea.Cmd — never in Update or View. nil means no execution is wired: the
// execution-start boundary still exits history and appends query history,
// but no database work is issued.
type WriteExecutor func(ctx context.Context, execution uint64, sql string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult

// WriteSettledMsg carries one resolved write back through Update with the
// execution identity that issued it, so duplicate, late, or superseded
// completions cannot finalize a second entry. Produced only by commands this
// package created or by test fakes driving the same seam.
type WriteSettledMsg struct {
	Execution uint64
	Result    connection.WriteResult
}

// WriteCancelRequestedMsg is produced by the cancellation closure dispatched
// at Ctrl+W over a cancellable write phase. The model has already entered its
// cancelling state before dispatching; settlement through rollback cleanup is
// what resolves the visible state.
type WriteCancelRequestedMsg struct{}

// CommitBoundaryFeedback is the exact Ctrl+W feedback (Issue #43) shown once
// the commit boundary has been crossed: rollback cleanup or committing is in
// progress, cancellation is permanently unavailable, and the write's work is
// never mutated by the key. It is presentation-only; routing uses the typed
// writeNoncancellable state, never this text.
const CommitBoundaryFeedback = "Commit in progress; cancellation is no longer available"

// Exact write-phase status labels (Issue #44), mapped from the typed
// connection.WritePhase state only. WriteRollingBackIndicator is the exact
// status while the noncancellable rollback cleanup runs, and
// WriteCommittingIndicator the exact status while the noncancellable commit
// runs. The cancellable beginning/executing phases reuse the established
// `Running…` wording, and a requested cancellation reuses the established
// `cancelling…` wording, mirroring the read phases without changing them.
const (
	WriteRollingBackIndicator = "Rolling back…"
	WriteCommittingIndicator  = "Committing…"
)

// startWrite enters the actual-write lifecycle: it records the execution
// identity, operation, and executed standalone SQL for finalization, marks
// the write pending and cancellable, and dispatches one batched pair of
// commands — the phase relay and the sole write execution. It is refused
// while a write is already pending, for a stale identity, or without a wired
// executor.
func (m *Model) startWrite(execution uint64, operation, sql string, params []any) tea.Cmd {
	if m.Write == nil || execution == 0 || m.writePending || execution == m.writeExecution {
		return nil
	}
	m.writeExecution = execution
	m.writeOperation = operation
	m.writeSQL = sql
	m.writePending = true
	m.writeCancelling = false
	m.writeFinalized = false
	m.ActiveCancellable = true
	ctx, cancel := context.WithCancel(context.Background())
	m.writeCancel = cancel
	m.CancelCommand = func() tea.Msg {
		cancel()
		return WriteCancelRequestedMsg{}
	}
	phases := make(chan connection.WritePhaseMsg, 4)
	m.writePhases = phases
	write := m.Write
	relay := func() tea.Msg {
		msg, ok := <-phases
		if !ok {
			return nil
		}
		return msg
	}
	run := func() tea.Msg {
		defer close(phases)
		return WriteSettledMsg{
			Execution: execution,
			Result:    write(ctx, execution, sql, params, func(p connection.WritePhaseMsg) { phases <- p }),
		}
	}
	// The write command runs first so its fake/executor buffers the phases
	// before the relay command reads them; the Bubble Tea runtime runs batch
	// members concurrently, so production behavior is unchanged.
	return tea.Batch(run, relay)
}

// waitForWritePhase returns the command that delivers the next buffered write
// phase message; a nil message (channel closed after settlement) ends the
// relay without touching state.
func (m *Model) waitForWritePhase() tea.Cmd {
	if m.writePhases == nil {
		return nil
	}
	phases := m.writePhases
	return func() tea.Msg {
		msg, ok := <-phases
		if !ok {
			return nil
		}
		return msg
	}
}

// applyWritePhase records one phase transition for the current execution.
// Stale identities and post-settlement arrivals are discarded. Once the write
// enters noncancellable rollback-cleanup or committing, cancellation
// ownership is retired so further Ctrl+W can reach no interrupt.
func (m *Model) applyWritePhase(msg connection.WritePhaseMsg) tea.Cmd {
	if m.writePhases == nil || msg.Execution != m.writeExecution || !m.writePending {
		return nil
	}
	if m.writeNoncancellable {
		// Issue #43: once the boundary is crossed, only the actual rollback
		// cleanup or committing transition is accepted; a regressed
		// beginning/executing phase or a repeated one can never cross the
		// boundary backward and re-enable cancellation.
		return m.waitForWritePhase()
	}
	if m.writePhase == msg.Phase {
		// Duplicate delivery of the current phase is an idempotent no-op.
		return m.waitForWritePhase()
	}
	m.writePhase = msg.Phase
	if msg.Phase == connection.WritePhaseRollbackCleanup || msg.Phase == connection.WritePhaseCommitting {
		m.writeNoncancellable = true
		m.ActiveCancellable = false
		m.CancelCommand = nil
		m.writeCancel = nil
	}
	return m.waitForWritePhase()
}

// applyWriteSettled finalizes one resolved write: it creates exactly one
// immutable non-tabular result entry for the execution, built from the
// retained executed SQL and the actual statement RowsAffected with
// operation-appropriate wording, then retires the write state. Duplicate,
// late, and stale settlement messages append nothing; the result store's
// execution-identity guard backstops the model's finalized flag.
func (m *Model) applyWriteSettled(msg WriteSettledMsg) {
	if m.writePhases == nil || msg.Execution != m.writeExecution || m.writeFinalized {
		return
	}
	m.writeFinalized = true
	m.writePending = false
	m.writeCancelling = false
	m.writeNoncancellable = false
	m.ActiveCancellable = false
	m.CancelCommand = nil
	m.writeCancel = nil
	m.writePhases = nil

	var status history.WriteStatus
	switch msg.Result.Outcome {
	case connection.WriteCommitted:
		status = history.WriteStatusCommitted
	case connection.WriteCancelled:
		status = history.WriteStatusCancelled
	default:
		status = history.WriteStatusFailed
	}
	cause := ""
	if msg.Result.Err != nil {
		cause = msg.Result.Err.Error()
	}
	m.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID:  msg.Execution,
		Kind:         history.KindWrite,
		SQL:          m.writeSQL,
		Summary:      history.WriteSummary(m.writeOperation, status, msg.Result.RowsAffected, msg.Result.RollbackConfirmed, cause),
		RowsAffected: msg.Result.RowsAffected,
	})
}

// writePhaseStatus is the authoritative write-phase presentation mapping
// (Issue #44): the exact status label for the current typed write state, or
// empty text when no write is pending. Rollback cleanup and committing are
// the most specific typed phases and take precedence over the cancellation
// request, which otherwise holds `cancelling…` from Ctrl+W until settlement;
// the cancellable beginning/executing phases render `Running…`. The mapping
// never inspects rendered label text or command shape, and stale/duplicate
// phase identities can never move it backward (guarded in applyWritePhase).
func (m Model) writePhaseStatus() string {
	switch {
	case m.writePhase == connection.WritePhaseRollbackCleanup:
		return WriteRollingBackIndicator
	case m.writePhase == connection.WritePhaseCommitting:
		return WriteCommittingIndicator
	case m.writeCancelling:
		return SelectCancellingIndicator
	case m.writePending:
		return SelectRunningIndicator
	default:
		return ""
	}
}

// beginConfirmedWrite is the confirmed-UPDATE/DELETE execution-start boundary:
// it exits either history first, appends the complete query state at actual
// execution start (subject only to consecutive-identical suppression), and
// dispatches the sole transactional write of the retained rendered statement.
func (m *Model) beginConfirmedWrite(msg WriteConfirmedMsg) tea.Cmd {
	m.exitHistoryMode()
	m.exitResultHistoryMode()
	if m.History != nil {
		m.History.AppendExecution(m.QB.HistoryState())
	}
	params := []any(nil)
	switch m.QB.Command() {
	case qb.CommandUpdate:
		params = m.QB.UpdateParams()
	case qb.CommandDelete:
		params = m.QB.DeleteParams()
	default:
		return nil
	}
	return m.startWrite(msg.Execution, msg.Operation, msg.SQL, params)
}
