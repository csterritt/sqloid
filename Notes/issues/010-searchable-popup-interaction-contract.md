## Issue 10: Searchable popup interaction contract

**Type**: AFK
**Blocked by**: Issue 9

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement reusable searchable and scroll-only popup behavior from **Builder and Display Interaction**. Use the refreshed Table popup as the first end-to-end consumer while supporting later column, grouping, aggregate, and operator flows.

### How to verify

- **Manual**: Open Table with empty, matching, and nonmatching searches; change search while scrolled; accept and cancel selections.
- **Automated**: Model tests assert case-insensitive subsequence matching, reset behavior, `no matches`, scrolling, single/multi-select Enter semantics, and Esc preservation.

### Acceptance criteria

- [ ] Given empty search, then all candidates appear; given no match, then the popup remains open with `no matches`.
- [ ] Given changed search text, then the highlighted selection resets deterministically.
- [ ] Given Enter or Esc, then selection and completed multi-selection behavior follows the popup contract without losing opener focus.

### User stories addressed

- User story 21: Search and navigate popups predictably

---
