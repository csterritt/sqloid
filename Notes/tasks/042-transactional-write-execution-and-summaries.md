# Tasks for #42: Transactional write execution and summaries

Parent issue: #42
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify phased transactional writes

**Type**: RED
**Output**: Failing fake/SQLite tests cover leased BEGIN, statement execution, cancellation flag after success, pre-COMMIT check, rollback cleanup, commit, constraints, and triggers.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-controlled fake tests and real modernc SQLite integration tests in `internal/connection` for Issue #42 and the Writes and commit boundary, Connection Module Design, write-transaction Implementation Decision, and write-cancellation Testing Decisions in `Notes/PRD-sqloid.md`. For confirmed qualified/unqualified UPDATE and DELETE plus runnable INSERT, require one path-identity check before the whole request, one dedicated leased physical connection, explicit BEGIN, exactly one statement execution on that lease, and no second actual write. Hold each beginning/executing transition deterministically; set cancellation before BEGIN, during statement execution, and immediately after a successful statement, and require the atomic cancellation flag check immediately before COMMIT to win. Require cancellation and statement errors—including constraint and trigger failures—to enter rollback cleanup and wait for confirmed rollback, while an uncancelled successful statement crosses to COMMIT and resolves it. Verify UPDATE, DELETE, INSERT, trigger side effects, and constraint outcomes against persisted SQLite state, lease settlement, and connection reuse. Keep this task test-only, use synchronization barriers rather than sleeps, and leave post-boundary Ctrl+W/quit ownership to Issue #43.

---

### 2. Implement transactional write execution

**Type**: GREEN
**Output**: Beginning/executing cancellation and confirmed rollback/commit tests pass for UPDATE/DELETE/INSERT.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the typed phased write request in `internal/connection` on one dedicated leased connection: verify identity once at the request boundary, begin a transaction, execute the sole UPDATE/DELETE/INSERT statement, atomically inspect the request cancellation flag after statement completion and immediately before starting COMMIT, then perform rollback cleanup or commit and resolve the outcome before releasing the lease. Reuse Issue #6's request context, scoped interrupt identity, settlement, and cancellation-wins infrastructure for beginning/executing; never force-close or reuse the lease while work remains. Ensure statement failure and cancellation both wait for rollback confirmation, successful execution crosses exactly once to commit, and trigger/constraint behavior remains native SQLite behavior. Expose typed phase/outcome messages needed by `internal/ui` without moving transaction logic there, and implement only enough to make Task 1 pass; detailed post-boundary interaction remains Issue #43.

---

### 3. Specify write history and summaries

**Type**: RED
**Output**: Failing tests cover sole actual execution, execution-start query append, exactly one result, executed SQL, RowsAffected labels, and no untouched claim before rollback confirmation.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted write-lifecycle tests in `internal/ui` and immutable non-tabular entry tests in `internal/history`, using the controllable `internal/connection` fake, for confirmed UPDATE/DELETE and direct runnable INSERT. Require confirmation or INSERT dispatch to begin the sole actual execution, exit either history first, and append the complete query state at execution start subject only to existing consecutive-identical suppression; preparation and pre-start messages must append nothing. For successful, cancelled, and failed writes, require exactly one immutable result entry tied to the execution identity and containing the executed standalone SQL. Assert UPDATE and DELETE summaries use actual statement `RowsAffected()` with operation-appropriate rows-affected wording, INSERT uses rows-added wording, and trigger/constraint behavior does not substitute estimate counts. Hold rollback cleanup after cancellation and statement failure and prove the UI/result history makes no database-untouched guarantee until successful rollback confirmation; cover rollback-confirmed final labels, duplicate/late phase outcomes, and no per-phase or per-message entries. Keep this task test-only and reserve unresolved rollback/commit outcome handling for Issue #45.

---

### 4. Implement write result finalization

**Type**: GREEN
**Output**: Successful, cancelled, and failed writes produce exactly one correctly labeled non-tabular result.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire typed `internal/connection` write phases and resolved outcomes into one execution lifecycle in `internal/ui` and one idempotent immutable append path in `internal/history`. At actual execution start, exit history and append the complete query state once; do not append for preparation. Retain execution identity and executed standalone SQL through beginning, executing, rollback cleanup, and commit resolution, then create exactly one non-tabular result after a definite outcome: successful UPDATE/DELETE with actual rows affected, successful INSERT with rows added, or rollback-confirmed cancelled/failed status. Mark finalization before duplicate or late messages can append again, reject stale execution/request identities, and never describe the database as untouched until rollback success is known. Keep estimate count separate from statement `RowsAffected()`, preserve native constraint/trigger errors, and implement only enough to make Task 3 pass without implementing Issue #45's outcome-unknown terminal workflow.

---

### 5. Document transactional writes

**Type**: DOCUMENT
**Output**: Wiki documentation records phases, cancellation check, rollback guarantee, commit, histories, and operation-specific summaries.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #42 implementation and tests from `internal/connection`, `internal/history`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the one-request/one-lease lifecycle from execution start through BEGIN, sole statement execution, atomic post-statement pre-COMMIT cancellation check, rollback cleanup or commit, settlement, and release; record that cancellation wins after statement success and that constraints/triggers participate in the same transaction. Explain execution-start query-history append, preparation exclusion, consecutive suppression, exactly one immutable non-tabular result, retained executed SQL, UPDATE/DELETE actual `RowsAffected()` versus INSERT rows-added labels, and why no untouched guarantee appears before confirmed rollback. Cross-reference Issues #6, #28, #39, #41, and #42 and the Identities and state, Writes and commit boundary, History and write-transaction Implementation Decisions, Connection/UI/History Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; identify Issues #43 and #45 as the post-boundary/unknown-outcome owners, update `Notes/wiki/index.md` for any added or removed page, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the transactional-write walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/042-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/042-06/code-walkthrough`. Exercise runnable INSERT and confirmed qualified/unqualified UPDATE and DELETE through leased BEGIN, sole statement execution, atomic pre-COMMIT cancellation check, rollback cleanup, commit, and settlement. Use deterministic barriers to show cancellation before/during execution and after statement success winning before COMMIT, plus constraint and trigger success/failure with persisted rows inspected after resolution. Capture query history appending only at actual execution start, exactly one result per write, executed SQL, actual UPDATE/DELETE rows-affected and INSERT rows-added summaries, and the absence of an untouched claim until rollback confirmation. Include duplicate/late-message idempotence and healthy lease reuse, reference Issue #42 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
