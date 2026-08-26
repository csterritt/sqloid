# Tasks for #21: Pre-execution schema-version validation

Parent issue: #21
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify schema-version revalidation outcomes

**Type**: RED
**Output**: Failing fake/SQLite tests cover unchanged cache reuse, changed refresh, identifier/eligibility/insertability/rowid invalidation, dependent-only clearing, stale failure, and DDL races.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent table-driven tests in `internal/schema` and `internal/querybuilder`, plus focused fake-Connection and `modernc.org/sqlite` integration coverage in `internal/connection`, for the pre-execution revalidation contract in Issue #21. Build on the typed catalog/version metadata from Issue #9, stale refresh classifications from Issue #13, and immutable builder/runnable-report patterns in `internal/querybuilder`. Require an unchanged `PRAGMA schema_version` to reuse the exact cached object and column metadata without issuing a catalog refresh, while a changed version refreshes the selected object and columns from `main.sqlite_master`/the established Schema seam before revalidating object identity, command eligibility, every referenced identifier, INSERT column insertability, rowid capability, and declared-rowid shadowing. For each removed or changed prerequisite, assert that only state transitively dependent on that prerequisite is cleared, unrelated completed builder state is preserved, and the authoritative runnable report identifies the first specific invalid field and reason deterministically. Cover dropped/renamed objects and columns, view/table eligibility changes, hidden/generated insertability changes, ordinary/WITHOUT ROWID or rowid-shadow changes, ordinary lock/corruption refresh failure retaining stale cache without partial replacement, and schema DDL both before refresh and after successful validation; require a post-validation race to remain an ordinary execution error rather than retroactively mutating the validation outcome. Keep this task test-only, use typed outcomes instead of error-string inference, and do not introduce Bubble Tea state.

---

### 2. Implement schema-version validation and repair

**Type**: GREEN
**Output**: UI-independent validation outcomes and builder repair tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal UI-independent validation and repair seam across `internal/connection`, `internal/schema`, and `internal/querybuilder` needed by Task 1. Extend the existing Connection-backed schema operations with cancellable version reads and selected-object refresh while keeping SQL and the pinned driver hidden in `internal/connection`; let `internal/schema` return typed unchanged, refreshed, stale, and terminal-health-aware outcomes without mutating the prior cache on failure. In QueryBuilder, consume refreshed stable object/column identities and capabilities through an immutable revalidation transition that preserves independent fields, clears only state depending on an invalidated object, identifier, insertability fact, or rowid property, and returns the first specific focus reason through the existing runnable-report contract. Reuse Issue #13's stale-data retention and Issue #7's typed deletion/replacement classification, preserve SQL safety and deterministic Schema ordering, and leave workflow presentation, retry/cancel keys, history effects, and execution startup to Tasks 3 and 4.

---

### 3. Specify validation workflow and cancellation

**Type**: RED
**Output**: Failing model tests cover runnable Enter→validation, retry/cancel, no history, Ctrl+W, `cancelling…`, late success, and health precedence.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea `(model, msg) → (model, cmd)` tests under `internal/ui` with a controllable fake Connection, using the request lifecycle from Issue #6, stale retry/cancel behavior from Issue #13, QueryBuilder's authoritative runnable report, and History boundaries in `internal/history`. Require Enter on runnable authoritative builder state to open a distinct pre-execution validation workflow and issue a schema-version request before any SELECT/write execution command; opening, pending, failed, cancelled, and dismissed validation must append neither query nor result history. Cover unchanged success and changed-schema repair, including exact focus on the first invalid reason and no execution when repair makes the builder non-runnable. For ordinary refresh failure, require stale retained data, visible `could not refresh: <cause>` and retry/cancel behavior, a retry with a fresh preparation/request identity, and cancel restoring the correct builder context without execution. During an in-flight validation, require Ctrl+W to request connection-scoped cancellation exactly once, render exact `cancelling…` until settlement, start no replacement request before settlement, and classify/discard a late success as cancelled. Inject path deletion and same-path replacement before work and after request error and assert that their typed terminal states override stale, cancellation, retry, ordinary error, and late-result handling. Keep this task test-only and include execution/preparation identity guards so superseded responses cannot alter current state.

---

### 4. Integrate cancellable pre-execution validation

**Type**: GREEN
**Output**: Validation state-machine tests pass and actual execution begins only after successful validation.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate the validation state machine in `internal/ui` with the UI-independent outcomes from `internal/schema` and `internal/querybuilder`, the cancellable request/settlement API in `internal/connection`, and append boundaries in `internal/history`. Route runnable Enter into a uniquely identified validation preparation, read schema version through a dedicated cancellable lease, use cached metadata only for unchanged versions, and apply refreshed builder repair atomically for changed versions. Begin the actual execution route and append query history only after a current validation response succeeds and leaves the builder runnable; never append result history merely for validation. Preserve stale cache and offer retry/cancel on ordinary refresh failure, restore deterministic focus on cancellation or invalidation, render `cancelling…` through true settlement, reject cancellation-late and superseded success, and prevent replacement work from starting before the prior lease settles. Apply Connection's deletion/replacement health classification before requests and after errors with terminal precedence, while allowing DDL after validation to surface through the later ordinary execution-error path. Keep database behavior out of the Bubble Tea model and do not implement result-grid or count behavior assigned to Issues #22 and #24.

---

### 5. Document schema-version validation

**Type**: DOCUMENT
**Output**: Wiki documentation records cache reuse, refresh/repair, cancellation, no-history behavior, health precedence, and post-validation races.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #21 implementation and tests from `internal/schema`, `internal/querybuilder`, `internal/connection`, `internal/history`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the runnable Enter-to-validation handoff, preparation/request identities, unchanged-version cache reuse, changed-version selected-object/column refresh, identifier and capability revalidation, dependent-only builder clearing, first-specific-reason focus, stale cache retention, retry/cancel, and the rule that validation creates no query or result history. Record Ctrl+W's connection-scoped cancellation, exact `cancelling…` settlement state, cancellation-wins late-success handling, deletion/replacement terminal precedence, and the distinction between a validation-time schema change and an ordinary post-validation DDL execution race. Cross-reference Issues #6, #7, #9, #13, and #21 and the Execution and Result Lifecycle, Schema scope, cache, and validation, Builder lifecycle, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every new or materially changed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the validation-workflow walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/021-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/021-06/code-walkthrough`. Demonstrate runnable Enter entering validation before execution and history, an unchanged schema version reusing cached metadata, and changed schemas refreshing and repairing only dependent builder state with the first specific invalid reason focused. Include changed identifier, eligibility, insertability, and rowid fixtures; an ordinary locked/stale refresh with retry and cancel; Ctrl+W showing exact `cancelling…` until settlement; late success being discarded; and deletion/replacement overriding ordinary workflow state. Show test-backed evidence that failed or cancelled validation appends no history, successful validation alone permits actual execution, and DDL after validation is reported as an ordinary execution error. Reference Issue #21 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review schema revalidation

**Type**: REVIEW
**Output**: Human confirms unchanged/changed/drop/lock cases and retry/cancel behavior.
**Depends on**: 6

Review the Issue #21 changes in `internal/schema`, `internal/querybuilder`, `internal/connection`, `internal/history`, and `internal/ui`, their fake and SQLite tests, wiki updates, and `Notes/walkthroughs/021-06/code-walkthrough`. With a runnable builder, verify unchanged validation reuses cache and proceeds; then externally rename, alter, or drop selected objects and columns and confirm refresh, eligibility/identifier/insertability/rowid checks, dependent-only clearing, preserved independent state, and exact first-invalid focus. Force lock/corruption-style ordinary refresh failures and exercise repeated retry and cancel while confirming no query or result history is appended. Cancel an in-flight validation with Ctrl+W and confirm `cancelling…` persists through settlement and late success cannot execute. Finally test deletion/replacement terminal precedence and a DDL race after successful validation, confirming the latter remains an ordinary execution error before approving the issue.

---
