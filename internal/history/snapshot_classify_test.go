// Truthful completeness and endpoint classification tests for Issues #33
// and #77, per the Cache and snapshot invariant of Notes/PRD-sqloid.md:
// exclusive `complete`, coexisting `partial` and `truncated`, limited-result
// semantics (rows beyond the user's Limit are irrelevant), count failure and
// short/empty observation behavior, unknown remainder, no clamping of
// inconsistent count/cache evidence, unseen low endpoints distinguished from
// truncation (Issue #77), empty-result completion without ReachedLow, and
// terminal outcome kept as an independent axis in every matrix case.

package history

import (
	"testing"
)

// classifyCase is one matrix case: immutable snapshot facts plus traversal
// and terminal facts, and the exact expected label combination.
type classifyCase struct {
	name       string
	fact       CacheFacts
	life       Lifecycle
	traversal  TraversalFacts
	wantLabels Completeness
}

// complete is the full success shape: every limited row retained.
func completeCase(name string, total int64) classifyCase {
	return classifyCase{
		name: name,
		fact: CacheFacts{HasRetainedRange: true, Start: 1, End: Position(total)},
		life: Lifecycle{
			Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: total,
			ReachedLow: true, ReachedHigh: true,
		},
		traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
		wantLabels: Completeness{Complete: true},
	}
}

// TestClassificationMatrix walks the truth table for completeness and
// endpoint classification across known totals, count failure, limited
// results, eviction, short and empty observations, and unknown remainder.
func TestClassificationMatrix(t *testing.T) {
	cases := []classifyCase{
		// --- complete (exclusive) ---
		completeCase("complete: known total, full retention", 10),
		{
			name:       "complete: limited known total equals retained range (Issue #79: equivalent to unbounded)",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 5},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 5, ReachedLow: true, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name: "complete: limited known total above retained range, rows beyond Limit irrelevant (Issue #79: equivalent to unbounded)",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 10, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name: "complete: empty result via count success",
			fact: CacheFacts{},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 0, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name: "complete: count failed but observed empty final page establishes high",
			fact: CacheFacts{},
			life: Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name: "complete: count failed but observed short final page establishes high",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 50},
			life: Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},

		// --- Issue #77: empty completion without ReachedLow ---
		{
			name:       "complete: empty known-total result, ReachedLow false, vacuous retention",
			fact:       CacheFacts{},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 0, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name:       "complete: observed empty final page, ReachedLow false, vacuous retention",
			fact:       CacheFacts{},
			life:       Lifecycle{Outcome: OutcomeSuccess, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},

		// --- truncated (may coexist with partial) ---
		{
			name: "partial and truncated: low-side row-cap eviction with unseen low endpoint",
			fact: CacheFacts{HasRetainedRange: true, Start: 91, End: 100, RowCapEvictions: 90},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: false, ReachedHigh: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name: "truncated only: rows beyond retained range, high endpoint reached",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10, RowCapEvictions: 90},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Truncated: true},
		},
		{
			name: "truncated only: byte-cap eviction fact persists",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10, TruncatedByByteCap: true},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 10, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Truncated: true},
		},

		// --- partial ---
		{
			name: "partial only: full pages, unknown remainder, no count",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 20},
			life: Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name:       "partial only: settled nonempty range, unseen low endpoint, no eviction",
			fact:       CacheFacts{HasRetainedRange: true, Start: 11, End: 20},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 20, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name: "partial only: count failure, no short or empty observation",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 20},
			life: Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name:       "partial only: count success but paging unfinished",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: false},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name:       "partial only: count never finished",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life:       Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true},
			traversal:  TraversalFacts{PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name: "partial only: count/cache inconsistency below retained range",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 5, ReachedLow: true, ReachedHigh: true},
			traversal: TraversalFacts{CountCacheInconsistent: true, CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},

		// --- partial and truncated coexisting ---
		{
			name: "partial and truncated: count failure plus byte-cap eviction",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 20, TruncatedByByteCap: true},
			life: Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name: "partial and truncated: unknown remainder plus row-cap eviction",
			fact: CacheFacts{HasRetainedRange: true, Start: 10001, End: 10020, RowCapEvictions: 10000},
			life: Lifecycle{Outcome: OutcomeSuccess},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name: "partial and truncated: rows beyond range unseen after eviction",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10, RowCapEvictions: 2},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true},
			traversal: TraversalFacts{CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name: "partial and truncated: inconsistent count above retained range",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life: Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true},
			traversal: TraversalFacts{CountCacheInconsistent: true, CountWorkFinished: true,
				PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name:       "partial and truncated: unseen low endpoint plus row-cap eviction",
			fact:       CacheFacts{HasRetainedRange: true, Start: 11, End: 20, RowCapEvictions: 10},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 20, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name:       "partial and truncated: unseen low endpoint plus byte-cap eviction",
			fact:       CacheFacts{HasRetainedRange: true, Start: 11, End: 20, TruncatedByByteCap: true},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 20, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
		{
			name:       "partial and truncated: unseen low endpoint plus known rows beyond range",
			fact:       CacheFacts{HasRetainedRange: true, Start: 11, End: 20},
			life:       Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: false, ReachedHigh: true},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := NewSnapshotMetadata(tc.fact, tc.life)
			if err != nil {
				t.Fatalf("NewSnapshotMetadata(%+v, %+v): %v", tc.fact, tc.life, err)
			}
			got := Classify(meta, tc.traversal)
			if got != tc.wantLabels {
				t.Errorf("Classify = %+v, want %+v", got, tc.wantLabels)
			}
		})
	}
}

// TestCountUnavailableOnlyObservationEstablishesHigh pins the endpoint rule:
// with the count unavailable or failed, only an actually observed short or
// empty final page establishes the high endpoint; an unobserved remainder is
// never inferred.
func TestCountUnavailableOnlyObservationEstablishesHigh(t *testing.T) {
	countFailed := Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true}
	for _, tc := range []struct {
		name       string
		traversal  TraversalFacts
		wantLabels Completeness
	}{
		{
			name:       "no observation: remainder unknown",
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name:       "full pages at requested size: remainder unknown",
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name:       "observed short final page: high established",
			traversal:  TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name:       "observed empty final page: high established",
			traversal:  TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact := CacheFacts{HasRetainedRange: true, Start: 1, End: 20}
			if tc.traversal.ObservedShortFinalPage {
				fact = CacheFacts{HasRetainedRange: true, Start: 1, End: 20}
			}
			meta, err := NewSnapshotMetadata(fact, countFailed)
			if err != nil {
				t.Fatalf("NewSnapshotMetadata: %v", err)
			}
			if got := Classify(meta, tc.traversal); got != tc.wantLabels {
				t.Errorf("Classify = %+v, want %+v", got, tc.wantLabels)
			}
		})
	}
}

// TestNoClampingOfInconsistentEvidence preserves contradictory count/cache
// evidence: metadata values are never rewritten (no clamped rows, range,
// total, or endpoints) and classification reports partial (or partial plus
// truncated) without ever reporting complete.
func TestNoClampingOfInconsistentEvidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fact       CacheFacts
		total      int64
		wantLabels Completeness
	}{
		{
			name:       "count below retained range",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			total:      5,
			wantLabels: Completeness{Partial: true},
		},
		{
			name:       "count above retained range",
			fact:       CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			total:      100,
			wantLabels: Completeness{Partial: true, Truncated: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := NewSnapshotMetadata(tc.fact, Lifecycle{
				Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: tc.total,
				ReachedLow: true, ReachedHigh: true,
			})
			if err != nil {
				t.Fatalf("NewSnapshotMetadata: %v", err)
			}
			got := Classify(meta, TraversalFacts{
				CountCacheInconsistent: true, CountWorkFinished: true, PageWorkFinished: true,
			})
			if got != tc.wantLabels {
				t.Errorf("Classify = %+v, want %+v", got, tc.wantLabels)
			}
			// No clamping: metadata facts stand exactly as observed.
			if meta.RetainedEnd != Position(tc.fact.End) || meta.KnownTotal != tc.total {
				t.Errorf("metadata clamped: %+v", meta)
			}
		})
	}
}

// TestTerminalOutcomeIndependentOfCompleteness keeps success, cancellation,
// and failure orthogonal to the completeness labels: the same retained-range,
// endpoint, and traversal facts produce identical labels regardless of the
// terminal outcome, with cancellation and failure reasons and one-based
// failure positions carried unchanged.
func TestTerminalOutcomeIndependentOfCompleteness(t *testing.T) {
	facts := []CacheFacts{
		{HasRetainedRange: true, Start: 1, End: 4},
		{HasRetainedRange: true, Start: 5, End: 8, RowCapEvictions: 4},
		{},
	}
	traversals := []TraversalFacts{
		{CountWorkFinished: true, PageWorkFinished: true},
		{PageWorkFinished: true},
	}
	// Expected labels depend on the completeness facts and traversal state
	// only; the terminal outcome never changes them.
	wants := [][]Completeness{
		{{Partial: true}, {Partial: true}},
		{{Partial: true, Truncated: true}, {Partial: true, Truncated: true}},
		{{Partial: true}, {Partial: true}},
	}
	for ci, fact := range facts {
		for ti, traversal := range traversals {
			for _, life := range []Lifecycle{
				{Outcome: OutcomeSuccess},
				{Outcome: OutcomeCancelled, Reason: "user cancel"},
				{Outcome: OutcomeFailed, Reason: "boom", HasFailurePosition: true, FailurePosition: 3},
			} {
				meta, err := NewSnapshotMetadata(fact, life)
				if err != nil {
					t.Fatalf("NewSnapshotMetadata(%+v, %+v): %v", fact, life, err)
				}
				got := Classify(meta, traversal)
				if got != wants[ci][ti] {
					t.Errorf("fact %d traversal %d outcome %v: Classify = %+v, want %+v",
						ci, ti, life.Outcome, got, wants[ci][ti])
				}
				// Reasons and positions ride along unchanged.
				if life.Outcome == OutcomeFailed && meta.Reason != "boom" || meta.FailurePosition != life.FailurePosition {
					t.Errorf("terminal details not carried unchanged: %+v", meta)
				}
			}
		}
	}
}

// TestCancellationAndFailureAroundRows covers terminal outcomes before and
// after rows arrive: a cancellation before any rows (no retained range) and a
// failure mid-page (failure position beyond the retained end) keep the
// completeness labels truthful and independent of the terminal outcome.
func TestCancellationAndFailureAroundRows(t *testing.T) {
	cases := []classifyCase{
		{
			name:       "cancelled before any rows: no range, remainder unknown",
			fact:       CacheFacts{},
			life:       Lifecycle{Outcome: OutcomeCancelled, Reason: "cancelled before rows"},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name: "failed mid-page: failure position beyond retained end",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 20},
			life: Lifecycle{
				Outcome: OutcomeFailed, Reason: "driver failure",
				HasFailurePosition: true, FailurePosition: 21,
				ReachedLow: true,
			},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
		{
			name: "cancelled after all rows retained: labels unaffected",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
			life: Lifecycle{
				Outcome: OutcomeCancelled, Reason: "cancelled after rows",
				ReachedLow: true, ReachedHigh: true,
				HasKnownTotal: true, KnownTotal: 10,
			},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Complete: true},
		},
		{
			name: "partial page failure with count failure",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 7},
			life: Lifecycle{
				Outcome: OutcomeFailed, Reason: "page 2 failed",
				HasFailurePosition: true, FailurePosition: 8,
				ReachedLow: true,
			},
			traversal:  TraversalFacts{CountWorkFinished: true, PageWorkFinished: true},
			wantLabels: Completeness{Partial: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := NewSnapshotMetadata(tc.fact, tc.life)
			if err != nil {
				t.Fatalf("NewSnapshotMetadata(%+v, %+v): %v", tc.fact, tc.life, err)
			}
			if got := Classify(meta, tc.traversal); got != tc.wantLabels {
				t.Errorf("Classify = %+v, want %+v", got, tc.wantLabels)
			}
		})
	}
}

// TestLimitedKnownTotalEquivalentToUnbounded (Issue #79) pins the
// classification boundary after removing the dead HasLimit/Limit traversal
// inputs: a successful known total already counts the complete SELECT
// including the user's Limit, so a limited known-total case produces labels
// identical to the equivalent unbounded fact set with the same total,
// retained range, and endpoint observations. The raw builder Limit is no
// longer a traversal fact, so classification must not depend on it.
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
