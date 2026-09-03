# Issue #081 Code Walkthrough: Deep-Copy Cached BLOB Values

*2026-09-03T17:28:58Z by Showboat 0.6.1*
<!-- showboat-id: 43ef717c-abcf-4464-8dfa-81334e91313a -->

Issue #81 (Notes/tasks/081-deep-copy-cached-blob-values.md, Notes/PRD-sqloid.md Cache and snapshot invariant) closes the two ownership boundaries of the result cache that Issues #30 and #31 left as shallow copy slices: the cache must own admitted BLOB bytes and must return independently mutable row snapshots from every Cache.Rows() call. NULL, INTEGER, REAL, and TEXT values, Row.Position, the values slice shape, every value kind, ascending ordering, contiguity, PayloadBytes(), the retained range, and the row/byte-cap eviction metadata all remain exact. This walkthrough demonstrates a mixed typed row entering the result cache, mutates the original BLOB source, retrieves and mutates one returned BLOB, then retrieves again and shows the cache still returns the original bytes with unchanged position, kinds, values, ordering, and payload accounting. It also covers an overlap-replacement case and the passing focused result-cache verification so both ownership boundaries and existing cap/eviction behavior are visible. Reference: Issue #81 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/081-04/code-walkthrough/.

## The shared copyValues helper

```bash
sed -n '/\/\/ copyValues copies one row/,/^}/p' internal/resultcache/cache.go
```

```output
// copyValues copies one row's payload so the cache never aliases caller
// storage. Only BLOB values carry mutable backing storage (result.Value.Bytes
// for KindBlob), so those bytes are deep-copied via the established
// result.NewBlob idiom; NULL, INTEGER, REAL, and TEXT keep their by-value
// fields unchanged. The returned slice has the same length and value order
// as the input.
func copyValues(values []result.Value) []result.Value {
	out := make([]result.Value, len(values))
	for i, v := range values {
		if v.Kind == result.KindBlob {
			out[i] = result.NewBlob(v.Bytes)
			continue
		}
		out[i] = v
	}
	return out
}
```

copyValues is the single shared helper used by both ownership boundaries. It deep-copies result.Value.Bytes for KindBlob via result.NewBlob (which appends into a fresh nil slice), and carries NULL, INTEGER, REAL, and TEXT through unchanged because their meaningful fields are by-value. The slice length and value order are preserved.

## The admission boundary: appendPageRow

```bash
sed -n '/\/\/ appendPageRow appends one page row/,/^}/p' internal/resultcache/cache.go
```

```output
// appendPageRow appends one page row, copying its payload so the cache never
// aliases caller storage, and returns the updated payload total.
func appendPageRow(dst []Row, row Row, payload int64) ([]Row, int64) {
	copied := Row{Position: row.Position, Values: copyValues(row.Values)}
	return append(dst, copied), payload + RowPayload(copied.Values)
}
```

appendPageRow is called from every Merge path that transfers a page row into cache-owned storage: initial insertion, forward append, backward prepend, and every overlap-replacement case (the two-way sorted merge in Cache.Merge calls appendPageRow for each page row, whether it lands on a new position or replaces an overlapping retained row). Row.Position is preserved exactly and the payload total is recomputed from the copied values so accounting stays exact.

## The retrieval boundary: Cache.Rows()

```bash
sed -n '/\/\/ Rows returns the retained rows/,/^}/p' internal/resultcache/cache.go
```

```output
// Rows returns the retained rows in ascending absolute-position order as a
// fresh, caller-owned slice: mutating it, including any BLOB byte slice it
// carries, does not affect the cache or any later Rows() result. Each
// retained BLOB payload is deep-copied into the returned row so the cache's
// own owned bytes are never aliased by a retrieval.
func (c *Cache) Rows() []Row {
	out := make([]Row, len(c.rows))
	for i, row := range c.rows {
		out[i] = Row{Position: row.Position, Values: copyValues(row.Values)}
	}
	return out
}
```

Rows() previously did a shallow copy of the retained rows slice, so a caller mutating a returned BLOB byte slice would corrupt the cache and every later retrieval. It now constructs each returned Row through copyValues, so each retrieval receives its own independent BLOB byte slice. Row.Position and the values slice shape are preserved.

## Demonstration: mixed typed row, mutate source, retrieve and mutate, retrieve again

The demonstration is a focused Go test (internal/resultcache/blob_isolation_demo_test.go) that builds a mixed-kind row (NULL, INTEGER, REAL, TEXT, BLOB) with a BLOB whose backing slice is shared with the caller via sharedBlobDemo (bypassing result.NewBlob's defensive copy), admits it through Cache.Merge, mutates the original BLOB source, retrieves via Rows() and mutates the returned BLOB, then retrieves again and asserts the cache still returns the original bytes with unchanged position, kinds, values, ordering, and payload accounting.

```bash
sed -n '1,40p' internal/resultcache/blob_isolation_demo_test.go
```

```output
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
```

```bash
go test ./internal/resultcache/ -run 'TestDemoBlobIsolationWalkthrough' -count=1 -v 2>&1 | sed -E 's/\([0-9.]+s\)/(0.00s)/; s/[[:space:]]+[0-9.]+s$//'
```

```output
=== RUN   TestDemoBlobIsolationWalkthrough
    blob_isolation_demo_test.go:70: after mutating original source: cache BLOB = [0 1 2 254 255] (original [0 1 2 254 255])
    blob_isolation_demo_test.go:77: mutated returned BLOB to [238 238 238 238 238]
    blob_isolation_demo_test.go:92: after mutating first retrieval: cache BLOB = [0 1 2 254 255] (original [0 1 2 254 255])
    blob_isolation_demo_test.go:98: PayloadBytes() = 30 (unchanged)
    blob_isolation_demo_test.go:104: retained range = 7..7, ascending, contiguous
    blob_isolation_demo_test.go:111: cap metadata: row-cap evictions=0, byte-cap disclosure=false
--- PASS: TestDemoBlobIsolationWalkthrough (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/resultcache
```

The test passes. After mutating the original BLOB source, the cache still holds the original bytes [0 1 2 254 255]. After mutating the returned BLOB to [238 238 238 238 238], a second retrieval still returns the original bytes [0 1 2 254 255]. PayloadBytes() stays 30 (8 INTEGER + 8 REAL + 9 TEXT 'demo-text' + 5 BLOB), the retained range stays 7..7 (ascending, contiguous), and the row/byte-cap metadata is unchanged (0 evictions, no byte-cap disclosure).

## Overlap-replacement case

The overlap-replacement demonstration proves the admission boundary deep-copies BLOB bytes when a page row replaces a retained row at the same position. A seed page of TEXT rows is admitted, then a partial high-edge overlap page carrying a new BLOB replaces position 2. Mutating the replacement page's BLOB after Merge leaves the cache holding the originally admitted replacement bytes, with the replaced position, untouched low row, ordering, kinds, and payload accounting all exact. The TestBlobIsolationOnOverlapReplacement test covers partial high-edge, partial low-edge, and exact full-range replacement.

```bash
go test ./internal/resultcache/ -run 'TestBlobIsolationOnOverlapReplacement' -count=1 -v 2>&1 | sed -E 's/\([0-9.]+s\)/(0.00s)/; s/[[:space:]]+[0-9.]+s$//'
```

```output
=== RUN   TestBlobIsolationOnOverlapReplacement
=== RUN   TestBlobIsolationOnOverlapReplacement/partial_overlap_at_the_high_edge
=== RUN   TestBlobIsolationOnOverlapReplacement/partial_overlap_at_the_low_edge
=== RUN   TestBlobIsolationOnOverlapReplacement/exact_full-range_replacement
--- PASS: TestBlobIsolationOnOverlapReplacement (0.00s)
    --- PASS: TestBlobIsolationOnOverlapReplacement/partial_overlap_at_the_high_edge (0.00s)
    --- PASS: TestBlobIsolationOnOverlapReplacement/partial_overlap_at_the_low_edge (0.00s)
    --- PASS: TestBlobIsolationOnOverlapReplacement/exact_full-range_replacement (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/resultcache
```

All three overlap-replacement subtests pass: partial overlap at the high edge, partial overlap at the low edge, and exact full-range replacement. Each mutates the replacement page's BLOB after Merge and asserts the cache holds the originally admitted replacement bytes with the replaced position, untouched rows, ordering, kinds, and payload accounting exact.

## Focused result-cache verification

```bash
go test ./internal/resultcache/ -count=1 2>&1 | sed -E 's/[[:space:]]+[0-9.]+s$//'
```

```output
ok  	github.com/chris/sqloid/internal/resultcache
```

The full internal/resultcache test suite passes: the new Issue #81 BLOB isolation tests (TestBlobIsolationOnAdmission, TestBlobIsolationOnRowsRetrieval, TestBlobIsolationOnForwardAppend, TestBlobIsolationOnBackwardPrepend, TestBlobIsolationOnOverlapReplacement, TestBlobIsolationSurvivesRowsMutationAcrossRetrievals, and the demo TestDemoBlobIsolationWalkthrough) alongside the existing cap, eviction, admission, payload accounting, and snapshot boundary tests. Both ownership boundaries and the existing cap/eviction behavior are visible and green.

## Repository-wide verification

```bash
gofmt -l internal/resultcache/cache.go internal/resultcache/blob_isolation_test.go internal/resultcache/blob_isolation_demo_test.go && echo 'gofmt: clean'
```

```output
gofmt: clean
```

```bash
go vet ./... 2>&1 && echo 'go vet: clean'
```

```output
go vet: clean
```

```bash
go build ./... 2>&1 && echo 'go build: clean'
```

```output
go build: clean
```

```bash
go test ./... 2>&1 | sed -E 's/[[:space:]]+[0-9.]+s$//'
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/070-06/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

Repository-wide verification is clean: gofmt, go vet ./..., go build ./..., and go test ./... all pass. The Issue #81 deep-copy ownership changes are confined to internal/resultcache/cache.go (copyValues, appendPageRow, and Rows()) and the new test files; every other package is unaffected.
