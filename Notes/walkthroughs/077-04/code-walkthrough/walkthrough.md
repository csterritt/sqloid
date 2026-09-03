# Issue #077 Code Walkthrough: Distinguish Unseen Low Endpoints from Truncation

*2026-09-03T13:07:32Z by Showboat 0.6.1*
<!-- showboat-id: a3494cb9-7c76-435b-b704-2fd7ce30a7b0 -->

Issue #77 (Notes/tasks/077-snapshot-completeness-low-endpoint.md, Notes/PRD-sqloid.md §Cache and snapshot invariant; user stories 55, 56) distinguishes an unseen low endpoint from truncation in `history.Classify`. A nonempty snapshot with `ReachedLow == false` means lower rows may be unseen and is therefore `partial`, not `truncated` by itself. An empty logical result (`high == 0`) completes without `ReachedLow` because there is no low row to observe. `truncated` remains reserved for row-cap or byte-cap eviction and known/observed rows outside the retained range. When a nonempty missing low endpoint coexists with independent truncation evidence, both `partial` and `truncated` are truthful and coexist. This walkthrough exercises the classifier with retained positions 11–20 of known total 20 and no low endpoint (partial, not truncated), known-total and observed-short empty results completing with `ReachedLow=false`, actual low/high eviction, known overflow, unknown work, and mixed missing-endpoint plus eviction evidence (partial+truncated). All artifacts are under Notes/walkthroughs/077-04/code-walkthrough/.

## The low-endpoint condition in Classify

The `complete` condition in `internal/history/snapshot_classify.go` changed from `meta.ReachedLow` to `(high == 0 || meta.ReachedLow)`: an empty logical result is vacuously complete without a low row to observe, while a nonempty result still requires `ReachedLow`. The `partial` condition gained `(high != 0 && !meta.ReachedLow)` so a nonempty result with an unseen low endpoint classifies partial rather than falling through to the default truncated branch. `truncated` remains limited to eviction or known/observed rows beyond the retained range — a missing low endpoint alone is never truncation.

```bash
sed -n '/Issue #77: an empty logical result/,/truncated := evicted || rowsBeyondRange/p' internal/history/snapshot_classify.go
```

```output
	// Issue #77: an empty logical result (high == 0) has no low row to
	// observe, so ReachedLow is not required; a nonempty result requires
	// ReachedLow so that unseen lower rows do not falsely complete.
	complete := !evicted && !inconsistent && highKnown && workFinished &&
		(high == 0 || meta.ReachedLow) && fullRetention

	// Partial: unseen limited-result rows may remain (unknown remainder, an
	// unobserved high endpoint with rows beyond the range, an unobserved low
	// endpoint on a nonempty result where lower rows may be unseen) or
	// count/page work did not finish, or count/cache evidence is
	// contradictory and cannot be trusted to be complete.
	partial := !complete && (!highKnown || !workFinished || inconsistent ||
		(!meta.ReachedHigh && rowsBeyondRange) || (high != 0 && !meta.ReachedLow))

	// Truncated: known or observed rows were evicted or lie beyond the
	// retained range. A missing low endpoint alone is never truncation.
	truncated := evicted || rowsBeyondRange
```

## Truth table: Issue #77 classification matrix cases

The extended classification matrix in `internal/history/snapshot_classify_test.go` covers the Issue #77 cases. The focused test run below shows every case name and its label result, exercising: a settled nonempty range 11–20 with unseen low endpoint and no eviction (partial, not truncated); empty known-total and observed-short-empty results with `ReachedLow=false` (complete); actual low-side and high-side row-cap eviction; byte-cap eviction; unseen low endpoint plus row-cap eviction, byte-cap eviction, and known rows beyond range (partial+truncated); unknown count with and without the low endpoint; and a complete control with both endpoints and full retention.

```bash
go test ./internal/history/ -run '^TestClassificationMatrix$' -v -count=1 2>&1
```

```output
=== RUN   TestClassificationMatrix
=== RUN   TestClassificationMatrix/complete:_known_total,_full_retention
=== RUN   TestClassificationMatrix/complete:_limit_at_observation
=== RUN   TestClassificationMatrix/complete:_limit_above_observations,_rows_beyond_Limit_irrelevant
=== RUN   TestClassificationMatrix/complete:_empty_result_via_count_success
=== RUN   TestClassificationMatrix/complete:_count_failed_but_observed_empty_final_page_establishes_high
=== RUN   TestClassificationMatrix/complete:_count_failed_but_observed_short_final_page_establishes_high
=== RUN   TestClassificationMatrix/complete:_empty_known-total_result,_ReachedLow_false,_vacuous_retention
=== RUN   TestClassificationMatrix/complete:_observed_empty_final_page,_ReachedLow_false,_vacuous_retention
=== RUN   TestClassificationMatrix/partial_and_truncated:_low-side_row-cap_eviction_with_unseen_low_endpoint
=== RUN   TestClassificationMatrix/truncated_only:_rows_beyond_retained_range,_high_endpoint_reached
=== RUN   TestClassificationMatrix/truncated_only:_byte-cap_eviction_fact_persists
=== RUN   TestClassificationMatrix/partial_only:_full_pages,_unknown_remainder,_no_count
=== RUN   TestClassificationMatrix/partial_only:_settled_nonempty_range,_unseen_low_endpoint,_no_eviction
=== RUN   TestClassificationMatrix/partial_only:_count_failure,_no_short_or_empty_observation
=== RUN   TestClassificationMatrix/partial_only:_count_success_but_paging_unfinished
=== RUN   TestClassificationMatrix/partial_only:_count_never_finished
=== RUN   TestClassificationMatrix/partial_only:_count/cache_inconsistency_below_retained_range
=== RUN   TestClassificationMatrix/partial_and_truncated:_count_failure_plus_byte-cap_eviction
=== RUN   TestClassificationMatrix/partial_and_truncated:_unknown_remainder_plus_row-cap_eviction
=== RUN   TestClassificationMatrix/partial_and_truncated:_rows_beyond_range_unseen_after_eviction
=== RUN   TestClassificationMatrix/partial_and_truncated:_inconsistent_count_above_retained_range
=== RUN   TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_row-cap_eviction
=== RUN   TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_byte-cap_eviction
=== RUN   TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_known_rows_beyond_range
--- PASS: TestClassificationMatrix (0.00s)
    --- PASS: TestClassificationMatrix/complete:_known_total,_full_retention (0.00s)
    --- PASS: TestClassificationMatrix/complete:_limit_at_observation (0.00s)
    --- PASS: TestClassificationMatrix/complete:_limit_above_observations,_rows_beyond_Limit_irrelevant (0.00s)
    --- PASS: TestClassificationMatrix/complete:_empty_result_via_count_success (0.00s)
    --- PASS: TestClassificationMatrix/complete:_count_failed_but_observed_empty_final_page_establishes_high (0.00s)
    --- PASS: TestClassificationMatrix/complete:_count_failed_but_observed_short_final_page_establishes_high (0.00s)
    --- PASS: TestClassificationMatrix/complete:_empty_known-total_result,_ReachedLow_false,_vacuous_retention (0.00s)
    --- PASS: TestClassificationMatrix/complete:_observed_empty_final_page,_ReachedLow_false,_vacuous_retention (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_low-side_row-cap_eviction_with_unseen_low_endpoint (0.00s)
    --- PASS: TestClassificationMatrix/truncated_only:_rows_beyond_retained_range,_high_endpoint_reached (0.00s)
    --- PASS: TestClassificationMatrix/truncated_only:_byte-cap_eviction_fact_persists (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_full_pages,_unknown_remainder,_no_count (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_settled_nonempty_range,_unseen_low_endpoint,_no_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_count_failure,_no_short_or_empty_observation (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_count_success_but_paging_unfinished (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_count_never_finished (0.00s)
    --- PASS: TestClassificationMatrix/partial_only:_count/cache_inconsistency_below_retained_range (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_count_failure_plus_byte-cap_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_unknown_remainder_plus_row-cap_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_rows_beyond_range_unseen_after_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_inconsistent_count_above_retained_range (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_row-cap_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_byte-cap_eviction (0.00s)
    --- PASS: TestClassificationMatrix/partial_and_truncated:_unseen_low_endpoint_plus_known_rows_beyond_range (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/history	0.002s
```

The truth table confirms all four Issue #77 acceptance criteria:

1. **AC1**: `partial_and_truncated:_low-side_row-cap_eviction_with_unseen_low_endpoint` and `partial_only:_settled_nonempty_range,_unseen_low_endpoint,_no_eviction` — a nonempty settled snapshot that reached the high endpoint but never observed the low endpoint and has no eviction/known overflow is labeled `partial` and not `truncated`.
2. **AC2**: `complete:_empty_known-total_result,_ReachedLow_false,_vacuous_retention` and `complete:_observed_empty_final_page,_ReachedLow_false,_vacuous_retention` — finished empty logical results with full retention and no contradictory evidence are labeled exclusively `complete` even when `ReachedLow` is false.
3. **AC3**: `partial_and_truncated:_unseen_low_endpoint_plus_row-cap_eviction`, `partial_and_truncated:_unseen_low_endpoint_plus_byte-cap_eviction`, and `partial_and_truncated:_unseen_low_endpoint_plus_known_rows_beyond_range` — rows evicted by row-cap or byte-cap, or known/observed outside the retained range, keep `truncated` true and coexist with `partial` when truthful.
4. **AC4**: `complete:_known_total,_full_retention` and the other complete cases — a nonempty complete result requires both endpoints established plus the full limited logical range retained.

## Full verification

All `internal/history` tests, dependent `internal/ui` snapshot/export tests, `go vet`, and `go build` pass.

```bash
go test ./internal/history/ ./internal/ui/ -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/history	0.095s
ok  	github.com/chris/sqloid/internal/ui	0.474s
```

```bash
go vet ./... 2>&1 && go build ./... 2>&1 && echo 'vet and build clean'
```

```output
vet and build clean
```

## References

- Issue #77: `Notes/issues/077-snapshot-completeness-low-endpoint.md` — distinguish unseen low endpoints from truncation
- PRD: `Notes/PRD-sqloid.md` §Cache and snapshot invariant — completeness labels, endpoint rules, and the limited logical result
- User stories 55 (truthful completeness metadata) and 56 (unseen remainder stays partial unless an endpoint is established)
- Implementation: `internal/history/snapshot_classify.go` — the `Classify` function with the Issue #77 low-endpoint condition
- Tests: `internal/history/snapshot_classify_test.go` — the extended classification matrix
- Wiki: `Notes/wiki/snapshot-metadata.md` — updated with the Issue #77 low-endpoint distinction
