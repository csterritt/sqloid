# Tasks for #33: Snapshot completeness, outcomes, and endpoints

Parent issue: #33
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify immutable snapshot metadata

**Type**: RED
**Output**: Failing matrix tests cover retained range, known total, endpoints, row/byte eviction, UTF status, failure position/reason, and independent terminal outcomes.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven metadata tests in `internal/history` and focused cache-to-snapshot boundary tests in `internal/resultcache` for Issue #33 and the Cache and snapshot invariant, History Module Design, and metadata Testing Decisions in `Notes/PRD-sqloid.md`. Specify an immutable typed snapshot metadata value independent of retained rows and presentation strings, containing the optional inclusive retained start/end range, optional known total, reached-low and reached-high endpoint observations, persistent row-cap and byte-cap eviction facts, UTF status, and independently typed terminal outcome. Cover success, cancellation, and failure, with cancellation/failure reason and optional one-based last failure position where applicable; include empty and nonempty retained ranges, unknown totals, both eviction flags together and separately, invalid UTF, and source/retrieved-value mutation attempts. Require defensive copies or value semantics so metadata cannot change after finalization, and require `truncated-by-byte-cap` to remain typed metadata without duplicating Issue #31's presentation literal. Keep this task test-only and do not classify completeness yet.

---

### 2. Implement the snapshot metadata model

**Type**: GREEN
**Output**: Immutable typed metadata tests pass independently of row storage and presentation strings.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the immutable typed snapshot metadata model in `internal/history`, with a narrow conversion boundary from authoritative `internal/resultcache` facts and lifecycle inputs supplied by `internal/ui`. Represent absent retained ranges, unknown totals, endpoints, row-cap eviction, persistent `truncated-by-byte-cap`, UTF status, terminal success/cancelled/failed outcome, reason, and optional one-based failure position without encoding these facts into labels or UI text. Deep-copy any mutable reason/detail data on construction and retrieval, validate range shape without rewriting observed facts, and keep metadata independently constructible and testable from snapshot row storage. Reuse the shared Issue #31 warning definition only at later presentation boundaries; do not duplicate it in the model. Implement only enough to make Task 1 pass, leaving completeness and endpoint classification to Tasks 3-4.

---

### 3. Specify completeness and endpoint classification

**Type**: RED
**Output**: Failing tests cover complete/partial/truncated combinations, limited-result semantics, count failure, short/empty observation, unknown remainder, no clamping, and ascending positions.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend table-driven tests in `internal/history` with focused `internal/resultcache` fixtures and `internal/ui` finalization cases to define truthful classification across known and unknown totals. Require `complete` to be exclusive and only possible when both logical endpoints are established and every row in the user's limited logical result is retained in ascending absolute position order; rows beyond Limit must be irrelevant. Require `truncated` whenever known or observed rows were evicted or lie beyond the retained range, and `partial` whenever unseen limited-result rows may remain or count/page work did not finish, allowing partial and truncated to coexist while neither coexists with complete. Cover count success, count failure, count/cache inconsistency, row-cap and byte-cap eviction, cancellation/failure before and after rows, partial page failure, low/high traversal, short and empty final-page observation, full pages with unknown remainder, zero retained rows, and limits below/at/above observations. Assert count unavailability permits only an observed short or empty page to establish the high endpoint, count inconsistencies are preserved without clamping rows, range, total, or endpoints, and all retained snapshot positions are ascending regardless of traversal direction. Keep this task test-only and preserve terminal outcome as an independent axis in every matrix case.

---

### 4. Implement truthful snapshot classification

**Type**: GREEN
**Output**: Completeness, endpoint, terminal-outcome, and `truncated-by-byte-cap` persistence tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure completeness and endpoint classification in `internal/history`, consuming immutable retained-range and eviction facts from `internal/resultcache` plus count, Limit, observed-page, and terminal lifecycle facts from `internal/ui`. Establish the high endpoint from an available count where applicable or from an actually observed short/empty final page when count is unavailable; never infer it from an unobserved remainder or clamp contradictory count/cache evidence. Compute exclusive complete versus independently coexisting partial/truncated labels over the limited logical result, preserve ascending absolute positions, and keep success/cancelled/failed terminal outcomes orthogonal to completeness. Carry reasons and one-based failure positions unchanged, retain row- and byte-cap facts cumulatively after traversal changes, and guarantee finalized `truncated-by-byte-cap` metadata persists even when later retained bytes are below the cap. Implement only enough to make Tasks 1 and 3 pass without adding result-history browsing or UI warning composition.

---

### 5. Document snapshot metadata semantics

**Type**: DOCUMENT
**Output**: Wiki documentation records every metadata field, label combination, endpoint rule, and separation from UI warnings.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #33 implementation and tests from `internal/history`, `internal/resultcache`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document every immutable typed field: retained inclusive range, optional known total, reached-low/reached-high endpoints, row- and byte-cap eviction, UTF status, completeness labels, terminal outcome, reason, and optional one-based failure position. Define complete exclusivity, truthful partial/truncated coexistence, limited-result semantics, ascending absolute positions, count-failure behavior, short/empty observed high endpoints, unknown remainder, and the no-clamping rule for inconsistent observations. Explain that terminal success/cancelled/failed is independent of completeness and that typed `truncated-by-byte-cap` persistence is separate from the shared Issue #31 UI/export warning string. Cross-reference Issues #24, #30, #31, and #33 and the SELECT lifecycle, Cache and snapshot invariant, UI/History Module Design, and metadata Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the snapshot-metadata walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/033-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/033-06/code-walkthrough`. Demonstrate immutable metadata independently of row storage, then exercise complete, partial, truncated, and partial-plus-truncated snapshots across known totals, count failure, limited results, row/byte eviction, short and empty final-page observations, and unknown remainder. Show ascending positions after forward and backward traversal, preserve contradictory count/cache evidence without clamping, and vary terminal success, cancellation, and failure independently with reasons and one-based failure positions. Capture persistent `truncated-by-byte-cap` metadata while showing that its presentation warning remains the shared Issue #31 definition rather than snapshot-model text. Reference Issue #33 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
