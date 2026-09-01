//go:build walkthrough

// Walkthrough proof tests for Issue #62. Print a deterministic identity
// label for the *sql.Conn observed by each reached write phase hook and the
// lease hook so the Notes/walkthroughs/062-04/code-walkthrough walkthrough
// can display them reproducibly. Each hook prints whether its connection is
// non-nil and whether it matches the lease connection (the first hook to
// fire records the reference identity). Isolated from the normal test suite
// by the `walkthrough` build tag; run with
// `go test -tags walkthrough ./internal/connection/`.

package connection

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

// walkthroughIdentity tracks the reference lease connection and prints
// deterministic identity labels for each reached hook.
type walkthroughIdentity struct {
	mu       sync.Mutex
	leaseRef *sql.Conn
}

func (wi *walkthroughIdentity) label(conn *sql.Conn) string {
	wi.mu.Lock()
	defer wi.mu.Unlock()
	if conn == nil {
		return "nil"
	}
	if wi.leaseRef == nil {
		wi.leaseRef = conn
		return "non-nil (lease reference)"
	}
	if conn == wi.leaseRef {
		return "non-nil (same as lease)"
	}
	return "non-nil (DIFFERENT from lease)"
}

// printHookIdentity installs a registry that prints the deterministic
// identity label observed by each reached phase hook and the lease hook.
func printHookIdentity(t *testing.T, db *DB, label string) *writeHookRegistry {
	t.Helper()
	wi := &walkthroughIdentity{}
	r := newWriteHookRegistry(t, db)
	r.leaseHooks = append(r.leaseHooks, func(l *Lease) {
		fmt.Printf("[%s] writeLeaseHook conn=%s\n", label, wi.label(l.Conn()))
	})
	r.beginHooks = append(r.beginHooks, func(_ context.Context, conn *sql.Conn) {
		fmt.Printf("[%s] beforeWriteBegin conn=%s\n", label, wi.label(conn))
	})
	r.execHooks = append(r.execHooks, func(_ context.Context, conn *sql.Conn) {
		fmt.Printf("[%s] beforeWriteExec  conn=%s\n", label, wi.label(conn))
	})
	r.commitHooks = append(r.commitHooks, func(_ context.Context, conn *sql.Conn) {
		fmt.Printf("[%s] beforeWriteCommit conn=%s\n", label, wi.label(conn))
	})
	r.rollbackHooks = append(r.rollbackHooks, func(_ context.Context, conn *sql.Conn) {
		fmt.Printf("[%s] beforeWriteRollback conn=%s\n", label, wi.label(conn))
	})
	return r
}

// TestWalkthrough062StatementFailure prints hook identities for a UNIQUE
// constraint failure (confirmed rollback).
func TestWalkthrough062StatementFailure(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := printHookIdentity(t, db, "stmt-fail")
	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`INSERT INTO "users" ("id", "email") VALUES (?, ?)`,
		[]any{int64(1), "dupe"})
	res := w.Wait()
	fmt.Printf("[stmt-fail] outcome=%v rollbackConfirmed=%v phases=%v\n",
		res.Outcome, res.RollbackConfirmed, collectPhases(t, w))
	r.clear()
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWalkthrough062PreCommitCancellation prints hook identities for a
// pre-COMMIT cancellation held at the executing barrier (confirmed rollback).
func TestWalkthrough062PreCommitCancellation(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := printHookIdentity(t, db, "pre-commit-cancel")
	held, release := r.holdBarrier(t, WritePhaseExecuting)
	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`UPDATE "users" SET "email" = 'new'`, nil)
	<-held
	w.Cancel()
	release()
	res := w.Wait()
	fmt.Printf("[pre-commit-cancel] outcome=%v rollbackConfirmed=%v phases=%v\n",
		res.Outcome, res.RollbackConfirmed, collectPhases(t, w))
	r.clear()
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWalkthrough062ConfirmedRollback prints hook identities for a
// successful committed write (begin/exec/commit reached, no rollback).
func TestWalkthrough062ConfirmedRollback(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := printHookIdentity(t, db, "committed")
	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`UPDATE "users" SET "email" = 'new'`, nil)
	res := w.Wait()
	fmt.Printf("[committed] outcome=%v rollbackConfirmed=%v phases=%v\n",
		res.Outcome, res.RollbackConfirmed, collectPhases(t, w))
	r.clear()
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWalkthrough062UnresolvedRollback prints hook identities for a COMMIT
// failure (unresolved rollback, RollbackConfirmed false).
func TestWalkthrough062UnresolvedRollback(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := printHookIdentity(t, db, "commit-fail")
	editFixture(t, db.path, `CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE child (
    id INTEGER PRIMARY KEY,
    parent_id INTEGER NOT NULL REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED
);`)
	enableForeignKeysOnWriteLease(t, r)
	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`INSERT INTO "child" ("id", "parent_id") VALUES (?, ?)`,
		[]any{int64(1), int64(999)})
	res := w.Wait()
	fmt.Printf("[commit-fail] outcome=%v rollbackConfirmed=%v phase=%v phases=%v\n",
		res.Outcome, res.RollbackConfirmed, res.Phase, collectPhases(t, w))
	r.clear()
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}
