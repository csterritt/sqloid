## Issue 31: Stable query history and consecutive suppression

**Type**: AFK
**Blocked by**: Issue 30

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the 20-entry stable-ID query history from **History**. Append only at actual execution, suppress only normalized consecutive duplicates, restore complete builder copies without mutation, and handle selected-entry eviction predictably.

### How to verify

- **Manual**: Execute A→A→B→A, browse with Ctrl+P/N, edit restored state, and force oldest-entry eviction.
- **Automated**: History/model tests assert full comparison fields, empty-vs-number distinctions, append timing, stable IDs, copy-on-restore, notices, and empty fallback.

### Acceptance criteria

- [ ] Given consecutive identical actual executions, then only the latter append is suppressed; A→B→A retains both A entries.
- [ ] Given Ctrl+P/N, then every builder field is restored from an immutable copy without adding history.
- [ ] Given selected-entry eviction, then selection moves to the new oldest with the exact query notice, or returns to base when empty.

### User stories addressed

- User story 62: Navigate and restore the 20 most recent query states
- User story 63: Suppress only consecutive identical actual executions
- User story 88: Keep history selection safe across stable-ID eviction

---
