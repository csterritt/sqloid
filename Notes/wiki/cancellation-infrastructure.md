# Cancellation Infrastructure: Requests, Contexts, and Connection-Scoped Interrupts

How `internal/connection` runs every database request as a cancellable, identity-bearing unit on a dedicated leased connection, with settlement detection, cancellation-wins classification, and no force-close (Issue #6; `Notes/PRD-sqloid.md` §Identities and state and §Errors and cancellation bounds). Leasing itself is covered by [connection-pool.md](connection-pool.md).

## Scope vs. later wiring

Issue #6 is the reusable infrastructure layer only: request identity, cancellable context ownership, interrupt dispatch, settlement, late-success classification, no force-close, and pinned-driver capability evidence. SELECT page/count cancellation is Issue #28 (documented in [scoped-select-cancellation.md](scoped-select-cancellation.md)), which consumes these primitives unchanged; pre-lease cancellation — when the context is cancelled while the request is queued for a lease before any work starts — is Issue #60 (documented in [prelease-cancellation.md](prelease-cancellation.md)); write-phase cancellation with its commit boundary is deferred to Issues #42/#43, and history effects/quit integration to their owning issues.

## Request lifecycle

`Lease.BeginRequest(parent) *Request` starts a request that exclusively owns its lease until `Close`. The lifecycle is exactly `BeginRequest → Cancel (at most once, from any goroutine) → Settle (exactly once) → Close (idempotent)`:

- **Identity** — each `Request` gets a process-unique increasing ID from an atomic counter; IDs are never reused, so a stale response can never be confused with a later request's.
- **Context ownership** — the request owns a derived cancellable context (`Request.Context()`); all work must run through it so cancellation reaches the driver as an interrupt. Cancelling one request never touches another's context; cancelling the parent propagates into the request.
- **Cancel** — idempotent. The first call sets an atomic-under-mutex cancellation flag, cancels the derived context exactly once, and dispatches the connection-scoped interrupt hook at most once. Late Cancell after settlement dispatch nothing.
- **Visible state** — `State()` is `running`, then `cancelling` from Cancel until `Settle` records settlement — even after the in-flight work has internally finished but before Settle is called. This is what Issue #28 renders as `cancelling…`. `Settled()` reports terminal state independently.
- **Settle** — records the work's final error and classifies exactly once:
  - success arriving after cancellation was requested is **discarded and classified as cancelled** (cancellation wins);
  - `context.Canceled` errors classify as cancelled;
  - any other error classifies as failed;
  - uncancelled success classifies as success;
  - a second Settle returns the stored outcome unchanged.
- **Close** — releases the lease (release occurs only after settlement) and is safe to repeat; the first release error is retained. `Request.Run(op)` is the synchronous convenience form.

## Interrupt scope and bounds

With the pinned modernc driver there is no separate public API to invoke: cancelling the request context makes the driver issue `sqlite3_interrupt` against **exactly the leased physical connection** (its per-statement `interruptOnDone` machinery). Because leases are exclusive (see [connection-pool.md](connection-pool.md)), the interrupt can never touch concurrent work on the other pooled connection. `Lease.interruptFn` is a narrow seam: production leaves it nil (context-driven interrupt); tests install fake hooks to observe or replace dispatch without a live driver.

Mandatory capability bounds proven by barrier-based tests on Linux/macOS against modernc v1.57.0:

| Scenario | Bound | Mechanism |
| --- | --- | --- |
| Controlled CPU-bound query | settles < 1 s of cancellation | context cancel → driver interrupt aborts statement mid-execution |
| Lock wait behind another connection's SHARED lock | settles ≤ 5 s | busy handler retry aborted immediately by interrupt, well within the five-second `_busy_timeout` |

Isolation (independent work on the second lease completes while the first is interrupted), deliberate late-success rejection, absence of force-close, and subsequent harmless work on the interrupted physical connection are all asserted. Tests synchronize through channels and registered scalar-function probes ("work has begun" barriers); timing assertions only measure the explicit PRD latency bounds.

## Guarantees

- Cancellation never force-closes a connection: interruption ends the statement, not the connection, which answers further queries and returns to the pool for reuse.
- No replacement work can acquire a cancelled request's lease before settlement: the lease is owned exclusively until `Close`, and a third `DB.Lease` still blocks while two leases are held.
- These tests are release- and dependency-upgrade-blocking: changing the modernc pin must re-prove CPU (< 1 s), lock-wait (≤ 5 s), isolation, and reuse behavior on Linux/macOS.

## Verification

- `internal/connection/request_test.go` (fake-backed, driver-independent)
  - `TestRequestIdentityIsUniqueAndContextOwned` — unique non-zero IDs, per-request contexts, parent propagation.
  - `TestCancelIsIdempotentAndInterruptDispatchedOnce/{concurrent,late}` — eight racing Cancellers and post-settlement Cancell dispatch the interrupt exactly once.
  - `TestVisibleLifecycleStateTracksCancellationUntilSettlement` — running → cancelling at Cancel; still cancelling after internal work completion; settled only at Settle.
  - `TestLateSuccessIsDiscardedAsCancelled` / `TestErrorsSettleNormally` / `TestCancellationErrorClassifiesAsCancelled` — the full outcome matrix including settle-idempotence.
  - `TestLeaseHeldUntilSettlementThenReusable` — third-lease acquisition fails fast while an unsettled request holds one of only two connections; reuse succeeds after Close.
  - `TestConnectionNotForceClosedByCancellationAndSafeForReuse` — real pool: same lease usable immediately after settled cancellation, release-after-settlement ordering, subsequent work unaffected.
- `internal/connection/interrupt_unix_test.go` (`//go:build unix`; Linux/macOS mandatory, release-blocking)
  - `TestCPUBoundWorkInterruptsWithinOneSecond` — 200k-row scan through an expensive registered scalar probe; started-barrier then Cancel proves settlement under 1 s; concurrent independent-lease work completes; same-lease PRAGMA plus re-leasing prove no force-close and reuse.
  - `TestLockWaitInterruptsWithinFiveSeconds` — unconsumed rows cursor keeps SHARED held while the target's INSERT commits toward EXCLUSIVE; probe barrier then Cancel proves settlement ≤ 5 s and reuse afterward.
  - `TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable` — deliberately released success after Cancel classifies as cancelled and the connection remains usable.
- Race-detector runs pass over both suites (`CGO_ENABLED=1 go test -race ./internal/connection/`).

Cross-references: [scoped-select-cancellation.md](scoped-select-cancellation.md), [connection-pool.md](connection-pool.md), [sqlite-startup.md](sqlite-startup.md), [source-code.md](source-code.md), [unit-tests.md](unit-tests.md).
