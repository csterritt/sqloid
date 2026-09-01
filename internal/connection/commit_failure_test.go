// Real driver-boundary COMMIT-failure coverage for Issue #61, per the Writes
// and commit boundary decisions in Notes/PRD-sqloid.md. A deferred
// foreign-key constraint induces an actual tx.Commit() failure after the
// write statement has executed: the result must preserve the committing
// phase and commit error, leave RollbackConfirmed false even when the
// subsequent rollback returns sql.ErrTxDone, and never classify persistence
// as definitely failed or untouched. A control case proves that a pre-COMMIT
// statement failure followed by a genuinely successful rollback remains
// confirmed untouched.

package connection

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// enableForeignKeysOnWriteLease registers a beforeWriteBegin hook that
// enables PRAGMA foreign_keys on the write's dedicated leased connection
// before BEGIN runs, so deferred FK constraints are checked at COMMIT time.
func enableForeignKeysOnWriteLease(t *testing.T, r *writeHookRegistry) {
	t.Helper()
	r.onBegin(func(ctx context.Context, conn *sql.Conn) {
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			t.Fatalf("enable foreign_keys on write lease: %v", err)
		}
	})
}

// TestWriteCommitFailurePreservesOutcomeUnknown induces an actual driver
// COMMIT failure via a deferred foreign-key constraint checked at COMMIT
// time: the INSERT statement succeeds (RowsAffected = 1), but COMMIT fails.
// The result must preserve the committing phase and commit error, leave
// RollbackConfirmed false (sql.ErrTxDone from the post-commit rollback
// attempt does not prove persistence did not occur), and the phase sequence
// must show beginning → executing → committing → rollback-cleanup. The lease
// must remain reusable after settlement.
func TestWriteCommitFailurePreservesOutcomeUnknown(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	editFixture(t, db.path, `CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE child (
    id INTEGER PRIMARY KEY,
    parent_id INTEGER NOT NULL REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED
);`)
	enableForeignKeysOnWriteLease(t, r)

	// Insert a child row referencing a non-existent parent. The INSERT
	// statement succeeds (RowsAffected = 1), but COMMIT fails because the
	// deferred FK constraint is violated.
	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`INSERT INTO "child" ("id", "parent_id") VALUES (?, ?)`,
		[]any{int64(1), int64(999)})
	res := w.Wait()

	if res.Outcome != WriteFailed {
		t.Fatalf("outcome = %v (err %v), want failed", res.Outcome, res.Err)
	}
	if res.Err == nil {
		t.Fatal("commit error was lost")
	}
	if !strings.Contains(strings.ToLower(res.Err.Error()), "foreign key") {
		t.Fatalf("err = %v, want the preserved commit FK constraint cause", res.Err)
	}
	if res.RollbackConfirmed {
		t.Fatal("RollbackConfirmed = true after COMMIT failure; sql.ErrTxDone must not confirm rollback")
	}
	if res.Phase != WritePhaseCommitting {
		t.Fatalf("phase = %v, want WritePhaseCommitting", res.Phase)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("RowsAffected = %d, want 1 (the statement succeeded before COMMIT failed)", res.RowsAffected)
	}
	assertPhaseSequence(t, collectPhases(t, w),
		WritePhaseBeginning, WritePhaseExecuting, WritePhaseCommitting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, true)

	// The lease is still reusable after settlement: the physical connection
	// was not force-closed and remains healthy for later work.
	r.clear()
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWritePreCommitFailureConfirmsRollback is the control case: a pre-COMMIT
// statement failure (UNIQUE constraint) followed by a genuinely successful
// rollback remains confirmed untouched, proving the outcome-unknown routing
// is exclusive to the COMMIT-failure path. The phase is WritePhaseExecuting,
// not WritePhaseCommitting.
func TestWritePreCommitFailureConfirmsRollback(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`INSERT INTO "users" ("id", "email") VALUES (?, ?)`,
		[]any{int64(1), "dupe"})
	res := w.Wait()

	if res.Outcome != WriteFailed {
		t.Fatalf("outcome = %v (err %v), want failed", res.Outcome, res.Err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "UNIQUE") {
		t.Fatalf("err = %v, want the preserved native UNIQUE constraint cause", res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("pre-COMMIT statement failure with genuine rollback did not confirm rollback")
	}
	if res.Phase != WritePhaseExecuting {
		t.Fatalf("phase = %v, want WritePhaseExecuting", res.Phase)
	}
	assertPhaseSequence(t, collectPhases(t, w),
		WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, false)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users"); got != 3 {
		t.Fatalf("persisted rows after failed write = %d, want 3 (rollback confirmed)", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWritePreCommitCancellationConfirmsRollback is the cancellation control
// case: a cancellation during the executing phase followed by a genuinely
// successful rollback remains confirmed untouched with the executing phase
// preserved, not the committing phase.
func TestWritePreCommitCancellationConfirmsRollback(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	held, release := r.holdBarrier(t, WritePhaseExecuting)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`UPDATE "users" SET "email" = 'new'`, nil)
	<-held
	w.Cancel()
	release()

	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("outcome = %v (err %v), want cancelled", res.Outcome, res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("pre-COMMIT cancellation with genuine rollback did not confirm rollback")
	}
	if res.Phase != WritePhaseExecuting {
		t.Fatalf("phase = %v, want WritePhaseExecuting", res.Phase)
	}
	assertPhaseSequence(t, collectPhases(t, w),
		WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, false)
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}
