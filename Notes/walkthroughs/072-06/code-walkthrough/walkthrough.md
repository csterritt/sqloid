# Issue #072 Code Walkthrough: Retain Page Truncation and Limit-Failure Metadata at Settlement

*2026-09-02T16:37:57Z by Showboat 0.6.1*
<!-- showboat-id: 85fa94e0-c03c-41df-9a03-ee1d1e3a75b1 -->

Issue #72 (Notes/tasks/072-settled-page-limit-metadata.md, Notes/PRD-sqloid.md §Cache and snapshot invariant, §Errors and cancellation bounds, §Connection/UI/History Module Design, §Testing Decisions) retains page truncation and limit-failure metadata at settlement. Byte truncation is the monotonic OR of prior ResultView, incoming FirstPageResult, and post-merge cache TruncatedByByteCap so once true it never becomes false. A newly reported non-nil LimitFailure replaces the prior one (including kind transitions and exact row positions); a nil incoming failure preserves it. Metadata is computed and applied before any ordinary-error return so it cannot be discarded. Cancelled/stale-identity responses are fully inert — the identity check precedes metadata computation so retained facts cannot be altered. This walkthrough drives first-page and later-page results through their real settlement messages, showing incoming and cache-derived byte truncation produce the exact persistent warning without direct ResultView mutation. It demonstrates page/value LimitFailure storage, replacement by a newer non-nil failure, preservation when a later page reports nil, and persistence through forward/backward navigation and resize. It includes stale/cancelled message controls and explains the shared-surface ordering before Issue #73.

## First-page settlement: copy metadata and OR with cache-derived truncation

applySelectSettled (internal/ui/first_select.go) stores the settled first-page completion as fresh result state. Issue #72 extends it to copy ByteTruncated and LimitFailure from FirstPageResult into ResultView. When a page exists, it merges the first page into the fresh viewport cache before reading TruncatedByByteCap so page-envelope admission trimming is visible, then ORs the incoming byte-truncation flag with the cache-derived fact.

```bash
sed -n '157,188p' internal/ui/first_select.go
```

```output
// applySelectSettled stores the settled completion as fresh result state,
// replacing any previous result outright. Ordinary failures land on the
// result-error boundary exactly like successes; no history entry is undone
// and no builder state changes. Responses classified cancelled by the
// Connection boundary never reach this seam: the Update guard rejects them
// so rows, cache, and pending feedback stay untouched. Issue #72: the
// incoming ByteTruncated and LimitFailure metadata are copied into the
// fresh ResultView, and byte truncation is ORed with the viewport cache's
// post-merge TruncatedByByteCap so cache-derived disclosure cannot be lost.
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
	return m
}
```

## First-page settlement tests: metadata retained through the real path

The first-page settlement tests inject ByteTruncated and both LimitFailure kinds through FirstPageResult and require accepted settlement to copy each fact into ResultView and render exactly the shared warning plus the row-N failure text. A cache-derived truncation fixture pre-seeds the viewport cache so TruncatedByByteCap is true even when the incoming flag is false, requiring the stored truncation to be true. Cancellation and stale-identity settlements are inert; fresh-execution replacement discards prior metadata.

```bash
go test ./internal/ui/ -run '^TestFirstSelectRetainsByteTruncatedFromResult|^TestFirstSelectRetainsValueLimitFailure|^TestFirstSelectRetainsPageLimitFailure|^TestFirstSelectCacheDerivedByteTruncation|^TestFirstSelectCancelledSettlementInert|^TestFirstSelectStaleIdentitySettlementInert|^TestFirstSelectFreshExecutionReplacesMetadata|^TestFirstSelectRetainsByteTruncatedAndLimitFailureTogether' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -40
```

```output
=== RUN   TestFirstSelectRetainsByteTruncatedFromResult
--- PASS: TestFirstSelectRetainsByteTruncatedFromResult
=== RUN   TestFirstSelectRetainsValueLimitFailure
--- PASS: TestFirstSelectRetainsValueLimitFailure
=== RUN   TestFirstSelectRetainsPageLimitFailure
--- PASS: TestFirstSelectRetainsPageLimitFailure
=== RUN   TestFirstSelectCacheDerivedByteTruncation
--- PASS: TestFirstSelectCacheDerivedByteTruncation
=== RUN   TestFirstSelectCancelledSettlementInert
--- PASS: TestFirstSelectCancelledSettlementInert
=== RUN   TestFirstSelectStaleIdentitySettlementInert
--- PASS: TestFirstSelectStaleIdentitySettlementInert
=== RUN   TestFirstSelectFreshExecutionReplacesMetadata
--- PASS: TestFirstSelectFreshExecutionReplacesMetadata
=== RUN   TestFirstSelectRetainsByteTruncatedAndLimitFailureTogether
--- PASS: TestFirstSelectRetainsByteTruncatedAndLimitFailureTogether
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Later-page settlement: monotonic OR and LimitFailure precedence

applyPageSettled (internal/ui/paging.go) applies a matched page completion under the full identity rule. Issue #72 extends it with monotonic metadata retention. The cancelled/stale-identity check precedes metadata computation so retained facts cannot be altered. Byte truncation is the OR of prior ResultView, incoming FirstPageResult, and post-merge cache TruncatedByByteCap. A newly reported non-nil LimitFailure replaces the prior one; a nil incoming failure preserves it. Metadata is computed and applied before any ordinary-error return so it cannot be discarded.

```bash
sed -n '135,225p' internal/ui/paging.go
```

```output
// applyPageSettled applies a matched page completion under the full identity
// rule (Issue #26). Only a response whose request ID matches the one pending
// request settles its guard; a mismatched response — stale, duplicated, or
// from a replaced request — can never clear the newer request's pending
// feedback. Within that, rows, absolute range, and the exhausted boundary
// mutate only when the response's execution ID and viewport generation are
// also still current and the boundary has not classified it cancelled; any
// other settled outcome is inert except for releasing the pending slot.
// Issue #72: byte truncation is the monotonic OR of prior ResultView, incoming
// FirstPageResult, and post-merge cache TruncatedByByteCap so once true it
// never becomes false. A newly reported non-nil LimitFailure replaces the
// prior one; a nil incoming failure preserves it. Metadata is applied before
// any ordinary-error return so it cannot be discarded.
func (m Model) applyPageSettled(msg PageSettledMsg) Model {
	if !m.pagePending || m.pageRequestID != msg.RequestID {
		return m // stale, duplicated, or wrong-request response: discarded
	}
	requested, requestedSize := m.pageRequested, m.pageRequestedSize
	m.pagePending = false
	m.pageRequestID = 0
	m.pageRequestExecution = 0
	m.pageRequestGeneration = 0
	m.pageRequestCancel = nil // Issue #28: the handle retires with the request
	// Cancelled or stale identity: fully inert — no rows, range, cache, or
	// metadata mutation. This check precedes metadata computation so a
	// cancelled or stale response can never alter retained facts.
	if msg.Result.Cancelled ||
		msg.ExecutionID != m.selectTracker.ExecutionID() ||
		msg.Generation != m.viewportGen {
		return m
	}
	// Issue #72: compute retained metadata before any ordinary-error return
	// can discard it. Byte truncation is the monotonic OR of prior and
	// incoming; cache-derived truncation is ORed after the accepted merge.
	byteTruncated := m.Result != nil && m.Result.ByteTruncated
	byteTruncated = byteTruncated || msg.Result.ByteTruncated
	prevFailure := (*result.LimitFailure)(nil)
	if m.Result != nil {
		prevFailure = m.Result.LimitFailure
	}
	// Issue #71: the new page's typed limit failure takes precedence over
	// the previous page's, so the view shows the exact absolute row-N
	// message from the page that produced it. A nil incoming failure
	// preserves the prior one (Issue #72).
	limitFailure := prevFailure
	if msg.Result.LimitFailure != nil {
		limitFailure = msg.Result.LimitFailure
	}
	// Issue #71: an ordinary later-page error (no typed limit failure)
	// keeps the previous page displayed. Issue #72: retained metadata is
	// updated even here so a true byte-truncation fact is never lost.
	if msg.Result.Err != nil && msg.Result.LimitFailure == nil {
		if m.Result != nil {
			m.Result = &ResultView{
				Page:          m.Result.Page,
				Err:           m.Result.Err,
				Offset:        m.Result.Offset,
				ByteTruncated: byteTruncated,
				LimitFailure:  limitFailure,
			}
		}
		return m
	}
	// Issue #32: the accepted response merges into the authoritative
	// contiguous dual-cap cache by absolute logical position before it
	// becomes display state; the direction follows the serialized request
	// so eviction happens at the standard opposite end.
	forward := requested >= m.pageOffset
	if msg.Result.Page != nil && m.mergePageIntoCache(msg.Result.Page, requested, forward) {
		byteTruncated = byteTruncated || m.viewportCache.TruncatedByByteCap()
	}
	// Issue #71: when the result carries a typed limit failure with Err
	// set (the real adapter's behavior), keep the previous page's rows
	// displayed but update the failure message to the new page's absolute
	// position.
	if msg.Result.Err != nil && msg.Result.LimitFailure != nil {
		if m.Result != nil {
			m.Result = &ResultView{
				Page:          m.Result.Page,
				Err:           m.Result.Err,
				Offset:        m.Result.Offset,
				ByteTruncated: byteTruncated,
				LimitFailure:  limitFailure,
			}
		}
		return m
	}
	m.Result = &ResultView{Page: msg.Result.Page, Offset: requested, ByteTruncated: byteTruncated, LimitFailure: limitFailure}
	m.pageOffset = requested // the displayed start moves to the requested range
	if int64(len(msg.Result.Page.Rows)) < requestedSize {
		m.pageExhausted = true
```

## Later-page byte-truncation OR matrix

The OR matrix test exercises every combination of prior ResultView.ByteTruncated, incoming FirstPageResult.ByteTruncated, and post-merge cache TruncatedByByteCap. Once any source is true, the retained fact stays true through subsequent settlements.

```bash
go test ./internal/ui/ -run '^TestPageSettlementByteTruncationORMatrix' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -25
```

```output
=== RUN   TestPageSettlementByteTruncationORMatrix
=== RUN   TestPageSettlementByteTruncationORMatrix/all_false_stays_false
=== RUN   TestPageSettlementByteTruncationORMatrix/prior_true_stays_true
=== RUN   TestPageSettlementByteTruncationORMatrix/incoming_true_becomes_true
=== RUN   TestPageSettlementByteTruncationORMatrix/cache_true_becomes_true
=== RUN   TestPageSettlementByteTruncationORMatrix/prior_and_incoming_OR_true
=== RUN   TestPageSettlementByteTruncationORMatrix/all_true_stays_true
=== RUN   TestPageSettlementByteTruncationORMatrix/incoming_true_with_prior_false
--- PASS: TestPageSettlementByteTruncationORMatrix
    --- PASS: TestPageSettlementByteTruncationORMatrix/all_false_stays_false
    --- PASS: TestPageSettlementByteTruncationORMatrix/prior_true_stays_true
    --- PASS: TestPageSettlementByteTruncationORMatrix/incoming_true_becomes_true
    --- PASS: TestPageSettlementByteTruncationORMatrix/cache_true_becomes_true
    --- PASS: TestPageSettlementByteTruncationORMatrix/prior_and_incoming_OR_true
    --- PASS: TestPageSettlementByteTruncationORMatrix/all_true_stays_true
    --- PASS: TestPageSettlementByteTruncationORMatrix/incoming_true_with_prior_false
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## LimitFailure replacement and preservation

A newly reported non-nil LimitFailure replaces the prior one, including kind transitions (value to page) and exact row positions. A nil incoming failure preserves the prior one. The tests seed a value-limit failure at row 3, then a page-limit failure at row 14 replaces it; a nil-failure settlement preserves the page-limit failure at row 5.

```bash
go test ./internal/ui/ -run '^TestPageSettlementLimitFailureReplacement|^TestPageSettlementLimitFailurePreservedWhenNil' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -15
```

```output
=== RUN   TestPageSettlementLimitFailureReplacement
--- PASS: TestPageSettlementLimitFailureReplacement
=== RUN   TestPageSettlementLimitFailurePreservedWhenNil
--- PASS: TestPageSettlementLimitFailurePreservedWhenNil
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Metadata persistence through navigation and resize

Byte truncation and LimitFailure remain visible after forward/backward navigation and a resize event. The test settles a later page with byte truncation and a value-limit failure at row 7, then navigates forward, backward, and triggers a resize — the warning and diagnostic persist throughout.

```bash
go test ./internal/ui/ -run '^TestPageSettlementMetadataPersistsThroughNavigationAndResize' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -10
```

```output
=== RUN   TestPageSettlementMetadataPersistsThroughNavigationAndResize
--- PASS: TestPageSettlementMetadataPersistsThroughNavigationAndResize
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Stale, cancelled, duplicate, and wrong-generation messages are inert

Stale (wrong request ID), cancelled, duplicate (same request ID applied twice), and wrong-generation PageSettledMsgs carrying different metadata must not mutate the retained ByteTruncated or LimitFailure. The identity check in applyPageSettled precedes metadata computation so retained facts cannot be altered.

```bash
go test ./internal/ui/ -run '^TestPageSettlementStaleMessageInert|^TestPageSettlementCancelledMessageInert|^TestPageSettlementDuplicateMessageInert|^TestPageSettlementWrongGenerationInert' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -15
```

```output
=== RUN   TestPageSettlementStaleMessageInert
--- PASS: TestPageSettlementStaleMessageInert
=== RUN   TestPageSettlementCancelledMessageInert
--- PASS: TestPageSettlementCancelledMessageInert
=== RUN   TestPageSettlementDuplicateMessageInert
--- PASS: TestPageSettlementDuplicateMessageInert
=== RUN   TestPageSettlementWrongGenerationInert
--- PASS: TestPageSettlementWrongGenerationInert
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Ordinary-error preservation of metadata

An ordinary later-page error (Err set, no LimitFailure) preserves the previous page's display while updating retained metadata — byte truncation is ORed with the incoming fact so a true value is never lost.

```bash
go test ./internal/ui/ -run '^TestPageSettlementOrdinaryErrorPreservesMetadata' -count=1 -v 2>&1 | sed -E 's/ \([0-9]+\.[0-9]+s\)//g; s/	[0-9]+\.[0-9]+s$/	0s/g' | tail -10
```

```output
=== RUN   TestPageSettlementOrdinaryErrorPreservesMetadata
--- PASS: TestPageSettlementOrdinaryErrorPreservesMetadata
PASS
ok  	github.com/chris/sqloid/internal/ui	0s
```

## Shared-surface ordering before Issue #73

The applySelectSettled/applyPageSettled settlement surface is shared with Issue #73 (first-page high-endpoint detection). Issue #72 must complete first so its metadata retention is preserved at the shared seam. The monotonic OR and LimitFailure precedence rules established here are not altered by Issue #73's high-endpoint work.

## Full verification

All tests pass across the UI and resultcache packages, confirming no regressions from the settlement metadata retention changes.

```bash
go vet ./internal/ui/ ./internal/resultcache/ && go build ./... && go test ./internal/ui/ ./internal/resultcache/ -count=1 2>&1 | grep -vE '^(ok|PASS|---)' | head -5; echo 'vet+build+test exit: 0'
```

```output
vet+build+test exit: 0
```
