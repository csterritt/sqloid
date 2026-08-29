// Stable-ID cursor navigation primitives for query history (Issue #35):
// pure older/newer/newest/oldest lookups over the Issue #20 retained list.
// These tests own the pure navigation contract — boundary no-ops, direction
// reversal, missing-ID behavior, and copy independence — while the Issue #20
// append policy (A→A suppression and A→B→A retention) stays regression-only.

package history

import (
	"fmt"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// cursorState builds a distinct normalized state per label so consecutive
// comparison can never collapse entries.
func cursorState(label string) qb.HistoryState {
	state := qb.HistoryState{Command: qb.CommandSelect, Table: label, TableSet: true}
	state.Projection = []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}}
	return state
}

// cursorStore returns a store holding three distinct entries A (oldest), B,
// C (newest) with their stable IDs.
func cursorStore() (*Store, [3]EntryID) {
	s := NewStore()
	ids := [3]EntryID{s.Append(cursorState("a")), s.Append(cursorState("b")), s.Append(cursorState("c"))}
	return s, ids
}

func TestNewestAndOldestOnEmptyStore(t *testing.T) {
	s := NewStore()
	if _, ok := s.Newest(); ok {
		t.Fatal("Newest reported an entry on an empty store")
	}
	if _, ok := s.Oldest(); ok {
		t.Fatal("Oldest reported an entry on an empty store")
	}
	if _, ok := s.OlderThan(7); ok {
		t.Fatal("OlderThan reported an entry on an empty store")
	}
	if _, ok := s.NewerThan(7); ok {
		t.Fatal("NewerThan reported an entry on an empty store")
	}
}

func TestOldestAndNewestIdentifyRetainedEntries(t *testing.T) {
	s, ids := cursorStore()
	oldest, ok := s.Oldest()
	if !ok || oldest.ID != ids[0] {
		t.Fatalf("Oldest = (%v, %v); want ID %d", oldest.ID, ok, ids[0])
	}
	newest, ok := s.Newest()
	if !ok || newest.ID != ids[2] {
		t.Fatalf("Newest = (%v, %v); want ID %d", newest.ID, ok, ids[2])
	}
	if oldest.State.Table != "a" || newest.State.Table != "c" {
		t.Fatalf("retained states = %q/%q; want a/c", oldest.State.Table, newest.State.Table)
	}
}

func TestOlderThanAndNewerThanNavigateByStableID(t *testing.T) {
	s, ids := cursorStore()
	if e, ok := s.OlderThan(ids[2]); !ok || e.ID != ids[1] {
		t.Fatalf("OlderThan(newest) = (%v, %v); want ID %d", e.ID, ok, ids[1])
	}
	if e, ok := s.NewerThan(ids[0]); !ok || e.ID != ids[1] {
		t.Fatalf("NewerThan(oldest) = (%v, %v); want ID %d", e.ID, ok, ids[1])
	}
	if e, ok := s.OlderThan(ids[1]); !ok || e.ID != ids[0] {
		t.Fatalf("OlderThan(middle) = (%v, %v); want ID %d", e.ID, ok, ids[0])
	}
	if e, ok := s.NewerThan(ids[1]); !ok || e.ID != ids[2] {
		t.Fatalf("NewerThan(middle) = (%v, %v); want ID %d", e.ID, ok, ids[2])
	}
}

func TestOlderAndNewerBoundaryNoOps(t *testing.T) {
	s, ids := cursorStore()
	if _, ok := s.OlderThan(ids[0]); ok {
		t.Fatal("OlderThan(oldest) moved past the oldest boundary")
	}
	if _, ok := s.NewerThan(ids[2]); ok {
		t.Fatal("NewerThan(newest) moved past the newest boundary")
	}
}

func TestOlderAndNewerMissingStableID(t *testing.T) {
	s, _ := cursorStore()
	if _, ok := s.OlderThan(0); ok {
		t.Fatal("OlderThan(0) — the never-allocated none ID — moved")
	}
	if _, ok := s.OlderThan(9999); ok {
		t.Fatal("OlderThan(unallocated) moved")
	}
	if _, ok := s.NewerThan(9999); ok {
		t.Fatal("NewerThan(unallocated) moved")
	}
}

func TestEvictedStableIDCannotNavigate(t *testing.T) {
	s, ids := cursorStore()
	// Fill to Capacity and append one more: A (ids[0]) is evicted oldest-first.
	for i := 3; i < Capacity; i++ {
		s.Append(cursorState(fmt.Sprintf("x%d", i)))
	}
	if s.Len() != Capacity {
		t.Fatalf("Len = %d before eviction append, want %d", s.Len(), Capacity)
	}
	s.Append(cursorState("z"))
	if _, ok := s.Lookup(ids[0]); ok {
		t.Fatal("setup: oldest entry was not evicted")
	}
	if _, ok := s.OlderThan(ids[1]); ok {
		t.Fatal("OlderThan(B) navigated onto a non-retained (evicted) identity")
	}
	if _, ok := s.NewerThan(ids[0]); ok {
		t.Fatal("NewerThan(A) reached an evicted identity")
	}
	// B and C survive with unchanged IDs; B is now the oldest.
	if _, ok := s.Lookup(ids[1]); !ok {
		t.Fatal("surviving ID B vanished after eviction")
	}
	if _, ok := s.Lookup(ids[2]); !ok {
		t.Fatal("surviving ID C vanished after eviction")
	}
	if _, ok := s.OlderThan(ids[2]); !ok || func() bool { e, _ := s.OlderThan(ids[2]); return e.ID != ids[1] }() {
		t.Fatal("OlderThan(C) no longer reaches surviving B")
	}
}

func TestNavigationReturnsIndependentCopies(t *testing.T) {
	s, ids := cursorStore()
	e, ok := s.OlderThan(ids[2])
	if !ok {
		t.Fatal("setup: navigation failed")
	}
	// Mutate the retrieved copy, the retained source through Entries, and the
	// copy's slices; the retained entry must not change.
	e.State.Table = "mutated"
	e.State.Groups = append(e.State.Groups, "injected")
	e.State.Projection[0] = qb.HistoryProjectionEntry{}
	fresh, ok := s.Lookup(ids[1])
	if !ok {
		t.Fatal("setup: lookup failed")
	}
	if fresh.State.Table != "b" || len(fresh.State.Groups) != 0 || fresh.State.Projection[0].Kind != qb.ProjectionWildcard {
		t.Fatalf("retained state changed through a retrieved copy: %v", fresh.State)
	}
}

func TestNavigationNeverAppendsOrAllocatesIDs(t *testing.T) {
	s, ids := cursorStore()
	before := s.nextID
	for _, id := range ids {
		s.OlderThan(id)
		s.NewerThan(id)
		s.Lookup(id)
	}
	s.Newest()
	s.Oldest()
	if s.Len() != 3 {
		t.Fatalf("Len = %d after browsing, want 3 (browsing appends nothing)", s.Len())
	}
	if s.nextID != before {
		t.Fatalf("nextID = %d after browsing, want unchanged %d", s.nextID, before)
	}
}

// Regression: the Issue #20 consecutive policy is unchanged by navigation.
func TestAppendPolicyRegressionSuppressionAndABA(t *testing.T) {
	s := NewStore()
	a := cursorState("a")
	if id, ok := s.AppendExecution(a); !ok || id == 0 {
		t.Fatal("first A execution must append")
	}
	if _, ok := s.AppendExecution(a); ok {
		t.Fatal("A→A must be suppressed with no ID allocated")
	}
	b := cursorState("b")
	if _, ok := s.AppendExecution(b); !ok {
		t.Fatal("A→B must append")
	}
	if id, ok := s.AppendExecution(a); !ok || id == 0 {
		t.Fatal("A→B→A must retain the second A entry")
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3 for A→B→A retention", s.Len())
	}
}
