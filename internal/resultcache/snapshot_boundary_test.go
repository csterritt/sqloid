// Focused cache-to-snapshot boundary tests for Issue #33: the authoritative
// resultcache facts (retained inclusive range, cumulative row-cap eviction,
// persistent `truncated-by-byte-cap` disclosure) that the internal/history
// snapshot metadata model consumes via FactsFromCache. Rows stay in ascending
// absolute-position order regardless of the traversal direction that fetched
// them, and finalized metadata facts never change with later cache activity.

package resultcache_test

import (
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	rc "github.com/chris/sqloid/internal/resultcache"
)

func rowAt(p rc.Position) rc.Row {
	return rc.Row{Position: p, Values: []result.Value{result.NewInteger(int64(p))}}
}

func page(start rc.Position, n int) rc.Page {
	p := rc.Page{Start: start}
	for i := 0; i < n; i++ {
		p.Rows = append(p.Rows, rowAt(start+rc.Position(i)))
	}
	return p
}

// TestSnapshotFactsTrackCacheLifecycle walks a full paging lifecycle and
// asserts the snapshot facts at each stage: empty cache, forward traversal,
// row-cap eviction, and the persistent byte-cap disclosure.
func TestSnapshotFactsTrackCacheLifecycle(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, c *rc.Cache)
		want history.CacheFacts
	}{
		{
			name: "empty cache has no retained range and no eviction",
			seed: func(t *testing.T, c *rc.Cache) {},
			want: history.CacheFacts{},
		},
		{
			name: "forward merge retains ascending range",
			seed: func(t *testing.T, c *rc.Cache) {
				if ok, err := c.Merge(page(1, 4), rc.Forward); !ok || err != nil {
					t.Fatalf("merge = (%v, %v)", ok, err)
				}
			},
			want: history.CacheFacts{HasRetainedRange: true, Start: 1, End: 4},
		},
		{
			name: "row-cap eviction appears cumulatively",
			seed: func(t *testing.T, c *rc.Cache) {
				for i := 0; i < rc.MaxPositions/1000+2; i++ {
					if ok, err := c.Merge(page(rc.Position(1000*i+1), 1000), rc.Forward); !ok || err != nil {
						t.Fatalf("merge %d = (%v, %v)", i, ok, err)
					}
				}
			},
			want: history.CacheFacts{
				HasRetainedRange: true,
				// The last 10,000 positions remain: 2001..12000, with 2,000
				// positions evicted cumulatively across the 12 merges.
				Start:           rc.Position(2001),
				End:             rc.Position(12000),
				RowCapEvictions: 2000,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := rc.New()
			tc.seed(t, c)
			got := history.FactsFromCache(c)
			if got != tc.want {
				t.Errorf("facts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestSnapshotFactsAscendingAfterBackwardTraversal proves snapshot metadata
// sees ascending absolute positions regardless of traversal direction:
// backward merges evict from the high end and the retained range never
// inverts.
func TestSnapshotFactsAscendingAfterBackwardTraversal(t *testing.T) {
	c := rc.New()
	if ok, err := c.Merge(page(21, 10), rc.Forward); !ok || err != nil {
		t.Fatalf("initial merge = (%v, %v)", ok, err)
	}
	if ok, err := c.Merge(page(11, 10), rc.Backward); !ok || err != nil {
		t.Fatalf("backward merge = (%v, %v)", ok, err)
	}
	if ok, err := c.Merge(page(1, 10), rc.Backward); !ok || err != nil {
		t.Fatalf("second backward merge = (%v, %v)", ok, err)
	}
	facts := history.FactsFromCache(c)
	if facts.Start != 1 || facts.End != 30 {
		t.Errorf("retained range = [%d,%d], want [1,30] in ascending order", facts.Start, facts.End)
	}
	rows := c.Rows()
	for i, r := range rows {
		if r.Position != rc.Position(i+1) {
			t.Fatalf("row %d has position %d: retained rows must ascend", i, r.Position)
		}
	}
}

// TestSnapshotFactsByteCapDisclosurePersists proves the typed
// `truncated-by-byte-cap` fact survives later traversal that brings retained
// payload back below the cap, so finalized snapshot metadata keeps the
// disclosure without re-deriving it from current bytes.
func TestSnapshotFactsByteCapDisclosurePersists(t *testing.T) {
	c := rc.New()
	// Two 40 MiB blob pages: the second merge pushes retained payload past
	// the 64 MiB cap, evicting complete low-end rows and setting the
	// persistent disclosure.
	big := rc.Page{Start: 1, Rows: []rc.Row{
		{Position: 1, Values: []result.Value{result.NewBlob(make([]byte, 40<<20))}},
	}}
	if ok, err := c.Merge(big, rc.Forward); !ok || err != nil {
		t.Fatalf("first blob merge = (%v, %v)", ok, err)
	}
	big2 := rc.Page{Start: 2, Rows: []rc.Row{
		{Position: 2, Values: []result.Value{result.NewBlob(make([]byte, 40<<20))}},
	}}
	if ok, err := c.Merge(big2, rc.Forward); !ok || err != nil {
		t.Fatalf("second blob merge = (%v, %v)", ok, err)
	}
	if !c.TruncatedByByteCap() {
		t.Fatalf("byte-cap disclosure not set after byte eviction")
	}
	before := history.FactsFromCache(c)
	if !before.TruncatedByByteCap {
		t.Fatalf("snapshot facts lost the byte-cap disclosure: %+v", before)
	}
	// Later navigation well below the cap must not clear the disclosure.
	if ok, err := c.Merge(page(2, 3), rc.Forward); !ok || err != nil {
		t.Fatalf("later small merge = (%v, %v)", ok, err)
	}
	after := history.FactsFromCache(c)
	if c.PayloadBytes() > rc.MaxPayloadBytes {
		t.Fatalf("payload %d still above cap; test setup no longer exercises persistence", c.PayloadBytes())
	}
	if !after.TruncatedByByteCap {
		t.Errorf("persistent disclosure cleared after payload fell below cap: %+v", after)
	}
}
