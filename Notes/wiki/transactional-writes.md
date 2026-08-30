# Transactional Write Execution and Summaries

Issue #42 turns a confirmed destructive write (UPDATE/DELETE, Issues #40–#41) or a dispatched runnable INSERT (Issue #39) into exactly one **transactional, cancellable, ph**as**ed** database execution, and finalizes it into exactly one immutable non-tabular result-history entry with an operation-appropriate summary. It implements the PRD's "Writes and commit boundary" lifecycle section `builder → estimating → awaiting-confirmation → beginning → executing → rollback-cleanup or committing → committed/failed/cancelled` (the outcome-unknown terminal workflow remains Issue #45's; post-boundary Ctrl+W/quit interaction rendering remains Issue #43's).

## One-request / one-lease lifecycle (`internal/connection/write.go`)

`(*DB).StartWrite(parent, execution, statement, params)` runs the entire write as **one request on one dedicated leased connection**:

1. **One path-identity check** — acquiring the lease is the write's single request-boundary identity check (Issue #7). There is exactly one pre-BEGIN check and none between statement and COMMIT, because the whole transaction is one request. The `identityChecks` counter is test-observable.
2. **BEGIN** — the write emits `WritePhaseBeginning`, then opens the transaction with `conn.BeginTx` (never raw SQL `BEGIN`/`COMMIT`, per the database-access rules). If the context is already cancelled the driver may fail BEGIN (nothing to roll back, no untouched claim) or open the transaction and let the statement fail on it; both classify as cancelled.
3. **Sole statement execution** — the write emits `WritePhaseExecuting` and executes exactly one statement (`tx.ExecContext`) with the ordered bound parameters from the QueryBuilder rendering seam. There is no second execution of any kind; a per-firing trigger test proves the statement ran once. `RowsAffected()` is captured from the statement result itself — never an estimate (Issue #40's count is deliberately independent).
4. **Atomic pre-COMMIT cancellation check** — after statement success, and before any committing announcement, the write performs the Issue #43-enforced atomic check under one mutex: the request cancellation flag (`Request.CancelRequested`) is read and the write is permanently marked noncancellable in the same critical section. **The cancellation flag wins even though the statement returned success**: a cancel requested at any cancellable point — before BEGIN, during execution, or after statement success — routes to rollback cleanup, never commit; a cancel arriving after the boundary is permanently ignored.
5. **Rollback cleanup or commit** — the `WritePhaseCommitting` message is announced only after the boundary is crossed and immediately before `tx.Commit()`, so a pre-COMMIT-cancelled write never emits it (`beginning → executing → rollback-cleanup`). Cancellation or any statement failure (including native constraint and trigger errors) enters `WritePhaseRollbackCleanup` — which re-establishes noncancellability for the statement-failure path — and waits for the rollback; `RollbackConfirmed` is true exactly when the rollback succeeded. An uncancelled statement crosses exactly once to `tx.Commit()`. Rollback cleanup and commit are **noncancellable**: `Cancel` issues no context cancellation and no driver interrupt after the boundary, and a commit failure resolves (for #42) as a failed write with rollback attempted; persistence after a failed commit is unprovable and belongs to Issue #45's outcome-unknown workflow.
6. **Settlement and release** — the Issue #6 request lifecycle settles (cancellation cause → `WriteCancelled`, other errors → `WriteFailed`, success → `WriteCommitted`), releases the lease, and re-verifies health after errors with race precedence. Cancellation never force-closes the connection; the lease is reused by later writes only after settlement.

Phases are delivered on `Phases()` in order (buffered, closed at settlement); `Wait()` returns the resolved `WriteResult{Outcome, RowsAffected, Err, Health, RollbackConfirmed}`. Exactly one definite outcome is produced per execution.

## Typed UI consumption (`internal/ui/write_exec.go`)

The UI consumes the typed phases through the `WriteExecutor` seam:

- **`WriteConfirmedMsg` delivery is the confirmed UPDATE/DELETE execution start**: the fresh-execution guard in `applyWriteConfirmed` rejects duplicate/stale deliveries, then `beginConfirmedWrite` exits both histories first, appends the complete query state through Issue #20's `AppendExecution` (subject only to consecutive-identical suppression — the identical restored state is suppressed without an ID), and dispatches `startWrite` with the retained rendered statement and SET-then-WHERE parameters.
- **`ExecutionStartedMsg` for INSERT** dispatches the sole transactional write of `InsertSQL()`/`InsertParams()` under a fresh `NextWriteExecutionID`; SELECT continues its page/count lifecycle. Both boundaries exit query and result history first.
- **Phase messages** update retained state only. The `beginning`/`executing` phases own `ActiveCancellable`/`CancelCommand` (Ctrl+W requests the scoped interrupt and shows cancelling state until settlement); the arrival of `rollback-cleanup` or `committing` **retires cancellation ownership** so later Ctrl+W is inert.
- **`WriteSettledMsg` finalization** is exactly-once per execution (model `writeFinalized` flag plus the store's execution-identity guard): duplicate, late, and stale phase/outcome messages append nothing and start nothing. There are no per-phase or per-message entries.

## History and summaries

- `history.ResultEntry` gains `KindWrite` with `SQL` (the executed standalone statement), `RowsAffected` (the actual statement count, retained for Issue #45's non-persistence labeling), and `Summary`. `AppendFinalized` ties the entry to the execution identity and rejects a second one.
- `history.WriteSummary` renders the exact label: committed UPDATE/DELETE use the actual `RowsAffected()` with rows-affected wording (`UPDATE committed: 3 rows affected`); committed INSERT uses rows-added wording (`INSERT committed: 1 rows added`); trigger/constraint behavior never substitutes estimate counts. Cancelled and failed writes carry the verbatim cause where present and append **`rollback confirmed, database untouched` only after confirmed rollback** — without confirmation the label makes no untouched claim at all (`UPDATE cancelled`, `UPDATE failed: <cause>`).

## Tests

- `internal/connection/write_test.go` — real SQLite barrier-seam coverage (see below).
- `internal/history/write_summary_test.go` — label wording matrix, exactly-once identity-tied retention with immutability, duplicate/late rejection, and the mandatory execution identity.
- `internal/ui/write_lifecycle_test.go` — scripted `Update` coverage with the controllable fake executor: execution-start append/exit-histories/sole-execution, INSERT dispatch, cancelled-with/without-confirmed-rollback labels, failed-cause preservation, duplicate/late/stale idempotence, and post-boundary noncancellability.

## Testing decisions

- The connection suite uses the DB's documented `beforeWriteBegin`/`beforeWriteExec`/`beforeWriteCommit`/`beforeWriteRollback` **barrier seams**: each write blocks inside the hook after its phase message until the test releases it, so cancellation lands deterministically (channel receives only, no sleeps).
- `TestWriteCommitLifecycle` (table-driven over qualified/unqualified UPDATE, DELETE, INSERT) proves exactly one identity check, the `beginning→executing→committing` sequence, actual `RowsAffected`, persisted state read through an independent connection, and healthy lease reuse.
- `TestWriteTriggerSideEffectsCommit` proves trigger effects commit inside the same transaction, `RowsAffected` remains the statement's own count, and a firing counter proves exactly one execution. `TestWriteConstraintFailureRollsBack` and `TestWriteTriggerFailureRollsBack` prove failed writes preserve the native cause, confirm rollback, and persist nothing.
- `TestWriteCancellationBeforeBegin`, `...DuringStatement`, and `TestWriteCancellationWinsAfterStatementSuccess` hold each transition behind its barrier and prove the pre-COMMIT cancellation check wins after statement success (cancelled + confirmed rollback + untouched database).

## Cross-references

- Issues #6 (request lifecycle, cancellation-wins settlement), #7 (one pre-BEGIN identity check), #9/#14 (schema identity and literal rendering feeding the statement), #20/#35/#36 (execution-start history append, suppression, exit-history-first, exactly-one result), #21 (validation precedes execution), #28 (scoped interrupt machinery), #39 (INSERT rendering), #40–#41 (estimate and confirmation producing `WriteConfirmedMsg`).
- PRD sections: Identities and state; Writes and commit boundary; write-transaction and Estimate SQL/modal Implementation Decisions; Connection/UI/History Module Design; Testing Decisions (items 3, 11, 14).
- Issue **#43** implements post-commit-boundary Ctrl+W feedback, typed boundary routing, permanent interrupt retirement, and accepted-quit settlement — documented in [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md); **#45** owns unresolved rollback/commit (outcome-unknown) terminal handling and the non-persistence `RowsAffected` labeling.
