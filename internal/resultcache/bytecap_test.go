// Bidirectional 64 MiB retained-payload cap coverage for Issue #31, per the
// Cache and snapshot invariant of Notes/PRD-sqloid.md: the byte cap is
// independent of and cumulative with the 10,000-position cap; accepted
// forward pages evict complete rows from the low end and accepted backward
// pages evict complete rows from the high end until both caps hold and one
// contiguous retained range remains; overlap replacement re-prices retained
// bytes; byte-cap eviction sets persistent typed `truncated-by-byte-cap`
// metadata that survives later navigation below the cap; row-cap-only
// eviction never sets byte-cap metadata.

package resultcache

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// blobRow builds one row whose payload is a single BLOB of exactly sizeBytes
// bytes, so a page's payload total is exactly known.
func blobRow(pos Position, sizeBytes int) Row {
	return Row{Position: pos, Values: []result.Value{result.NewBlob(make([]byte, sizeBytes))}}
}

// blobPage builds a page occupying consecutive positions starting at start,
// with every row carrying a BLOB of exactly perRowBytes bytes.
func blobPage(start Position, rows, perRowBytes int) Page {
	out := make([]Row, rows)
	for i := range out {
		out[i] = blobRow(start+Position(i), perRowBytes)
	}
	return Page{Start: start, Rows: out}
}

// textRow builds one row whose payload is a single TEXT value of exactly n
// bytes, mixing value kinds across byte-cap fixtures.
func textRow(pos Position, n int) Row {
	return Row{Position: pos, Values: []result.Value{result.NewText(strings.Repeat("t", n))}}
}

// TestByteCapExactBoundary specifies the exact-boundary and one-byte cases in
// both traversal directions: payload exactly at MaxPayloadBytes retains
// everything and never records truncation; one byte more evicts complete rows
// from the opposite end of the incoming direction.
// TestByteCapExactBoundary specifies the exact-boundary and one-byte cases in
// both traversal directions, mixing page-envelope admission and byte-cap
// eviction: payload exactly at MaxPayloadBytes never records truncation, and
// one byte more fails typed at the first nonfitting position or evicts
// complete rows from the opposite end of the incoming direction.
func TestByteCapExactBoundary(t *testing.T) {
	half := int(MaxPayloadBytes / 2)
	t.Run("cumulative payload exactly at the cap evicts nothing and does not disclose", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, Page{Start: 1, Rows: []Row{blobRow(1, half)}}, Forward)
		mustMergeChecked(t, c, Page{Start: 2, Rows: []Row{blobRow(2, half)}}, Forward)
		assertRange(t, c, 1, 2)
		if got := c.PayloadBytes(); got != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes)
		}
		if c.TruncatedByByteCap() {
			t.Fatal("exact-cap retention disclosed truncation")
		}
	})

	t.Run("forward page one byte past the cap fails typed and evicts the low end", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, Page{Start: 1, Rows: []Row{blobRow(1, half)}}, Forward)
		page := Page{Start: 2, Rows: []Row{blobRow(2, half+1), blobRow(3, half)}}
		accepted, err := c.Merge(page, Forward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Kind != result.KindPage || failure.Position != 3 {
			t.Fatalf("Merge error = %v, want *result.LimitFailure{KindPage, position 3}", err)
		}
		if got := err.Error(); got != "result page exceeds the 64 MiB v1 limit at row 3" {
			t.Fatalf("error text = %q, want exact shared message", got)
		}
		if !accepted {
			t.Fatal("Merge rejected, want the fitting leading rows admitted")
		}
		if got := c.PayloadBytes(); got != MaxPayloadBytes+1-int64(half) {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes+1-int64(half))
		}
		assertContiguousBounded(t, c)
	})

	t.Run("backward page one byte past the cap fails typed and evicts the high end", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, Page{Start: 2, Rows: []Row{blobRow(2, 4), blobRow(3, half)}}, Forward)
		page := Page{Start: 1, Rows: []Row{blobRow(1, half+1), blobRow(2, half)}}
		accepted, err := c.Merge(page, Backward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Kind != result.KindPage || failure.Position != 1 {
			t.Fatalf("Merge error = %v, want *result.LimitFailure{KindPage, position 1}", err)
		}
		if !accepted {
			t.Fatal("Merge rejected, want the fitting rows admitted")
		}
		// In traversal order (row 2 first) only row 2 fits; it merges with the
		// retained rows and the total is exactly the boundary.
		assertRange(t, c, 2, 3)
		if got := c.PayloadBytes(); got != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes)
		}
		if got, _ := c.End(); got != 3 {
			t.Fatalf("End() = %d, want 3 (prior rows kept)", got)
		}
		assertContiguousBounded(t, c)
	})
}

// TestByteCapEvictionDirection specifies complete-row opposite-end eviction
// once the byte cap is crossed: forward merges shed the low end, backward
// merges shed the high end, the retained range stays contiguous, exact BLOB
// bytes survive in surviving rows, and totals stay exact.
func TestByteCapEvictionDirection(t *testing.T) {
	// Seed: two rows of half the cap each (rows 1, 2). Then merge one more
	// half-sized row; forward evicts row 1 (low), backward (as a page below
	// rows 1..2) evicts row 2 (high).
	seedRowSize := int(MaxPayloadBytes / 2)

	t.Run("forward crossing evicts complete low rows", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(1, 2, seedRowSize), Forward)
		mustMergeChecked(t, c, Page{Start: 3, Rows: []Row{blobRow(3, seedRowSize)}}, Forward)
		assertRange(t, c, 2, 3)
		if got := c.PayloadBytes(); got != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes)
		}
		if !c.TruncatedByByteCap() {
			t.Fatal("byte-cap eviction did not record truncated-by-byte-cap")
		}
		rows := c.Rows()
		if len(rows) != 2 {
			t.Fatalf("retained %d rows, want complete rows only", len(rows))
		}
		if got := rows[0].Values[0]; got.Kind != result.KindBlob || len(got.Bytes) != seedRowSize {
			t.Fatalf("surviving low row not a complete BLOB row: %+v", got)
		}
	})

	t.Run("backward crossing evicts complete high rows", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(2, 2, seedRowSize), Forward)
		mustMergeChecked(t, c, Page{Start: 1, Rows: []Row{blobRow(1, seedRowSize)}}, Backward)
		assertRange(t, c, 1, 2)
		if got := c.PayloadBytes(); got != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes)
		}
		if !c.TruncatedByByteCap() {
			t.Fatal("byte-cap eviction did not record truncated-by-byte-cap")
		}
		if got, _ := c.End(); got != 2 {
			t.Fatalf("End() = %d, want 2 (high end evicted, low end kept)", got)
		}
	})

	t.Run("both caps hold cumulatively with the position cap", func(t *testing.T) {
		// One page of MaxPositions+5 tiny rows crosses the position cap but
		// stays far below the byte cap: eviction is row-cap only and byte
		// metadata must not appear.
		c := New()
		rows := make([]Row, MaxPositions+5)
		for i := range rows {
			rows[i] = textRow(Position(i+1), 4)
		}
		mustMergeChecked(t, c, Page{Start: 1, Rows: rows}, Forward)
		assertRange(t, c, 6, Position(MaxPositions+5))
		if c.TruncatedByByteCap() {
			t.Fatal("row-cap-only eviction set byte-cap metadata")
		}
		if c.PayloadBytes() != int64(MaxPositions)*4 {
			t.Fatalf("PayloadBytes() = %d, want %d", c.PayloadBytes(), int64(MaxPositions)*8)
		}
	})

	t.Run("byte cap and position cap hold together", func(t *testing.T) {
		// A full position-cap range of tiny rows plus a big two-row page:
		// the position cap evicts prior rows, the byte cap evicts the rest
		// of the prior retention, and both caps hold with a contiguous range.
		c := New()
		rows := make([]Row, MaxPositions)
		for i := range rows {
			rows[i] = textRow(Position(i+1), 4)
		}
		mustMergeChecked(t, c, Page{Start: 1, Rows: rows}, Forward)
		big := int(MaxPayloadBytes / 2)
		page := Page{Start: Position(MaxPositions + 1), Rows: []Row{blobRow(MaxPositions+1, big), blobRow(MaxPositions+2, big)}}
		mustMergeChecked(t, c, page, Forward)
		assertRange(t, c, Position(MaxPositions+1), Position(MaxPositions+2))
		if got := c.PayloadBytes(); got != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", got, MaxPayloadBytes)
		}
		if !c.TruncatedByByteCap() {
			t.Fatal("byte eviction did not disclose truncated-by-byte-cap")
		}
	})

	t.Run("disclosure persists after later navigation falls below the cap", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(1, 2, int(MaxPayloadBytes/2)), Forward)
		mustMergeChecked(t, c, Page{Start: 3, Rows: []Row{blobRow(3, int(MaxPayloadBytes/4)+1)}}, Forward)
		assertRange(t, c, 2, 3)
		if !c.TruncatedByByteCap() {
			t.Fatal("byte eviction did not disclose truncated-by-byte-cap")
		}
		// Replace the big row with a tiny one: payload falls below the cap.
		mustMergeChecked(t, c, Page{Start: 3, Rows: []Row{blobRow(3, 4)}}, Forward)
		if c.PayloadBytes() >= MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want below cap after replacement", c.PayloadBytes())
		}
		if !c.TruncatedByByteCap() {
			t.Fatal("truncated-by-byte-cap metadata cleared after falling below the cap")
		}
	})

	t.Run("overlap replacement raising retained bytes evicts and discloses", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(1, 4, int(MaxPayloadBytes/4)), Forward) // exactly at cap
		if c.PayloadBytes() != MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d, want %d", c.PayloadBytes(), MaxPayloadBytes)
		}
		if c.TruncatedByByteCap() {
			t.Fatal("exact-cap retention disclosed truncation")
		}
		// Overlap-replace rows 2..3 with rows half the cap bigger in total.
		mustMergeChecked(t, c, Page{Start: 2, Rows: []Row{blobRow(2, int(MaxPayloadBytes/2))}}, Forward)
		if !c.TruncatedByByteCap() {
			t.Fatal("byte eviction from overlap replacement did not disclose")
		}
		if c.PayloadBytes() > MaxPayloadBytes {
			t.Fatalf("PayloadBytes() = %d exceeds cap after replacement", c.PayloadBytes())
		}
		assertContiguousBounded(t, c)
	})

	t.Run("stale gap page after byte eviction rejected atomically", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(1, 2, int(MaxPayloadBytes/2)), Forward)
		mustMergeChecked(t, c, Page{Start: 3, Rows: []Row{blobRow(3, int(MaxPayloadBytes/4)+1)}}, Forward)
		assertRange(t, c, 2, 3) // byte eviction removed row 1
		before := c.Rows()
		if accepted, err := c.Merge(blobPage(Position(MaxPayloadBytes)+100, 2, 8), Forward); accepted || err != nil {
			t.Fatal("nonadjacent stale page accepted after byte eviction")
		}
		after := c.Rows()
		if len(before) != len(after) {
			t.Fatalf("rejected merge changed cache size: %d -> %d", len(before), len(after))
		}
		for i := range before {
			if before[i].Position != after[i].Position || !bytes.Equal(before[i].Values[0].Bytes, after[i].Values[0].Bytes) {
				t.Fatalf("rejected merge mutated row %d", i)
			}
		}
	})
}
