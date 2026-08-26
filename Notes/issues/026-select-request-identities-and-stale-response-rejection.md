## Issue 26: SELECT request identities and stale-response rejection

**Type**: AFK
**Blocked by**: Issue 25

### Parent PRD

`PRD-sqloid.md`

### What to build

Extend the execution and first-page/count request identities introduced by Issue 24 to later page requests and viewport generations, so only current responses mutate the grid/cache. Cancellation, resize, deactivation, and newer executions must reject late results without starting replacement work before settlement.

### How to verify

- **Manual**: Start slow first/later pages, cancel or resize, then run another SELECT and observe that old rows never appear.
- **Automated**: Barrier-controlled model tests deliver responses out of order and assert identity/generation guards, cancellation classification, and replacement serialization.

### Acceptance criteria

- [ ] Given a late response from an old execution or request, then it cannot mutate current UI or cache state.
- [ ] Given resize or deactivation, then the old viewport generation is rejected even if its request succeeds.
- [ ] Given cancellation, then no replacement request starts until every replaced request has settled.
- [ ] Given a later-page response, then the Issue 24 execution/request guards and its viewport generation are tracked independently so the response is accepted only when all three are current.
- [ ] Given a newer execution begins after cancellation was requested, then late responses from the old execution are discarded and classified as cancelled even after the newer execution starts.

### User stories addressed

- User story 50: Cancel current SELECT requests and discard stale responses

---
