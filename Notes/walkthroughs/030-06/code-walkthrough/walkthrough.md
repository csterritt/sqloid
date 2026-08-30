# Issue #30 — Contiguous 10,000-position result cache

*2026-08-28T16:59:56Z by Showboat 0.6.1*
<!-- showboat-id: 13ab82c6-da36-453a-95a8-6ddd5d59084a -->

Issue #30 implements the positional heart of the Cache and snapshot invariant of Notes/PRD-sqloid.md: internal/resultcache retains one active SELECT's rows as a single contiguous inclusive range of absolute logical positions (one-based, independent of row values and slice indexes), so duplicate-valued rows at different positions stay distinct rows. (Timing digits are stripped below for deterministic verification.)

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestMerge' | sed 's/ ([0-9.]*s)//' | grep -E 'duplicate-valued|exact_overlap_replacement'
```

```output
=== RUN   TestMerge/duplicate-valued_rows_stay_distinct_positions
=== RUN   TestMerge/exact_overlap_replacement_does_not_duplicate
    --- PASS: TestMerge/duplicate-valued_rows_stay_distinct_positions
    --- PASS: TestMerge/exact_overlap_replacement_does_not_duplicate
```

Every merge decision is classified by absolute positions only. Initial insertion accepts any nonempty page; forward-adjacent pages append; backward-adjacent pages prepend — and the cache stays ascending regardless of traversal direction.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestMerge' | sed 's/ ([0-9.]*s)//' | grep -E 'initial_insertion|forward_adjacent|backward_adjacent|alternating'
```

```output
=== RUN   TestMerge/initial_insertion_into_empty_cache_accepts_any_positions
=== RUN   TestMerge/forward_adjacent_append
=== RUN   TestMerge/backward_adjacent_prepend_keeps_ascending_order
=== RUN   TestMerge/alternating_forward_and_backward_traversal_stays_ascending
    --- PASS: TestMerge/initial_insertion_into_empty_cache_accepts_any_positions
    --- PASS: TestMerge/forward_adjacent_append
    --- PASS: TestMerge/backward_adjacent_prepend_keeps_ascending_order
    --- PASS: TestMerge/alternating_forward_and_backward_traversal_stays_ascending
```

Overlap pages replace rows at their same absolute positions — exactly, partially, spanning either retained edge, or covering the whole range — and never duplicate a position. The page's payload wins at shared positions.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestMerge' | sed 's/ ([0-9.]*s)//' | grep -E 'overlap|spanning'
```

```output
=== RUN   TestMerge/exact_overlap_replacement_does_not_duplicate
=== RUN   TestMerge/partial_overlap_replacement_at_high_edge
=== RUN   TestMerge/partial_overlap_replacement_at_low_edge
=== RUN   TestMerge/page_spanning_the_whole_retained_range_replaces_it
=== RUN   TestMerge/page_spanning_the_low_retained_edge_prepends_and_replaces
=== RUN   TestMerge/page_spanning_the_high_retained_edge_appends_and_replaces
=== RUN   TestMerge/repeated_overlap_merge_is_idempotent_on_range
    --- PASS: TestMerge/exact_overlap_replacement_does_not_duplicate
    --- PASS: TestMerge/partial_overlap_replacement_at_high_edge
    --- PASS: TestMerge/partial_overlap_replacement_at_low_edge
    --- PASS: TestMerge/page_spanning_the_whole_retained_range_replaces_it
    --- PASS: TestMerge/page_spanning_the_low_retained_edge_prepends_and_replaces
    --- PASS: TestMerge/page_spanning_the_high_retained_edge_appends_and_replaces
    --- PASS: TestMerge/repeated_overlap_merge_is_idempotent_on_range
```

A stale page whose remaining positions are nonadjacent to the retained range would create a low-side or high-side gap. Such pages are rejected atomically: rows, retained range metadata, and eviction counters are all unchanged afterwards. Empty pages and zero-value caches are likewise rejected.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestMerge|TestMergeZeroValueCache' | sed 's/ ([0-9.]*s)//' | grep -E 'stale|empty_page|PASS: TestMergeZeroValueCache'
```

```output
=== RUN   TestMerge/stale_low-side_gap_page_rejected
=== RUN   TestMerge/stale_high-side_gap_page_rejected
=== RUN   TestMerge/empty_page_rejected_without_mutating_cache
    --- PASS: TestMerge/stale_low-side_gap_page_rejected
    --- PASS: TestMerge/stale_high-side_gap_page_rejected
    --- PASS: TestMerge/empty_page_rejected_without_mutating_cache
--- PASS: TestMergeZeroValueCache
```

Beyond merging, the cache enforces the independent hard limit of MaxPositions = 10000 retained logical positions. After an accepted forward merge it evicts deterministically from the low end; after a backward merge from the high end — always by exactly the excess, so a page landing exactly at the cap evicts nothing.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestPositionCapEviction' | sed 's/ ([0-9.]*s)//' | grep -E 'landing_exactly|one_past|one_high_position'
```

```output
=== RUN   TestPositionCapEviction/forward_merge_landing_exactly_at_the_cap_evicts_nothing
=== RUN   TestPositionCapEviction/forward_append_one_past_the_cap_evicts_one_low_position
=== RUN   TestPositionCapEviction/backward_append_one_past_the_cap_evicts_one_high_position
    --- PASS: TestPositionCapEviction/forward_merge_landing_exactly_at_the_cap_evicts_nothing
    --- PASS: TestPositionCapEviction/forward_append_one_past_the_cap_evicts_one_low_position
    --- PASS: TestPositionCapEviction/backward_append_one_past_the_cap_evicts_one_high_position
```

Pages that cross the cap by several page sizes — including a single page larger than the cap itself, which retains only its last (forward) or first (backward) 10000 positions — evict exactly the excess, keeping the retained interval contiguous and bounded after every merge.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestPositionCapEviction' | sed 's/ ([0-9.]*s)//' | grep -E 'crossing|single_page'
```

```output
=== RUN   TestPositionCapEviction/forward_append_crossing_the_cap_by_several_page_sizes_evicts_exactly_the_excess
=== RUN   TestPositionCapEviction/single_page_larger_than_the_cap_retains_its_last_MaxPositions_positions_forward
=== RUN   TestPositionCapEviction/single_page_larger_than_the_cap_retains_its_first_MaxPositions_positions_backward
=== RUN   TestPositionCapEviction/backward_page_crossing_the_cap_by_several_page_sizes_evicts_exactly_the_excess
    --- PASS: TestPositionCapEviction/forward_append_crossing_the_cap_by_several_page_sizes_evicts_exactly_the_excess
    --- PASS: TestPositionCapEviction/single_page_larger_than_the_cap_retains_its_last_MaxPositions_positions_forward
    --- PASS: TestPositionCapEviction/single_page_larger_than_the_cap_retains_its_first_MaxPositions_positions_backward
    --- PASS: TestPositionCapEviction/backward_page_crossing_the_cap_by_several_page_sizes_evicts_exactly_the_excess
```

Overlap replacement composes with cap eviction: a page overlapping one retained edge and extending past the other replaces its overlap first, then the opposite end is trimmed — so values at retained overlapping positions ("r<N>" payloads) are unaffected while evicted positions disappear from the far end. Alternating direction after prior eviction evicts the other end on the next arrival, and stale gap pages are still rejected atomically after eviction.

```bash
go test ./internal/resultcache/ -v -count=1 -run 'TestPositionCapEviction' | sed 's/ ([0-9.]*s)//' | grep -E 'overlap|spanning|alternating|stale'
```

```output
=== RUN   TestPositionCapEviction/forward_overlap_near_the_low_edge_replaces_then_evicts_the_low_end
=== RUN   TestPositionCapEviction/backward_overlap_near_the_high_edge_replaces_then_evicts_the_high_end
=== RUN   TestPositionCapEviction/page_spanning_the_retained_low_edge_under_cap_pressure_replaces_the_overlap
=== RUN   TestPositionCapEviction/alternating_direction_after_prior_eviction_evicts_the_other_end
=== RUN   TestPositionCapEviction/alternating_direction_again_evicts_the_low_end_once_more
=== RUN   TestPositionCapEviction/stale_gap_page_after_eviction_rejected_atomically
    --- PASS: TestPositionCapEviction/forward_overlap_near_the_low_edge_replaces_then_evicts_the_low_end
    --- PASS: TestPositionCapEviction/backward_overlap_near_the_high_edge_replaces_then_evicts_the_high_end
    --- PASS: TestPositionCapEviction/page_spanning_the_retained_low_edge_under_cap_pressure_replaces_the_overlap
    --- PASS: TestPositionCapEviction/alternating_direction_after_prior_eviction_evicts_the_other_end
    --- PASS: TestPositionCapEviction/alternating_direction_again_evicts_the_low_end_once_more
    --- PASS: TestPositionCapEviction/stale_gap_page_after_eviction_rejected_atomically
```

Finally, the whole package's invariants — one contiguous ascending gap-free interval, at most MaxPositions positions, exact start/end metadata, no duplicates — are asserted after every single merge in the suite, and the repository verification (tests, vet, gofmt) stays green.

```bash
go test ./internal/resultcache/ -count=1 >/dev/null && go vet ./internal/resultcache/ && test -z "$(gofmt -l internal/resultcache/)" && echo 'tests, vet, gofmt: all clean'
```

```output
tests, vet, gofmt: all clean
```
