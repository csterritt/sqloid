# Issue #81 — Deep-copy cached BLOB values

Issue #81 closes the two ownership boundaries of the Cache and snapshot invariant of `Notes/PRD-sqloid.md` that [positional-result-cache.md](positional-result-cache.md) (Issue #30) and [byte-cap-oversized-values.md](byte-cap-oversized-values.md) (Issue #31) describe but left as shallow `copy` slices: the result cache must **own admitted BLOB bytes** and must **return independently mutable row snapshots** from every `Cache.Rows()` call. NULL, INTEGER, REAL, and TEXT values, `Row.Position`, the values slice shape, every value kind, ascending ordering, contiguity, `PayloadBytes()`, the retained range, and the row/byte-cap eviction metadata all remain exact. Cross-references: Issues #30–#31 (the cache invariant and caps), #47 (the immutable export capture that copies BLOB bytes byte-for-byte through `export.CaptureRows`), the Grid rendering/cache, Export formats and values, History, and Testing Decisions sections of `Notes/PRD-sqloid.md`, [positional-result-cache.md](positional-result-cache.md), [byte-cap-oversized-values.md](byte-cap-oversized-values.md), [immutable-export-capture.md](immutable-export-capture.md), [shared-typed-result-rendering.md](shared-typed-result-rendering.md), [snapshot-metadata.md](snapshot-metadata.md), and [result-history-browsing.md](result-history-browsing.md).

## The two ownership boundaries

`internal/resultcache/cache.go` is the only place that transfers page rows into cache-owned storage or hands cache-owned rows back to a caller. Issue #81 makes both boundaries deep-copy BLOB bytes:

- **Admission deep copy.** When a page row is accepted into cache-owned storage — on initial insertion, forward append, backward prepend, and every overlap-replacement path — the cache calls `copyValues` on the row's `[]result.Value`. `copyValues` now deep-copies `result.Value.Bytes` for `result.KindBlob` values via the established `result.NewBlob` idiom (`append([]byte(nil), v.Bytes...)`); NULL, INTEGER, REAL, and TEXT keep their by-value fields unchanged. The retained row never aliases the caller's BLOB backing storage, regardless of how the page row's value was constructed (`result.NewBlob` already copies, but Issue #81 makes the cache's own ownership boundary independent of that constructor contract).
- **Retrieval deep copy.** `Cache.Rows()` previously returned a shallow `copy` of the retained rows slice, so a caller mutating a returned BLOB byte slice would corrupt the cache and every later retrieval. `Rows()` now constructs each returned `Row` through `copyValues`, so each retrieval receives its own independent BLOB byte slice. Mutating a returned BLOB does not affect the cache or any later `Rows()` result.

`copyValues` is the single shared helper used by both `appendPageRow` (admission) and `Rows()` (retrieval), keeping the two boundaries consistent by construction. The helper preserves the values slice length and order, every non-BLOB field and kind, and `Row.Position`; it changes only BLOB backing storage.

## What is preserved unchanged

Issue #81 changes only BLOB byte ownership. Everything else the cache invariant defines is preserved exactly:

- **`Row.Position` and ascending ordering**: positions are independent of values and never re-derived; rows stay in ascending absolute-position order regardless of traversal direction.
- **Values slice shape and every value kind**: NULL, INTEGER, REAL, TEXT, and BLOB keep their `Kind` and the meaningful field for that kind; only BLOB `Bytes` are deep-copied.
- **Non-BLOB fields**: `Int`, `Float`, and `Str` are by-value (INTEGER/REAL/TEXT), so they need no copy; they are carried through unchanged.
- **`PayloadBytes()` accounting**: the Issue #31 retained-payload total is computed from `len(Bytes)` for BLOBs, so a deep copy with the same byte length produces the same total. Admission, overlap replacement, and eviction accounting are unchanged.
- **Retained range, contiguity, and cap behavior**: `Start()`/`End()`/`Len()`, the `MaxPositions` and `MaxPayloadBytes` caps, opposite-end eviction direction, `RowCapEvictions()`, and the persistent `TruncatedByByteCap()` disclosure are all untouched.
- **`result.Value`, serialization, accounting rules, and cache caps**: the `internal/result` package, the `LimitFailure` kinds and messages, `ValuePayload`/`RowPayload`, and the cap constants are unchanged. The deep copy lives entirely inside `internal/resultcache`.

## Regression tests

`internal/resultcache/blob_isolation_test.go` (Issue #81) is pure, table-driven where useful, and exercises both ownership boundaries across every admission path:

- **`TestBlobIsolationOnAdmission`** — initial insertion of mixed-kind rows (NULL, INTEGER, REAL, TEXT, BLOB) with shared-backing BLOB values; after `Merge`, mutating the caller's original BLOB slices leaves every retained row's BLOB bytes, kinds, non-BLOB fields, positions, ordering, `PayloadBytes()`, retained range, and cap metadata exact.
- **`TestBlobIsolationOnRowsRetrieval`** — successive `Rows()` calls return independent BLOB byte slices; mutating a returned BLOB does not corrupt the cache or any later retrieval, while position, kinds, non-BLOB values, ordering, and `PayloadBytes()` stay exact.
- **`TestBlobIsolationOnForwardAppend`** and **`TestBlobIsolationOnBackwardPrepend`** — the forward-append and backward-prepend admission paths deep-copy BLOB bytes; mutating the appended/prepended page's BLOB after `Merge` leaves the cache holding the originally admitted bytes with positions, ordering, kinds, and payload accounting exact.
- **`TestBlobIsolationOnOverlapReplacement`** — partial high-edge, partial low-edge, and exact full-range overlap replacement each deep-copy the replacing BLOB bytes; mutating the replacement page's BLOB after `Merge` leaves the cache holding the originally admitted replacement bytes with the replaced position, untouched rows, ordering, kinds, and payload accounting exact.
- **`TestBlobIsolationSurvivesRowsMutationAcrossRetrievals`** — three successive `Rows()` results are mutually independent and independent of the cache even when no caller mutation of the original page happens, exercising the retrieval boundary independently of the admission boundary.

The tests build BLOB values through a `sharedBlob` helper that constructs a `KindBlob` value whose `Bytes` slice is exactly the caller's slice (no copy), bypassing `result.NewBlob`'s defensive copy so the cache's own ownership boundary is the one under test. Every test asserts both byte equality with the original bytes and non-aliasing (the retained BLOB must not share backing storage with the caller's slice). Existing cap, eviction, admission, and snapshot tests in `cache_test.go`, `cap_test.go`, `bytecap_test.go`, `admission_test.go`, `payload_test.go`, and `snapshot_boundary_test.go` remain green and unchanged.
