## Issue 32: Immutable result history and query-error recovery

**Type**: AFK
**Blocked by**: Issue 29, Issue 30

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the 20-entry stable-ID result history and Ctrl+E/Y browsing without refetch. Include lifecycle-defined query errors, exact dismissal, terminal-height reslicing, and defensive selected-entry eviction without rendering missing backing rows.

### How to verify

- **Manual**: Finalize successes and errors, dismiss an error, browse history after database changes, resize, and force eviction.
- **Automated**: History/model tests assert immutability, zero DB requests while browsing, one entry per execution, reslicing, older-result reachability, eviction notices, and empty fallback.

### Acceptance criteria

- [ ] Given Ctrl+E/Y, then immutable snapshots are selected without database work; rerun remains the only fresh-data path.
- [ ] Given a query error, then it replaces the result view, Esc dismisses it, and older results remain reachable.
- [ ] Given selected-entry eviction, then no missing rows render and selection moves with the exact result notice or returns to base.
- [ ] Given a request that exceeds the five-second busy timeout, then `database is locked` is displayed as an ordinary query error (not a terminal state) unless health classification overrides it.

### User stories addressed

- User story 64: Browse immutable result snapshots without refetching
- User story 65: Display and dismiss query errors while retaining older results
- User story 88: Keep history selection safe across stable-ID eviction

---
