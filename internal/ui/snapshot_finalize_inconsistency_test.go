// UI finalization cases for Issue #78: the model derives count/cache
// inconsistency from a successful limited-result count total below the
// authoritative retained cache end at finalization, preserves both facts
// without clamping either, and never classifies the snapshot complete. Count
// and cache are independent autocommit facts: a successful count whose total
// falls below the retained end contradicts the cache, so finalization records
// the contradiction and lets the corrected history.Classify behavior from
// Issue #77 reject complete naturally. Equality/lower retained ends and
// pending/unavailable/failed/cancelled counts do not set the flag.

package ui

import (
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// finalizeContradictionModel builds an active SELECT over a fresh result
// history with the given count state and a seeded cache, ready to finalize
// through the production seam. The active SELECT identity is set so
// enterResultHistory drives appendFinalizedResultEntry exactly once.
func finalizeContradictionModel(t *testing.T, count result.CountState, seed func(t *testing.T, m *Model)) Model {
	t.Helper()
	m := New()
	m.selectActive = true
	m.activeExecID = 1
	m.countState = count
	if seed != nil {
		seed(t, &m)
	}
	return m
}

// finalizeAndInspect drives finalization through the production seam and
// returns the single stored entry plus the model after finalization.
func finalizeAndInspect(t *testing.T, m Model) (history.ResultEntry, Model) {
	t.Helper()
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want exactly one", len(entries))
	}
	return entries[0], m
}

// exportSelectionFor selects the finalized entry in result-history mode and
// resolves its export selection, so tests can inspect the metadata and
// completeness the export path observes for the finalized snapshot.
func exportSelectionFor(t *testing.T, m Model, id history.EntryID) exportSelection {
	t.Helper()
	m.resultHistoryMode = true
	m.resultHistoryCursorID = id
	return m.exportSelection()
}

// TestFinalizationDerivesCountCacheInconsistent walks the count/cache boundary
// over a successful count and a retained cache range: only a retained end
// greater than the successful total propagates CountCacheInconsistent, both
// facts are preserved without clamping, and completeness is never complete.
// Equal and lower retained ends keep the flag clear.
func TestFinalizationDerivesCountCacheInconsistent(t *testing.T) {
	cases := []struct {
		name       string
		count      result.CountState
		seed       func(t *testing.T, m *Model)
		wantFlag   bool
		wantLabels history.Completeness
		wantTotal  int64
		wantStart  history.Position
		wantEnd    history.Position
	}{
		{
			// Successful count total 5 below the retained end 10: the cache
			// contradicts the count. Both are preserved unclamped and the
			// snapshot is partial, never complete.
			name:       "retained end greater than successful count total",
			count:      result.CountState{Status: result.CountSuccess, Total: 5},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantFlag:   true,
			wantLabels: history.Completeness{Partial: true},
			wantTotal:  5,
			wantStart:  1,
			wantEnd:    10,
		},
		{
			// Retained end equal to the successful total: no contradiction.
			// The full limited result is retained and work finished, so the
			// snapshot is complete.
			name:       "retained end equal to successful count total",
			count:      result.CountState{Status: result.CountSuccess, Total: 10},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantFlag:   false,
			wantLabels: history.Completeness{Complete: true},
			wantTotal:  10,
			wantStart:  1,
			wantEnd:    10,
		},
		{
			// Retained end below the successful total: the count reports more
			// rows than the cache retained. This is the ordinary unknown
			// remainder, not a count/cache contradiction: the flag stays clear
			// and the snapshot is partial+truncated.
			name:       "retained end less than successful count total",
			count:      result.CountState{Status: result.CountSuccess, Total: 15},
			seed:       func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) },
			wantFlag:   false,
			wantLabels: history.Completeness{Partial: true, Truncated: true},
			wantTotal:  15,
			wantStart:  1,
			wantEnd:    10,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := finalizeContradictionModel(t, tc.count, tc.seed)
			entry, finalized := finalizeAndInspect(t, m)

			if entry.Metadata.HasKnownTotal != (tc.count.Status == result.CountSuccess) {
				t.Errorf("HasKnownTotal = %v, want %v", entry.Metadata.HasKnownTotal, tc.count.Status == result.CountSuccess)
			}
			if entry.Metadata.KnownTotal != tc.wantTotal {
				t.Errorf("KnownTotal = %d, want %d (must not be clamped)", entry.Metadata.KnownTotal, tc.wantTotal)
			}
			if !entry.Metadata.HasRetainedRange {
				t.Fatal("finalized metadata lost the retained range")
			}
			if entry.Metadata.RetainedStart != tc.wantStart {
				t.Errorf("RetainedStart = %d, want %d (must not be clamped)", entry.Metadata.RetainedStart, tc.wantStart)
			}
			if entry.Metadata.RetainedEnd != tc.wantEnd {
				t.Errorf("RetainedEnd = %d, want %d (must not be clamped)", entry.Metadata.RetainedEnd, tc.wantEnd)
			}

			// The contradiction flag is observable through completeness: a
			// successful-count contradiction prevents a complete label. The
			// flag itself is a traversal fact, so completeness is the
			// observable contract; the wantLabels encode the flag's effect.
			if got := entry.Completeness; got != tc.wantLabels {
				t.Errorf("Completeness = %v, want %v", got, tc.wantLabels)
			}
			if tc.wantFlag && entry.Completeness.Complete {
				t.Errorf("contradiction classified complete; the snapshot must never be complete when the count contradicts the cache")
			}

			// The export selection over the finalized entry carries the same
			// metadata and completeness, so the contradiction's effect reaches
			// export without inventing the flag for non-contradictory cases.
			sel := exportSelectionFor(t, finalized, entry.ID)
			if !sel.tabular {
				t.Fatal("export selection lost the tabular finalized entry")
			}
			if sel.meta.KnownTotal != tc.wantTotal || sel.meta.RetainedStart != tc.wantStart || sel.meta.RetainedEnd != tc.wantEnd {
				t.Errorf("export metadata clamped facts: total=%d start=%d end=%d", sel.meta.KnownTotal, sel.meta.RetainedStart, sel.meta.RetainedEnd)
			}
			if sel.comp != tc.wantLabels {
				t.Errorf("export completeness = %v, want %v", sel.comp, tc.wantLabels)
			}
		})
	}
}

// TestFinalizationNoFlagForNonSuccessCounts walks the non-success count
// controls over the same cache range as the greater-than contradiction case
// (retained 1..10): pending, unavailable, failed-outcome, and cancelled-count
// controls never set the successful-count contradiction flag.
func TestFinalizationNoFlagForNonSuccessCounts(t *testing.T) {
	cacheRange := func(t *testing.T, m *Model) { mergeRowsIntoCache(t, m, 1, 10) }
	cases := []struct {
		name   string
		count  result.CountState
		ending func(m *Model)
	}{
		{
			name:  "pending count",
			count: result.CountState{Status: result.CountPending},
		},
		{
			name:  "unavailable count",
			count: result.CountState{Status: result.CountUnavailable},
		},
		{
			// A failed terminal outcome over an unavailable count: the
			// terminal outcome is an independent axis and never invents a
			// successful-count contradiction.
			name:   "failed outcome with unavailable count",
			count:  result.CountState{Status: result.CountUnavailable},
			ending: func(m *Model) { m.pendingFailure = &selectFailure{reason: "disk I/O error"} },
		},
		{
			// A cancelled count stays pending (the guard rejects it) and the
			// cancelled terminal outcome never invents a contradiction.
			name:   "cancelled outcome with pending count",
			count:  result.CountState{Status: result.CountPending},
			ending: func(m *Model) { m.pendingCancelReason = "cancelled" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := finalizeContradictionModel(t, tc.count, cacheRange)
			if tc.ending != nil {
				tc.ending(&m)
			}
			entry, _ := finalizeAndInspect(t, m)

			// No successful count means no known total and no contradiction.
			if entry.Metadata.HasKnownTotal {
				t.Errorf("non-success count produced a known total: %d", entry.Metadata.KnownTotal)
			}
			if entry.Completeness.Complete {
				t.Errorf("non-success count classified complete; a successful-count contradiction requires a successful count")
			}
			// The retained range is preserved unclamped regardless of count.
			if entry.Metadata.RetainedStart != 1 || entry.Metadata.RetainedEnd != 10 {
				t.Errorf("retained range = %d..%d, want 1..10 (preserved without clamping)", entry.Metadata.RetainedStart, entry.Metadata.RetainedEnd)
			}
		})
	}
}

// TestFinalizationEmptyCachePreservesSuccessfulCount proves an empty cache
// with a successful count preserves the known total without a contradiction
// flag: there is no retained end to contradict the count.
func TestFinalizationEmptyCachePreservesSuccessfulCount(t *testing.T) {
	m := finalizeContradictionModel(t, result.CountState{Status: result.CountSuccess, Total: 5}, nil)
	entry, _ := finalizeAndInspect(t, m)

	if !entry.Metadata.HasKnownTotal || entry.Metadata.KnownTotal != 5 {
		t.Errorf("KnownTotal = %d (HasKnownTotal=%v), want 5 preserved", entry.Metadata.KnownTotal, entry.Metadata.HasKnownTotal)
	}
	if entry.Metadata.HasRetainedRange {
		t.Errorf("empty cache produced a retained range: %+v", entry.Metadata)
	}
	if entry.Completeness.Complete {
		t.Errorf("empty cache with a nonzero successful count classified complete; the limited result is not retained")
	}
}

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
