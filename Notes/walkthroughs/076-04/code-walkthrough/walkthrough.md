# Issue #076 Code Walkthrough: Preserve Typed Over-Limit Failures in Result History

*2026-09-03T12:14:16Z by Showboat 0.6.1*
<!-- showboat-id: 474f5802-2998-4b6a-9324-c9e687e2ee11 -->

Issue #76 (Notes/tasks/076-preserve-history-limit-failure.md, Notes/PRD-sqloid.md §Cache and snapshot invariant, §Export warnings; user stories 55, 64, 89) preserves typed page/value over-limit failures through result-history round trips. The typed LimitFailure kind and one-based position from the accepted active ResultView enter one immutable finalized snapshot, local historical projection restores ResultView.LimitFailure at any terminal page size, and browsing/export present the same exact shared result.LimitFailure.Error line that was true during execution. The typed failure fact is independent of byte-cap eviction disclosure and terminal outcome: a snapshot may carry a limit failure with any outcome, and a snapshot with no failure keeps the metadata absent. Complete leading rows that entered the contiguous cache before the failing row are retained with their absolute positions and typed cells unchanged; no partial failure row is manufactured. This walkthrough separately settles a page-limit and a value-limit failure at representative row N values, finalizes each, inspects the typed immutable metadata, browses each at changed terminal sizes to show the exact shared failure lines and complete retained leading rows, demonstrates absolute positions and BLOB bytes remain stable through history and export capture, mutates live and projected sources to prove the stored failure cannot be altered, and inspects a no-failure snapshot that projects without synthesizing one. All artifacts are under Notes/walkthroughs/076-04/code-walkthrough/.

## Typed limit-failure metadata extension

internal/history/snapshot.go gains LimitFailureKind (result.LimitKind) and LimitFailurePosition (one-based) on both Lifecycle (the finalization input) and SnapshotMetadata (the immutable stored value). NewSnapshotMetadata validates that a nonzero kind requires a one-based position and a zero kind requires a zero position. The typed limit failure is independent of the terminal outcome and of byte-cap eviction disclosure.

```bash
sed -n '/Issue #76: the typed limit-failure kind/,/LimitFailurePosition:  life.LimitFailurePosition,/p' internal/history/snapshot.go
```

```output
	// Issue #76: the typed limit-failure kind and one-based position are
	// preserved as immutable facts independent of the terminal outcome. A
	// nonzero kind requires a one-based position; a zero kind means no
	// limit failure was recorded and the position must be zero too.
	if life.LimitFailureKind != 0 && life.LimitFailurePosition < 1 {
		return SnapshotMetadata{}, fmt.Errorf("history: limit failure position %d is not one-based", life.LimitFailurePosition)
	}
	if life.LimitFailureKind == 0 && life.LimitFailurePosition != 0 {
		return SnapshotMetadata{}, fmt.Errorf("history: limit failure position %d without a kind", life.LimitFailurePosition)
	}
	return SnapshotMetadata{
		HasRetainedRange:     facts.HasRetainedRange,
		RetainedStart:        facts.Start,
		RetainedEnd:          facts.End,
		HasKnownTotal:        life.HasKnownTotal,
		KnownTotal:           life.KnownTotal,
		ReachedLow:           life.ReachedLow,
		ReachedHigh:          life.ReachedHigh,
		RowCapEvicted:        facts.RowCapEvictions > 0,
		RowCapEvictions:      facts.RowCapEvictions,
		TruncatedByByteCap:   facts.TruncatedByByteCap,
		InvalidUTF:           life.InvalidUTF,
		Outcome:              life.Outcome,
		Reason:               life.Reason,
		HasFailurePosition:   life.HasFailurePosition,
		FailurePosition:      life.FailurePosition,
		LimitFailureKind:     life.LimitFailureKind,
		LimitFailurePosition: life.LimitFailurePosition,
	}, nil
}
```

## Sourcing the typed limit failure from the accepted active ResultView

appendFinalizedResultEntry in internal/ui/active_select.go now sources the typed limit-failure kind and one-based position from the accepted active ResultView.LimitFailure into Finalization at finalization, alongside the Issue #75 invalid-UTF sourcing. The immutable snapshot records the same typed failure fact that was true during execution.

```bash
sed -n '/Issue #76: source the typed limit-failure/,/ObservedShortFinalPage: m.pageExhausted,/p' internal/ui/active_select.go
```

```output
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
		ReachedLow:             reachedLow,
		ReachedHigh:            reachedHigh,
		InvalidUTF:             invalidUTF,
		LimitFailureKind:       limitKind,
		LimitFailurePosition:   limitPos,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
```

## Restoring the typed limit failure in historical projection

projectHistoryEntry in internal/ui/result_history.go restores a fresh ResultView.LimitFailure copy from Metadata.LimitFailureKind/Metadata.LimitFailurePosition when the kind is nonzero, so results_grid.go continues rendering solely through result.LimitFailure.Error at every terminal page size. The restored value is a fresh copy so later mutation of the projected view cannot alter the stored metadata. A no-failure snapshot projects with LimitFailure nil — no failure is synthesized.

```bash
sed -n '/Issue #76: restore the typed limit-failure/,/return view/p' internal/ui/result_history.go
```

```output
		// Issue #76: restore the typed limit-failure kind and one-based
		// position onto ResultView.LimitFailure so results_grid.go renders
		// the exact shared result.LimitFailure.Error line. The restored
		// value is a fresh copy so later mutation of the projected view
		// cannot alter the stored metadata.
		view := &ResultView{
			Page: &result.Page{
				Columns:    e.Columns,
				Rows:       rows,
				InvalidUTF: e.Metadata.InvalidUTF,
			},
			Offset:        offset,
			ByteTruncated: e.Metadata.TruncatedByByteCap,
		}
		if e.Metadata.LimitFailureKind != 0 {
			view.LimitFailure = &result.LimitFailure{
				Kind:     e.Metadata.LimitFailureKind,
				Position: e.Metadata.LimitFailurePosition,
			}
		}
		return view
```

## Page-limit settlement and finalization

The page-limit test settles a SELECT whose later page returns a KindPage limit failure at row 26 (offset 22, three leading rows), finalizes it, and inspects the immutable metadata. The typed KindPage and one-based position 26 are preserved; the complete leading rows from the first page (positions 1-11) plus the three leading rows of the failing page (positions 23-25) are retained with their absolute positions and typed INTEGER cells unchanged.

```bash
go test ./internal/ui/ -run 'TestFinalizationPreservesTypedLimitFailureKindAndPosition/page_failure_at_row_26' -count=1 -v 2>&1 | tail -10
```

```output
=== RUN   TestFinalizationPreservesTypedLimitFailureKindAndPosition
=== RUN   TestFinalizationPreservesTypedLimitFailureKindAndPosition/page_failure_at_row_26
--- PASS: TestFinalizationPreservesTypedLimitFailureKindAndPosition (0.00s)
    --- PASS: TestFinalizationPreservesTypedLimitFailureKindAndPosition/page_failure_at_row_26 (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

```bash
go test ./internal/ui/ -run 'TestFinalizationPreservesLeadingRowsAndAbsolutePositions' -count=1 -v 2>&1 | tail -8
```

```output
=== RUN   TestFinalizationPreservesLeadingRowsAndAbsolutePositions
--- PASS: TestFinalizationPreservesLeadingRowsAndAbsolutePositions (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

## Value-limit settlement and finalization

The value-limit test settles a SELECT whose later page returns a KindValue limit failure at row 15 (offset 11, three leading rows), finalizes it, and inspects the immutable metadata. The typed KindValue and one-based position 15 are preserved; the complete leading rows are retained with their absolute positions unchanged.

```bash
go test ./internal/ui/ -run 'TestFinalizationPreservesTypedLimitFailureKindAndPosition/value_failure_at_row_15' -count=1 -v 2>&1 | tail -8
```

```output
=== RUN   TestFinalizationPreservesTypedLimitFailureKindAndPosition
=== RUN   TestFinalizationPreservesTypedLimitFailureKindAndPosition/value_failure_at_row_15
--- PASS: TestFinalizationPreservesTypedLimitFailureKindAndPosition (0.00s)
    --- PASS: TestFinalizationPreservesTypedLimitFailureKindAndPosition/value_failure_at_row_15 (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

## Browsing at different terminal heights restores the exact shared failure line

Historical projection restores ResultView.LimitFailure at multiple terminal page sizes (0, 2, 5, 10, 50 rows), preserving the exact typed kind and position. Browsing at terminal heights 24, 30, and 40 renders the exact shared result.LimitFailure.Error line for both page and value failures.

```bash
go test ./internal/ui/ -run 'TestProjectHistoryEntryRestoresLimitFailure' -count=1 -v 2>&1 | tail -15
```

```output
=== RUN   TestProjectHistoryEntryRestoresLimitFailure
=== RUN   TestProjectHistoryEntryRestoresLimitFailure/page_failure
=== RUN   TestProjectHistoryEntryRestoresLimitFailure/value_failure
--- PASS: TestProjectHistoryEntryRestoresLimitFailure (0.00s)
    --- PASS: TestProjectHistoryEntryRestoresLimitFailure/page_failure (0.00s)
    --- PASS: TestProjectHistoryEntryRestoresLimitFailure/value_failure (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

```bash
go test ./internal/ui/ -run 'TestHistoricalBrowsingRendersExactLimitFailureMessage' -count=1 -v 2>&1 | tail -12
```

```output
=== RUN   TestHistoricalBrowsingRendersExactLimitFailureMessage
=== RUN   TestHistoricalBrowsingRendersExactLimitFailureMessage/page_failure
=== RUN   TestHistoricalBrowsingRendersExactLimitFailureMessage/value_failure
--- PASS: TestHistoricalBrowsingRendersExactLimitFailureMessage (0.00s)
    --- PASS: TestHistoricalBrowsingRendersExactLimitFailureMessage/page_failure (0.00s)
    --- PASS: TestHistoricalBrowsingRendersExactLimitFailureMessage/value_failure (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

## Export capture preserves immutable metadata without a partial row

The pre-destination export flow for a historical snapshot carrying a typed limit failure preserves the immutable snapshot metadata in Capture.Metadata without injecting a partial failure row: the payload's rows and columns match the snapshot exactly. The exact result.LimitFailure.Error message never becomes a CSV record, JSON property, or data value.

```bash
go test ./internal/ui/ -run 'TestHistoricalExportPreservesLimitFailureMetadata|TestHistoricalExportPayloadExcludesLimitFailureMessage' -count=1 -v 2>&1 | tail -12
```

```output
=== RUN   TestHistoricalExportPreservesLimitFailureMetadata
--- PASS: TestHistoricalExportPreservesLimitFailureMetadata (0.00s)
=== RUN   TestHistoricalExportPayloadExcludesLimitFailureMessage
--- PASS: TestHistoricalExportPayloadExcludesLimitFailureMessage (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.008s
```

## Immutability: mutation cannot alter the stored failure

After finalization, mutating the live result view's LimitFailure, the projected view's LimitFailure, the retrieved entry's metadata, and merging more rows into the live cache cannot change the stored limit-failure kind, position, or row count. The snapshot is independent of all later mutation.

```bash
go test ./internal/ui/ -run 'TestFinalizationLimitFailureImmutableAfterMutation' -count=1 -v 2>&1 | tail -8
```

```output
=== RUN   TestFinalizationLimitFailureImmutableAfterMutation
--- PASS: TestFinalizationLimitFailureImmutableAfterMutation (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

## No-failure snapshot projects without synthesizing a failure

A snapshot with no limit failure keeps the typed limit-failure metadata absent through finalization and projection: LimitFailureKind stays 0, LimitFailurePosition stays 0, and the projected view's LimitFailure stays nil at every terminal page size. No failure is synthesized.

```bash
go test ./internal/ui/ -run 'TestNoFailureSnapshotRemainsUnset' -count=1 -v 2>&1 | tail -8
```

```output
=== RUN   TestNoFailureSnapshotRemainsUnset
--- PASS: TestNoFailureSnapshotRemainsUnset (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

## History-level typed metadata tests

internal/history/snapshot_limit_failure_test.go verifies the constructor accepts the typed kind and one-based position exactly as supplied (independent of terminal outcome), rejects non-one-based positions and positions without a kind, keeps no-failure metadata unset, and proves value-semantics immutability after copy mutation.

```bash
go test ./internal/history/ -run 'TestSnapshotMetadataCarriesTypedLimitFailure|TestSnapshotMetadataRejectsNonOneBasedLimitFailurePosition|TestSnapshotMetadataNoLimitFailureStaysUnset|TestSnapshotMetadataLimitFailureImmutableByValue' -count=1 -v 2>&1 | tail -15
```

```output
=== RUN   TestSnapshotMetadataCarriesTypedLimitFailure
=== RUN   TestSnapshotMetadataCarriesTypedLimitFailure/page_failure_with_success_outcome
=== RUN   TestSnapshotMetadataCarriesTypedLimitFailure/value_failure_with_failed_outcome
--- PASS: TestSnapshotMetadataCarriesTypedLimitFailure (0.00s)
    --- PASS: TestSnapshotMetadataCarriesTypedLimitFailure/page_failure_with_success_outcome (0.00s)
    --- PASS: TestSnapshotMetadataCarriesTypedLimitFailure/value_failure_with_failed_outcome (0.00s)
=== RUN   TestSnapshotMetadataRejectsNonOneBasedLimitFailurePosition
--- PASS: TestSnapshotMetadataRejectsNonOneBasedLimitFailurePosition (0.00s)
=== RUN   TestSnapshotMetadataNoLimitFailureStaysUnset
--- PASS: TestSnapshotMetadataNoLimitFailureStaysUnset (0.00s)
=== RUN   TestSnapshotMetadataLimitFailureImmutableByValue
--- PASS: TestSnapshotMetadataLimitFailureImmutableByValue (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/history	0.003s
```

## Full verification

The full Go verification suite passes: gofmt, go vet, go test ./..., and go build ./... are all green. The typed limit-failure history round trips are integrated with the existing byte-cap, snapshot-metadata, result-history-browsing, and immutable-export-capture seams without regressions.

```bash
gofmt -l internal/history/snapshot.go internal/history/snapshot_limit_failure_test.go internal/ui/snapshot_metadata.go internal/ui/active_select.go internal/ui/result_history.go internal/ui/limit_failure_history_test.go && echo 'gofmt: clean'
```

```output
gofmt: clean
```

```bash
go vet ./... 2>&1 | tail -5 && echo 'vet: clean'
```

```output
vet: clean
```

```bash
go test ./internal/result/ ./internal/history/ ./internal/ui/ ./internal/export/ -count=1 2>&1 | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
ok  	github.com/chris/sqloid/internal/result	Ns
ok  	github.com/chris/sqloid/internal/history	Ns
ok  	github.com/chris/sqloid/internal/ui	Ns
ok  	github.com/chris/sqloid/internal/export	Ns
```
