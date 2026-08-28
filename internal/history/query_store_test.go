// Pure table-driven storage tests for the minimal in-memory query-history
// store (Issue #20 Task 1): stable nonzero identities that are not slice
// positions and never change on eviction, immutable deep-copied complete
// states, exact 20-entry capacity with oldest-first eviction, chronological
// order, lookup by stable ID, repeated payloads, empty storage, and defensive
// copy behavior. No navigation, cursors, restoration, or append policy here.

package history

import (
	"reflect"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// sampleState returns one distinct history state per distinct name so table
// cases can build identifiable appends.
func sampleState(name string) qb.HistoryState {
	return qb.HistoryState{
		Command:    qb.CommandSelect,
		Table:      name,
		TableSet:   true,
		Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionColumn, Column: "id"}},
	}
}

// richState carries every mutable slice populated, to exercise deep copying.
func richState() qb.HistoryState {
	return qb.HistoryState{
		Command:  qb.CommandUpdate,
		Table:    "items",
		TableSet: true,
		Projection: []qb.HistoryProjectionEntry{
			{Kind: qb.ProjectionColumn, Column: "id"},
			{Kind: qb.ProjectionCountStar},
		},
		WhereSet:       true,
		WhereColumn:    "name",
		WhereOperator:  qb.OpEq,
		WhereHasValue:  true,
		WhereValue:     qb.Value{Kind: qb.KindInteger, Int: 7},
		WhereEntered:   "7",
		Groups:         []string{"name", "score"},
		OrderSet:       true,
		OrderDirection: qb.DirDesc,
		LimitHas:       true,
		LimitValue:     5,
		Sets: []qb.HistorySetAssignment{
			{Column: "name", Choice: qb.SetChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindText, Text: "x"}},
			{Column: "score", Choice: qb.SetChoiceNull},
		},
		Inserts: []qb.HistoryInsertColumn{
			{Column: "id", Choice: qb.InsertChoiceOmit},
			{Column: "name", Choice: qb.InsertChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindText, Text: "y"}},
		},
	}
}

// TestEmptyStoreHasDeterministicEmptyBehavior requires a fresh store to
// report zero length, an empty chronological list, and not-found lookups.
func TestEmptyStoreHasDeterministicEmptyBehavior(t *testing.T) {
	s := NewStore()
	if s.Len() != 0 {
		t.Fatalf("fresh store Len = %d, want 0", s.Len())
	}
	if got := s.Entries(); len(got) != 0 {
		t.Fatalf("fresh store Entries = %v, want empty", got)
	}
	if _, ok := s.Lookup(1); ok {
		t.Fatal("fresh store Lookup(1) found an entry")
	}
}

// TestAppendAssignsStableNonPositionalIDs requires every retained append to
// receive a stable nonzero identity that is not its slice position and never
// changes as older entries are evicted.
func TestAppendAssignsStableNonPositionalIDs(t *testing.T) {
	s := NewStore()
	ids := make([]EntryID, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		id := s.Append(sampleState(name))
		if id == 0 {
			t.Fatalf("Append(%q) returned zero ID", name)
		}
		if EntryID(s.Len()-1) == id && s.Len() > 1 {
			t.Errorf("Append(%q) ID %d equals its slice position", name, id)
		}
		ids = append(ids, id)
	}
	// Push well past capacity so every original entry would be evicted; a
	// surviving ID must be its original value, never renumbered.
	for i := 0; i < 18; i++ {
		s.Append(sampleState("bulk"))
	}
	if got, ok := s.Lookup(ids[2]); !ok || got.ID != ids[2] {
		t.Fatalf("Lookup of evicted-context ID: got (%v, %v), want ID %d", got, ok, ids[2])
	}
}

// TestEntriesPreserveChronologicalOrder requires the returned list to order
// entries oldest first, addressing them by stable ID.
func TestEntriesPreserveChronologicalOrder(t *testing.T) {
	s := NewStore()
	var want []EntryID
	for _, name := range []string{"a", "b", "c", "d"} {
		want = append(want, s.Append(sampleState(name)))
	}
	got := s.Entries()
	if len(got) != len(want) {
		t.Fatalf("Entries length = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("Entries[%d].ID = %d, want %d (chronological order)", i, got[i].ID, id)
		}
	}
}

// TestLookupAddressesEntriesByStableID requires lookup by ID to return the
// exact retained entry regardless of its position, and unknown/never-used IDs
// to report not-found.
func TestLookupAddressesEntriesByStableID(t *testing.T) {
	s := NewStore()
	first := s.Append(sampleState("a"))
	second := s.Append(sampleState("b"))
	got, ok := s.Lookup(second)
	if !ok || got.State.Table != "b" {
		t.Fatalf("Lookup(second) = (%v, %v), want table b", got, ok)
	}
	got, ok = s.Lookup(first)
	if !ok || got.State.Table != "a" {
		t.Fatalf("Lookup(first) = (%v, %v), want table a", got, ok)
	}
	if _, ok := s.Lookup(0); ok {
		t.Error("Lookup(0) found an entry; zero is never allocated")
	}
	if _, ok := s.Lookup(second + 999); ok {
		t.Error("Lookup of a never-allocated ID found an entry")
	}
}

// TestCapacityIsExactlyTwentyWithOldestFirstEviction requires capacity to be
// exactly 20: the first 20 retained entries stay ordered, and each subsequent
// retained append evicts exactly the oldest before exposing the new list
// while preserving all surviving IDs.
func TestCapacityIsExactlyTwentyWithOldestFirstEviction(t *testing.T) {
	if Capacity != 20 {
		t.Fatalf("Capacity = %d, want exactly 20", Capacity)
	}
	s := NewStore()
	var want []EntryID
	for i := 0; i < 20; i++ {
		want = append(want, s.Append(sampleState(string(rune('a'+i)))))
	}
	if s.Len() != 20 {
		t.Fatalf("Len after 20 appends = %d, want 20", s.Len())
	}
	for i, id := range want {
		if e, ok := s.Lookup(id); !ok || e.State.Table != string(rune('a'+i)) {
			t.Errorf("entry %d missing or wrong after fill: (%v, %v)", i, e, ok)
		}
	}
	for i := 0; i < 5; i++ {
		id := s.Append(sampleState(string(rune('A' + i)))) // subsequent append evicts oldest
		want = append(want, id)
		want = want[1:]
		if s.Len() != 20 {
			t.Fatalf("Len after eviction append %d = %d, want 20", i, s.Len())
		}
		if _, ok := s.Lookup(want[0] - 1); ok {
			t.Errorf("append %d: evicted ID %d still retained", i, want[0]-1)
		}
		got := s.Entries()
		for j, id := range want {
			if got[j].ID != id {
				t.Fatalf("after eviction append %d: Entries[%d].ID = %d, want surviving ID %d", i, j, got[j].ID, id)
			}
		}
	}
}

// TestRepeatedPayloadsRetainDistinctEntries requires identical payloads at
// the storage layer to each receive their own stable identity (suppression
// belongs to the append policy, not to Append).
func TestRepeatedPayloadsRetainDistinctEntries(t *testing.T) {
	s := NewStore()
	state := sampleState("a")
	first := s.Append(state)
	second := s.Append(state)
	if first == second || second == 0 {
		t.Fatalf("repeated payload IDs = %d, %d; want distinct nonzero", first, second)
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
}

// TestAppendStoresImmutableCompleteState requires a complete immutable copy
// of the execution state: mutating the source state's slices after append
// cannot alter a retained entry.
func TestAppendStoresImmutableCompleteState(t *testing.T) {
	s := NewStore()
	state := richState()
	id := s.Append(state)
	// Mutate the source state's every slice after retention.
	state.Projection[0].Column = "mutated"
	state.Groups[0] = "mutated"
	state.Sets[0].Column = "mutated"
	state.Inserts[0].Column = "mutated"
	state.WhereValue.Int = 99
	got, ok := s.Lookup(id)
	if !ok {
		t.Fatal("retained entry vanished")
	}
	if !reflect.DeepEqual(got.State, richState()) {
		t.Errorf("retained state changed after source mutation: %+v", got.State)
	}
}

// TestRetrievalReturnsDefensiveCopies requires mutating a retrieved state —
// from Entries or Lookup — to leave the store's retained entries unchanged.
func TestRetrievalReturnsDefensiveCopies(t *testing.T) {
	s := NewStore()
	id := s.Append(richState())
	entries := s.Entries()
	entries[0].State.Projection[0].Column = "mutated"
	entries[0].State.Groups[0] = "mutated"
	entries[0].State.Sets[0].Column = "mutated"
	entries[0].State.Inserts[0].Column = "mutated"
	looked, _ := s.Lookup(id)
	looked.State.Projection[1].Kind = qb.ProjectionWildcard
	looked.State.LimitValue = 42
	if !reflect.DeepEqual(s.Entries()[0].State, richState()) {
		t.Error("store state changed after mutating a retrieved value")
	}
}
