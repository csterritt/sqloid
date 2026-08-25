## Issue 7: Responsive TUI shell and minimum-size restoration

**Type**: HITL
**Blocked by**: Issue 1

### Parent PRD

`PRD-sqloid.md`

### What to build

Create the Bubble Tea shell and exact region arithmetic from **Resize/layout** and **Acceptance Criteria**. The builder scrolls to its focused field, results receive every remaining row, and screens below 80×24 preserve hidden state behind the required message while still routing Ctrl+W to any active cancellable request.

### How to verify

- **Manual**: Review the mandatory 80×24, 100×30, and 160×50 matrix, then resize below and above 80×24 while focus and content are nontrivial and cancel active work while the too-small message is visible.
- **Automated**: Model/layout tests assert footer, border ownership, builder cap, results height, focused-field scrolling, exact context restoration across the minimum-size boundary, and generic Ctrl+W dispatch only when hidden state owns active cancellable work.

### Acceptance criteria

- [ ] Given supported height H, then exactly one footer row is reserved, the builder is capped at floor(H/3), and results receive all remaining rows and exceed half-height.
- [ ] Given a growing builder, when focus moves, then the complete focused field remains visible through internal scrolling.
- [ ] Given a screen below 80×24, then `terminal too small` replaces the layout without losing state, which is restored exactly on resize.
- [ ] Given a screen below 80×24 with an active cancellable request, then Ctrl+W remains available and enters the normal cancellation flow without exposing or otherwise mutating hidden UI state.

### User stories addressed

- User story 15: Apply exact layout arithmetic
- User story 16: Keep the complete focused builder field visible
- User story 79: Preserve and restore state below the minimum screen size

---
