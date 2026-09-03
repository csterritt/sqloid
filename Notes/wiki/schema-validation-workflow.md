# Pre-Execution Schema-Version Validation (Issue #21)

Cancellable pre-execution schema-version validation per Issue #21, the Execution and Result Lifecycle requirement 81, and the Schema scope, cache, and validation decision in `Notes/PRD-sqloid.md`: Enter on otherwise runnable builder data starts a distinct validation workflow that reads `PRAGMA schema_version` before any execution; an unchanged version reuses the exact cached catalog, a changed version refreshes the selected object and columns and repairs only dependent builder state, ordinary refresh failures retain stale data behind retry/cancel, deletion/replacement takes terminal precedence, failed or cancelled validation appends no history, and a post-validation DDL race is an ordinary execution error.

## Typed revalidation outcomes — `internal/schema/revalidate.go`

`Revalidate(prior *Catalog, currentVersion int64, refresh func() Attempt) Revalidation` is the pure classification core: an equal version returns `RevalidateUnchanged` carrying the exact prior catalog pointer (no catalog refresh is ever invoked), while a changed version invokes `refresh` at most once and maps the established Issue #13 `Attempt` kinds onto the typed revalidation outcomes. The complete Attempt-to-Revalidation status map (Issue #82 finalizes the malformed-status fallback):

- **`RefreshOK` → `RevalidateRefreshed`**: the refreshed `Catalog` is installed as the authoritative metadata; `Cause` is nil.
- **`RefreshFailed` → `RevalidateRefreshFailed`**: ordinary refresh failure (lock, corruption, change race); only `Cause` is carried verbatim, `Catalog` is nil, and every consumer retains the prior cache unchanged behind `could not refresh: <cause>` reporting. The cause is preserved verbatim and never reinterpreted.
- **`RefreshDeleted` → `RevalidateDeleted`**: terminal — the database file no longer exists at the request boundary; neither `Catalog` nor `Cause` is set.
- **`RefreshReplaced` → `RevalidateReplaced`**: terminal — a different file now owns the startup path (device/inode mismatch); neither `Catalog` nor `Cause` is set.
- **zero or unknown `Attempt.Status` → `RevalidateRefreshFailed` (defensive)**: a zero `RefreshStatus` or any out-of-range value — never produced by the constructors in `refresh.go` — defensively settles as `RevalidateRefreshFailed` with a non-nil diagnostic `Cause` of the form `schema revalidate: unsettled refresh attempt status <status>` and a nil `Catalog`, rather than leaking an unset or unknown status to consumers. Contradictory payload fields (a stray `Catalog` or `Cause` on the malformed attempt) are ignored: the default status mapping is authoritative. This keeps the result actionable under the existing stale-refresh workflow — the prior cache stands behind the diagnostic cause exactly as for an ordinary failure — and pushes no malformed-state handling into UI consumers.

Outcomes are typed values; `Revalidation` payloads follow the same discipline as `Attempt` (one status meaningful, immutable after settlement) so no consumer infers anything from error strings. `Revalidation.Valid()` (Issue #83) is the authoritative invariant guard mirroring `Attempt.Valid()`: it enforces the exact settled payload contract so consumers can rely on a nil-Catalog refresh-failed outcome alone meaning "retain the prior cache". The complete validity matrix:

- **`RevalidateUnchanged` accepted**: exactly a non-nil `Catalog` (the exact prior cache pointer) with nil `Cause`.
- **`RevalidateRefreshed` accepted**: exactly a non-nil `Catalog` (the refreshed snapshot) with nil `Cause`.
- **`RevalidateRefreshFailed` accepted**: exactly a non-nil `Cause` (preserved verbatim, never reinterpreted) with nil `Catalog` so every consumer retains the prior cache unchanged.
- **`RevalidateDeleted` accepted**: only nil `Catalog` and nil `Cause` (terminal, no payload).
- **`RevalidateReplaced` accepted**: only nil `Catalog` and nil `Cause` (terminal, no payload).
- **zero `RevalidateStatus` rejected**: the zero value is not a settled outcome.
- **unknown `RevalidateStatus` rejected**: any out-of-range value is not a settled outcome, regardless of payload.
- **missing required field rejected**: `RevalidateUnchanged`/`RevalidateRefreshed` without a `Catalog`, or `RevalidateRefreshFailed` without a `Cause`, are inconsistent.
- **forbidden extra field rejected**: any accepted status carrying the field it must not carry (`Cause` on unchanged/refreshed, `Catalog` on refresh-failed, either `Catalog` or `Cause` on the terminal statuses) is inconsistent.

Issue #82 ensures malformed `Attempt` payloads (zero or unknown `RefreshStatus`, including contradictory stray `Catalog`/`Cause` fields) are first converted into a valid `RevalidateRefreshFailed` value with a non-nil diagnostic `Cause` of the form `schema revalidate: unsettled refresh attempt status <status>` and a nil `Catalog`, so every `Revalidate` output — unchanged, changed-success, ordinary failure, terminal deletion/replacement, and the malformed-attempt fallback — satisfies `Valid()`. The invariant mirrors `Attempt.Valid()` exactly without changing runtime consumer semantics: it adds no constructor, status value, mapping, catalog identity, cause preservation, or UI behavior — it only codifies the contract every settled `Revalidation` already obeys so a nil-Catalog refresh-failed outcome alone is a reliable "retain the prior cache" signal.

`VersionAttempt{Status, Version, Cause}` types the transport step — one `PRAGMA schema_version` read with the same `RefreshOK/Failed/Deleted/Replaced` kinding and constructors (`NewVersionOK`, `NewVersionFailure`, `NewVersionDeleted`, `NewVersionReplaced`) — and `ReadSchemaVersion` in `internal/connection/schema.go` runs it as one ordinary cancellable `RunRequest`, so request-boundary identity checks and cancellation apply exactly as to any other database request.

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

Cross-references: [schema-catalog.md](schema-catalog.md) (Issue #9 cache), [stale-schema-refresh.md](stale-schema-refresh.md) (Issue #13 retry/terminal and `Attempt.Valid()`), [cancellation-infrastructure.md](cancellation-infrastructure.md) (Issue #6 request lifecycle), [session-health.md](session-health.md) (Issue #7 terminal classification), [runnable-state-feedback.md](runnable-state-feedback.md) (Issue #19 report), [query-history-append.md](query-history-append.md) (Issue #20 append timing), Issues #13, #21, #82, and #83 (the invariant encoding this finalized mapping), and the Execution and Result Lifecycle, Schema scope/cache/validation, and Builder lifecycle sections of `Notes/PRD-sqloid.md`.
