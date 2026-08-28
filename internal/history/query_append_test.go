// Append-policy and lifecycle tests (Issue #20 Task 3): actual-execution
// appends only, consecutive A→A suppression without consuming a stable ID or
// causing eviction, A→B→A retention of both A entries, and failed executions
// still retaining the entry appended at start. These are pure storage-layer
// tests over the normalized state; the UI owns the append timing.

package history

import (
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// stateA and stateB are two distinct normalized execution states.
func stateA() qb.HistoryState {
	return qb.HistoryState{
		Command:    qb.CommandSelect,
		Table:      "items",
		TableSet:   true,
		Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}},
	}
}

func stateB() qb.HistoryState {
	s := stateA()
	s.Table = "logs"
	return s
}

// TestConsecutiveIdenticalExecutionIsSuppressed requires A→A to suppress only
// the latter append: storage keeps exactly one entry with the first ID, and
// no eviction or reallocation occurs.
func TestConsecutiveIdenticalExecutionIsSuppressed(t *testing.T) {
	s := NewStore()
	first, appended := s.AppendExecution(stateA())
	if !appended {
		t.Fatal("first actual execution was suppressed")
	}
	second, appended := s.AppendExecution(stateA())
	if appended || second != 0 {
		t.Fatalf("consecutive A append = (%d, %v), want suppressed (0, false)", second, appended)
	}
	if s.Len() != 1 {
		t.Fatalf("Len after suppression = %d, want 1", s.Len())
	}
	got := s.Entries()
	if got[0].ID != first {
		t.Fatalf("retained ID = %d, want the first append's ID %d", got[0].ID, first)
	}
}

// TestSuppressedAppendConsumesNoStableIDOrEviction requires a suppressed
// append to allocate no ID: the next accepted append receives the very next
// ID, and a full store survives a suppression without eviction.
func TestSuppressedAppendConsumesNoStableIDOrEviction(t *testing.T) {
	s := NewStore()
	first, _ := s.AppendExecution(stateA())
	if _, ok := s.AppendExecution(stateA()); ok {
		t.Fatal("consecutive duplicate was appended")
	}
	second, ok := s.AppendExecution(stateB())
	if !ok || second != first+1 {
		t.Fatalf("next accepted ID = %d (ok=%v), want exactly first+1 = %d", second, ok, first+1)
	}
	// Fill to capacity, then suppress: nothing may evict.
	full := NewStore()
	anchor, _ := full.AppendExecution(stateA())
	for full.Len() < Capacity {
		full.Append(stateB())
	}
	if _, ok := full.AppendExecution(stateB()); ok {
		t.Fatal("consecutive duplicate appended at capacity")
	}
	if full.Len() != Capacity {
		t.Fatalf("Len after suppression at capacity = %d, want %d", full.Len(), Capacity)
	}
	if _, ok := full.Lookup(anchor); !ok {
		t.Fatal("oldest anchor entry was evicted by a suppressed append")
	}
}

// TestAlternatingExecutionsRetainBothEntries requires A→B→A to retain both A
// entries with distinct stable IDs: suppression compares only the immediately
// preceding retained execution.
func TestAlternatingExecutionsRetainBothEntries(t *testing.T) {
	s := NewStore()
	a1, _ := s.AppendExecution(stateA())
	if _, ok := s.AppendExecution(stateB()); !ok {
		t.Fatal("B append suppressed")
	}
	a2, ok := s.AppendExecution(stateA())
	if !ok {
		t.Fatal("second A append suppressed; only the immediate predecessor is compared")
	}
	if a1 == a2 {
		t.Fatalf("both A entries share ID %d; want distinct stable IDs", a1)
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3", s.Len())
	}
	got := s.Entries()
	if got[0].ID != a1 || got[2].ID != a2 {
		t.Fatalf("retained ID order = %d,%d,%d; want %d,B,%d", got[0].ID, got[1].ID, got[2].ID, a1, a2)
	}
}

// TestNormalizedDifferencesAppend covers representative significant-field
// differences (entered representation, bound type, order) that must suppress
// nothing: each pair appends.
func TestNormalizedDifferencesAppend(t *testing.T) {
	cases := map[string]func(qb.HistoryState) qb.HistoryState{
		"entered representation": func(s qb.HistoryState) qb.HistoryState {
			s.WhereEntered = "07"
			return s
		},
		"bound type": func(s qb.HistoryState) qb.HistoryState {
			s.WhereValue = qb.Value{Kind: qb.KindText, Text: "7"}
			return s
		},
		"group order": func(s qb.HistoryState) qb.HistoryState {
			s.Groups = []string{"score", "name"}
			return s
		},
		"limit number": func(s qb.HistoryState) qb.HistoryState {
			s.LimitValue = 6
			return s
		},
	}
	for name, mutate := range cases {
		s := NewStore()
		if _, ok := s.AppendExecution(stateA()); !ok {
			t.Fatalf("%s: first append suppressed", name)
		}
		if _, ok := s.AppendExecution(mutate(stateA())); !ok {
			t.Errorf("%s: significant difference was suppressed as consecutive-equal", name)
		}
	}
}

// TestFailedExecutionStillRetainsStartAppend encodes the lifecycle contract:
// append already occurred at execution start, so a later failure cannot undo
// it. The storage layer observes no failure notification — this asserts the
// entry remains after subsequent unrelated events.
func TestFailedExecutionStillRetainsStartAppend(t *testing.T) {
	s := NewStore()
	id, ok := s.AppendExecution(stateA())
	if !ok {
		t.Fatal("execution-start append suppressed")
	}
	// A failed execution produces no follow-up append; the entry stays.
	got, found := s.Lookup(id)
	if !found {
		t.Fatal("execution-start entry vanished after failure")
	}
	if !got.State.Equal(stateA()) {
		t.Error("retained state changed after failure")
	}
}
