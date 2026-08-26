# Tasks for #46: Deletion and replacement terminal workflow

Parent issue: #46
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify typed-health terminal mapping

**Type**: RED
**Output**: Failing model tests map Issue 7 classifications to both exact terminal strings and prohibit database work.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven model tests in `internal/ui` that inject Issue #7's typed deleted-file and same-path-replacement health classifications at request boundaries and during ordinary activity. Require deleted-file classification to enter a terminal state whose primary message is exactly `Database file no longer exists — session ended`, and replacement classification to enter the distinct terminal state whose primary message is exactly `Database file was replaced — session ended`. Assert these UI strings are defined only in `internal/ui`, not duplicated in the health/connection layer, and that typed classification rather than error-text matching selects them. From either state, exercise every database-capable key/path and require no validation, execution, paging, refresh, rerun, health request, or other database command. Keep this task test-only and require that no transaction or driver work is pending on entry.

---

### 2. Implement deletion/replacement terminal entry

**Type**: GREEN
**Output**: Exact-message and terminal-transition tests pass with UI strings defined only here.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement typed health-to-terminal mapping in `internal/ui`, consuming Issue #7's deletion and same-path-replacement classifications without parsing driver text. Define the two exact user-facing messages at this UI ownership point, transition atomically into the corresponding terminal state only after no transaction or driver work remains, and suppress all database-starting actions before constructing commands. Preserve immutable in-memory histories for later navigation and keep the health layer free of terminal UI wording. Implement only enough to make Task 1 pass without adding history, reduced-help, or quit behavior.

---

### 3. Specify populated and empty history behavior

**Type**: RED
**Output**: Failing tests cover initial newest selection, Ctrl+P/N and Ctrl+E/Y, empty no-op fallback, primary terminal message, and no synthetic/missing-backed entry.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted tests in `internal/ui` backed by immutable query and result stores in `internal/history` for both deletion and replacement terminals. With populated result history, require terminal entry to select the newest stable-backed result and Ctrl+E/Y to navigate older/newer immutable entries; require Ctrl+P/N to navigate complete query-history states, all without database commands. Cover deterministic boundaries and independently empty query or result histories. When result history is empty, require selection to remain empty, Ctrl+E/Y to be a no-op, and the exact deletion/replacement terminal message to remain the primary view; no synthetic result, absent stable ID, stale columns/rows, or missing-backed entry may render. Keep this task test-only and verify both typed terminal variants in every populated/empty case.

---

### 4. Implement terminal history navigation

**Type**: GREEN
**Output**: Populated/empty in-memory history tests pass without database requests.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement local terminal-history selection across `internal/ui` and `internal/history` for both deletion and replacement states. On entry, select the newest immutable result only when a backing entry exists; otherwise keep result selection empty and preserve the exact terminal message as the primary view. Route Ctrl+P/N and Ctrl+E/Y through stable in-memory older/newer primitives with deterministic no-op boundaries, clear any stale projection when backing data is absent, and never manufacture a terminal result. Ensure entry, navigation, resize, and rendering produce no connection or database requests, and implement only enough to make Task 3 pass.

---

### 5. Specify reduced help and immediate quit

**Type**: RED
**Output**: Failing tests require only available in-memory actions, no database suggestions, and q/Ctrl+C immediate status 1.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend scripted `internal/ui` tests for both deletion and replacement terminals, with populated and empty histories. Require `?` to open reduced help listing only available in-memory history selection, help dismissal, separately owned save/export actions, and immediate quit keys; prohibit execution, refresh, paging, rerun, cancellation, or any other database suggestion. From the primary message, selected query/result history, and reduced help, require `q` and Ctrl+C to exit immediately with status 1, bypass confirmation, and schedule no cancellation, cleanup, connection, or delayed command. Reassert that all normally database-capable keys remain blocked and keep this task test-only; Ctrl+S and Ctrl+X behavior remains owned by Issues #48 and #49.

---

### 6. Implement terminal help and exit behavior

**Type**: GREEN
**Output**: Reduced-help, work-prohibition, and immediate-quit tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement shared deletion/replacement terminal input handling in `internal/ui`. Render reduced help from the actions actually available in these in-memory states, with no database-recovery or new-work guidance, and preserve the appropriate exact terminal message when help closes. Give `q` and Ctrl+C precedence in every terminal subview so they exit immediately with status 1 and never open ordinary quit confirmation or issue asynchronous work. Keep the database-action gate authoritative across primary, history, and help views, preserve the no-pending-work invariant, and implement only enough to make Task 5 pass.

---

### 7. Document deletion/replacement terminals

**Type**: DOCUMENT
**Output**: Wiki documentation records typed mapping, exact messages, history fallbacks, restrictions, help, and quit.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #46 implementation and tests from `internal/history` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the mapping from Issue #7's typed deletion and same-path-replacement classifications to the exact UI-owned messages `Database file no longer exists — session ended` and `Database file was replaced — session ended`. Record terminal entry with no pending transaction/driver work, complete database-work prohibition, newest immutable result selection when populated, empty selection and primary-message fallback when empty, Ctrl+P/N and Ctrl+E/Y local navigation/no-op behavior, no synthetic or missing-backed entries, reduced help, and immediate status-1 `q`/Ctrl+C without confirmation. Identify Issues #48 and #49 as save/export owners, cross-reference Issues #7, #36, #45, and #46 plus the terminal context/action rules in `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md` for any added or removed page, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the health-terminal walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/046-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/046-08/code-walkthrough`. Trigger Issue #7's typed deletion and replacement classifications and capture each exact terminal message, UI-only string ownership, settled/no-pending-work entry, and rejection of every database-capable action. For each terminal, exercise populated query/result histories, initial newest result selection, Ctrl+P/N and Ctrl+E/Y boundaries, and empty-history fallback with the primary message, no synthetic entry, and no stale or missing-backed rows. Open reduced help and verify only available actions appear, then demonstrate both `q` and Ctrl+C exiting immediately with status 1 and no confirmation. Reference Issue #46 and `Notes/PRD-sqloid.md`, distinguish later save/export ownership, and place every showboat-generated artifact under the approved directory.

---

### 9. Review deletion/replacement behavior

**Type**: REVIEW
**Output**: Human triggers both states with populated/empty histories and verifies actions and exit status.
**Depends on**: 8

Review typed terminal mapping and interaction in `internal/ui`, immutable selection behavior in `internal/history`, the wiki updates, and `Notes/walkthroughs/046-08/code-walkthrough` against Issue #46. Manually trigger both deletion and same-path replacement with populated and empty query/result histories; verify exact messages, newest selection or empty primary fallback, Ctrl+P/N and Ctrl+E/Y navigation/no-ops, and absence of synthetic, stale, or missing-backed results. Attempt all database actions, inspect reduced help for only valid in-memory actions, and confirm no command starts. Use `q` and Ctrl+C from primary, history, and help contexts and verify immediate status 1 without confirmation or pending work before approving the issue.

---
