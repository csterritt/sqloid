# Issue #36: Immutable result-history browsing and query-error recovery

The user-facing result-history contract, per the **Execution and Result
Lifecycle** (finalization, history-entries, and SELECT lifecycle rules),
**History Module Design**, and **history Testing Decisions** in
`Notes/PRD-sqloid.md`. Storage and snapshot creation come from Issues #34
(exactly-once finalization, [active-select-lifetime.md](active-select-lifetime.md))
and #33 (immutable typed metadata, [snapshot-metadata.md](snapshot-metadata.md));
this issue owns the store's bounded retention, stable-ID browsing, local
reslicing, query-error recovery, and defensive eviction.

## Bounded immutable snapshots (`internal/history`)

- **One entry per actual execution**: every finalized SELECT execution
  appends exactly one entry through the single `ResultStore.AppendFinalized`
  seam, which rejects a second entry for an already-finalized execution ID
  (Issue #34). Tabular entries carry columns, rows in ascending absolute
  logical-position order, the Issue #33 `SnapshotMetadata`, and the truthful
  `Classify` completeness; non-tabular `KindCancelled` and `KindError`
  entries carry the verbatim terminal reason.
- **Stable non-positional IDs**: `EntryID` values are allocated
  monotonically by `AppendFinalized`, never reused or renumbered, and stay
  with an entry for its whole lifetime even as older entries are evicted and
  slice indices shift. Zero is never allocated.
- **Exact 20-entry retention**: `ResultCapacity` is 20. Each append beyond
  it evicts exactly the oldest retained entry first; surviving IDs and their
  chronological order are never changed.
- **Deep immutability**: columns, rows, and typed values — including exact
  BLOB bytes — are deep-copied on append and on every retrieval
  (`Entries()`, `Lookup()`). Mutation of the source snapshot, the live result
  cache, or any retrieved copy can never alter retained history. Metadata is
  a pure value type with copy semantics.

## Stable-ID selection (`internal/history/result_selection.go`)

`Oldest`, `Newest`, `Lookup`, `OlderThan`, and `NewerThan` are pure
read-only cursor operations mirroring the Issue #35 query-history primitives:
deterministic false results on empty stores, boundaries (no crossing oldest
or newest), evicted IDs, never-allocated IDs, and the zero none-ID; every
return is freshly deep-copied. Navigation never appends, allocates IDs,
evicts, or reorders.

## Browsing (Ctrl+E / Ctrl+Y, `internal/ui/result_history.go`)

- **Entering**: Ctrl+E or Ctrl+Y from the base context exits any query- or
  result-history mode, finalizes the active SELECT exactly once through the
  Issue #34 `enterResultHistory` seam, and selects the newest retained
  entry. With no retained entries the key is a no-op. While any SELECT
  request is in flight the Issue #27 gate rejects Ctrl+E/Y with the exact
  `result history is unavailable while a request is in flight` feedback.
- **Traversal**: Ctrl+E steps older, Ctrl+Y steps newer, by stable ID.
  Boundary presses are no-ops; Ctrl+Y at the newest entry and Esc exit
  result-history mode to the base builder/result context. Tabular snapshots
  render their rows; non-tabular error/cancelled entries render their exact
  reason on the ordinary result-error boundary. All three entry kinds are
  traversable; empty results render `No rows`.
- **Local terminal-height reslicing**: `projectHistoryEntry` is a pure
  projection reslicing the selected immutable snapshot to the current
  layout's complete-row capacity (`CalculateLayout(...).PageRows`), with the
  absolute displayed offset derived from the entry's retained-range metadata
  (`RetainedStart`). Resizing while browsing reprojects locally from the
  same stored snapshot — the stored entry is never rewritten and
  `internal/resultcache` is never consulted as live backing state. Issue #75
  adds that the projection restores the immutable snapshot's warning
  metadata onto the projected view: `Metadata.InvalidUTF` is restored onto
  the new `result.Page` (the grid's invalid-UTF truth source) and
  `Metadata.TruncatedByByteCap` onto `ResultView.ByteTruncated` (the
  persistent byte-cap disclosure), so the shared `result.UTFWarning` and
  `result.ByteCapWarning` render truthfully at every terminal page size.
  Issue #76 adds that the projection restores the typed limit-failure kind
  and one-based position from `Metadata.LimitFailureKind`/
  `Metadata.LimitFailurePosition` onto a fresh `ResultView.LimitFailure`
  copy, so `results_grid.go` renders the exact shared
  `result.LimitFailure.Error` line for page versus value failures at every
  terminal page size. The restored value is a fresh copy so later mutation
  of the projected view cannot alter the stored metadata. A snapshot with
  no limit failure keeps `LimitFailure` nil on the projected view — no
  failure is synthesized. Offset, rows, columns, and BLOB copies are
  preserved; the stored entry is never touched.
- **Zero refetch**: entering, stepping, resizing, and rendering while
  browsing issue zero database, page, or count requests; no Bubble Tea
  command touches the executors. The only fresh-data path remains an actual
  rerun.

## Execution exits history first

`ExecutionStartedMsg` handling calls `exitResultHistoryMode()` (clearing the
historical selection, cursor, and displayed rows) before the execution and
its Issue #34 finalization proceed, so historical rows are never the active
view of a new execution and no stale selected rows survive execution start.
The selected entry stays in storage only when it is not the one evicted by
the new finalization's append.

## Query-error recovery

- **Error replacement**: an ordinary SELECT failure (first-page failure
  before rows, or a later-page failure after retained rows) records the
  lifecycle-defined ending and finalizes exactly one entry — a non-tabular
  `KindError` entry before rows, or a tabular failed snapshot preserving
  captured rows after them — that becomes the newest result. The visible
  result area is replaced by the error boundary; no stale selected rows
  survive.
- **`database is locked` is ordinary**: a request that exceeds the
  five-second busy timeout and fails with `database is locked` is an
  ordinary query error — one finalized error entry and the ordinary
  result-error boundary — never a terminal state.
- **Esc dismissal**: Esc in the base context dismisses only the displayed
  error to the base builder/result context. No retained history is deleted;
  older successful and error entries remain reachable through Ctrl+E/Y.
- **Terminal override**: where an authoritative health classification is
  present (path deletion/replacement or another terminal state, per
  [session-health.md](session-health.md) and
  [stale-schema-refresh.md](stale-schema-refresh.md)), that terminal state
  overrides the lock/query error: the terminal message replaces the result
  area and no query-error rendering survives.

## Defensive selected-ID eviction

Normal actual execution exits history before its append, so selection is
never evicted by Sqloid's own work. Defensively, after every possible store
mutation (including externally driven appends or store replacement while an
entry is selected), `validateResultHistorySelection` resolves the selected
stable ID before any visible rows or metadata are derived:

- **Entries remain**: selection moves to the new oldest retained entry, only
  that entry's immutable rows are resliced at the current terminal height,
  and the results region shows exactly
  `Previously viewed result was evicted from history`. This covers oldest,
  middle, and newest selected IDs under full and partially filled histories,
  for tabular, error, and cancelled entries.
- **History empty**: result-history mode clears and the base builder/result
  fallback returns with no historical rows.
- **No evicted data ever renders**: no frame, intermediate model state,
  resize, navigation step, or dismissal can render rows, columns, metadata,
  or errors from an evicted backing entry. Surviving stable IDs and
  snapshots are unchanged, and no database request is ever issued — there is
  no refetch, only reconciliation over retained immutable entries.

## Tests

- `internal/history/result_history_store_test.go` — bounded retention matrix
  (under/at/over capacity), stable IDs, oldest-first eviction with surviving
  IDs preserved, deep immutability including exact BLOB bytes and ascending
  positions, non-tabular retention, and the selection-primitive matrix.
- `internal/history/result_history_eviction_test.go` — defensive eviction
  matrix over full and partially filled histories with tabular and error
  entries: exactly the excess oldest entries unresolvable, every surviving
  ID intact, deterministic new-oldest target.
- `internal/ui/result_history_browse_test.go` — pure reslicing table
  (capacity vs snapshot size, absolute offsets, non-tabular projections,
  immutability through projection) and the zero-refetch browsing walk
  (selection, repeated navigation with boundaries and newest-boundary exit,
  resize reslicing, rendering; zero executors invoked throughout).
- `internal/ui/result_history_ui_test.go` — the key seam (Ctrl+E/Y entry and
  traversal, boundary no-ops, Esc exit), execution start exiting history
  before finalization, error replacement with Esc dismissal and older-entry
  reachability, later-page failure finalizing a tabular failed snapshot,
  `database is locked` as an ordinary error, and terminal-health override.
- `internal/ui/result_history_eviction_test.go` — scripted defensive
  eviction: new-oldest fallback with the exact notice, kind-aware fallback
  projections, empty-history base return, and no evicted data in any frame
  after resize, navigation, or dismissal, with zero requests.
- `internal/ui/snapshot_warning_roundtrip_test.go` (Issue #75) — invalid-UTF
  and byte-cap warning metadata restored through local historical projection
  at multiple terminal page sizes, rendered in browsing, and excluded from
  export payloads; immutability after mutating live pages, projected views,
  source BLOBs, and cache state.
- `internal/ui/limit_failure_history_test.go` (Issue #76) — typed page and
  value limit failures restored onto `ResultView.LimitFailure` through local
  historical projection at multiple terminal page sizes, rendering the exact
  shared `result.LimitFailure.Error` line in browsing at multiple terminal
  heights; retained leading rows, absolute positions, and typed cells
  unchanged; immutability after mutating live result views, projected views,
  and cache state; the no-failure control projects without synthesizing a
  `LimitFailure`.

Cross-referenced Issues #20, #31, #33, #34, #36, #49, #72, #74, #75, and #76 and the Execution and Result
Lifecycle, SELECT lifecycle, errors/cancellation
implementation decision, Global Key Precedence and context/action matrix,
UI/History Module Design, and history Testing Decisions in
`Notes/PRD-sqloid.md`.
