// Issue #81 cache-owned BLOB isolation coverage: the result cache must own
// admitted BLOB bytes and return independently mutable row snapshots from
// every Rows() call. Mutating the original page's BLOB slice after admission
// must not reach the cache, and mutating BLOB slices returned by successive
// Rows() calls must not corrupt later retrievals or the cache itself. NULL,
// INTEGER, REAL, and TEXT values, Row.Position, the values slice shape, every
// value kind, ascending ordering, contiguity, PayloadBytes(), the retained
// range, and the row/byte-cap eviction metadata all remain exact. Both
// initial insertion and every overlap-replacement path that transfers a page
// row into cache ownership are exercised.

package resultcache

import (
	"bytes"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// sharedBlob builds a KindBlob value whose Bytes slice is exactly blobBytes
// with no copy, so the caller and the value share backing storage. This
// bypasses result.NewBlob's defensive copy to exercise the cache's own
// ownership boundary: the cache must deep-copy admitted BLOB bytes regardless
// of how the page row's value was constructed.
func sharedBlob(blobBytes []byte) result.Value {
	return result.Value{Kind: result.KindBlob, Bytes: blobBytes}
}

// mixedRow builds one row at pos carrying one value of each kind: NULL,
// INTEGER, REAL, TEXT, and a BLOB whose backing slice is blobBytes (shared,
// not copied). The caller retains the blobBytes slice so it can mutate it
// after admission to prove the cache does not alias caller storage.
func mixedRow(pos Position, blobBytes []byte) Row {
	return Row{Position: pos, Values: []result.Value{
		result.NewNull(),
		result.NewInteger(int64(pos)),
		result.NewReal(float64(pos) + 0.5),
		result.NewText("text-" + pos.String()),
		sharedBlob(blobBytes),
	}}
}

// assertMixedRow asserts that row carries the exact mixed payload originally
// built by mixedRow for pos with originalBlobBytes, preserving every kind,
// the non-BLOB fields, the BLOB bytes, and the position. It does not accept
// shared backing storage with originalBlobBytes: a deep copy is required.
func assertMixedRow(t *testing.T, row Row, pos Position, originalBlobBytes []byte) {
	t.Helper()
	if row.Position != pos {
		t.Fatalf("row position = %d, want %d", row.Position, pos)
	}
	if len(row.Values) != 5 {
		t.Fatalf("row at %d has %d values, want 5 (one per kind)", pos, len(row.Values))
	}
	if got := row.Values[0]; got.Kind != result.KindNull {
		t.Fatalf("row %d value 0 kind = %v, want KindNull", pos, got.Kind)
	}
	if got := row.Values[1]; got.Kind != result.KindInteger || got.Int != int64(pos) {
		t.Fatalf("row %d value 1 = %+v, want KindInteger %d", pos, got, int64(pos))
	}
	if got := row.Values[2]; got.Kind != result.KindReal || got.Float != float64(pos)+0.5 {
		t.Fatalf("row %d value 2 = %+v, want KindReal %v", pos, got, float64(pos)+0.5)
	}
	if got := row.Values[3]; got.Kind != result.KindText || got.Str != "text-"+pos.String() {
		t.Fatalf("row %d value 3 = %+v, want KindText %q", pos, got, "text-"+pos.String())
	}
	got := row.Values[4]
	if got.Kind != result.KindBlob {
		t.Fatalf("row %d value 4 kind = %v, want KindBlob", pos, got.Kind)
	}
	if !bytes.Equal(got.Bytes, originalBlobBytes) {
		t.Fatalf("row %d BLOB = %v, want original bytes %v", pos, got.Bytes, originalBlobBytes)
	}
	// Non-aliasing: the retained BLOB must not share backing storage with the
	// caller's original slice. Mutating the retained copy must not affect the
	// original.
	if len(got.Bytes) > 0 && len(originalBlobBytes) > 0 && &got.Bytes[0:1][0] == &originalBlobBytes[0:1][0] {
		t.Fatalf("row %d BLOB aliases caller storage; cache must own a deep copy", pos)
	}
}

// wantMixedPayload is the exact RowPayload total for one mixedRow whose BLOB
// is len(blobBytes) bytes: NULL 0 + INTEGER 8 + REAL 8 + TEXT len("text-N") +
// BLOB len(blobBytes).
func wantMixedPayload(pos Position, blobBytes []byte) int64 {
	return 0 + 8 + 8 + int64(len("text-"+pos.String())) + int64(len(blobBytes))
}

// TestBlobIsolationOnAdmission proves the cache deep-copies BLOB bytes when a
// page row is accepted into cache-owned storage on initial insertion: mutating
// the original page's BLOB slice after Merge must not change any retained row,
// every value kind and non-BLOB field stays exact, positions and ordering are
// preserved, and PayloadBytes plus the retained range remain unchanged.
func TestBlobIsolationOnAdmission(t *testing.T) {
	c := New()
	blob1 := []byte{0x00, 0x01, 0x02, 0xFE, 0xFF}
	blob2 := []byte("second-blob-payload")
	page := Page{Start: 7, Rows: []Row{
		mixedRow(7, blob1),
		mixedRow(8, blob2),
	}}
	wantPayload := wantMixedPayload(7, blob1) + wantMixedPayload(8, blob2)
	if accepted, err := c.Merge(page, Forward); err != nil || !accepted {
		t.Fatalf("initial merge = (%v, %v), want accepted with no error", accepted, err)
	}

	// Mutate the caller's original BLOB slices after admission. The cache must
	// be unaffected: it owns deep copies of the admitted bytes.
	for i := range blob1 {
		blob1[i] = 0xAA
	}
	for i := range blob2 {
		blob2[i] = 'Z'
	}

	assertRange(t, c, 7, 8)
	assertContiguousBounded(t, c)
	if got := c.PayloadBytes(); got != wantPayload {
		t.Fatalf("PayloadBytes() = %d, want %d after caller mutation", got, wantPayload)
	}
	rows := c.Rows()
	if len(rows) != 2 {
		t.Fatalf("Rows() = %d rows, want 2", len(rows))
	}
	assertMixedRow(t, rows[0], 7, []byte{0x00, 0x01, 0x02, 0xFE, 0xFF})
	assertMixedRow(t, rows[1], 8, []byte("second-blob-payload"))
	if c.TruncatedByByteCap() {
		t.Fatal("byte-cap disclosure set without any byte eviction")
	}
	if got := c.RowCapEvictions(); got != 0 {
		t.Fatalf("RowCapEvictions() = %d, want 0", got)
	}
}

// TestBlobIsolationOnRowsRetrieval proves every Cache.Rows() result receives
// an independent deep copy of the retained BLOB bytes: mutating a returned
// BLOB slice must not corrupt the cache or any later Rows() retrieval, while
// positions, kinds, non-BLOB values, ordering, and PayloadBytes stay exact.
func TestBlobIsolationOnRowsRetrieval(t *testing.T) {
	c := New()
	blob := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60}
	original := append([]byte(nil), blob...)
	if accepted, err := c.Merge(Page{Start: 1, Rows: []Row{mixedRow(1, blob)}}, Forward); err != nil || !accepted {
		t.Fatalf("merge = (%v, %v)", accepted, err)
	}
	wantPayload := wantMixedPayload(1, original)

	first := c.Rows()
	if len(first) != 1 {
		t.Fatalf("first Rows() = %d rows, want 1", len(first))
	}
	assertMixedRow(t, first[0], 1, original)

	// Mutate the BLOB returned by the first Rows() call. The cache and every
	// later retrieval must keep the original bytes.
	if len(first[0].Values[4].Bytes) > 0 {
		for i := range first[0].Values[4].Bytes {
			first[0].Values[4].Bytes[i] = 0xEE
		}
	}

	second := c.Rows()
	if len(second) != 1 {
		t.Fatalf("second Rows() = %d rows, want 1", len(second))
	}
	assertMixedRow(t, second[0], 1, original)
	if got := c.PayloadBytes(); got != wantPayload {
		t.Fatalf("PayloadBytes() = %d, want %d after mutating a returned BLOB", got, wantPayload)
	}
	assertRange(t, c, 1, 1)
	assertContiguousBounded(t, c)

	// Mutate the second retrieval too and pull a third to prove each retrieval
	// is independent of both the cache and every other retrieval.
	if len(second[0].Values[4].Bytes) > 0 {
		second[0].Values[4].Bytes[0] = 0x99
	}
	third := c.Rows()
	assertMixedRow(t, third[0], 1, original)
}

// TestBlobIsolationOnForwardAppend proves the forward-append admission path
// deep-copies BLOB bytes: after seeding the cache, appending an adjacent
// forward page whose BLOB is then mutated must leave the cache holding the
// originally admitted bytes, with positions, ordering, kinds, and payload
// accounting all exact.
func TestBlobIsolationOnForwardAppend(t *testing.T) {
	c := New()
	mustMerge(t, c, page(1, "seed1", "seed2"), Forward)
	appended := []byte("appended-blob-bytes")
	original := append([]byte(nil), appended...)
	p := Page{Start: 3, Rows: []Row{mixedRow(3, appended)}}
	wantExtra := wantMixedPayload(3, original)
	wantPayload := int64(len("seed1")) + int64(len("seed2")) + wantExtra
	if accepted, err := c.Merge(p, Forward); err != nil || !accepted {
		t.Fatalf("append merge = (%v, %v)", accepted, err)
	}
	for i := range appended {
		appended[i] = 'X'
	}
	assertRange(t, c, 1, 3)
	assertContiguousBounded(t, c)
	if got := c.PayloadBytes(); got != wantPayload {
		t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
	}
	rows := c.Rows()
	if len(rows) != 3 {
		t.Fatalf("Rows() = %d rows, want 3", len(rows))
	}
	if rows[0].Values[0].Str != "seed1" || rows[1].Values[0].Str != "seed2" {
		t.Fatalf("seed rows altered by append: %q, %q", rows[0].Values[0].Str, rows[1].Values[0].Str)
	}
	assertMixedRow(t, rows[2], 3, original)
}

// TestBlobIsolationOnBackwardPrepend proves the backward-prepend admission
// path deep-copies BLOB bytes: prepending an adjacent backward page whose
// BLOB is then mutated must leave the cache holding the originally admitted
// bytes, with ascending ordering, positions, kinds, and payload accounting
// all exact.
func TestBlobIsolationOnBackwardPrepend(t *testing.T) {
	c := New()
	mustMerge(t, c, page(4, "seed4", "seed5"), Forward)
	prepended := []byte("prepended-blob")
	original := append([]byte(nil), prepended...)
	p := Page{Start: 1, Rows: []Row{
		mixedRow(1, []byte("first-blob")),
		mixedRow(2, prepended),
		mixedRow(3, []byte("third-blob")),
	}}
	wantExtra := wantMixedPayload(1, []byte("first-blob")) +
		wantMixedPayload(2, original) +
		wantMixedPayload(3, []byte("third-blob"))
	wantPayload := int64(len("seed4")) + int64(len("seed5")) + wantExtra
	if accepted, err := c.Merge(p, Backward); err != nil || !accepted {
		t.Fatalf("prepend merge = (%v, %v)", accepted, err)
	}
	for i := range prepended {
		prepended[i] = 'Q'
	}
	assertRange(t, c, 1, 5)
	assertContiguousBounded(t, c)
	if got := c.PayloadBytes(); got != wantPayload {
		t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
	}
	rows := c.Rows()
	if len(rows) != 5 {
		t.Fatalf("Rows() = %d rows, want 5", len(rows))
	}
	if rows[3].Values[0].Str != "seed4" || rows[4].Values[0].Str != "seed5" {
		t.Fatalf("seed rows altered by prepend: %q, %q", rows[3].Values[0].Str, rows[4].Values[0].Str)
	}
	assertMixedRow(t, rows[0], 1, []byte("first-blob"))
	assertMixedRow(t, rows[1], 2, original)
	assertMixedRow(t, rows[2], 3, []byte("third-blob"))
}

// TestBlobIsolationOnOverlapReplacement proves every overlap-replacement
// admission path deep-copies the replacing BLOB bytes: after replacing a
// retained row with a page row carrying a new BLOB, mutating the replacement
// page's BLOB must leave the cache holding the originally admitted replacement
// bytes, with the replaced position, kinds, ordering, and payload accounting
// all exact. Both partial low-edge and high-edge overlaps are covered.
func TestBlobIsolationOnOverlapReplacement(t *testing.T) {
	t.Run("partial overlap at the high edge", func(t *testing.T) {
		c := New()
		mustMerge(t, c, page(1, "v1", "v2", "v3"), Forward)
		replacement := []byte("replace-high-edge-blob")
		original := append([]byte(nil), replacement...)
		p := Page{Start: 2, Rows: []Row{
			mixedRow(2, replacement),
			mixedRow(3, []byte("other-blob")),
			mixedRow(4, []byte("new-row-blob")),
		}}
		wantExtra := wantMixedPayload(2, original) +
			wantMixedPayload(3, []byte("other-blob")) +
			wantMixedPayload(4, []byte("new-row-blob"))
		wantPayload := int64(len("v1")) + wantExtra
		if accepted, err := c.Merge(p, Forward); err != nil || !accepted {
			t.Fatalf("overlap merge = (%v, %v)", accepted, err)
		}
		for i := range replacement {
			replacement[i] = '!'
		}
		assertRange(t, c, 1, 4)
		assertContiguousBounded(t, c)
		if got := c.PayloadBytes(); got != wantPayload {
			t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
		}
		rows := c.Rows()
		if len(rows) != 4 {
			t.Fatalf("Rows() = %d rows, want 4", len(rows))
		}
		if rows[0].Values[0].Str != "v1" {
			t.Fatalf("untouched low row altered: %q", rows[0].Values[0].Str)
		}
		assertMixedRow(t, rows[1], 2, original)
		assertMixedRow(t, rows[2], 3, []byte("other-blob"))
		assertMixedRow(t, rows[3], 4, []byte("new-row-blob"))
	})

	t.Run("partial overlap at the low edge", func(t *testing.T) {
		c := New()
		mustMerge(t, c, page(3, "v3", "v4", "v5"), Forward)
		replacement := []byte("replace-low-edge-blob")
		original := append([]byte(nil), replacement...)
		p := Page{Start: 1, Rows: []Row{
			mixedRow(1, []byte("new-low-blob")),
			mixedRow(2, []byte("also-new-blob")),
			mixedRow(3, replacement),
		}}
		wantExtra := wantMixedPayload(1, []byte("new-low-blob")) +
			wantMixedPayload(2, []byte("also-new-blob")) +
			wantMixedPayload(3, original)
		wantPayload := wantExtra + int64(len("v4")) + int64(len("v5"))
		if accepted, err := c.Merge(p, Backward); err != nil || !accepted {
			t.Fatalf("overlap merge = (%v, %v)", accepted, err)
		}
		for i := range replacement {
			replacement[i] = '#'
		}
		assertRange(t, c, 1, 5)
		assertContiguousBounded(t, c)
		if got := c.PayloadBytes(); got != wantPayload {
			t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
		}
		rows := c.Rows()
		if len(rows) != 5 {
			t.Fatalf("Rows() = %d rows, want 5", len(rows))
		}
		assertMixedRow(t, rows[0], 1, []byte("new-low-blob"))
		assertMixedRow(t, rows[1], 2, []byte("also-new-blob"))
		assertMixedRow(t, rows[2], 3, original)
		if rows[3].Values[0].Str != "v4" || rows[4].Values[0].Str != "v5" {
			t.Fatalf("untouched high rows altered: %q, %q", rows[3].Values[0].Str, rows[4].Values[0].Str)
		}
	})

	t.Run("exact full-range replacement", func(t *testing.T) {
		c := New()
		mustMerge(t, c, page(1, "old1", "old2"), Forward)
		r1 := []byte("replacement-one")
		r2 := []byte("replacement-two")
		o1 := append([]byte(nil), r1...)
		o2 := append([]byte(nil), r2...)
		p := Page{Start: 1, Rows: []Row{
			mixedRow(1, r1),
			mixedRow(2, r2),
		}}
		wantPayload := wantMixedPayload(1, o1) + wantMixedPayload(2, o2)
		if accepted, err := c.Merge(p, Forward); err != nil || !accepted {
			t.Fatalf("overlap merge = (%v, %v)", accepted, err)
		}
		for i := range r1 {
			r1[i] = 'A'
		}
		for i := range r2 {
			r2[i] = 'B'
		}
		assertRange(t, c, 1, 2)
		if got := c.PayloadBytes(); got != wantPayload {
			t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
		}
		rows := c.Rows()
		if len(rows) != 2 {
			t.Fatalf("Rows() = %d rows, want 2", len(rows))
		}
		assertMixedRow(t, rows[0], 1, o1)
		assertMixedRow(t, rows[1], 2, o2)
	})
}

// TestBlobIsolationSurvivesRowsMutationAcrossRetrievals proves that successive
// Rows() results are mutually independent: mutating BLOB slices in an earlier
// retrieval does not affect a later retrieval even when no caller mutation of
// the original page happens, exercising the second ownership boundary
// (Rows() deep copy) independently of the first (admission deep copy).
func TestBlobIsolationSurvivesRowsMutationAcrossRetrievals(t *testing.T) {
	c := New()
	blobs := [][]byte{
		[]byte("blob-at-1"),
		[]byte("blob-at-2"),
		[]byte("blob-at-3"),
	}
	originals := make([][]byte, len(blobs))
	for i, b := range blobs {
		originals[i] = append([]byte(nil), b...)
	}
	rows := make([]Row, len(blobs))
	for i, b := range blobs {
		rows[i] = mixedRow(Position(i+1), b)
	}
	wantPayload := int64(0)
	for i := range rows {
		wantPayload += wantMixedPayload(Position(i+1), originals[i])
	}
	if accepted, err := c.Merge(Page{Start: 1, Rows: rows}, Forward); err != nil || !accepted {
		t.Fatalf("merge = (%v, %v)", accepted, err)
	}

	// Mutate the original page's BLOB slices after admission.
	for i := range blobs {
		for j := range blobs[i] {
			blobs[i][j] = 'M'
		}
	}

	first := c.Rows()
	if len(first) != 3 {
		t.Fatalf("first Rows() = %d rows, want 3", len(first))
	}
	for i, row := range first {
		assertMixedRow(t, row, Position(i+1), originals[i])
	}
	// Mutate every BLOB in the first retrieval.
	for i := range first {
		for j := range first[i].Values[4].Bytes {
			first[i].Values[4].Bytes[j] = 'F'
		}
	}

	second := c.Rows()
	if len(second) != 3 {
		t.Fatalf("second Rows() = %d rows, want 3", len(second))
	}
	for i, row := range second {
		assertMixedRow(t, row, Position(i+1), originals[i])
	}
	// Mutate every BLOB in the second retrieval too.
	for i := range second {
		for j := range second[i].Values[4].Bytes {
			second[i].Values[4].Bytes[j] = 'S'
		}
	}

	third := c.Rows()
	for i, row := range third {
		assertMixedRow(t, row, Position(i+1), originals[i])
	}
	if got := c.PayloadBytes(); got != wantPayload {
		t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
	}
	assertRange(t, c, 1, 3)
	assertContiguousBounded(t, c)
}
