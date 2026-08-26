# Tasks for #43: Commit-boundary cancellation and quit cleanup

Parent issue: #43
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify commit-boundary interrupt behavior

**Type**: RED
**Output**: Failing barrier tests cover atomic pre-COMMIT cancellation, interrupt before boundary, no interrupt during rollback/commit, and exact boundary feedback.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic barrier tests in `internal/connection` and scripted Ctrl+W model tests in `internal/ui` for Issue #43 and the Writes and commit boundary, Global Key Precedence and Context/Action Matrix, Connection/UI Module Design, and write-cancellation Testing Decisions in `Notes/PRD-sqloid.md`. Hold writes separately in beginning, executing, the atomic after-statement/before-COMMIT decision, rollback cleanup, and committing. Require Ctrl+W before the boundary to set cancellation once and issue a scoped interrupt only against the active leased write connection; release successful statement completion after cancellation and prove the cancellation flag wins atomically before COMMIT begins. Once rollback cleanup or committing has begun, require Ctrl+W to issue no context cancellation or driver interrupt, leave phase/work unchanged, and show exactly `Commit in progress; cancellation is no longer available`. Assert repeated keys, phase races, and stale execution/request identities cannot cross the boundary backward, interrupt cleanup/commit, or start replacement work. Keep this task test-only, use synchronization barriers rather than sleeps, and preserve Issue #42's transaction and result contracts.

---

### 2. Enforce the noncancellable commit boundary

**Type**: GREEN
**Output**: Beginning/executing versus rollback/committing Ctrl+W tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Make the cancellation-to-commit transition in `internal/connection` atomic with respect to the request cancellation flag: beginning/executing remain cancellable through the exact leased connection, the final check after statement completion must choose rollback when cancellation was requested, and crossing into rollback cleanup or committing must permanently disable interrupt issuance for that execution. Expose typed cancellability/phase state to `internal/ui`; route Ctrl+W by that state rather than label text, deduplicate pre-boundary cancellation, and return the exact boundary feedback without mutating work for rollback/commit phases. Reject stale IDs and phase regressions, retain settlement and lease ownership until cleanup/commit resolves, and implement only enough to make Task 1 pass without changing result finalization or adding quit behavior.

---

### 3. Specify accepted-quit write cleanup

**Type**: RED
**Output**: Failing tests cover quit during cancellable work, rollback resolution, committing, unresolved outcomes, no abandoned driver work, and exit only after settlement.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add accepted-quit lifecycle tests in `internal/ui` with barrier-controlled write phases and settlement instrumentation in `internal/connection`, plus result-entry assertions in `internal/history`. Open the shared quit confirmation from beginning, executing, rollback cleanup, and committing, accept it, and require cancellable work to receive one cancellation request and proceed through rollback resolution while noncancellable work receives no interrupt and continues resolving. Hold successful rollback, successful commit, unresolved rollback, and unresolved commit separately; require the application to remain alive with no exit command while transaction or driver work is pending, finalize exactly once under the definite or unknown outcome, and emit exit only after all work has ended. Assert no lease, goroutine, driver call, transaction, or late outcome is abandoned; duplicate acceptance/settlement cannot exit early or finalize twice; and stale identities cannot settle current cleanup. Cover cancelled quit confirmation separately to prove exact suspended write phase restoration and no cleanup side effects. Keep this task test-only and consume the unknown-outcome signal contract without implementing Issue #45's terminal UI.

---

### 4. Implement quit settlement for writes

**Type**: GREEN
**Output**: Accepted quit waits for cancellation/rollback or commit resolution and exits only after work ends.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the shared accepted-quit coordinator in `internal/ui` to retain the active write execution/request identity and wait state until `internal/connection` reports that transaction and driver work have fully ended. For beginning/executing, request cancellation once and wait through rollback resolution; for rollback cleanup or committing, issue no interrupt and wait for the existing operation. Feed definite outcomes through the Issue #42 exactly-once `internal/history` finalizer and retain unresolved commit/rollback metadata for Issue #45, but do not emit the quit command until settlement is explicit in either case. Make repeated quit acceptance and duplicate/late phase messages idempotent, prohibit replacement database work during cleanup, preserve exact state when quit is declined, and ensure the leased connection is not released or the process torn down early. Implement only enough to make Task 3 pass without taking ownership of outcome-unknown terminal presentation.

---

### 5. Document commit boundaries and quit cleanup

**Type**: DOCUMENT
**Output**: Wiki documentation records phase cancellability, atomic boundary, feedback, settlement, and quit rules.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #43 implementation and tests from `internal/connection`, `internal/history`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document beginning/executing cancellability, the atomic post-statement pre-COMMIT cancellation decision, cancellation-wins behavior, and the irreversible noncancellable rollback-cleanup/committing side of the boundary. Record scoped pre-boundary interrupts, the absence of interrupts after crossing, exact `Commit in progress; cancellation is no longer available` feedback, no replacement or lease release before settlement, and stale/duplicate-message guards. Explain accepted quit for each phase: cancel and await rollback when cancellable, await existing rollback/commit when noncancellable, finalize definite or unresolved outcomes only after driver work ends, and exit only after settlement with no abandoned cleanup. Cross-reference Issues #6, #28, #42, #43, and #45 and the Identities and state, Writes and commit boundary, Global Key Precedence and Context/Action Matrix, write-transaction Implementation Decision, Connection/UI/History Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the commit-boundary walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/043-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/043-06/code-walkthrough`. Hold one write in each beginning, executing, atomic pre-COMMIT, rollback-cleanup, and committing phase. Demonstrate Ctrl+W issuing a scoped interrupt before the boundary, cancellation winning after statement success, and no interrupt after the boundary with exact `Commit in progress; cancellation is no longer available` feedback. From cancellable and noncancellable phases, open and accept quit, then hold rollback, commit, and unresolved outcomes to prove no exit occurs while transaction/driver work remains and exit follows settlement only. Capture no replacement/lease release, no abandoned work, exactly-once finalization, duplicate/stale-message resistance, and declined-quit restoration. Reference Issue #43 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review boundary behavior

**Type**: REVIEW
**Output**: Human confirms Ctrl+W and accepted quit before/after the boundary and final database outcomes.
**Depends on**: 6

Review commit-boundary orchestration in `internal/connection`, finalization state in `internal/history`, Ctrl+W/quit behavior in `internal/ui`, wiki updates, and `Notes/walkthroughs/043-06/code-walkthrough` against Issue #43. Manually hold beginning, executing, pre-COMMIT, rollback-cleanup, and committing writes; press Ctrl+W before and after the boundary and verify scoped interrupt versus exact boundary feedback, then inspect final database state after rollback or commit. Accept quit in each phase and in controlled unresolved rollback/commit cases, confirming no process exit, replacement work, lease release, or abandoned driver operation before settlement and exactly one final outcome afterward. Cancel quit once per phase to verify exact restoration, then approve only when pre/post-boundary behavior and persisted outcomes match the documented rules.

---
