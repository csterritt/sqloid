# Tasks for #32: Resize-safe vertical viewport recovery

Parent issue: #32
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify viewport recovery decisions

**Type**: RED
**Output**: Failing pure tests cover exact first-row preservation, dual-cap retained validity, low/high endpoint clamp, and containing-page request calculation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven pure viewport-recovery tests in `internal/ui`, using retained range and endpoint metadata from `internal/resultcache`, for Issue #32 and the SELECT lifecycle, Cache and snapshot invariant, Module Design, and resize Testing Decisions in `Notes/PRD-sqloid.md`. Given the prior first logical row, exact newly computed page size, post-eviction contiguous range, row- and byte-cap metadata, and known low/high endpoint state, require one explicit decision: preserve the exact row when it remains valid and retained; clamp to the known low or high retained endpoint when the target is outside an established result boundary; or request the page containing the target when it is not retained and cannot be safely clamped. Cover empty caches, single-row ranges, both caps having evicted data, exact range edges, targets below and above the range, known versus unknown endpoints, and page-size boundary arithmetic. Require containing-page offsets and limits to use absolute logical positions and the exact new page size. Keep this task test-only and free of Bubble Tea commands or database dispatch.

---

### 2. Implement viewport recovery calculations

**Type**: GREEN
**Output**: Preserve/clamp/fetch decision tests pass against post-eviction cache metadata.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the pure resize recovery decision seam in `internal/ui`, consuming authoritative contiguous retained-range, cap-eviction, and endpoint metadata from `internal/resultcache`. Preserve the prior exact first logical row only when it remains valid in the post-eviction retained range; otherwise choose the appropriate known retained low/high endpoint clamp or calculate the absolute request for the new-size page containing the required row. Keep row-cap and byte-cap validity equivalent, avoid using an inconsistent count to clamp, and make empty/unknown metadata produce a deterministic safe decision. Return enough typed information for orchestration to distinguish local preserve/clamp from fetch without issuing commands. Implement only enough to make Task 1 pass; request cancellation and generation handling belong to Task 4.

---

### 3. Specify resize request orchestration

**Type**: RED
**Output**: Failing model tests cover exact new page size, pending old-generation cancellation/rejection, settlement before replacement, and active/idle resize branches.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model tests in `internal/ui` with the controllable `internal/connection` fake for resize orchestration around an active SELECT. Compute the exact new page size from complete visible data rows after current builder, border, status/count, frozen-header, and footer allocation; cover grow, shrink, unchanged, minimum supported, and too-small restoration cases. For idle active SELECTs, require immediate local preserve/clamp with no request when retained metadata suffices, or one containing-page request using the exact new size when fetch is required. For a pending old-size page, require viewport-generation advancement, scoped cancellation or invalidation, rejection of both late success and late failure from the old generation, no replacement request before true settlement, and exactly one correctly sized containing-page request afterward when needed. Cover count-only pending work remaining independent, page-plus-count concurrency, repeated resize before settlement, and inactive/finalized/history contexts that must not fetch. Keep this task test-only and assert `internal/resultcache` row/byte invariants after every accepted response.

---

### 4. Integrate resize-safe viewport recovery

**Type**: GREEN
**Output**: Model tests pass without stale overwrites or violations of row/byte cache invariants.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate terminal resize handling in `internal/ui` with the pure decision logic from Task 2, `internal/resultcache` metadata, and the existing cancellable page-request lifecycle in `internal/connection`. Recompute the exact page size from complete visible result rows, advance the viewport generation whenever old-size page work must become stale, and route every response through current execution, request, and generation guards. Preserve or clamp locally when valid; otherwise cancel/invalidate pending old-generation page work, wait for actual settlement before dispatching one replacement request for the page containing the required first row, and apply only a current response. Keep independent count work intact, coalesce repeated resizes to the latest generation and size, avoid fetches for inactive/history/finalized results, and preserve both the 10,000-position and 64 MiB contiguous cache invariants on the replacement merge. Implement only enough to make Tasks 1 and 3 pass without changing horizontal resize behavior owned by Issue #29.

---

### 5. Document vertical resize recovery

**Type**: DOCUMENT
**Output**: Wiki documentation records preserve/clamp/fetch rules, page-size recomputation, generations, and settlement.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #32 implementation and tests from `internal/ui`, `internal/resultcache`, and `internal/connection` into the appropriate pages under `Notes/wiki`. Document exact page-size recomputation from complete visible rows and the recovery decision order: preserve the exact prior first logical row when valid and retained after dual-cap eviction, clamp only to a known retained low/high endpoint when appropriate, otherwise fetch the new-size page containing the required row. Record viewport-generation advancement, old-size success/failure rejection, scoped cancellation/invalidation, mandatory settlement before replacement, repeated-resize handling, count independence, and active versus idle/inactive/history branches. Cross-reference Issues #25, #26, #30, #31, and #32 and the Identities and state, SELECT lifecycle, Cache and snapshot invariant, UI/Connection Module Design, Testing Decisions, and manual layout matrix sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the viewport-resize walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/032-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/032-06/code-walkthrough`. Resize at first, middle, and end logical positions with targets retained and unretained after row-cap and byte-cap eviction; capture exact first-row preservation, low/high endpoint clamps, containing-page calculation, and exact new page size. Demonstrate idle local recovery, idle fetch, resize with a pending page and independent count, old-generation cancellation/invalidation, late success and failure rejection, settlement before the sole replacement request, and repeated resize resolving to the latest generation. Include inactive/history controls that issue no fetch and show cache contiguity and both caps after recovery. Reference Issue #32 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review viewport resize behavior

**Type**: REVIEW
**Output**: Human verifies first/middle/end positions, retained/unretained rows, and idle/pending resize.
**Depends on**: 6

Review resize decisions and orchestration in `internal/ui`, retained metadata and merge invariants in `internal/resultcache`, page cancellation/settlement in `internal/connection`, wiki updates, and `Notes/walkthroughs/032-06/code-walkthrough` against Issue #32. At the required terminal sizes, resize from first, middle, and end positions with prior first rows retained, evicted by each cap, below a known low endpoint, above a known high endpoint, and unretained with unknown boundaries. Confirm exact page-size arithmetic, preserve/clamp/fetch choices, containing-page requests, idle and pending branches, count independence, repeated-resize behavior, old-generation late-response rejection, no replacement before settlement, and intact row/byte cache invariants before approving the issue.

---
