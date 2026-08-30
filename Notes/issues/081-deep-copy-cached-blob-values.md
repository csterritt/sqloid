## Issue 81: Deep-copy cached BLOB values

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Enforce the result cache's ownership boundary for BLOB payloads. Clone `result.Value.Bytes` when rows enter the cache and clone them again when `Rows()` returns retained rows, so neither caller input nor a mutable snapshot can alter cache-owned bytes. Preserve row positions, value kinds, payload accounting, ordering, and all non-BLOB copy behavior.

### How to verify

- **Manual**: Insert a row containing a BLOB into a cache, mutate both the original byte slice and a BLOB returned by `Rows()`, and confirm a fresh `Rows()` call still returns the original bytes.
- **Automated**: Add cache tests for ingest-side and retrieval-side BLOB mutation, including multiple calls to `Rows()` and mixed typed values; assert exact bytes, unchanged payload accounting and positions, and no aliasing while existing cap/eviction tests remain green.

### Acceptance criteria

- [ ] Given a caller mutates a BLOB slice after its row is accepted, then the retained cache bytes remain unchanged.
- [ ] Given a caller mutates a BLOB slice returned by `Rows()`, then neither the cache nor a later `Rows()` result changes.
- [ ] Given rows contain NULL, INTEGER, REAL, TEXT, and BLOB values, then copies preserve kinds, values, positions, ascending order, and payload accounting.

### User stories addressed

- User story 54: Retain bounded result-cache payloads without corrupting logical rows
- User story 55: Preserve exact snapshot rows and truthful metadata
- User story 74: Preserve BLOB bytes for format-specific display and export

---
