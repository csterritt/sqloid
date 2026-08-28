# Searchable Popup Interaction Contract (Issue #12)

Issue #12 introduced the reusable popup interaction contract described by the Builder and Display Interaction section of [`../PRD-sqloid.md`](../PRD-sqloid.md): searchable and scroll-only candidate lists shared by every later builder field, with the refreshed Table popup wired end-to-end as the first consumer. The implementation lives in `internal/ui/popup.go` (state), `internal/ui/popup_view.go` (presentation), `internal/ui/table_popup.go` (Table integration), and `internal/ui/model.go` (routing); see [source-code.md](source-code.md).

## Variants

- **Searchable** (`PopupSearchable`): filters candidates against typed case-insensitive subsequence search text; used by Table today and by the later column, GROUP BY, and ORDER BY flows.
- **Scroll-only** (`PopupScrollOnly`): presents every candidate in source order with Up/Down navigation and **no search input modality at all** — printable keys are no-ops; reserved for aggregates and operators.

A `Multi` flag turns any searchable list into a multi-select whose Enter adds-and-reopens instead of accepting-and-closing. Candidate identity (`ID`) is kept separate from displayed text so acceptance always commits identity, never presentation.

## Matching and ordering

- `matchesSubsequence(query, display)` requires every lower-cased rune of the query to appear somewhere after earlier query runes in the lower-cased display: case-insensitive subsequence matching only.
- Empty search shows all candidates in source order; a matching search keeps matches in source order.
- Filtering applies **only to searchable variants**; scroll-only lists never filter.
- With empty candidate data or an exhausted filter the popup stays open showing exactly `no matches` (`NoMatchesMessage`); it neither closes nor invents entries.

## Navigation, viewport, and reset

- Up/Down move the highlight one row within bounds; both boundaries clamp as deterministic no-ops (no wrap).
- `viewportHeight` caps visible rows (`SetViewportHeight`; ≤0 shows everything unwindowed). `viewportTop` shifts minimally so the highlight stays inside `[top, top+height)`.
- **Every actual search-text change — typed rune, Backspace, full replace — resets the highlight to the first visible result and the viewport to its top** (`SetSearch`). Identical text changes nothing. This is the PRD's "changing search resets its highlighted selection" rule.

## Search input modality

While any popup is open it consumes keys before base-context handling (the global key-precedence matrix's popup row): printable characters including S/U/D/I and `?` append to the search text, space appends via the space key type, Backspace deletes the last search rune, and Tab/Shift+Tab do not move builder focus. Nothing leaks into builder shortcuts. Scroll-only variants route through the same code but ignore every append, preserving their input-free contract.

## Selection semantics

| Action | Single-select | Multi-select |
|---|---|---|
| Enter on highlighted row | `EnterAccepted`: caller closes the popup and commits the accepted ID | `EnterAdded`: completed selection appended in insertion order, duplicate excluded, popup stays open for another choice |
| Enter on already-completed row | n/a | `EnterDuplicate`: nothing changes |
| Enter with no matches / empty candidates | ignored, popup remains open | same |
| Esc | closes unchanged | closes unchanged; only unfinished work (search edits, highlight moves) is discarded |

Completed multi-selections survive filtering (including temporary no-match states), reopening, and the Esc cancel path via `Completed()` — Esc preserves only *already completed* selections, per the PRD's cancellation rule.

## Opener focus restoration

`Model.installPopup(popup, accept)` captures the exact UI focus index at open into `openerFocus`. Both accept and cancel paths restore that opener through `closePopupRestore(opener)` rather than inferring a default focus. For single-select Enter, the order is fixed: capture opener/ID/hook → close and restore → invoke the feature-specific `accept` hook, which commits identity through owning transitions.

## Table integration (first end-to-end consumer)

`internal/ui/table_popup.go` opens the fresh searchable single-select popup when Enter is pressed while the Table field holds focus:

- Candidates derive from `QueryBuilder.EligibleTables()` — the refreshed Schema catalog filtered by the currently selected command's own eligibility rules (`EligibleTables`, see [builder-command-table.md](builder-command-table.md)); the UI duplicates no builder rules.
- Each candidate's `ID` and displayed text are exactly the cataloged object name, so acceptance commits object identity back through `QueryBuilder.SelectTable` inside the accept hook; `applyBuilder` re-renders the field bar from the resulting snapshot.
- Viewport caps at 8 visible rows so eligible lists longer than the window scroll.
- Enter commits (e.g. typing `ser` narrows UPDATE's candidates to `users`) and restores exact Table-opener focus; Esc discards without touching builder state; an unrefreshed catalog opens as an open no-match state where Enter is inert.

Later column, GROUP BY, ORDER BY, aggregate, and operator flows are future consumers of the same state/rendering/routing seams — none are implemented for them yet.

## Rendering

`RenderPopup` draws a rounded-bordered box: one `Search: <text>_` line for searchable variants (absent for scroll-only), status lines such as the exact `no matches`, then the visible candidate window with `> ` marking the highlighted row and `  ` the others. Lines truncate to whole runes at the interior width. When open, `View` composites this box over the results region just below its top border via `composeOverlay` — following the Issue #8 overlay rule that overlays draw over regions and **never reflow their borders or rows**; total rendered height stays exactly H and focus stays readable with color disabled (`> ` prefix, not color alone). See [responsive-tui-shell.md](responsive-tui-shell.md).

Cross-references: [builder-command-table.md](builder-command-table.md), [responsive-tui-shell.md](responsive-tui-shell.md), [schema-catalog.md](schema-catalog.md), and the Builder and Display Interaction, UI Module Design, and Testing Decisions sections of [`../PRD-sqloid.md`](../PRD-sqloid.md).
