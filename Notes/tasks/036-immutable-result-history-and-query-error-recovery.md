# Tasks for #36: Immutable result history and query-error recovery

Parent issue: #36
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify bounded immutable result history

**Type**: RED
**Output**: Failing History tests cover stable IDs, 20-entry eviction, immutable snapshots, terminal-height reslicing, and zero refetch while browsing.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven result-store tests in `internal/history` and focused scripted browsing tests in `internal/ui`, using Issue #33 metadata and `internal/resultcache` row values, for Issue #36 and the Execution and Result Lifecycle, History Module Design, and history Testing Decisions in `Notes/PRD-sqloid.md`. Require each finalized actual execution to append one stable non-positional ID, retain exactly the 20 newest entries in chronological order, and evict oldest first without changing surviving IDs. Store deep immutable copies of tabular rows, columns, typed values including exact BLOB bytes, ascending absolute positions, snapshot metadata, and non-tabular outcomes; mutation of source or retrieved data must not alter history. Specify older/newer selection and deterministic boundaries independent of displayed slice indices. Given terminal height changes while browsing, require rows to be resliced locally from the selected immutable snapshot using current complete-row layout capacity, with no mutation of the stored snapshot. Assert entry selection, repeated navigation, resize/reslicing, and rendering issue zero database/page/count requests. Keep this task test-only and leave key binding, error replacement, and defensive selected eviction to later tasks.

---

### 2. Implement result-history storage and selection

**Type**: GREEN
**Output**: Snapshot storage, selection, navigation primitives, and reslicing tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the bounded result-history store and selection primitives in `internal/history`, accepting exactly-once finalized snapshots from Issue #34 and immutable metadata from Issue #33. Allocate monotonically stable IDs independent of indices, deep-copy all rows, columns, BLOB bytes, positions, metadata, reasons, and non-tabular details on append and retrieval, cap storage at 20 with oldest-first eviction, and expose stable-ID older/newer/current operations with deterministic empty and boundary behavior. In `internal/ui`, add a pure local projection that reslices the selected immutable snapshot for current terminal height and visible complete-row capacity without rewriting the stored entry or consulting `internal/resultcache` as live backing state. Keep selection/navigation free of Bubble Tea database commands and prove the only fresh-data path remains actual rerun. Implement only enough to make Task 1 pass; Ctrl+E/Y integration and query-error behavior belong to Tasks 3-4.

---

### 3. Specify result-history UI and query errors

**Type**: RED
**Output**: Failing model tests cover Ctrl+E/Y, execution exit, errors replacing results, Esc dismissal, older reachability, ordinary `database is locked`, and health override.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model tests in `internal/ui` with controllable request-result messages and stable entries from `internal/history`. Specify Ctrl+E/Y entry and older/newer traversal through tabular and non-tabular snapshots, boundary behavior, current-height reslicing, and zero database work. When an actual execution starts while result history is selected, require history mode to exit before execution and Issue #34 finalization proceed, so historical rows are no longer the active view. For an ordinary SELECT or write query error, require one lifecycle-defined finalized error entry to become the newest result and replace the visible result area; Esc dismisses the displayed error to the base builder/result context without deleting history, and older successful/error entries remain reachable through Ctrl+E/Y. Exercise first-page and later execution errors, and classify a request exceeding the five-second busy timeout with `database is locked` as an ordinary query error rather than terminal state. In paired cases where path deletion/replacement or another authoritative health classification is present, require that terminal health state to override the lock/query error. Keep this task test-only and assert no stale selected rows survive execution start or error replacement.

---

### 4. Integrate history browsing and error recovery

**Type**: GREEN
**Output**: Browsing/error tests pass without database requests or stale selected rows.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate Ctrl+E/Y result-history mode and query-error recovery across `internal/ui` and `internal/history`. Browse immutable snapshots by stable ID, project current-height slices locally, and issue no page, count, or other database request while entering, stepping, resizing, or rendering history. Before any actual execution begins, clear historical selection and stale displayed rows while preserving the selected entry in storage for later navigation; allow Issue #34 to finalize the new execution exactly once. On ordinary query failure, append/select the correctly typed lifecycle entry and replace the result view with its error; make Esc dismiss only the visible error and retain all entries so older results remain reachable. Treat five-second `database is locked` failures as ordinary query errors unless the existing health classification carried by the event identifies deletion, replacement, or another terminal override, in which case enter that terminal state instead. Ensure current selection always resolves to one backing immutable entry and no prior selected rows leak into base or error views. Implement only enough to make Tasks 1 and 3 pass; external-append eviction defense belongs to Tasks 5-6.

---

### 5. Specify defensive result eviction

**Type**: RED
**Output**: Failing tests cover exact notice, new-oldest fallback, empty base fallback, and never rendering evicted backing rows.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend stable-ID tests in `internal/history` and scripted selection/rendering tests in `internal/ui` to simulate defensive external appends or store replacement while a result-history entry is selected. Evict selected oldest, middle, and newest IDs under full and partially filled histories, including tabular snapshots, errors, and non-tabular outcomes. When the selected ID disappears and entries remain, require selection to move to the new oldest retained entry, reslice only that entry's immutable rows at current terminal height, and show exactly `Previously viewed result was evicted from history`. When no entry remains, require result-history mode to clear and return to the base builder/result fallback with no historical rows. Assert that no frame, intermediate model state, resize, navigation step, or dismissal can render rows, columns, metadata, or errors from the evicted backing entry; surviving stable IDs and snapshots remain unchanged and no database request is issued. Keep this task test-only and distinguish this defensive external-mutation path from normal execution, which exits history before append.

---

### 6. Implement stable result-selection fallback

**Type**: GREEN
**Output**: Defensive external-append eviction tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement defensive selected-ID reconciliation after every result-history mutation across `internal/history` and `internal/ui`. Resolve selection by stable ID before deriving any visible rows or metadata; if the backing entry was evicted and entries remain, select the new oldest retained ID, retrieve an immutable copy, reslice it for current terminal height, and surface exactly `Previously viewed result was evicted from history`. If history is empty, clear selection, historical projections, and error/snapshot references before returning to the base fallback. Make reconciliation atomic from the model's perspective so no render path can observe stale rows from a missing entry, preserve all surviving IDs and immutable data, and never trigger a refetch. Keep normal actual-execution exit behavior from Task 4 unchanged and implement only enough to make Task 5 pass.

---

### 7. Document result history and errors

**Type**: DOCUMENT
**Output**: Wiki documentation records immutability, navigation, reslicing, errors, dismissal, busy timeout, and eviction.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #36 implementation and tests from `internal/history`, `internal/resultcache`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document one immutable stable-ID snapshot per actual execution, exact 20-entry oldest-first retention, deep copies including BLOB bytes and metadata, Ctrl+E/Y navigation, local terminal-height reslicing, and zero refetch while browsing. Record execution exiting history before new work/finalization and the prohibition on stale selected rows. Explain query errors replacing the result view, lifecycle-defined error entries, Esc dismissal without deletion, continued older-result reachability, and five-second `database is locked` as an ordinary query error unless authoritative health classification overrides it with a terminal state. Document defensive selected-ID eviction with exact notice `Previously viewed result was evicted from history`, new-oldest fallback, empty base fallback, and never rendering missing backing rows. Cross-reference Issues #33, #34, and #36 and the Execution and Result Lifecycle, SELECT lifecycle, errors/cancellation implementation decision, context/action matrix, UI/History Module Design, and history/error Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the result-history walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/036-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/036-08/code-walkthrough`. Finalize successful tabular, empty, error, cancelled, and failed executions; browse them with Ctrl+E/Y while proving stable IDs, immutable rows/metadata/BLOB bytes after source and database changes, and zero refetch. Resize at multiple terminal heights and capture local reslicing from the same snapshots. Start a new execution from history and show selection/stale rows clear before finalization. Produce ordinary errors including `database is locked`, dismiss with Esc, reach older results, and contrast an authoritative terminal health override. Exceed 20 entries and force external selected-entry eviction with remaining and empty histories, showing the exact notice, new-oldest/base fallback, and no evicted rows. Reference Issue #36 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
