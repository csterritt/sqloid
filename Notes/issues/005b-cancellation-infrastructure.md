## Issue 5b: Cancellation infrastructure (context + connection-scoped interrupt)

**Type**: AFK
**Blocked by**: Issue 5

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the reusable cancellation infrastructure layer from **Errors and cancellation bounds** and **Connection** module design: Go `context.Context` cancellation plus connection-scoped SQLite interrupt semantics on the pinned modernc driver. This is the infrastructure half of Issue 24, extracted early so that pre-execution schema validation (Issue 18) and other early cancellable workflows can use it without waiting for the full SELECT/write cancellation application layer.

Scope is limited to: cancellable context setup, connection-scoped interrupt invocation, settlement detection, late-success classification as cancelled, no force-close, and the mandatory pinned-driver capability evidence for interrupt behavior (CPU-bound and lock-wait cases). It does not cover SELECT page/count-specific cancellation wiring, write-phase cancellation, or in-flight UI feedback — those remain in Issue 24.

### How to verify

- **Manual**: Review the pinned modernc version's interrupt behavior on Linux and macOS.
- **Automated**: Release-blocking barrier-based integration tests prove connection-scoped interrupt works: interrupt a long CPU-bound query and settle within one second; interrupt a lock-wait and settle within five seconds; late success after cancellation is classified as cancelled and discarded; the connection is not force-closed; a subsequent request on the same connection is unaffected. Use synchronization barriers rather than sleeps except for explicit latency assertions.

### Acceptance criteria

- [ ] Given a cancellable context on a leased connection, when cancellation is requested, then connection-scoped SQLite interrupt is invoked and visible state can show `cancelling…` until settlement.
- [ ] Given success arrives after cancellation was requested, then it is classified as cancelled and discarded without force-closing the connection.
- [ ] Given a controlled CPU-bound query, then settlement occurs within one second of cancellation; given a lock-wait, settlement occurs no later than five seconds. A subsequent request on the connection is unaffected.

### User stories addressed

- User story 12: Reserve Ctrl+W for cancellable database work (infrastructure layer)
- User story 82: Settle cancellation visibly, safely, and within required bounds (infrastructure layer)

---
