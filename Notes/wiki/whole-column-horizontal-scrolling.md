# Issue #29 — Whole-column horizontal scrolling

Issue #29 makes the result grid scroll horizontally in whole-column units only: the grid's horizontal position is exactly one first-visible output-column index, moved one whole output column per accepted press by Shift+Page Down or `.`, and back by Shift+Page Up or `,`. Widths are recomputed from the new first column at every layout pass, a single oversized column is capped and ellipsized within the available cell area, and there is no character or byte offset anywhere — no intra-cell horizontal scrolling exists by construction. See `Notes/PRD-sqloid.md` (Builder and Display Interaction, Global Key Precedence and Context/Action Matrix, SELECT lifecycle, Keybinding portability, Module Design, Testing Decisions), and cross-references to [first-select-result-grid.md](first-select-result-grid.md), [serialized-vertical-paging.md](serialized-vertical-paging.md), [in-flight-gating.md](in-flight-gating.md), [select-request-identities.md](select-request-identities.md), and [responsive-tui-shell.md](responsive-tui-shell.md).

## The horizontal position: one index, nothing else

`Model.firstColumn` (unexported, `internal/ui/model.go`) is the **first-visible output-column index** into the frozen deduplicated header (`result.Page.HeaderNames()`). It is the grid's entire horizontal state:

- Every layout pass recomputes the visible columns and their widths afresh from the current output columns, the rendered cell display widths, the available grid width, and the index — nothing is cached across passes.
- There is no character or byte offset in the horizontal state, so no render path can scroll inside a cell. The layout type `gridVisibleLayout` carries only `First` (the clamped index), `Widths` (one display width per visible column), and `Total`.
- Each new SELECT execution resets the index to zero alongside the Issue #25 paging reset (`resetPagingState`).

## The pure layout seam (`internal/ui/horizontal_layout.go`)

- **`visibleGridLayout(names, cells, availWidth, first)`** derives `naturalGridWidths` (widest Unicode display width per column, header included, capped at `gridColumnCap` = 32), then packs whole columns starting at the clamped `first`: the first visible column is always included with its width capped to the available cell area (an oversized column's header and cells ellipsize within it via `fitGridCell`, never splitting visible glyphs), and every later column joins only when it fits **completely**, grid separator (`" | "`, width 3) included. "No room for another complete column" therefore excludes the column instead of clipping it. Reapplying a returned layout is idempotent — the layout is a pure function of the index and current widths.
- **`horizontalStep(first, total, delta)`** moves the index by delta whole columns and reports acceptance. Presses at the first (retreat) or last (advance) boundary, and any navigation with zero or one output columns, are no-ops that return the unchanged index.
- **`clampFirstColumn(first, total)`** normalizes the index after column or width changes: a valid index is preserved unchanged, a negative index clamps to the first column, an index beyond the last clamps to the last, and empty results collapse to zero.

## Key bindings and precedence (`internal/ui/horizontal_keys.go`)

- **Portable bindings**: `.` advances exactly one column; `,` retreats exactly one — handled in the base result context next to the Issue #25 page keys, regardless of how many columns fit.
- **Shift+Page bindings**: real terminals send the xterm sequences `ESC[6;2~` (Shift+Page Down) and `ESC[5;2~` (Shift+Page Up), which the Bubble Tea input reader reports as unknown CSI messages rather than KeyMsgs. `shiftPageDirection` bridges those String representations (and the synthetic `ShiftPageMsg{Down bool}` the model handles directly) onto the same one-column bindings.
- **Boundary no-ops**: an advance at the last column or a retreat at the first column changes no state and dispatches no command.
- **Purely local movement**: an accepted move changes only `firstColumn` and issues no database command — request counts never change and no request slot is claimed.
- **Local while requests are pending**: the Issue #27 in-flight gate falls horizontal keys through to base handling, so one-column movement remains available while first-page, later-page, or count work is in flight, with any stale gate notice cleared. (While the first page itself is pending there are no output columns yet, so presses are consumed no-ops by the boundary arithmetic.)
- **Higher-precedence contexts consume the keys first**, exactly per the matrix: the too-small suspension gate, terminal states, the quit confirmation, the top popup overlay, and focused value input (a `.` typed into a focused prompt is prompt input, not scrolling) all handle the keys before base result scrolling can see them.

## Resize: preserve, then clamp

Every visible resize (and restore from suspension) bumps the viewport generation per Issue #26 and then calls `clampFirstColumnModel`: a still-valid index is preserved exactly, and an invalid one (columns disappeared or the result changed) clamps to the nearest valid output-column boundary, including single-column and empty results. Vertical request orchestration on resize stays with Issue #26/#32.

## Grid rendering integration (`internal/ui/results_grid.go`, `view.go`)

`renderResultPage` now renders only the whole columns of the pure layout starting at `firstColumn`, sized to the results region's interior width (`width - 2` border rows). The frozen deduplicated header, absolute range status, typed cells, and complete-row vertical allocation are unchanged; the capped oversized column ellipsizes through the existing `fitGridCell` path. The vertical paging seam, count presentation, and cancellation behavior are untouched.

## Testing

- `internal/ui/horizontal_layout_test.go` — table-driven pure coverage: wide terminals fitting all columns, narrow terminals fitting only the first, layout restarting at the selected index with recomputed widths, exact-fit boundaries, no room for another complete column, Unicode double-width runes (both width computation and packing), oversized-first-column capping with no room left over, grid-cap-sized columns still packing followers, invalid indexes at both ends, empty results, idempotent recomputation, boundary stepping (`horizontalStep`), and resize clamping (`clampFirstColumn`).
- `internal/ui/horizontal_keys_test.go` — scripted `(model, msg) → (model, cmd)` coverage of all four bindings: exact one-column moves repeated across presses, boundary no-ops at both ends, no command and unchanged page-request counts on accepted moves, local movement during held first-page/later-page/count requests through the fake executor seams with cleared gate notices, raw xterm CSI bridge equivalence, precedence consumption in quit confirmation, popup overlay, focused input, too-small suspension, and terminal state, rendering that hides the abandoned first column and reveals the newly visible one on a narrow viewport, and resize preservation (valid index across width changes) with clamping (beyond-last, single-column, and empty results).
