# Release capability suite and the modernc.org/sqlite pin/upgrade gate (Issue #56, expanded by Issue #88)

Issue #56 turns the PRD's mandatory release blockers (Testing Decisions items 2 and 3 in `Notes/PRD-sqloid.md`) into one executable gate: a single canonical capability-suite command, identical on Linux and macOS, that blocks every release and every `modernc.org/sqlite` dependency upgrade. Issue #88 expands that sole workflow into the full cross-platform release gate: both jobs now also run repository-wide `go test ./...`, `go build ./...`, and `go vet ./...`, and the Issue #57 PTY-driven built-binary integration test runs unattended on both platforms through `go test ./...` (it lives in `cmd/sqloid`). The targeted race/capability suite from Issue #56 is retained as a separate gate for its specialized cancellation guarantees — it is not replaced by the ordinary repository tests. There are no skips, `continue-on-error`, allowed failures, retries that hide reproducible failures, platform exclusions, or weakened assertions anywhere in the gate.

## The vetted pin

- `go.mod` requires `modernc.org/sqlite v1.57.0` — a **direct, exact semantic-version pin**: no branch, no wildcard, no `replace` directive, no local path substitution, no best-effort fallback in any open path (`internal/connection` opens with the pinned driver only; pure Go, no cgo in production builds).
- `go.sum` carries the matching `v1.57.0` module hashes; the indirect graph (`modernc.org/libc v1.74.4`, `modernc.org/memory v1.11.0`, `modernc.org/mathutil v1.7.1`) is exactly what that pin resolves to.
- **v1.57.0 is the only version accepted by the release gate.** The CI gate fails immediately (before running any test) if `go list -m modernc.org/sqlite` does not print `modernc.org/sqlite v1.57.0`.

## The one canonical capability-suite command

```
scripts/capability-suite.sh
```

which runs, from the repository root:

```
CGO_ENABLED=1 go test -race -count=1 -timeout 20m ./internal/connection ./internal/ui ./internal/history
```

- Selects **all and only** the integrated release-blocking capability tests: every capability test lives beside the code in `internal/connection` (journal/pool, value limits, lease overlap, driver-level SELECT/write cancellation, estimate/schema, health identity), `internal/ui` (orchestration, identities, commit-boundary routing, health terminals, settlement), and `internal/history` (exactly-once finalization, quit settlement, no-history guards). Developer- and CI-runnable with no special environment.
- `-race` runs under cgo (`CGO_ENABLED=1`) even though production remains pure-Go; a data race fails the job.
- Any setup, test, timeout (`-timeout 20m` per package plus job-level timeout), race, or cleanup failure fails the script with non-zero status and blocks the job. There is no `|| true`, no retry, no platform-specific filter, and no conditional skip for any supported capability (the only build tag, `unix`, selects exactly the supported Linux/macOS platforms; the capability tests are release-blocking on both).

## CI configuration

`.github/workflows/capability-suite.yml` defines two jobs, `capability-suite (linux)` on `ubuntu-latest` and `capability-suite (macos)` on `macos-latest`. Both jobs run the same seven steps in the same order:

1. clean checkout (`actions/checkout@v4`);
2. Go installed from `go-version-file: go.mod`;
3. pin assertion: `go list -m modernc.org/sqlite` must equal `modernc.org/sqlite v1.57.0` or the job fails before testing;
4. **repository-wide tests** — `go test -count=1 -timeout 20m ./...` exercises every shipped package, including the Issue #57 PTY-driven built-binary integration test in `cmd/sqloid` (`TestSqloidPTYEndToEndBuildsAndRunsRealBinary`). That test builds the real `sqloid` binary from `cmd/sqloid`, creates a real temporary SQLite database fixture, spawns the binary under `github.com/creack/pty.StartWithSize` with a 100×30 terminal, responds to Bubble Tea's `\x1b[6n` cursor-position-report request so the UI renders, waits for the builder's `Command` field to appear, sends `q` then `Enter` to confirm the universal quit, and asserts the process exits with status 0. No injected runners or fakes are used: this is the shipped binary through the shipped composition root through a real terminal. The test fails if the program launch, any real adapter, or the TUI run is bypassed even if package fake-seam tests pass;
5. **repository-wide build** — `go build ./...` proves every shipped package compiles;
6. **repository-wide vet** — `go vet ./...` proves every shipped package is vet-clean;
7. **canonical capability suite** — `scripts/capability-suite.sh`, the identical targeted race/cancellation command from Issue #56, retained as a separate gate for its specialized `-race` cancellation guarantees rather than replaced by the ordinary repository tests above.

The workflow runs on every `pull_request` and every push to `main`. A `modernc.org/sqlite` change is always a `go.mod`/`go.sum` change carried by a pull request, so **any dependency change triggers both jobs, and the upgrade cannot merge as successful unless the same gate passes on both platforms** (with branch protection treating `capability-suite (linux)` and `capability-suite (macos)` as required checks). `timeout-minutes: 45` bounds each job; there are no `continue-on-error` steps.

### Ordering, timeouts, and fail-closed semantics

Steps run sequentially within each job; the first failing step fails the job immediately — no later step runs. The per-test `-timeout 20m` flag bounds each package's test execution; the job-level `timeout-minutes: 45` bounds the whole job. The capability suite's own `set -eu` and `-timeout 20m` bound the targeted race gate. Any setup, test, build, vet, capability, binary-integration, timeout, or cleanup failure fails the job and blocks merging. There is no `|| true`, no retry, no platform-specific filter, and no conditional skip for any supported platform.

### Captured diagnostics and temporary-fixture cleanup

The PTY integration test captures all PTY output in a `bytes.Buffer` and includes it in `t.Fatalf` messages on failure, so CI logs show the rendered TUI state at the point of failure. The test uses `t.TempDir()` for both the built binary and the SQLite fixture, so Go's test framework removes them automatically when the test ends; `t.Cleanup` also removes the binary and closes/kills the PTY/process. The capability suite's fixtures are likewise temporary and cleaned up by Go's test framework. No goroutines, leases, temporary save artifacts, or open database handles are left behind on a green run.

### Regression classes the expanded gate catches

Before Issue #88 the gate ran only the targeted race/capability suite (`internal/connection`, `internal/ui`, `internal/history` under `-race`). The expanded gate now also catches:

- **Build failures in any shipped package** — `go build ./...` covers `cmd/sqloid`, `internal/cli`, `internal/connection`, `internal/d1`, `internal/export`, `internal/filepicker`, `internal/history`, `internal/querybuilder`, `internal/result`, `internal/resultcache`, `internal/schema`, `internal/session`, and `internal/ui`. A compilation error in any of these blocks merging.
- **Vet failures in any shipped package** — `go vet ./...` catches suspicious constructs (printf format mismatches, unreachable code, shadowed variables, struct tag issues) in every package.
- **Test failures in any shipped package** — `go test ./...` runs every `*_test.go` file in every package, not just the three targeted by the capability suite. This catches regressions in `internal/cli`, `internal/d1`, `internal/export`, `internal/filepicker`, `internal/querybuilder`, `internal/result`, `internal/resultcache`, `internal/schema`, and `internal/session` that the capability suite alone did not cover.
- **Production-composition regressions** — the PTY integration test in `cmd/sqloid` proves the shipped binary reaches and operates the real application composition root (`internal/session.Compose` → `tea.NewProgram` → real adapters → real SQLite). A regression that bypasses production composition — for example, a CLI handler that returns after validation without constructing the UI, or a broken adapter that leaks a driver type — fails this test even if every package-local fake-seam test passes.
- **Cross-platform regressions** — both Linux and macOS run the identical gate, so a platform-specific failure (e.g., a syscall or PTY behavior difference) blocks merging on the failing platform.

## modernc.org/sqlite upgrade procedure

1. Change the exact pin in `go.mod` (`go get modernc.org/sqlite@<version>`) and commit the updated `go.mod`/`go.sum` together.
2. Run the local gate: `scripts/capability-suite.sh` from a clean checkout.
3. Open a pull request; the dependency change triggers both CI jobs, which run the same command on Linux and macOS.
4. Require both platforms green with the new pin's assertion updated. **Reject or replace any failing version** — the remedy is an implementation fix or a different exact vetted version, never a weakened test, a skip, or best-effort acceptance.
5. Retain the passing evidence (job logs) for the release record and record the new vetted version here.

## Traceability: PRD Testing Decisions item 2 (journal/pool release blocker)

Every requirement maps one-to-one to a named test in the gated suite:

| Required case | Named gated test |
| --- | --- |
| WAL and rollback-journal count/page overlap on two distinct leases (barrier-held, no application mutex/queue) | `connection.TestConcurrentPageAndCountOverlapDistinctLeases` |
| The two leases are distinct physical connections | `connection.TestPageAndCountLeasesAreDistinctPhysicalConnections`, `connection.TestConcurrentLeasesAreDistinctConnections` |
| Independent autocommit reads with permitted drift, no shared snapshot/clamp | `connection.TestIndependentSnapshotsPermitDrift`, `ui.TestCountSettlesIndependentlyWhilePagePending` |
| External writer delay or ordinary lock error while both requests still succeed | `connection.TestRollbackJournalExternalWriterDelayOrLockError` |
| Journal mode byte-for-byte unchanged throughout | `connection.TestOpenPreservesJournalMode` (asserted inside the overlap fixtures for both modes) |
| Pool size exactly two (min and max) | `connection.TestPoolHoldsExactlyTwoUsableConnections` |
| Five-second busy timeout on every connection, including replacement connections | `connection.TestEveryConnectionHasFiveSecondBusyTimeout` |
| Connection-local `SQLITE_LIMIT_LENGTH` exactly 64 MiB on every connection | `connection.TestEveryConnectionHasExactLengthLimit`, `connection.TestRunFirstPageValueLimitTypedFailure`, `connection.TestExecutePageValueLimitNonFirstPagePosition` |
| Lease release on success, error, cancellation, and teardown; no hidden serialization | `connection.TestLeaseReleaseIsSafeAndRefusesReuse`, `connection.TestLeaseHeldUntilSettlementThenReusable`, `connection.TestCancelledRequestHoldsLeaseUntilSettlement` |
| Count failure does not invalidate page rows (independent completion) | `connection.TestCountFailureDoesNotInvalidatePageRows` |

## Traceability: PRD Testing Decisions item 3 (pinned-driver cancellation release blocker)

| Required case | Named gated test |
| --- | --- |
| Interrupt long CPU-bound page and count independently, each cancelled alone and together, scoped to the intended request | `connection.TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond`, `connection.TestCapabilityLaterPageCancelInterruptsWithinOneSecond`, `connection.TestCPUBoundWorkInterruptsWithinOneSecond` |
| CPU cancellation settles within one second | `connection.TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond`, `connection.TestCPUBoundWorkInterruptsWithinOneSecond` |
| Lock-wait cancellation no later than the five-second busy timeout | `connection.TestCapabilityLockWaitCountCancelsWithinBusyTimeout`, `connection.TestLockWaitInterruptsWithinFiveSeconds` |
| Cancel one without affecting the other active request | `connection.TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond` (isolation assertions), `connection.TestPageAndCountSettleIndependentlyInEitherOrder` |
| Subsequent request on the same connection unaffected | `connection.TestConnectionNotForceClosedByCancellationAndSafeForReuse`, `connection.TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable` |
| Discard late success (cancellation wins); identity rejection for stale/duplicate/late completions | `connection.TestCapabilityLateSuccessIsCancellationWinsOnRealConnection`, `connection.TestLateSuccessIsDiscardedAsCancelled`, `connection.TestCancelledLaterPageWinsOverLateSuccess`, `connection.TestCancelledCountStaysInert`, `connection.TestLateCancelledResponseAfterNewerExecutionStaysInert` |
| Lease held until true settlement; no replacement request before settlement | `connection.TestCapabilityNoLeaseReuseBeforeEveryRequestSettles`, `connection.TestCancelledRequestHoldsLeaseUntilSettlement`, `ui.TestReplacementWaitsForCancelledPredecessorOnNewerExecution` |
| Cancel schema validation with no history | `connection.TestReadSchemaVersionCancelledContextFailsWithCancellation`, `connection.TestRevalidateUnchangedVersionSkipsCatalogRefresh` (changed-schema refresh matrix), `ui` validation-workflow cancellation suites (see [schema-validation-workflow.md](schema-validation-workflow.md)) |
| Cancel destructive estimate with no history, no actual execution | `connection.TestExecuteEstimateHonoursCancellation`, `connection.TestExecuteEstimateNeverWrites`, `ui.TestDestructivePreparationEscDismissesWithCancellation` |
| No force-close, pool shrink, cross-connection interrupt, leaked lease/goroutine | `connection.TestConnectionNotForceClosedByCancellationAndSafeForReuse`, `connection.TestCapabilityNoLeaseReuseBeforeEveryRequestSettles` |
| Fixtures remain usable afterward; journal mode untouched | `connection.TestLeaseReleaseIsSafeAndRefusesReuse`, `connection.TestOpenPreservesJournalMode` |

## Traceability: Issue #56 identity-race and transaction evidence

| Required case | Named gated test |
| --- | --- |
| Deletion / rename-away / same-path replacement before request or new-connection boundaries with exact terminal messages | `connection.TestVerifyHealthClassifications`, `connection.TestLeaseVerifiesIdentityBeforeConnectionIsUsed`, `ui.TestSelectBoundaryHealthErrorEntersDeletionTerminal`, `ui.TestSelectBoundaryHealthErrorEntersReplacementTerminal`, `ui.TestHealthTerminalImmediateQuitFromEveryContext` |
| Raced replacement after successful precheck + request error classified terminal immediately | `connection.TestPostErrorReclassificationPrecedence` |
| Raced replacement + request success accepted for the already-open original, replacement detected at the next boundary before more work | `connection.TestRacedReplacementThenSuccessStands` |
| One precheck for the whole transaction, none between statement and COMMIT | `connection.TestPhasedWriteReceivesExactlyOnePreBEGINCheck` |
| Pre-COMMIT cancellation wins after statement success and resolves through confirmed rollback | `connection.TestWriteInterruptScopedToLeaseBeforeBoundary`, `ui.TestWriteCtrlWCancelsOnceBeforeBoundary` |
| Post-boundary Ctrl+W issues no interrupt; commit/rollback cleanup noncancellable | `connection.TestWriteNoInterruptDuringRollbackCleanup`, `connection.TestWriteNoInterruptAfterBoundarySettlement`, `ui.TestRepeatedCtrlWIsIdempotentAndStaleCancellationsInert` |
| Exactly-once outcome finalization / unresolved settlement before outcome unknown or exit | `history.TestResultStoreQuitFinalizesWriteEntryOnce`, `history.TestResultStoreQuitEntriesNeverOverclaimOutcomes`, `ui` outcome-unknown suites (see [outcome-unknown-terminal.md](outcome-unknown-terminal.md)) |

## Manual release checklist

Complete one copy per release per platform (Linux and macOS), at 80×24, 100×30, and 160×50, per the PRD's pure-rendering matrix and Testing Decisions final paragraph:

| Scenario | 80×24 | 100×30 | 160×50 |
| --- | --- | --- | --- |
| Exactly one bottom global footer row; no shared/overlapping borders | | | |
| Builder desired height (border/padding) capped at floor(H/3) with internal focused-field scroll | | | |
| Every remaining row owned by results; results area greater than half-height; exact complete-row page size | | | |
| Focused builder field scrolls internally without stealing result rows | | | |
| Horizontal overflow moves exactly one whole column per binding; boundary no-ops | | | |
| Oversized column: capped width with ellipsis, no intra-cell offset | | | |
| One-column overflow layout recomputation at the size | | | |
| Multiline values render fully inside the row budget | | | |
| Edge overlays (help, popups, pickers) do not reflow the layout | | | |
| Resize: first row preserved / clamped to retained low endpoint / fetch branch | | | |
| Resize: first column preserved then clamped | | | |
| Resize while a page/count/write request is active | | | |
| Below-80×24: exact `terminal too small`, state preserved, quit confirmation functional | | | |
| Restore from below-80×24: exact context/focus/scroll restoration | | | |
| Above-minimum resize back up: exact restoration | | | |

Fields to record per release:

- Release / version: ____________  Date: ____________
- Platform: Linux / macOS  Terminal: ____________
- Tester: ____________  Result: PASS / FAIL (attach notes)

## References

- `scripts/capability-suite.sh` — the canonical targeted race/capability command (Issue #56, retained).
- `.github/workflows/capability-suite.yml` — both platform jobs with the full seven-step gate (Issue #88).
- `cmd/sqloid/pty_integration_test.go` — the Issue #57 PTY-driven built-binary integration test, invoked by `go test ./...`.
- [production-tui-composition.md](production-tui-composition.md) — the Issue #57 composition root and PTY integration test the expanded gate exercises.
- [concurrent-page-count.md](concurrent-page-count.md), [connection-pool.md](connection-pool.md), [scoped-select-cancellation.md](scoped-select-cancellation.md), [cancellation-infrastructure.md](cancellation-infrastructure.md), [select-request-identities.md](select-request-identities.md), [health-terminal.md](health-terminal.md), [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md), [outcome-unknown-terminal.md](outcome-unknown-terminal.md), [schema-validation-workflow.md](schema-validation-workflow.md), [destructive-preparation.md](destructive-preparation.md), [transactional-writes.md](transactional-writes.md), [unit-tests.md](unit-tests.md).
- Issues #5–#7, #21, #24, #28–#29, #32, #41–#43, #55–#57, #81–#88; the Language and stack, Connection pool, Session health, History, Module Design, Testing Decisions, and Acceptance Criteria sections of `Notes/PRD-sqloid.md`.
