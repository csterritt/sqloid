# Issue #078 Code Walkthrough: Finalized Count/Cache Contradiction Propagation

*2026-09-03T14:38:14Z by Showboat 0.6.1*
<!-- showboat-id: ecb9e5f6-db5b-4c0f-a888-f81ac784ab92 -->

Issue #78 (Notes/tasks/078-finalized-count-cache-inconsistency.md, Notes/PRD-sqloid.md §Cache and snapshot invariant; user stories 55, 59, 61) records the count/cache contradiction at finalization. Count and cache are independent autocommit reads with no shared snapshot, so a successful limited-result count whose total falls below the authoritative retained cache end contradicts the cache: rows the count says do not exist are nonetheless retained. `appendFinalizedResultEntry` in `internal/ui/active_select.go` derives `CountCacheInconsistent` from the same authoritative cache snapshot used for rows and range, before `SnapshotFacts`, and passes it through `Finalization.CountCacheInconsistent` into `TraversalFacts.CountCacheInconsistent`. Both facts — the successful known total and the retained range — are preserved without clamping either, and the corrected `history.Classify` from Issue #77 rejects `complete` whenever the flag is set. This walkthrough finalizes an active result whose successful count total is below the retained cache end, inspects the propagated `CountCacheInconsistent`, the original known total and range, the stored history/export metadata, and the non-complete classification; contrasts equal and lower retained-end boundaries plus pending, unavailable, failed, and cancelled count states to show the flag is not invented; and includes exactly-once/immutability evidence and the focused passing tests. All artifacts are under Notes/walkthroughs/078-04/code-walkthrough/.

## The contradiction derivation in appendFinalizedResultEntry

The derivation lives in `appendFinalizedResultEntry` immediately before the `Finalization` struct is built. It reads the same authoritative cache snapshot used for rows and range, so the contradiction cannot drift from the retained facts it contradicts. The flag is set when and only when the count succeeded, a retained end exists, and that end exceeds `countState.Total`. The total, retained range, endpoint observations, rows, and count state are never rewritten.

```bash
sed -n '/Issue #78: derive count\/cache inconsistency/,/invalidUTF := false/p' internal/ui/active_select.go
```

```output
	// Issue #78: derive count/cache inconsistency from the same authoritative
	// cache snapshot used for rows and range, before SnapshotFacts. Count and
	// cache are independent autocommit facts: a successful limited-result
	// count whose total falls below the retained cache end contradicts the
	// cache. The contradiction is recorded without rewriting the total,
	// retained range, endpoint observations, rows, or count state; the
	// corrected history.Classify from Issue #77 then rejects complete.
	countCacheInconsistent := false
	if m.countState.Status == result.CountSuccess && m.viewportCache != nil {
		if end, ok := m.viewportCache.End(); ok && int64(end) > m.countState.Total {
			countCacheInconsistent = true
		}
	}
	// Issue #75: source invalid-UTF truth from the accepted active page so
	// it enters the immutable snapshot through Finalization. The persistent
	// byte-cap truth is already sourced from the authoritative cache via
	// FactsFromCache in SnapshotFacts, never re-derived from payload size.
	// Issue #76: source the typed limit-failure kind and one-based position
	// from the accepted active ResultView so they enter the immutable
	// snapshot as typed facts independent of the terminal outcome.
	invalidUTF := false
```

```bash
sed -n '/CountWorkFinished:.*m.countState.Status != result.CountPending/,/ObservedShortFinalPage: m.pageExhausted,/p' internal/ui/active_select.go
```

```output
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
```

```bash
grep -n 'CountCacheInconsistent' internal/ui/active_select.go
```

```output
203:		CountCacheInconsistent: countCacheInconsistent,
```

## The flag passes through Finalization into TraversalFacts

`SnapshotFacts` in `internal/ui/snapshot_metadata.go` passes `f.CountCacheInconsistent` straight through into `history.TraversalFacts.CountCacheInconsistent`. The known total and retained range come from the settled count state and the authoritative cache respectively; the contradiction flag is the only finalization-derived traversal fact. `history.Classify` consumes it and rejects `complete` whenever it is set.

```bash
sed -n '/traversal := history.TraversalFacts{/,/return meta, traversal, nil/p' internal/ui/snapshot_metadata.go
```

```output
	traversal := history.TraversalFacts{
		HasLimit:               m.countState.HasLimit,
		Limit:                  m.countState.Limit,
		CountWorkFinished:      countWorkFinished,
		PageWorkFinished:       f.PageWorkFinished,
		ObservedShortFinalPage: f.ObservedShortFinalPage,
		CountCacheInconsistent: f.CountCacheInconsistent,
	}
	return meta, traversal, nil
```

```bash
sed -n '/inconsistent := t.CountCacheInconsistent/,/truncated := evicted || rowsBeyondRange/p' internal/history/snapshot_classify.go
```

```output
	inconsistent := t.CountCacheInconsistent

	// Empty logical results (high endpoint 0) are fully retained vacuously;
	// otherwise every limited row must sit inside the retained range with
	// its low end at position 1.
	fullRetention := high == 0 ||
		(meta.HasRetainedRange && meta.RetainedStart == 1 && int64(meta.RetainedEnd) >= high)

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

## Focused tests: the contradiction and its controls

The focused tests in `internal/ui/snapshot_finalize_inconsistency_test.go` build active caches whose retained end is greater than, equal to, and less than a successful `result.CountState.Total`, finalize through the production seam (`enterResultHistory` → `finalizeActiveSelect` → `appendFinalizedResultEntry`), and inspect the stored `history.ResultEntry`, `SnapshotMetadata`, `TraversalFacts`-driven completeness, and export selection. Only the greater-than case propagates `CountCacheInconsistent` (observable as a partial, never complete, classification); both the known total and the retained range are preserved without clamping. Equal and lower retained-end boundaries, pending/unavailable/failed/cancelled counts, and an empty cache with a successful count never set the flag. The exact boundary finalizes complete, exactly-once, and immutably.

```bash
go test ./internal/ui/ -run 'TestFinalizationDerivesCountCacheInconsistent|TestFinalizationNoFlagForNonSuccessCounts|TestFinalizationEmptyCachePreservesSuccessfulCount|TestFinalizationExactBoundaryNoContradiction' -v -count=1 2>&1
```

```output
=== RUN   TestFinalizationDerivesCountCacheInconsistent
=== RUN   TestFinalizationDerivesCountCacheInconsistent/retained_end_greater_than_successful_count_total
=== RUN   TestFinalizationDerivesCountCacheInconsistent/retained_end_equal_to_successful_count_total
=== RUN   TestFinalizationDerivesCountCacheInconsistent/retained_end_less_than_successful_count_total
--- PASS: TestFinalizationDerivesCountCacheInconsistent (0.00s)
    --- PASS: TestFinalizationDerivesCountCacheInconsistent/retained_end_greater_than_successful_count_total (0.00s)
    --- PASS: TestFinalizationDerivesCountCacheInconsistent/retained_end_equal_to_successful_count_total (0.00s)
    --- PASS: TestFinalizationDerivesCountCacheInconsistent/retained_end_less_than_successful_count_total (0.00s)
=== RUN   TestFinalizationNoFlagForNonSuccessCounts
=== RUN   TestFinalizationNoFlagForNonSuccessCounts/pending_count
=== RUN   TestFinalizationNoFlagForNonSuccessCounts/unavailable_count
=== RUN   TestFinalizationNoFlagForNonSuccessCounts/failed_outcome_with_unavailable_count
=== RUN   TestFinalizationNoFlagForNonSuccessCounts/cancelled_outcome_with_pending_count
--- PASS: TestFinalizationNoFlagForNonSuccessCounts (0.00s)
    --- PASS: TestFinalizationNoFlagForNonSuccessCounts/pending_count (0.00s)
    --- PASS: TestFinalizationNoFlagForNonSuccessCounts/unavailable_count (0.00s)
    --- PASS: TestFinalizationNoFlagForNonSuccessCounts/failed_outcome_with_unavailable_count (0.00s)
    --- PASS: TestFinalizationNoFlagForNonSuccessCounts/cancelled_outcome_with_pending_count (0.00s)
=== RUN   TestFinalizationEmptyCachePreservesSuccessfulCount
--- PASS: TestFinalizationEmptyCachePreservesSuccessfulCount (0.00s)
=== RUN   TestFinalizationExactBoundaryNoContradiction
--- PASS: TestFinalizationExactBoundaryNoContradiction (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.011s
```

## Exactly-once and immutability evidence

`TestFinalizationExactBoundaryNoContradiction` proves the exact boundary (retained end equal to total) finalizes complete, then re-invokes `enterResultHistory` and merges more rows into the cache. The repeated finalizer creates no second entry and never mutates the first; later cache activity cannot reach the finalized snapshot. This is the same exactly-once/immutability contract the Issue #34 finalization seam guarantees for every execution, now exercised on the count/cache contradiction boundary.

```bash
sed -n '/TestFinalizationExactBoundaryNoContradiction/,/^}/p' internal/ui/snapshot_finalize_inconsistency_test.go
```

```output
// TestFinalizationExactBoundaryNoContradiction proves the exact boundary —
// retained end equal to the successful total — never sets the contradiction
// flag and finalizes immutable, exactly-once.
func TestFinalizationExactBoundaryNoContradiction(t *testing.T) {
	m := finalizeContradictionModel(t, result.CountState{Status: result.CountSuccess, Total: 10}, func(t *testing.T, m *Model) {
		mergeRowsIntoCache(t, m, 1, 10)
	})
	entry, _ := finalizeAndInspect(t, m)
	if !entry.Completeness.Complete {
		t.Errorf("exact boundary classified %v, want complete (no contradiction)", entry.Completeness)
	}

	// Finalization is exactly once: a repeated finalizer creates no second
	// entry and never mutates the first.
	m.enterResultHistory()
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("repeated finalizer created %d entries", m.ResultHistory.Len())
	}
	again := m.ResultHistory.Entries()[0]
	if again.ID != entry.ID || again.Completeness != entry.Completeness || again.Metadata.KnownTotal != entry.Metadata.KnownTotal {
		t.Fatal("repeated finalizer mutated the finalized entry")
	}

	// Finalization is immutable: later cache activity cannot reach the
	// finalized snapshot.
	mergeRowsIntoCache(t, &m, 11, 10)
	after := m.ResultHistory.Entries()[0]
	if after.Metadata.RetainedEnd != entry.Metadata.RetainedEnd || after.Metadata.KnownTotal != entry.Metadata.KnownTotal {
		t.Fatal("later cache activity mutated the finalized entry")
	}
}
```

## Full verification

The established Go verification suite passes: `gofmt`, `go vet ./...`, `go test ./...`, and `go build ./...`. The contradiction derivation adds no new packages, dependencies, or presentation text; it reuses the existing `Finalization.CountCacheInconsistent` and `TraversalFacts.CountCacheInconsistent` fields and the corrected `history.Classify` from Issue #77.

```bash
gofmt -l internal/ui/active_select.go internal/ui/snapshot_finalize_inconsistency_test.go && go vet ./... && go test -count=1 ./... 2>&1 | grep -E '^(ok|FAIL|---)' | sed 's/[0-9.]*s$//' | sort && echo '---BUILD---' && go build ./... && echo OK
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	
ok  	github.com/chris/sqloid/internal/cli	
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/d1	
ok  	github.com/chris/sqloid/internal/export	
ok  	github.com/chris/sqloid/internal/filepicker	
ok  	github.com/chris/sqloid/internal/history	
ok  	github.com/chris/sqloid/internal/querybuilder	
ok  	github.com/chris/sqloid/internal/result	
ok  	github.com/chris/sqloid/internal/resultcache	
ok  	github.com/chris/sqloid/internal/schema	
ok  	github.com/chris/sqloid/internal/session	
ok  	github.com/chris/sqloid/internal/ui	
---BUILD---
OK
```
