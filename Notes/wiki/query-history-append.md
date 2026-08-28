# Issue #20: Minimal query-history append (stable ID + consecutive suppression)

The session-only query-history storage contract, its normalized comparison
equality, and the execution-start append timing, per the **History** and
**Execution and Result Lifecycle** decisions in `Notes/PRD-sqloid.md`.
Ownership is split three ways: `internal/querybuilder` owns the canonical
normalized snapshot and its equality, `internal/history` owns stable IDs,
chronological storage, the 20-entry cap, and the consecutive-suppression
policy, and `internal/ui` owns only the timing — calling the single append
entry point exactly when an actual execution starts.

## Storage (`internal/history/query_store.go`)

- **Stable IDs**: `EntryID` values are nonzero, allocated monotonically by
  `Store.Append`, never reused, and never renumbered. They are identities, not
  positions: an entry keeps its ID for its whole lifetime even as older
  entries are evicted and its slice index changes. Zero is never allocated;
  functions return it to mean "none" (e.g. a suppressed append).
- **Immutable complete states**: each retained `Entry{ID, State}` holds a deep
  copy of the history-ready `qb.HistoryState` taken at append time. Mutation
  of the source builder, the appended state, or any retrieved value
  (`Entries()`, `Lookup()`) can never alter a retained entry — every
  mutable slice is freshly copied on append and on retrieval.
- **Chronological order**: entries are retained oldest first;
  `Entries()` returns a fresh slice in exactly that order, addressable by
  stable ID through `Lookup(id)`, which reports not-found (no error
  distinction) for evicted or never-allocated IDs.
- **Capacity and eviction**: `Capacity` is exactly 20. Each retained append
  beyond 20 evicts exactly the oldest entry before the new list is exposed,
  preserving all surviving IDs and their order. Empty storage is
  deterministic: `Len()` 0, empty `Entries()`, `Lookup` not-found.
- **No policy leakage**: `Append` performs no suppression and makes no
  validation or timing decisions; the package has no database and no
  Bubble Tea dependency. Repeated identical payloads each receive their own
  ID at the storage layer.

## Normalized execution state (`internal/querybuilder/history_state.go`)

`QueryBuilder.HistoryState()` returns the canonical `HistoryState` — only
fields significant for history comparison and restoration, with every slice
freshly allocated and no mutation of the receiver:

- **Command** and the **stable table identity** (`Table`/`TableSet`).
- **Ordered projection entries** (`Kind`, `Column`, `Aggregate`) in commit
  order.
- **WHERE**: presence, column identity, operator, and — when the completed
  predicate submitted a value — the parsed `Value` (concrete bound kind:
  INTEGER/REAL/TEXT) and the exact entered representation byte-for-byte.
- **GROUP BY** names in acceptance order.
- **ORDER BY** as the committed expression key plus direction.
- **Limit** as the empty-versus-accepted-number distinction only (`LimitHas`,
  `LimitValue`); the entered bytes ("5" vs "05") are transient once the same
  integer is accepted.
- **UPDATE SET assignments** in SET order and **INSERT per-column choices** in
  declared order, each carrying the structural choice (Value/NULL /
  Value/NULL/Default/Omit) and, for submitted Value choices, the parsed value
  and exact entered text.

Deliberately excluded transient state: focus, popup/input cursors, open
drafts, inline errors, layout, request IDs, and catalogs.

`HistoryState.Equal` proves each of these significant — entered
representation (`"7"` vs `"07"`), concrete bound type (TEXT `"7"` vs INTEGER
`7`), structural choice (typed `NULL` vs the SQL-NULL choice), column order,
projection order, group order, ORDER BY expression/direction, and Limit
empty-vs-number vs a different number — even where rendered SQL or bound
database values could match. Invalid Limit text can never be appended because
it is never runnable.

## Append policy (`internal/history/query_append.go`)

`Store.AppendExecution(state)` is the single append entry point. It compares
only against the **immediately preceding retained execution**:

- **A→A** suppresses only the latter append — no ID is allocated, nothing is
  evicted, storage is untouched — and returns `(0, false)`.
- **A→B→A** retains both A entries with distinct stable IDs.
- Otherwise the state is deep-copied and appended through the stable capped
  store.

## Append timing (`internal/ui/model.go`)

The UI owns only timing, via one seam:

- `ExecutionStartedMsg` is the actual-execution-start lifecycle message.
  Handling it calls `appendQueryHistoryAtExecutionStart()`, which appends the
  current `QB.HistoryState()` through `AppendExecution` — but only for
  **SELECT and INSERT**. UPDATE/DELETE append only when destructive
  confirmation begins the sole actual write (Issues #37/#38); no implemented
  flow can emit their start yet, so they never append through this seam.
- **Never append**: runnable evaluation (`Enter` on runnable data emits only
  `PreExecutionRequestedMsg`, which appends nothing), pre-execution schema
  validation, destructive estimation (open/complete/fail/cancel), confirmation
  dismissal, and mere runnable checks. Issue #22 will emit
  `ExecutionStartedMsg` only after successful validation, so failed actual
  executions still retain the entry appended at start.
- A nil `Model.History` store is an unchanged no-op.

## Deferred to Issue #35

Ctrl+P/N navigation, restoration into the builder, history cursors, result
history, and selected-entry eviction fallback are explicitly **not** part of
Issue #20. Storage makes no navigation decision and the UI renders no history
surface.

Cross-references: [Issue #19 runnable feedback](runnable-state-feedback.md)
for the pre-execution seam, and the **History implementation decision**,
**Execution and Result Lifecycle**, **QueryBuilder**, **UI**, and **Testing
Decisions** in `Notes/PRD-sqloid.md` (Issues #19, #20, #22, and #35).
