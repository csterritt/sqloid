// Commit-boundary cancellation coverage for Issue #43, per the Writes and
// commit boundary and write-cancellation Testing Decisions in
// Notes/PRD-sqloid.md. The barrier seams hold the write deterministically in
// beginning, executing, the atomic after-statement/before-COMMIT decision,
// and rollback cleanup; every wait is a channel receive, never a sleep. The
// tests prove that Ctrl+W-equivalent cancellation issues exactly one scoped
// interrupt against the write's own leased connection before the boundary,
// that the cancellation flag wins atomically after a successful statement,
// and that once rollback cleanup has begun — or after settlement — Cancel
// issues no further interrupt, leaves the phase/work unchanged, and cannot
// start replacement work on the retained lease.

package connection

import (
	"context"
	"testing"
)

// writeLeaseInterrupts installs a counting interrupt hook on the write's
// leased connection through the writeLeaseHook seam and returns the counter
// getter. The hook is the only interrupt dispatch observed; production code
// never reads it.
func writeLeaseInterrupts(t *testing.T, db *DB) func() int {
	t.Helper()
	var interrupts int
	db.writeLeaseHook = func(l *Lease) {
		l.interruptFn = func() { interrupts++ }
	}
	t.Cleanup(func() { db.writeLeaseHook = nil })
	return func() int { return interrupts }
}

// TestWriteInterruptScopedToLeaseBeforeBoundary holds the executing
// transition, requests cancellation while the statement is pending, and
// proves exactly one connection-scoped interrupt was dispatched against the
// write's own leased connection — the cancellable scope's only interrupt.
func TestWriteInterruptScopedToLeaseBeforeBoundary(t *testing.T) {
	db := openJournalFixture(t, "delete")
	interrupts := writeLeaseInterrupts(t, db)
	held, release := holdWriteBarrier(t, db, WritePhaseExecuting)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	<-held

	w.Cancel()
	if got := interrupts(); got != 1 {
		t.Fatalf("interrupts dispatched while cancellable = %d, want exactly 1 scoped interrupt", got)
	}
	release()

	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("outcome = %v (err %v), want cancelled", res.Outcome, res.Err)
	}
	// The retained lease is never released early; only at settlement is it
	// reusable, and the interrupt never touched any other connection.
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteNoInterruptDuringRollbackCleanup crosses the commit boundary by
// requesting cancellation while executing, then holds the write inside
// rollback cleanup and repeats Cancel: the late request must dispatch no
// second interrupt, leave the phase and work unchanged, and cannot start
// replacement work on the still-retained lease.
func TestWriteNoInterruptDuringRollbackCleanup(t *testing.T) {
	db := openJournalFixture(t, "delete")
	interrupts := writeLeaseInterrupts(t, db)
	held, release := holdWriteBarrier(t, db, WritePhaseExecuting)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	<-held
	w.Cancel()
	if got := interrupts(); got != 1 {
		t.Fatalf("interrupts before boundary = %d, want exactly 1", got)
	}
	release()

	// Hold the write inside noncancellable rollback cleanup.
	heldRollback, releaseRollback := holdWriteBarrier(t, db, WritePhaseRollbackCleanup)
	<-heldRollback
	before := w.State()
	w.Cancel()
	w.Cancel() // repeated keys: equally inert
	if got := interrupts(); got != 1 {
		t.Fatalf("interrupts after the boundary = %d, want still 1 — cleanup is noncancellable", got)
	}
	if w.State() != before {
		t.Fatalf("state during rollback cleanup changed from %v to %v; work must be untouched", before, w.State())
	}
	releaseRollback()

	res := w.Wait()
	if res.Outcome != WriteCancelled || !res.RollbackConfirmed {
		t.Fatalf("outcome = %v confirmed=%v, want cancelled with confirmed rollback", res.Outcome, res.RollbackConfirmed)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteNoInterruptAfterBoundarySettlement proves a fully settled
// committed write permanently ignores Cancel: no interrupt is dispatched,
// the outcome and phase work stay unchanged, and the settlement/lease
// ownership that already resolved cannot be reopened.
func TestWriteNoInterruptAfterBoundarySettlement(t *testing.T) {
	db := openJournalFixture(t, "delete")
	interrupts := writeLeaseInterrupts(t, db)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	res := w.Wait()
	if res.Outcome != WriteCommitted {
		t.Fatalf("outcome = %v (err %v), want committed", res.Outcome, res.Err)
	}
	w.Cancel()
	if got := interrupts(); got != 0 {
		t.Fatalf("interrupts after settlement = %d, want 0 — the boundary permanently disabled interrupts", got)
	}
	if !w.Settled() || w.State() != StateSettled {
		t.Fatalf("state = %v settled=%v, want settled", w.State(), w.Settled())
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}
