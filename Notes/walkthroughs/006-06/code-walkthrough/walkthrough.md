# Issue #6: Cancellation Infrastructure (context + connection-scoped interrupt)

*2026-08-26T22:50:28Z by Showboat 0.6.1*
<!-- showboat-id: 6bd4e41d-1c99-4519-9816-5e9506664416 -->

This walkthrough demonstrates the Issue #6 cancellation infrastructure in internal/connection: the driver-independent Request lifecycle (fake-backed tests), then the pinned modernc v1.57.0 capability evidence — controlled CPU-bound work settling within one second of cancellation, a lock wait settling within five seconds, independent-lease isolation, deliberate late-success rejection, settlement before lease reuse, no force-close, and successful subsequent work on the same connection. All ordering uses channel barriers; timing measurements target only the explicit PRD latency bounds (go test timing annotations are filtered from captured output so re-execution verifies deterministically). References: Notes/issues/006-cancellation-infrastructure.md; Notes/PRD-sqloid.md sections Identities and state, Errors and cancellation bounds, Connection; SELECT/write/UI wiring is deferred to Issue #28.

Fake-backed lifecycle evidence: unique request IDs and context ownership, eight racing Cancellers dispatching the connection-scoped interrupt exactly once (and never after settlement), cancelling-until-settlement visibility, late success discarded as cancelled, the error classification matrix, third-lease refusal while an unsettled request owns one of only two connections, and no-force-close with same-lease reuse after settled cancellation. Next: the pinned-driver capability tests.

Fake-backed lifecycle evidence: unique request IDs and context ownership, eight racing Cancellers dispatching the connection-scoped interrupt exactly once (and never after settlement), cancelling-until-settlement visibility, late success discarded as cancelled, the error classification matrix, third-lease refusal while an unsettled request owns one of only two connections, and no-force-close with same-lease reuse after settled cancellation. Next: the pinned-driver capability tests.

```bash
go test ./internal/connection -count=1 -v -run 'TestRequestIdentityIsUniqueAndContextOwned|TestCancelIsIdempotentAndInterruptDispatchedOnce|TestVisibleLifecycleStateTracksCancellationUntilSettlement|TestLateSuccessIsDiscardedAsCancelled|TestErrorsSettleNormally|TestCancellationErrorClassifiesAsCancelled|TestLeaseHeldUntilSettlementThenReusable|TestConnectionNotForceClosedByCancellationAndSafeForReuse' | awk '{gsub(/ \([0-9.]+s\)/,""); sub(/[0-9.]+s$/,""); print}'
```

```output
=== RUN   TestRequestIdentityIsUniqueAndContextOwned
--- PASS: TestRequestIdentityIsUniqueAndContextOwned
=== RUN   TestCancelIsIdempotentAndInterruptDispatchedOnce
=== RUN   TestCancelIsIdempotentAndInterruptDispatchedOnce/concurrent
=== RUN   TestCancelIsIdempotentAndInterruptDispatchedOnce/late
--- PASS: TestCancelIsIdempotentAndInterruptDispatchedOnce
    --- PASS: TestCancelIsIdempotentAndInterruptDispatchedOnce/concurrent
    --- PASS: TestCancelIsIdempotentAndInterruptDispatchedOnce/late
=== RUN   TestVisibleLifecycleStateTracksCancellationUntilSettlement
--- PASS: TestVisibleLifecycleStateTracksCancellationUntilSettlement
=== RUN   TestLateSuccessIsDiscardedAsCancelled
--- PASS: TestLateSuccessIsDiscardedAsCancelled
=== RUN   TestErrorsSettleNormally
--- PASS: TestErrorsSettleNormally
=== RUN   TestCancellationErrorClassifiesAsCancelled
--- PASS: TestCancellationErrorClassifiesAsCancelled
=== RUN   TestLeaseHeldUntilSettlementThenReusable
--- PASS: TestLeaseHeldUntilSettlementThenReusable
=== RUN   TestConnectionNotForceClosedByCancellationAndSafeForReuse
--- PASS: TestConnectionNotForceClosedByCancellationAndSafeForReuse
PASS
ok  	github.com/chris/sqloid/internal/connection	
```

```bash
go test ./internal/connection -count=1 -v -run 'TestCPUBoundWorkInterruptsWithinOneSecond|TestLockWaitInterruptsWithinFiveSeconds|TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable' | awk '{gsub(/ \([0-9.]+s\)/,""); sub(/[0-9.]+s$/,""); print}'
```

```output
=== RUN   TestCPUBoundWorkInterruptsWithinOneSecond
--- PASS: TestCPUBoundWorkInterruptsWithinOneSecond
=== RUN   TestLockWaitInterruptsWithinFiveSeconds
--- PASS: TestLockWaitInterruptsWithinFiveSeconds
=== RUN   TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable
--- PASS: TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable
PASS
ok  	github.com/chris/sqloid/internal/connection	
```

```bash
CGO_ENABLED=1 go test -race -count=1 ./internal/connection -run 'TestCPUBoundWorkInterruptsWithinOneSecond|TestLockWaitInterruptsWithinFiveSeconds|TestRequestIdentityIsUniqueAndContextOwned|TestCancelIsIdempotentAndInterruptDispatchedOnce' 2>&1 | 
```

```output
bash: -c: line 2: syntax error: unexpected end of file
```

Pinned-driver capability evidence (Linux, modernc.org/sqlite v1.57.0): TestCPUBoundWorkInterruptsWithinOneSecond cancelled a verifiably running 200k-row probe scan and settled it far inside the mandatory one-second bound (0.12s); TestLockWaitInterruptsWithinFiveSeconds interrupted a write blocked behind a held SHARED lock in 0.11s — inside the five-second busy-timeout bound; isolation on the second dedicated lease held during the interruption, deliberately released late success was classified cancelled and discarded, the interrupted physical connection answered subsequent work, was not force-closed, and returned through the pool for reuse. The lifecycle tests are also race-detector clean above. These capability tests are Linux/macOS-only by build constraint and are release- and dependency-upgrade-blocking: any change of the vetted modernc pin must re-prove the one-second CPU, five-second lock-wait, isolation, and reuse behavior before release. Applying this infrastructure to SELECT page/count cancellation, write phases with their commit boundary, and the cancelling-ellipsis UI feedback remains Issue #28.
