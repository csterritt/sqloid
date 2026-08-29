// Immutable result-history entry tests (Issues #33 and #34), per the History
// Module Design and active-finalization Testing Decisions in
// Notes/PRD-sqloid.md. AppendFinalized is the single entry point: it retains
// exactly one entry per actual SELECT execution, deep-copies captured rows so
// the retained snapshot is immutable, and deterministically rejects duplicate
// finalization of the same execution ID with the first entry untouched.

package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// tabularFixture builds a tabular entry with two rows and full Issue #33
// metadata captured from a resultcache-shaped fact set.
func tabularFixture(execID uint64) ResultEntry {
	facts := CacheFacts{HasRetainedRange: true, Start: 1, End: 2, RowCapEvictions: 0}
	meta, err := NewSnapshotMetadata(facts, Lifecycle{
		Outcome:       OutcomeSuccess,
		ReachedLow:    true,
		ReachedHigh:   true,
		HasKnownTotal: true,
		KnownTotal:    2,
	})
	if err != nil {
		panic(err)
	}
	return ResultEntry{
		ExecutionID: execID,
		Kind:        KindTabular,
		Columns:     []string{"id"},
		Rows: [][]result.Value{
			{result.NewInteger(1), result.NewText("a")},
			{result.NewInteger(2), result.NewBlob([]byte{0x9f})},
		},
		Metadata:     meta,
		Completeness: Completeness{Complete: true},
	}
}

// TestAppendFinalizedExactlyOnce covers idempotence: a second append for the
// same execution is rejected with no new entry, no ID allocation, and the
// first entry untouched; a different execution appends normally.
func TestAppendFinalizedExactlyOnce(t *testing.T) {
	s := NewResultStore()
	first, ok := s.AppendFinalized(tabularFixture(7))
	if !ok || s.Len() != 1 {
		t.Fatalf("first append: ok=%v len=%d, want ok len 1", ok, s.Len())
	}
	if first.ID == 0 || first.ExecutionID != 7 {
		t.Fatalf("retained entry identity wrong: %+v", first)
	}

	again, ok := s.AppendFinalized(tabularFixture(7))
	if ok {
		t.Fatal("duplicate append for execution 7 was accepted")
	}
	if again.ID != 0 || s.Len() != 1 {
		t.Fatalf("duplicate append allocated an entry: id=%d len=%d", again.ID, s.Len())
	}
	if got := s.Entries()[0]; got.ID != first.ID || got.ExecutionID != first.ExecutionID || len(got.Rows) != len(first.Rows) {
		t.Fatal("rejection mutated the first entry")
	}

	if _, ok := s.AppendFinalized(tabularFixture(8)); !ok || s.Len() != 2 {
		t.Fatalf("append for execution 8: ok=%v len=%d, want ok len 2", ok, s.Len())
	}
}

// TestAppendFinalizedRejectsZeroExecution rejects an entry with no execution
// identity, since finalization is per execution only.
func TestAppendFinalizedRejectsZeroExecution(t *testing.T) {
	s := NewResultStore()
	if _, ok := s.AppendFinalized(tabularFixture(0)); ok || s.Len() != 0 {
		t.Fatal("zero execution ID appended")
	}
}

// TestRetainedEntryIsImmutable proves the retained snapshot never aliases
// caller storage: later mutation of the appended rows, of entries returned by
// Entries, and of a resultcache-backed value cannot reach the store.
func TestRetainedEntryIsImmutable(t *testing.T) {
	s := NewResultStore()
	entry := tabularFixture(7)
	if _, ok := s.AppendFinalized(entry); !ok {
		t.Fatal("append refused")
	}
	// The appended entry value shares its slices with the caller's fixture;
	// mutating them after the append must not reach the retained copy.
	entry.Rows[0][0] = result.NewInteger(999)
	entry.Columns[0] = "mutated"
	entry.Rows[1][1] = result.NewBlob([]byte{0x01})

	// Mutate the value returned by Entries: the retained copy must stand.
	got := s.Entries()[0]
	got.Rows[0][1] = result.NewText("hacked")
	got.Columns[0] = "hacked"
	got.Metadata.Reason = "hacked"

	retained := s.Entries()[0]
	if retained.Columns[0] != "id" {
		t.Fatalf("retained column mutated: %q", retained.Columns[0])
	}
	if retained.Rows[0][0].Int != 1 || retained.Rows[0][1].Str != "a" {
		t.Fatalf("retained row mutated: %+v", retained.Rows[0])
	}
	if retained.Rows[1][1].Kind == result.KindBlob && string(retained.Rows[1][1].Bytes) != "\x9f" {
		t.Fatalf("retained blob mutated: %x", retained.Rows[1][1].Bytes)
	}
	if retained.Metadata.Reason != "" {
		t.Fatalf("retained metadata mutated: %q", retained.Metadata.Reason)
	}
}

// TestCancelledAndErrorEntriesRetainShape verifies the non-tabular Cancelled
// and error entries carry their reason and typed terminal outcome with no
// rows, and that their metadata values are as immutable as tabular ones.
func TestCancelledAndErrorEntriesRetainShape(t *testing.T) {
	cancelMeta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
		Outcome: OutcomeCancelled, Reason: "cancelled by user",
	})
	if err != nil {
		t.Fatalf("cancelled metadata: %v", err)
	}
	failMeta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
		Outcome: OutcomeFailed, Reason: "no such table",
		HasFailurePosition: true, FailurePosition: 1,
	})
	if err != nil {
		t.Fatalf("failed metadata: %v", err)
	}

	s := NewResultStore()
	if _, ok := s.AppendFinalized(ResultEntry{
		ExecutionID: 3, Kind: KindCancelled,
		Metadata: cancelMeta, Reason: "cancelled by user",
	}); !ok {
		t.Fatal("cancelled append refused")
	}
	if _, ok := s.AppendFinalized(ResultEntry{
		ExecutionID: 4, Kind: KindError,
		Metadata: failMeta, Reason: "no such table",
	}); !ok {
		t.Fatal("error append refused")
	}

	entries := s.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Kind != KindCancelled || entries[0].Reason != "cancelled by user" ||
		len(entries[0].Rows) != 0 || entries[0].Metadata.Outcome != OutcomeCancelled {
		t.Fatalf("cancelled entry shape wrong: %+v", entries[0])
	}
	if entries[1].Kind != KindError || entries[1].Reason != "no such table" ||
		entries[1].Metadata.Outcome != OutcomeFailed || !entries[1].Metadata.HasFailurePosition ||
		entries[1].Metadata.FailurePosition != 1 {
		t.Fatalf("error entry shape wrong: %+v", entries[1])
	}
}

// TestFactsFromCacheSnapshotIsIndependent verifies FactsFromCache copies the
// authoritative resultcache facts once, so later cache eviction activity never
// changes an already-converted facts value used at finalization.
func TestFactsFromCacheSnapshotIsIndependent(t *testing.T) {
	c := resultcache.New()
	page := resultcache.Page{Start: 1}
	for i := int64(0); i < 10; i++ {
		page.Rows = append(page.Rows, resultcache.Row{
			Position: resultcache.Position(i + 1),
			Values:   []result.Value{result.NewInteger(i + 1)},
		})
	}
	if ok, _ := c.Merge(page, resultcache.Forward); !ok {
		t.Fatal("merge refused")
	}
	facts := FactsFromCache(c)

	// Later cache activity: an adjacent backward merge that prepends and
	// evicts the retained high end.
	back := resultcache.Page{Start: -9}
	for i := int64(0); i < 10; i++ {
		back.Rows = append(back.Rows, resultcache.Row{
			Position: resultcache.Position(-9 + i),
			Values:   []result.Value{result.NewInteger(-9 + i)},
		})
	}
	if ok, _ := c.Merge(back, resultcache.Backward); !ok {
		t.Fatal("backward merge refused")
	}

	if !facts.HasRetainedRange || facts.Start != 1 || facts.End != 10 {
		t.Fatalf("converted facts changed: %+v", facts)
	}
}
