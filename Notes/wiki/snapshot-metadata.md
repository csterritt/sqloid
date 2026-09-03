# Snapshot Metadata and Completeness Classification

Issue #33 delivers Sqloid's immutable typed snapshot metadata and truthful completeness/endpoint classification, per the Cache and snapshot invariant and History Module Design decisions in `Notes/PRD-sqloid.md` (user story #55: truthful retained range, endpoints, known-total, eviction, completeness, and terminal-outcome metadata in ascending logical-position order).

## Immutable typed snapshot metadata — `internal/history/snapshot.go`

`SnapshotMetadata` is a pure value struct with scalar fields and copy semantics: constructing it copies every input, and value copies are fully independent, so a finalized value can never change through any alias. It is independent of retained row storage and of all presentation strings — rendering, including the shared Issue #31 byte-cap warning, happens only later from these typed facts. The fields:

- **Retained inclusive range** — `HasRetainedRange` (absent means zero retained rows), `RetainedStart`, `RetainedEnd`, meaningful only when present.
- **Optional known total** — `HasKnownTotal` / `KnownTotal`: the total of the limited logical result (the count of the SELECT including the user's Limit), absent when the count did not succeed.
- **Endpoint observations** — `ReachedLow` and `ReachedHigh`: whether traversal established the first and last logical rows.
- **Eviction facts** — `RowCapEvicted` / `RowCapEvictions` (cumulative count of `MaxPositions` evictions, retained cumulatively after traversal changes) and the persistent `TruncatedByByteCap` typed `truncated-by-byte-cap` disclosure, which stays set even when later retained bytes fall below the cap. Its presentation is the shared `internal/result` `ByteCapWarning`, never duplicated in the model (architecture-style test scans `internal/history` production files for the literal).
- **UTF status** — `InvalidUTF`: at least one retained TEXT value required maximal invalid-UTF-8 replacement.
- **Terminal outcome** — independently typed `Outcome` (`OutcomeSuccess`, `OutcomeCancelled`, `OutcomeFailed`) with `Reason`, and `HasFailurePosition` / `FailurePosition`: an optional one-based last failure position applicable only to cancellation and failure outcomes.

`NewSnapshotMetadata(facts, life)` validates shape without rewriting observed facts: a supplied range must have `End >= Start`, the outcome must be one of the three typed outcomes, and a failure position must be one-based and only accompany cancellation or failure. Absent facts are zero values, never rewritten observations.

`CacheFacts` / `FactsFromCache(c)` is the narrow conversion boundary from the authoritative [positional result cache](positional-result-cache.md): retained range, cumulative row-cap evictions, and the persistent byte-cap disclosure, copied out by value so later cache activity never changes converted facts.

## Truthful classification — `internal/history/snapshot_classify.go`

`Classify(meta, traversal)` computes a `Completeness` value `{Complete, Partial, Truncated}`:

- **`complete` is exclusive** and possible only when both logical endpoints are established, every row of the limited logical result is retained in ascending absolute-position order, no eviction occurred, all count and page work finished, and count/cache evidence is not contradictory. Rows beyond the user's Limit are irrelevant — the known total already counts only the limited result. An empty logical result (high endpoint 0) is fully retained vacuously.
- **`truncated`** means known or observed rows were evicted (row-cap or persistent byte-cap facts) or lie beyond the retained range (retained end short of the established high endpoint).
- **`partial`** means unseen limited-result rows may remain (unknown remainder, an unobserved high endpoint with rows beyond the range), or count/page work did not finish, or count/cache evidence is contradictory.
- **Partial and truncated may coexist** (e.g. unknown remainder plus byte-cap eviction, or an inconsistent count above the retained range); neither coexists with complete.

### Endpoint rules

- With a known total, the high endpoint comes from the count. Otherwise **only an actually observed short or empty final page establishes the high endpoint** — never an unobserved remainder (count success, count failure, and count/cache inconsistency behavior all covered in the test matrix). The low endpoint comes from the `ReachedLow` observation. Issue #73 extends this to the first page: an accepted first-page response returning fewer rows than the layout-derived requested size (including zero rows) sets `pageExhausted` and establishes the high endpoint through `ObservedShortFinalPage`, even when the count is unavailable. An exactly-full first page leaves the high endpoint unknown. Stale or cancelled first-page responses are fully inert and cannot establish or clear any endpoint.
- **No clamping**: contradictory count/cache evidence (count below or above the retained range) is preserved without clamping rows, range, total, or endpoints; classification reports `partial` (plus `truncated` when known rows lie beyond the range) and never `complete`.

`TraversalFacts` carries the count/Limit/observation inputs: `HasLimit`/`Limit` (executed metadata), `CountWorkFinished`, `PageWorkFinished`, `ObservedShortFinalPage`, and `CountCacheInconsistent`.

## UI finalization — `internal/ui/snapshot_metadata.go`

At finalization the model converts its authoritative state through the narrow boundary: `Model.SnapshotFacts(Finalization)` returns the immutable metadata plus the traversal facts captured at the same instant. The known total and executed Limit come from the settled `result.CountState`, the retained range and eviction facts from the viewport cache ([positional result cache](positional-result-cache.md), [byte cap](byte-cap-oversized-values.md)), and the terminal outcome, reason, failure position, UTF status, endpoint observations, and work-finished flags from the `Finalization` inputs owned by the paging and cancellation seams ([scoped cancellation](scoped-select-cancellation.md)). Issue #73 adds that `pageExhausted` feeds `ObservedShortFinalPage` consistently so an accepted short or empty first page establishes `ReachedHigh` (and `ReachedLow` for an empty page, where both endpoints sit at position 0) even when the cache retained no rows. Issue #75 adds that invalid-UTF truth is sourced from the accepted active page (`m.Result.Page.InvalidUTF`) into `Finalization.InvalidUTF` at finalization, so the immutable snapshot records the same invalid-UTF fact that was true during execution; the persistent byte-cap truth continues to come from the authoritative cache via `FactsFromCache` without re-deriving it from current payload size. The returned values are independent of the model: later navigation, eviction, or count settlement cannot change them. No completeness or warning text is produced at this boundary.

## Separation from UI warnings

Terminal success/cancelled/failed is independent of completeness in every matrix case. The typed `truncated-by-byte-cap` persistence is separate from the shared Issue #31 UI/export warning string, which remains defined once in `internal/result` ([byte cap](byte-cap-oversized-values.md)); the snapshot model adds no label, literal, or warning text.

## Tests

- `internal/history/snapshot_test.go` — metadata matrix (success/cancellation/failure with reasons and one-based positions, empty and nonempty ranges, unknown totals, eviction flags together and separately, invalid UTF), validation rejections, value-semantics mutation attempts on inputs, copies, and later cache activity, and the no-presentation-duplication source scan.
- `internal/history/snapshot_classify_test.go` — the classification matrix (complete exclusivity, limited-result semantics with limits below/at/above observations, count failure, short/empty observation, full pages with unknown remainder, zero retained rows, partial-only, truncated-only, partial+truncated, no-clamping, and terminal outcome as an independent axis).
- `internal/resultcache/snapshot_boundary_test.go` — cache-to-snapshot boundary: facts track the cache lifecycle, ascending absolute positions after backward traversal, and persistent byte-cap disclosure surviving below-cap navigation.
- `internal/ui/snapshot_finalize_test.go` — finalization cases over the model and metadata independence from later navigation. Issue #73 adds: an accepted empty first page finalizes with both endpoints established (position 0) and classifies complete when count work finished; an accepted short first page finalizes with `ReachedHigh` from `ObservedShortFinalPage` and classifies complete when count work finished and all rows are retained.
- `internal/ui/snapshot_warning_roundtrip_test.go` (Issue #75) — invalid-UTF and byte-cap warning metadata round trips from accepted active-page settlement through finalization into immutable `SnapshotMetadata`, through local historical projection at multiple terminal page sizes, through browsing and pre-destination export presentation, and through serializer-spy exclusion proofs; stored rows/metadata remain unchanged after mutating live pages, projected views, source BLOBs, and cache state.

Cross-references: Issues #24 (count), #30 (positional cache), #31 (byte cap), #33 (this work), #72 (settlement metadata retention), #73 (first-page high endpoint), #75 (historical warning preservation); [concurrent-page-count.md](concurrent-page-count.md), [positional-result-cache.md](positional-result-cache.md), [byte-cap-oversized-values.md](byte-cap-oversized-values.md), [scoped-select-cancellation.md](scoped-select-cancellation.md), [serialized-vertical-paging.md](serialized-vertical-paging.md), [result-history-browsing.md](result-history-browsing.md).
