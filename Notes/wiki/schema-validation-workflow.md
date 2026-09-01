# Pre-Execution Schema-Version Validation (Issue #21)

Cancellable pre-execution schema-version validation per Issue #21, the Execution and Result Lifecycle requirement 81, and the Schema scope, cache, and validation decision in `Notes/PRD-sqloid.md`: Enter on otherwise runnable builder data starts a distinct validation workflow that reads `PRAGMA schema_version` before any execution; an unchanged version reuses the exact cached catalog, a changed version refreshes the selected object and columns and repairs only dependent builder state, ordinary refresh failures retain stale data behind retry/cancel, deletion/replacement takes terminal precedence, failed or cancelled validation appends no history, and a post-validation DDL race is an ordinary execution error.

## Typed revalidation outcomes — `internal/schema/revalidate.go`

`Revalidate(prior *Catalog, currentVersion int64, refresh func() Attempt) Revalidation` is the pure classification core: an equal version returns `RevalidateUnchanged` carrying the exact prior catalog pointer (no catalog refresh is ever invoked), while a changed version invokes `refresh` at most once and maps the established Issue #13 `Attempt` kinds onto `RevalidateRefreshed` (new catalog), `RevalidateRefreshFailed` (cause only — the prior cache stands), and the terminal `RevalidateDeleted`/`RevalidateReplaced`. Outcomes are typed values; `Revalidation` payloads follow the same discipline as `Attempt` (one status meaningful, immutable after settlement) so no consumer infers anything from error strings. `VersionAttempt{Status, Version, Cause}` types the transport step — one `PRAGMA schema_version` read with the same `RefreshOK/Failed/Deleted/Replaced` kinding and constructors (`NewVersionOK`, `NewVersionFailure`, `NewVersionDeleted`, `NewVersionReplaced`) — and `ReadSchemaVersion` in `internal/connection/schema.go` runs it as one ordinary cancellable `RunRequest`, so request-boundary identity checks and cancellation apply exactly as to any other database request.

## Builder repair — `internal/querybuilder/revalidate.go`

`QueryBuilder.Revalidate(c *schema.Catalog)` is the immutable changed-version repair transition, returning the repaired snapshot plus a `RevalidateReport{Cleared, Report}` whose `Report` is the authoritative Issue #19 `RunnableReport` of the repaired state:

- a selected object that vanished or lost command eligibility drops the table and everything downstream, exactly like a vanished-table catalog refresh;
- a change in rowid capability or declared-rowid shadowing invalidates the committed ORDER BY expression (the only v1 rowid-addressing consumer);
- vanished projection entries, GROUP BY names, committed WHERE predicates and drafts, ORDER BY column expressions, UPDATE SET assignments, and INSERT prompts for columns that became hidden/non-insertable are removed individually;
- unrelated completed state — Limit, surviving entries, surviving assignments — is always preserved, and focus follows the established clearing rule only through removed states.

The transition performs no validation workflow, history effect, or execution; the UI consumes the report's first specific invalid field and reason. Issue #65 extends the authoritative `RunnableReport` itself to validate every committed named projection against refreshed visible columns before later SELECT fields, so a stale projection is caught at `RunFieldProjection` with `the projected column no longer exists` even when `Revalidate` has not been invoked (for example when `RefreshSchema` swapped the catalog without the state-clearing repair).

## Validation workflow — `internal/ui/schema_validation.go`

Enter on runnable data in the idle base context still emits the Issue #19 `PreExecutionRequestedMsg` seam; `Update` now routes it into `beginValidation()`, which opens the distinct workflow under a fresh monotonic `validationAttempt` preparation/request identity, arms `ActiveCancellable` with a `CancelCommand` returning `CancelValidationMsg`, and issues the schema-version read through the `VersionReader` seam (never inside `Update`; `beginValidation` refuses once terminal health ended the session, while a validation is already open — no replacement may start before the prior lease settles — or while a stale-refresh request is outstanding). The command captures the cached catalog snapshot at issue time, runs `schema.Revalidate` against it (refreshing through the wired Issue #13 `CatalogRefresher` only on a changed version), and delivers `ValidationSettledMsg{Preparation, Result}` back through `Update`.

`applyValidationSettled` first clears pending/cancellable flags, then discards superseded preparations (identity mismatch or zero) and any arrival after terminal health. A settlement arriving after a cancellation request is classified as cancelled and discarded wholesale — cancellation wins over late success. Otherwise:

- **unchanged**: the exact cached metadata stands with no builder transition and no refresh; the workflow closes and the execution-start route returns.
- **refreshed**: the refreshed catalog installs atomically via `QueryBuilder.Revalidate` + `applyBuilder`; if the repaired snapshot is still runnable the execution-start route returns, otherwise focus moves to the report's typed first-invalid field and the exact reason renders inline — and no execution may start.
- **refresh-failed**: the prior cache stands; the Issue #13 indicators (`Schema data is stale — retry or cancel` plus inline `could not refresh: <cause>`) raise within the validation workflow, blocking continuation.
- **deleted/replaced**: the workflow closes and the exact `Database file no longer exists/was replaced — session ended` terminal states take precedence, rejecting late completions.

While stale, Enter retries with a fresh preparation identity (re-arming cancellability, re-issuing the version read; duplicate retries while a request is outstanding are refused) and Esc cancels: indicators clear, the exact pre-validation builder context stands, the attempt identity advances so an outstanding response can never mutate the restored state, and no execution runs.

## Cancellation and presentation

Ctrl+W during an in-flight validation (normal or suspended contexts) marks the workflow cancelling and dispatches the connection-scoped `CancelCommand` exactly once per request; repeated presses and Enter are refused, and the results region renders the exact `cancelling…` status until true settlement. After settlement the model returns to the exact pre-validation builder context with no execution and no history. `validating…` renders while a request is in flight; stale indicators render through the Issue #13 path during failed-refresh validation.

## History boundaries

Validation appends neither query nor result history in every outcome: opening, pending, failed, cancelled (including cancelled-late), dismissed, and invalid-repair states all append nothing. Only settled successful validation returns the execution-start route — a placeholder command emitting the Issue #20 `ExecutionStartedMsg` seam that Issue #22 replaces with the real execution — which is the sole path through which history appends (SELECT/INSERT at actual start). A post-validation DDL race therefore never retroactively mutates the settled outcome: validation does not re-run after settlement, the execution route stands, and a later schema change surfaces through the ordinary execution-error path of the actual query.

Cross-references: [schema-catalog.md](schema-catalog.md) (Issue #9 cache), [stale-schema-refresh.md](stale-schema-refresh.md) (Issue #13 retry/terminal), [cancellation-infrastructure.md](cancellation-infrastructure.md) (Issue #6 request lifecycle), [session-health.md](session-health.md) (Issue #7 terminal classification), [runnable-state-feedback.md](runnable-state-feedback.md) (Issue #19 report), [query-history-append.md](query-history-append.md) (Issue #20 append timing), and the Execution and Result Lifecycle, Schema scope/cache/validation, and Builder lifecycle sections of `Notes/PRD-sqloid.md`.
