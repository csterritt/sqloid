# Contextual Help and Overlay Precedence (Issue #54)

Issue #54 centralizes Sqloid's non-quit key routing into one ordered
precedence dispatcher and adds a contextual `?` help overlay, per the Global
Key Precedence and Context/Action Matrix in `Notes/PRD-sqloid.md`.

## The ordered non-quit precedence dispatcher

`internal/ui` routes every non-quit key event through one ordered dispatcher
(`Model.handleKey` in `model.go`). Layers apply in exactly this order, and
each decision consumes the key so no lower layer can run:

1. **Terminal state** — the deletion/replacement/outcome-unknown terminals
   (Issues #45–#46) consume every key and own their reduced in-memory key
   sets. No key may open popups, move builder state, or start database work.
2. **Contextual help overlay** (new in #54) — while the help overlay is open
   it consumes every key above every other context; Esc restores its exact
   opener snapshot atomically and `q`/Ctrl+C open the shared quit
   confirmation like every other non-quit modal.
3. **Top overlays** — export warnings (#49), overwrite confirmation (#53),
   inline save failure (#53), destination picker (#52), quit confirmation
   (#27), destructive preparation modal (#40), popup, universal value entry.
   Each consumes every key until resolved; nothing leaks into a lower layer.
4. **History browsing contexts** — Ctrl+P/N and Esc in query-history mode,
   Ctrl+E/Y and Esc in result-history mode; other keys fall through.
5. **Request-in-flight gate** (Issue #27, fed by write phases per Issue #44)
   — Enter/history/save/export rejected with exact feedback while any typed
   request phase is pending; permitted local horizontal movement and
   serialized page keys pass through; Ctrl+W routes to scoped cancellation
   only in cancellable phases (`writeNoncancellable` gets the exact
   commit-boundary feedback).
6. **Base builder/result context** — validation retry/Enter, paging,
   horizontal columns, history, save/export, error dismissal, and `?`.

Routing classifies terminal state, the current top overlay, focused input,
and pending phases from typed state (`terminalState`, `helpOpen`,
`prepOpen`, `Popup`, `ValuePrompt`, `firstPagePending`/`pagePending`/
`countPendingFlag`, `writePending`/`writeNoncancellable`) — never from
rendered labels. Each decision consumes the key so lower layers cannot
mutate focus, selection, viewport, history, save/export state, or issue
commands. `?`-escaping changes and quit suspension are excluded: universal
`q`/Ctrl+C confirmation and quit's one-overlay suspension belong to Issues
#27 and #55.

A table-driven scripted matrix (`precedence_matrix_test.go`) drives the
non-quit keys through every row — terminal, top overlays and modals, focused
value prompts, searchable/scroll-only popups, request-pending base state in
cancellable and noncancellable phases, ordinary base builder, and the
too-small screen — asserting single consumption, command-count no-leakage,
and unchanged history/save/export invariants.

## Literal `?` versus contextual help

- In every focused text/search component — the universal value prompt (with
  cursor-aware mid-buffer insertion), searchable popup search, and the
  picker's filename input — `?` inserts one literal character at the cursor
  and never opens help.
- In scroll-only popups, modals (preparation, quit confirmation), the
  file-picker's directory focus, and the too-small screen, `?` is consumed as
  a no-op.
- Only from eligible base contexts (ordinary builder fields, WHERE
  value/operator context, a settled result or result-history selection) does
  `?` open one contextual help overlay. In terminal states `?` still routes
  through the terminal branch's reduced help.
- Help is nonstacking by construction: it opens only from base contexts and
  consumes every key while open, so repeated `?` never stacks a second
  overlay. `q`/Ctrl+C inside help open the quit confirmation above it.
- On open, an exact immutable opener snapshot is captured (focus, scroll,
  first visible column, page offset, builder values, selected history by
  stable ID). Esc restores that descriptor exactly — nothing is rebuilt from
  rendered text, and the dismissal key is never applied beneath the closing
  overlay.

## Required help content

- **WHERE help** (from a WHERE value/operator context): a typed token spelled
  `NULL` binds as literal TEXT, never SQL NULL; direct SQL-null intent routes
  to the `IS NULL` / `IS NOT NULL` operators; ordinary comparisons and LIKE
  do not match rows where the column actually holds NULL; `%` and `_` keep
  their SQLite wildcard meaning inside LIKE values.
- **Result-count help** (opened from a result with independent count state):
  the count covers the complete executed SELECT including the user's Limit —
  it is not a table count and not a pre-Limit row count; it runs as an
  independent autocommit read that may drift from displayed or cached rows;
  it never clamps fetched pages or the retained result cache.
- **Reduced terminal help** (deletion, replacement, outcome-unknown): derived
  from the actions actually available for the immutable selection and
  histories — Ctrl+P/N query-history selection, Ctrl+E/Y result-history
  selection, Ctrl+S query saving from immutable memory, Ctrl+X's
  tabular-selection rule with non-tabular rejection, Esc dismissal, and
  immediate status-1 quit. Only in-memory actions are listed; no validation,
  execution, estimate, paging, rerun, cancellation, recovery, or any database
  suggestion appears.

## Overlay cancellation and exact restoration

Every ordinary overlay is nonstacking: modals are created only over the base
context (the dispatcher's ordered ladder cannot stack them), and help opens
only from base. Esc cancels exactly the current top overlay — never a second
underlying layer — and restores the exact opener or flow-specific intact
parent path: popups restore the captured opener focus and preserve completed
multi-selections while discarding only the incomplete current choice
(exercised end-to-end on the GROUP BY multi-select); stale-validation retry
cancel closes the flow and reopens continuation; the preparation modal
dismisses without appending history or starting the write; overwrite cancel
returns to the intact picker with the frozen captured copy; save-failure
cancel restores the exact opener; and terminal help dismissal preserves the
exact selected history. Esc during a pending schema validation now cancels
that workflow through its established cleanup (attempt identity advance, no
replacement request, exact pre-validation builder context) — previously only
the stale phase offered Esc cancel. Request settlement behind an overlay
remains contractually allowed and identity-guarded. A repeated Esc after
closure is handled only by the newly exposed context on the later key event,
never by leakage from the closing event.

## Tests

`precedence_matrix_test.go`, `contextual_help_test.go`, and
`esc_restoration_test.go` in `internal/ui` hold the table-driven scripted
matrices: key routing across terminal, top-overlay, focused-input,
request-pending (cancellable and noncancellable), base, and too-small
contexts; literal `?` insertion in builder text, popup search, and picker
filename entry; nonstacking help with exact snapshot restoration; the
required WHERE/result/terminal help content; and the Esc restoration table
over every overlay with repeated-Esc stability.

## References

- `Notes/PRD-sqloid.md` — Global Key Precedence and Context/Action Matrix,
  Builder and Display Interaction, Paging consistency, UI Module Design,
  Testing Decisions.
- [in-flight-gating.md](in-flight-gating.md) — Issues #27/#44 request gate.
- [searchable-popups.md](searchable-popups.md) — Issue #12 popup contract.
- [file-picker.md](file-picker.md), [atomic-saves.md](atomic-saves.md),
  [immutable-export-capture.md](immutable-export-capture.md),
  [sql-save-targeting-serialization.md](sql-save-targeting-serialization.md) —
  picker, save, export, and overwrite flows restored by Esc.
- [schema-validation-workflow.md](schema-validation-workflow.md),
  [destructive-preparation.md](destructive-preparation.md) — validation and
  estimate workflows behind the same dispatcher.
- [outcome-unknown-terminal.md](outcome-unknown-terminal.md),
  [health-terminal.md](health-terminal.md) — reduced terminal help contexts.
- [where-guided-predicates.md](where-guided-predicates.md) — the SQL-NULL
  guidance origin in guided predicates (Issues #17/#19).
- [concurrent-page-count.md](concurrent-page-count.md) — the independent
  count semantics the result help explains.
- [outcome-unknown-terminal.md](outcome-unknown-terminal.md),
  [health-terminal.md](health-terminal.md) — Issues #45–#46 terminal
  contracts the reduced help reuses.
- [responsive-tui-shell.md](responsive-tui-shell.md) — too-small suspension.
