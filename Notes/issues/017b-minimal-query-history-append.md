## Issue 17b: Minimal query-history append (stable ID + consecutive suppression)

**Type**: AFK
**Blocked by**: Issue 17

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the minimal query-history append contract from **History** so that every actual execution from Issue 19 onward appends history correctly. Scope is limited to: stable entry IDs, append-only-on-actual-execution, consecutive-identical suppression (normalized comparison of command, table, ordered projection, WHERE column/operator/value/bound type, GROUP BY order, ORDER BY/direction, Limit empty-vs-number, and UPDATE/INSERT choices and values), and oldest-first eviction at 20 entries.

This does not cover Ctrl+P/N navigation, builder state restoration, or selected-entry eviction fallback — those remain in Issue 31. The append contract must exist before the first execution (Issue 19) so the invariant is tested from the start.

### How to verify

- **Manual**: Execute A→A→B→A sequences and inspect the in-memory history list.
- **Automated**: History unit tests assert append timing (only on actual execution, not validation/estimation), consecutive-identical suppression, A→B→A retention of both A entries, stable IDs, and oldest-first eviction at 20.

### Acceptance criteria

- [ ] Given an actual execution starts, then a query-history entry with a stable ID is appended at that point only — not during validation or estimation.
- [ ] Given consecutive identical actual executions (normalized comparison), then only the latter append is suppressed; A→B→A retains both A entries.
- [ ] Given more than 20 entries, then the oldest is evicted.

### User stories addressed

- User story 63: Suppress only consecutive identical actual executions (append contract)

---
