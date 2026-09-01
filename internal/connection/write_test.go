// Transactional-write coverage for Issue #42, per the Writes and commit
// boundary, write-transaction, and write-cancellation decisions in
// Notes/PRD-sqloid.md. Real modernc.org/sqlite fixtures prove the whole
// one-request/one-lease lifecycle: exactly one path-identity pre-BEGIN
// check, BEGIN and exactly one statement on the dedicated lease, the atomic
// pre-COMMIT cancellation check winning after statement success, rollback
// cleanup with confirmed rollback for cancellation and statement failure,
// commit for uncancelled success, and native constraint/trigger behavior
// checked against persisted state. The DB's documented barrier seams hold
// every beginning/executing transition deterministically; every wait is a
// channel receive, never a sleep. Post-boundary Ctrl+W/quit ownership is
// Issue #43's and is deliberately absent here.

package connection

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

// writeExecSeq mints process-unique write execution identities for tests.
var writeExecSeq atomic.Uint64

// writeRows queries the fixture through an independent pooled connection so
// assertions observe persisted state rather than the write's lease.
func writeRows(t *testing.T, path, query string) int {
	t.Helper()
	ext, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open external reader: %v", err)
	}
	defer ext.Close()
	var n int
	if err := ext.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

// editFixture opens the fixture for schema preparation through an
// independent connection (triggers, extra tables) and closes it.
func editFixture(t *testing.T, path, ddl string) {
	t.Helper()
	ext, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture editor: %v", err)
	}
	defer ext.Close()
	if _, err := ext.Exec(ddl); err != nil {
		t.Fatalf("prepare fixture schema: %v", err)
	}
}

// collectPhases drains the write's phase stream after settlement.
func collectPhases(t *testing.T, w *StartedWriteRequest) []WritePhase {
	t.Helper()
	var phases []WritePhase
	for msg := range w.Phases() {
		phases = append(phases, msg.Phase)
	}
	return phases
}

// assertPhaseSequence requires the write to have produced exactly the named
// phases in order, proving no second statement or extra commit/rollback ran.
func assertPhaseSequence(t *testing.T, got []WritePhase, want ...WritePhase) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phases = %v, want %v", got, want)
		}
	}
}

// assertLeaseReusable starts one more short write after settlement and
// requires it to commit, proving the lease was released and the physical
// connection is healthy and reusable for later work.
func assertLeaseReusable(t *testing.T, db *DB, statement string) {
	t.Helper()
	next := db.StartWrite(context.Background(), writeExecSeq.Add(1), statement, nil)
	res := next.Wait()
	if res.Outcome != WriteCommitted {
		t.Fatalf("post-settlement write outcome = %v (err %v), want committed; lease was not released cleanly", res.Outcome, res.Err)
	}
}

// writeConnIdentity records the exact *sql.Conn observed by each write phase
// hook and the lease hook for one write execution, so barrier tests can prove
// every phase of one write runs on the same dedicated leased connection. The
// rollback hook is counted so tests can require exactly one rollback-hook
// call. Fields are written by the work goroutine before settlement and read
// by the test goroutine after Wait returns, synchronized by the settled
// channel.
type writeConnIdentity struct {
	lease    *sql.Conn
	begin    *sql.Conn
	exec     *sql.Conn
	commit   *sql.Conn
	rollback *sql.Conn

	rollbackCalls int
}

// assertSameConn requires every reached phase hook to have observed the same
// non-nil *sql.Conn as the lease hook. reachedCommit controls whether the
// commit hook is required to have fired. When rollback was reached, the
// rollback hook must have been called exactly once with a non-nil connection
// pointer-identical to the lease. The exec hook is required when either
// reachedCommit or rollback was reached, since both imply the transaction
// opened and the statement phase began.
func (id *writeConnIdentity) assertSameConn(t *testing.T, reachedCommit bool) {
	t.Helper()
	if id.lease == nil {
		t.Fatal("writeLeaseHook did not observe the leased connection")
	}
	if id.begin == nil {
		t.Fatal("beforeWriteBegin did not observe a connection")
	}
	if id.begin != id.lease {
		t.Fatalf("beforeWriteBegin conn %p != lease conn %p", id.begin, id.lease)
	}
	requireExec := reachedCommit || id.rollbackCalls > 0
	if requireExec {
		if id.exec == nil {
			t.Fatal("beforeWriteExec did not observe a connection")
		}
		if id.exec != id.lease {
			t.Fatalf("beforeWriteExec conn %p != lease conn %p", id.exec, id.lease)
		}
	}
	if reachedCommit {
		if id.commit == nil {
			t.Fatal("beforeWriteCommit did not observe a connection")
		}
		if id.commit != id.lease {
			t.Fatalf("beforeWriteCommit conn %p != lease conn %p", id.commit, id.lease)
		}
	}
	if id.rollbackCalls > 0 {
		if id.rollback == nil {
			t.Fatal("beforeWriteRollback observed nil connection; want non-nil leased connection")
		}
		if id.rollback != id.lease {
			t.Fatalf("beforeWriteRollback conn %p != lease conn %p", id.rollback, id.lease)
		}
		if id.rollbackCalls != 1 {
			t.Fatalf("rollback hook calls = %d, want exactly 1", id.rollbackCalls)
		}
	}
}

// writeHookRegistry composes multiple test hooks per write phase into single
// hook fields on DB, so a test can simultaneously hold a barrier at one
// phase, record connection identities at every phase, count interrupts on
// the lease, and run setup like PRAGMA foreign_keys on the begin hook. Each
// phase hook field is set exactly once by the registry and dispatches to
// every registered function in registration order.
type writeHookRegistry struct {
	db            *DB
	leaseHooks    []func(*Lease)
	beginHooks    []func(context.Context, *sql.Conn)
	execHooks     []func(context.Context, *sql.Conn)
	commitHooks   []func(context.Context, *sql.Conn)
	rollbackHooks []func(context.Context, *sql.Conn)
}

// newWriteHookRegistry creates a registry that owns all write hook fields on
// db for the duration of the test, dispatching to registered hooks in
// registration order. The hooks are cleared on test cleanup.
func newWriteHookRegistry(t *testing.T, db *DB) *writeHookRegistry {
	t.Helper()
	r := &writeHookRegistry{db: db}
	db.writeLeaseHook = func(l *Lease) {
		for _, h := range r.leaseHooks {
			h(l)
		}
	}
	db.beforeWriteBegin = func(ctx context.Context, conn *sql.Conn) {
		for _, h := range r.beginHooks {
			h(ctx, conn)
		}
	}
	db.beforeWriteExec = func(ctx context.Context, conn *sql.Conn) {
		for _, h := range r.execHooks {
			h(ctx, conn)
		}
	}
	db.beforeWriteCommit = func(ctx context.Context, conn *sql.Conn) {
		for _, h := range r.commitHooks {
			h(ctx, conn)
		}
	}
	db.beforeWriteRollback = func(ctx context.Context, conn *sql.Conn) {
		for _, h := range r.rollbackHooks {
			h(ctx, conn)
		}
	}
	t.Cleanup(func() {
		db.clearWriteHooks()
		db.writeLeaseHook = nil
	})
	return r
}

// recordIdentity installs hooks that record the *sql.Conn observed by each
// phase hook and the lease hook into the returned identity recorder.
func (r *writeHookRegistry) recordIdentity() *writeConnIdentity {
	id := &writeConnIdentity{}
	r.leaseHooks = append(r.leaseHooks, func(l *Lease) { id.lease = l.Conn() })
	r.beginHooks = append(r.beginHooks, func(_ context.Context, conn *sql.Conn) { id.begin = conn })
	r.execHooks = append(r.execHooks, func(_ context.Context, conn *sql.Conn) { id.exec = conn })
	r.commitHooks = append(r.commitHooks, func(_ context.Context, conn *sql.Conn) { id.commit = conn })
	r.rollbackHooks = append(r.rollbackHooks, func(_ context.Context, conn *sql.Conn) {
		id.rollback = conn
		id.rollbackCalls++
	})
	return id
}

// holdBarrier installs a barrier at the given phase that blocks the write
// until release, returning the channel announcing the phase's emission and
// the release function. The barrier is composed with other registered hooks
// for the same phase and never overwrites them.
func (r *writeHookRegistry) holdBarrier(t *testing.T, phase WritePhase) (held <-chan WritePhase, release func()) {
	t.Helper()
	seen := make(chan WritePhase, 1)
	releaseCh := make(chan struct{})
	hook := func(_ context.Context, _ *sql.Conn) {
		select {
		case seen <- phase:
		default:
		}
		<-releaseCh
	}
	switch phase {
	case WritePhaseBeginning:
		r.beginHooks = append(r.beginHooks, hook)
	case WritePhaseExecuting:
		r.execHooks = append(r.execHooks, hook)
	case WritePhaseCommitting:
		r.commitHooks = append(r.commitHooks, hook)
	case WritePhaseRollbackCleanup:
		r.rollbackHooks = append(r.rollbackHooks, hook)
	default:
		t.Fatalf("holdBarrier: unsupported phase %v", phase)
	}
	return seen, func() { close(releaseCh) }
}

// countInterrupts installs a lease hook that counts interrupt dispatches on
// the write's leased connection, returning the counter getter.
func (r *writeHookRegistry) countInterrupts() func() int {
	var interrupts int
	r.leaseHooks = append(r.leaseHooks, func(l *Lease) {
		l.interruptFn = func() { interrupts++ }
	})
	return func() int { return interrupts }
}

// onBegin installs a hook that runs before BEGIN on the write's leased
// connection, composed with other registered begin hooks.
func (r *writeHookRegistry) onBegin(hook func(context.Context, *sql.Conn)) {
	r.beginHooks = append(r.beginHooks, hook)
}

// clear removes all registry-installed hooks from db so later writes in the
// same test run unimpeded.
func (r *writeHookRegistry) clear() {
	r.db.clearWriteHooks()
	r.db.writeLeaseHook = nil
}

// clearWriteHooks removes every write barrier seam so later requests in the
// same test run unimpeded.
func (db *DB) clearWriteHooks() {
	db.beforeWriteBegin = nil
	db.beforeWriteExec = nil
	db.beforeWriteCommit = nil
	db.beforeWriteRollback = nil
}

// TestWriteCommitLifecycle covers confirmed qualified/unqualified UPDATE,
// DELETE, and runnable INSERT: one pre-BEGIN identity check, the exact
// beginning→executing→committing phase sequence with no second statement,
// actual statement RowsAffected, persisted state, and a healthy reusable
// lease after settlement.
func TestWriteCommitLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		params    []any
		wantRows  int64
		check     string
		wantState int
	}{
		{
			name:      "qualified update commits and persists",
			statement: `UPDATE "users" SET "email" = 'new' WHERE "id" = ?`,
			params:    []any{int64(2)},
			wantRows:  1,
			check:     "SELECT COUNT(*) FROM users WHERE email = 'new'",
			wantState: 1,
		},
		{
			name:      "unqualified update commits and persists",
			statement: `UPDATE "users" SET "email" = 'new'`,
			wantRows:  3,
			check:     "SELECT COUNT(*) FROM users WHERE email = 'new'",
			wantState: 3,
		},
		{
			name:      "qualified delete commits and persists",
			statement: `DELETE FROM "users" WHERE "id" = ?`,
			params:    []any{int64(2)},
			wantRows:  1,
			check:     "SELECT COUNT(*) FROM users",
			wantState: 2,
		},
		{
			name:      "unqualified delete commits and persists",
			statement: `DELETE FROM "users"`,
			wantRows:  3,
			check:     "SELECT COUNT(*) FROM users",
			wantState: 0,
		},
		{
			name:      "runnable insert commits and persists",
			statement: `INSERT INTO "users" ("id", "email") VALUES (?, ?)`,
			params:    []any{int64(9), "u9"},
			wantRows:  1,
			check:     "SELECT COUNT(*) FROM users WHERE email = 'u9'",
			wantState: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openJournalFixture(t, "delete")
			r := newWriteHookRegistry(t, db)
			id := r.recordIdentity()
			before := db.identityChecks.Load()

			w := db.StartWrite(context.Background(), writeExecSeq.Add(1), tt.statement, tt.params)
			res := w.Wait()

			if res.Outcome != WriteCommitted {
				t.Fatalf("outcome = %v (err %v), want committed", res.Outcome, res.Err)
			}
			if res.Err != nil {
				t.Fatalf("committed write carried err %v", res.Err)
			}
			if res.RowsAffected != tt.wantRows {
				t.Fatalf("RowsAffected = %d, want %d", res.RowsAffected, tt.wantRows)
			}
			if res.RollbackConfirmed {
				t.Fatal("committed write reported rollback confirmation")
			}
			if !w.Settled() || w.State() != StateSettled {
				t.Fatalf("write state = %v settled=%v, want settled", w.State(), w.Settled())
			}
			assertPhaseSequence(t, collectPhases(t, w), WritePhaseBeginning, WritePhaseExecuting, WritePhaseCommitting)
			id.assertSameConn(t, true)

			if delta := db.identityChecks.Load() - before; delta != 1 {
				t.Fatalf("identity checks during write = %d, want exactly 1 (one pre-BEGIN boundary check)", delta)
			}
			if got := writeRows(t, db.path, tt.check); got != tt.wantState {
				t.Fatalf("persisted rows for %q = %d, want %d", tt.check, got, tt.wantState)
			}
			assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
		})
	}
}

// TestWriteTriggerSideEffectsCommit proves trigger side effects participate
// in the same transaction and persist with it, that RowsAffected remains the
// sole statement's count (trigger effects are never counted), and that a
// per-firing trigger proves the statement executed exactly once.
func TestWriteTriggerSideEffectsCommit(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	editFixture(t, db.path, `CREATE TABLE audit (id INTEGER PRIMARY KEY, why TEXT);
CREATE TRIGGER users_audit AFTER UPDATE ON users BEGIN INSERT INTO audit (why) VALUES ('updated'); END;
CREATE TABLE fires (n INTEGER);
INSERT INTO fires VALUES (0);
CREATE TRIGGER users_count AFTER UPDATE ON users BEGIN UPDATE fires SET n = n + 1; END;`)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	res := w.Wait()
	if res.Outcome != WriteCommitted {
		t.Fatalf("outcome = %v (err %v), want committed", res.Outcome, res.Err)
	}
	if res.RowsAffected != 3 {
		t.Fatalf("RowsAffected = %d, want the statement's own 3 rows (trigger effects excluded)", res.RowsAffected)
	}
	id.assertSameConn(t, true)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM audit"); got != 3 {
		t.Fatalf("persisted trigger side effects = %d, want 3 committed with the same transaction", got)
	}
	if got := writeRows(t, db.path, "SELECT n FROM fires"); got != 3 {
		t.Fatalf("statement firings observed by trigger = %d, want exactly 3 (one per row, no second execution)", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteConstraintFailureRollsBack proves a native constraint failure
// enters rollback cleanup, waits for confirmed rollback, resolves as failed,
// and leaves the database untouched.
func TestWriteConstraintFailureRollsBack(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `INSERT INTO "users" ("id", "email") VALUES (?, ?)`, []any{int64(1), "dupe"})
	res := w.Wait()

	if res.Outcome != WriteFailed {
		t.Fatalf("outcome = %v (err %v), want failed", res.Outcome, res.Err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "UNIQUE") {
		t.Fatalf("err = %v, want the preserved native UNIQUE constraint cause", res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("constraint failure did not confirm rollback; no untouched guarantee may be made")
	}
	assertPhaseSequence(t, collectPhases(t, w), WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, false)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users"); got != 3 {
		t.Fatalf("persisted rows after failed write = %d, want 3 (rollback confirmed)", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteTriggerFailureRollsBack proves a trigger RAISE (ABORT) failure
// follows the same rollback-cleanup path with the native cause preserved.
func TestWriteTriggerFailureRollsBack(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	editFixture(t, db.path, `CREATE TRIGGER users_guard BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'guard rejected update'); END;`)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	res := w.Wait()

	if res.Outcome != WriteFailed {
		t.Fatalf("outcome = %v (err %v), want failed", res.Outcome, res.Err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "guard rejected update") {
		t.Fatalf("err = %v, want the preserved trigger RAISE cause", res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("trigger failure did not confirm rollback")
	}
	assertPhaseSequence(t, collectPhases(t, w), WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, false)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'new'"); got != 0 {
		t.Fatalf("persisted updated rows = %d, want 0", got)
	}
	assertLeaseReusable(t, db, `CREATE TABLE IF NOT EXISTS lease_probe (x INTEGER)`)
}

// writeCancelledPhases returns the phase sequence allowed for a write
// cancelled before BEGIN: either the transaction never opened (beginning
// only) or the pinned driver opened it and the statement failed on the
// already-cancelled context, requiring confirmed rollback cleanup.
func writeCancelledPhases(res WriteResult) []WritePhase {
	if res.RollbackConfirmed {
		return []WritePhase{WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup}
	}
	return []WritePhase{WritePhaseBeginning}
}

// TestWriteCancellationBeforeBegin holds the beginning transition behind its
// barrier, requests cancellation before BEGIN runs, and proves the write
// settles cancelled with no persisted change and no rollback confirmation to
// claim (nothing was begun), all without sleeps.
func TestWriteCancellationBeforeBegin(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	held, release := r.holdBarrier(t, WritePhaseBeginning)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	<-held // deterministically at the beginning transition
	w.Cancel()
	release()

	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("outcome = %v (err %v), want cancelled", res.Outcome, res.Err)
	}
	// The pinned driver may fail BEGIN on the already-cancelled context or
	// open the transaction and let the statement fail on it; both shapes are
	// accepted, but confirmed rollback must then be reflected in the phase
	// stream, and no shape may persist any change.
	assertPhaseSequence(t, collectPhases(t, w), writeCancelledPhases(res)...)
	id.assertSameConn(t, false)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'new'"); got != 0 {
		t.Fatalf("persisted updated rows = %d, want 0", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteCancellationDuringStatement holds the executing transition,
// requests cancellation while the statement is pending, and proves the write
// rolls back with confirmation and settles cancelled.
func TestWriteCancellationDuringStatement(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	held, release := r.holdBarrier(t, WritePhaseExecuting)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	<-held
	w.Cancel()
	release()

	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("outcome = %v (err %v), want cancelled", res.Outcome, res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("cancelled write did not wait for and confirm rollback")
	}
	assertPhaseSequence(t, collectPhases(t, w), WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, false)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'new'"); got != 0 {
		t.Fatalf("persisted updated rows = %d, want 0 (rollback confirmed)", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWriteCancellationWinsAfterStatementSuccess is the release-blocking
// pre-COMMIT rule: the statement succeeds, the after-statement/before-COMMIT
// decision is held behind its barrier while still cancellable, cancellation
// is requested there, and the atomic pre-COMMIT cancellation check must
// still win — the write settles cancelled with confirmed rollback and the
// database is untouched. The committing phase is announced only after the
// boundary is crossed, so a pre-COMMIT-cancelled write never emits it
// (Issue #43 redefined the boundary; Issue #42's result contract is kept).
func TestWriteCancellationWinsAfterStatementSuccess(t *testing.T) {
	db := openJournalFixture(t, "delete")
	r := newWriteHookRegistry(t, db)
	id := r.recordIdentity()
	interrupts := r.countInterrupts()
	held, release := r.holdBarrier(t, WritePhaseCommitting)

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	<-held // statement already succeeded; the atomic check has not run yet
	w.Cancel()
	if got := interrupts(); got != 1 {
		t.Fatalf("interrupts before the boundary = %d, want exactly 1 scoped interrupt", got)
	}
	release()

	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("outcome = %v (err %v), want cancelled: the pre-COMMIT cancellation check must win after statement success", res.Outcome, res.Err)
	}
	if !res.RollbackConfirmed {
		t.Fatal("pre-COMMIT cancellation did not confirm rollback")
	}
	assertPhaseSequence(t, collectPhases(t, w), WritePhaseBeginning, WritePhaseExecuting, WritePhaseRollbackCleanup)
	id.assertSameConn(t, true)
	if got := writeRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'new'"); got != 0 {
		t.Fatalf("persisted updated rows = %d, want 0 (rollback confirmed after pre-COMMIT cancellation)", got)
	}
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}
