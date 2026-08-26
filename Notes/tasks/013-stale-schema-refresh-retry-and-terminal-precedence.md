# Tasks for #13: Stale schema refresh, retry, and terminal precedence

Parent issue: #13
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify table-catalog refresh and stale retention

**Type**: RED
**Output**: Failing fake-Connection/model tests cover refresh-before-open, unchanged stale candidates, exact persistent status/cause, and blocked continuation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused refresh-contract tests around `internal/schema` and scripted Bubble Tea model/rendering tests under `internal/ui`, using a deterministic fake Connection rather than a real database. Follow the catalog contracts established by Issue #9, the reusable searchable popup contract from Issue #12, and the `(model, msg) → (model, cmd)` conventions in the Testing Decisions of `Notes/PRD-sqloid.md`. Require every Table-popup open to request a fresh main-schema catalog before current candidates are presented. Seed a prior typed catalog and make refresh fail; assert that the candidate identities, ordering, metadata, search state, and selected builder table remain unchanged, the popup visibly retains those stale candidates, and the persistent status is exactly `Schema data is stale — retry or cancel` while the inline cause is exactly `could not refresh: <cause>`. Prove that both indicators survive ordinary model updates and that accepting a stale candidate, advancing to another builder field, or executing is blocked. Keep this task test-only, preserve the prior catalog as immutable test data, and do not implement retry, cancellation, or terminal override yet.

---

### 2. Implement refresh and stale-schema state

**Type**: GREEN
**Output**: Table popup refresh and stale-state rendering tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement only the refresh initiation, result transition, and stale presentation needed by Task 1 in `internal/schema`, `internal/querybuilder`, and `internal/ui`. Reuse the typed catalog and eligibility data from Issue #9 and the reusable Table-popup integration from Issue #12; keep database access behind the existing Connection boundary and keep Bubble Tea concerns out of Schema and QueryBuilder. On each Table opener, start catalog refresh before exposing current candidates; on success, install the refreshed typed catalog and derive eligible candidates for the active command, and on ordinary failure retain the exact prior catalog without partial replacement. Represent stale state and its cause explicitly, render the exact persistent status and inline cause, and gate popup acceptance, downstream continuation, and execution while stale. Preserve popup overlay geometry, candidate identity, deterministic ordering, and exact opener focus behavior. Do not add the retry/cancel lifecycle or health-terminal precedence assigned to Tasks 3 and 4, and make the Task 1 tests pass with the smallest production change.

---

### 3. Specify retry, cancel, and health precedence

**Type**: RED
**Output**: Failing tests cover retry success clearing indicators, cancel restoration, repeated failure, and deletion/replacement overriding the workflow.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the fake-Connection and scripted model/rendering coverage under `internal/schema`, `internal/querybuilder`, and `internal/ui` without changing production behavior. Drive the stale workflow through explicit retry and cancel messages: require retry to issue a new catalog request, keep the unchanged stale catalog and both exact indicators while pending, replace the catalog only after success, clear stale status and cause atomically, reopen or continue the Table popup with refreshed eligible candidates, and preserve deterministic focus. Require a repeated ordinary failure to retain the same prior catalog, update the inline cause to that attempt's exact cause, and leave retry/cancel available. Require cancel to close only the stale refresh flow, restore the exact Table opener and pre-open builder/catalog state, and perform no continuation or execution. Inject typed Connection health outcomes for path deletion and same-path replacement before refresh and when classifying each refresh error; assert that `Database file no longer exists — session ended` and `Database file was replaced — session ended` override stale state, clear retry/cancel affordances, prevent late refresh results from mutating state, and leave no database work available. Keep this task test-only and cover precedence independently of error-string matching.

---

### 4. Implement stale-flow lifecycle and terminal override

**Type**: GREEN
**Output**: Retry/cancel and typed health-precedence tests pass without mutating the prior catalog.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Complete the stale refresh state machine across `internal/schema`, `internal/querybuilder`, and `internal/ui` to satisfy Task 3. Route retries through the same Connection-backed catalog refresh command used by the initial Table open, give attempts sufficient identity to discard late or superseded results, and retain the prior typed catalog by replacement rather than in-place mutation until a retry succeeds. On success, install the new catalog and clear status/cause together; on repeated ordinary failure, preserve stale candidates and retry/cancel; on cancel, restore the captured opener focus and unchanged pre-open state. Consume typed deletion and replacement health classifications from the Connection boundary before work and after errors, transition immediately to the established terminal model state, suppress stale controls and ordinary causes, and prevent pending or late refresh messages from reviving the workflow. Keep terminal behavior consistent with Issues #7 and #8, preserve in-memory state allowed by the PRD, and implement only enough to make Tasks 1 and 3 pass.

---

### 5. Document stale schema handling

**Type**: DOCUMENT
**Output**: Wiki documentation records refresh timing, stale indicators, blocked actions, retry/cancel, and terminal precedence.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #13 implementation and tests from `internal/schema`, `internal/querybuilder`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document refresh-before-presentation on every Table-popup open, typed catalog replacement, unchanged stale candidate retention, the exact persistent `Schema data is stale — retry or cancel` status, the exact `could not refresh: <cause>` inline form, blocked acceptance/continuation/execution, retry attempt identity and successful clearing, repeated failure, cancel restoration, and typed deletion/replacement terminal precedence over stale errors and late results. Cross-reference Issues #7, #9, #12, and #13 and the Schema scope, cache, and validation, Session health, Builder and Display Interaction, and Testing Decisions in `Notes/PRD-sqloid.md`. Update `Notes/wiki/index.md` for any new or materially changed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the stale-schema walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/013-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/013-06/code-walkthrough`. Demonstrate refresh occurring before a Table popup presents candidates, then use controlled schema change and lock/failure scenarios to show unchanged stale candidates, the exact persistent status and inline cause, blocked selection/continuation, repeated failure, successful retry with both indicators cleared, and cancel with exact opener restoration. Include test-backed deletion and same-path replacement cases that visibly replace the stale workflow with their terminal states and reject late refresh completion. Reference Issue #13 and the relevant contracts in `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
