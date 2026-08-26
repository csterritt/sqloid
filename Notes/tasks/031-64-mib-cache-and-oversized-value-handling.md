# Tasks for #31: 64 MiB cache and oversized-value handling

Parent issue: #31
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify retained-payload accounting

**Type**: RED
**Output**: Failing pure tests cover raw TEXT/BLOB bytes, 8-byte INTEGER/REAL, zero-byte NULL, exact totals, and BLOB identity.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven pure value, row, and retained-range accounting tests in `internal/resultcache` for Issue #31 and the Cache and snapshot invariant in `Notes/PRD-sqloid.md`. Cover empty and multibyte TEXT by raw encoded byte length, empty and arbitrary BLOBs by exact byte length, INTEGER and finite REAL values at exactly 8 bytes each, NULL at zero bytes, mixed rows, repeated values, replacement at an existing logical position, and exact totals across retained rows. Prove accounting excludes Go/model container overhead and cache metadata, does not use display width or formatted token length, and preserves BLOB type and byte-for-byte identity rather than converting it to text. Keep this task test-only and independent from cap eviction and SQLite scanning.

---

### 2. Implement payload accounting

**Type**: GREEN
**Output**: Row/value accounting tests pass independently of model overhead.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement exact retained-payload accounting in `internal/resultcache` using the typed result values supplied by the connection boundary. Count raw TEXT and BLOB bytes, exactly 8 bytes for each INTEGER or REAL, and zero for NULL; aggregate value costs into row and cache totals without including model, slice, string-header, metadata, or other implementation overhead. Preserve BLOB values as copied exact bytes with their distinct type, and update totals correctly when logical positions are inserted, replaced through overlap, or evicted. Keep the accounting API pure and usable by cache admission decisions, `internal/ui` metadata, and later snapshot/export work. Implement only enough to make Task 1 pass without enforcing the 64 MiB cap yet.

---

### 3. Specify byte-cap eviction and disclosure

**Type**: RED
**Output**: Failing cache/model tests cover independent 64 MiB cap, opposite-end eviction, persistent typed metadata, and the shared exact truncation warning.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend pure `internal/resultcache` tests and scripted rendering/model tests in `internal/ui` to specify the independent 64 MiB retained-payload cap alongside Issue #30's 10,000-position cap. Cross the byte cap in forward and backward traversal, by exact-boundary and one-byte cases, through overlap replacement that raises or lowers retained bytes, and while alternating directions; require standard opposite-end eviction until both caps hold and a contiguous retained range remains. Require persistent typed `truncated-by-byte-cap` metadata once byte eviction has occurred, including after later navigation falls below the cap and through result finalization/history metadata. Assert the result header uses the single shared Issue #31 definition and renders exactly `Result truncated: 64 MiB cache limit`, with no duplicate UI literal, while row-cap-only eviction does not set byte-cap metadata. Keep this task test-only and leave export-flow consumption to its later owning issue.

---

### 4. Implement the 64 MiB retained cache cap

**Type**: GREEN
**Output**: Bidirectional byte-cap and persistent disclosure tests pass alongside the position cap.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Enforce the 64 MiB retained-payload cap in `internal/resultcache` independently and cumulatively with the existing 10,000-position cap. For accepted forward pages evict complete rows from the low end, and for accepted backward pages evict complete rows from the high end, until both caps hold; preserve contiguity, overlap replacement, exact retained-byte totals, BLOB identity, and atomic stale-gap rejection. Add persistent typed byte-truncation metadata and define the shared presentation value for `Result truncated: 64 MiB cache limit` at this issue's authoritative result metadata boundary. Have `internal/ui` render the shared definition rather than duplicating its literal, preserve disclosure through subsequent traversal and snapshot finalization, and expose the same definition for later export flows. Implement only enough to make Tasks 1 and 3 pass without adding oversized-page/value failures assigned to Task 6.

---

### 5. Specify oversized page/value failures

**Type**: RED
**Output**: Failing SQLite/cache tests require complete leading rows, no partial row, one-based first-failure position, and both exact page/value messages.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add `modernc.org/sqlite` integration tests in `internal/connection`, admission tests in `internal/resultcache`, and focused model tests in `internal/ui` for the two distinct Issue #31 over-limit failures. For a fetched page whose retained rows collectively exceed 64 MiB, require retention of only complete leading rows that fit, failure at the first nonfitting absolute logical result position, no bytes or fields from that row, preserved previously valid cache rows, and the exact message `result page exceeds the 64 MiB v1 limit at row N`. For one row or value that exceeds the connection-local 64 MiB value limit, require early typed failure from the SQLite scan boundary, no partial row retention, exact BLOB bytes for all earlier complete rows, the one-based logical result position, and the exact message `result value exceeds the 64 MiB v1 limit at row N`. Cover non-first pages, backward requests, boundary-sized values/pages, TEXT and BLOB payloads, and prove the page and value failure kinds cannot be conflated. Keep this task test-only.

---

### 6. Implement oversized-result handling

**Type**: GREEN
**Output**: Page/value limit tests pass with exact messages, positions, retention, and BLOB bytes.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement typed oversized-result handling across `internal/connection`, `internal/resultcache`, and `internal/ui`. At the SQLite scan boundary, preserve typed TEXT/BLOB/INTEGER/REAL/NULL values and exact BLOB bytes, stop a connection-local oversized row or value without exposing a partial row, and report its one-based absolute logical result position as a value-limit failure. At cache admission, accept only complete leading rows from a page while they fit the 64 MiB v1 envelope, stop at the first nonfitting position without partial mutation, retain earlier complete rows and prior valid cache content under both caps, and classify the distinct page-limit failure. Carry typed failure kind and position into result metadata/history and render the exact page or value message from shared definitions, without rebuilding messages at multiple layers. Implement only enough to make Task 5 and all prior Issue #31 tests pass.

---

### 7. Document byte limits and failures

**Type**: DOCUMENT
**Output**: Wiki documentation records accounting, eviction, metadata, exact warnings/errors, and no-partial-row guarantees.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #31 implementation and tests from `internal/resultcache`, `internal/connection`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document exact retained-payload accounting for every SQLite value type, exclusion of model overhead, exact BLOB identity, and independence of the 64 MiB and 10,000-position caps. Record directional complete-row eviction, persistent typed `truncated-by-byte-cap` metadata, and the shared exact warning `Result truncated: 64 MiB cache limit`. Distinguish page-envelope and connection-local value failures, their exact messages, one-based absolute positions, complete-leading-row retention, prior-cache preservation, and the no-partial-row guarantee. Cross-reference Issues #5, #30, and #31 and the Cache and snapshot invariant, Errors and cancellation bounds, Connection/UI/History Module Design, and high-risk Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the byte-cap walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/031-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/031-08/code-walkthrough`. Demonstrate exact mixed-type payload totals and unchanged BLOB bytes, then cross the independent 64 MiB cap in both traversal directions while showing complete-row opposite-end eviction, coexistence with the position cap, contiguous retained ranges, and persistent `truncated-by-byte-cap` disclosure using the shared exact warning. Run separate oversized-page and oversized-TEXT/BLOB-value fixtures, capturing complete leading rows, no partial failing row, one-based non-first-page positions, preserved prior retention, and both exact failure messages. Reference Issue #31 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 9. Review byte-cap behavior

**Type**: REVIEW
**Output**: Human confirms cap crossing, persistent warning, oversized page, and oversized value fixtures.
**Depends on**: 8

Review payload accounting and admission in `internal/resultcache`, bounded SQLite scanning in `internal/connection`, disclosure and failure rendering in `internal/ui`, wiki updates, and `Notes/walkthroughs/031-08/code-walkthrough` against Issue #31. Use fixtures that cross the byte cap forward and backward, overlap with larger and smaller replacements, alternate traversal, and independently reach the position cap. Confirm exact totals, deterministic complete-row opposite-end eviction, persistent typed warning metadata and exact shared header text, exact BLOB bytes, and contiguous retained ranges. Exercise separate oversized page, TEXT value, and BLOB value failures at first and later logical positions; verify complete leading-row retention, no partial row, preserved prior cache, exact one-based positions, and both exact messages before approving the issue.

---
