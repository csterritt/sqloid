# Issue #30 — Contiguous 10,000-position result cache

Issue #30 implements the positional heart of the Cache and snapshot invariant of `Notes/PRD-sqloid.md`: `internal/resultcache` holds one active SELECT's fetched rows as a **single contiguous inclusive range of absolute logical positions**, merging fetched pages in place, replacing overlap at matching positions, rejecting stale nonadjacent pages atomically, and evicting from the deterministic opposite end of traversal when the independent hard limit of **10,000 retained logical positions** is exceeded. The package is UI-independent: it imports only `internal/result` typed values and never Bubble Tea or `internal/connection` concerns. Cross-references: [serialized-vertical-paging.md](serialized-vertical-paging.md) (the page requests whose ranges this cache retains), [select-request-identities.md](select-request-identities.md) (which settled responses may reach a merge at all), [concurrent-page-count.md](concurrent-page-count.md), and [whole-column-horizontal-scrolling.md](whole-column-horizontal-scrolling.md).

## Positions, pages, and the cache

- **`Position` is an absolute logical result row position** (one-based: position 1 is the first row of the limited logical result). Positions are independent of row values and of any slice index, so **duplicate-valued rows remain distinct positions** — two rows carrying the same TEXT payload at positions 1 and 3 never collapse.
- **`Row` carries `Position` plus `[]result.Value`**; the cache copies payloads at acceptance (`copyValues`) so later caller mutation can never corrupt retained rows. **`Page` is a fetch-side unit occupying consecutive positions starting at `Start`** with `End()` = `Start + len(Rows) - 1`.
- **`Cache` is the single contiguous range.** Rows are always retained in **ascending absolute-position order regardless of the traversal direction that fetched them** (a backward prepend inserts before, never in front of the slice unsorted). Metadata for `internal/ui` and later immutable snapshots: `Start()`/`End()` (inclusive, with an `ok` false on empty), `Len()`, `RowCapEvictions()` (cumulative positions evicted by the cap), and `Rows()` returning a fresh ascending copy. The zero-value `Cache` rejects merges; construct with `New()`.

## Merge semantics: adjacency, overlap replacement, atomic rejection

`Cache.Merge(page, dir)` classifies the page **after accounting for overlap** — the only geometric rule is contiguity of the union:

- **Empty cache** accepts any nonempty page (the initial insertion); **empty pages are rejected** as no-ops. A zero-value cache rejects everything.
- **Accepted iff the union stays contiguous**: the page is rejected exactly when `page.End() < retainedStart-1` or `page.Start > retainedEnd+1`. Overlapping positions contribute no new positions, so a page fully inside or covering the retained range merges without eviction pressure.
- **Overlap replacement, never duplication.** The merge is a two-way sorted merge of the retained rows and the ascending page rows; positions present on both sides keep the **page's** payload and drop the retained copy, so the same position is never stored twice.
- **Adjacent append/prepend.** A page whose remaining positions start at `end+1` (forward) or end at `start-1` (backward) appends/prepends; alternating forward/backward traversal keeps one ascending contiguous interval throughout.
- **Atomic gap rejection.** A stale page that would create a **low-side gap** (ends before `start-1`), a **high-side gap** (starts after `end+1`), or any interior island leaves rows, range metadata, and eviction counters completely unchanged.

## The independent 10,000-position cap

`MaxPositions = 10000` is enforced immediately after an accepted merge, driven only by the incoming traversal direction:

- **Forward (Page Down direction) evicts from the low end**; **backward evicts from the high end** — the "standard opposite end" of the PRD invariant — by exactly the excess count, so eviction is deterministic and minimal. A merge landing exactly at the cap evicts nothing; a single page larger than the cap retains its last (forward) or first (backward) 10,000 positions.
- Overlap replacement interacts cleanly with eviction: a page overlapping one edge and extending past the other replaces its overlap first, then cap eviction trims the opposite end, so **values at retained overlaps are unaffected** while evicted positions disappear from the low (forward) or high (backward) end.
- Alternating direction after prior eviction evicts the other end on the next arrival; `RowCapEvictions()` accumulates every cap-driven eviction for later completeness metadata (`truncated`), while gap rejections never touch it.
- The byte cap (64 MiB payload accounting) arrived with Issue #31 — see [byte-cap-oversized-values.md](byte-cap-oversized-values.md) for byte eviction, the persistent `truncated-by-byte-cap` disclosure, and the oversized page/value failures.

## Testing

`internal/resultcache/cache_test.go` and `internal/resultcache/cap_test.go` are pure, table-driven, and isolate merge decisions from UI navigation and database fetching (no Bubble Tea, no driver, no clocks):

- Positional-identity coverage: initial insertion, duplicate-valued rows at distinct positions, forward-adjacent append, backward-adjacent prepend keeping ascending order, exact and partial overlap replacement at each edge, pages spanning the whole range or either retained edge, repeated overlap, and alternating traversal.
- Gap rejection: stale low-side and high-side pages rejected with exact retained range/values/counters asserted afterward, plus an empty page and the zero-value cache.
- Cap coverage: forward and backward traversal beyond 10,000 positions in single-position, one-past, multi-page, and oversized-page shapes; landing exactly at the cap; overlap replacement near each edge and spanning an edge under cap pressure; alternating direction after prior eviction; and a stale gap page after eviction rejected atomically.
- Shared helpers assert after every merge: exact retained start/end metadata, one row per position, ascending gap-free positions bounded by `MaxPositions`, no duplicate positions, and (for overlap cases) the exact replaced payload at retained overlapping positions.
