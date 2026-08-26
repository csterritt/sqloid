# Tasks for #6: Cancellation infrastructure (context + connection-scoped interrupt)

Parent issue: #6
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify cancellable request lifecycle semantics

**Type**: RED
**Output**: Failing fake-connection tests cover cancellation flags, visible settlement state, late-success classification, no force-close, and safe lease reuse.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic fake-connection tests in `internal/connection` for the reusable request lifecycle defined by Issue #6 and the Execution identities, Errors and cancellation bounds, and Connection module sections of `Notes/PRD-sqloid.md`. Cover unique request identity, cancellable context ownership, an atomic cancellation-requested flag, observable cancelling-versus-settled state, exactly one connection-scoped interrupt request, success arriving after cancellation being discarded and classified as cancelled, and errors settling normally. Prove cancellation never force-closes the physical connection, no replacement work can reuse its dedicated lease before settlement, and the same lease or connection is safe for subsequent work after settlement; use controllable barriers and channels rather than sleeps and keep this task test-only.

---

### 2. Implement the reusable cancellation lifecycle

**Type**: GREEN
**Output**: Request cancellation and settlement tests pass behind the Connection abstraction.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the driver-independent cancellable request lifecycle in `internal/connection`, building on Issue #5's dedicated lease ownership while keeping request identity, cancellation flags, interrupt dispatch, result classification, and settlement hidden behind the Connection abstraction. Ensure cancellation is idempotent, visible lifecycle state remains cancelling until the in-flight operation actually settles, late success cannot escape as success, lease release occurs only after settlement, and no path force-closes a connection. Preserve hooks or narrow seams for the controlled fake and the pinned-driver interrupt integration in later tasks, without adding SELECT page/count, write-phase, history, or UI-specific cancellation behavior owned by later issues.

---

### 3. Specify modernc interrupt capability bounds

**Type**: RED
**Output**: Failing barrier-based integration tests cover connection-scoped CPU interruption, lock-wait interruption, isolation, and unaffected subsequent work.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add mandatory Linux/macOS capability integration tests in `internal/connection` against the exact modernc version pinned by Issue #2. Use synchronization barriers to establish that controlled CPU-bound work and a controlled SQLite lock wait have started before cancellation, then require connection-scoped interruption to settle CPU work within one second and lock-wait work within five seconds. Run independent work on the other dedicated lease to prove isolation, cover a deliberately released late-success result being classified as cancelled and discarded, assert no force-close, and prove subsequent harmless work on the interrupted physical connection is unaffected. Keep sleeps out of ordering logic except where measuring the explicit PRD latency bounds, and make these tests release- and dependency-upgrade-blocking.

---

### 4. Integrate connection-scoped SQLite interruption

**Type**: GREEN
**Output**: CPU work settles within one second, lock waits within five seconds, late success is discarded, and the connection remains reusable.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate the pinned modernc driver's supported connection-scoped SQLite interrupt capability into `internal/connection` and connect it to the reusable lifecycle from Task 2. Keep the interrupt targeted to the physical connection leased by the cancelled request, combine it with Go context cancellation, preserve the five-second busy bound, wait for true operation settlement before releasing the lease, and retain the lifecycle's cancellation-wins handling for late success. Do not force-close connections, affect concurrent work on the second lease, weaken capability assertions, or leak modernc-specific APIs through Connection; if the pinned version cannot meet the mandatory CPU, lock-wait, isolation, and reuse tests, change the vetted pin or implementation rather than accepting best-effort behavior.

---

### 5. Document cancellation infrastructure

**Type**: DOCUMENT
**Output**: Wiki documentation records request identity, interrupt scope, settlement, bounds, and driver assumptions.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #6 implementation, fake tests, and pinned-driver capability tests into the appropriate pages under `Notes/wiki`. Document request and lease identity, context and cancellation-flag ownership, connection-scoped interrupt targeting, visible cancelling and settlement semantics, cancellation-wins late-success classification, no-force-close and safe-reuse guarantees, the one-second CPU and five-second lock-wait bounds, isolation between leases, and assumptions tied to the exact modernc version on Linux and macOS. Distinguish this infrastructure from the SELECT/write/UI wiring deferred to Issue #28, cross-reference Issue #6 and the relevant PRD sections, update `Notes/wiki/index.md`, and append the required dated ingest record to `Notes/wiki/log.md` without changing older entries.

---

### 6. Create the cancellation-infrastructure walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/006-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/006-06/code-walkthrough`. Demonstrate the fake-backed cancellation lifecycle and the pinned-modernc capability evidence for controlled CPU work, lock waiting, independent lease isolation, late-success rejection, settlement before lease reuse, no force-close, and successful subsequent work on the same connection. Capture the one-second and five-second bound results on the available supported platform, explain the Linux/macOS release requirement, reference Issue #6, Issue #28's deferred application scope, and `Notes/PRD-sqloid.md`, and keep all generated artifacts in the approved directory.

---

### 7. Review driver cancellation capability

**Type**: REVIEW
**Output**: Human confirms the pinned driver behavior and capability evidence on Linux and macOS.
**Depends on**: 6

Review the completed `internal/connection` lifecycle, pinned-driver integration, release-blocking tests, wiki updates, and walkthrough against Issue #6 and the cancellation requirements in `Notes/PRD-sqloid.md`. Confirm on Linux and macOS that cancellation targets only the leased physical connection, CPU work settles within one second, lock waits settle within five seconds, independent work is unaffected, late success is discarded as cancelled, no connection is force-closed, no replacement starts before settlement, and subsequent work remains healthy. Verify the documented modernc assumptions match the exact pinned dependency before approving the capability or requiring a driver-version or implementation change.

---
