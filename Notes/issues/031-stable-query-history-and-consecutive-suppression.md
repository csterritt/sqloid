## Issue 31: Stable query history and consecutive suppression

**Type**: AFK
**Blocked by**: Issue 17b, Issue 30

### Parent PRD

`PRD-sqloid.md`

### What to build

Extend the 20-entry stable-ID query-history store implemented by Issue 17b with Ctrl+P/N navigation, immutable copy-on-restore, cursor/selection behavior, and predictable selected-entry eviction. Do not reimplement append timing, normalized consecutive suppression, stable IDs, or the 20-entry cap; preserve those Issue 17b contracts unchanged.

### How to verify

- **Manual**: Execute A→A→B→A, browse with Ctrl+P/N, edit restored state, and force oldest-entry eviction.
- **Automated**: History/model tests assert navigation and cursor behavior, copy-on-restore, execution exiting query history before append, notices, and empty fallback, plus regression coverage proving Issue 17b's full comparison fields, empty-vs-number distinctions, append timing, stable IDs, suppression, and cap remain unchanged.

### Acceptance criteria

- [ ] Given Issue 31 navigation/restoration behavior is added, then Issue 17b remains the sole append path and its consecutive suppression contract is unchanged: A→A suppresses the latter append while A→B→A retains both A entries.
- [ ] Given Ctrl+P/N, then every builder field is restored from an immutable copy without adding history.
- [ ] Given an actual execution starts while query history is selected, then query-history mode exits before append/execution proceeds, the restored/current builder state executes, and any resulting eviction cannot leave the UI pointing at the evicted entry.
- [ ] Given selected-entry eviction, then selection moves to the new oldest with the exact query notice, or returns to base when empty.

### User stories addressed

- User story 62: Navigate and restore the 20 most recent query states
- User story 63: Suppress only consecutive identical actual executions
- User story 88: Keep history selection safe across stable-ID eviction

---
