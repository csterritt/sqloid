# Issue #075 Code Walkthrough: Preserve Invalid-UTF and Byte-Cap Warnings in Result History

*2026-09-03T00:43:10Z by Showboat 0.6.1*
<!-- showboat-id: f521fc07-60d7-49a0-be92-78484400ee80 -->

Issue #75 (Notes/tasks/075-preserve-history-utf8-byte-warnings.md, Notes/PRD-sqloid.md §Cache and snapshot invariant, §Export warnings; user stories 55, 64, 70) preserves invalid-UTF and byte-cap warning metadata through result-history round trips. Accepted active-page invalid-UTF truth and persistent cache byte-cap truth enter one immutable finalized snapshot, local historical projection restores both warnings at any terminal page size, and browsing/export present the same shared warning definitions that were true during execution. Warning facts do not alter rows or logical positions and never become CSV records, JSON properties, or synthetic values. This walkthrough separately executes/finalizes a malformed-TEXT result and a byte-cap-truncated result, inspects their immutable snapshot metadata, browses each at changed terminal sizes, shows the restored shared warning in the historical result and pre-destination export flow, mutates live and projected sources to prove rows, BLOB bytes, positions, and warning facts remain immutable, and inspects CSV/JSON serializer input/output to prove no warning record or property is emitted. All artifacts are under Notes/walkthroughs/075-04/code-walkthrough/.

```bash
sed -n '/Issue #75: source invalid-UTF/,/ObservedShortFinalPage: m.pageExhausted,/p' internal/ui/active_select.go
```

```output
	// Issue #75: source invalid-UTF truth from the accepted active page so
	// it enters the immutable snapshot through Finalization. The persistent
	// byte-cap truth is already sourced from the authoritative cache via
	// FactsFromCache in SnapshotFacts, never re-derived from payload size.
	invalidUTF := false
	if m.Result != nil && m.Result.Page != nil {
		invalidUTF = m.Result.Page.InvalidUTF
	}
	final := Finalization{
		Outcome:                outcome,
		Reason:                 reason,
		ReachedLow:             reachedLow,
		ReachedHigh:            reachedHigh,
		InvalidUTF:             invalidUTF,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
```

## Sourcing invalid-UTF truth from the accepted active page

appendFinalizedResultEntry in internal/ui/active_select.go now sources invalid-UTF truth from the accepted active page (m.Result.Page.InvalidUTF) into Finalization.InvalidUTF at finalization. The persistent byte-cap truth continues to come from the authoritative cache via FactsFromCache in SnapshotFacts, never re-derived from current payload size.

```bash
sed -n '/Issue #75: source invalid-UTF/,/ObservedShortFinalPage: m.pageExhausted,/p' internal/ui/active_select.go
```

```output
	// Issue #75: source invalid-UTF truth from the accepted active page so
	// it enters the immutable snapshot through Finalization. The persistent
	// byte-cap truth is already sourced from the authoritative cache via
	// FactsFromCache in SnapshotFacts, never re-derived from payload size.
	invalidUTF := false
	if m.Result != nil && m.Result.Page != nil {
		invalidUTF = m.Result.Page.InvalidUTF
	}
	final := Finalization{
		Outcome:                outcome,
		Reason:                 reason,
		ReachedLow:             reachedLow,
		ReachedHigh:            reachedHigh,
		InvalidUTF:             invalidUTF,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
```

## Restoring warnings in historical projection

projectHistoryEntry in internal/ui/result_history.go now restores the immutable snapshot's warning metadata onto the projected view: Metadata.InvalidUTF is restored onto the new result.Page (the grid's invalid-UTF truth source) and Metadata.TruncatedByByteCap onto ResultView.ByteTruncated (the persistent byte-cap disclosure). Offset, rows, columns, and BLOB copies are preserved; the stored entry is never touched.

```bash
sed -n '/Issue #75: restore the immutable/,/ByteTruncated: e.Metadata.TruncatedByByteCap,/p' internal/ui/result_history.go
```

```output
		// Issue #75: restore the immutable snapshot's warning metadata onto
		// the projected view so the shared UTF and byte-cap warnings render
		// truthfully at every terminal page size. InvalidUTF is restored
		// onto the new result.Page (the grid's invalid-UTF truth source)
		// and TruncatedByByteCap onto ResultView.ByteTruncated (the
		// persistent byte-cap disclosure). Offset, rows, columns, and BLOB
		// copies are preserved; the stored entry is never touched.
		return &ResultView{
			Page: &result.Page{
				Columns:    e.Columns,
				Rows:       rows,
				InvalidUTF: e.Metadata.InvalidUTF,
			},
			Offset:        offset,
			ByteTruncated: e.Metadata.TruncatedByByteCap,
```

## Finalizing a malformed-TEXT result preserves InvalidUTF metadata

TestFinalizationPreservesInvalidUTFFromAcceptedPage drives a real first-page execution carrying malformed TEXT (Page.InvalidUTF set) through validation, execution start, and first-page settlement, then finalizes via enterResultHistory. The finalized snapshot's Metadata.InvalidUTF is true.

```bash
go test ./internal/ui/ -run '^TestFinalizationPreservesInvalidUTFFromAcceptedPage$' -v -count=1 2>&1
```

```output
=== RUN   TestFinalizationPreservesInvalidUTFFromAcceptedPage
--- PASS: TestFinalizationPreservesInvalidUTFFromAcceptedPage (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.012s
```

## Finalizing a byte-cap-truncated result preserves TruncatedByByteCap metadata

TestFinalizationPreservesByteCapFromAuthoritativeCache drives a real first-page execution, replaces the active SELECT's authoritative cache with a pre-seeded byte-cap-truncated cache (TruncatedByByteCap sticky), mirrors the disclosure onto ResultView.ByteTruncated, then finalizes via enterResultHistory. The finalized snapshot's Metadata.TruncatedByByteCap is true; the persistent byte-cap truth is sourced from the authoritative cache via FactsFromCache, never re-derived from current payload size.

```bash
go test ./internal/ui/ -run '^TestFinalizationPreservesByteCapFromAuthoritativeCache$' -v -count=1 2>&1
```

```output
=== RUN   TestFinalizationPreservesByteCapFromAuthoritativeCache
--- PASS: TestFinalizationPreservesByteCapFromAuthoritativeCache (0.23s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.269s
```

## Inspecting immutable snapshot metadata

The finalized entries' Metadata carries both warning facts. InvalidUTF comes from the accepted active page; TruncatedByByteCap comes from the authoritative cache. Both are immutable: later mutation of live pages, projected views, source BLOBs, or cache state cannot change them.

```bash
go test ./internal/ui/ -run '^TestFinalizationWarningMetadataImmutableAfterMutation$' -v -count=1 2>&1
```

```output
=== RUN   TestFinalizationWarningMetadataImmutableAfterMutation
--- PASS: TestFinalizationWarningMetadataImmutableAfterMutation (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.010s
```

## Browsing at changed terminal sizes restores both warnings

TestProjectHistoryEntryRestoresInvalidUTFAndByteCap projects a snapshot carrying both warning facts at multiple terminal page sizes (0, 2, 5, 10 rows). Page.InvalidUTF and ResultView.ByteTruncated are restored at every size; offset, rows, columns, and BLOB copies are preserved. TestProjectHistoryEntryRestoresByteCapOnly proves a byte-cap-only snapshot restores ByteTruncated without wrongly setting InvalidUTF.

```bash
go test ./internal/ui/ -run '^TestProjectHistoryEntryRestoresInvalidUTFAndByteCap$|^TestProjectHistoryEntryRestoresByteCapOnly$' -v -count=1 2>&1
```

```output
=== RUN   TestProjectHistoryEntryRestoresInvalidUTFAndByteCap
--- PASS: TestProjectHistoryEntryRestoresInvalidUTFAndByteCap (0.00s)
=== RUN   TestProjectHistoryEntryRestoresByteCapOnly
--- PASS: TestProjectHistoryEntryRestoresByteCapOnly (0.15s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.223s
```

## Shared warnings render in historical browsing

TestHistoricalBrowsingRendersSharedWarnings enters result-history browsing for a snapshot carrying both warning facts and resizes across multiple terminal heights (24, 30, 40). The shared result.UTFWarning and result.ByteCapWarning both appear in the browsing view at every height — the same shared definitions that were true during execution.

```bash
go test ./internal/ui/ -run '^TestHistoricalBrowsingRendersSharedWarnings$' -v -count=1 2>&1
```

```output
=== RUN   TestHistoricalBrowsingRendersSharedWarnings
--- PASS: TestHistoricalBrowsingRendersSharedWarnings (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.013s
```

## Pre-destination export flow shows both warnings

TestHistoricalExportWarningsRenderBeforeDestination opens the Ctrl+X export flow from a historical snapshot carrying both warning facts. The shared result.UTFWarning and result.ByteCapWarning both appear in the pre-destination export warnings, rendered before any destination selection.

```bash
go test ./internal/ui/ -run '^TestHistoricalExportWarningsRenderBeforeDestination$' -v -count=1 2>&1
```

```output
=== RUN   TestHistoricalExportWarningsRenderBeforeDestination
--- PASS: TestHistoricalExportWarningsRenderBeforeDestination (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.012s
```

## CSV/JSON serializer input/output excludes warnings

TestHistoricalExportPayloadExcludesWarnings proves neither warning becomes a CSV record, JSON object property, or data value. The serializer-visible Capture.Payload carries only names/positions/rows — no warning text appears in any name or cell token. Direct export.CSV and export.JSON output contains no warning literals, and the payload has exactly the snapshot's columns and rows (no warning column or row injected). Warning facts do not alter rows or logical positions.

```bash
go test ./internal/ui/ -run '^TestHistoricalExportPayloadExcludesWarnings$' -v -count=1 2>&1
```

```output
=== RUN   TestHistoricalExportPayloadExcludesWarnings
--- PASS: TestHistoricalExportPayloadExcludesWarnings (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.011s
```

## Full focused test suite

All internal/ui, internal/history, and internal/export tests pass after the change.

```bash
go test ./internal/ui/ ./internal/history/ ./internal/export/ -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/ui	1.533s
ok  	github.com/chris/sqloid/internal/history	0.300s
ok  	github.com/chris/sqloid/internal/export	0.165s
```

## References

- Issue #75: Notes/tasks/075-preserve-history-utf8-byte-warnings.md
- PRD: Notes/PRD-sqloid.md §Cache and snapshot invariant, §Export warnings; user stories 55, 64, 70.
- Wiki: Notes/wiki/snapshot-metadata.md, Notes/wiki/result-history-browsing.md, Notes/wiki/byte-cap-oversized-values.md, Notes/wiki/shared-typed-result-rendering.md, Notes/wiki/immutable-export-capture.md
- Source: internal/ui/active_select.go (appendFinalizedResultEntry), internal/ui/result_history.go (projectHistoryEntry), internal/ui/snapshot_warning_roundtrip_test.go
- All artifacts are under Notes/walkthroughs/075-04/code-walkthrough/.
