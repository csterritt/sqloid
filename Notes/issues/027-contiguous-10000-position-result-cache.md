## Issue 27: Contiguous 10,000-position result cache

**Type**: AFK
**Blocked by**: Issue 21

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the active cache as one contiguous range of absolute logical positions capped at 10,000 positions. Apply deterministic adjacent append/prepend, overlap replacement, opposite-end eviction, and nonadjacent stale rejection.

### How to verify

- **Manual**: Traverse forward and backward beyond 10,000 rows, alternate direction, and inspect retained ranges.
- **Automated**: Pure cache tests cover duplicates, overlap, both eviction directions, alternating traversal, cap invariants, and gap-producing stale pages.

### Acceptance criteria

- [ ] Given adjacent forward/backward pages, then the cache remains contiguous and evicts only the standard opposite end when required.
- [ ] Given overlap, then matching logical positions are replaced without duplication.
- [ ] Given a stale nonadjacent response, then it is rejected rather than creating a gap; duplicate-valued rows remain distinct positions.

### User stories addressed

- User story 54: Browse through one bounded contiguous logical-position cache

---
