# Tasks for #27: In-flight feedback and execution gating (SELECT/page/count)

Parent issue: #27
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify generic request-in-flight gating

**Type**: RED
**Output**: Failing scripted tests cover Enter, query/result history, save/export, horizontal interaction, quit, cancellation, request counts, and context precedence.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven scripted `(model, msg) → (model, cmd)` tests in `internal/ui` for the generic request-in-flight row of the PRD's Global Key Precedence and Context/Action Matrix, using the controllable `internal/connection` fake to hold requests and count dispatches. Exercise the same gates with SELECT first-page, later-page, and count work pending: Enter must be consumed without another execution; Ctrl+P/N, Ctrl+E/Y, Ctrl+S, and Ctrl+X must be rejected; permitted one-column horizontal navigation must remain local; `q` and Ctrl+C must open the shared quit confirmation; and Ctrl+W must reach cancellation only for a cancellable request. Assert unchanged database request counts and explanatory feedback for every blocked action. Cover terminal, quit-confirmation, top-overlay, focused text/search, request-pending, and base-context precedence so higher contexts consume keys first, and prove behavior derives from generic in-flight state rather than SELECT phase-label strings. Keep this task test-only and exclude write-phase feedback/integration owned by Issue #44.

---

### 2. Implement generic in-flight action gating

**Type**: GREEN
**Output**: Blocked/permitted action and no-stacking tests pass independently of phase labels.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement a generic request-in-flight action gate in `internal/ui` at the authoritative precedence point between focused input/overlays and base-context handling. Derive pending state from active request ownership/settlement supplied by the `internal/connection` orchestration, not rendered phase labels, and consume execution, query/result history, save, and export actions with contextual feedback and no command dispatch. Preserve accepted quit suspension/cleanup, scoped cancellation routing, base help where allowed, and local horizontal result-column movement; Page keys remain governed by Issue #25's page-pending rule. Reuse this seam for SELECT first-page, later-page, and count requests without importing UI concerns into `internal/querybuilder` or `internal/connection`, and implement only enough to make Task 1 pass without taking ownership of Issue #44's write-phase integration.

---

### 3. Specify SELECT/page/count feedback

**Type**: RED
**Output**: Failing rendering/model tests require `Running…`, `Counting rows…`, page-loading state, Ctrl+W hints, and explanatory rejections.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused model and rendering assertions in `internal/ui` for responsive read-request feedback while fake `internal/connection` first-page, count, and later-page requests are independently held and settled. Require exact `Running…` feedback for initial SELECT page work, exact `Counting rows…` while the independent count is pending, and a distinct visible page-loading state during later navigation; verify labels update correctly when page and count settle in either order and remain visible through permitted local interaction. For Enter during each pending read phase, require consumption, no stacked request, and a Ctrl+W cancellation hint; for query/result history and save/export, require action-specific explanatory rejection. Include cancellation-requested `cancelling…` handoff without duplicating Issue #28's interrupt semantics, count failure transition to the established `Count unavailable`, and context-precedence controls. Keep this task test-only and assert that no write-phase label is introduced or changed.

---

### 4. Integrate read-request phase feedback

**Type**: GREEN
**Output**: Responsive labels and gating tests pass without taking ownership of write-phase feedback.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire SELECT first-page, count, and later-page lifecycle state from the existing `internal/connection` commands into `internal/ui` status/help rendering. Show `Running…`, `Counting rows…`, page-loading feedback, Ctrl+W hints, `cancelling…`, count-unavailable state, and blocked-action explanations according to actual request ownership and settlement while preserving Issue #24's independent count and Issue #25's serialized pages. Ensure rendering remains responsive under horizontal movement, resize, overlays, and quit suspension, and ensure feedback from a stale response cannot overwrite a current phase under Issue #26's guards. Keep generic gating independent of labels, leave query construction in `internal/querybuilder`, and do not add, rename, or assume ownership of estimating/beginning/executing/rollback/commit write feedback reserved for Issues #41–#44. Implement only enough to make Tasks 1 and 3 pass.

---

### 5. Document in-flight UI contracts

**Type**: DOCUMENT
**Output**: Wiki documentation records generic gates, allowed local interaction, read-phase labels, and Issue 44’s write boundary.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #27 implementation and tests from `internal/ui` plus its `internal/connection` request-state and `internal/querybuilder` execution boundaries into the appropriate pages under `Notes/wiki`. Document the generic request-in-flight gate independently from phase text; list blocked Enter, query/result history, save, and export actions, permitted local horizontal interaction, quit-confirmation behavior, scoped Ctrl+W routing, unchanged request counts, and the terminal/overlay/input/pending/base precedence order. Record the exact SELECT/page/count feedback contracts for `Running…`, `Counting rows…`, page loading, `cancelling…`, Ctrl+W hints, explanatory rejection, and count failure. Explicitly state that Issue #27 integrates only read requests and that Issue #44 owns write-phase labels and generic-gate integration for writes. Cross-reference Issues #24–#28 and #44 and the Runnable-State Contract, Execution and Result Lifecycle, Global Key Precedence and Context/Action Matrix, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the in-flight-feedback walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/027-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/027-06/code-walkthrough`. Hold SELECT first-page, count, and later-page requests independently and demonstrate `Running…`, `Counting rows…`, page-loading state, Ctrl+W hints, `cancelling…`, and the settled/count-unavailable transitions. During each phase, exercise Enter, both histories, save/export, horizontal movement, quit, and cancellation; capture explanatory rejection, unchanged request counts, preserved local interaction, and correct context precedence. Include evidence that generic gates do not inspect labels and that no write-phase feedback was changed, identifying Issue #44 as that integration boundary. Reference Issue #27 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review request gating

**Type**: REVIEW
**Output**: Human confirms SELECT/page/count pending behavior across all relevant keys.
**Depends on**: 6

Review generic gates and read-phase rendering in `internal/ui`, request state from `internal/connection`, QueryBuilder execution boundaries, wiki updates, and `Notes/walkthroughs/027-06/code-walkthrough` against Issue #27 and the authoritative key matrix. Hold SELECT first-page, count, and later-page work pending in turn and together; press Enter, history, save/export, horizontal, quit, cancellation, help, and Page keys in base, popup/input, overlay, and quit-confirmation contexts. Confirm exact phase feedback and Ctrl+W guidance, no stacked database requests, explanatory blocked-action messages, permitted local movement, and correct higher-context consumption. Verify no write-phase label or ownership from Issue #44 was introduced before approving the issue.

---
