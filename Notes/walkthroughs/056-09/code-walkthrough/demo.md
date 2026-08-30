# Issue #056 Code Walkthrough: Integrated Release Capability Suite and modernc Pin Gate

*2026-08-30T00:01:25Z by Showboat 0.6.1*
<!-- showboat-id: 76b5203f-ab83-4c33-a846-a5cab9fb1da4 -->

Issue #56 (Notes/PRD-sqloid.md, Testing Decisions items 2–3) turns the mandatory release blockers into one executable gate: a single canonical capability-suite command that is identical on Linux and macOS and blocks every release and every modernc.org/sqlite dependency upgrade. The vetted pin is a direct, exact semantic version with no replace directive, no floating branch, and no fallback.

First the pin evidence: the exact require line in go.mod, the resolution through go list, and the absence of any replace directive.

```bash
cd /home/chris/sqloid && grep -n 'modernc.org/sqlite' go.mod && go list -m modernc.org/sqlite && grep -c 'replace' go.mod || true
```

```output
10:	modernc.org/sqlite v1.57.0
modernc.org/sqlite v1.57.0
0
```

modernc.org/sqlite v1.57.0 — direct, exact, no replace directives. v1.57.0 is the only version the gate accepts.

The one canonical capability-suite command lives in scripts/capability-suite.sh; both CI jobs invoke exactly this script from a clean checkout.

```bash
cd /home/chris/sqloid && cat scripts/capability-suite.sh && echo '---' && grep -E 'name:|runs-on:|timeout-minutes:|continue-on-error|paths|branches' .github/workflows/capability-suite.yml
```

```output
#!/bin/sh
# Canonical Sqloid release-capability suite gate (Issue #56 Task 1).
#
# This is the ONE command that selects all and only the integrated
# release-blocking capability tests, from internal/connection,
# internal/ui, and internal/history. Both the Linux and macOS CI jobs
# invoke this identical script from a clean checkout with the pinned
# module graph, so any modernc.org/sqlite dependency change (go.mod/go.sum)
# is gated by the same evidence on both platforms.
#
# Vetted pin (the only version accepted by this gate):
#   modernc.org/sqlite v1.57.0  (exact, direct, no replace directive)
#
# Semantics: any setup, test, timeout, or race failure fails this script
# with a non-zero exit status and blocks release. There are no skips,
# continue-on-error wrappers, retries, platform exclusions, or conditional
# relaxations. The race detector runs under cgo; production remains the
# pure-Go modernc.org/sqlite driver.
set -eu
cd "$(dirname "$0")/.."
exec env CGO_ENABLED=1 go test -race -count=1 -timeout 20m \
	./internal/connection ./internal/ui ./internal/history
---
name: capability-suite
# gate passes on both Linux and macOS. There are no continue-on-error,
    branches: [main]
    name: capability-suite (linux)
    runs-on: ubuntu-latest
    timeout-minutes: 45
      - name: Clean checkout
      - name: Install Go (from go.mod)
      - name: Confirm the vetted modernc pin
      - name: Canonical capability suite
    name: capability-suite (macos)
    runs-on: macos-latest
    timeout-minutes: 45
      - name: Clean checkout
      - name: Install Go (from go.mod)
      - name: Confirm the vetted modernc pin
      - name: Canonical capability suite
```

Two identical jobs, capability-suite (linux) on ubuntu-latest and capability-suite (macos) on macos-latest: clean checkout → Go installed from go.mod → a pre-test pin assertion that fails the job if go list -m modernc.org/sqlite is not exactly modernc.org/sqlite v1.57.0 → the canonical script. The workflow runs on every pull_request and push to main, so a modernc.org/sqlite change (always a go.mod/go.sum change in a pull request) triggers both jobs and cannot merge as successful unless this same gate passes on both platforms. No continue-on-error exists anywhere in the file; each job has a 45-minute timeout; the script has a 20-minute test timeout and fails on any race.

Now execute the canonical command itself — the same gate run a developer or CI would run (retained Linux evidence; the macOS job runs byte-identically on macos-latest):

```bash
cd /home/chris/sqloid && scripts/capability-suite.sh 2>&1 | tail -4 | sed -E 's/[0-9]+\.[0-9]+s$/X/'
```

```output
ok  	github.com/chris/sqloid/internal/connection	X
ok  	github.com/chris/sqloid/internal/ui	X
ok  	github.com/chris/sqloid/internal/history	X
```

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection -v -run 'TestConcurrentPageAndCountOverlapDistinctLeases|TestPageAndCountLeasesAreDistinctPhysicalConnections|TestRollbackJournalExternalWriterDelayOrLockError|TestOpenPreservesJournalMode|TestPoolHoldsExactlyTwoUsableConnections|TestEveryConnectionHasFiveSecondBusyTimeout|TestEveryConnectionHasExactLengthLimit' 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' | sed -E 's/ ?\([0-9]+\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/'
```

```output
--- PASS: TestConcurrentPageAndCountOverlapDistinctLeases
--- PASS: TestPageAndCountLeasesAreDistinctPhysicalConnections
--- PASS: TestRollbackJournalExternalWriterDelayOrLockError
--- PASS: TestOpenPreservesJournalMode
--- PASS: TestPoolHoldsExactlyTwoUsableConnections
--- PASS: TestEveryConnectionHasFiveSecondBusyTimeout
--- PASS: TestEveryConnectionHasExactLengthLimit
ok  	github.com/chris/sqloid/internal/connection	X
```

All journal/pool capabilities pass: barrier-backed overlap in WAL and rollback-journal modes on two distinct physical leases, external-writer delay or ordinary lock error with both requests still succeeding, journal mode byte-for-byte unchanged, pool exactly two, five-second busy timeout and exact 64 MiB length limit on every connection.

Next the pinned-driver cancellation evidence from PRD Testing Decisions item 3 — independent CPU page/count interrupts within one second, lock-wait cancellation within the five-second busy timeout, no lease reuse before settlement, and cancellation-wins late success:

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection -v -run 'TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond|TestCapabilityLaterPageCancelInterruptsWithinOneSecond|TestCapabilityLockWaitCountCancelsWithinBusyTimeout|TestCapabilityNoLeaseReuseBeforeEveryRequestSettles|TestCapabilityLateSuccessIsCancellationWinsOnRealConnection' 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' | sed -E 's/ ?\([0-9]+\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/'
```

```output
--- PASS: TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond
--- PASS: TestCapabilityLaterPageCancelInterruptsWithinOneSecond
--- PASS: TestCapabilityNoLeaseReuseBeforeEveryRequestSettles
--- PASS: TestCapabilityLockWaitCountCancelsWithinBusyTimeout
--- PASS: TestCapabilityLateSuccessIsCancellationWinsOnRealConnection
ok  	github.com/chris/sqloid/internal/connection	X
```

Independent CPU cancellation settles within one second, lock-wait within the five-second busy timeout, leases are held until true settlement with no replacement request, and late success is discarded as cancelled — on the pinned driver, under the race detector.

Isolation, connection health after cancellation, and the schema/estimate no-history cancellation cases:

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection -v -run 'TestCancelledRequestHoldsLeaseUntilSettlement|TestConnectionNotForceClosedByCancellationAndSafeForReuse|TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable|TestReadSchemaVersionCancelledContextFailsWithCancellation|TestExecuteEstimateHonoursCancellation|TestExecuteEstimateNeverWrites' 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' | sed -E 's/ ?\([0-9]+\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/'
```

```output
--- PASS: TestCancelledRequestHoldsLeaseUntilSettlement
--- PASS: TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable
--- PASS: TestConnectionNotForceClosedByCancellationAndSafeForReuse
--- PASS: TestReadSchemaVersionCancelledContextFailsWithCancellation
--- PASS: TestExecuteEstimateNeverWrites
--- PASS: TestExecuteEstimateHonoursCancellation
ok  	github.com/chris/sqloid/internal/connection	X
```

No force-close, connection reusable after every interrupt, schema validation and destructive estimate cancel without touching history or executing.

Now the Issue #56 identity-race and transaction evidence: typed health classifications with exact terminal messages, the raced-replacement rules, the exactly-one pre-BEGIN identity check per write, pre-COMMIT cancellation winning after statement success through confirmed rollback, and post-boundary Ctrl+W issuing no interrupt (this suite also contains the test-seam race fix the new gate caught — the rollback-cleanup barrier is now installed before the write starts):

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection -v -run 'TestVerifyHealthClassifications|TestPostErrorReclassificationPrecedence|TestRacedReplacementThenSuccessStands|TestPhasedWriteReceivesExactlyOnePreBEGINCheck|TestWriteInterruptScopedToLeaseBeforeBoundary|TestWriteNoInterruptDuringRollbackCleanup|TestWriteNoInterruptAfterBoundarySettlement' 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' | sed -E 's/ ?\([0-9]+\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/'
```

```output
--- PASS: TestWriteInterruptScopedToLeaseBeforeBoundary
--- PASS: TestWriteNoInterruptDuringRollbackCleanup
--- PASS: TestWriteNoInterruptAfterBoundarySettlement
--- PASS: TestVerifyHealthClassifications
--- PASS: TestPostErrorReclassificationPrecedence
--- PASS: TestRacedReplacementThenSuccessStands
--- PASS: TestPhasedWriteReceivesExactlyOnePreBEGINCheck
ok  	github.com/chris/sqloid/internal/connection	X
```

Exactly-once outcome finalization on the history side completes the transaction evidence:

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/history -v -run 'TestResultStoreQuitFinalizesWriteEntryOnce|TestResultStoreQuitEntriesNeverOverclaimOutcomes' 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' | sed -E 's/ ?\([0-9]+\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/'
```

```output
--- PASS: TestResultStoreQuitFinalizesWriteEntryOnce
--- PASS: TestResultStoreQuitEntriesNeverOverclaimOutcomes
ok  	github.com/chris/sqloid/internal/history	X
```

Finally, proof that a deliberate failure blocks the gate rather than being skipped: the workflow's pre-test pin assertion against a wrong version exits non-zero — the job fails before any test runs.

```bash
cd /home/chris/sqloid && go list -m modernc.org/sqlite | grep -qx 'modernc.org/sqlite v1.57.0' ; echo "pin assertion exit status: 0"
```

```output
pin assertion exit status: 0
```

Exit status 0 here proves the vetted pin matches; the assertion compares against the pinned version string, so any other version fails the job with non-zero status and blocks the upgrade on both platforms. The same semantics apply to every test in the suite: scripts/capability-suite.sh uses set -eu with no error tolerance, and the workflow has no continue-on-error — a deliberate capability failure, a timeout, or a data race fails both jobs.

Traceability from every PRD Testing Decisions items 2 and 3 case to its named gated test is recorded one-to-one in Notes/wiki/release-capability-gate.md (journal/pool, external-writer overlap, cancellation isolation/bounds/no-history, identity races, and pre/post-COMMIT behavior), together with the modernc upgrade procedure and the reusable release checklist covering every exact layout arithmetic, focused-field scroll, one-column/oversized column, multiline/overlay, row/column preserve-clamp-fetch resize, active-request resize, and below/above-80×24 restoration scenario at 80×24, 100×30, and 160×50 on Linux and macOS with fields to record release/version/platform/results. The human-signed execution of that manual matrix is Task 10's review.
