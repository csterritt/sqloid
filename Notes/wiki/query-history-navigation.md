# Issue #35: Stable query-history navigation and consecutive suppression

Ctrl+P/N browsing over the Issue #20 query-history store, immutable
copy-on-restore of every builder field, execution-time mode exit, and the
defensive selected-ID eviction fallback, per the **History**, **Execution and
Result Lifecycle**, and **UI/History Module Design** decisions in
`Notes/PRD-sqloid.md`. Ownership stays split exactly as Issue #20 defined it:
`internal/querybuilder` owns normalized snapshots and their equality plus the
new pure `RestoreBuilder` reconstruction seam, `internal/history` owns stable
IDs, chronological storage, the 20-entry cap, consecutive suppression, and
the new pure navigation primitives, and `internal/ui` owns only the mode,
cursor detachment, restoration calls, and notice rendering. Browsing is
strictly read-only; Issue #20 remains the sole owner of actual-execution
append timing.

## Navigation (`internal/history/query_cursor.go`)

Pure read-only primitives over the retained list — no appends, no ID
allocation, no reordering, no eviction:

- `Newest()` / `Oldest()` return the newest/oldest retained entry, freshly
  deep-copied; `false` on an empty store.
- `OlderThan(id)` / `NewerThan(id)` return the entry immediately older/newer
  than the retained entry carrying the stable ID `id`, freshly deep-copied.
  They report `false` when `id` is not retained (evicted, never allocated,
  or the zero none-ID) or when the identified entry is already at the oldest
  / newest boundary, so the cursor can never cross a boundary nor resolve
  through a missing backing entry.
- Navigation moves toward **older** entries (Ctrl+P) and toward **newer**
  entries (Ctrl+N) by stable ID, never by slice index; Browsing, cursor
  movement, and retrieval allocate no stable ID and append nothing.

## Copy-on-restore (`internal/querybuilder/restore.go`)

`RestoreBuilder(state, catalog)` rebuilds a complete `QueryBuilder` from a
stored `HistoryState` through the canonical immutable transitions:

- **Command and table identity**: SELECT/UPDATE/DELETE/INSERT with the
  catalog-eligible stored table.
- **Projection**: ordered entries across wildcard, bare `COUNT(*)`, and
  named columns with aggregates — the fresh SELECT wildcard default is
  cleared first, then each stored entry is re-committed in order.
- **WHERE**: the guided draft flow reproduces column, operator, parsed bound
  type, and the exact entered representation byte-for-byte.
- **GROUP BY order**, **ORDER BY expression key and direction**, and
  **Limit** (empty versus accepted number via `SetLimitInput`).
- **UPDATE SET** assignments and **INSERT** per-column choices in stored
  order, with each Value choice's exact submitted representation.

The input state is never mutated and the store is untouched. When a stored
identity no longer resolves (table, column, or ORDER BY key absent from the
catalog), the seam returns `false` so a caller never installs a partially
restored builder. A successful restore satisfies
`builder.HistoryState().Equal(state)`.

Restored builders are ordinary mutable snapshots: subsequent edits — value
prompts, whole-value clearing, projection removal, any builder transition —
affect only current UI state and can never alter a retained entry, because
the restored builder shares no storage with the store's deep copies.

## UI integration (`internal/ui/query_history.go`, `model.go`)

- **Entering**: `ctrl+p` or `ctrl+n` in the base context opens history
  browsing at the **newest** retained entry and restores it. With an empty
  or nil store the key is a no-op. While a request is in flight the Issue
  #27 gate blocks the keys first (exact `query history is unavailable while
  a request is in flight`), unchanged.
- **Moving**: `ctrl+p` steps toward older entries, `ctrl+n` toward newer.
  Boundary presses are deterministic no-ops; repeated Ctrl+P at the oldest
  entry keeps it selected, and direction reversal walks back the other way.
  Each step restores an immutable deep copy; `historyNotice` clears on any
  successful step.
- **Newest boundary**: `ctrl+n` at the newest entry **exits history mode**
  back to the base builder view, keeping the restored (and possibly edited)
  builder state current.
- **Esc** exits history mode at any time, likewise keeping the current
  builder state.
- **Editing while browsing**: ordinary base-context keys still reach the
  restored builder, so a restored query can be revised before execution.
- Browsing, cursor movement, restoration, and edits append nothing and
  consume no stable ID — store length and all retained IDs stay unchanged.

## Execution exit

An actual execution start (`ExecutionStartedMsg`) exits history mode **first**
— detaching the cursor while preserving the current deep-copied, possibly
edited builder state — and then runs the unchanged Issue #20 append seam.
The executed query is therefore the current restored state, not a fresh
lookup or stale backing entry, and the executed append participates in the
unchanged consecutive-suppression policy: executing the exact state of the
immediately preceding retained entry suppresses (no ID consumed, no
eviction), while an edited state appends a fresh entry. Normal execution
can never leave the selection pointing at an evicted entry, because the
cursor is detached before any append can evict.

## Defensive eviction fallback

After every possible history mutation — including externally driven appends
or wholesale store replacement — `Update` resolves the selected stable ID
against the retained entries before anything renders:

- **Selected ID still retained**: nothing changes; surviving stable IDs are
  never renumbered.
- **Selected ID evicted, entries remain**: the selection moves to the new
  **oldest** retained entry, restores an immutable copy of it, and the
  footer shows exactly `Previously viewed query was evicted from history`.
- **No entries remain**: history mode exits back to the base builder view
  (the last viewed builder data remains valid) with the same exact notice.

A missing backing entry is never rendered, restored, or executed through:
list indices are never used as identity, resolution is always by stable-ID
lookup, and a state that fails catalog restoration detaches the cursor
instead of installing a partial builder.

## Issue #20 ownership (unchanged)

Issue #20 remains the sole owner of actual-execution append timing, full
normalized comparison, stable IDs, the exact 20-entry cap, oldest-first
eviction, A→A consecutive suppression, and A→B→A retention — Issue #35
reuses, never replaces, them. Result-history behavior stays with Issue #34.

## References

- Issues #20 (append store), #34 (active-SELECT lifetime), #35 (this work)
- [query-history-append.md](query-history-append.md) — the append contract
- [active-select-lifetime.md](active-select-lifetime.md) — finalization seam
- [in-flight-gating.md](in-flight-gating.md) — the blocked Ctrl+P/N gate
- PRD sections: Execution and Result Lifecycle, SELECT lifecycle, the
  context/action matrix, History/UI Module Design, and history Testing
  Decisions in `Notes/PRD-sqloid.md`
