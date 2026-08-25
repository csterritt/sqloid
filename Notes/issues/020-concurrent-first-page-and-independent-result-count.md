## Issue 20: Concurrent first page and independent result count

**Type**: HITL
**Blocked by**: Issue 5, Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Launch first-page and complete-limited-result count requests concurrently on distinct leases in both journal modes. Introduce the SELECT execution ID and distinct first-page/count request IDs required by this concurrency; a response mutates state only when its execution and request identities are current. Preserve independent autocommit semantics, count wording, failure isolation, stale-response rejection, and release-blocking capability evidence. Issue 22 extends these identities for later pages, viewport generations, resize, deactivation, and cancellation.

### How to verify

- **Manual**: Review behavior in WAL and rollback-journal fixtures with a delayed count and an external writer.
- **Automated**: Mandatory Linux/macOS barrier-based integration tests prove overlap, distinct leases, unchanged journal mode, drift handling, count SQL/Limit semantics, independent failures, and execution/request identity rejection of delayed superseded responses.

### Acceptance criteria

- [ ] Given a SELECT starts, then first page and count launch concurrently on two distinct dedicated connections without hidden serialization.
- [ ] Given count succeeds, then the exact limited-result header wording is shown without clamping independently read pages.
- [ ] Given count fails or drifts, then paging remains available and the UI changes to `Count unavailable` with documented help.
- [ ] Given a first-page or count response, then it mutates state only when both its SELECT execution ID and distinct request ID are current; a delayed response from a superseded execution is discarded.

### User stories addressed

- User story 57: Run first page and count concurrently on distinct leases
- User story 58: Keep rows and paging when count fails
- User story 59: Explain and display complete limited-result count semantics

---
