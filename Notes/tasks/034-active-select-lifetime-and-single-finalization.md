# Tasks for #34: Active SELECT lifetime and single finalization

Parent issue: #34
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify active-SELECT lifecycle transitions

**Type**: RED
**Output**: Failing lifecycle matrix tests enumerate every finalizing and nonfinalizing event independently of individual request completion.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven lifecycle tests in `internal/ui` with focused state assertions in `internal/history` for Issue #34 and the Identities and state, SELECT, global context/action matrix, and active-finalization Testing Decisions in `Notes/PRD-sqloid.md`. Begin with an active SELECT in idle, count-pending, page-pending, count-settled, and page-settled combinations, and prove request completion alone does not define the active lifetime. Enumerate every nonfinalizing event independently: builder edits and focus changes, popups/help and other overlays, save flow, result export/copy, destructive estimate opening/progress/completion/dismissal, query-history browsing/restoration, resize, serialized paging and each page/count success or count failure, and idle periods. Enumerate each finalizer: starting an actual new SELECT or write execution, entering result history, cancellation or failure that ends the SELECT, and accepted quit. Assert pre-execution validation, merely opening an estimate, rejected/invalid execution attempts, and unaccepted quit do not finalize. Keep this task test-only and classify events at the active execution boundary rather than inferring finalization from an individual request settling.

---

### 2. Implement active SELECT state transitions

**Type**: GREEN
**Output**: Builder edits, overlays, save/export, estimate, query history, resize, paging, and idle events preserve activity; enumerated finalizers end it.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement an explicit active-SELECT lifecycle in `internal/ui` that is separate from count/page request state and consumes retained metadata from `internal/resultcache`. Route every event in Task 1 through one authoritative transition seam: preserve the active execution and its ability to own future serialized page requests across editing, overlays, save/export/copy, estimates, query-history navigation/restoration, resize, count/page settlement, count failure, and idle time; deactivate only for the PRD's exhaustive finalizing events. Starting an actual execution must finalize before replacing the active execution, entering result history must deactivate and invalidate future page mutation, terminal cancellation/failure must end it after settlement, and accepted quit must perform required cancellation/settlement before finalization. Keep execution, request, and viewport-generation identities distinct so late request messages cannot reactivate or mutate a finalized SELECT. Implement only enough to make Task 1 pass; exactly-once snapshot creation belongs to Tasks 3-4.

---

### 3. Specify exactly-once snapshot finalization

**Type**: RED
**Output**: Failing tests cover success, count failure, partial page failure, cancellation/failure before and after rows, new execution, result history, and accepted quit.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend lifecycle tests in `internal/ui` and immutable entry tests in `internal/history`, using `internal/resultcache` fixtures, to require exactly one finalized snapshot for every actual SELECT execution. Cover ordinary successful idle finalization, successful rows with count unavailable, partial page failure after retained rows, cancellation and terminal failure before any rows, cancellation and failure after rows, starting each kind of actual new execution, entering result history, and accepted quit with idle or pending count/page work. Require success, count failure with rows, partial page failure, and cancellation/failure after rows to preserve immutable captured rows and Issue #33 metadata; require cancellation before rows to create one non-tabular Cancelled entry and first-page failure before rows to create one error entry. Replay duplicate finalizer messages, late count/page success and failure, repeated cancellation settlement, old execution IDs, repeated history-entry commands, and quit cleanup messages, asserting no second entry and no mutation of the first. Keep this task test-only and assert finalization is per execution, never per page or count request.

---

### 4. Implement immutable SELECT finalization

**Type**: GREEN
**Output**: Every execution creates exactly one correctly typed snapshot and duplicate/late finalization is harmless.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one idempotent finalization path across `internal/ui` and `internal/history`, taking an immutable ascending copy of retained rows and authoritative metadata from `internal/resultcache` exactly once for the active execution ID. Build tabular success/count-failed/partial-failed/cancelled-after-rows snapshots with Issue #33 completeness, endpoint, eviction, UTF, failure, and terminal-outcome metadata, and build the defined non-tabular Cancelled or error entries when cancellation or first-page failure occurs before rows. Invoke this seam only from actual-new-execution start, result-history entry, active-ending cancellation/failure, and accepted-quit cleanup; mark the execution finalized before any duplicate or late message can append or rewrite it. Reject stale request/generation results, preserve captured data against later cache or source mutation, and make repeated finalization calls deterministic no-ops. Implement only enough to make Tasks 1 and 3 pass without adding result-history navigation owned by Issue #36.

---

### 5. Document active SELECT lifetime

**Type**: DOCUMENT
**Output**: Wiki documentation records active/request distinction, exhaustive event lists, and snapshot outcomes.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #34 implementation and tests from `internal/ui`, `internal/history`, and `internal/resultcache` into the appropriate pages under `Notes/wiki`. Document the distinction among active SELECT, execution ID, individual request ID, and viewport generation, and record the exhaustive finalizing list: actual new execution, entering result history, cancellation/failure ending the SELECT, and accepted quit. Record every tested nonfinalizing category—builder/focus edits, overlays/help, save/export/copy, estimate workflows, query-history navigation/restoration, resize, paging, individual count/page completion or count failure, and idle periods—and clarify invalid execution, validation, estimate opening, and rejected quit behavior. Describe exactly-once immutable finalization and the tabular versus non-tabular outcomes for success, count failure, partial page failure, and cancellation/failure before or after rows, including duplicate/late-message idempotence. Cross-reference Issues #26, #33, and #34 and the Identities and state, SELECT lifecycle, context/action matrix, UI/History Module Design, and active-finalization Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the SELECT-lifecycle walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/034-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/034-06/code-walkthrough`. Start active SELECTs with independent page and count requests, then demonstrate that editing, overlays/help, save/export/copy, estimation, query-history browsing, resize, page/count settlement, count failure, and idle periods preserve activity. Exercise each finalizer separately: a new actual SELECT and write execution, result-history entry, ending cancellation/failure, and accepted quit with idle and pending work. Capture exactly one immutable entry for success, count-failed rows, partial page failure, cancellation/failure before and after rows, showing correct tabular/non-tabular outcomes and retained metadata. Replay duplicate finalization and late old-request messages to prove they are harmless. Reference Issue #34 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
