## Issue 24: Scoped Ctrl+W cancellation and bounded settlement

**Type**: HITL
**Blocked by**: Issue 5, Issue 22, Issue 23

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement context cancellation plus connection-scoped SQLite interrupt for cancellable schema, estimate, SELECT, and write phases. Show settlement, discard late success, never force-close, and establish the mandatory pinned-driver capability evidence on Linux and macOS.

### How to verify

- **Manual**: Review cancellation behavior for CPU work, lock waits, page/count isolation, and an unaffected subsequent request on both supported systems.
- **Automated**: Release-blocking barrier tests assert independent interrupts, `cancelling…`, no replacement before settlement, one-second controlled CPU and five-second lock bounds, and late-success rejection.

### Acceptance criteria

- [ ] Given cancellable work, when Ctrl+W is pressed, then only applicable active requests are interrupted and visible state remains `cancelling…` until settlement.
- [ ] Given success after cancellation was requested, then it is classified as cancelled and discarded without force-closing the connection.
- [ ] Given controlled CPU or lock-wait cases, then settlement satisfies the required cross-platform bounds and later work is unaffected.

### User stories addressed

- User story 12: Reserve Ctrl+W for cancellable database work
- User story 82: Settle cancellation visibly, safely, and within required bounds

---
