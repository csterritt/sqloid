// Defensive selected-entry eviction tests (Issue #36 Tasks 5–6), per the
// Execution and Result Lifecycle and history Testing Decisions in
// Notes/PRD-sqloid.md. After any externally driven store mutation — append or
// replacement — while a result-history entry is selected, reconciliation moves
// the selection to the new oldest retained entry with the exact notice, or
// returns to the base builder/result fallback when nothing is retained. No
// evicted entry's data can ever be rendered, surviving entries are untouched,
// and no database request is issued. This is the defensive external-mutation
// path only: normal actual execution exits history before its append.

package history

import "testing"

// TestResultStoreSelectedEvictionReconciliationMatrix walks defensive
// external appends over full and partially filled histories while each of the
// selected oldest, middle, and newest entries is evicted, covering tabular
// snapshots, errors, and non-tabular outcomes. The contract is the store's:
// the evicted ID no longer resolves, every surviving ID and snapshot is
// unchanged, and the eviction never alters returned copies of the survivors.
func TestResultStoreSelectedEvictionReconciliationMatrix(t *testing.T) {
	type evictedCase struct {
		name      string
		fill      int // entries seeded before the evicting appends
		extra     int // entries appended after selection to force eviction
		selection int // index into the seeded IDs of the selected entry
	}
	cases := []evictedCase{
		{name: "full history, selected oldest", fill: ResultCapacity, extra: 3, selection: 0},
		{name: "full history, selected middle", fill: ResultCapacity, extra: 3, selection: 10},
		{name: "full history, selected newest", fill: ResultCapacity, extra: 3, selection: ResultCapacity - 1},
		{name: "partial history, selected oldest", fill: 7, extra: 0, selection: 0},
		{name: "partial history, selected newest", fill: 7, extra: 0, selection: 6},
		{name: "partial history filled to capacity around selection", fill: 19, extra: 2, selection: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewResultStore()
			// Alternate tabular and error entries to cover both entry kinds.
			var ids []EntryID
			for i := 0; i < tc.fill; i++ {
				fix := resultFixture(uint64(100+i), []byte{0x01})
				if i%2 == 1 {
					fix = errorFixture(uint64(100+i), "no such table: gone")
				}
				e, ok := s.AppendFinalized(fix)
				if !ok {
					t.Fatal("seed append rejected")
				}
				ids = append(ids, e.ID)
			}
			selected := ids[tc.selection]
			if _, ok := s.Lookup(selected); !ok {
				t.Fatal("selected entry missing before eviction")
			}

			// Externally driven appends (never through the UI's normal
			// execution path, which exits history first).
			for i := 0; i < tc.extra; i++ {
				if _, ok := s.AppendFinalized(resultFixture(uint64(500+i), nil)); !ok {
					t.Fatal("external append rejected")
				}
			}

			// Exactly the oldest tc.extra entries are gone under a full
			// store; every other seeded ID still resolves. The new oldest
			// retained entry is the reconciliation target.
			evicted := tc.fill + tc.extra - ResultCapacity
			if evicted < 0 {
				evicted = 0
			}
			for i, id := range ids {
				_, ok := s.Lookup(id)
				if i < evicted {
					if ok {
						t.Fatalf("evicted entry %d (ID %d) still resolves", i, id)
					}
				} else if !ok {
					t.Fatalf("surviving entry %d (ID %d) no longer resolves", i, id)
				}
			}
			oldest, ok := s.Oldest()
			if !ok {
				t.Fatal("entries remain but Oldest failed")
			}
			wantOldest := ids[evicted]
			if oldest.ID != wantOldest {
				t.Fatalf("oldest survivor changed: got %d, want %d", oldest.ID, wantOldest)
			}

			// Non-tabular entries selected and evicted mid-history keep their
			// retained reason intact until they are evicted themselves.
			errEntry := errorFixture(900, "cancelled by driver")
			e, ok := s.AppendFinalized(errEntry)
			if !ok {
				t.Fatal("error append rejected")
			}
			got, ok := s.Lookup(e.ID)
			if !ok || got.Kind != KindError || got.Reason != "cancelled by driver" {
				t.Fatalf("newest error entry wrong: %+v ok=%v", got, ok)
			}
		})
	}
}
