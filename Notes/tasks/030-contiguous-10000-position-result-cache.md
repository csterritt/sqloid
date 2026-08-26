# Tasks for #30: Contiguous 10,000-position result cache

Parent issue: #30
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify contiguous positional merging

**Type**: RED
**Output**: Failing pure tests cover absolute positions, duplicate-valued rows, adjacent append/prepend, overlap replacement, alternating traversal, and gap rejection.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create table-driven pure cache tests in `internal/resultcache` for Issue #30 and the Cache and snapshot invariant, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`. Model every row by its absolute logical result position rather than its value or slice index, and prove that duplicate-valued rows at different positions remain distinct. Cover initial insertion, forward-adjacent append, backward-adjacent prepend, exact and partial overlap replacement, pages spanning either retained edge, repeated overlap, and alternating forward/backward traversal. Assert ascending position order, exact retained start/end metadata, one row per position, replacement at matching positions, and rejection of stale nonadjacent pages that would create either a low-side or high-side gap without mutating rows or metadata. Keep this task test-only and isolate merge decisions from UI navigation and database fetching.

---

### 2. Implement contiguous page merging

**Type**: GREEN
**Output**: Cache range and merge tests pass without duplicating positions or admitting gaps.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the positional range and merge behavior in `internal/resultcache` required by Task 1. Store one contiguous inclusive range keyed by absolute logical positions, preserve duplicate row values as independent positions, append and prepend adjacent pages, and replace rows at overlapping positions without duplication. Classify pages after accounting for overlap and reject any page whose remaining positions are nonadjacent and would create a gap, leaving the prior cache and metadata unchanged. Keep rows ordered by ascending logical position regardless of traversal direction, expose retained range metadata needed by `internal/ui`, and avoid importing Bubble Tea or `internal/connection` concerns. Implement only enough to pass Task 1; hard-cap eviction belongs to Task 4.

---

### 3. Specify position-cap eviction

**Type**: RED
**Output**: Failing tests traverse beyond 10,000 positions in both directions and require deterministic opposite-end eviction with contiguous retained ranges.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the pure `internal/resultcache` tests to specify the independent hard limit of 10,000 retained logical positions. Traverse beyond the cap with forward-adjacent pages and require low-end eviction; traverse backward and require high-end eviction. Cover pages that land exactly at the cap, exceed it by one, cross it by multiple page sizes, replace overlap near each edge, span a retained edge, and alternate directions after prior eviction. After every merge, assert a contiguous retained interval of at most 10,000 positions, deterministic standard opposite-end eviction based on incoming traversal direction, exact start/end metadata, no duplicate positions, and unaffected values at retained overlaps. Include stale gap-producing pages after eviction and prove rejection is atomic. Keep this task test-only and do not add the byte cap owned by Issue #31.

---

### 4. Implement the 10,000-position cap

**Type**: GREEN
**Output**: Forward, backward, overlap, and alternating eviction tests pass at the hard cap.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic position-cap enforcement to `internal/resultcache`. After an accepted forward adjacent or overlapping merge, evict from the retained low end until no more than 10,000 logical positions remain; after an accepted backward merge, evict from the retained high end by the same rule. Preserve overlap replacement semantics, ascending order, contiguity, duplicate-valued positional identity, and atomic rejection of nonadjacent stale pages. Update retained start/end and row-cap eviction metadata consistently for `internal/ui` and later immutable snapshots, while keeping this cap independent so Issue #31 can add byte accounting without changing positional behavior. Implement only enough to make Tasks 1 and 3 pass.

---

### 5. Document cache position invariants

**Type**: DOCUMENT
**Output**: Wiki documentation records ranges, positions, merging, duplicates, stale rejection, and eviction direction.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #30 implementation and tests from `internal/resultcache` and its consumed range metadata in `internal/ui` into the appropriate pages under `Notes/wiki`. Document the single contiguous inclusive range of absolute logical positions, ascending ordering, separate identities for duplicate-valued rows, adjacent append/prepend behavior, same-position overlap replacement, and atomic rejection of nonadjacent stale pages. Record the independent 10,000-position hard cap and deterministic low-end eviction for forward arrivals versus high-end eviction for backward arrivals, including behavior under overlap and alternating traversal. Cross-reference Issues #25 and #30 and the Cache and snapshot invariant, History and UI Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the positional-cache walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/030-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/030-06/code-walkthrough`. Demonstrate absolute-position identity with duplicate-valued rows, forward append, backward prepend, partial and full overlap replacement, and atomic rejection of stale pages that would form gaps. Traverse beyond 10,000 positions in both directions and alternate direction after eviction, capturing exact retained ranges and deterministic opposite-end eviction while proving every intermediate range remains contiguous and bounded. Reference Issue #30 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
