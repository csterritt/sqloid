# Issue #079 Code Walkthrough: Remove Unused Traversal Limit Fields

*2026-09-03T15:15:26Z by Showboat 0.6.1*
<!-- showboat-id: cfa6f2e4-e527-47e2-8984-bf4995421fa0 -->

Issue #79 (Notes/tasks/079-remove-unused-traversal-limit-fields.md, Notes/PRD-sqloid.md Cache and snapshot invariant; user stories 55, 56, 59) removes the dead HasLimit and Limit fields from history.TraversalFacts, from the SnapshotFacts initializer in internal/ui/snapshot_metadata.go, and from the activeExportFacts initializer in internal/ui/export.go. The executed builder Limit is intentionally not a traversal fact: a successful known total already counts the complete SELECT including the user Limit (the count subquery wraps the entire SelectSQL with the Limit inside it), so rows beyond that limited logical result are irrelevant and history.Classify consumes no raw builder Limit. This walkthrough shows the reduced TraversalFacts definition and each finalization/active-export producer with no HasLimit or Limit, exercises paired limited known-total and equivalent unbounded classifier cases to demonstrate identical complete, partial, truncated, endpoint, and count/cache-inconsistency results, includes repository search evidence that no removed field remains, focused passing history/UI/export tests, and the established verification output. Reference: Issue #79 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/079-03/code-walkthrough/.

## The reduced TraversalFacts definition

```bash
sed -n '/^\/\/ TraversalFacts carries the count and observed-page/,/^}/p' internal/history/snapshot_classify.go
```

```output
// TraversalFacts carries the count and observed-page lifecycle facts
// supplied by internal/ui at finalization time. CountWorkFinished reports
// that the count request settled (success or failure) or was never issued;
// PageWorkFinished reports that no page work was outstanding or deferred.
// ObservedShortFinalPage records that a final page was actually observed to
// return fewer rows than requested (including an empty page), which alone —
// when the count is unavailable or failed — establishes the high endpoint.
// CountCacheInconsistent records contradictory count/cache evidence, which
// classification preserves without clamping any observed fact. The executed
// builder Limit is intentionally absent: a successful known total already
// counts the complete SELECT including the user's Limit, so rows beyond
// that limited logical result are irrelevant and classification needs no
// raw builder Limit.
type TraversalFacts struct {
	CountWorkFinished      bool
	PageWorkFinished       bool
	ObservedShortFinalPage bool
	CountCacheInconsistent bool
}
```

The struct now carries only the four consumed traversal inputs. The doc comment states explicitly that the executed builder Limit is intentionally absent because a successful known total already counts the complete SELECT including the user's Limit.

## The Classify doc comment now states the known total includes Limit

```bash
sed -n '/^\/\/ Classify computes/,/func Classify/p' internal/history/snapshot_classify.go
```

```output
// Classify computes the exclusive-or-coexisting completeness labels from
// immutable snapshot metadata and traversal facts. It never mutates its
// inputs: rows, retained range, known total, and endpoint observations stand
// exactly as observed, even when contradictory. Complete is possible only
// when the high endpoint is established, every row of the limited logical
// result is retained, no eviction occurred, all count and page work finished,
// and the low-endpoint condition holds: an empty logical result (high == 0)
// is vacuously complete without a low row to observe, while a nonempty result
// requires ReachedLow. A successful known total already counts the complete
// SELECT including the user's Limit, so rows beyond that limited logical
// result are irrelevant and classification consumes no raw builder Limit.
func Classify(meta SnapshotMetadata, t TraversalFacts) Completeness {
```

## The SnapshotFacts producer in internal/ui/snapshot_metadata.go

```bash
sed -n '/traversal := history.TraversalFacts{/,/return meta, traversal, nil/p' internal/ui/snapshot_metadata.go
```

```output
	traversal := history.TraversalFacts{
		CountWorkFinished:      countWorkFinished,
		PageWorkFinished:       f.PageWorkFinished,
		ObservedShortFinalPage: f.ObservedShortFinalPage,
		CountCacheInconsistent: f.CountCacheInconsistent,
	}
	return meta, traversal, nil
```

No  or  field is set; the initializer uses only the four retained traversal inputs.

## The activeExportFacts producer in internal/ui/export.go

```bash
sed -n '/traversal := history.TraversalFacts{/,/return meta, history.Classify/p' internal/ui/export.go
```

```output
	traversal := history.TraversalFacts{
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
	}
	return meta, history.Classify(meta, traversal)
```

The active export path likewise sets only the three retained traversal inputs (CountCacheInconsistent is a finalization-only fact and is not set here, per Issue #78).

## Repository search evidence: no removed field remains in production or test code

```bash
grep -RnE 'HasLimit|Limit:' internal/history/snapshot_classify.go internal/ui/snapshot_metadata.go internal/ui/export.go internal/history/snapshot_classify_test.go internal/ui/snapshot_finalize_test.go internal/ui/first_select_test.go || echo 'no matches in the touched files'
```

```output
internal/history/snapshot_classify_test.go:447:// classification boundary after removing the dead HasLimit/Limit traversal
```

```bash
grep -RnE 'TraversalFacts\{[^}]*HasLimit|TraversalFacts\{[^}]*Limit:' internal/ || echo 'no TraversalFacts initializer sets HasLimit or Limit'
```

```output
no TraversalFacts initializer sets HasLimit or Limit
```

No TraversalFacts initializer anywhere in internal/ sets HasLimit or Limit. The only remaining textual mention is the explanatory comment in the new equivalence test. CountState in internal/result/count.go retains its own HasLimit/Limit fields because they drive the exact 'Result count: N (after Limit M)' presentation wording, which is unrelated to classification.

## Paired limited known-total vs equivalent unbounded classifier cases

The new TestLimitedKnownTotalEquivalentToUnbounded pins the boundary: a successful known total already counts the complete SELECT including the user's Limit, so limited known-total cases produce labels identical to the equivalent unbounded fact set with the same total, retained range, and endpoint observations. The traversal facts carry no Limit field, so a single Classify call covers both interpretations.

```bash
sed -n '/func TestLimitedKnownTotalEquivalentToUnbounded/,/^}/p' internal/history/snapshot_classify_test.go
```

```output
func TestLimitedKnownTotalEquivalentToUnbounded(t *testing.T) {
	cases := []struct {
		name       string
		fact       CacheFacts
		life       Lifecycle
		traversal  TraversalFacts
		wantLabels Completeness
	}{
		{
			name:       "complete: limited total equals unbounded total of same size",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 5},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 5, ReachedLow: true, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name:       "truncated: limited total above retained range matches unbounded",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10, RowCapEvictions: 90},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Truncated: true},
		},
		{
			name:       "partial+truncated: limited total with unseen low endpoint matches unbounded",
			fact:       CacheFacts{HasRetainedRange: true, Start: 11, End: 20},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name:       "partial: limited total with count/cache inconsistency matches unbounded",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 5, ReachedLow: true, ReachedHigh: true},
			traversal:  TraversalFacts{CountCacheInconsistent: true, CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := NewSnapshotMetadata(tc.fact, tc.life)
			if err != nil {
				t.Fatalf("NewSnapshotMetadata(%+v, %+v): %v", tc.fact, tc.life, err)
			}
			// The traversal facts carry no Limit field: the same set
			// applies to both the limited and unbounded interpretation,
			// so a single Classify call covers both.
			got := Classify(meta, tc.traversal)
			if got != tc.wantLabels {
				t.Errorf("Classify = %+v, want %+v (limited known total must match unbounded equivalent)", got, tc.wantLabels)
			}
		})
	}
}
```

## Focused passing history tests

```bash
go test -count=1 ./internal/history/...
```

```output
ok  	github.com/chris/sqloid/internal/history	0.186s
```

## Focused passing UI tests (finalization and export metadata)

```bash
go test -count=1 -run 'Finalization|Export|Snapshot|LimitFailure|Inconsistency' ./internal/ui/...
```

```output
ok  	github.com/chris/sqloid/internal/ui	0.117s
```

## Established verification: build, vet, and the capability-suite race command

```bash
go build ./... && go vet ./... && echo 'build and vet clean'
```

```output
build and vet clean
```

```bash
CGO_ENABLED=1 go test -race -count=1 -timeout 20m ./internal/connection ./internal/ui ./internal/history
```

```output
ok  	github.com/chris/sqloid/internal/connection	74.082s
ok  	github.com/chris/sqloid/internal/ui	4.396s
ok  	github.com/chris/sqloid/internal/history	1.682s
```

All three release-blocking capability suites pass green under the race detector. The refactor is complete: TraversalFacts, SnapshotFacts, and activeExportFacts no longer carry HasLimit or Limit; the classifier truth table compares limited known-total cases against equivalent unbounded fact sets and produces identical labels; complete, partial, truncated, endpoint, and count/cache-inconsistency outcomes are unchanged. Reference: Issue #79 and Notes/PRD-sqloid.md.
