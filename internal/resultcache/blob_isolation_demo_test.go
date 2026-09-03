package resultcache

import (
	"bytes"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// sharedBlobDemo builds a KindBlob value whose Bytes slice is exactly the
// caller's slice (no copy), so the cache's own ownership boundary is the one
// under test, not result.NewBlob's defensive copy.
func sharedBlobDemo(b []byte) result.Value {
	return result.Value{Kind: result.KindBlob, Bytes: b}
}

func TestDemoBlobIsolationWalkthrough(t *testing.T) {
	c := New()
	// Build a mixed-kind row at position 7 with a BLOB whose backing slice is
	// shared with the caller.
	original := []byte{0x00, 0x01, 0x02, 0xFE, 0xFF}
	blob := append([]byte(nil), original...) // caller's mutable slice
	row := Row{Position: 7, Values: []result.Value{
		result.NewNull(),
		result.NewInteger(42),
		result.NewReal(3.5),
		result.NewText("demo-text"),
		sharedBlobDemo(blob),
	}}
	wantPayload := int64(0 + 8 + 8 + len("demo-text") + len(original))

	// Admit the row into cache-owned storage.
	if accepted, err := c.Merge(Page{Start: 7, Rows: []Row{row}}, Forward); err != nil || !accepted {
		t.Fatalf("Merge = (%v, %v), want accepted with no error", accepted, err)
	}

	// Mutate the original BLOB source after admission. The cache must be
	// unaffected: it owns a deep copy of the admitted bytes.
	for i := range blob {
		blob[i] = 0xAA
	}

	// First retrieval: assert the cache still holds the original bytes.
	first := c.Rows()
	if len(first) != 1 {
		t.Fatalf("first Rows() = %d rows, want 1", len(first))
	}
	got := first[0]
	if got.Position != 7 {
		t.Fatalf("position = %d, want 7", got.Position)
	}
	if len(got.Values) != 5 {
		t.Fatalf("values count = %d, want 5", len(got.Values))
	}
	if got.Values[0].Kind != result.KindNull {
		t.Fatalf("value 0 kind = %v, want KindNull", got.Values[0].Kind)
	}
	if got.Values[1].Kind != result.KindInteger || got.Values[1].Int != 42 {
		t.Fatalf("value 1 = %+v, want KindInteger 42", got.Values[1])
	}
	if got.Values[2].Kind != result.KindReal || got.Values[2].Float != 3.5 {
		t.Fatalf("value 2 = %+v, want KindReal 3.5", got.Values[2])
	}
	if got.Values[3].Kind != result.KindText || got.Values[3].Str != "demo-text" {
		t.Fatalf("value 3 = %+v, want KindText demo-text", got.Values[3])
	}
	if got.Values[4].Kind != result.KindBlob || !bytes.Equal(got.Values[4].Bytes, original) {
		t.Fatalf("value 4 = %+v, want KindBlob original bytes %v", got.Values[4], original)
	}
	t.Logf("after mutating original source: cache BLOB = %v (original %v)", got.Values[4].Bytes, original)

	// Mutate the BLOB returned by the first Rows() call. The cache and every
	// later retrieval must keep the original bytes.
	for i := range first[0].Values[4].Bytes {
		first[0].Values[4].Bytes[i] = 0xEE
	}
	t.Logf("mutated returned BLOB to %v", first[0].Values[4].Bytes)

	// Second retrieval: assert the cache still holds the original bytes,
	// independent of the mutated first retrieval.
	second := c.Rows()
	if len(second) != 1 {
		t.Fatalf("second Rows() = %d rows, want 1", len(second))
	}
	got2 := second[0]
	if got2.Position != 7 {
		t.Fatalf("second position = %d, want 7", got2.Position)
	}
	if got2.Values[4].Kind != result.KindBlob || !bytes.Equal(got2.Values[4].Bytes, original) {
		t.Fatalf("second value 4 = %+v, want KindBlob original bytes %v", got2.Values[4], original)
	}
	t.Logf("after mutating first retrieval: cache BLOB = %v (original %v)", got2.Values[4].Bytes, original)

	// Payload accounting and retained range are unchanged.
	if got := c.PayloadBytes(); got != int64(wantPayload) {
		t.Fatalf("PayloadBytes() = %d, want %d", got, wantPayload)
	}
	t.Logf("PayloadBytes() = %d (unchanged)", c.PayloadBytes())
	start, _ := c.Start()
	end, _ := c.End()
	if start != 7 || end != 7 {
		t.Fatalf("retained range = %d..%d, want 7..7", start, end)
	}
	t.Logf("retained range = %d..%d, ascending, contiguous", start, end)
	if c.TruncatedByByteCap() {
		t.Fatal("TruncatedByByteCap set without any byte eviction")
	}
	if got := c.RowCapEvictions(); got != 0 {
		t.Fatalf("RowCapEvictions() = %d, want 0", got)
	}
	t.Logf("cap metadata: row-cap evictions=0, byte-cap disclosure=false")
}
