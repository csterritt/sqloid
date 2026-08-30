# Issue #26 — SELECT request identities and stale-response rejection

*2026-08-28T01:42:50Z by Showboat 0.6.1*
<!-- showboat-id: 96e2a125-0995-4ca9-8510-fd2e1d76f5ef -->

Issue #26 completes the SELECT request-identity system: every first-page and later-page request captures its immutable execution ID, request ID, and viewport generation at dispatch, and a response may mutate rows, range, loading state, or retained cache only when every applicable identity is still current. It also proves cancellation-wins late-success classification and that replacement work cannot start or reuse a lease before every replaced predecessor settles — without serializing the normal first-page/count launch. Per Notes/tasks/026-select-request-identities-and-stale-response-rejection.md and Notes/PRD-sqloid.md (Identities and state, SELECT, Errors and cancellation bounds, Testing Decisions); cross-references Issues #24, #25, and #28. Every step re-runs the real repository tests as evidence.

**1. Out-of-order first-page and later-page responses across execution IDs, request IDs, and viewport generations are rejected.** Barrier-held messages (never sleeps) deliver successes and failures out of order while independently varying all three identities: a superseded execution's responses, a replaced request within the same execution, resize generation advancement, and SELECT deactivation/finalization all leave rows, range, cache metadata, and newer feedback untouched, while the fully current control case applies exactly once.

```bash
go test ./internal/ui -count=1 -run 'TestCurrentFirstPageControlApplies|TestFirstPageRejectedAfterResizeGenerationAdvance|TestFirstPageRejectedAfterDeactivationFinalization|TestStaleLaterPageRejectedAfterResizeGenerationAdvance|TestStaleLaterPageRejectedAfterExecutionSuperseded|TestStaleLaterPageRejectedAfterDeactivation|TestStaleResponseCannotClearNewerRequestFeedback|TestStalePageFailureCannotClearNewerRequestFeedback' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCurrentFirstPageControlApplies
--- PASS: TestCurrentFirstPageControlApplies ((t))
=== RUN   TestFirstPageRejectedAfterResizeGenerationAdvance
--- PASS: TestFirstPageRejectedAfterResizeGenerationAdvance ((t))
=== RUN   TestFirstPageRejectedAfterDeactivationFinalization
--- PASS: TestFirstPageRejectedAfterDeactivationFinalization ((t))
=== RUN   TestStaleLaterPageRejectedAfterResizeGenerationAdvance
--- PASS: TestStaleLaterPageRejectedAfterResizeGenerationAdvance ((t))
=== RUN   TestStaleLaterPageRejectedAfterExecutionSuperseded
--- PASS: TestStaleLaterPageRejectedAfterExecutionSuperseded ((t))
=== RUN   TestStaleLaterPageRejectedAfterDeactivation
--- PASS: TestStaleLaterPageRejectedAfterDeactivation ((t))
=== RUN   TestStaleResponseCannotClearNewerRequestFeedback
--- PASS: TestStaleResponseCannotClearNewerRequestFeedback ((t))
=== RUN   TestStalePageFailureCannotClearNewerRequestFeedback
--- PASS: TestStalePageFailureCannotClearNewerRequestFeedback ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**2. Cancellation wins over late success, on every role.** A page request classified cancelled by the boundary leaves rows, range, and cache metadata unchanged, settles its pending guard, and cannot clear state belonging to a newer execution — for first pages, later pages, and counts alike, with late ordinary errors equally inert after a newer execution begins.

```bash
go test ./internal/ui -count=1 -run 'TestCancelledLaterPageWinsOverLateSuccess|TestCancelledFirstPageNeverMutatesRows|TestCancelledCountStaysInert|TestLateCancelledResponseAfterNewerExecutionStaysInert|TestLateErrorAfterNewerExecutionStaysInert' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCancelledLaterPageWinsOverLateSuccess
--- PASS: TestCancelledLaterPageWinsOverLateSuccess ((t))
=== RUN   TestCancelledFirstPageNeverMutatesRows
--- PASS: TestCancelledFirstPageNeverMutatesRows ((t))
=== RUN   TestCancelledCountStaysInert
--- PASS: TestCancelledCountStaysInert ((t))
=== RUN   TestLateCancelledResponseAfterNewerExecutionStaysInert
--- PASS: TestLateCancelledResponseAfterNewerExecutionStaysInert ((t))
=== RUN   TestLateErrorAfterNewerExecutionStaysInert
--- PASS: TestLateErrorAfterNewerExecutionStaysInert ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**3. Replacement waits for predecessor settlement — page/count concurrency preserved.** No replacement page command dispatches while a cancelled or pending predecessor is unsettled; after settlement a replacement issues and applies. The count settles independently in either order without releasing the page's guard, and the Issue #24/#25 normal first-page/count concurrency suites still pass unchanged.

```bash
go test ./internal/ui -count=1 -run 'TestCountSettlesIndependentlyWhilePagePending|TestReplacementWaitsForCancelledPredecessorOnNewerExecution|TestConcurrentLaunchCarriesDistinctIdentities|TestCountArrivesBeforePage' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCountSettlesIndependentlyWhilePagePending
--- PASS: TestCountSettlesIndependentlyWhilePagePending ((t))
=== RUN   TestReplacementWaitsForCancelledPredecessorOnNewerExecution
--- PASS: TestReplacementWaitsForCancelledPredecessorOnNewerExecution ((t))
=== RUN   TestConcurrentLaunchCarriesDistinctIdentities
--- PASS: TestConcurrentLaunchCarriesDistinctIdentities ((t))
=== RUN   TestCountArrivesBeforePage
--- PASS: TestCountArrivesBeforePage ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**4. On the Connection boundary: a cancelled request holds its dedicated lease until true settlement.** With the exact-two pool exhausted, a cancelled-but-unsettled request blocks replacement lease acquisition; the late nil success classifies as OutcomeCancelled (never success); and only after Settle/Close does the lease return and the work become reusable. Page and count then settle independently in either order across distinct leases.

```bash
go test ./internal/connection -count=1 -run 'TestCancelledRequestHoldsLeaseUntilSettlement|TestPageAndCountSettleIndependentlyInEitherOrder' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCancelledRequestHoldsLeaseUntilSettlement
--- PASS: TestCancelledRequestHoldsLeaseUntilSettlement ((t))
=== RUN   TestPageAndCountSettleIndependentlyInEitherOrder
=== RUN   TestPageAndCountSettleIndependentlyInEitherOrder/page
=== RUN   TestPageAndCountSettleIndependentlyInEitherOrder/count
--- PASS: TestPageAndCountSettleIndependentlyInEitherOrder ((t))
    --- PASS: TestPageAndCountSettleIndependentlyInEitherOrder/page ((t))
    --- PASS: TestPageAndCountSettleIndependentlyInEitherOrder/count ((t))
PASS
ok  	github.com/chris/sqloid/internal/connection	(t)
```

**5. Full verification: the whole module builds, vets, and passes, including the race detector over the concurrency-relevant packages.**

```bash
gofmt -l cmd internal | grep . ; go vet ./... && CGO_ENABLED=1 go test -race ./internal/ui ./internal/connection ./internal/result -count=1 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
ok  	github.com/chris/sqloid/internal/ui	(t)
ok  	github.com/chris/sqloid/internal/connection	(t)
ok  	github.com/chris/sqloid/internal/result	(t)
```
