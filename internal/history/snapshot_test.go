// Immutable typed snapshot metadata tests for Issue #33, per the Cache and
// snapshot invariant and History Module Design decisions in
// Notes/PRD-sqloid.md. The metadata value is independent of retained rows and
// presentation strings: it carries the optional inclusive retained range,
// optional known total, reached-low/reached-high endpoint observations,
// persistent row-cap and byte-cap eviction facts, UTF status, and an
// independently typed terminal outcome with reason and optional one-based
// last failure position. Completeness classification is a separate concern
// (snapshot_classify_test.go); these tests only specify the metadata model.

package history

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// metaFacts builds cache facts for a retained range [start,end].
func metaFacts(start, end Position) CacheFacts {
	return CacheFacts{
		HasRetainedRange: true,
		Start:            start,
		End:              end,
	}
}

// TestSnapshotMetadataCarriesTypedFacts walks the core metadata matrix: the
// constructor accepts the full fact set and records each typed fact exactly
// as supplied, without deriving or rewriting any observation.
func TestSnapshotMetadataCarriesTypedFacts(t *testing.T) {
	cases := []struct {
		name string
		fact CacheFacts
		life Lifecycle
		want SnapshotMetadata
	}{
		{
			name: "success full range and known total",
			fact: metaFacts(1, 10),
			life: Lifecycle{
				Outcome:       OutcomeSuccess,
				HasKnownTotal: true,
				KnownTotal:    10,
				ReachedLow:    true,
				ReachedHigh:   true,
			},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 10,
				HasKnownTotal: true, KnownTotal: 10,
				ReachedLow: true, ReachedHigh: true,
				Outcome: OutcomeSuccess,
			},
		},
		{
			name: "cancelled with reason, no failure position",
			fact: CacheFacts{HasRetainedRange: true, Start: 5, End: 9},
			life: Lifecycle{
				Outcome:    OutcomeCancelled,
				Reason:     "cancelled by user",
				ReachedLow: false, ReachedHigh: true,
			},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 5, RetainedEnd: 9,
				ReachedHigh: true,
				Outcome:     OutcomeCancelled, Reason: "cancelled by user",
			},
		},
		{
			name: "failed with one-based last failure position",
			fact: metaFacts(1, 20),
			life: Lifecycle{
				Outcome:            OutcomeFailed,
				Reason:             "connection lost",
				HasFailurePosition: true,
				FailurePosition:    37,
				ReachedLow:         true,
			},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 20,
				ReachedLow:         true,
				Outcome:            OutcomeFailed,
				Reason:             "connection lost",
				HasFailurePosition: true, FailurePosition: 37,
			},
		},
		{
			name: "empty retained range with unknown total",
			fact: CacheFacts{},
			life: Lifecycle{Outcome: OutcomeSuccess},
			want: SnapshotMetadata{Outcome: OutcomeSuccess},
		},
		{
			name: "both eviction flags together",
			fact: CacheFacts{
				HasRetainedRange:   true,
				Start:              11,
				End:                20,
				RowCapEvictions:    7,
				TruncatedByByteCap: true,
			},
			life: Lifecycle{Outcome: OutcomeSuccess},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 11, RetainedEnd: 20,
				RowCapEvicted: true, RowCapEvictions: 7,
				TruncatedByByteCap: true,
				Outcome:            OutcomeSuccess,
			},
		},
		{
			name: "row-cap eviction alone",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 4, RowCapEvictions: 3},
			life: Lifecycle{Outcome: OutcomeCancelled, Reason: "interrupted"},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 4,
				RowCapEvicted: true, RowCapEvictions: 3,
				Outcome: OutcomeCancelled, Reason: "interrupted",
			},
		},
		{
			name: "byte-cap disclosure alone persists as typed metadata",
			fact: CacheFacts{HasRetainedRange: true, Start: 1, End: 2, TruncatedByByteCap: true},
			life: Lifecycle{Outcome: OutcomeSuccess},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 2,
				TruncatedByByteCap: true,
				Outcome:            OutcomeSuccess,
			},
		},
		{
			name: "invalid UTF status",
			fact: metaFacts(1, 3),
			life: Lifecycle{Outcome: OutcomeSuccess, InvalidUTF: true},
			want: SnapshotMetadata{
				HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 3,
				InvalidUTF: true,
				Outcome:    OutcomeSuccess,
			},
		},
		{
			name: "cancelled failure position where applicable",
			fact: CacheFacts{},
			life: Lifecycle{
				Outcome:            OutcomeCancelled,
				Reason:             "cancelled after rows",
				HasFailurePosition: true,
				FailurePosition:    4,
			},
			want: SnapshotMetadata{
				Outcome:            OutcomeCancelled,
				Reason:             "cancelled after rows",
				HasFailurePosition: true, FailurePosition: 4,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewSnapshotMetadata(tc.fact, tc.life)
			if err != nil {
				t.Fatalf("NewSnapshotMetadata(%+v, %+v): %v", tc.fact, tc.life, err)
			}
			if got != tc.want {
				t.Errorf("metadata = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestSnapshotMetadataRejectsInvalidShape pins the validation contract: the
// range shape is validated (never rewritten), outcomes must be one of the
// three typed terminal outcomes, and a one-based failure position is
// applicable only to cancellation and failure outcomes.
func TestSnapshotMetadataRejectsInvalidShape(t *testing.T) {
	cases := []struct {
		name string
		fact CacheFacts
		life Lifecycle
	}{
		{
			name: "retained end before start",
			fact: CacheFacts{HasRetainedRange: true, Start: 9, End: 4},
			life: Lifecycle{Outcome: OutcomeSuccess},
		},
		{
			name: "zero outcome",
			fact: metaFacts(1, 1),
			life: Lifecycle{},
		},
		{
			name: "unknown outcome value",
			fact: metaFacts(1, 1),
			life: Lifecycle{Outcome: TerminalOutcome(99)},
		},
		{
			name: "failure position below one",
			fact: CacheFacts{},
			life: Lifecycle{Outcome: OutcomeFailed, HasFailurePosition: true, FailurePosition: 0},
		},
		{
			name: "failure position on success",
			fact: CacheFacts{},
			life: Lifecycle{Outcome: OutcomeSuccess, HasFailurePosition: true, FailurePosition: 3},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSnapshotMetadata(tc.fact, tc.life); err == nil {
				t.Errorf("NewSnapshotMetadata(%+v, %+v) = nil error, want validation failure", tc.fact, tc.life)
			}
		})
	}
}

// TestSnapshotMetadataValueSemantics requires value semantics so metadata
// cannot change after finalization: mutating the construction inputs, the
// returned value, or the authoritative cache afterwards never changes
// already-constructed metadata.
func TestSnapshotMetadataValueSemantics(t *testing.T) {
	t.Run("constructor copies from inputs", func(t *testing.T) {
		fact := CacheFacts{HasRetainedRange: true, Start: 1, End: 5, RowCapEvictions: 2}
		life := Lifecycle{
			Outcome: OutcomeFailed, Reason: "boom",
			HasFailurePosition: true, FailurePosition: 6,
			HasKnownTotal: true, KnownTotal: 500,
		}
		meta, err := NewSnapshotMetadata(fact, life)
		if err != nil {
			t.Fatalf("NewSnapshotMetadata: %v", err)
		}
		// Mutate the source inputs after construction.
		fact.Start = 100
		fact.End = 200
		fact.RowCapEvictions = 99
		life.Reason = "rewritten"
		life.FailurePosition = 1234
		life.KnownTotal = 1
		if meta != (SnapshotMetadata{
			HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 5,
			RowCapEvicted: true, RowCapEvictions: 2,
			HasKnownTotal: true, KnownTotal: 500,
			Outcome:            OutcomeFailed,
			Reason:             "boom",
			HasFailurePosition: true, FailurePosition: 6,
		}) {
			t.Errorf("metadata changed after input mutation: %+v", meta)
		}
	})
	t.Run("value copies are independent", func(t *testing.T) {
		meta, err := NewSnapshotMetadata(metaFacts(1, 3), Lifecycle{Outcome: OutcomeSuccess})
		if err != nil {
			t.Fatalf("NewSnapshotMetadata: %v", err)
		}
		mutated := meta
		mutated.RetainedEnd = 999
		mutated.Reason = "tampered"
		mutated.Outcome = OutcomeFailed
		if meta.RetainedEnd != 3 || meta.Reason != "" || meta.Outcome != OutcomeSuccess {
			t.Errorf("original metadata changed through a value copy: %+v", meta)
		}
	})
	t.Run("cache mutation after FactsFromCache does not change facts", func(t *testing.T) {
		cache := resultcache.New()
		page := resultcache.Page{Start: 1}
		for i := int64(1); i <= 4; i++ {
			page.Rows = append(page.Rows, resultcache.Row{
				Position: resultcache.Position(i),
				Values:   []result.Value{result.NewInteger(i)},
			})
		}
		if ok, err := cache.Merge(page, resultcache.Forward); !ok || err != nil {
			t.Fatalf("initial merge = (%v, %v), want (true, nil)", ok, err)
		}
		facts := FactsFromCache(cache)
		// Later cache activity: further pages whose payload triggers byte-cap
		// eviction (and the persistent disclosure).
		first := resultcache.Page{Start: 5, Rows: []resultcache.Row{
			{Position: 5, Values: []result.Value{result.NewBlob(make([]byte, 40<<20))}},
		}}
		if ok, _ := cache.Merge(first, resultcache.Forward); !ok {
			t.Fatalf("first big merge rejected")
		}
		second := resultcache.Page{Start: 6, Rows: []resultcache.Row{
			{Position: 6, Values: []result.Value{result.NewBlob(make([]byte, 40<<20))}},
		}}
		if ok, _ := cache.Merge(second, resultcache.Forward); !ok {
			t.Fatalf("second big merge rejected")
		}
		if !cache.TruncatedByByteCap() {
			t.Fatalf("test setup: byte-cap eviction did not occur")
		}
		if facts.End != 4 || facts.Start != 1 || facts.RowCapEvictions != 0 || facts.TruncatedByByteCap {
			t.Errorf("facts from cache changed after later cache mutation: %+v", facts)
		}
	})
}

// TestSnapshotMetadataNoPresentationDuplication keeps the Issue #31
// presentation literal out of the snapshot model: the metadata carries typed
// `truncated-by-byte-cap` facts only, and the shared warning definition stays
// in internal/result. This is an architecture-style source scan of the
// package's production files.
func TestSnapshotMetadataNoPresentationDuplication(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read history package: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), "Result truncated: 64 MiB cache limit") {
			t.Errorf("%s duplicates the shared Issue #31 warning literal; the model carries typed facts only", name)
		}
		if _, err := parser.ParseFile(fset, name, src, 0); err != nil {
			t.Errorf("parse %s: %v", name, err)
		}
	}
}
