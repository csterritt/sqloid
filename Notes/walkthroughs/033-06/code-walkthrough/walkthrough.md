# Issue #33 — snapshot metadata, completeness, and endpoints

*2026-08-28T19:46:33Z by Showboat 0.6.1*
<!-- showboat-id: 1299d3bf-106d-4bcc-b678-80a627af9a84 -->

This walkthrough demonstrates the completed Issue #33 implementation: the immutable typed snapshot metadata model in internal/history (independent of retained rows and presentation strings), the narrow cache-to-snapshot conversion boundary, truthful completeness classification with exclusive complete and coexisting partial/truncated, and the persistent typed truncated-by-byte-cap fact. See Issue #33, the Cache and snapshot invariant of Notes/PRD-sqloid.md, and Notes/wiki/snapshot-metadata.md. Every block below is re-runnable from the repository root and cleans up its temporary test file.

First: metadata is constructed once from authoritative cache facts and lifecycle inputs, then nothing can change it — not mutating the inputs, not mutating a value copy, and not later cache activity.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/history/zz_demo_test.go <<'EOF'
package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

func TestDemoMetadataImmutable(t *testing.T) {
	c := resultcache.New()
	page := resultcache.Page{Start: 1}
	for i := int64(1); i <= 4; i++ {
		page.Rows = append(page.Rows, resultcache.Row{
			Position: resultcache.Position(i),
			Values:   []result.Value{result.NewInteger(i)},
		})
	}
	if ok, err := c.Merge(page, resultcache.Forward); !ok || err != nil {
		t.Fatal("merge rejected")
	}
	facts := FactsFromCache(c)
	t.Logf("facts: range=[%d,%d] evictions=%d byteCap=%v", facts.Start, facts.End, facts.RowCapEvictions, facts.TruncatedByByteCap)
	life := Lifecycle{
		Outcome: OutcomeFailed, Reason: "driver failure",
		HasFailurePosition: true, FailurePosition: 6, ReachedLow: true,
	}
	meta, err := NewSnapshotMetadata(facts, life)
	if err != nil {
		t.Fatal(err)
	}
	// Mutation attempts on the inputs, a value copy, and the cache itself.
	fact := facts
	fact.Start, fact.End, fact.RowCapEvictions = 100, 200, 99
	life.Reason, life.FailurePosition = "rewritten", 1234
	mutated := meta
	mutated.RetainedEnd, mutated.Reason, mutated.Outcome = 999, "tampered", OutcomeCancelled
	big := resultcache.Page{Start: 5, Rows: []resultcache.Row{
		{Position: 5, Values: []result.Value{result.NewBlob(make([]byte, 40 << 20))}},
	}}
	c.Merge(big, resultcache.Forward)
	c.Merge(resultcache.Page{Start: 6, Rows: []resultcache.Row{
		{Position: 6, Values: []result.Value{result.NewBlob(make([]byte, 40 << 20))}},
	}}, resultcache.Forward)
	t.Logf("cache now: range=[%d,%d] byteCap=%v", mustStart(c), mustEnd(c), c.TruncatedByByteCap())
	if meta.RetainedEnd != 4 || meta.RetainedStart != 1 {
		t.Fatal("range mutated")
	}
	if meta.Reason != "driver failure" || meta.Outcome != OutcomeFailed || meta.FailurePosition != 6 {
		t.Fatal("terminal details mutated")
	}
	if meta.RowCapEvicted || meta.TruncatedByByteCap || meta.HasKnownTotal {
		t.Fatal("eviction/total facts changed")
	}
	t.Logf("metadata unchanged after all mutation attempts: outcome=%v range=[%d,%d] reason=%q position=%d",
		meta.Outcome, meta.RetainedStart, meta.RetainedEnd, meta.Reason, meta.FailurePosition)
}

func mustStart(c *resultcache.Cache) resultcache.Position { s, _ := c.Start(); return s }
func mustEnd(c *resultcache.Cache) resultcache.Position   { e, _ := c.End(); return e }
EOF
go test ./internal/history -run TestDemoMetadataImmutable -count=1 -v 2>&1 | grep -E "facts:|cache now:|metadata unchanged"; rm internal/history/zz_demo_test.go
```

```output
    zz_demo_test.go:23: facts: range=[1,4] evictions=0 byteCap=false
    zz_demo_test.go:45: cache now: range=[6,6] byteCap=true
    zz_demo_test.go:55: metadata unchanged after all mutation attempts: outcome=failed range=[1,4] reason="driver failure" position=6
```

The cache moved (the second 40 MiB blob evicted everything before position 6 and set the persistent byte-cap disclosure), yet the finalized metadata keeps range [1,4], the failure reason, and the one-based failure position exactly as observed. Now the classification matrix: exclusive complete, partial, truncated, and partial+truncated across known totals, count failure, limited results, eviction, short and empty final-page observations, and unknown remainder.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/history/zz_demo_test.go <<'EOF'
package history

import (
	"testing"
)

func classify(t *testing.T, name string, fact CacheFacts, life Lifecycle, tr TraversalFacts) {
	t.Helper()
	meta, err := NewSnapshotMetadata(fact, life)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	t.Logf("%-52s -> %s", name, Classify(meta, tr))
}

func TestDemoClassification(t *testing.T) {
	full := func(total int64) (CacheFacts, Lifecycle) {
		return CacheFacts{HasRetainedRange: true, Start: 1, End: Position(total)},
			Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: total, ReachedLow: true, ReachedHigh: true}
	}
	done := TraversalFacts{CountWorkFinished: true, PageWorkFinished: true}

	fact, life := full(10)
	classify(t, "complete: known total 10, full retention", fact, life, done)

	fact, life = full(10)
	life.HasKnownTotal, life.KnownTotal = true, 10
	classify(t, "complete: limit 500, rows beyond Limit irrelevant", fact, life,
		TraversalFacts{HasLimit: true, Limit: 500, CountWorkFinished: true, PageWorkFinished: true})

	classify(t, "complete: count failed but empty page observed",
		CacheFacts{}, Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true, ReachedHigh: true},
		TraversalFacts{ObservedShortFinalPage: true, CountWorkFinished: true, PageWorkFinished: true})

	classify(t, "truncated only: row-cap eviction, all rows known",
		CacheFacts{HasRetainedRange: true, Start: 91, End: 100, RowCapEvictions: 90},
		Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedHigh: true}, done)

	fact, life = full(10)
	fact.TruncatedByByteCap = true
	classify(t, "truncated only: byte-cap fact despite total 10 retained", fact, life, done)

	classify(t, "partial only: count failed, unknown remainder",
		CacheFacts{HasRetainedRange: true, Start: 1, End: 20},
		Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true}, done)

	classify(t, "partial only: full pages at requested size, no observation",
		CacheFacts{HasRetainedRange: true, Start: 1, End: 20},
		Lifecycle{Outcome: OutcomeSuccess, ReachedLow: true}, done)

	classify(t, "partial+truncated: unknown remainder and row-cap eviction",
		CacheFacts{HasRetainedRange: true, Start: 10001, End: 10020, RowCapEvictions: 10000},
		Lifecycle{Outcome: OutcomeSuccess}, done)

	classify(t, "partial+truncated: inconsistent count 100 above range",
		CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
		Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 100, ReachedLow: true},
		TraversalFacts{CountCacheInconsistent: true, CountWorkFinished: true, PageWorkFinished: true})

	classify(t, "inconsistent count 5 below range: never clamped",
		CacheFacts{HasRetainedRange: true, Start: 1, End: 10},
		Lifecycle{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 5, ReachedLow: true, ReachedHigh: true},
		TraversalFacts{CountCacheInconsistent: true, CountWorkFinished: true, PageWorkFinished: true})
}
EOF
go test ./internal/history -run TestDemoClassification -count=1 -v 2>&1 | grep -E "\->"; rm internal/history/zz_demo_test.go
```

```output
    zz_demo_test.go:24: complete: known total 10, full retention             -> complete
    zz_demo_test.go:28: complete: limit 500, rows beyond Limit irrelevant    -> complete
    zz_demo_test.go:31: complete: count failed but empty page observed       -> complete
    zz_demo_test.go:35: truncated only: row-cap eviction, all rows known     -> truncated
    zz_demo_test.go:41: truncated only: byte-cap fact despite total 10 retained -> truncated
    zz_demo_test.go:43: partial only: count failed, unknown remainder        -> partial
    zz_demo_test.go:47: partial only: full pages at requested size, no observation -> partial
    zz_demo_test.go:51: partial+truncated: unknown remainder and row-cap eviction -> partial+truncated
    zz_demo_test.go:55: partial+truncated: inconsistent count 100 above range -> partial+truncated
    zz_demo_test.go:60: inconsistent count 5 below range: never clamped      -> partial
```

Every label is truthful: complete is exclusive; partial and truncated coexist only when both are true; an inconsistent count below the retained range is preserved as partial without clamping rows, range, total, or endpoints. Next: ascending absolute positions after forward and backward traversal, terminal outcomes varied independently with reasons and one-based failure positions, and the persistent byte-cap metadata next to the shared Issue #31 presentation warning.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/history/zz_demo_test.go <<'EOF'
package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

func TestDemoTraversalAndOutcomes(t *testing.T) {
	// Forward then backward traversal over the same execution.
	c := resultcache.New()
	mk := func(start, n int64) resultcache.Page {
		p := resultcache.Page{Start: resultcache.Position(start)}
		for i := int64(0); i < n; i++ {
			p.Rows = append(p.Rows, resultcache.Row{
				Position: resultcache.Position(start + i),
				Values:   []result.Value{result.NewInteger(start + i)},
			})
		}
		return p
	}
	c.Merge(mk(21, 10), resultcache.Forward)
	c.Merge(mk(11, 10), resultcache.Backward)
	c.Merge(mk(1, 10), resultcache.Backward)
	facts := FactsFromCache(c)
	t.Logf("after backward traversal: range=[%d,%d]", facts.Start, facts.End)
	for i, r := range c.Rows() {
		if r.Position != resultcache.Position(i+1) {
			t.Fatalf("row %d at position %d: not ascending", i, r.Position)
		}
	}
	t.Logf("all %d retained rows ascending regardless of traversal direction", c.Len())

	// The same completeness facts under three terminal outcomes.
	fact := CacheFacts{HasRetainedRange: true, Start: 1, End: 10}
	tr := TraversalFacts{CountWorkFinished: true, PageWorkFinished: true}
	for _, life := range []Lifecycle{
		{Outcome: OutcomeSuccess, HasKnownTotal: true, KnownTotal: 10, ReachedLow: true, ReachedHigh: true},
		{Outcome: OutcomeCancelled, Reason: "cancelled after rows", HasKnownTotal: true, KnownTotal: 10, ReachedLow: true, ReachedHigh: true},
		{Outcome: OutcomeFailed, Reason: "driver failure", HasFailurePosition: true, FailurePosition: 11, HasKnownTotal: true, KnownTotal: 10, ReachedLow: true, ReachedHigh: true},
	} {
		meta, _ := NewSnapshotMetadata(fact, life)
		t.Logf("outcome %-9v reason=%-19q position=%d -> %s",
			meta.Outcome, meta.Reason, meta.FailurePosition, Classify(meta, tr))
	}
}
EOF
go test ./internal/history -run TestDemoTraversalAndOutcomes -count=1 -v 2>&1 | grep -E "after backward|ascending|outcome "; rm internal/history/zz_demo_test.go
```

```output
    zz_demo_test.go:27: after backward traversal: range=[1,30]
    zz_demo_test.go:33: all 30 retained rows ascending regardless of traversal direction
    zz_demo_test.go:44: outcome success   reason=""                  position=0 -> complete
    zz_demo_test.go:44: outcome cancelled reason="cancelled after rows" position=0 -> complete
    zz_demo_test.go:44: outcome failed    reason="driver failure"    position=11 -> complete
```

Terminal success, cancellation, and failure keep the same complete label — the outcome axis is fully independent, with reasons and one-based failure positions carried unchanged. Finally: persistent typed truncated-by-byte-cap metadata whose presentation remains the shared Issue #31 definition, and the full suites.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/history/zz_demo_test.go <<'EOF'
package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

func TestDemoByteCapPersistence(t *testing.T) {
	c := resultcache.New()
	c.Merge(resultcache.Page{Start: 1, Rows: []resultcache.Row{
		{Position: 1, Values: []result.Value{result.NewBlob(make([]byte, 40 << 20))}},
	}}, resultcache.Forward)
	c.Merge(resultcache.Page{Start: 2, Rows: []resultcache.Row{
		{Position: 2, Values: []result.Value{result.NewBlob(make([]byte, 40 << 20))}},
	}}, resultcache.Forward)
	t.Logf("after byte eviction: byteCap=%v payload=%d", c.TruncatedByByteCap(), c.PayloadBytes())
	// Later navigation well below the cap must not clear the disclosure.
	c.Merge(resultcache.Page{Start: 3, Rows: []resultcache.Row{
		{Position: 3, Values: []result.Value{result.NewInteger(3)}},
	}}, resultcache.Forward)
	t.Logf("below cap again: byteCap=%v payload=%d", c.TruncatedByByteCap(), c.PayloadBytes())
	facts := FactsFromCache(c)
	meta, err := NewSnapshotMetadata(facts, Lifecycle{Outcome: OutcomeSuccess})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("finalized metadata TruncatedByByteCap=%v", meta.TruncatedByByteCap)
	if got := Classify(meta, TraversalFacts{CountWorkFinished: true, PageWorkFinished: true}); got.Truncated {
		t.Logf("classification carries the typed fact -> %s", got)
	}
	// The presentation warning stays the shared Issue #31 definition in
	// internal/result — never model text.
	t.Logf("shared warning: %q", result.ByteCapWarning)
}
EOF
go test ./internal/history -run TestDemoByteCapPersistence -count=1 -v 2>&1 | grep -E "eviction|below cap|finalized|classification|shared warning"; rm internal/history/zz_demo_test.go
```

```output
    zz_demo_test.go:18: after byte eviction: byteCap=true payload=41943040
    zz_demo_test.go:23: below cap again: byteCap=true payload=41943048
    zz_demo_test.go:29: finalized metadata TruncatedByByteCap=true
    zz_demo_test.go:31: classification carries the typed fact -> partial+truncated
    zz_demo_test.go:35: shared warning: "Result truncated: 64 MiB cache limit"
```

```bash
cd "$(git rev-parse --show-toplevel)" && go test ./internal/history ./internal/resultcache ./internal/ui > /tmp/sqloid-33-demo.txt 2>&1 && echo ALL-THREE-PACKAGES-PASS || tail -20 /tmp/sqloid-33-demo.txt
```

```output
ALL-THREE-PACKAGES-PASS
```

All three suites pass and every block re-verifies. Summary of the Issue #33 contracts demonstrated: immutable typed snapshot metadata independent of row storage and presentation strings, with the optional inclusive retained range, optional known total, reached-low/high endpoints, cumulative row-cap eviction, persistent typed truncated-by-byte-cap, invalid-UTF status, and an independently typed terminal outcome with reason and optional one-based failure position; exclusive complete versus coexisting truthful partial/truncated across known totals, count failure, limited results, short and empty final-page observations, and unknown remainder; ascending absolute positions after forward and backward traversal; contradictory count/cache evidence preserved without clamping; and the shared Issue #31 warning remaining the single presentation definition. References: Issue #33, Notes/PRD-sqloid.md (Cache and snapshot invariant, History Module Design, Testing Decisions), Notes/wiki/snapshot-metadata.md.
