# Tasks for #20: Minimal query-history append (stable ID + consecutive suppression)

Parent issue: #20
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify stable bounded query-history storage

**Type**: RED
**Output**: Failing pure tests cover stable IDs, immutable stored states, 20-entry capacity, and oldest-first eviction.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create pure table-driven storage tests in `internal/history/query_store_test.go` for the minimal in-memory query-history list. Require every retained append to receive a stable, nonzero identity that is not its slice position and never changes when older entries are evicted; lookups and returned lists must preserve chronological order and address entries by stable ID. Store a complete immutable copy of the normalized QueryBuilder execution state, including all slices, nested choices, entered values, and typed identities, so mutation of the source builder or a retrieved value cannot alter a retained entry. Require capacity to be exactly 20, the first 20 retained entries to remain ordered, and each subsequent retained append to evict exactly the oldest before exposing the new list while preserving all surviving IDs. Cover empty storage, repeated payloads at the storage layer, many evictions, and defensive copy behavior. Keep this task test-only, do not add navigation/cursors/restoration/selected-entry fallback from Issue #35, and do not make storage decide validation or execution timing.

---

### 2. Implement the minimal query-history store

**Type**: GREEN
**Output**: Stable-ID storage and eviction tests pass without navigation behavior.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the session-only query-history list in `internal/history/query_store.go`, consuming a history-ready immutable state representation from `internal/querybuilder` while keeping ownership of stable entry IDs, chronological storage, and the 20-entry cap in `internal/history`. Allocate monotonically stable identities independently of list indices, deep-copy all mutable slices and nested builder values on append and retrieval, and evict only the oldest retained entry when capacity is exceeded. Expose only the minimal append/list/lookup-by-ID operations needed by execution integration, with deterministic empty and not-found behavior and no database or Bubble Tea dependency. Do not add Ctrl+P/N navigation, restoration, cursors, selection notices, result history, or selected-entry eviction fallback assigned to Issue #35; keep consecutive suppression and execution timing for Tasks 3-4. Make Task 1 pass without mutating QueryBuilder state.

---

### 3. Specify normalized execution equality and append timing

**Type**: RED
**Output**: Failing tests cover all normalized fields, entered representation/bound type, empty-vs-number, only-actual-execution append, A→A suppression, and A→B→A retention.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exhaustive normalized comparison tests in `internal/querybuilder/history_state_test.go` and append-policy/lifecycle tests in `internal/history/query_append_test.go`, with narrow scripted integration coverage in `internal/ui/query_history_append_test.go` only where execution-start events are owned. Define equality over the full normalized execution state: command, stable table identity, ordered projection entries, WHERE presence/column/operator/entered value/parsed bound type, GROUP BY order, ORDER BY expression/direction, Limit empty versus accepted number, ordered UPDATE assignments and Value/NULL choices, and ordered INSERT Value/NULL/Default choices and values. Prove entered representation, concrete bound type, choice, column order, projection order, and group order are significant even when rendered SQL or database values could match; exclude transient focus, popup/input cursor, inline error, layout, and request IDs. Require append exactly when an actual SELECT or INSERT execution starts and when confirmed UPDATE/DELETE starts, only after successful pre-execution validation and any destructive confirmation; validation, estimation, cancellation, dismissal, and mere runnable checks never append. Assert consecutive A→A suppresses only the latter append, A→B→A retains both A entries, suppressed appends consume no stable ID or eviction, and failed actual executions still retain the entry appended at start. Keep this task test-only and use execution-start seams without implementing Issue #22 database execution.

---

### 4. Implement append and consecutive suppression

**Type**: GREEN
**Output**: Execution-start append and normalized consecutive suppression tests pass; validation/estimation never append.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the canonical history-ready execution snapshot and normalized equality in `internal/querybuilder/history_state.go`, preserving every significant field and exact entered/bound representation while omitting transient UI state. Add the append policy to `internal/history/query_store.go` or a focused `internal/history/query_append.go`: compare only against the immediately preceding retained execution, suppress an equal consecutive state without allocating an ID, and otherwise deep-copy and append through the stable capped store. Connect this single append entry point to the actual-execution-start lifecycle seam in `internal/ui/model.go` so SELECT/INSERT append only after successful validation as execution begins, and UPDATE/DELETE append only when confirmation begins the sole actual write; ensure runnable evaluation, schema validation, estimate opening/completion/failure/cancellation, and confirmation dismissal cannot call it. Keep failed execution outcomes in history because append already occurred at start, preserve A→B→A, and structure the seam for Issue #22 and later write flows without adding navigation or execution itself. Make Task 3 pass and keep ownership split between QueryBuilder normalization, History storage/policy, and UI lifecycle timing.

---

### 5. Document the minimal history contract

**Type**: DOCUMENT
**Output**: Wiki documentation records ownership, identity, comparison fields, append timing, suppression, capacity, and deferred navigation.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #20 implementation and tests from `internal/querybuilder`, `internal/history`, and the narrow `internal/ui` execution-start seam into the appropriate pages under `Notes/wiki`. Document session-only query-history ownership, stable IDs independent of positions, immutable deep-copied complete states, chronological order, exact 20-entry capacity, and oldest-first eviction. Enumerate every normalized comparison field and explain why entered representation, bound type, choice, empty-versus-number Limit, and ordering are significant while focus/layout/transient request state are excluded. Record append timing at actual execution start after validation and, for destructive writes, confirmation; failed executions remain appended, while runnable checks, validation, estimation, cancellation, and dismissal never append. Explain immediate-predecessor-only A→A suppression, A→B→A retention, suppressed-ID behavior, and that Ctrl+P/N navigation, restoration, cursors, and selected-entry fallback are explicitly deferred to Issue #35. Cross-reference Issues #19, #20, #22, and #35 and the Execution and Result Lifecycle, History implementation decision, QueryBuilder, UI, History, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the history-append walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/020-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/020-06/code-walkthrough`. Demonstrate stable non-positional IDs, immutable stored copies after source/retrieved-state mutation attempts, capacity 20, and repeated oldest-first eviction with surviving IDs unchanged. Exercise normalized comparison differences for command/table, projection order, WHERE operator/value/entered representation/bound type, GROUP BY order, ORDER BY/direction, Limit empty versus number, and UPDATE/INSERT ordered choices and values. Run the append policy through A→A→B→A and show only the consecutive duplicate suppressed without consuming an ID. Show no append during runnable evaluation, validation, estimation, cancellation, or dismissal, then append at the actual SELECT/INSERT start and confirmed UPDATE/DELETE start, including an execution that later fails. State clearly that navigation/restoration is deferred, reference Issue #20 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
