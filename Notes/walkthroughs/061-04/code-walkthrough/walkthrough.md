# Issue #061 Code Walkthrough: Classify COMMIT Failure as Outcome Unknown

*2026-09-01T01:33:33Z by Showboat 0.6.1*
<!-- showboat-id: 9a9a62fc-34ee-4c73-a400-ba04a9ce9e52 -->

Issue #61 (Notes/tasks/061-commit-failure-outcome-unknown.md, Notes/PRD-sqloid.md §Writes and commit boundary, §terminal-state, §History; user stories 45, 47, 85) corrects the destructive-write commit boundary so a failed `tx.Commit()` is classified as outcome unknown rather than a definite failed write with confirmed rollback. Before this issue, `rollback()` treated `sql.ErrTxDone` from the post-commit rollback attempt as rollback confirmation (`RollbackConfirmed = true`), which led the UI to finalize the write as a definite `WriteFailed` with `rollback confirmed, database untouched` wording. But `sql.ErrTxDone` after `Commit()` just means the transaction is no longer active in `database/sql` — the COMMIT may have persisted at the SQLite level even though the driver returned an error (e.g., an I/O error after the commit was applied). Issue #61 adds a `Phase WritePhase` field to `WriteResult` preserving the failure-boundary phase, changes `rollback()` to skip rollback confirmation when the phase is `WritePhaseCommitting`, and updates `finalizeOutcomeUnknown` to prefer the result's preserved phase. A pre-COMMIT statement failure or cancellation followed by a genuinely successful rollback keeps `RollbackConfirmed = true` and the confirmed-untouched label unchanged.

```bash
sed -n '109,145p' internal/connection/write.go
```

```output
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

	// Phase preserves the write phase at the failure boundary so callers
	// can distinguish a failed COMMIT (WritePhaseCommitting) from a
	// statement or cancellation rollback path (WritePhaseExecuting). It is
	// zero when the write committed or never crossed into a noncancellable
	// phase. The UI uses this to select the outcome-unknown entry's
	// commit-versus-rollback phase.
	Phase WritePhase
}

// StartedWriteRequest is one running transactional write on its dedicated
// leased connection whose cancellation is requested from outside the work
// goroutine. The lifecycle is exactly: StartWrite → Cancel at most
```

The `Phase` field is the key addition. It is zero for committed writes or writes that never crossed into a noncancellable phase. For a failed COMMIT it carries `WritePhaseCommitting`; for a statement failure or cancellation it carries `WritePhaseExecuting`. The UI's `finalizeOutcomeUnknown` uses this to select `UnknownPhaseCommit` versus `UnknownPhaseRollback`.

```bash
sed -n '270,320p' internal/connection/write.go
```

```output
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
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseExecuting})
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
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseExecuting})
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
		return w.rollback(ctx, tx, WriteResult{Err: err, RowsAffected: rowsAffected, Phase: WritePhaseCommitting})
	}
	return WriteResult{Outcome: WriteCommitted, RowsAffected: rowsAffected}
}

```

Three paths enter `rollback()`: (1) statement failure sets `Phase: WritePhaseExecuting`, (2) pre-COMMIT cancellation sets `Phase: WritePhaseExecuting`, and (3) COMMIT failure sets `Phase: WritePhaseCommitting`. The phase is what `rollback()` uses to decide whether `sql.ErrTxDone` confirms rollback.

```bash
sed -n '328,375p' internal/connection/write.go
```

```output
//
// Issue #61: when res.Phase is WritePhaseCommitting the rollback is
// best-effort cleanup after a failed COMMIT. sql.ErrTxDone from the
// post-commit rollback attempt just means the transaction is no longer
// active — it does not prove whether the commit persisted — so
// RollbackConfirmed stays false and the original commit error is never
// overwritten with a less informative cleanup result. For pre-COMMIT paths
// (statement failure or cancellation), a nil or sql.ErrTxDone rollback
// return confirms the open transaction was ended without persisting.
func (w *StartedWriteRequest) rollback(ctx context.Context, tx *sql.Tx, res WriteResult) WriteResult {
	w.mu.Lock()
	w.noncancellable = true
	w.mu.Unlock()
	w.emit(WritePhaseRollbackCleanup)
	if w.owner.beforeWriteRollback != nil {
		w.owner.beforeWriteRollback(ctx, nil)
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
```

The fix is the `res.Phase == WritePhaseCommitting` branch. When the COMMIT itself failed, `tx.Rollback()` returns `sql.ErrTxDone` because `database/sql` marks the transaction as done after `Commit()` is called. The old code treated `sql.ErrTxDone` as rollback success (`RollbackConfirmed = true`), but that is wrong — the transaction is no longer active, but we cannot tell whether it was committed or rolled back at the SQLite level. The new branch ignores the rollback return entirely: `RollbackConfirmed` stays false, the commit error is preserved, and the result routes through the outcome-unknown terminal workflow. For pre-COMMIT paths, the old behavior is unchanged: nil or `sql.ErrTxDone` from `tx.Rollback()` confirms the open transaction was ended without persisting.

```bash
sed -n '305,325p' internal/ui/write_exec.go
```

```output
	m.ActiveCancellable = false
	m.CancelCommand = nil
	m.writeCancel = nil
	m.writePhases = nil

	phase := history.UnknownPhaseRollback
	// Issue #61: prefer the result's preserved failure-boundary phase when
	// set — a failed COMMIT carries WritePhaseCommitting even though the
	// last emitted phase was rollback-cleanup. Fall back to the model's
	// tracked phase for results that predate the Phase field.
	resultPhase := msg.Result.Phase
	if resultPhase == 0 {
		resultPhase = m.writePhase
	}
	if resultPhase == connection.WritePhaseCommitting {
		phase = history.UnknownPhaseCommit
	}
	cause := ""
	if msg.Result.Err != nil {
		cause = msg.Result.Err.Error()
	}
```

The UI prefers `msg.Result.Phase` (from the connection layer) when set. This matters because the last emitted phase for a COMMIT failure is `rollback-cleanup` (emitted by `rollback()`), but the failure boundary is `committing`. Without the `Phase` field, the UI would select `UnknownPhaseRollback` instead of `UnknownPhaseCommit`. The fallback to `m.writePhase` preserves backward compatibility for results that predate the field (e.g., fake-executor tests in `outcome_unknown_terminal_test.go`).

```bash
go test ./internal/connection/ -run '^TestWriteCommitFailurePreservesOutcomeUnknown$' -count=1 -v 2>&1
```

```output
=== RUN   TestWriteCommitFailurePreservesOutcomeUnknown
--- PASS: TestWriteCommitFailurePreservesOutcomeUnknown (0.02s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.023s
```

The test passes. Let's trace what happens inside it. The test uses a deferred foreign-key constraint (`DEFERRABLE INITIALLY DEFERRED`) with `PRAGMA foreign_keys = ON` on the write's leased connection. The INSERT statement succeeds (RowsAffected = 1), but COMMIT fails because the deferred FK constraint is violated at COMMIT time. The phase sequence is `beginning → executing → committing → rollback-cleanup`, `RollbackConfirmed` is false, and `Phase` is `WritePhaseCommitting`.

```bash
go test ./internal/connection/ -run '^TestWriteCommitFailurePreservesOutcomeUnknown$|^TestWritePreCommitFailureConfirmsRollback$|^TestWritePreCommitCancellationConfirmsRollback$' -count=1 -v 2>&1
```

```output
=== RUN   TestWriteCommitFailurePreservesOutcomeUnknown
--- PASS: TestWriteCommitFailurePreservesOutcomeUnknown (0.02s)
=== RUN   TestWritePreCommitFailureConfirmsRollback
--- PASS: TestWritePreCommitFailureConfirmsRollback (0.01s)
=== RUN   TestWritePreCommitCancellationConfirmsRollback
--- PASS: TestWritePreCommitCancellationConfirmsRollback (0.01s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.052s
```

```bash
go test ./internal/ui/ -run '^TestRealCommitFailureRoutesThroughOutcomeUnknown$|^TestRealPreCommitFailureWithConfirmedRollback$' -count=1 -v 2>&1
```

```output
=== RUN   TestRealCommitFailureRoutesThroughOutcomeUnknown
--- PASS: TestRealCommitFailureRoutesThroughOutcomeUnknown (0.01s)
=== RUN   TestRealPreCommitFailureWithConfirmedRollback
--- PASS: TestRealPreCommitFailureWithConfirmedRollback (0.01s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.023s
```

The UI test drives a real `tx.Commit()` failure through `write_exec.go`'s finalization path. The COMMIT failure produces exactly one `KindOutcomeUnknown` entry selected in `TerminalOutcomeUnknown` with `UnknownPhaseCommit`, commit-phase/error wording (`the commit did not resolve`), non-persistence `RowsAffected` disclosure (`does not prove persistence`), and no `committed`/`untouched`/`rollback confirmed` claims. The control case (`TestRealPreCommitFailureWithConfirmedRollback`) produces a definite `KindWrite` failed entry with `rollback confirmed, database untouched` wording — the outcome-unknown routing is exclusive to the COMMIT-failure path.

```bash
sed -n '48,100p' internal/connection/commit_failure_test.go
```

```output
);`)
	enableForeignKeysOnWriteLease(t, db)

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

	// The lease is still reusable after settlement: the physical connection
	// was not force-closed and remains healthy for later work.
	db.beforeWriteBegin = nil
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestWritePreCommitFailureConfirmsRollback is the control case: a pre-COMMIT
// statement failure (UNIQUE constraint) followed by a genuinely successful
// rollback remains confirmed untouched, proving the outcome-unknown routing
// is exclusive to the COMMIT-failure path. The phase is WritePhaseExecuting,
// not WritePhaseCommitting.
func TestWritePreCommitFailureConfirmsRollback(t *testing.T) {
	db := openJournalFixture(t, "delete")

	w := db.StartWrite(context.Background(), writeExecSeq.Add(1),
		`INSERT INTO "users" ("id", "email") VALUES (?, ?)`,
		[]any{int64(1), "dupe"})
	res := w.Wait()

	if res.Outcome != WriteFailed {
		t.Fatalf("outcome = %v (err %v), want failed", res.Outcome, res.Err)
```

```bash
sed -n '120,165p' internal/ui/commit_failure_test.go
```

```output
		`INSERT INTO "child" ("id", "parent_id") VALUES (?, ?)`,
		[]any{int64(1), int64(999)})
	if cmd == nil {
		t.Fatal("startWrite produced no command")
	}
	m = dispatchWriteBatch(t, m, cmd, true)

	if m.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", m.terminalState)
	}
	if m.writePending {
		t.Fatal("settled unresolved write left pending state behind")
	}
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one outcome-unknown entry", m.ResultHistory.Len())
	}
	e, ok := m.ResultHistory.Newest()
	if !ok || e.ID != m.resultHistoryCursorID {
		t.Fatalf("selected cursor = %v, want the newest appended entry (ok=%v)", m.resultHistoryCursorID, ok)
	}
	if e.Kind != history.KindOutcomeUnknown {
		t.Errorf("entry kind = %v, want outcome-unknown", e.Kind)
	}
	if e.ExecutionID != execution {
		t.Errorf("entry execution = %d, want %d", e.ExecutionID, execution)
	}
	if e.Operation != "INSERT" {
		t.Errorf("entry operation = %q, want INSERT", e.Operation)
	}
	if e.Phase != history.UnknownPhaseCommit {
		t.Errorf("entry phase = %v, want commit", e.Phase)
	}
	if !strings.Contains(e.Summary, "commit did not resolve") {
		t.Errorf("summary %q lacks the commit-phase wording", e.Summary)
	}
	if !strings.Contains(strings.ToLower(e.Summary), "foreign key") {
		t.Errorf("summary %q lost the driver commit error", e.Summary)
	}
	if !strings.Contains(e.Summary, "does not prove persistence") {
		t.Errorf("summary %q lacks the non-persistence RowsAffected disclosure", e.Summary)
	}
	for _, forbidden := range []string{"committed", "untouched", "rollback confirmed", "rows added"} {
		if strings.Contains(strings.ToLower(e.Summary), strings.ToLower(forbidden)) {
			t.Errorf("summary %q claims %q the resolution never proved", e.Summary, forbidden)
		}
	}
```

```bash
sed -n '230,255p' internal/ui/commit_failure_test.go
```

```output
	if cmd == nil {
		t.Fatal("startWrite produced no command")
	}
	m = dispatchWriteBatch(t, m, cmd, true)

	if m.terminalState == TerminalOutcomeUnknown {
		t.Fatal("pre-COMMIT failure with confirmed rollback entered outcome-unknown; want a definite failed entry")
	}
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", m.ResultHistory.Len())
	}
	e, ok := m.ResultHistory.Newest()
	if !ok {
		t.Fatal("no entry was appended")
	}
	if e.Kind != history.KindWrite {
		t.Errorf("entry kind = %v, want write (definite failed)", e.Kind)
	}
	if !strings.Contains(e.Summary, "failed") {
		t.Errorf("summary %q lacks the definite failed wording", e.Summary)
	}
	if !strings.Contains(e.Summary, "rollback confirmed") {
		t.Errorf("summary %q lacks the confirmed-untouched wording for genuine rollback", e.Summary)
	}
}
```

The control case proves the outcome-unknown routing is exclusive to the COMMIT-failure path. A pre-COMMIT UNIQUE constraint failure with genuine rollback produces a definite `KindWrite` entry with `failed` and `rollback confirmed` wording — not an outcome-unknown entry. The terminal state is NOT `TerminalOutcomeUnknown`.

```bash
go test ./internal/connection/ ./internal/ui/ -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/connection	40.182s
ok  	github.com/chris/sqloid/internal/ui	0.149s
```

Issue #57's production composition root (`internal/session`) wires the `Write` seam to a thin adapter over `connection.DB` that drains write phases through the callback before `Wait()`. The COMMIT-failure fix is reachable through the shipped TUI: a real write whose `tx.Commit()` fails will produce a `WriteResult` with `Phase: WritePhaseCommitting` and `RollbackConfirmed: false`, which the UI's `writeUnresolved` classifies as outcome unknown, entering `TerminalOutcomeUnknown` with exactly one non-persistence entry. The headless harness in `internal/session/compose_test.go` exercises the same composition path. After Issue #57 lands, the terminal workflow is reachable through the shipped TUI; before that, the package/seam-level tests verified here are the authoritative proof.
