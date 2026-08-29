# Issue #28 — Scoped Ctrl+W SELECT cancellation and bounded settlement code walkthrough

*2026-08-28T15:01:13Z by Showboat 0.6.1*
<!-- showboat-id: 88295a4c-494c-4aa1-90f7-939cdd934ba9 -->

Issue #28 applies Issue #6's cancellable request infrastructure to an active SELECT (Notes/PRD-sqloid.md, Identities and state; Errors and cancellation bounds; Global Key Precedence; SELECT lifecycle): Ctrl+W requests one connection-scoped interrupt for each currently active first-page, later-page, or count request through independent identities, holds the exact `cancelling…` feedback until every targeted request truly settles, dispatches no replacement work before that settlement, rejects cancellation-classified late successes through Issue #26's identity guards, and never force-closes a connection. Ownership split: schema validation cancellation is Issue #21, estimate cancellation Issue #41, beginning/executing write cancellation Issue #42, and post-commit-boundary behavior Issue #43.
```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestStartedPageAndCountCancelIndependentlyAndSettleAll|TestStartedRequestCancellationIsIdempotentAndSingleSettled|TestCountOnlyCancellationLeavesUnrelatedWorkAlone' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g"
```

```output
=== RUN   TestStartedPageAndCountCancelIndependentlyAndSettleAll
--- PASS: TestStartedPageAndCountCancelIndependentlyAndSettleAll (NT)
=== RUN   TestStartedRequestCancellationIsIdempotentAndSingleSettled
--- PASS: TestStartedRequestCancellationIsIdempotentAndSingleSettled (NT)
=== RUN   TestCountOnlyCancellationLeavesUnrelatedWorkAlone
--- PASS: TestCountOnlyCancellationLeavesUnrelatedWorkAlone (NT)
PASS
ok  	github.com/chris/sqloid/internal/connection	NT
```

The connection-layer handles prove the settlement contract deterministically: page and count run on distinct leased connections with distinct interrupt identities, cancelling only the page leaves the count's context untouched while its lease stays owned, every cancelled request settles only through its own lifecycle, repeated and late Cancel are idempotent with exactly one settled classification, and the pool serves healthy work immediately after each settlement.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestCtrlWCancelsFirstPageAndCountUntilAllSettle|TestCtrlWCancelsCountOnlyScope|TestCtrlWCancelsLaterPageOnlyScope' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g"
```

```output
=== RUN   TestCtrlWCancelsFirstPageAndCountUntilAllSettle
--- PASS: TestCtrlWCancelsFirstPageAndCountUntilAllSettle (NT)
=== RUN   TestCtrlWCancelsCountOnlyScope
--- PASS: TestCtrlWCancelsCountOnlyScope (NT)
=== RUN   TestCtrlWCancelsLaterPageOnlyScope
--- PASS: TestCtrlWCancelsLaterPageOnlyScope (NT)
PASS
ok  	github.com/chris/sqloid/internal/ui	NT
```

The scripted model tests cover all three Ctrl+W scopes. First-page+count: both active requests receive independent cancellations, `cancelling…` stays visible while either request is unsettled, Enter and Page Down are consumed with no replacement dispatch, repeat Ctrl+W is idempotent, the count settles first with its role inert while the page is still pending, the settled page is classified cancelled and stores no rows, and a healthy replacement execution is accepted afterwards. Count-only and later-page-only scopes cancel exactly the one active request, keep the displayed rows and range metadata untouched, and reopen the gate at settlement.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok)' | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g"
```

```output
=== RUN   TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond
--- PASS: TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond (NT)
ok  	github.com/chris/sqloid/internal/connection	NT
```

Linux/macOS release requirement: the capability suite in internal/connection/select_cancellation_capability_test.go (//go:build unix) is mandatory and release-blocking — a pinned-driver failure on the one-second CPU bound, the five-second lock bound, isolation, late-result, or reuse evidence must be fixed in the pin or integration, never skipped. The exact Issue #6 modernc capability seam is reused: real page and count statements generated through internal/querybuilder run CPU-bound via virtual generated columns invoking registered scalar probes, with barrier signals proving each query verifiably started before cancellation. The captured run above proves the concurrent page+count case: both queries run on distinct leased connections, only the page is cancelled first and settles under one second while the count keeps running on its own identity, the count then settles under the same bound, and both connections serve harmless subsequent work afterwards (no force-close). TestCapabilityLaterPageCancelInterruptsWithinOneSecond proves the StartPage later-page path under the same one-second bound, and TestCapabilityNoLeaseReuseBeforeEveryRequestSettles proves deterministically that third-lease acquisition fails while both cancelled requests are unsettled and each lease returns only at its own settlement.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestCapabilityLockWaitCountCancelsWithinBusyTimeout|TestCapabilityLateSuccessIsCancellationWinsOnRealConnection|TestCapabilityLaterPageCancelInterruptsWithinOneSecond' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok)' | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g"
```

```output
=== RUN   TestCapabilityLaterPageCancelInterruptsWithinOneSecond
--- PASS: TestCapabilityLaterPageCancelInterruptsWithinOneSecond (NT)
=== RUN   TestCapabilityLockWaitCountCancelsWithinBusyTimeout
--- PASS: TestCapabilityLockWaitCountCancelsWithinBusyTimeout (NT)
=== RUN   TestCapabilityLateSuccessIsCancellationWinsOnRealConnection
--- PASS: TestCapabilityLateSuccessIsCancellationWinsOnRealConnection (NT)
ok  	github.com/chris/sqloid/internal/connection	NT
```

Bounds evidence: the controlled CPU-bound page, later page, and count scans each settle within the mandatory one second of cancellation (interrupt aborts the statement mid-execution). For the lock-wait bound, an EXCLUSIVE lock holder plus a 50 ms canary write prove the count is verifiably blocked before cancellation; settlement lands within the five-second busy-timeout window measured from contention start. Documented pinned-driver reality for modernc v1.57.0: sqlite3_interrupt does not preempt a busy-handler wait, so the blocked read settles at the configured five-second expiry with SQLITE_BUSY, which classifies as failed with the busy cause preserved per the Issue #6 rules — the PRD bound 'no later than the five-second busy timeout' still holds, and the late-success test proves a real count completing before cancellation but released after it is discarded as cancelled with no total leaked. Journal mode is never set or changed by any of these tests.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ ./internal/ui/ -run 'TestCapabilityNoLeaseReuseBeforeEveryRequestSettles|TestCtrlWCancelsCountOnlyScope' 2>&1 | tail -2 | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g" && CGO_ENABLED=1 go test -race ./internal/ui/ -run 'TestCtrlW' -count=1 2>&1 | tail -1 | sed -E "s/[0-9]+(\.[0-9]+)?s\b/NT/g"
```

```output
ok  	github.com/chris/sqloid/internal/connection	NT
ok  	github.com/chris/sqloid/internal/ui	NT
ok  	github.com/chris/sqloid/internal/ui	NT
```

Summary: scoped Ctrl+W cancels exactly the active first-page, later-page, and count requests through independent connection-scoped interrupts; cancelling… persists until every targeted request settles; no replacement or lease reuse happens before settlement; late successes are inert through cancellation-wins classification; connections are never force-closed and healthy subsequent work runs on each affected pool. Isolation is proven both at the fake level (count-only and later-page-only scopes) and against the real driver (page cancellation leaving the count verifiably running). The CPU (one second) and lock-wait (five-second busy timeout) bounds are release-blocking on Linux/macOS, with the ownership split keeping schema-validation cancellation (Issue #21), estimate cancellation (Issue #41), write-execution cancellation (Issue #42), and post-commit behavior (Issue #43) outside Issue #28. Cross-reference: Notes/PRD-sqloid.md (Identities and state; SELECT; Errors and cancellation bounds; Global Key Precedence and Context/Action Matrix) and the wiki page scoped-select-cancellation.md.
