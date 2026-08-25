## Issue 24: Scoped Ctrl+W cancellation and bounded settlement

**Type**: HITL
**Blocked by**: Issue 5b, Issue 22, Issue 23

### Parent PRD

`PRD-sqloid.md`

### What to build

Apply the Issue 5b cancellation infrastructure to active SELECT first-page, count, and later page requests. Wire scoped Ctrl+W, visible settlement, independent page/count interruption, late-success rejection, no force-close, and the mandatory SELECT cancellation capability evidence on Linux and macOS. Issue 18 owns schema-validation integration, Issue 37 owns estimate integration, Issue 38 owns beginning/executing write integration, and Issue 39 owns post-commit-boundary behavior.

### How to verify

- **Manual**: Review SELECT cancellation behavior for CPU work, lock waits, page/count isolation, and an unaffected subsequent request on both supported systems.
- **Automated**: Release-blocking barrier tests assert independent page/count interrupts, `cancelling…`, no replacement before settlement, one-second controlled CPU and five-second lock bounds, and late-success rejection.

### Acceptance criteria

- [ ] Given active SELECT page and/or count work, when Ctrl+W is pressed, then only those active requests are interrupted independently and visible state remains `cancelling…` until all requested cancellations settle.
- [ ] Given SELECT success after cancellation was requested, then it is classified as cancelled and discarded without force-closing the connection.
- [ ] Given controlled SELECT CPU or lock-wait cases, then settlement satisfies the required cross-platform bounds and later work is unaffected.

### User stories addressed

- User story 12: Reserve Ctrl+W for cancellable database work
- User story 82: Settle cancellation visibly, safely, and within required bounds

---
