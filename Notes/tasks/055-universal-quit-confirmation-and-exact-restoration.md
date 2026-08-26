# Tasks for #55: Universal quit confirmation and exact restoration

Parent issue: #55
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify universal quit suspension

**Type**: RED
**Output**: Failing full-matrix tests require q/Ctrl+C to open one confirmation in every nonterminal enabled context while preserving the exact suspended state.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add a full table-driven quit matrix in `internal/ui`, extending Issue #54's centralized precedence fixtures and fake Connection instrumentation. In every enabled nonterminal base builder/result state, focused input, searchable and scroll-only popup, help or other top overlay, schema-validation state, estimate/preparation phase, write confirmation, active SELECT page/count state, picker/save/overwrite state, pending cleanup/commit state, and too-small screen, require q and Ctrl+C to open the same single quit confirmation except that q remains literal input when focused text/search owns it. Capture the complete suspended context: typed overlay and opener, focus/cursor/search/highlight, builder/result/history selection, viewport and first visible row/column, active execution/request/preparation identities and phase, immutable export/save copy, destination/format/path, warnings/errors, and pending settlement state. Require ordinary overlays not to stack while quit alone may suspend one, repeated q to no-op in confirmation, and no quit-opening key to leak into text, navigation, cancellation, save, or database work. For deletion, replacement, and outcome-unknown terminal states, reassert immediate status-1 q/Ctrl+C with no confirmation. Keep this task test-only and defer accepted-quit cleanup to Tasks 5-6.

---

### 2. Implement quit confirmation and suspension

**Type**: GREEN
**Output**: Base, input, popup, overlay, pending, preparation, save, and too-small contexts suspend correctly; terminal states remain immediate.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement universal quit routing at the terminal/quit levels of the centralized `internal/ui` dispatcher. Preserve terminal precedence so deletion, replacement, and outcome-unknown states emit immediate status-1 exit; elsewhere route Ctrl+C, and q when not owned by focused text/search, into one shared confirmation. Suspend rather than replace the exact current context in a typed quit frame that retains the active top overlay and its opener, focus/edit/search/viewport state, builder/result/history selection, active request or preparation identities/phases, immutable save/export state, destination/format/path, and too-small wrapper. Allow quit to suspend at most one existing overlay, reject repeated opens, consume all confirmation keys according to the PRD, and issue no cancellation or lifecycle command merely by opening. Keep underlying asynchronous work represented by identity so later settlement can update the suspended state. Implement only enough to make Task 1 pass without accepted-quit coordination.

---

### 3. Specify quit-cancellation restoration

**Type**: RED
**Output**: Failing tests cover Esc/n, focus, overlay/search/viewport, settled-behind-quit estimates, overwrite destination/format/copy/selection/path, and no key leakage.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the `internal/ui` quit matrix with Esc and n cancellation from every suspended nonterminal context. Require exact restoration of focus, cursor, overlay type and opener, popup/picker search and highlight, completed selections, result first row/column and viewport generation, builder/history selection, errors/warnings, too-small hidden state, and active request phase. Hold in-flight estimate and schema-validation work behind quit, deliver current and stale settlement messages, and require cancellation to reveal the latest identity-valid settled state rather than the stale opening snapshot; an awaiting estimate must retain its result and confirmation-enabled state. For overwrite, assert restoration of destination, format, immutable payload/copy, source selection, directory/filename path, warnings, confirmation state, and intact subsequent overwrite-cancel return path without reserialization. Cover active page/count and write-phase progression behind quit, duplicate/late responses, Esc versus n parity, repeated cancellation keys, and exact command counts proving neither key dismisses the restored overlay, edits input, navigates, cancels work, or leaks to a lower handler. Keep this task test-only and do not accept quit in these cases.

---

### 4. Implement latest-state restoration

**Type**: GREEN
**Output**: Exact restoration tests pass for every suspended context.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement Esc/n quit cancellation in `internal/ui` by removing only the quit frame and restoring the suspended context's latest identity-valid state. While confirmation is visible, route asynchronous schema-validation, estimate, SELECT, write, save, and other completion messages into the suspended model using their ordinary stale/duplicate guards, without allowing new user actions behind the modal. Preserve exact opener/focus/cursor/search/highlight/viewport and immutable workflow data when no settlement changes them; where work settles, restore the newly settled phase, result, error, or confirmation-enabled state while retaining unrelated presentation state. Keep overwrite destination/format/copy/selection/path and its parent save flow intact, preserve too-small wrapping, and ensure Esc/n is consumed by quit so it cannot also close the revealed overlay or alter state. Implement only enough to make Task 3 pass without starting accepted-quit cleanup.

---

### 5. Specify accepted-quit lifecycle cleanup

**Type**: RED
**Output**: Failing tests cover schema-validation cancellation/no history, active SELECT cancellation/finalization, estimate/preparation cleanup, write rollback/commit resolution, outcome unknown, and exit after settlement only.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-controlled accepted-quit tests across `internal/ui`, `internal/connection`, and `internal/history`, using existing cancellation, SELECT-finalization, preparation, write-phase, and outcome fakes. Accept with Enter, y, and Ctrl+C and require idle nonterminal contexts to exit with the ordinary successful status only after any owned cleanup is complete. During pre-execution schema validation and destructive estimation, require one scoped cancellation request, visible/retained settlement, no actual execution, no query or result history, and late success classified cancelled. During active SELECT, independently hold page and count requests, require cancellation of every active request, no lease reuse, stale/late-success rejection, exactly-once finalization under the settled cancellation/success state, and direct finalization for an idle active SELECT. During awaiting confirmation or other preparation, require dismissal with no history. For writes, cover beginning/executing cancellation followed by confirmed rollback, rollback cleanup and committing with no interrupt, successful commit, failed/unresolved rollback or commit, and outcome-unknown metadata; require no exit while transaction, driver, lease, finalization, or cleanup work remains. Assert duplicate acceptance/settlement is idempotent, terminal status-1 exceptions remain immediate, and no goroutine, request, transaction, or history append is abandoned. Keep this task test-only and use synchronization barriers rather than sleeps except explicit cancellation bounds.

---

### 6. Implement accepted-quit coordination

**Type**: GREEN
**Output**: All request/transaction cleanup and terminal status-1 exit tests pass without abandoned work.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one accepted-quit coordinator in `internal/ui` that delegates cleanup through typed lifecycle boundaries in `internal/connection` and exactly-once finalization in `internal/history`. For validation and estimation, request scoped cancellation once, await true settlement, discard late success, and exit with neither history appended. For active SELECT, cancel all current page/count work, retain execution/request identities and leases through settlement, finalize exactly once from the resulting immutable state, or finalize an idle SELECT directly. Dismiss awaiting preparation without execution. For writes, reuse Issue #43's pre-COMMIT cancellation and post-boundary noninterrupt rules: beginning/executing cancel and await rollback resolution; rollback cleanup/committing continue without interrupt; definite and outcome-unknown results finalize only after all transaction/driver work ends. Gate process exit on an explicit no-pending-work condition, make repeated acceptance and stale/duplicate completions harmless, block replacement work, and emit the correct ordinary nonterminal exit versus immediate terminal status 1. Implement only enough to make Task 5 pass without weakening cancellation bounds or outcome reporting.

---

### 7. Document universal quit behavior

**Type**: DOCUMENT
**Output**: Wiki documentation records confirmation, suspension/restoration, every cleanup path, terminal exceptions, and statuses.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #55 implementation and tests from `internal/ui`, `internal/connection`, and `internal/history` into the appropriate pages under `Notes/wiki`. Document q/Ctrl+C behavior in every Global Key Precedence matrix context, focused-search q ownership, one shared nonterminal confirmation, quit's sole one-overlay suspension exception, and immediate terminal status-1 exits. Record every suspended field needed for exact focus/search/highlight/viewport/builder/history/request/preparation/save restoration, Esc/n no-leak cancellation, latest identity-valid settlement behind quit, awaiting-estimate restoration, and complete overwrite destination/format/copy/selection/path restoration. Describe accepted cleanup for schema validation, estimation/preparation, idle and pending active SELECTs, cancellable and post-boundary writes, confirmed rollback/commit, and unresolved outcome-unknown settlement; include no-history and exactly-once finalization rules, late-success rejection, no lease/work abandonment, exit only after no pending work, and ordinary versus terminal status. Cross-reference Issues #6, #21, #28, #34, #41-#46, #53-#55 and the Execution and Result Lifecycle, Writes and commit boundary, Global Key Precedence, UI/Connection/History Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the quit-lifecycle walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/055-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/055-08/code-walkthrough`. Open the shared confirmation with q and Ctrl+C from base, focused input/search, popup, help/overlay, validation, estimate, active SELECT, write phases, picker/save/overwrite, pending cleanup, and too-small contexts, noting focused-search q's literal behavior and immediate terminal status-1 exceptions. Cancel with Esc and n and compare exact focus, overlay/search/highlight, viewport, builder/history, request phase, and immutable save state; settle an estimate behind quit and show latest-state restoration, then restore and cancel an overwrite while retaining destination/format/copy/selection/path and parent flow. Accept quit during schema validation, estimation, idle and pending SELECT, beginning/executing write, rollback cleanup, commit, and unresolved outcomes; use barriers to show cancellation/finalization/rollback-or-commit resolution, no histories where required, exactly-once outcomes, and no exit until all work settles. Reference Issue #55 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 9. Review quit across all contexts

**Type**: REVIEW
**Output**: Human opens, cancels, and confirms quit from every specified context and verifies cleanup/restoration.
**Depends on**: 8

Review quit dispatch and suspension in `internal/ui`, lifecycle cleanup in `internal/connection`, finalization in `internal/history`, the wiki updates, and `Notes/walkthroughs/055-08/code-walkthrough` against Issue #55. Manually open q/Ctrl+C confirmation from every nonterminal matrix context, including focused text/search, every overlay, validation, estimate, active SELECT, each write phase, picker/save/overwrite, too-small, and pending cleanup; verify terminal states alone exit immediately with status 1. Cancel with Esc and n and compare exact focus, overlay/search/highlight, viewport, selection, request/preparation phase, and save destination/format/copy/path, including estimate settlement behind quit and a subsequent overwrite cancellation. Accept quit in every lifecycle category, inspect no-history preparation cancellation, active SELECT request settlement and exactly-once finalization, pre-boundary rollback, post-boundary commit/cleanup, and outcome-unknown handling. Approve only when no key leaks, no connection/lease/transaction/goroutine is abandoned, no exit occurs before settlement, and statuses match the documented terminal and nonterminal rules.

---
