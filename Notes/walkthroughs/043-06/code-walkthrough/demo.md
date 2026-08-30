# Issue #43: Commit-boundary cancellation and quit cleanup

*2026-08-29T15:33:18Z by Showboat 0.6.1*
<!-- showboat-id: be7df831-db6e-4524-ac2d-c3726b846f45 -->

Issue #43 enforces the write commit boundary (Issues #42/#43, Notes/PRD-sqloid.md 'Writes and commit boundary'). One write is held in each phase — beginning, executing, the atomic after-statement/before-COMMIT decision, rollback-cleanup, and committing — using channel barrier seams (no sleeps).

## 1. One write held in every phase, deterministically

```bash
go test ./internal/connection -run '^TestWriteCommitLifecycle$|^TestWriteCancellationBeforeBegin$|^TestWriteCancellationDuringStatement$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestWriteCommitLifecycle
=== RUN   TestWriteCommitLifecycle/qualified_update_commits_and_persists
=== RUN   TestWriteCommitLifecycle/unqualified_update_commits_and_persists
=== RUN   TestWriteCommitLifecycle/qualified_delete_commits_and_persists
=== RUN   TestWriteCommitLifecycle/unqualified_delete_commits_and_persists
=== RUN   TestWriteCommitLifecycle/runnable_insert_commits_and_persists
--- PASS: TestWriteCommitLifecycle (0.08s)
=== RUN   TestWriteCancellationBeforeBegin
--- PASS: TestWriteCancellationBeforeBegin (0.01s)
=== RUN   TestWriteCancellationDuringStatement
--- PASS: TestWriteCancellationDuringStatement (0.02s)
ok  	github.com/chris/sqloid/internal/connection	0.109s
```

The lifecycle suite proves one lease, exactly one pre-BEGIN identity check, and the exact phase sequences: committed writes run beginning→executing→committing; a write cancelled before BEGIN or during execution rolls back with confirmation and persists nothing.

## 2. Ctrl+W before the boundary: one scoped interrupt against the leased connection

```bash
go test ./internal/connection -run '^TestWriteInterruptScopedToLeaseBeforeBoundary$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestWriteInterruptScopedToLeaseBeforeBoundary
--- PASS: TestWriteInterruptScopedToLeaseBeforeBoundary (0.02s)
ok  	github.com/chris/sqloid/internal/connection	0.020s
```

A counting interrupt hook is installed on exactly the write's own leased connection via the writeLeaseHook seam. While the write is cancellable, Cancel dispatches exactly one connection-scoped interrupt; the lease stays owned and is reused only after cancelled settlement.

## 3. The atomic pre-COMMIT check: cancellation wins after statement success

```bash
go test ./internal/connection -run '^TestWriteCancellationWinsAfterStatementSuccess$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestWriteCancellationWinsAfterStatementSuccess
--- PASS: TestWriteCancellationWinsAfterStatementSuccess (0.02s)
ok  	github.com/chris/sqloid/internal/connection	0.019s
```

The statement has already succeeded; the after-statement/before-COMMIT decision is still cancellable. The mutex-protected check reads the cancellation flag and marks the execution noncancellable in one critical section — the flag wins, rollback is confirmed, the database is untouched, and the committing phase is never announced (beginning → executing → rollback-cleanup).

## 4. After the boundary: no interrupt during cleanup or after settlement

```bash
go test ./internal/connection -run '^TestWriteNoInterruptDuringRollbackCleanup$|^TestWriteNoInterruptAfterBoundarySettlement$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestWriteNoInterruptDuringRollbackCleanup
--- PASS: TestWriteNoInterruptDuringRollbackCleanup (0.02s)
=== RUN   TestWriteNoInterruptAfterBoundarySettlement
--- PASS: TestWriteNoInterruptAfterBoundarySettlement (0.02s)
ok  	github.com/chris/sqloid/internal/connection	0.040s
```

Once rollback cleanup has begun — or the write has settled — repeated Cancel calls dispatch no second interrupt, leave phase/work/state unchanged, and cannot reopen the settled lease. Rollback still resolves with confirmation; the lease is reusable afterwards.

## 5. UI Ctrl+W routing: pre-boundary dedupe, exact post-boundary feedback

```bash
go test ./internal/ui -run '^TestWriteCtrlWCancelsOnceBeforeBoundary$|^TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback$|^TestWriteBoundaryResistsRegressionAndStaleIdentities$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestWriteCtrlWCancelsOnceBeforeBoundary
--- PASS: TestWriteCtrlWCancelsOnceBeforeBoundary (0.00s)
=== RUN   TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback
=== RUN   TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback/committing
=== RUN   TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback/rollback-cleanup
--- PASS: TestWriteCtrlWAfterBoundaryIsInertWithExactFeedback (0.00s)
=== RUN   TestWriteBoundaryResistsRegressionAndStaleIdentities
--- PASS: TestWriteBoundaryResistsRegressionAndStaleIdentities (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.010s
```

Scripted model tests hold the sole write behind a blocking fake executor: before the boundary Ctrl+W requests cancellation exactly once (repeats are deduped); during rollback-cleanup or committing it returns no command, cancels no context, mutates no work, and shows exactly 'Commit in progress; cancellation is no longer available'. Regressed executing phases, duplicate boundary phases, stale execution identities, and replacement-write attempts cannot cross the boundary backward.

## 6. Accepted quit from cancellable and noncancellable phases: exit only after settlement

```bash
go test ./internal/ui -run '^TestAcceptedQuitDuringCancellableWriteWaitsForRollback$|^TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestAcceptedQuitDuringCancellableWriteWaitsForRollback
--- PASS: TestAcceptedQuitDuringCancellableWriteWaitsForRollback (0.00s)
=== RUN   TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution
=== RUN   TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution/committing_commits
=== RUN   TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution/rollback_cleanup_resolves_failed
--- PASS: TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.009s
```

Accepting quit during cancellable work issues exactly one cancellation request and the application stays alive — no exit command — until the write settles through rollback resolution. Accepting quit during committing or rollback cleanup issues no interrupt and waits for the existing operation. In every case tea.Quit is emitted only after the settlement finalizes exactly one result entry.

## 7. Unresolved outcomes: quit waits, finalizes once, never overclaims

```bash
go test ./internal/ui -run '^TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'; go test ./internal/history -run '^TestResultStoreQuit' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork
=== RUN   TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork/unresolved_rollback
=== RUN   TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork/unresolved_commit
--- PASS: TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
--- PASS: TestResultStoreQuitFinalizesWriteEntryOnce (0.00s)
--- PASS: TestResultStoreQuitEntriesNeverOverclaimOutcomes (0.00s)
ok  	github.com/chris/sqloid/internal/history	0.001s
```

Unresolved rollback and unresolved commit both keep the application alive until pending work ends, then finalize exactly one entry whose label makes no untouched claim and no persistence claim (rollback unconfirmed). The history-level tests pin the entry contract: exactly-once retention per execution identity, newest selection, and labels claiming 'untouched' only after confirmed rollback.

## 8. Guards: duplicates, stale identities, declined quit, no abandonment

```bash
go test ./internal/ui -run '^TestAcceptedQuitDuplicatesStaleSettlementAndDecline$|^TestQuitWaitProhibitsReplacementWork$' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)'
```

```output
=== RUN   TestAcceptedQuitDuplicatesStaleSettlementAndDecline
=== RUN   TestAcceptedQuitDuplicatesStaleSettlementAndDecline/duplicate_acceptance
=== RUN   TestAcceptedQuitDuplicatesStaleSettlementAndDecline/stale_settlement_identity
=== RUN   TestAcceptedQuitDuplicatesStaleSettlementAndDecline/declined_quit_restores_exact_phase
--- PASS: TestAcceptedQuitDuplicatesStaleSettlementAndDecline (0.00s)
=== RUN   TestQuitWaitProhibitsReplacementWork
--- PASS: TestQuitWaitProhibitsReplacementWork (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.007s
```

Duplicate acceptance never exits early nor requests a second cancellation; a stale settlement identity can neither exit nor finalize; a declined quit restores the exact suspended write phase with no side effects and Ctrl+W remains live; and no replacement write can start while quit awaits settlement — the sole write keeps its lease until it resolves.

## 9. Full verification

```bash
gofmt -l internal cmd; go vet ./... && go build ./... && go test ./... 2>&1 | tail -10
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```
