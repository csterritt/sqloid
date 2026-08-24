## Issue 20: Concurrent first page and independent result count

**Type**: HITL
**Blocked by**: Issue 5, Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Launch first-page and complete-limited-result count requests concurrently on distinct leases in both journal modes. Preserve their independent autocommit semantics, count wording, failure isolation, and release-blocking capability evidence.

### How to verify

- **Manual**: Review behavior in WAL and rollback-journal fixtures with a delayed count and an external writer.
- **Automated**: Mandatory Linux/macOS barrier-based integration tests prove overlap, distinct leases, unchanged journal mode, drift handling, count SQL/Limit semantics, and independent failures.

### Acceptance criteria

- [ ] Given a SELECT starts, then first page and count launch concurrently on two distinct dedicated connections without hidden serialization.
- [ ] Given count succeeds, then the exact limited-result header wording is shown without clamping independently read pages.
- [ ] Given count fails or drifts, then paging remains available and the UI changes to `Count unavailable` with documented help.

### User stories addressed

- User story 57: Run first page and count concurrently on distinct leases
- User story 58: Keep rows and paging when count fails
- User story 59: Explain and display complete limited-result count semantics

---
