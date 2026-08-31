# Pre-Lease Cancellation Classification

How `internal/connection` classifies cancellation that arrives while a database request is queued for a lease — before any connection is acquired, before any operation callback runs, before BEGIN, before any statement, and before any transaction or phase work starts (Issue #60; `Notes/PRD-sqloid.md` §Identities and state, §Errors and cancellation bounds, §Connection/cancellation invariant; user stories 12, 14, 82). The request lifecycle, cancellation-wins late-success classification, and connection-scoped interrupt dispatch are Issue #6 infrastructure, documented in [cancellation-infrastructure.md](cancellation-infrastructure.md); the exact-two pool and dedicated leasing are Issue #5, documented in [connection-pool.md](connection-pool.md); typed health classification is Issue #7, documented in [session-health.md](session-health.md); started SELECT cancellation is Issue #28, documented in [scoped-select-cancellation.md](scoped-select-cancellation.md); transactional write cancellation and the commit boundary are Issues #42/#43, documented in [transactional-writes.md](transactional-writes.md) and [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md).

## Pool saturation and the pre-lease window

The exact-two pool ([connection-pool.md](connection-pool.md)) holds at most two dedicated connections. When both are leased by concurrent work (the first-page and count of one SELECT, or any two concurrent requests), a third `DB.Lease(ctx)` call blocks inside `database/sql`'s `Conn(ctx)` — it queues a `connRequest` and waits on a select for either a freed connection or `ctx.Done()`. Every database entry point in this package (`RunRequest`, `startRequest`-backed `StartFirstPage`/`StartPage`/`StartCount`, and `StartWrite`) acquires its lease through `DB.Lease(parent)` before any work begins, so a request that cannot immediately acquire a lease enters this queued wait with no operation callback, no BEGIN, no statement, no transaction hook, and no phase work started.

## Classification ordering

When `DB.Lease(parent)` returns an error, each entry point classifies it in this order:

1. **Cancellation precedence** — `errors.Is(err, context.Canceled)` is checked first. A wrapped `context.Canceled` (the `database/sql` pool returns `ctx.Err()` and `DB.Lease` wraps it through `fmt.Errorf("lease connection from pool: %w", err)`) classifies as the existing cancelled outcome: `OutcomeCancelled` for `RunRequest` and started SELECT requests, `WriteCancelled` for `StartWrite`. The cancellation cause is preserved in `Err` for callers to inspect.
2. **Health precedence** — `errors.As(err, &he)` is checked next. A typed `*HealthError` from `VerifyHealth` (deletion or same-path replacement detected at the request boundary) classifies as `OutcomeFailed` / `WriteFailed` with `Health` set. Health is never masked by cancellation: a genuine `HealthError` is not a `context.Canceled`, so the cancellation branch does not consume it.
3. **Ordinary failure** — every other lease error (configuration failure, `context.DeadlineExceeded`, driver error) classifies as `OutcomeFailed` / `WriteFailed` with `Err` carrying the cause. Non-cancellation errors are unchanged.

This ordering establishes cancellation precedence without masking health or altering non-cancellation error handling. The cancellation and health branches are mutually exclusive in practice — `DB.Lease` fails at either `db.SQL.Conn(ctx)` (context or pool error) or `VerifyHealth` (typed health error), never both — but the explicit ordering guarantees the contract even if a future error type wraps both.

## What pre-lease cancellation never starts

Because the lease is never acquired, pre-lease cancellation starts nothing:

- **No operation callback** — `RunRequest`'s `op` and the started-request operation functions are never invoked; the request never reaches `lease.BeginRequest` or `request.Run`.
- **No `Request` created** — `BeginRequest` is called only after a successful lease; a pre-lease failure returns before any `Request` is constructed, so no request identity, no derived context, and no interrupt dispatch exist.
- **No BEGIN, statement, or transaction hook** — `StartWrite`'s `run` goroutine is never started; `beforeWriteBegin`, `beforeWriteExec`, `beforeWriteCommit`, and `beforeWriteRollback` are never invoked.
- **No phase work** — `StartWrite` emits no `WritePhase` messages; the phase channel is closed empty at pre-settlement.
- **No replacement work before settlement** — while the request is queued both pool connections are held by existing work, so no third lease can be acquired. The cancelled request settles immediately when the context error is returned (the lease was never acquired, so there is nothing to release), and only after the existing holders are released can subsequent work proceed.
- **No interrupt dispatched** — no `Request.Cancel` is called and no `Lease.interruptFn` is invoked, because no `Request` or `Lease` exists.

## Exactly-once settlement

Each entry point pre-settles its result through the existing exactly-once channel before returning the handle:

- `RunRequest` returns `RequestResult` directly.
- `startRequest` sends the `RequestResult` on the `StartedRequest.done` channel (buffered 1) and closes `settled` before returning the handle; `Wait` receives it exactly once.
- `StartWrite` calls `deliver` which records the `WriteResult`, closes the phase channel, and closes `settled` before returning the handle; `Wait` reads `final` after `<-settled`.

No second settlement is possible: the handle is already settled when returned, and the work goroutine is never started.

## Pool usability after settlement

Pre-lease cancellation never touches a physical connection — no lease was acquired, no connection was checked out, and no interrupt was dispatched. After the cancelled request settles and the existing holders release their leases, both pool connections answer harmless work (e.g. `PRAGMA schema_version`) and a subsequent `RunRequest`, started SELECT, or `StartWrite` succeeds normally. The pool's physical connections are unaffected.

## Distinguishing the three failure shapes

| Failure shape | Error type | Classification | Health | Err |
| --- | --- | --- | --- | --- |
| Wrapped cancellation | `fmt.Errorf("lease connection from pool: %w", context.Canceled)` | `OutcomeCancelled` / `WriteCancelled` | nil | preserved `context.Canceled` cause |
| Typed health error | `*HealthError` from `VerifyHealth` | `OutcomeFailed` / `WriteFailed` | set (deleted or replaced) | the `*HealthError` itself |
| Ordinary lease failure | `fmt.Errorf("lease connection from pool: %w", context.DeadlineExceeded)` or configuration error | `OutcomeFailed` / `WriteFailed` | nil | the wrapped cause |

The cancellation branch uses `errors.Is` (matching the cause chain through `fmt.Errorf` wrapping); the health branch uses `errors.As` (matching the typed `*HealthError`). An ordinary `context.DeadlineExceeded` is not `context.Canceled`, so it falls through to the ordinary failure branch unchanged.

## Verification

- `internal/connection/prelease_cancellation_test.go` — synchronized (channel-based, no sleeps) coverage of every entry point:
  - `TestRunRequestCancelledBeforeLeaseAcquisition` — both pool connections held, a third `RunRequest` queued, context cancelled before release: settles exactly once as `OutcomeCancelled` with preserved `context.Canceled` cause, no operation callback invoked, both pool connections and a subsequent `RunRequest` usable after holder release.
  - `TestStartedFirstPageCancelledBeforeLeaseAcquisition` / `TestStartedPageCancelledBeforeLeaseAcquisition` / `TestStartedCountCancelledBeforeLeaseAcquisition` — started SELECT first-page, later-page, and count handles queued and cancelled before lease acquisition: settle exactly once as `OutcomeCancelled` with nil page/zero total, no `beforeFirstPage`/`beforeCount` hook invoked, both pool connections usable after holder release.
  - `TestStartWriteCancelledBeforeLeaseAcquisition` — `StartWrite` queued and cancelled before lease acquisition: settles exactly once as `WriteCancelled` with preserved cause, no phases emitted, no `writeLeaseHook` or `beforeWriteBegin` invoked, no rollback confirmation, both pool connections and a subsequent write usable after holder release.
  - `TestPreLeaseCancellationClassificationRunRequest` / `...StartedRequest` / `...Write` — direct table-driven classification rows at each entry point: wrapped `context.Canceled` → cancelled (cancellation precedence), typed `*HealthError` → failed with health (not masked), ordinary `context.DeadlineExceeded` → failed (unchanged); no operation callback, hook, or phase runs in any row.
- `internal/connection/firstpage_test.go` — `TestRunFirstPagePrecancelledContext` updated: an already-cancelled context classifies as `OutcomeCancelled` (was `OutcomeFailed` before Issue #60).
- `internal/connection/revalidate_test.go` — `TestReadSchemaVersionCancelledContextFailsWithCancellation` updated: already-cancelled context classifies as `OutcomeCancelled`.
- `internal/connection/schema_test.go` — `TestReadCatalogCancelledContextFailsWithCancellation` updated: already-cancelled context classifies as `OutcomeCancelled`.
- Race-detector run over `internal/connection` passes (`CGO_ENABLED=1 go test -race ./internal/connection`).

Cross-references: [cancellation-infrastructure.md](cancellation-infrastructure.md), [connection-pool.md](connection-pool.md), [session-health.md](session-health.md), [scoped-select-cancellation.md](scoped-select-cancellation.md), [transactional-writes.md](transactional-writes.md), [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md), [source-code.md](source-code.md), [unit-tests.md](unit-tests.md).
