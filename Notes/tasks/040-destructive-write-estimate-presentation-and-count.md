# Tasks for #40: Destructive-write estimate presentation and count

Parent issue: #40
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify destructive SQL and estimate construction

**Type**: RED
**Output**: Failing QueryBuilder/Connection tests cover shared literal rendering, exact `COUNT(*)` target/WHERE SQL, WHERE-only params, excluded SET values, and qualified/unqualified writes.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent, exact-string tests in `internal/querybuilder` and focused request/integration tests in `internal/connection` for destructive UPDATE and DELETE preparation. Starting from the executable statements delivered by Issues #37 and #38, require one canonical rendered standalone write SQL produced with Issue #14's shared identifier and typed literal atoms: safely quote table/column names, render INTEGER/REAL/TEXT literals exactly, quote-double TEXT, preserve typed TEXT `NULL` as text, and render assignment/predicate SQL NULL as keywords. Cover qualified and no-WHERE writes, DELETE and UPDATE, all-Value/all-NULL/mixed SET assignments, value-taking and null-operator predicates, unusual identifiers, and trailing standalone-statement form where the shared contract requires it. Specify the estimate request as exactly `SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]`, reusing the same predicate SQL and binding semantics. Assert it binds only WHERE values, has no params for absent/null-operator WHERE, and never includes UPDATE SET SQL fragments or SET parameters. Verify `internal/connection` receives and executes that independent query as an estimated matching-target count without executing the write. Keep this task test-only and make a modal-private serializer or predicate reconstruction fail ownership expectations.

---

### 2. Implement estimate request construction

**Type**: GREEN
**Output**: Canonical rendered write SQL and independent estimate-query tests pass without a private serializer.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement destructive preparation artifacts in `internal/querybuilder`: derive canonical standalone rendered UPDATE/DELETE SQL from the same structured state as executable SQL, calling Issue #14's sole shared typed-literal renderer and identifier atoms rather than adding serialization in `internal/ui`; derive the independent estimate request by composing the quoted selected table with the exact shared optional predicate. Preserve qualified versus unqualified shape, Value/NULL distinctions, typed value identity, and safe literal errors. Exclude every UPDATE SET assignment and parameter from estimate SQL/params, passing only the predicate's existing bound values in predicate order. Add the narrow `internal/connection` estimate request seam needed to run `SELECT COUNT(*)` as a cancellable independent read and return count/error without initiating a write, history append, or transaction. Reuse existing request identity, health checks, and dedicated-connection plumbing where available, and implement only enough to make Task 1 pass; modal state and confirmation remain Tasks 3-4.

---

### 3. Specify preparation modal presentation

**Type**: RED
**Output**: Failing model tests cover operation, table, continuously visible SQL, prominent all-rows warning, `Estimating matching target rows…`, disabled confirmation, and no history.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model and focused view tests in `internal/ui` for opening destructive preparation from runnable, validated UPDATE and DELETE states. For qualified and unqualified writes, require the modal to open immediately with a unique preparation/request identity and continuously display operation type, table name, and QueryBuilder's canonical rendered SQL; for absent WHERE, require a visually prominent all-rows warning that remains visible through every estimate state, while qualified writes show no false warning. Initially render exactly `Estimating matching target rows…`, dispatch one estimate request through the controllable `internal/connection` fake, and disable Enter/y confirmation until that request settles; attempts must be consumed without write execution or history. Assert SQL, operation, table, and warning remain present while estimating and under unrelated redraw/resize messages. Cover estimate success and failure messages as retained modal state for later confirmation, cancellation/dismissal seams, stale identity responses, and prove opening, pending, success, failure, cancellation, and dismissal append neither query nor result history. Keep this task test-only; do not implement actual write confirmation/execution.

---

### 4. Implement destructive preparation and estimation

**Type**: GREEN
**Output**: Qualified/unqualified UPDATE/DELETE modal and pending-estimate tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate destructive preparation state in `internal/ui` with canonical rendered write/estimate artifacts from `internal/querybuilder` and cancellable estimate execution from `internal/connection`. After successful pre-execution validation of runnable UPDATE or DELETE, allocate the preparation/request identity, retain the opener state, open the modal immediately, and dispatch exactly one independent estimate. Render operation, table, standalone SQL, and the no-WHERE all-rows warning continuously; show exactly `Estimating matching target rows…` while pending and keep Enter/y disabled until settlement. Route estimate responses through current identity guards so stale success/failure cannot mutate the modal; on current success retain the estimated matching-target count, and on current failure retain the failure while preserving SQL/warning and transitioning to the later confirmation-ready seam. Esc/n dismissal and estimate cancellation must return to the exact builder focus with no execution or history, while pending Enter/y remains a no-op. Do not create a UI serializer, include SET params in estimates, or begin the actual transactional write; implement only enough to make Tasks 1 and 3 pass.

---

### 5. Document destructive preparation

**Type**: DOCUMENT
**Output**: Wiki documentation records modal contents, shared serialization, warning, estimate meaning/SQL/params, disabled confirmation, and no-history status.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #40 implementation and tests from `internal/querybuilder`, `internal/connection`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the continuously visible operation, table, canonical standalone write SQL, and prominent no-WHERE all-rows warning; identify Issue #14's QueryBuilder literal renderer as the sole serializer and state that the modal owns none. Record exact estimate SQL, identical shared predicate reuse, WHERE-only parameter semantics, exclusion of UPDATE SET values, the `estimated matching target rows` meaning, and its exclusion of trigger effects/guaranteed changed rows. Describe immediate modal opening, preparation/request identity, exact `Estimating matching target rows…` text, disabled confirmation while pending, stale-response rejection, success/failure retained state, dismissal/cancellation restoration, and no query/result history before actual execution. Cross-reference Issues #14, #17, #21, #37, #38, and #40 and the pre-execution identities, Writes lifecycle, Estimate SQL and modal, SQL safety, History, Connection/QueryBuilder/UI Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the estimate-modal walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/040-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/040-06/code-walkthrough`. Open preparation for qualified and unqualified UPDATE and DELETE statements, including UPDATE with mixed Value/NULL SET assignments and predicates using bound values and SQL-null operators. Capture operation, table, canonical rendered SQL, safely rendered literals, and the prominent all-rows warning only for no-WHERE statements. Compare each write with exact `SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]` evidence and parameter lists, proving SET fragments/values are absent and WHERE values retain exact bound types/order. While the fake or controlled estimate is pending, show exact `Estimating matching target rows…`, continuously visible SQL/warning, disabled Enter/y, one request, and no history. Demonstrate current/stale success and failure handling plus dismissal/cancellation restoring the builder without execution/history. Reference Issue #40 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
