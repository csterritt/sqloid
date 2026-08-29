// Exactly-once immutable snapshot finalization (Issue #34 Tasks 3–4), per the
// SELECT finalization and Testing Decisions in Notes/PRD-sqloid.md and the
// Issue #33 metadata model in internal/history. Every actual SELECT execution
// creates exactly one correctly typed immutable result-history entry:
//
//   - ordinary successful idle finalization: one tabular snapshot;
//   - successful rows with the count unavailable: one tabular snapshot;
//   - partial page failure after retained rows: one tabular failed snapshot;
//   - cancellation/first-page failure before rows: one non-tabular
//     Cancelled/error entry;
//   - cancellation/failure after rows: tabular snapshots preserving rows.
//
// Duplicate finalizer messages, late count/page results, repeated
// cancellation settlements, old execution IDs, repeated history-entry
// commands, and quit cleanup messages create no second entry and never mutate
// the first. Finalization is per execution, never per page or count request.

package ui

import (
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// requireEntryAt returns the single finalized entry and asserts its kind.
func requireEntryAt(t *testing.T, m Model, kind history.ResultKind, context string) history.ResultEntry {
	t.Helper()
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("%s: result history = %d entries, want exactly one", context, m.ResultHistory.Len())
	}
	entry := m.ResultHistory.Entries()[0]
	if entry.Kind != kind {
		t.Fatalf("%s: entry kind = %v, want %v", context, entry.Kind, kind)
	}
	return entry
}

// TestSnapshotFinalizedOncePerOutcome walks the defined snapshot outcomes and
// asserts each produces exactly one correctly typed immutable entry.
func TestSnapshotFinalizedOncePerOutcome(t *testing.T) {
	t.Run("ordinary successful idle finalization", func(t *testing.T) {
		m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
		m.enterResultHistory()
		entry := requireEntryAt(t, m, history.KindTabular, "idle finalization")
		if entry.ExecutionID == 0 || len(entry.Rows) == 0 {
			t.Fatalf("tabular snapshot missing identity/rows: %+v", entry)
		}
		if entry.Metadata.Outcome != history.OutcomeSuccess {
			t.Fatalf("outcome = %v, want success", entry.Metadata.Outcome)
		}
	})

	t.Run("successful rows with count unavailable", func(t *testing.T) {
		m, _, _, count := fixtureFor(t, activeState{name: "count pending", countPending: true})
		count.Result = CountResult{Err: errors.New("count failed")}
		m = apply(m, count)
		m.enterResultHistory()
		entry := requireEntryAt(t, m, history.KindTabular, "count failure with rows")
		if len(entry.Rows) != 3 {
			t.Fatalf("rows = %d, want the 3 captured rows", len(entry.Rows))
		}
		if entry.Metadata.Outcome != history.OutcomeSuccess {
			t.Fatalf("count failure with rows must stay a successful outcome, got %v", entry.Metadata.Outcome)
		}
		if entry.Completeness.Complete {
			t.Fatalf("count failure must not classify complete: %v", entry.Completeness)
		}
	})

	t.Run("partial page failure after retained rows", func(t *testing.T) {
		m, _, page, count := fixtureFor(t, activeState{name: "page pending", pagePending: true})
		m = apply(m, count)
		page.Result = FirstPageResult{Err: errors.New("page 2 failed")}
		m = apply(m, page)
		m.enterResultHistory()
		entry := requireEntryAt(t, m, history.KindTabular, "partial page failure")
		if len(entry.Rows) != 3 || entry.Metadata.Outcome != history.OutcomeFailed {
			t.Fatalf("snapshot must preserve rows with a failed outcome: rows=%d outcome=%v",
				len(entry.Rows), entry.Metadata.Outcome)
		}
		if entry.Metadata.Reason != "page 2 failed" {
			t.Fatalf("reason = %q, want the failure cause", entry.Metadata.Reason)
		}
	})

	t.Run("cancellation before rows creates Cancelled entry", func(t *testing.T) {
		m, page, _ := startActiveSelect(t)
		execID := m.ActiveSelectExecutionID()
		page.Result = FirstPageResult{Cancelled: true}
		ended := apply(m, page)
		requireFinalized(t, ended, execID, "cancelled before rows")
		entry := requireEntryAt(t, ended, history.KindCancelled, "cancelled before rows")
		if len(entry.Rows) != 0 || entry.Metadata.Outcome != history.OutcomeCancelled {
			t.Fatalf("cancelled entry must be non-tabular with cancelled outcome: %+v", entry)
		}
	})

	t.Run("first-page failure before rows creates error entry", func(t *testing.T) {
		m, page, _ := startActiveSelect(t)
		execID := m.ActiveSelectExecutionID()
		page.Result = FirstPageResult{Err: errors.New("no such table")}
		ended := apply(m, page)
		requireFinalized(t, ended, execID, "first-page failure")
		entry := requireEntryAt(t, ended, history.KindError, "first-page failure")
		if len(entry.Rows) != 0 || entry.Metadata.Outcome != history.OutcomeFailed {
			t.Fatalf("error entry must be non-tabular with failed outcome: %+v", entry)
		}
	})

	t.Run("cancellation after rows preserves captured rows", func(t *testing.T) {
		m, _, page, count := fixtureFor(t, activeState{name: "page pending", pagePending: true})
		m = apply(m, count)
		page.Result = FirstPageResult{Cancelled: true}
		m = apply(m, page)
		m.enterResultHistory()
		entry := requireEntryAt(t, m, history.KindTabular, "cancelled after rows")
		if len(entry.Rows) != 3 {
			t.Fatalf("rows = %d, want the 3 captured rows preserved", len(entry.Rows))
		}
		if entry.Metadata.Outcome != history.OutcomeCancelled {
			t.Fatalf("outcome = %v, want cancelled", entry.Metadata.Outcome)
		}
	})

	t.Run("failure after rows preserves captured rows", func(t *testing.T) {
		m, _, page, count := fixtureFor(t, activeState{name: "page pending", pagePending: true})
		m = apply(m, count)
		page.Result = FirstPageResult{Err: errors.New("disk I/O error")}
		m = apply(m, page)
		m.acceptedQuitCleanup()
		entry := requireEntryAt(t, m, history.KindTabular, "failure after rows")
		if len(entry.Rows) != 3 || entry.Metadata.Outcome != history.OutcomeFailed {
			t.Fatalf("snapshot must preserve rows with failed outcome: rows=%d outcome=%v",
				len(entry.Rows), entry.Metadata.Outcome)
		}
	})
}

// TestFinalizationEntryCarriesMetadataFromCache verifies the tabular snapshot
// copies ascending rows and Issue #33 metadata out of the authoritative
// resultcache, and stays immutable against later cache activity.
func TestFinalizationEntryCarriesMetadataFromCache(t *testing.T) {
	m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
	m.enterResultHistory()
	entry := requireEntryAt(t, m, history.KindTabular, "metadata entry")

	if entry.Metadata.HasRetainedRange && entry.Metadata.RetainedStart != 1 {
		t.Fatalf("retained start = %v, want 1", entry.Metadata.RetainedStart)
	}
	if entry.Completeness.String() == "none" {
		t.Fatal("snapshot must carry a completeness classification")
	}

	// Later cache activity must not mutate the finalized entry: merging an
	// adjacent page through the model's own cache seam cannot reach the
	// snapshot.
	mergeRowsIntoCache(t, &m, 4, 5)
	after := m.ResultHistory.Entries()[0]
	if len(after.Rows) != len(entry.Rows) || after.Metadata.RetainedEnd != entry.Metadata.RetainedEnd {
		t.Fatal("finalized entry mutated after later cache activity")
	}
	for i := range entry.Rows {
		for j := range entry.Rows[i] {
			if sameValue := valueEqual(entry.Rows[i][j], after.Rows[i][j]); !sameValue {
				t.Fatalf("retained row %d changed after finalization", i)
			}
		}
	}
}

// TestDuplicateFinalizationIsHarmless replays duplicate and late finalizer
// messages: repeated history-entry commands, old execution IDs, late
// count/page success and failure, repeated cancellation settlement, and quit
// cleanup messages create no second entry and never rewrite the first.
func TestDuplicateFinalizationIsHarmless(t *testing.T) {
	t.Run("repeated history-entry commands", func(t *testing.T) {
		m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
		m.enterResultHistory()
		first := m.ResultHistory.Entries()[0]
		m.enterResultHistory()
		m.enterResultHistory()
		if m.ResultHistory.Len() != 1 {
			t.Fatalf("repeated finalizers created %d entries", m.ResultHistory.Len())
		}
		if got := m.ResultHistory.Entries()[0]; got.ID != first.ID || got.ExecutionID != first.ExecutionID {
			t.Fatal("repeated finalizer rewrote the first entry")
		}
	})

	t.Run("late old-execution messages", func(t *testing.T) {
		m, pageFirst, _, count := fixtureFor(t, activeState{name: "idle"})
		m.enterResultHistory()
		first := m.ResultHistory.Entries()[0]

		// Late count success and failure from the old execution, plus a
		// stale first-page response from an old execution ID.
		count.Result = CountResult{Total: 99}
		m = apply(m, count)
		count.Result = CountResult{Err: errors.New("late count failure")}
		m = apply(m, count)
		pageFirst.Result = FirstPageResult{Page: &result.Page{Columns: []string{"stale"}}}
		m = apply(m, pageFirst)

		if m.ResultHistory.Len() != 1 {
			t.Fatalf("late messages created %d entries", m.ResultHistory.Len())
		}
		if got := m.ResultHistory.Entries()[0]; got.ID != first.ID || len(got.Rows) != len(first.Rows) {
			t.Fatal("late message mutated the finalized entry")
		}
	})

	t.Run("repeated cancellation settlement and quit cleanup", func(t *testing.T) {
		m, page, _ := startActiveSelect(t)
		execID := m.ActiveSelectExecutionID()
		page.Result = FirstPageResult{Cancelled: true}
		m = apply(m, page)
		requireFinalized(t, m, execID, "ending cancellation")

		// Replay the same cancelled settlement and repeat quit cleanup.
		m = apply(m, page)
		m.acceptedQuitCleanup()
		m.acceptedQuitCleanup()
		if m.ResultHistory.Len() != 1 {
			t.Fatalf("replayed finalizers created %d entries", m.ResultHistory.Len())
		}
	})

	t.Run("accepted quit with pending page and count work", func(t *testing.T) {
		m, page, _, count := fixtureFor(t, activeState{name: "count pending", countPending: true})
		m.acceptedQuitCleanup()
		entry := requireEntryAt(t, m, history.KindTabular, "quit with pending work")
		if len(entry.Rows) != 3 {
			t.Fatalf("rows = %d, want the captured rows", len(entry.Rows))
		}
		// The withheld page/count messages settle after quit: no new entry.
		m = apply(m, count)
		m = apply(m, page)
		if m.ResultHistory.Len() != 1 {
			t.Fatalf("post-quit settlements created %d entries", m.ResultHistory.Len())
		}
	})
}

// TestQuitFinalizationDistinctFromRequestCompletion proves finalization is
// per execution: a quit during idle work creates one entry, not one per
// settled request.
func TestQuitFinalizationDistinctFromRequestCompletion(t *testing.T) {
	m, page, _, count := fixtureFor(t, activeState{name: "idle"})
	// Both requests settle normally and quit finalizes once.
	m = apply(m, count)
	m = apply(m, page)
	m.acceptedQuitCleanup()
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("finalization is per request, not per execution: %d entries", m.ResultHistory.Len())
	}
}

// valueEqual compares two result values by their meaningful fields, so BLOB
// payloads (uncopyable structs) can be compared byte-exactly.
func valueEqual(a, b result.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case result.KindBlob:
		return string(a.Bytes) == string(b.Bytes)
	case result.KindText:
		return a.Str == b.Str
	default:
		return a.Int == b.Int && a.Float == b.Float
	}
}
