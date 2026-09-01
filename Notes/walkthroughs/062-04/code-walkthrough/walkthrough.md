# Issue #062 Code Walkthrough: Pass the Leased Connection to the Rollback Test Hook

*2026-09-01T14:11:36Z by Showboat 0.6.1*
<!-- showboat-id: caab2b25-d1ba-47e9-ab79-6649d38a6082 -->

Issue #62 (Notes/tasks/062-pass-leased-connection-to-rollback-hook.md, Notes/PRD-sqloid.md §Writes and commit boundary, §Module Design §Connection, §Testing Decisions; user stories 45 and 82) passes the write's dedicated leased `*sql.Conn` into the `beforeWriteRollback` test hook so every reached phase hook — `beforeWriteBegin`, `beforeWriteExec`, `beforeWriteCommit`, `beforeWriteRollback` — and the `writeLeaseHook` seam observe the same non-nil pointer-identical connection for one write execution. Before this issue, `rollback()` invoked `beforeWriteRollback(ctx, nil)`, so the rollback barrier could not prove it ran on the same connection as the begin/exec/commit hooks. The fix threads the leased connection (already held by `run()` for BEGIN, the statement, and the pre-COMMIT check) through `rollback()` and into the hook. The hook remains nil in production and the transaction sequence, error precedence, cancellation classification, settlement, and lease release are unchanged; the connection argument is observability-only. The test suite replaces the single-hook `holdWriteBarrier`/`writeLeaseInterrupts` helpers with a composable `writeHookRegistry` that dispatches to multiple registered hooks per phase in registration order, plus a `writeConnIdentity` recorder that captures the `*sql.Conn` seen by each reached phase hook and the lease hook. `assertSameConn` requires pointer-identical non-nil identity across all reached hooks with exactly one rollback-hook call. This walkthrough runs the synchronized write cases for statement failure, pre-COMMIT cancellation, confirmed rollback, and unresolved (COMMIT-failure) rollback, displaying the non-nil connection identity seen by each reached begin/execute/commit/rollback hook and proving all identities match the owning lease. It captures unchanged phase order, classification, settlement-before-release, and successful later reuse.

```bash
sed -n '255,320p' internal/connection/write.go
```

```output
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
		return w.rollback(ctx, conn, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseExecuting})
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
		return w.rollback(ctx, conn, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseExecuting})
	}
	w.emit(WritePhaseCommitting)
	if err := tx.Commit(); err != nil {
		// Persistence after a failed commit is unprovable; Issue #45 owns
		// the outcome-unknown terminal workflow. Issue #61: preserve the
		// committing phase and commit error, perform best-effort
		// noncancellable rollback cleanup, but never treat sql.ErrTxDone
		// from the subsequent rollback attempt as confirmation that
		// persistence did not occur — the transaction is no longer active
		// but may or may not have been committed at the SQLite level.
		return w.rollback(ctx, conn, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseCommitting})
	}
	return WriteResult{Outcome: WriteCommitted, RowsAffected: rowsAffected}
}

```

`run()` already holds the leased `*sql.Conn` (acquired by `StartWrite` and passed in as `conn`) for BEGIN, the sole statement, and the pre-COMMIT check. The Issue #62 change threads that same `conn` into every `rollback()` call — three call sites: statement failure, pre-COMMIT cancellation, and COMMIT failure — so the rollback barrier observes the same connection as the other phase hooks. The transaction sequence, error precedence, and `Phase`/`RollbackConfirmed` semantics are unchanged.

```bash
sed -n '320,365p' internal/connection/write.go
```

```output

// rollback performs the noncancellable rollback cleanup phase and marks the
// result with confirmed rollback exactly when the rollback succeeded. It
// re-establishes the noncancellable boundary (the statement-failure path
// enters rollback cleanup from executing without passing the pre-COMMIT
// check), so no later Cancel can interrupt cleanup. The outcome is
// provisional: Settle classifies a cancellation cause as WriteCancelled and
// everything else as WriteFailed. conn is the write's dedicated leased
// connection, passed to beforeWriteRollback so test barriers observe the same
// identity as every other phase hook; it is nil in production.
//
// Issue #61: when res.Phase is WritePhaseCommitting the rollback is
// best-effort cleanup after a failed COMMIT. sql.ErrTxDone from the
// post-commit rollback attempt just means the transaction is no longer
// active — it does not prove whether the commit persisted — so
// RollbackConfirmed stays false and the original commit error is never
// overwritten with a less informative cleanup result. For pre-COMMIT paths
// (statement failure or cancellation), a nil or sql.ErrTxDone rollback
// return confirms the open transaction was ended without persisting.
func (w *StartedWriteRequest) rollback(ctx context.Context, conn *sql.Conn, tx *sql.Tx, res WriteResult) WriteResult {
	w.mu.Lock()
	w.noncancellable = true
	w.mu.Unlock()
	w.emit(WritePhaseRollbackCleanup)
	if w.owner.beforeWriteRollback != nil {
		w.owner.beforeWriteRollback(ctx, conn)
	}
	if res.Phase == WritePhaseCommitting {
		// COMMIT failed: persistence is unprovable. Perform best-effort
		// cleanup but never confirm rollback and never overwrite the commit
		// error, so the UI routes through the outcome-unknown terminal
		// workflow.
		_ = tx.Rollback()
		return res
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
```

The `rollback()` signature gains a `conn *sql.Conn` parameter and the hook call changes from `beforeWriteRollback(ctx, nil)` to `beforeWriteRollback(ctx, conn)`. The doc comment records that `conn` is the write's dedicated leased connection, passed for test-barrier identity observation, and nil in production (the hook field itself is nil in production). The COMMIT-failure branch (`res.Phase == WritePhaseCommitting`) and the pre-COMMIT rollback-confirmation branch are unchanged — only the hook argument differs.

```bash
sed -n '95,210p' internal/connection/write_test.go
```

```output
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
```

The `writeHookRegistry` replaces the old single-hook `holdWriteBarrier`/`writeLeaseInterrupts` helpers. Each phase hook field on `DB` is set exactly once by the registry and dispatches to every registered function in registration order, so a barrier, identity recorder, and interrupt counter can coexist on one phase without overwriting each other. `recordIdentity()` installs hooks that capture the `*sql.Conn` observed by each reached phase hook and the lease hook into a `writeConnIdentity`. `assertSameConn` then requires pointer-identical non-nil identity across all reached hooks, with exactly one rollback-hook call when rollback was reached. The `reachedCommit` flag controls whether the commit hook is required (a pre-COMMIT cancellation or statement failure never reaches commit).

To display the connection identity seen by each hook, the walkthrough uses proof tests in `internal/connection/walkthrough_062_proof_test.go`, isolated from the shipped suite by the `walkthrough` build tag. Each hook prints a deterministic label — `non-nil (lease reference)` for the first hook to fire, `non-nil (same as lease)` for subsequent hooks observing the same connection, or `non-nil (DIFFERENT from lease)` if a hook observed a different connection. Run them with `go test -tags walkthrough ./internal/connection/`. The shipped `assertSameConn` assertions enforce the same contract permanently without printing.

## Case 1: Statement failure → confirmed rollback

A UNIQUE constraint failure enters rollback cleanup from the executing phase. The phase sequence is `beginning → executing → rollback-cleanup`, `RollbackConfirmed` is true, and the rollback hook must observe the same non-nil leased connection as the begin/exec hooks and the lease hook, with exactly one rollback-hook call.

```bash
go test -tags walkthrough ./internal/connection/ -run '^TestWalkthrough062StatementFailure$' -count=1 -v 2>&1
```

```output
=== RUN   TestWalkthrough062StatementFailure
[stmt-fail] writeLeaseHook conn=non-nil (lease reference)
[stmt-fail] beforeWriteBegin conn=non-nil (same as lease)
[stmt-fail] beforeWriteExec  conn=non-nil (same as lease)
[stmt-fail] beforeWriteRollback conn=non-nil (same as lease)
[stmt-fail] outcome=failed rollbackConfirmed=true phases=[beginning executing rollback-cleanup]
--- PASS: TestWalkthrough062StatementFailure (0.02s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.024s
```

All four reached hooks — `writeLeaseHook`, `beforeWriteBegin`, `beforeWriteExec`, and `beforeWriteRollback` — observe a non-nil connection, and every hook after the lease hook reports `same as lease`. The phase sequence is `beginning → executing → rollback-cleanup`, `RollbackConfirmed` is true, and the lease is reusable after settlement.

## Case 2: Pre-COMMIT cancellation → confirmed rollback

A cancellation requested while the statement is pending (held at the executing barrier) enters rollback cleanup from the executing phase. The phase sequence is `beginning → executing → rollback-cleanup`, `RollbackConfirmed` is true, and the rollback hook must observe the same non-nil leased connection as the begin/exec hooks and the lease hook.

```bash
go test -tags walkthrough ./internal/connection/ -run '^TestWalkthrough062PreCommitCancellation$' -count=1 -v 2>&1
```

```output
=== RUN   TestWalkthrough062PreCommitCancellation
[pre-commit-cancel] writeLeaseHook conn=non-nil (lease reference)
[pre-commit-cancel] beforeWriteBegin conn=non-nil (same as lease)
[pre-commit-cancel] beforeWriteExec  conn=non-nil (same as lease)
[pre-commit-cancel] beforeWriteRollback conn=non-nil (same as lease)
[pre-commit-cancel] outcome=cancelled rollbackConfirmed=true phases=[beginning executing rollback-cleanup]
--- PASS: TestWalkthrough062PreCommitCancellation (0.02s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.017s
```

All four reached hooks observe a non-nil connection, every hook after the lease reports `same as lease`. The outcome is `cancelled` with `RollbackConfirmed = true` and the `beginning → executing → rollback-cleanup` phase sequence.

## Case 3: Confirmed commit (no rollback reached)

A successful committed write reaches begin, exec, and commit but never rollback. The phase sequence is `beginning → executing → committing`, `RollbackConfirmed` is false, and the three reached hooks must observe the same non-nil leased connection as the lease hook.

```bash
go test -tags walkthrough ./internal/connection/ -run '^TestWalkthrough062ConfirmedRollback$' -count=1 -v 2>&1
```

```output
=== RUN   TestWalkthrough062ConfirmedRollback
[committed] writeLeaseHook conn=non-nil (lease reference)
[committed] beforeWriteBegin conn=non-nil (same as lease)
[committed] beforeWriteExec  conn=non-nil (same as lease)
[committed] beforeWriteCommit conn=non-nil (same as lease)
[committed] outcome=committed rollbackConfirmed=false phases=[beginning executing committing]
--- PASS: TestWalkthrough062ConfirmedRollback (0.02s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.019s
```

The lease hook, begin, exec, and commit hooks all observe a non-nil connection, every hook after the lease reports `same as lease`. The rollback hook is never reached (no rollback-cleanup phase). The outcome is `committed` with `RollbackConfirmed = false`.

## Case 4: Unresolved rollback (COMMIT failure)

A deferred foreign-key constraint induces a real `tx.Commit()` failure after the INSERT statement succeeds. The phase sequence is `beginning → executing → committing → rollback-cleanup`, `RollbackConfirmed` is false (the post-commit `sql.ErrTxDone` is not rollback confirmation), `Phase` is `WritePhaseCommitting`, and all four reached hooks — including the rollback hook — must observe the same non-nil leased connection.

```bash
go test -tags walkthrough ./internal/connection/ -run '^TestWalkthrough062UnresolvedRollback$' -count=1 -v 2>&1
```

```output
=== RUN   TestWalkthrough062UnresolvedRollback
[commit-fail] writeLeaseHook conn=non-nil (lease reference)
[commit-fail] beforeWriteBegin conn=non-nil (same as lease)
[commit-fail] beforeWriteExec  conn=non-nil (same as lease)
[commit-fail] beforeWriteCommit conn=non-nil (same as lease)
[commit-fail] beforeWriteRollback conn=non-nil (same as lease)
[commit-fail] outcome=failed rollbackConfirmed=false phase=committing phases=[beginning executing committing rollback-cleanup]
--- PASS: TestWalkthrough062UnresolvedRollback (0.02s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.022s
```

All five reached hooks — lease, begin, exec, commit, and rollback — observe a non-nil connection, every hook after the lease reports `same as lease`. The rollback hook receives the same non-nil leased connection even on the COMMIT-failure cleanup path. `RollbackConfirmed` is false, `Phase` is `committing`, and the phase sequence is `beginning → executing → committing → rollback-cleanup`. The lease is reusable after settlement.

## Unchanged behavior: full shipped suite with the race detector

The shipped barrier suite (proof tests excluded by the `walkthrough` build tag) proves unchanged phase order, classification, settlement-before-release, and successful later reuse under the race detector.

```bash
CGO_ENABLED=1 go test -race ./internal/connection/ -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/connection	64.431s
```

The full shipped `internal/connection` suite passes under the race detector with no data races. The shipped `writeConnIdentity`/`assertSameConn` assertions in `write_test.go`, `commit_boundary_test.go`, and `commit_failure_test.go` enforce the pointer-identical connection identity contract permanently.

## Summary

Issue #62 threads the write's dedicated leased `*sql.Conn` from `run()` into `rollback()` so `beforeWriteRollback` observes the same connection as the begin/exec/commit hooks and the `writeLeaseHook` seam. The change is observability-only: the hook is nil in production and the transaction sequence, error precedence, cancellation classification, settlement, and lease release are unchanged. The four synchronized write cases — statement failure, pre-COMMIT cancellation, confirmed commit, and unresolved (COMMIT-failure) rollback — all prove every reached hook observes the same non-nil leased connection, with exactly one rollback-hook call when rollback was reached. Unchanged phase order, classification, settlement-before-release, and successful later reuse hold under the race detector.

Cross-references: Issue #62 (Notes/tasks/062-pass-leased-connection-to-rollback-hook.md), Notes/PRD-sqloid.md §Writes and commit boundary, §Module Design §Connection, §Testing Decisions; user stories 45 and 82.
