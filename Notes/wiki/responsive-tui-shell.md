# Responsive TUI Shell and Minimum-Size Restoration (Issue #8)

How the `internal/ui` Bubble Tea shell divides the terminal into regions, scrolls the builder, and suspends its state below the 80×24 minimum. Cross-references: [source-code.md](source-code.md), [unit-tests.md](unit-tests.md), [cancellation-infrastructure.md](cancellation-infrastructure.md).

## Dependencies and boundary

The shell pins exact `github.com/charmbracelet/bubbletea v1.3.6` and `lipgloss v1.1.0`. No Bubbles component applies to the shell itself: the builder's internal scrolling uses exact custom arithmetic the PRD requires, and text-entry components arrive with later builder tasks (`internal/ui` contains no database behavior — Connection is a future composition seam, and `cmd/sqloid` remains a thin process boundary).

## Region arithmetic (Resize/layout)

At any supported height H ≥ 24 (`layout.go`, `CalculateLayout`):

- **Footer**: exactly one bottom global footer row (`FooterHeight = 1`) is reserved for global status/help.
- **Builder**: desired height includes its own border (2 rows) and padding (2 rows) around the summed display lines of every field (`DesiredBuilderHeight`). The assigned height is that desired value capped at `floor(H/3)`.
- **Results**: every remaining row — `H − 1 − builderHeight` — goes to an independently bordered results region. It always exceeds half of H at supported sizes.
- **Page area**: the complete-row paging area subtracts only rows owned by results from its height: top/bottom border (2), status/count line (1), frozen header (1). No border row is shared or overlapping; each region exclusively owns its borders.
- **Overlays** draw over regions without reflowing them.

Regions partition the screen exactly: `footer + builder + results == H`.

## Focused-field internal scrolling

As fields grow beyond the cap, `adjustScroll` keeps the offset so the complete focused field — including its full multiline extent — remains visible inside the builder's interior viewport (`builderHeight − border − padding`); scrolling clamps to `[0, total−viewport]`. Tab/Shift+Tab/Up/Down move focus among labeled fields, then adjust scroll. Rendering shows each visible field line with a `>` marker on the focused field's first label line.

## Below-minimum suspension (<80×24)

When either dimension drops below the minimum (80 wide × 24 tall):

- `View()` returns exactly the string `terminal too small` — nothing else.
- Before entering, the entire current model is shallow-copied into `suspendedModel` without any mutation of hidden state (context, fields, focus, scroll, cancellable-request ownership).
- Ordinary keys are ignored in both directions: they can neither leak through nor mutate the hidden application state.
- Ctrl+W routes **only when hidden state owns active cancellable work** (`ActiveCancellable` true): it returns the model's generic cancellation command (the future Connection cancellation flow per [cancellation-infrastructure.md](cancellation-infrastructure.md)) as a `tea.Cmd`; otherwise it is ignored.
- Resizing back to supported dimensions restores the exact prior context and focus from the retained copy, then applies normal layout calculation — no reconstruction or reset of application state.

The global quit seam (`q`/Ctrl+C confirmation) is preserved untouched for later full key-precedence work.

## Tested contracts

Pure table-driven arithmetic at 80×24, 100×30, and 160×50 with minimal and growing builders, plus scripted `(model, msg) → (model, cmd)` behavior: region ownership rendered row counts, exact undersized view, ignored-key preservation, exact restoration, and conditional Ctrl+W routing. See [unit-tests.md](unit-tests.md). Pixel rendering beyond these assertions remains manual matrix review (PRD Testing Decisions).
