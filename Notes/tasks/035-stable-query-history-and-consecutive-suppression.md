# Tasks for #35: Stable query history and consecutive suppression

Parent issue: #35
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify query-history navigation and immutable restoration

**Type**: RED
**Output**: Failing History/model tests cover Ctrl+P/N cursors, every builder field, copy-on-restore, editing restored state, and no append during browsing.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add pure cursor/navigation tests in `internal/history` and scripted model tests in `internal/ui` for Issue #35, preserving Issue #20's stable-ID store and append policy unchanged. Specify Ctrl+P as movement toward older retained queries and Ctrl+N as movement toward newer entries, including first entry into history, repeated boundary no-ops, direction reversal, current stable-ID lookup, and exit or base behavior at the newest boundary according to the model contract. Restore an immutable copy of every complete builder field represented by the Issue #20 normalized state: command, stable table identity, ordered projection/aggregate entries, WHERE presence/column/operator/entered representation/bound type, GROUP BY order, ORDER BY expression/direction, Limit empty versus number, ordered UPDATE Value/NULL assignments, and ordered INSERT Value/NULL/Default choices and values. Mutate source history, retrieved copies, and restored builder slices; edit each restored field and prove no retained entry changes. Assert browsing, cursor movement, restoration, and edits append nothing and consume no stable ID. Keep this task test-only and retain A→A suppression and A→B→A behavior as regression assertions rather than reimplementing them.

---

### 2. Implement stable query-history navigation

**Type**: GREEN
**Output**: Navigation/restoration tests pass while preserving Issue 20’s append and suppression implementation.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the existing Issue #20 query-history store in `internal/history` with a stable-ID cursor and pure older/newer/current navigation primitives, then integrate Ctrl+P/N handling and builder restoration in `internal/ui`. Resolve selection by stable ID rather than slice index, deep-copy the complete stored state on every retrieval/restoration, and let subsequent builder edits affect only current UI state. Keep navigation read-only: it must not call or duplicate the sole actual-execution append path, allocate IDs, change chronological order, or alter immediate-predecessor consecutive suppression. Preserve all normalized comparison distinctions and exact 20-entry oldest-first storage semantics from Issue #20, including A→A suppression and A→B→A retention. Implement deterministic empty and boundary behavior and only enough to make Task 1 pass; execution exit and eviction fallback belong to Tasks 3-4.

---

### 3. Specify execution exit and eviction fallback

**Type**: RED
**Output**: Failing tests cover leaving history before execution, current restored state execution, selected eviction, exact notice, new-oldest fallback, empty return, and no missing backing state.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui` model tests and `internal/history` stable-ID cursor tests to specify execution and defensive eviction behavior. When an actual SELECT or write execution starts while a query-history entry is selected, require query-history mode to exit before Issue #20's append/execution path runs and require the current restored-and-possibly-edited builder state—not a fresh lookup or stale backing entry—to execute. Exercise append suppression and append-caused oldest eviction, including selected oldest, middle, newest, and already edited restored states, and prove normal execution cannot leave selection pointing at an evicted entry. Separately simulate an externally driven store append or replacement that evicts the selected stable ID: move selection to the new oldest retained entry and show exactly `Previously viewed query was evicted from history`; if history is empty, return to the base builder view. Assert no missing backing state is rendered or restored at any intermediate step, surviving stable IDs remain unchanged, and current builder data remains valid. Keep this task test-only and include Issue #20 regressions for exact capacity, normalized full-field comparison, append timing, A→A suppression, and A→B→A retention.

---

### 4. Implement query-history UI safety

**Type**: GREEN
**Output**: Execution-exit and stable-ID eviction tests pass with Issue 20 regressions green.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement query-history mode exit and defensive selected-entry validation across `internal/ui` and `internal/history`. Before any actual execution reaches the canonical Issue #20 append seam, detach the history cursor while preserving the current deep-copied, potentially edited builder state as the execution input. After every history mutation or external append, resolve the selected stable ID against retained entries; if absent, select and restore a copy of the new oldest and surface exactly `Previously viewed query was evicted from history`, or clear history mode and return safely to the base builder when no entries remain. Never render, restore, or execute through an absent backing entry, and do not use list indices as identity. Reuse rather than replace Issue #20's append timing, stable ID allocation, immutable storage, 20-entry cap, full normalized comparison, and consecutive suppression. Implement only enough to make Tasks 1 and 3 pass without adding result-history behavior.

---

### 5. Document query-history navigation

**Type**: DOCUMENT
**Output**: Wiki documentation records navigation, restoration, append ownership, execution exit, and eviction behavior.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #35 implementation and tests from `internal/history` and `internal/ui`, along with the unchanged Issue #20 contracts they exercise, into the appropriate pages under `Notes/wiki`. Document Ctrl+P/N older/newer cursor behavior and boundaries, stable-ID selection, complete immutable copy-on-restore for every builder field, safe edit-after-restore behavior, and the rule that browsing never appends. Record Issue #20 as the sole owner of actual-execution append timing, full normalized comparison, stable IDs, 20-entry cap, oldest-first eviction, A→A suppression, and A→B→A retention. Explain that actual execution exits history first and executes the current restored state, then document defensive selected-ID eviction, exact notice `Previously viewed query was evicted from history`, new-oldest copied fallback, empty-history return to base, and the prohibition on missing backing state. Cross-reference Issues #20, #34, and #35 and the Execution and Result Lifecycle, SELECT lifecycle, context/action matrix, History/UI Module Design, and history Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the query-history walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/035-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/035-06/code-walkthrough`. Execute A→A→B→A to show only the consecutive duplicate suppressed, then browse older/newer entries with Ctrl+P/N through both boundaries and reverse direction. Restore examples covering every builder field, mutate restored/source/retrieved values, and edit restored state while proving retained history remains immutable and browsing creates no append. Start actual executions from unchanged and edited restored states, capturing history-mode exit before the unchanged Issue #20 append path and execution of current state. Force selected stable-ID eviction with surviving entries and with an empty store, showing the exact query notice, new-oldest fallback, base return, unchanged surviving IDs, and no missing backing state. Reference Issue #35 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
