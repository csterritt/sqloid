// UI finalization cases for Issue #33: the model converts its authoritative
// cache and count state, plus the paging seam's terminal and observation
// inputs, into immutable history snapshot metadata and traversal facts, then
// classifies them truthfully. Terminal outcome stays an independent axis.

package ui

import (
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// mergeRowsIntoCache merges one ascending page at start into the model's
// viewport cache through the ordinary merge seam.
func mergeRowsIntoCache(t *testing.T, m *Model, start int64, n int) {
	t.Helper()
	page := resultcache.Page{Start: resultcache.Position(start)}
	for i := 0; i < n; i++ {
		page.Rows = append(page.Rows, resultcache.Row{
			Position: resultcache.Position(start + int64(i)),
			Values:   []result.Value{result.NewInteger(start + int64(i))},
		})
	}
	ok, err := m.viewportCacheOf().Merge(page, resultcache.Forward)
	if !ok || err != nil {
		t.Fatalf("merge page at %d: (%v, %v)", start, ok, err)
	}
}

// TestFinalizationClassifiesModelFacts walks finalization cases over the
// model: complete snapshots with known totals, count failure with a short
// observation, count/cache inconsistency preserved without clamping, and
// eviction facts carried from the cache.
func TestFinalizationClassifiesModelFacts(t *testing.T) {
	cases := []struct {
		name       string
		count      result.CountState
		final      Finalization
		seed       func(t *testing.T, m *Model)
		wantLabels history.Completeness
	}{
		{
			name:  "complete with known total",
			count: result.CountState{Status: result.CountSuccess, Total: 10},
			final: Finalization{
				Outcome:    history.OutcomeSuccess,
				ReachedLow: true, ReachedHigh: true,
				CountWorkFinished: true, PageWorkFinished: true,
			},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantLabels: history.Completeness{Complete: true},
		},
		{
			name:  "count failed, short final page observed",
			count: result.CountState{Status: result.CountUnavailable},
			final: Finalization{
				Outcome:    history.OutcomeSuccess,
				ReachedLow: true, ReachedHigh: true,
				CountWorkFinished: true, PageWorkFinished: true,
				ObservedShortFinalPage: true,
			},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 20) },
			wantLabels: history.Completeness{Complete: true},
		},
		{
			name:  "count failed, remainder unknown",
			count: result.CountState{Status: result.CountUnavailable},
			final: Finalization{
				Outcome:           history.OutcomeSuccess,
				ReachedLow:        true,
				CountWorkFinished: true, PageWorkFinished: true,
			},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 20) },
			wantLabels: history.Completeness{Partial: true},
		},
		{
			name:  "inconsistent count preserved without clamping",
			count: result.CountState{Status: result.CountSuccess, Total: 5},
			final: Finalization{
				Outcome:    history.OutcomeSuccess,
				ReachedLow: true, ReachedHigh: true,
				CountWorkFinished: true, PageWorkFinished: true,
				CountCacheInconsistent: true,
			},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantLabels: history.Completeness{Partial: true},
		},
		{
			name:  "terminal outcome independent of completeness",
			count: result.CountState{Status: result.CountSuccess, Total: 10},
			final: Finalization{
				Outcome: history.OutcomeFailed, Reason: "driver failure",
				HasFailurePosition: true, FailurePosition: 11,
				ReachedLow: true, ReachedHigh: true,
				CountWorkFinished: true, PageWorkFinished: true,
			},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantLabels: history.Completeness{Complete: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			tc.seed(t, &m)
			m.countState = tc.count
			meta, traversal, err := m.SnapshotFacts(tc.final)
			if err != nil {
				t.Fatalf("SnapshotFacts: %v", err)
			}
			if meta.Outcome != tc.final.Outcome || meta.Reason != tc.final.Reason {
				t.Errorf("terminal facts not carried: %+v", meta)
			}
			if meta.HasKnownTotal != (tc.count.Status == result.CountSuccess) ||
				(tc.count.Status == result.CountSuccess && meta.KnownTotal != tc.count.Total) {
				t.Errorf("known total wrong: %+v", meta)
			}
			if got := history.Classify(meta, traversal); got != tc.wantLabels {
				t.Errorf("Classify = %v, want %v", got, tc.wantLabels)
			}
		})
	}
}

// TestFinalizationMetadataIndependentOfLaterNavigation proves finalized
// metadata cannot change: further merges, eviction, and count settlement
// after finalization leave the already-finalized value untouched.
func TestFinalizationMetadataIndependentOfLaterNavigation(t *testing.T) {
	m := New()
	mergeRowsIntoCache(t, &m, 1, 10)
	m.countState = result.CountState{Status: result.CountSuccess, Total: 10}
	meta, _, err := m.SnapshotFacts(Finalization{
		Outcome:    history.OutcomeSuccess,
		ReachedLow: true, ReachedHigh: true,
		CountWorkFinished: true, PageWorkFinished: true,
	})
	if err != nil {
		t.Fatalf("SnapshotFacts: %v", err)
	}
	// Later activity: more rows (evicting the low end), a failed count.
	mergeRowsIntoCache(t, &m, 11, 10)
	mergeRowsIntoCache(t, &m, 21, 10)
	m.countState = result.CountState{Status: result.CountUnavailable}
	if meta.RetainedStart != 1 || meta.RetainedEnd != 10 || !meta.HasKnownTotal || meta.KnownTotal != 10 {
		t.Errorf("finalized metadata changed after later navigation: %+v", meta)
	}
	if got := history.Classify(meta, history.TraversalFacts{CountWorkFinished: true, PageWorkFinished: true}); got != (history.Completeness{Complete: true}) {
		t.Errorf("finalized classification changed: %v", got)
	}
}

// TestFinalizationEmptyFirstPageCompleteWithCountUnavailable proves that
// an empty observed first page with count unavailable classifies complete
// in the finalized path: both endpoints are established at position 0 and
// ObservedShortFinalPage feeds the high endpoint (Issue #73 AC3).
func TestFinalizationEmptyFirstPageCompleteWithCountUnavailable(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(0)}
	count := &fakeCountExecutor{err: errors.New("count failed")}
	m := firstSelectModel(exec)
	m.Count = count.count
	m.ResultHistory = history.NewResultStore()
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	if !m.pageExhausted {
		t.Fatal("empty first page did not set pageExhausted")
	}
	// Finalize through the real seam (entering result history).
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	if !entries[0].Completeness.Complete {
		t.Errorf("empty first page finalized completeness = %v, want Complete", entries[0].Completeness)
	}
}

// TestFinalizationShortFirstPageCompleteWithCountUnavailable proves that
// a short nonempty observed first page with count unavailable classifies
// complete in the finalized path: the retained range establishes both
// endpoints and ObservedShortFinalPage feeds the high endpoint
// (Issue #73 AC3).
func TestFinalizationShortFirstPageCompleteWithCountUnavailable(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(3)}
	count := &fakeCountExecutor{err: errors.New("count failed")}
	m := firstSelectModel(exec)
	m.Count = count.count
	m.ResultHistory = history.NewResultStore()
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	if !m.pageExhausted {
		t.Fatal("short first page did not set pageExhausted")
	}
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	if !entries[0].Completeness.Complete {
		t.Errorf("short first page finalized completeness = %v, want Complete", entries[0].Completeness)
	}
}
