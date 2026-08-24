## Issue 29: Snapshot completeness, outcomes, and endpoints

**Type**: AFK
**Blocked by**: Issue 20, Issue 27, Issue 28

### Parent PRD

`PRD-sqloid.md`

### What to build

Model immutable snapshot metadata from **Cache and snapshot invariant** independently of rows: retained range, endpoints, known total, row/byte eviction, completeness labels, terminal outcome, failures, cancellation, and UTF status.

### How to verify

- **Manual**: Finalize complete, partial, truncated, count-failed, cancelled, and failed SELECTs after forward/backward traversal.
- **Automated**: History/cache matrix tests assert truthful label combinations, ascending positions, limited-result semantics, short/empty endpoint observation, and no count inconsistency clamping.

### Acceptance criteria

- [ ] Given both logical endpoints and all limited-result rows retained, then completeness is exclusively `complete`.
- [ ] Given eviction, unseen work, cancellation, or failure, then truthful `partial`/`truncated` labels coexist independently with terminal outcome.
- [ ] Given count is unavailable, then only an observed short/empty page establishes the high endpoint; otherwise unseen remainder stays unknown.

### User stories addressed

- User story 55: Store and export truthful snapshot metadata in ascending order
- User story 56: Infer high endpoints only from observed final pages when count is unavailable

---
