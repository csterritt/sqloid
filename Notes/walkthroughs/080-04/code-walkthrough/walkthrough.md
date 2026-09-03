# Issue #080 Code Walkthrough: Active-Export/Finalization Fact Parity

*2026-09-03T17:10:45Z by Showboat 0.6.1*
<!-- showboat-id: c7ee1142-5147-4e3e-9443-fc28983d9a85 -->

Issue #80 (Notes/tasks/080-active-export-completeness-facts.md, Notes/PRD-sqloid.md Cache and snapshot invariant; user stories 55, 56, 70) unifies active export and finalization to derive equivalent endpoint/traversal facts through one shared helper. The shared deriveAuthoritativeFacts helper in internal/ui/snapshot_metadata.go derives ReachedLow from the retained low boundary and truthful eviction evidence, ReachedHigh from a successful limited-result count relative to the retained range or the accepted pageExhausted observation, and CountCacheInconsistent exactly as Issue #78 specifies (preserved without clamping). Both activeExportFacts in internal/ui/export.go and appendFinalizedResultEntry in internal/ui/active_select.go call this single helper, so identical active state produces equivalent endpoint/traversal facts and matching completeness labels before destination selection and after finalization. The terminal outcome is the only field that differs: active export carries none and finalization supplies it. This walkthrough shows the shared helper, each producer calling it, the parity-table states with matching facts and warnings, proof that active export leaves execution identity/cache/viewport/lifetime unchanged, focused passing tests, and the established verification output. Reference: Issue #80 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/080-04/code-walkthrough/.

## The shared deriveAuthoritativeFacts helper

```bash
sed -n '/\/\/ authoritativeFacts holds the endpoint/,/^}/p' internal/ui/snapshot_metadata.go
```

```output
// authoritativeFacts holds the endpoint observations and traversal facts
// derived from one shared helper for both active export and finalization
// (Issue #80). Identical active model state produces equivalent facts
// through this single seam: the retained-cache low evidence, the successful
// limited-result count or observed short/empty page high evidence, the
// count/page work state, and the count/cache contradiction without clamping.
// The terminal outcome is not derived here: active capture carries none and
// finalization supplies it separately through Finalization.
type authoritativeFacts struct {
	ReachedLow  bool
	ReachedHigh bool
	Traversal   history.TraversalFacts
}
```

```bash
sed -n '/\/\/ deriveAuthoritativeFacts derives/,/^}/p' internal/ui/snapshot_metadata.go
```

```output
// deriveAuthoritativeFacts derives the endpoint observations and traversal
// facts from the authoritative cache facts, count state, page pending state,
// and the accepted pageExhausted observation. Both active export and
// finalization call this single helper so identical active state produces
// equivalent endpoint/traversal facts and matching completeness labels.
//
// ReachedLow derives from the retained low boundary (position 1 retained) or
// truthful row-cap eviction evidence; an empty observed page (pageExhausted
// with no retained range) establishes both endpoints at position 0.
// ReachedHigh derives from a successful limited-result count relative to the
// retained range (total at or below the retained end) or the accepted
// pageExhausted observation. CountCacheInconsistent derives exactly as
// Issue #78 specifies: a successful count whose total falls below the
// retained cache end contradicts the cache, and the contradiction is
// preserved without clamping either the total or the retained range.
func deriveAuthoritativeFacts(facts history.CacheFacts, count result.CountState, pagePending, pageExhausted bool) authoritativeFacts {
	af := authoritativeFacts{
		Traversal: history.TraversalFacts{
			CountWorkFinished:      count.Status != result.CountPending,
			PageWorkFinished:       !pagePending,
			ObservedShortFinalPage: pageExhausted,
		},
	}
	if facts.HasRetainedRange {
		af.ReachedLow = facts.Start == 1 || facts.RowCapEvictions > 0
		if count.Status == result.CountSuccess && count.Total <= int64(facts.End) {
			af.ReachedHigh = true
		}
	}
	if pageExhausted {
		af.ReachedHigh = true
		if !facts.HasRetainedRange {
			af.ReachedLow = true
		}
	}
	// Issue #78: a successful limited-result count whose total falls below
	// the retained cache end contradicts the cache. The contradiction is
	// recorded without rewriting the total, retained range, endpoint
	// observations, or count state; the corrected history.Classify from
	// Issue #77 then rejects complete.
	if count.Status == result.CountSuccess && facts.HasRetainedRange && int64(facts.End) > count.Total {
		af.Traversal.CountCacheInconsistent = true
	}
	return af
}
```

## activeExportFacts calls the shared helper

```bash
sed -n '/func (m Model) activeExportFacts/,/^}/p' internal/ui/export.go
```

```output
func (m Model) activeExportFacts(page *result.Page) (history.SnapshotMetadata, history.Completeness) {
	facts := history.CacheFacts{}
	if c := m.viewportCache; c != nil {
		facts = history.FactsFromCache(c)
	}
	if m.Result.ByteTruncated {
		facts.TruncatedByByteCap = true
	}
	af := deriveAuthoritativeFacts(facts, m.countState, m.pagePending, m.pageExhausted)
	meta := history.SnapshotMetadata{
		HasRetainedRange:   facts.HasRetainedRange,
		RetainedStart:      facts.Start,
		RetainedEnd:        facts.End,
		HasKnownTotal:      m.countState.Status == result.CountSuccess,
		KnownTotal:         m.countState.Total,
		RowCapEvicted:      facts.RowCapEvictions > 0,
		RowCapEvictions:    facts.RowCapEvictions,
		TruncatedByByteCap: facts.TruncatedByByteCap,
		InvalidUTF:         page.InvalidUTF,
		ReachedLow:         af.ReachedLow,
		ReachedHigh:        af.ReachedHigh,
	}
	return meta, history.Classify(meta, af.Traversal)
}
```

## appendFinalizedResultEntry calls the shared helper

```bash
sed -n '/Issue #80: derive endpoint observations/,/^}/p' internal/ui/active_select.go
```

```output
	// Issue #80: derive endpoint observations and traversal facts through
	// the same shared helper as active export, so identical active state
	// produces equivalent facts and matching completeness labels. The
	// retained-cache low evidence, successful limited-result count or
	// observed short/empty page high evidence, count/page work state, and
	// count/cache contradiction (Issue #78, preserved without clamping) all
	// derive from this single seam.
	cacheFacts := history.CacheFacts{}
	if m.viewportCache != nil {
		cacheFacts = history.FactsFromCache(m.viewportCache)
	}
	af := deriveAuthoritativeFacts(cacheFacts, m.countState, m.pagePending, m.pageExhausted)
	// Issue #75: source invalid-UTF truth from the accepted active page so
	// it enters the immutable snapshot through Finalization. The persistent
	// byte-cap truth is already sourced from the authoritative cache via
	// FactsFromCache in SnapshotFacts, never re-derived from payload size.
	// Issue #76: source the typed limit-failure kind and one-based position
	// from the accepted active ResultView so they enter the immutable
	// snapshot as typed facts independent of the terminal outcome.
	invalidUTF := false
	if m.Result != nil && m.Result.Page != nil {
		invalidUTF = m.Result.Page.InvalidUTF
	}
	var limitKind result.LimitKind
	var limitPos int64
	if m.Result != nil && m.Result.LimitFailure != nil {
		limitKind = m.Result.LimitFailure.Kind
		limitPos = m.Result.LimitFailure.Position
	}
	final := Finalization{
		Outcome:                outcome,
		Reason:                 reason,
		ReachedLow:             af.ReachedLow,
		ReachedHigh:            af.ReachedHigh,
		InvalidUTF:             invalidUTF,
		LimitFailureKind:       limitKind,
		LimitFailurePosition:   limitPos,
		CountWorkFinished:      af.Traversal.CountWorkFinished,
		PageWorkFinished:       af.Traversal.PageWorkFinished,
		ObservedShortFinalPage: af.Traversal.ObservedShortFinalPage,
		CountCacheInconsistent: af.Traversal.CountCacheInconsistent,
	}
	meta, traversal, err := m.SnapshotFacts(final)
	if err != nil {
		// Snapshot facts come from validated model state; a construction
		// error must not create a second entry, so the execution stays
		// finalized with no entry rather than a malformed one.
		return
	}
	if !hasRows {
		kind := history.KindTabular // an observed empty result
		if outcome == history.OutcomeCancelled {
			kind = history.KindCancelled
		} else if outcome == history.OutcomeFailed {
			kind = history.KindError
		}
		m.ResultHistory.AppendFinalized(history.ResultEntry{
			ExecutionID:  m.finalizedExecID,
			Kind:         kind,
			Reason:       reason,
			Metadata:     meta,
			Completeness: history.Classify(meta, traversal),
			QueryEntryID: m.lastExecQueryEntryID,
		})
		return
	}
	m.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID:  m.finalizedExecID,
		Kind:         history.KindTabular,
		Columns:      columns,
		Rows:         rows,
		Metadata:     meta,
		Completeness: history.Classify(meta, traversal),
		QueryEntryID: m.lastExecQueryEntryID,
	})
}
```

## The parity table test

```bash
sed -n '/func TestActiveExportFinalizationFactParity/,/^}/p' internal/ui/active_export_parity_test.go
```

```output
func TestActiveExportFinalizationFactParity(t *testing.T) {
	cases := []parityState{
		{
			// Fully retained successful limited count: positions 1..10 of
			// known total 10, both endpoints reached, work finished.
			name: "fully retained successful limited count",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 10}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Count unavailable with an accepted short nonempty final page:
			// pageExhausted establishes the high endpoint; the retained
			// range establishes the low endpoint. The result is complete.
			name: "count unavailable with accepted short final page",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 3)
				m.countState = result.CountState{Status: result.CountUnavailable}
				m.pagePending = false
				m.pageExhausted = true
				installActivePage(m, 3)
			},
		},
		{
			// Count unavailable with an accepted empty final page: both
			// endpoints sit at position 0 and the result is complete.
			name: "count unavailable with accepted empty final page",
			seed: func(t *testing.T, m *Model) {
				m.countState = result.CountState{Status: result.CountUnavailable}
				m.pagePending = false
				m.pageExhausted = true
				installActivePage(m, 0)
			},
		},
		{
			// Missing low endpoint: retained range 11..20 of known total 20
			// with no low-end eviction. The low endpoint is unobserved, so
			// the snapshot is partial (Issue #77).
			name: "missing low endpoint without eviction",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 11, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 20}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Missing high endpoint: retained range 1..10 of known total 15
			// with no short-page observation. The high endpoint is
			// unobserved and rows beyond the range are known, so the
			// snapshot is partial+truncated.
			name: "missing high endpoint with known rows beyond retention",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 15}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Unfinished work: count still pending. The high endpoint is
			// unknown and work has not finished, so the snapshot is partial.
			name: "unfinished count work",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountPending}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Row-cap eviction: forward traversal evicted the low end, so
			// the retained range starts above 1 with RowCapEvicted set.
			// The snapshot is truncated (and partial unless complete).
			name: "row-cap eviction",
			seed: func(t *testing.T, m *Model) {
				seedRowCapEviction(t, m)
				m.countState = result.CountState{Status: result.CountSuccess, Total: int64(resultcache.MaxPositions + 100)}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Byte-cap eviction: retained payload exceeded MaxPayloadBytes,
			// so TruncatedByByteCap is persistent. The snapshot is truncated.
			name: "byte-cap eviction",
			seed: func(t *testing.T, m *Model) {
				seedByteCapEviction(t, m)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 3}
				m.pagePending = false
				installActivePage(m, 1)
			},
		},
		{
			// Successful count below the retained end: total 8 with retained
			// range 1..10. Issue #78: a successful limited-result count whose
			// total falls below the retained cache end contradicts the cache.
			// The contradiction is preserved without clamping either value
			// and the snapshot is partial, never complete.
			name: "successful count below retained end",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 8}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			active := buildParityModel(t, tc)
			// Snapshot the active model state before capture so finalization
			// runs on an identical copy.
			finalized := cloneParityModel(active)
			activeFacts := activeParityFacts(t, active)
			finalizedFacts := finalizedParityFacts(t, finalized)
			assertParity(t, activeFacts, finalizedFacts)

			// Export itself must not finalize or mutate the active SELECT:
			// the active model's identity, cache, viewport, and pending
			// state are unchanged after the active capture.
			if !active.SelectIsActive() {
				t.Error("active export deactivated the active SELECT lifetime")
			}
			if active.finalizedExecID != 0 {
				t.Errorf("active export finalized the active SELECT: finalized=%d", active.finalizedExecID)
			}
		})
	}
}
```

## Focused passing parity test

```bash
go test ./internal/ui/ -run '^TestActiveExportFinalizationFactParity' -count=1 -v 2>&1 | sed -E 's/\([0-9.]+s\)/(0.00s)/; s/[[:space:]]+[0-9.]+s$//'
```

```output
=== RUN   TestActiveExportFinalizationFactParity
=== RUN   TestActiveExportFinalizationFactParity/fully_retained_successful_limited_count
=== RUN   TestActiveExportFinalizationFactParity/count_unavailable_with_accepted_short_final_page
=== RUN   TestActiveExportFinalizationFactParity/count_unavailable_with_accepted_empty_final_page
=== RUN   TestActiveExportFinalizationFactParity/missing_low_endpoint_without_eviction
=== RUN   TestActiveExportFinalizationFactParity/missing_high_endpoint_with_known_rows_beyond_retention
=== RUN   TestActiveExportFinalizationFactParity/unfinished_count_work
=== RUN   TestActiveExportFinalizationFactParity/row-cap_eviction
=== RUN   TestActiveExportFinalizationFactParity/byte-cap_eviction
=== RUN   TestActiveExportFinalizationFactParity/successful_count_below_retained_end
--- PASS: TestActiveExportFinalizationFactParity (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/fully_retained_successful_limited_count (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/count_unavailable_with_accepted_short_final_page (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/count_unavailable_with_accepted_empty_final_page (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/missing_low_endpoint_without_eviction (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/missing_high_endpoint_with_known_rows_beyond_retention (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/unfinished_count_work (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/row-cap_eviction (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/byte-cap_eviction (0.00s)
    --- PASS: TestActiveExportFinalizationFactParity/successful_count_below_retained_end (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

## Established verification

```bash
go vet ./... 2>&1 && go test ./internal/ui/ ./internal/history/ -count=1 2>&1 | sed -E 's/[[:space:]]+[0-9.]+s$//' && go build ./... 2>&1 && echo 'ALL VERIFICATION PASSED'
```

```output
ok  	github.com/chris/sqloid/internal/ui
ok  	github.com/chris/sqloid/internal/history
ALL VERIFICATION PASSED
```
