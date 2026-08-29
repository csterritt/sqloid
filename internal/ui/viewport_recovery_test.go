// Table-driven pure coverage for the resize-safe vertical viewport recovery
// decisions (Issue #32 Task 1), per the SELECT lifecycle, Cache and snapshot
// invariant, Module Design, and resize Testing Decisions in
// Notes/PRD-sqloid.md. Each case supplies the prior first logical row, the
// exact newly computed page size, authoritative post-eviction retained-range
// and dual-cap metadata derived from internal/resultcache caches, and the
// known endpoint state, and requires exactly one explicit decision: preserve
// the exact row while it remains valid and retained; clamp to the known
// retained low/high endpoint at an established result boundary; or request
// the absolute containing page of the target. No Bubble Tea commands and no
// database dispatch appear here — this is a pure decision seam.
package ui

import (
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// cacheOf builds a populated contiguous resultcache from adjacent pages of
// rowCounts rows each, starting at the one-based absolute position start.
// Every row carries one INTEGER payload so the byte total is exact.
func cacheOf(t *testing.T, start int64, rowCounts ...int) *resultcache.Cache {
	t.Helper()
	c := resultcache.New()
	pos := start
	for _, n := range rowCounts {
		page := resultcache.Page{Start: resultcache.Position(pos)}
		for i := 0; i < n; i++ {
			page.Rows = append(page.Rows, resultcache.Row{
				Position: resultcache.Position(pos),
				Values:   []result.Value{result.NewInteger(pos)},
			})
			pos++
		}
		accepted, err := c.Merge(page, resultcache.Forward)
		if !accepted || err != nil {
			t.Fatalf("cacheOf: merge of page at %d rejected: accepted=%v err=%v", start, accepted, err)
		}
	}
	return c
}

// TestRecoverViewportPreservesExactPriorRowWhileRetained requires the exact
// prior first logical row to be preserved — including at both exact range
// edges — whenever it remains valid and retained in the post-eviction range.
func TestRecoverViewportPreservesExactPriorRowWhileRetained(t *testing.T) {
	cache := cacheOf(t, 5, 10) // retained 5..14
	tests := []struct {
		name  string
		prior int64
	}{
		{"prior first retained row", 5},
		{"prior middle row", 9},
		{"prior last retained row", 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), tt.prior, 35)
			if got.Action != RecoveryPreserve || got.FirstRow != tt.prior {
				t.Fatalf("prior %d: got (%v, first row %d), want preserve %d", tt.prior, got.Action, got.FirstRow, tt.prior)
			}
			if got.Size != 35 {
				t.Fatalf("preserve size = %d, want the exact new page size 35", got.Size)
			}
		})
	}
}

// TestRecoverViewportSingleRowRange covers a single-row retained range on
// every side: inside, below with an established boundary, above unknown, and
// above with the high endpoint established.
func TestRecoverViewportSingleRowRange(t *testing.T) {
	cache := cacheOf(t, 7, 1) // retained 7..7
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 7, 3); got.Action != RecoveryPreserve || got.FirstRow != 7 {
		t.Fatalf("single-row range: got (%v, %d), want preserve 7", got.Action, got.FirstRow)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, true, false, 0), 3, 3); got.Action != RecoveryClampLow || got.FirstRow != 7 {
		t.Fatalf("single-row low clamp: got (%v, %d), want clamp-low 7", got.Action, got.FirstRow)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 8, 3); got.Action != RecoveryFetch || got.FirstRow != 7 || got.Size != 3 {
		t.Fatalf("single-row unknown high: got (%v, %d, size %d), want fetch containing page 7 size 3", got.Action, got.FirstRow, got.Size)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, true, 0), 8, 3); got.Action != RecoveryClampHigh || got.FirstRow != 7 {
		t.Fatalf("single-row established high: got (%v, %d), want clamp-high 7", got.Action, got.FirstRow)
	}
}

// TestRecoverViewportClampsToKnownRetainedEndpoints requires clamping to the
// retained low endpoint for targets below the range and to the retained high
// endpoint only when that endpoint is established, never clamping to any
// position derived from an inconsistent count.
func TestRecoverViewportClampsToKnownRetainedEndpoints(t *testing.T) {
	cache := cacheOf(t, 5, 10) // retained 5..14
	if got := RecoverViewport(ViewportMetaFromCache(cache, true, false, 0), 4, 20); got.Action != RecoveryClampLow || got.FirstRow != 5 {
		t.Fatalf("below range: got (%v, %d), want clamp-low 5", got.Action, got.FirstRow)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, true, 0), 15, 20); got.Action != RecoveryClampHigh || got.FirstRow != 14 {
		t.Fatalf("above established range: got (%v, %d), want clamp-high 14", got.Action, got.FirstRow)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 15, 20); got.Action != RecoveryFetch || got.FirstRow != 1 {
		t.Fatalf("above unknown range: got (%v, %d), want fetch the new-size page containing 15 (start 1)", got.Action, got.FirstRow)
	}
	// A count larger than the retained end never establishes the high
	// endpoint: an inconsistent count is never used to clamp.
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, true, 1000), 15, 20); got.Action != RecoveryFetch {
		t.Fatalf("inconsistent count misuse: got %v, want fetch — a count beyond the end must not clamp", got.Action)
	}
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 1000, 20); got.Action != RecoveryFetch {
		t.Fatalf("target above an oversized count: got %v, want fetch — the count must not clamp", got.Action)
	}
}

// TestRecoverViewportCountEstablishesHighEndpointOnlyWithinRange requires a
// known count to establish the high boundary only when the retained end
// reaches it; a count beyond the retained end leaves the boundary unknown.
func TestRecoverViewportCountEstablishesHighEndpointOnlyWithinRange(t *testing.T) {
	cache := cacheOf(t, 5, 10) // retained 5..14
	// Count 14 ≤ End 14: the retained end is the final row.
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, true, 14), 15, 20); got.Action != RecoveryClampHigh || got.FirstRow != 14 {
		t.Fatalf("count at end: got (%v, %d), want clamp-high 14", got.Action, got.FirstRow)
	}
	// Count 40 > End 14: rows 15..40 are unseen, so the boundary is unknown.
	if got := RecoverViewport(ViewportMetaFromCache(cache, false, true, 40), 15, 20); got.Action != RecoveryFetch || got.FirstRow != 1 {
		t.Fatalf("count beyond end: got (%v, %d), want fetch the new-size page containing 15", got.Action, got.FirstRow)
	}
}

// TestRecoverViewportEmptyCacheIsDeterministic requires empty (or absent)
// metadata to produce one deterministic safe decision: request the absolute
// containing page of the target with the exact new page size.
func TestRecoverViewportEmptyCacheIsDeterministic(t *testing.T) {
	empty := resultcache.New()
	tests := []struct {
		name    string
		meta    ViewportMeta
		prior   int64
		newSize int64
	}{
		{"empty cache fetches the prior row's page", ViewportMetaFromCache(empty, true, false, 0), 1, 30},
		{"empty cache fetches a high prior row's page", ViewportMetaFromCache(empty, true, false, 0), 40, 30},
		{"zero metadata fetches deterministically", ViewportMeta{}, 1, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecoverViewport(tt.meta, tt.prior, tt.newSize)
			start := (tt.prior-1)/tt.newSize*tt.newSize + 1
			if start < 1 {
				start = 1
			}
			if got.Action != RecoveryFetch || got.FirstRow != start || got.Size != tt.newSize {
				t.Fatalf("prior %d: got (%v, first %d, size %d), want fetch of containing page %d at exact size %d", tt.prior, got.Action, got.FirstRow, got.Size, start, tt.newSize)
			}
		})
	}
}

// TestRecoverViewportRowCapEvictionMetadata exercises decisions against a
// cache that evicted from the low end under the 10,000-position cap: rows
// below the retained start clamp to the retained low endpoint while rows
// inside the retained range keep their exact positions, and the eviction
// metadata flows through to the decision inputs.
func TestRecoverViewportRowCapEvictionMetadata(t *testing.T) {
	c := cacheOf(t, 1, 10000, 1000) // forward merge evicts 1..1000
	if got := c.RowCapEvictions(); got != 1000 {
		t.Fatalf("setup: row-cap evictions = %d, want 1000", got)
	}
	if start, _ := c.Start(); start != 1001 {
		t.Fatalf("setup: retained start = %v, want 1001", start)
	}
	meta := ViewportMetaFromCache(c, false, false, 0)
	if meta.Start != 1001 || meta.End != 11000 || !meta.HasRows || meta.RowCapEvictions != 1000 {
		t.Fatalf("metadata = %+v, want range 1001..11000 with 1000 evictions", meta)
	}
	if got := RecoverViewport(meta, 10500, 25); got.Action != RecoveryPreserve || got.FirstRow != 10500 {
		t.Fatalf("retained prior row: got (%v, %d), want preserve 15000", got.Action, got.FirstRow)
	}
	if got := RecoverViewport(meta, 500, 25); got.Action != RecoveryClampLow || got.FirstRow != 1001 {
		t.Fatalf("evicted prior row: got (%v, %d), want clamp-low 1001", got.Action, got.FirstRow)
	}
}

// TestRecoverViewportByteCapEvictionEquivalence requires the byte cap
// (Issue #31) to produce decisions identical to the row cap for the same
// retained range: the dual caps share one contiguous retained range, so a
// prior row evicted under either cap clamps to the same known endpoint.
func TestRecoverViewportByteCapEvictionEquivalence(t *testing.T) {
	// Four 20 MiB rows exceed the 64 MiB envelope, so the byte cap evicts
	// from the low end until 3 rows (60 MiB) remain: retained 2..4.
	big := strings.Repeat("x", 20<<20)
	c2 := resultcache.New()
	for i := int64(1); i <= 4; i++ {
		page := resultcache.Page{Start: resultcache.Position(i), Rows: []resultcache.Row{{
			Position: resultcache.Position(i),
			Values:   []result.Value{result.NewText(big)},
		}}}
		if accepted, _ := c2.Merge(page, resultcache.Forward); !accepted {
			t.Fatalf("setup: oversized byte-cap merge of row %d unexpectedly rejected", i)
		}
	}
	if !c2.TruncatedByByteCap() {
		t.Fatal("setup: byte-cap eviction did not set the persistent disclosure")
	}
	start, _ := c2.Start()
	if start != 2 {
		t.Fatalf("setup: byte-cap retained start = %v, want 2", start)
	}
	rowCap := RecoverViewport(ViewportMetaFromCache(cacheOf(t, 2, 3), true, false, 0), 1, 10) // retained 2..4
	byteCap := RecoverViewport(ViewportMetaFromCache(c2, true, false, 0), 1, 10)
	if byteCap.Action != RecoveryClampLow || byteCap.FirstRow != 2 {
		t.Fatalf("byte-cap evicted prior row: got (%v, %d), want clamp-low 2", byteCap.Action, byteCap.FirstRow)
	}
	if byteCap.Action != rowCap.Action || byteCap.FirstRow != rowCap.FirstRow {
		t.Fatalf("dual-cap divergence: byte cap gave (%v, %d), row cap gave (%v, %d)",
			byteCap.Action, byteCap.FirstRow, rowCap.Action, rowCap.FirstRow)
	}
}

// TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize
// requires every fetch decision to name the absolute one-based logical
// position of the target as the request's first row with exactly the newly
// computed page size — never a page-relative offset and never a stale size.
func TestRecoverViewportContainingPageUsesAbsolutePositionsAndExactSize(t *testing.T) {
	cache := cacheOf(t, 5, 10) // retained 5..14
	tests := []struct {
		name           string
		prior, newSize int64
		wantStart      int64
	}{
		{"one row above the range", 15, 9, 10},
		{"far above the range", 444, 1, 444},
		{"page-grid boundary target", 21, 10, 21},
		{"mid-page target", 23, 10, 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), tt.prior, tt.newSize)
			if got.Action != RecoveryFetch || got.FirstRow != tt.wantStart || got.Size != tt.newSize {
				t.Fatalf("prior %d: got (%v, first %d, size %d), want fetch of containing page %d at exact size %d",
					tt.prior, got.Action, got.FirstRow, got.Size, tt.wantStart, tt.newSize)
			}
		})
	}
}

// TestRecoverViewportNonpositivePageSizeIsDeterministic requires a zero or
// negative newly computed page size — no complete data row fits — to
// downgrade the request size deterministically to one row without changing
// the decision itself.
func TestRecoverViewportNonpositivePageSizeIsDeterministic(t *testing.T) {
	cache := cacheOf(t, 5, 10)
	for _, newSize := range []int64{0, -3} {
		if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 15, newSize); got.Action != RecoveryFetch || got.FirstRow != 15 || got.Size != 1 {
			t.Fatalf("newSize %d: got (%v, %d, size %d), want fetch of the one-row page containing 15", newSize, got.Action, got.FirstRow, got.Size)
		}
		if got := RecoverViewport(ViewportMetaFromCache(cache, false, false, 0), 7, newSize); got.Action != RecoveryPreserve || got.FirstRow != 7 || got.Size != 1 {
			t.Fatalf("newSize %d: got (%v, %d, size %d), want preserve 7 size 1", newSize, got.Action, got.FirstRow, got.Size)
		}
	}
}
