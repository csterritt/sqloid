## Issue 26: Whole-column horizontal scrolling

**Type**: AFK
**Blocked by**: Issue 7, Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Represent horizontal position as a first-visible output-column index. Bind both Shift+Page and comma/period fallbacks to one-column movement with width recomputation, capped oversized columns, and resize preservation/clamping.

### How to verify

- **Manual**: Browse narrow, wide, and oversized columns with all four bindings and resize at both boundaries.
- **Automated**: Rendering/model tests assert one-index movement, boundary no-ops, recomputed widths, ellipsis without intra-cell scrolling, and resize clamp.

### Acceptance criteria

- [ ] Given room to move, then each horizontal binding advances or retreats exactly one whole result column.
- [ ] Given a boundary, then the binding is a no-op; given resize, then the first-column index is preserved and clamped.
- [ ] Given one oversized visible column, then it is capped and ellipsized without intra-cell horizontal scrolling.

### User stories addressed

- User story 48: Page vertically and move horizontally by exactly one column

---
