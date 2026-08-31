# Tasks for #61: Classify COMMIT failure as outcome unknown

Parent issue: #61
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the real COMMIT-failure boundary and terminal result

**Type**: RED  
**Output**: Failing connection/UI tests induce a driver COMMIT failure and require one commit-phase outcome-unknown result without rollback claims.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add a Connection-level integration test in `internal/connection` that induces an actual SQLite/driver `tx.Commit()` failure after the write statement has executed, rather than passing a preclassified `WriteResult` into the UI. Use a deterministic driver or database boundary supported by the existing modernc test patterns and capture ordered phases, rows affected, commit cause, rollback return, lease settlement, and final database observability. Require the result to preserve `WritePhaseCommitting` and the commit error, leave `RollbackConfirmed` false even when the later rollback returns `sql.ErrTxDone`, and never classify persistence as definitely failed/untouched. Drive that real result through `internal/ui/write_exec.go` and require exactly one `finalizeOutcomeUnknown` entry selected in `TerminalOutcomeUnknown`, with commit-phase/error wording and non-persistence RowsAffected disclosure, no ordinary `WriteFailed` summary, and no rollback/untouched claim. Keep a control case where a pre-COMMIT statement failure or cancellation followed by genuinely successful rollback remains confirmed untouched. Keep this task test-only and follow `commit_boundary_test.go`, `write_test.go`, and `internal/ui/commit_boundary_test.go` identity/finalization conventions.

---

### 2. Preserve unresolved state after COMMIT failure

**Type**: GREEN  
**Output**: Failed COMMIT settles through the outcome-unknown workflow while genuine pre-COMMIT rollback remains definite.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Refine the phased write result in `internal/connection/write.go` so it retains enough phase information to distinguish a failed COMMIT from statement/cancellation rollback paths. When commit fails, preserve the original commit error and committing phase, perform any required noncancellable cleanup, but never treat `sql.ErrTxDone` from the subsequent rollback attempt as confirmation that persistence did not occur; keep `RollbackConfirmed` false and do not overwrite the commit cause with a less informative cleanup result. Update `internal/ui/write_exec.go` and existing history summary use only as needed so the settled commit-failure result always enters `finalizeOutcomeUnknown` exactly once and carries the commit phase/error without a definite failed/untouched statement. Preserve successful commit, genuinely successful pre-COMMIT rollback, health-terminal precedence, settlement, close ordering, and duplicate/stale guards. Implement only enough to make Task 1 pass and run focused connection/UI tests plus the established Go verification command.

---

### 3. Document failed-COMMIT uncertainty

**Type**: DOCUMENT  
**Output**: Wiki documentation records phase/error preservation, `sql.ErrTxDone` limits, outcome-unknown routing, and the confirmed-rollback control case.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #61 implementation and tests from `internal/connection`, `internal/ui`, and `internal/history` into the write/transaction pages under `Notes/wiki`. Explain why a COMMIT error leaves persistence unresolved, why a later `sql.ErrTxDone` is not rollback confirmation, which original phase/error/RowsAffected facts are preserved, and how the UI creates exactly one outcome-unknown entry and terminal state without saying failed, untouched, or rolled back. Contrast statement failure or cancellation before COMMIT followed by a genuinely successful rollback. Cross-reference Issue #61, user stories 45, 47, and 85, and the Writes and commit boundary, terminal-state, history, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the COMMIT-failure walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/061-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/061-04/code-walkthrough`, with the main file named `walkthrough.md`. Run the real driver-boundary COMMIT-failure test and trace statement completion, committing phase, commit error, later `sql.ErrTxDone`, false rollback confirmation, settlement, and database observability. Show the resulting single selected outcome-unknown history entry and terminal UI wording with no untouched/rolled-back claim, then contrast a genuine pre-COMMIT failure with confirmed rollback. After Issue #57, exercise the production composition path or its headless harness to show the terminal workflow is reachable through the shipped TUI. Reference Issues #57 and #61 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
