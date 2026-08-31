# Tasks for #81: Deep-copy cached BLOB values

Parent issue: #81
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify cache-owned BLOB isolation

**Type**: RED  
**Output**: Failing result-cache tests prove BLOB bytes are isolated on admission and on every `Rows()` retrieval while all typed values, positions, ordering, and payload accounting remain exact.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused tests in `internal/resultcache`, preferably alongside the ownership and payload coverage in `cache_test.go`, `payload_test.go`, and `snapshot_boundary_test.go`. Build rows containing NULL, INTEGER, REAL, TEXT, and BLOB values, retain them through `Cache.Merge`, then mutate the original page's BLOB slice after admission and mutate BLOB slices returned by successive `Cache.Rows()` calls. Require every fresh retrieval to preserve the initially admitted bytes with no shared backing storage, while preserving each value kind and non-BLOB payload, exact logical positions, ascending order, `PayloadBytes()`, retained range, and existing row/byte-cap metadata. Cover overlapping replacement as well as initial insertion so every path that transfers a page row into cache ownership is exercised. Keep this task test-only and confirm the new assertions fail for the current shallow row/value copies without weakening existing cap, eviction, admission, or snapshot tests.

---

### 2. Enforce deep-copy ownership at cache boundaries

**Type**: GREEN  
**Output**: The result cache owns admitted BLOB bytes and returns independently mutable row snapshots; all Issue #81 and existing result-cache tests pass.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `internal/resultcache/cache.go` with the smallest shared row/value copy helper needed to satisfy Task 1. Clone `result.Value.Bytes` only for `result.KindBlob` when a page row is accepted into cache-owned storage, including initial, append, prepend, and overlap-replacement paths, and clone retained BLOB bytes again whenever `Cache.Rows()` constructs its caller-owned result. Preserve `Row.Position`, the values slice shape, every non-BLOB field and kind, ascending ordering, contiguity, admission behavior, payload accounting, eviction direction, and truncation metadata. Use `result.NewBlob` or the established byte-copy idiom from `internal/result/result.go` as the ownership reference, but do not change `result.Value`, serialization, accounting rules, or cache caps. Run the focused `internal/resultcache` tests and the repository-wide Go test suite after the RED tests pass.

---

### 3. Document result-cache BLOB ownership

**Type**: DOCUMENT  
**Output**: The wiki records both cache copy boundaries, preserved accounting and metadata, and the regression tests that enforce non-aliasing.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #81 implementation and tests from `internal/resultcache/cache.go` and the affected result-cache test files into the appropriate pages under `Notes/wiki`. Explain that accepted BLOB payloads are copied away from page/caller storage and that each `Rows()` result receives a second independent copy, while NULL, INTEGER, REAL, TEXT, positions, ordering, payload totals, cap behavior, and snapshot metadata remain unchanged. Cross-reference Issue #81, Issues #30-#31 and #47, and the Grid rendering/cache, Export formats and values, History, and Testing Decisions sections of `Notes/PRD-sqloid.md`. Update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the cache BLOB ownership walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/081-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/081-04/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate a mixed typed row entering the result cache, mutate the original BLOB source, retrieve and mutate one returned BLOB, then retrieve again and show the cache still returns the original bytes with unchanged position, kinds, values, ordering, and payload accounting. Include an overlap-replacement case and passing focused result-cache verification so both ownership boundaries and existing cap/eviction behavior are visible. Reference Issue #81 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
