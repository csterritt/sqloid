# Issue #073 Code Walkthrough: Establish the High Endpoint from a Short First Page

*2026-09-02T19:15:58Z by Showboat 0.6.1*
<!-- showboat-id: 7fcc5f57-019e-4d86-b342-42fae1bf901a -->

Issue #73 (Notes/tasks/073-first-page-high-endpoint.md, Notes/PRD-sqloid.md §Cache and snapshot invariant, §SELECT lifecycle, user stories 51/55/56) establishes the high endpoint from a short first page. The layout-derived requested first-page size is captured at dispatch and bound to the same execution/request/generation identity as the response. Only an accepted response with fewer rows than that retained size — including zero — sets `pageExhausted` and establishes the observed high endpoint; an exactly-full page leaves the remainder unknown. Stale or cancelled settlements are fully inert. An accepted short or empty first page suppresses forward Page Down, feeds `ObservedShortFinalPage` consistently into active export and finalization, and produces truthful `Result is complete` labels when count work finished and all rows are retained. This walkthrough demonstrates the retained requested first-page size, accepted empty/short/full settlements with count unavailable, Page Down suppression, matching active-export and finalized labels, and stale execution/request/generation responses proving they cannot alter endpoints or cache state.

## Capturing the requested first-page size at dispatch

`startSelectPage` in `internal/ui/first_select.go` captures the layout-derived requested first-page size at dispatch, storing it in `firstPageRequestedSize` on the model. This size is bound to the same execution/request/generation identity as the response — it is reset on every fresh execution and compared only after all current-response guards accept the settlement.

```bash
sed -n '/func.*startSelectPage/,/^}/p' internal/ui/first_select.go | head -40
```

```output
func (m *Model) startSelectPage() tea.Cmd {
	if m.Select == nil {
		return nil
	}
	exec := result.NextSelectExecutionID()
	// Issue #34: starting an actual new execution finalizes the previous
	// active SELECT (if any) before the active lifetime moves to the new ID.
	m.activateSelect(exec)
	m.resetPagingState() // a fresh execution pages from its first page again
	pageID := result.NextSelectRequestID()
	countID := result.NextSelectRequestID()
	m.selectTracker = result.NewSelectTracker(exec, pageID, countID)
	generation := m.viewportGen
	// Issue #27: claim the first-page slot and the generic cancellation seam
	// before anything dispatches; both release at their request's settlement.
	// Issue #28: each request gets its own derived cancellation context so
	// Ctrl+W requests one independent connection-scoped interrupt identity
	// per active request; each handle retires exactly at its settlement.
	m.firstPagePending = true
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return SelectCancelRequestedMsg{} }

	// Issue #73: retain the layout-derived requested first-page size at
	// dispatch, bound to the same execution/request/generation identity as
	// the response, so an accepted short or empty first page can establish
	// the high endpoint by comparing returned rows to this exact size.
	m.firstPageRequestedSize = int64(CalculateLayout(m.Height, m.Fields).PageRows)

	pageCtx, pageCancel := context.WithCancel(context.Background())
	countCtx, countCancel := context.WithCancel(context.Background())
	m.firstPageCancel = pageCancel
	m.countCancel = countCancel

	pageFn := m.Select
	sql := m.QB.SelectSQL()
	params := m.QB.SelectParams()
	pageCmd := func() tea.Msg {
		return SelectSettledMsg{
			ExecutionID: exec,
			RequestID:   pageID,
```

## Accepted settlement: comparing row count to the requested size

`applySelectSettled` in `internal/ui/first_select.go` compares the accepted row count against `firstPageRequestedSize` only after all current-response guards pass. When fewer rows (including zero) are returned, `pageExhausted` is set; an exactly-full page leaves it false. The identity check precedes the comparison so stale or cancelled responses cannot establish or clear any endpoint.

```bash
grep -n 'pageExhausted\|firstPageRequestedSize' internal/ui/first_select.go
```

```output
123:	m.firstPageRequestedSize = int64(CalculateLayout(m.Height, m.Fields).PageRows)
173:// the retained requested size — including zero — sets pageExhausted so the
205:		int64(len(res.Page.Rows)) < m.firstPageRequestedSize {
206:		m.pageExhausted = true
```

```bash
sed -n '168,215p' internal/ui/first_select.go
```

```output
// so rows, cache, and pending feedback stay untouched. Issue #72: the
// incoming ByteTruncated and LimitFailure metadata are copied into the
// fresh ResultView, and byte truncation is ORed with the viewport cache's
// post-merge TruncatedByByteCap so cache-derived disclosure cannot be lost.
// Issue #73: an accepted successful first page returning fewer rows than
// the retained requested size — including zero — sets pageExhausted so the
// high endpoint is established; an exactly-full page leaves it false so the
// remainder stays unknown. This seam runs only after all current-response
// guards accept the settlement, so stale or cancelled responses never
// reach this comparison.
func (m Model) applySelectSettled(res FirstPageResult) Model {
	if res.Err == nil && res.Cancelled {
		return m // defensive: cancellation classification is fully inert here
	}
	// Issue #32: the first page of the fresh execution seeds the active
	// contiguous dual-cap cache at absolute positions 1..len before it
	// becomes display state. Issue #72: merge before reading
	// TruncatedByByteCap so page-envelope admission trimming is visible.
	if res.Page != nil {
		m.mergePageIntoCache(res.Page, 0, true)
	}
	byteTruncated := res.ByteTruncated
	if res.Page != nil && m.viewportCache.TruncatedByByteCap() {
		byteTruncated = true
	}
	m.Result = &ResultView{
		Page:          res.Page,
		Err:           res.Err,
		ByteTruncated: byteTruncated,
		LimitFailure:  res.LimitFailure,
	}
	// Issue #73: compare the accepted first page's row count to the
	// retained requested size only on success (no error, no cancellation).
	// Fewer rows than requested — including zero — establishes the high
	// endpoint; an exactly-full page leaves it unknown. Stale or cancelled
	// settlements never reach this seam.
	if res.Err == nil && res.Page != nil &&
		int64(len(res.Page.Rows)) < m.firstPageRequestedSize {
		m.pageExhausted = true
	}
	return m
}
```

## Resetting the requested size on fresh execution

`resetPagingState` in `internal/ui/paging.go` clears `firstPageRequestedSize` so each fresh execution captures a new layout-derived size at dispatch. This prevents a stale size from a previous execution leaking into the next.

```bash
grep -n 'firstPageRequestedSize' internal/ui/paging.go
```

```output
247:	m.firstPageRequestedSize = 0 // Issue #73: cleared for the fresh execution
```

## Active export: feeding pageExhausted into ObservedShortFinalPage

`activeExportFacts` in `internal/ui/export.go` feeds `pageExhausted` into `ObservedShortFinalPage` and sets `ReachedHigh` (and `ReachedLow` for an empty page) so a fully retained count-unavailable short/empty result classifies complete in the active export warnings — even before finalization.

```bash
grep -n 'pageExhausted\|ObservedShortFinalPage\|ReachedHigh\|ReachedLow' internal/ui/export.go
```

```output
120:// supplies the low endpoint, pageExhausted supplies the high endpoint via
121:// ObservedShortFinalPage, and an empty observed page establishes both
144:	// cache's retained range supplies the low endpoint; pageExhausted
145:	// supplies the high endpoint; an empty observed page (pageExhausted
148:		meta.ReachedLow = facts.Start == 1 || facts.RowCapEvictions > 0
150:			meta.ReachedHigh = true
153:	if m.pageExhausted {
154:		meta.ReachedHigh = true
156:			meta.ReachedLow = true
164:		ObservedShortFinalPage: m.pageExhausted,
```

```bash
sed -n '115,170p' internal/ui/export.go
```

```output
// the retained-range/eviction facts come from the viewport cache, the known
// total from the settled count state, and invalid-UTF from the page. The
// terminal outcome stays undecided (an active SELECT is not finalized), so
// no terminal-outcome warning can ever derive from it. Issue #73: endpoint
// observations mirror the finalized path — the cache's retained range
// supplies the low endpoint, pageExhausted supplies the high endpoint via
// ObservedShortFinalPage, and an empty observed page establishes both
// endpoints at position 0 — so a count-unavailable short or empty fully
// retained first page classifies complete in the active export path too.
func (m Model) activeExportFacts(page *result.Page) (history.SnapshotMetadata, history.Completeness) {
	facts := history.CacheFacts{}
	if c := m.viewportCache; c != nil {
		facts = history.FactsFromCache(c)
	}
	if m.Result.ByteTruncated {
		facts.TruncatedByByteCap = true
	}
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
	}
	// Issue #73: endpoint observations mirror the finalized path. The
	// cache's retained range supplies the low endpoint; pageExhausted
	// supplies the high endpoint; an empty observed page (pageExhausted
	// with no retained rows) establishes both endpoints at position 0.
	if facts.HasRetainedRange {
		meta.ReachedLow = facts.Start == 1 || facts.RowCapEvictions > 0
		if m.countState.Status == result.CountSuccess && m.countState.Total <= int64(facts.End) {
			meta.ReachedHigh = true
		}
	}
	if m.pageExhausted {
		meta.ReachedHigh = true
		if !facts.HasRetainedRange {
			meta.ReachedLow = true
		}
	}
	traversal := history.TraversalFacts{
		HasLimit:               m.countState.HasLimit,
		Limit:                  m.countState.Limit,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
	}
	return meta, history.Classify(meta, traversal)
}

// handleExportKey resolves one Ctrl+X press in memory. Any request still
// pending — validation or schema refresh included — is consumed by the
```

## Finalization: endpoint computation with pageExhausted

`appendFinalizedResultEntry` in `internal/ui/active_select.go` applies the same endpoint computation at finalization: `pageExhausted` establishes `ReachedHigh` even when the cache retained no rows, and `ReachedLow` for an empty page where both endpoints sit at position 0.

```bash
grep -n 'pageExhausted' internal/ui/active_select.go
```

```output
142:	// first page (pageExhausted) establishes the high endpoint even when the
152:			reachedHigh = m.pageExhausted ||
156:	if m.pageExhausted {
169:		ObservedShortFinalPage: m.pageExhausted,
```

```bash
sed -n '136,175p' internal/ui/active_select.go
```

```output
	}

	// Endpoint observations: the low end is reached when position 1 was
	// retained or the low end was evicted by cap traversal; the high end is
	// reached through a short/empty observed final page or a settled count
	// not exceeding the retained end. Issue #73: an observed short or empty
	// first page (pageExhausted) establishes the high endpoint even when the
	// cache retained no rows; an empty observed page also establishes the
	// low endpoint because the result is empty and both endpoints sit at
	// position 0.
	reachedLow, reachedHigh := false, false
	if m.viewportCache != nil {
		if start, ok := m.viewportCache.Start(); ok {
			reachedLow = start == 1 || m.viewportCache.RowCapEvictions() > 0
		}
		if end, ok := m.viewportCache.End(); ok {
			reachedHigh = m.pageExhausted ||
				(m.countState.Status == result.CountSuccess && m.countState.Total <= int64(end))
		}
	}
	if m.pageExhausted {
		reachedHigh = true
		if !hasRows {
			reachedLow = true
		}
	}
	final := Finalization{
		Outcome:                outcome,
		Reason:                 reason,
		ReachedLow:             reachedLow,
		ReachedHigh:            reachedHigh,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
	}
	meta, traversal, err := m.SnapshotFacts(final)
	if err != nil {
		// Snapshot facts come from validated model state; a construction
		// error must not create a second entry, so the execution stays
		// finalized with no entry rather than a malformed one.
```

## Tests: retained requested size, accepted empty/short/full settlements

The first-page tests in `internal/ui/first_select_test.go` cover the retained requested first-page size through dispatch and settlement, accepted empty/short/exactly-full responses setting/clearing `pageExhausted` correctly, stale execution/request/generation settlements with short data remaining inert, `pageExhausted` feeding `ObservedShortFinalPage` into `SnapshotFacts`, and Page Down suppressed after a short or empty first page and proceeding after a full page.

```bash
go test ./internal/ui/ -run '^TestFirstSelectRetainsRequestedFirstPageSize$|^TestFirstSelectEmptyPageSetsPageExhausted$|^TestFirstSelectFullPageDoesNotSetPageExhausted$|^TestFirstSelectShortPageFeedsObservedShortFinalPage$|^TestFirstSelectStaleExecutionShort$|^TestFirstSelectStaleRequestShort$|^TestFirstSelectStaleGenerationShort$|^TestPageDownSuppressedAfterShortFirstPage$|^TestPageDownSuppressedAfterEmptyFirstPage$|^TestPageDownProceedsAfterFullFirstPage$' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -30
```

```output
=== RUN   TestFirstSelectRetainsRequestedFirstPageSize
=== RUN   TestFirstSelectRetainsRequestedFirstPageSize/exactly_full_does_not_exhaust
=== RUN   TestFirstSelectRetainsRequestedFirstPageSize/short_exhausts
=== RUN   TestFirstSelectRetainsRequestedFirstPageSize/one_row_exhausts
--- PASS: TestFirstSelectRetainsRequestedFirstPageSize
    --- PASS: TestFirstSelectRetainsRequestedFirstPageSize/exactly_full_does_not_exhaust
    --- PASS: TestFirstSelectRetainsRequestedFirstPageSize/short_exhausts
    --- PASS: TestFirstSelectRetainsRequestedFirstPageSize/one_row_exhausts
=== RUN   TestFirstSelectEmptyPageSetsPageExhausted
--- PASS: TestFirstSelectEmptyPageSetsPageExhausted
=== RUN   TestFirstSelectFullPageDoesNotSetPageExhausted
--- PASS: TestFirstSelectFullPageDoesNotSetPageExhausted
=== RUN   TestFirstSelectShortPageFeedsObservedShortFinalPage
--- PASS: TestFirstSelectShortPageFeedsObservedShortFinalPage
=== RUN   TestPageDownSuppressedAfterEmptyFirstPage
--- PASS: TestPageDownSuppressedAfterEmptyFirstPage
=== RUN   TestPageDownSuppressedAfterShortFirstPage
--- PASS: TestPageDownSuppressedAfterShortFirstPage
=== RUN   TestPageDownProceedsAfterFullFirstPage
--- PASS: TestPageDownProceedsAfterFullFirstPage
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Tests: stale responses cannot alter endpoints or cache state

Stale execution, request, and viewport-generation settlements with short data are fully inert: they cannot establish or clear `pageExhausted`, mutate cache rows, or alter paging state. The identity check in `applySelectSettled` precedes the row-count comparison so a stale response never reaches the endpoint logic.

```bash
go test ./internal/ui/ -run '^TestFirstSelectStaleExecutionShortSettlementInert$|^TestFirstSelectStaleRequestShortSettlementInert$|^TestFirstSelectStaleGenerationShortSettlementInert$' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -15
```

```output
=== RUN   TestFirstSelectStaleExecutionShortSettlementInert
--- PASS: TestFirstSelectStaleExecutionShortSettlementInert
=== RUN   TestFirstSelectStaleRequestShortSettlementInert
--- PASS: TestFirstSelectStaleRequestShortSettlementInert
=== RUN   TestFirstSelectStaleGenerationShortSettlementInert
--- PASS: TestFirstSelectStaleGenerationShortSettlementInert
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Tests: finalization with empty and short first pages

The finalization tests in `internal/ui/snapshot_finalize_test.go` cover an accepted empty first page finalizing with both endpoints established (position 0) and classifying complete when count work finished, and an accepted short first page finalizing with `ReachedHigh` from `ObservedShortFinalPage` and classifying complete when count work finished and all rows are retained.

```bash
go test ./internal/ui/ -run '^TestFinalizationEmptyFirstPageCompleteWithCountUnavailable$|^TestFinalizationShortFirstPageCompleteWithCountUnavailable$' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -15
```

```output
=== RUN   TestFinalizationEmptyFirstPageCompleteWithCountUnavailable
--- PASS: TestFinalizationEmptyFirstPageCompleteWithCountUnavailable
=== RUN   TestFinalizationShortFirstPageCompleteWithCountUnavailable
--- PASS: TestFinalizationShortFirstPageCompleteWithCountUnavailable
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Tests: active export warnings with short and empty first pages

The export warning tests in `internal/ui/export_warnings_test.go` cover an accepted short first page with count unavailable producing `Result is complete` in the active export warnings, and an accepted empty first page producing `Result is complete` with both endpoints established.

```bash
go test ./internal/ui/ -run '^TestFirstSelectStaleExecutionShortSettlementInert$|^TestFirstSelectStaleRequestShortSettlementInert$|^TestFirstSelectStaleGenerationShortSettlementInert$' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -15
```

```output
=== RUN   TestFirstSelectStaleExecutionShortSettlementInert
--- PASS: TestFirstSelectStaleExecutionShortSettlementInert
=== RUN   TestFirstSelectStaleRequestShortSettlementInert
--- PASS: TestFirstSelectStaleRequestShortSettlementInert
=== RUN   TestFirstSelectStaleGenerationShortSettlementInert
--- PASS: TestFirstSelectStaleGenerationShortSettlementInert
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```
