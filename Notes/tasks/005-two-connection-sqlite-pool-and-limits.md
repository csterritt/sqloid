# Tasks for #5: Two-connection SQLite pool and limits

Parent issue: #5
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify exact pool and per-connection configuration

**Type**: RED
**Output**: Failing integration tests require an exact-two pool, five-second busy timeout, and exact 64 MiB SQLite length limit on every connection.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused `modernc.org/sqlite` integration tests in `internal/connection` for Issue #5 and the Connection pool, limits, and busy handling decision in `Notes/PRD-sqloid.md`. Extend the shared opener established by Issue #2 as the test entrypoint, and require the `database/sql` pool to maintain both its minimum and maximum at exactly two usable connections. Inspect each physical connection rather than only the pool as a whole, prove every connection receives a five-second busy timeout and an exact 64 MiB connection-local SQLite length limit, and make failures identify which per-connection invariant is absent. Keep this task test-only and do not prescribe the production implementation.

---

### 2. Configure the exact-two SQLite pool

**Type**: GREEN
**Output**: Pool-size and per-connection configuration tests pass without changing journal mode.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Modify `internal/connection` to make the Issue #2 shared read-write opener own an exact-two `database/sql` pool and configure every underlying modernc connection with the PRD-required five-second busy handling and exact 64 MiB SQLite length limit. Follow the existing startup validation, non-creating `mode=rw`, schema-probe, error-wrapping, and resource-cleanup patterns; use supported driver configuration mechanisms that apply to newly created physical connections, and preserve the input database's journal mode. Implement only enough production behavior to pass Task 1 without exposing the pinned driver outside the Connection abstraction.

---

### 3. Specify dedicated leasing and journal preservation

**Type**: RED
**Output**: Failing WAL and rollback-journal tests prove concurrent callers receive distinct dedicated leases and journal mode remains unchanged.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-driven integration tests in `internal/connection` for the dedicated-leasing contract in Issue #5, the SELECT lifecycle, and the high-risk pool coverage in `Notes/PRD-sqloid.md`. Exercise concurrent harmless requests against fixtures already placed in WAL and rollback-journal modes, hold both leases concurrently to prove callers receive distinct physical connections rather than silently serialized access, and verify each lease has the required busy and length configuration. Record journal mode before opening and after lease use, assert that Connection never sets or mutates it, use synchronization barriers rather than timing sleeps except for an explicit bound, and leave page/count application behavior for later issues.

---

### 4. Implement dedicated connection leasing

**Type**: GREEN
**Output**: Lease lifecycle, distinct-connection, and journal-preservation tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement dedicated lease acquisition, ownership, and release in `internal/connection` so each concurrent request exclusively uses one configured physical connection until its work has settled. Build on the exact-two pool from Task 2, make cleanup safe on success and error, prevent a released or unsettled lease from being reused incorrectly, and retain Connection ownership of all driver-specific details. Do not add journal pragmas or serialize independent leases, and limit the change to the reusable leasing foundation required by Issue #5 and later page/count requests.

---

### 5. Document pool and connection limits

**Type**: DOCUMENT
**Output**: Wiki documentation records pool ownership, leasing, busy handling, length limits, and journal invariants.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #5 implementation and tests into the appropriate pages under `Notes/wiki`. Document `internal/connection` ownership, the exact minimum and maximum pool size of two, dedicated lease lifecycle, five-second busy handling on every physical connection, the exact 64 MiB connection-local SQLite length limit, cleanup behavior, and the invariant that WAL and rollback-journal modes are neither set nor changed. Cross-reference Issue #5 and the relevant Connection pool and Module Design sections of `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md` for any added or materially changed pages, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior log entries.

---

### 6. Create the connection-pool walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/005-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/005-06/code-walkthrough`. Demonstrate the `internal/connection` exact-two pool and dedicated lease lifecycle with harmless concurrent requests in both WAL and rollback-journal fixtures, show evidence that the leases are distinct and each physical connection has the five-second busy timeout and exact 64 MiB length limit, and verify journal mode remains unchanged. Include the relevant automated test results and references to Issue #5 and `Notes/PRD-sqloid.md`, and place every generated walkthrough artifact under the approved directory.

---
