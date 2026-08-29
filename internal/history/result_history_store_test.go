// Bounded immutable result-history store tests (Issue #36), per the
// Execution and Result Lifecycle, History Module Design, and history Testing
// Decisions in Notes/PRD-sqloid.md. Every finalized execution appends one
// stable non-positional ID; the store retains exactly the 20 newest entries in
// chronological order, evicts oldest first without changing surviving IDs,
// deep-copies tabular rows, columns, and typed values (including exact BLOB
// bytes) with ascending absolute positions, retains non-tabular outcomes, and
// exposes stable-ID older/newer/current selection with deterministic empty and
// boundary behavior.

package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// resultFixture builds a tabular entry whose rows and metadata come from a
// real resultcache-shaped fact set, carrying one BLOB to prove exact byte
// retention.
func resultFixture(execID uint64, blob []byte) ResultEntry {
	cache := resultcache.New()
	page := resultcache.Page{Start: 3}
	for i := 0; i < 3; i++ {
		row := []result.Value{result.NewInteger(int64(3 + i))}
		if i == 1 {
			row = append(row, result.NewBlob(blob))
		}
		page.Rows = append(page.Rows, resultcache.Row{Position: resultcache.Position(3 + i), Values: row})
	}
	if _, err := cache.Merge(page, resultcache.Forward); err != nil {
		panic(err)
	}
	meta, err := NewSnapshotMetadata(FactsFromCache(cache), Lifecycle{
		Outcome:    OutcomeSuccess,
		ReachedLow: true,
	})
	if err != nil {
		panic(err)
	}
	rows := make([][]result.Value, 3)
	for i, r := range cache.Rows() {
		rows[i] = r.Values
	}
	return ResultEntry{
		ExecutionID:  execID,
		Kind:         KindTabular,
		Columns:      []string{"id", "payload"},
		Rows:         rows,
		Metadata:     meta,
		Completeness: Completeness{Partial: true},
	}
}

// errorFixture builds a non-tabular error entry for an execution whose first
// page failed before any row was retained.
func errorFixture(execID uint64, reason string) ResultEntry {
	meta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{Outcome: OutcomeFailed, Reason: reason})
	if err != nil {
		panic(err)
	}
	return ResultEntry{
		ExecutionID:  execID,
		Kind:         KindError,
		Reason:       reason,
		Metadata:     meta,
		Completeness: Completeness{Partial: true},
	}
}

// appendSome appends n tabular entries for executions exec, exec+1, ... and
// returns the retained IDs in append (chronological) order.
func appendSome(s *ResultStore, exec, n uint64) []EntryID {
	var ids []EntryID
	for i := uint64(0); i < n; i++ {
		e, ok := s.AppendFinalized(resultFixture(exec+i, []byte{0x01}))
		if !ok {
			panic("append rejected unexpectedly")
		}
		ids = append(ids, e.ID)
	}
	return ids
}

// TestResultStoreBoundedRetention walks the retention matrix: exactly 20
// newest entries survive, oldest-first eviction never changes surviving IDs,
// IDs stay stable non-positional identities, and chronological order holds.
func TestResultStoreBoundedRetention(t *testing.T) {
	cases := []struct {
		name   string
		append uint64
		want   int
	}{
		{"under capacity", 5, 5},
		{"exactly at capacity", 20, 20},
		{"one past capacity", 21, 20},
		{"several past capacity", 27, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewResultStore()
			ids := appendSome(s, 100, tc.append)
			if s.Len() != tc.want {
				t.Fatalf("Len = %d, want %d", s.Len(), tc.want)
			}
			if len(ids) != int(tc.append) || ids[0] == 0 {
				t.Fatalf("append did not allocate one stable ID per finalized execution: %v", ids)
			}
			entries := s.Entries()
			// Exactly the 20 newest survive in chronological order.
			kept := ids[len(ids)-tc.want:]
			for i, e := range entries {
				if e.ID != kept[i] {
					t.Fatalf("entries[%d].ID = %d, want %d (chronological order broken)", i, e.ID, kept[i])
				}
				if e.ExecutionID != 100+uint64(len(ids)-tc.want)+uint64(i) {
					t.Fatalf("entries[%d].ExecutionID = %d, want the %d-th newest", i, e.ExecutionID, i)
				}
			}
			// Eviction never changes surviving IDs: for a history already at
			// capacity, one more append evicts the oldest survivor, which keeps
			// its ID and becomes the new oldest. Under capacity nothing is
			// evicted and nothing reorders.
			newest, ok := s.AppendFinalized(resultFixture(999, nil))
			if !ok {
				t.Fatal("append rejected")
			}
			if tc.want == ResultCapacity {
				if s.Len() != ResultCapacity {
					t.Fatalf("Len after eviction append = %d, want %d", s.Len(), ResultCapacity)
				}
				if s.Entries()[0].ID != kept[1] {
					t.Fatalf("oldest surviving ID changed on eviction: got %d, want %d", s.Entries()[0].ID, kept[1])
				}
				if s.Entries()[19].ID != newest.ID {
					t.Fatal("newest entry is not last in chronological order")
				}
				return
			}
			if s.Len() != tc.want+1 || s.Entries()[0].ID != ids[0] || s.Entries()[tc.want].ID != newest.ID {
				t.Fatalf("under-capacity append evicted or reordered entries: len=%d", s.Len())
			}
		})
	}
}

// TestResultStoreImmutableSnapshots proves deep immutability: mutating the
// source rows after append, or a retrieved entry afterwards, can never alter
// retained rows, columns, typed values (including exact BLOB bytes), positions,
// or metadata.
func TestResultStoreImmutableSnapshots(t *testing.T) {
	blob := []byte{0x9f, 0x00, 0xff}
	s := NewResultStore()
	src := resultFixture(7, blob)
	if _, ok := s.AppendFinalized(src); !ok {
		t.Fatal("append rejected")
	}

	// Mutate the caller's source entry after append.
	src.Columns[0] = "mutated"
	src.Rows[0][0] = result.NewText("mutated")
	src.Rows[1][1].Bytes[0] = 0x00
	if len(src.Rows) > 2 {
		src.Rows = src.Rows[:1]
	}

	// Retrieve and mutate the retrieved copy too.
	got, ok := s.Lookup(s.Entries()[0].ID)
	if !ok {
		t.Fatal("Lookup failed for a retained entry")
	}
	got.Columns[1] = "mutated"
	got.Rows[2][0] = result.NewText("mutated")

	// The retained snapshot must be exactly as appended.
	retained := s.Entries()[0]
	if retained.Columns[0] != "id" || retained.Columns[1] != "payload" {
		t.Fatalf("retained columns mutated: %v", retained.Columns)
	}
	if v := retained.Rows[0][0]; v.Kind != result.KindInteger || v.Int != 3 {
		t.Fatalf("retained row 0 value mutated: %+v", v)
	}
	if v := retained.Rows[1][1]; v.Kind != result.KindBlob || string(v.Bytes) != string(blob) {
		t.Fatalf("retained BLOB bytes changed: %x", v.Bytes)
	}
	if v := retained.Rows[2][0]; v.Kind != result.KindInteger || v.Int != 5 {
		t.Fatalf("retained row 2 value mutated: %+v", v)
	}
	// Ascending absolute positions and metadata are preserved exactly.
	if !retained.Metadata.HasRetainedRange || retained.Metadata.RetainedStart != 3 || retained.Metadata.RetainedEnd != 5 {
		t.Fatalf("retained absolute positions changed: %+v", retained.Metadata)
	}
	if retained.Metadata.Outcome != OutcomeSuccess {
		t.Fatalf("retained outcome changed: %v", retained.Metadata.Outcome)
	}
}

// TestResultStoreRetainsNonTabularOutcomes proves error (and cancelled-style)
// entries are retained like any other finalized execution, with their reason.
func TestResultStoreRetainsNonTabularOutcomes(t *testing.T) {
	s := NewResultStore()
	tab, ok := s.AppendFinalized(resultFixture(1, nil))
	if !ok {
		t.Fatal("tabular append rejected")
	}
	errEntry, ok := s.AppendFinalized(errorFixture(2, "database is locked"))
	if !ok || s.Len() != 2 {
		t.Fatalf("error entry: ok=%v len=%d, want ok len 2", ok, s.Len())
	}
	got, ok := s.Lookup(errEntry.ID)
	if !ok || got.Kind != KindError || got.Reason != "database is locked" || len(got.Rows) != 0 {
		t.Fatalf("non-tabular entry not retained intact: %+v ok=%v", got, ok)
	}
	if got.ExecutionID != 2 {
		t.Fatalf("error entry execution = %d, want 2", got.ExecutionID)
	}
	if _, ok := s.Lookup(tab.ID); !ok {
		t.Fatal("tabular entry lost after error append")
	}
}

// TestResultStoreSelection walks stable-ID selection: lookup, oldest/newest,
// older/newer steps, deterministic empty and boundary behavior, and
// deep-copied returns that cannot alias the store.
func TestResultStoreSelection(t *testing.T) {
	t.Run("empty store", func(t *testing.T) {
		s := NewResultStore()
		if _, ok := s.Oldest(); ok {
			t.Error("Oldest succeeded on an empty store")
		}
		if _, ok := s.Newest(); ok {
			t.Error("Newest succeeded on an empty store")
		}
		if _, ok := s.Lookup(1); ok {
			t.Error("Lookup of a never-allocated ID succeeded")
		}
		if _, ok := s.OlderThan(1); ok {
			t.Error("OlderThan succeeded on an empty store")
		}
		if _, ok := s.NewerThan(1); ok {
			t.Error("NewerThan succeeded on an empty store")
		}
	})

	s := NewResultStore()
	ids := appendSome(s, 10, 4)

	t.Run("oldest and newest", func(t *testing.T) {
		oldest, ok := s.Oldest()
		if !ok || oldest.ID != ids[0] {
			t.Fatalf("Oldest = %+v ok=%v, want ID %d", oldest, ok, ids[0])
		}
		newest, ok := s.Newest()
		if !ok || newest.ID != ids[3] {
			t.Fatalf("Newest = %+v ok=%v, want ID %d", newest, ok, ids[3])
		}
	})

	t.Run("older and newer steps are independent of indices", func(t *testing.T) {
		older, ok := s.OlderThan(ids[2])
		if !ok || older.ID != ids[1] {
			t.Fatalf("OlderThan = %+v ok=%v, want ID %d", older, ok, ids[1])
		}
		newer, ok := s.NewerThan(ids[2])
		if !ok || newer.ID != ids[3] {
			t.Fatalf("NewerThan = %+v ok=%v, want ID %d", newer, ok, ids[3])
		}
	})

	t.Run("boundaries are deterministic no-ops", func(t *testing.T) {
		if _, ok := s.OlderThan(ids[0]); ok {
			t.Error("OlderThan crossed the oldest boundary")
		}
		if _, ok := s.NewerThan(ids[3]); ok {
			t.Error("NewerThan crossed the newest boundary")
		}
		if _, ok := s.OlderThan(0); ok {
			t.Error("OlderThan resolved through the none ID")
		}
		if _, ok := s.NewerThan(0); ok {
			t.Error("NewerThan resolved through the none ID")
		}
	})

	t.Run("evicted and unknown IDs never resolve", func(t *testing.T) {
		full := NewResultStore()
		fullIDs := appendSome(full, 50, 25)
		if _, ok := full.Lookup(fullIDs[0]); ok {
			t.Error("Lookup resolved an evicted ID")
		}
		if _, ok := full.OlderThan(fullIDs[0]); ok {
			t.Error("OlderThan resolved through an evicted ID")
		}
		if _, ok := full.NewerThan(fullIDs[0]); ok {
			t.Error("NewerThan resolved through an evicted ID")
		}
		if _, ok := full.Lookup(12345); ok {
			t.Error("Lookup resolved a never-allocated ID")
		}
	})

	t.Run("returned entries are deep copies", func(t *testing.T) {
		e, _ := s.Oldest()
		e.Columns[0] = "mutated"
		if len(e.Rows) > 0 {
			e.Rows[0][0] = result.NewText("mutated")
		}
		again, _ := s.Oldest()
		if again.Columns[0] != "id" {
			t.Fatal("returned entry aliased stored columns")
		}
		if len(again.Rows) > 0 && again.Rows[0][0].Kind != result.KindInteger {
			t.Fatal("returned entry aliased stored rows")
		}
	})
}
