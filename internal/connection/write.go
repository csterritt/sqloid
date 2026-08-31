// Transactional write execution (Issue #42), per the Write transaction
// implementation decision and the Writes and commit boundary section of
// Notes/PRD-sqloid.md. One confirmed UPDATE/DELETE or one runnable INSERT is
// the sole actual execution of its write: it verifies the startup path
// identity exactly once at the request boundary (inside Lease, before
// BEGIN), runs BEGIN and exactly one statement on one dedicated leased
// connection, and then either commits or rolls back after an atomic
// cancellation-flag check that occurs after statement completion and
// immediately before beginning COMMIT — so a cancellation requested at any
// cancellable point, even after a successful statement, wins over commit.
// Rollback cleanup and commit are noncancellable. Cancellation and statement
// failure (including native constraint and trigger errors) both pass through
// rollback cleanup and wait for confirmed rollback before resolving; the
// result never reports the database as untouched unless rollback succeeded.
// The write runs entirely inside one request, so there is exactly one
// pre-BEGIN identity check and none between statement and COMMIT.
// Cancellation never force-closes the connection and the lease is released
// only at settlement, after which the physical connection is reusable.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"sync"
)

// WritePhase is one observable phase of a transactional write request. The
// phases run in this order for a committed write: Beginning, Executing,
// Committing. A cancelled or failed write replaces Committing with
// RollbackCleanup; a commit failure additionally passes through
// RollbackCleanup after Committing.
type WritePhase int

const (
	// WritePhaseBeginning means the write owns its lease and is starting its
	// transaction on it. Cancellation is still available.
	WritePhaseBeginning WritePhase = iota + 1
	// WritePhaseExecuting means the transaction is open and the sole
	// statement is about to run or is running. Cancellation is still
	// available.
	WritePhaseExecuting
	// WritePhaseRollbackCleanup means a cancellation or statement failure
	// initiated the noncancellable rollback of the open transaction.
	WritePhaseRollbackCleanup
	// WritePhaseCommitting means the statement succeeded, the atomic
	// pre-COMMIT cancellation check passed, and the noncancellable commit is
	// starting.
	WritePhaseCommitting
)

// String renders the phase name for tests and diagnostics.
func (p WritePhase) String() string {
	switch p {
	case WritePhaseBeginning:
		return "beginning"
	case WritePhaseExecuting:
		return "executing"
	case WritePhaseRollbackCleanup:
		return "rollback-cleanup"
	case WritePhaseCommitting:
		return "committing"
	default:
		return "WritePhase(" + itoa(int(p)) + ")"
	}
}

// WriteOutcome is the definite terminal classification of a resolved write.
// Unresolved rollback/commit outcomes are Issue #45's terminal workflow and
// are not produced here.
type WriteOutcome int

const (
	// WriteCommitted means the transaction committed and the summary may
	// report the statement's RowsAffected as persisted.
	WriteCommitted WriteOutcome = iota + 1
	// WriteCancelled means the write was cancelled and rolled back;
	// RollbackConfirmed says whether the rollback itself succeeded.
	WriteCancelled
	// WriteFailed means the statement or commit failed (including native
	// constraint and trigger errors) and the transaction rolled back;
	// RollbackConfirmed says whether the rollback itself succeeded.
	WriteFailed
)

// String renders the outcome name for tests and diagnostics.
func (o WriteOutcome) String() string {
	switch o {
	case WriteCommitted:
		return "committed"
	case WriteCancelled:
		return "cancelled"
	case WriteFailed:
		return "failed"
	default:
		return "WriteOutcome(" + itoa(int(o)) + ")"
	}
}

// WritePhaseMsg carries one write phase transition to the UI for the named
// execution. Produced by StartWrite's phase channel; duplicate or late
// delivery is the UI's concern (idempotent handling), never re-emitted here.
type WritePhaseMsg struct {
	Execution uint64
	Phase     WritePhase
}

// WriteResult is the resolved terminal result of one transactional write.
// RowsAffected is the actual statement RowsAffected() whenever the statement
// itself produced a result, and is meaningful as persisted exactly on
// WriteCommitted; callers must never treat it as persistence on other
// outcomes.
type WriteResult struct {
	Outcome WriteOutcome

	// RowsAffected is the actual statement RowsAffected(), not an estimate.
	RowsAffected int64

	// Err preserves the underlying statement or commit cause on failure; it
	// stays nil on success unless lease release itself failed, in which case
	// the release error replaces it.
	Err error

	// Health is non-nil only when deletion or replacement was classified at
	// the request boundary or after an error, taking the race precedence.
	Health *HealthError

	// RollbackConfirmed is true exactly when a rollback of the open
	// transaction was confirmed successful. Until it is true no result may
	// claim the database was untouched.
	RollbackConfirmed bool
}

// StartedWriteRequest is one running transactional write on its dedicated
// leased connection whose cancellation is requested from outside the work
// goroutine. The lifecycle is exactly: StartWrite → Cancel at most
// effectively once (meaningful only while the write is cancellable) → the
// write settles through rollback cleanup or commit and releases its lease
// automatically → Wait returns the resolved WriteResult. Phases are
// delivered on the Phases channel in order; the channel is closed at
// settlement, so a consumer can always drain every transition that occurred.
type StartedWriteRequest struct {
	execution uint64
	owner     *DB
	req       *Request
	lease     *Lease
	phases    chan WritePhaseMsg

	// mu guards the atomic cancellation-to-commit boundary: Cancel reads
	// and mutates noncancellable under it, and the pre-COMMIT check flips
	// noncancellable under it in the same critical section that reads the
	// cancellation flag. A cancellation therefore either wins the flag check
	// and forces rollback, or is permanently disabled — never both and never
	// neither.
	mu sync.Mutex

	// noncancellable is set exactly once, at the atomic pre-COMMIT boundary,
	// and permanently disables interrupt issuance for this execution: after
	// it is set, Cancel dispatches no context cancellation and no driver
	// interrupt, and rollback cleanup/committing cannot be interrupted.
	noncancellable bool

	// final is written by the work goroutine before settlement is announced;
	// Wait reads it only after SettledChan is closed, which synchronizes it.
	final   WriteResult
	settled chan struct{}
}

// StartWrite begins one transactional write of the complete statement (and
// ordered bound parameters) rendered by the QueryBuilder seam under the
// given process-unique execution identity. It acquires one dedicated lease —
// which is the write's single request-boundary identity check — and runs
// BEGIN, the one statement, and the pre-COMMIT cancellation check, rollback
// cleanup, and commit on it. Exactly one definite outcome is produced per
// execution; the lease is never force-closed and is released only after
// settlement, so the physical connection stays healthy for later requests.
func (db *DB) StartWrite(parent context.Context, execution uint64, statement string, params []any) *StartedWriteRequest {
	w := &StartedWriteRequest{
		execution: execution,
		owner:     db,
		phases:    make(chan WritePhaseMsg, 4),
		settled:   make(chan struct{}),
	}

	lease, err := db.Lease(parent)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			w.deliver(WriteResult{Outcome: WriteCancelled, Err: err})
		} else {
			var he *HealthError
			if errors.As(err, &he) {
				w.deliver(WriteResult{Outcome: WriteFailed, Err: err, Health: he})
			} else {
				w.deliver(WriteResult{Outcome: WriteFailed, Err: err})
			}
		}
		return w
	}
	if db.writeLeaseHook != nil {
		db.writeLeaseHook(lease)
	}

	request := lease.BeginRequest(parent)
	w.req = request
	w.lease = lease
	conn := lease.Conn()
	go func() {
		res := w.run(conn, statement, params)
		outcome := request.Settle(res.Err)
		closeErr := request.Close()
		switch outcome {
		case OutcomeSuccess:
			res.Outcome = WriteCommitted
		case OutcomeCancelled:
			res.Outcome = WriteCancelled
		default:
			res.Outcome = WriteFailed
			var he *HealthError
			if errors.As(db.VerifyHealth(), &he) {
				res.Health = he
			}
		}
		if res.Err == nil && closeErr != nil {
			res.Err = closeErr
		}
		w.deliver(res)
	}()
	return w
}

// emit delivers one phase message without blocking the write: the channel is
// buffered for the maximum number of phases a single write can produce.
func (w *StartedWriteRequest) emit(phase WritePhase) {
	w.phases <- WritePhaseMsg{Execution: w.execution, Phase: phase}
}

// deliver closes the phase stream first, so every consumer observes the full
// phase history before settlement is announced, then records the final
// result and announces settlement exactly once.
func (w *StartedWriteRequest) deliver(res WriteResult) {
	w.final = res
	close(w.phases)
	close(w.settled)
}

// run executes the phased transaction on the leased connection and returns
// the provisional result whose Err drives the Issue #6 settlement
// classification. The control flow is exactly the PRD's commit boundary:
// BEGIN → sole statement → atomic cancellation check → rollback cleanup or
// COMMIT, with the barrier seams (test-only) between a phase message and the
// next transaction step.
func (w *StartedWriteRequest) run(conn *sql.Conn, statement string, params []any) WriteResult {
	ctx := w.req.Context()
	w.emit(WritePhaseBeginning)
	if w.owner.beforeWriteBegin != nil {
		w.owner.beforeWriteBegin(ctx, conn)
	}
	tx, beginErr := conn.BeginTx(ctx, nil)
	if beginErr != nil {
		// Nothing to roll back: the transaction never opened. A cancellation
		// cause stays classified cancelled with no rollback confirmation, so
		// no untouched claim can be made.
		return WriteResult{Err: beginErr}
	}

	w.emit(WritePhaseExecuting)
	if w.owner.beforeWriteExec != nil {
		w.owner.beforeWriteExec(ctx, conn)
	}
	var rowsAffected int64
	res, err := tx.ExecContext(ctx, statement, params...)
	if err == nil {
		rowsAffected, err = res.RowsAffected()
	}
	if err != nil {
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected})
	}

	// Statement completed successfully. The atomic pre-COMMIT boundary
	// follows immediately: under one lock the request's cancellation flag is
	// read and noncancellable is set permanently, so a cancellation either
	// wins the flag check and forces rollback after the successful statement,
	// or arrives after crossing and is ignored. Only after the boundary is
	// crossed may the committing phase be announced and COMMIT begin.
	if w.owner.beforeWriteCommit != nil {
		w.owner.beforeWriteCommit(ctx, conn)
	}
	w.mu.Lock()
	cancelled := w.req.CancelRequested()
	w.noncancellable = true
	w.mu.Unlock()
	if cancelled {
		err = context.Canceled
	}
	if err != nil {
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected})
	}
	w.emit(WritePhaseCommitting)
	if err := tx.Commit(); err != nil {
		// Persistence after a failed commit is unprovable; Issue #45 owns
		// the outcome-unknown terminal workflow. Issue #42 resolves it as a
		// failed write whose open transaction is rolled back if possible.
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected})
	}
	return WriteResult{Outcome: WriteCommitted, RowsAffected: rowsAffected}
}

// rollback performs the noncancellable rollback cleanup phase and marks the
// result with confirmed rollback exactly when the rollback succeeded. It
// re-establishes the noncancellable boundary (the statement-failure path
// enters rollback cleanup from executing without passing the pre-COMMIT
// check), so no later Cancel can interrupt cleanup. The outcome is
// provisional: Settle classifies a cancellation cause as WriteCancelled and
// everything else as WriteFailed.
func (w *StartedWriteRequest) rollback(ctx context.Context, tx *sql.Tx, res WriteResult) WriteResult {
	w.mu.Lock()
	w.noncancellable = true
	w.mu.Unlock()
	w.emit(WritePhaseRollbackCleanup)
	if w.owner.beforeWriteRollback != nil {
		w.owner.beforeWriteRollback(ctx, nil)
	}
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		res.Err = err
		res.RollbackConfirmed = false
		return res
	}
	res.RollbackConfirmed = true
	return res
}

// Cancel requests cancellation of the write. The first call while the write
// is in the cancellable beginning/executing phases is the only meaningful
// one: it sets the request's cancellation flag once and dispatches one
// connection-scoped interrupt against the write's leased connection. Once
// the atomic pre-COMMIT boundary has been crossed — rollback cleanup or
// committing has begun, or the write settled — Cancel is permanently inert:
// it issues no context cancellation and no driver interrupt and leaves the
// phase and work unchanged. Safe from any goroutine.
func (w *StartedWriteRequest) Cancel() {
	if w == nil || w.req == nil {
		return
	}
	w.mu.Lock()
	if w.noncancellable {
		w.mu.Unlock()
		return
	}
	w.req.Cancel()
	w.mu.Unlock()
}

// State reports the visible lifecycle state of the underlying request:
// Cancelling stays observable from Cancel until true settlement.
func (w *StartedWriteRequest) State() RequestState {
	if w.req == nil {
		return StateSettled
	}
	return w.req.State()
}

// Settled reports whether the write reached its definite outcome and
// released its lease.
func (w *StartedWriteRequest) Settled() bool {
	if w.req == nil {
		return true
	}
	return w.req.Settled()
}

// SettledChan exposes settlement as a receive-only channel so callers can
// wait without timing sleeps.
func (w *StartedWriteRequest) SettledChan() <-chan struct{} { return w.settled }

// Phases exposes the ordered phase stream, which is closed at settlement.
func (w *StartedWriteRequest) Phases() <-chan WritePhaseMsg { return w.phases }

// Wait blocks until the write truly settles, then returns its resolved
// result. It must be called at most once, from one goroutine.
func (w *StartedWriteRequest) Wait() WriteResult {
	<-w.settled
	return w.final
}
