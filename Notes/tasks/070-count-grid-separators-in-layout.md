# Tasks for #70: Count every separator in horizontal grid packing

Parent issue: #70
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify cumulative separator-aware grid packing

**Type**: RED  
**Output**: Failing pure layout tests enforce sum(widths)+(n-1)*gridSeparatorWidth at exact-fit, one-below, and one-above boundaries.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui/horizontal_layout_test.go` with table-driven layouts containing three or more narrow columns, because the existing two-column cases cannot expose a missing cumulative separator. For each first-visible index, calculate the rendered invariant as the sum of returned cell widths plus one `gridSeparatorWidth` per adjacent pair and require it never exceed `availWidth`. Cover exact fit, one display cell below, and one above for three, four, and Unicode-width columns; require a fitting final column to remain and an overflowing one to be excluded. Include empty and one-column controls, invalid/clamped first indices, oversized first columns, and shifted starts. Keep this task test-only and use the shared constant rather than duplicating the literal separator width.

---

### 2. Account for every accepted separator in used width

**Type**: GREEN  
**Output**: visibleGridLayout maintains the cumulative rendered-width invariant for every accepted column.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Fix `visibleGridLayout` in `internal/ui/horizontal_layout.go` so after the first visible column every accepted column contributes both `gridSeparatorWidth` and its own width to `used`. Preserve the existing first-column cap, natural Unicode display-width calculation, minimum available cell area, exact-fit comparison, clamped first index, and no intra-cell offset. Do not subtract a separator globally, count one before the first column, or alter `horizontalStep`. Make only the arithmetic change required for Task 1 and keep comments aligned with the invariant.

---

### 3. Specify rendered header and row width integration

**Type**: RED  
**Output**: Failing rendering tests prove joined headers and data rows fit the available grid width across scrolling and oversized-column cases.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused integration cases in `internal/ui/results_grid_test.go`, using `visibleGridLayout`, `renderGridRow`, and the established display-width assertions rather than byte length. Render headers and rows for multiple narrow columns at exact-fit and overflow boundaries and require every joined line's terminal display width to stay within the supplied grid row width while header/data column counts and padding remain aligned. Repeat after moving the first-visible index one column at a time and include a single oversized first column that is capped and ellipsized. Add a regression proving a column whose width plus its separator fits exactly is not unnecessarily omitted. Keep this task test-only and avoid snapshotting unrelated shell borders.

---

### 4. Preserve whole-column scrolling with corrected packing

**Type**: GREEN  
**Output**: Rendering and horizontal-navigation regressions pass with no wrapping, off-screen rows, or unnecessary omission.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Adjust the smallest necessary result-grid composition in `internal/ui/results_grid.go` or its shared helpers if Task 3 reveals assumptions inconsistent with the corrected layout. Keep `visibleGridLayout` as the sole selector of visible widths, join cells with exactly `" | "`, and preserve header/data alignment, padding, terminal display-width handling, cap/ellipsis behavior, and first-visible-column state. Verify the four horizontal bindings still move exactly one whole column, boundary presses are no-ops, and resize preserves or clamps the index through existing logic. Do not introduce character scrolling or drop a column that exactly fits.

---

### 5. Document separator-aware horizontal layout

**Type**: DOCUMENT  
**Output**: Wiki documentation records the cumulative packing formula, exact boundaries, rendering alignment, and preserved scrolling/oversized behavior.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #70's layout and rendering implementation/tests from `internal/ui/horizontal_layout.go`, `results_grid.go`, and their test files into the appropriate pages under `Notes/wiki`. Record the invariant `sum(column widths) + (visible column count - 1) * gridSeparatorWidth <= available width`, exact-fit inclusion, overflow exclusion, Unicode display-width basis, and no separator before the first column. Document preserved one-column scrolling, boundary no-ops, shifted starts, oversized capping/ellipsis, and header/data alignment. Cross-reference Issue #70 and the horizontal-scroll, visual invariants, UI Module Design, manual matrix, and Testing Decisions in `Notes/PRD-sqloid.md`; update the wiki index and append the required dated log entry.

---

### 6. Create the grid-packing walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/070-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/070-06/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate three-or-more-column exact-fit, one-below, and one-above layouts and calculate the cumulative separator invariant from returned widths. Render matching headers/data to show no wrapping or off-screen output and no unnecessary omission, then scroll through shifted first-column indices and show one-column, Unicode, and oversized cap/ellipsis controls. Include focused automated-test output, reference Issue #70 and `Notes/PRD-sqloid.md`, and store every generated artifact in the approved directory.

---
