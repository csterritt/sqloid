## Issue 77: Count every separator in horizontal grid packing

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Make horizontal grid packing account for the cumulative width of every rendered `" | "` separator. After the first visible column, each accepted column must add both its width and `gridSeparatorWidth` to the used-width invariant so `visibleGridLayout` never selects columns whose joined header or row exceeds the available width. Preserve one-column scrolling, oversized-column capping/ellipsis, and boundary no-ops.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: At supported terminal widths, display several narrow columns whose widths sit near the row boundary and scroll horizontally; confirm no selected header/data row wraps or draws off-screen and no fitting column is unnecessarily omitted.
- **Automated**: Layout tests cover multiple narrow columns at exact-fit, one-below, and one-above boundaries and assert `sum(widths) + (n-1)*gridSeparatorWidth <= available`; rendering tests compare joined row width, plus regressions for one column, oversized first columns, and shifted first-visible indices.

### Acceptance criteria

- [ ] Given two or more visible columns, then packing includes one separator width for every adjacent pair in its cumulative used width.
- [ ] Given several narrow columns near the width boundary, then the visible set's fully rendered joined width never exceeds the available row width.
- [ ] Given an additional column including its separator fits exactly, then it remains visible; if it exceeds by any width, it is excluded.
- [ ] Given horizontal scrolling or an oversized column, then existing one-column movement, capping, ellipsis, and boundary behavior remains unchanged.

### User stories addressed

- User story 48: Move and render whole result columns within the viewport
- User story 51: Keep frozen headers and result rows aligned and visible

---
