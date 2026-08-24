## Issue 14: Ordered projection editing and deduplication

**Type**: AFK
**Blocked by**: Issue 13

### Parent PRD

`PRD-sqloid.md`

### What to build

Complete SELECT projection state and UI editing. Preserve ordered `(column, aggregate)` entries, allow distinct aggregates on one column, reject exact duplicates, enforce wildcard exclusivity, and remove only the latest entry.

### How to verify

- **Manual**: Add repeated columns with different aggregates, attempt an exact duplicate, select wildcard after entries, and use Backspace/Delete repeatedly.
- **Automated**: Pure transition and model tests assert order, duplicate rules, wildcard clearing, last-entry removal, and empty no-ops.

### Acceptance criteria

- [ ] Given distinct aggregate pairs, when selected, then their insertion order is preserved even on the same column.
- [ ] Given an identical pair, then it is not added twice; given wildcard, then all prior entries are cleared and wildcard is sole.
- [ ] Given Backspace/Delete on projection, then only the most recent entry is removed and empty state is unchanged.

### User stories addressed

- User story 26: Preserve valid ordered projection entries
- User story 27: Remove the latest projection or clear whole fields safely

---
