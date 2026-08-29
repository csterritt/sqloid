# Scoped SELECT Ctrl+W Cancellation and Bounded Settlement

How Ctrl+W cancels every active request of an active SELECT — the first page, one later page, and the independent count — through per-request connection-scoped interrupts, holds the exact `cancelling…` feedback until every targeted request truly settles, and rejects any late success (Issue #28; `Notes/PRD-sqloid.md` §Identities and state, §SELECT, §Errors and cancellation bounds, §Global Key Precedence). The underlying request lifecycle, cancellation-wins classification, and interrupt dispatch are Issue #6 infrastructure consumed unchanged, documented in [cancellation-infrastructure.md](cancellation-infrastructure.md); the `cancelling…` handoff and gate interaction are Issue #27's, documented in [in-flight-gating.md](in-flight-gating.md); identity rejection of stale responses is Issue #26's, documented in [select-request-identities.md](select-request-identities.md).

## Scope and ownership split

Ctrl+W applies only to cancellable active database requests/phases. Issue #28 owns **only** SELECT page and count work:

- schema-validation cancellation → Issue #21 ([schema-validation-workflow.md](schema-validation-workflow.md));
- destructive-estimate cancellation → Issue #41;
- beginning/executing write cancellation with the atomic pre-commit check → Issue #42;
- post-commit-boundary behavior (`committing`/`rollback-cleanup` noncancellable, boundary feedback) → Issue #43.

## Connection layer: started-request handles

`internal/connection/started_request.go` adds the `StartedRequest`/`StartedPageRequest`/`StartedCountRequest` handles on top of Issue #6's `Request` lifecycle. Each `StartFirstPage`/`StartPage`/`StartCount` call leases a dedicated connection, begins a `Request`, and runs the statement in a separate goroutine — so a caller (the UI) can request cancellation from another goroutine while the work runs. Contracts:

- **Independent identities** — each started request owns its own lease and its own derived cancellable context; cancelling the page never disturbs the count's context or state.
- **Cancel is a request, not settlement** — `Cancel` is idempotent and connection-scoped; the work settles only when the in-flight statement actually ends (interrupted mid-execution for CPU-bound work, busy-timeout expiry for lock waits).
- **True settlement before release** — the lease is released only at settlement, so no replacement page/count work or lease reuse can occur while a cancelled request is unsettled; `Wait` blocks until terminal classification and `SettledChan` exposes settlement for selects.
- **Classification preserved** — every result flows through Issue #6's `Settle` (late success discarded as cancelled; `context.Canceled` as cancelled; anything else failed) and Issue #7's post-error health verification; a lease-acquisition failure pre-settles as failed so callers always observe exactly one settlement.
- **SQL ownership unchanged** — statements and parameters come from the QueryBuilder rendering seam (`PageSQL`/`PageParams`, `CountSQL`/`CountParams`); this package never rebuilds SQL.

## UI layer: scoped Ctrl+W orchestration

`internal/ui` wires Ctrl+W through Issue #27's cancellation seam to the per-request cancellation contexts:

- `startSelectPage` derives one cancellable context per concurrent request (first page and count) and installs `firstPageCancel`/`countCancel`; `handlePageKey` derives `pageRequestCancel` for the one later page and re-arms the generic cancellation seam (a later page is its own cancellable unit after the first settlement closed it).
- The `SelectCancelRequestedMsg` handler requests cancellation for every handle that is currently installed — exactly the active first-page/later-page/count set — and nothing else. Handles are installed at dispatch and cleared exactly at their request's settlement, so repeat Ctrl+W presses are idempotent no-ops on already-cancelled requests and settled work is never touched.
- `selectCancelling` renders exactly `cancelling…` until `clearSelectCancellingIfSettled` runs at the last settlement; the Issue #27 gate keeps consuming Enter and issue-level actions with no replacement dispatch until then.
- Settlement flows through Issue #26's identity guards (`SelectTracker` acceptance plus viewport generation), so a cancelled-classified late result — even one arriving after a newer execution begins — is fully inert and cannot mutate rows, count state, or cache metadata.
- Cancellation after rows retains the established rows and lifecycle metadata; cancellation before rows leaves the established cancelled-before-rows finalization to the existing settlement paths. Connections are never force-closed, and after all-request settlement the gate accepts healthy replacement executions.

## Capability evidence (Linux/macOS, release-blocking)

`internal/connection/select_cancellation_capability_test.go` (`//go:build unix`) extends Issue #6's proven modernc seam to real page/count SELECTs whose statements are generated through `internal/querybuilder`. CPU-bound cost comes from virtual generated columns invoking registered expensive scalar probes — the probe lives in the fixture, keeping querybuilder the sole owner of the SQL text — with channel-drained barrier signals proving "work has begun" (SQLite evaluates generated columns during fixture INSERT too, so stale signals are drained before the queries run):

- `TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond` — distinct leased connections and contexts, page-only cancellation leaving the count's identity untouched, ≤ 1 s settlement for each interrupted CPU-bound scan, count still running after the page settles, both leases reused afterward for harmless work.
- `TestCapabilityLaterPageCancelInterruptsWithinOneSecond` — the `StartPage` later-page path under the same one-second bound.
- `TestCapabilityNoLeaseReuseBeforeEveryRequestSettles` — both requests cancelled while held behind barriers: third-lease acquisition fails while both are unsettled, each lease returns only at its own settlement, and both connections accept healthy subsequent work.
- `TestCapabilityLockWaitCountCancelsWithinBusyTimeout` — an EXCLUSIVE lock holder plus a 50 ms canary write prove the count is verifiably blocked before cancellation; settlement lands within the five-second busy-timeout window measured from contention start. **Pinned-driver reality (modernc v1.57.0):** `sqlite3_interrupt` does not preempt a busy-handler wait, so the blocked read settles at the configured expiry with `SQLITE_BUSY`, which classifies as failed (busy cause preserved) rather than cancelled — the PRD bound "no later than the five-second busy timeout" still holds.
- `TestCapabilityLateSuccessIsCancellationWinsOnRealConnection` — a real count completing before cancellation but released after it classifies as cancelled, leaks no total, and leaves the connection healthy.

These tests are release-blocking (capability failure on Linux/macOS must be fixed in the pin or integration, never skipped). Note that after a mid-flight interrupt `database/sql` may itself recycle the physical connections at the exact-two floor — that is pool bookkeeping, never a Sqloid force-close; settlement-before-release and healthy subsequent work are what Sqloid owns.

## Verification

- `internal/connection/select_cancellation_test.go` — barrier-driven handle coverage: independent page/count cancellation with settlement ordering, idempotent repeated/late `Cancel` with single settled classification, count-only scope leaving the pool healthy.
- `internal/ui/select_cancellation_test.go` — scripted first-page+count, count-only, and later-page-only scopes: independent identities, `cancelling…` persisting per-request, no replacement Enter/page dispatch before settlement, inert cancelled late pages, healthy replacement work after settlement.
- `cancellation_settlement_test.go` suites in both packages (Issue #26) continue to cover cancellation-wins over late success on every role.
- Race-detector runs pass (`CGO_ENABLED=1 go test -race ./internal/ui ./internal/connection`).

Cross-references: [cancellation-infrastructure.md](cancellation-infrastructure.md), [in-flight-gating.md](in-flight-gating.md), [select-request-identities.md](select-request-identities.md), [serialized-vertical-paging.md](serialized-vertical-paging.md), [concurrent-page-count.md](concurrent-page-count.md), [connection-pool.md](connection-pool.md), [source-code.md](source-code.md), [unit-tests.md](unit-tests.md).
