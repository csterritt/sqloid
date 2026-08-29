package resultcache

import (
	"fmt"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// txt builds one TEXT result value used to label a row in tests. The label is
// the row's observable payload; two rows with the same label at different
// positions must remain distinct cache rows.
func txt(label string) result.Value { return result.NewText(label) }

// vals builds one row's values from labels.
func vals(labels ...string) []result.Value {
	out := make([]result.Value, len(labels))
	for i, l := range labels {
		out[i] = txt(l)
	}
	return out
}

// page builds a page whose rows occupy consecutive absolute positions
// starting at start, with each row carrying a single labeled TEXT value.
func page(start Position, labels ...string) Page {
	rows := make([]Row, len(labels))
	for i, l := range labels {
		rows[i] = Row{Position: start + Position(i), Values: vals(l)}
	}
	return Page{Start: start, Rows: rows}
}

// mustMerge merges page in dir and requires acceptance; it seeds cache state.
func mustMerge(t *testing.T, c *Cache, page Page, dir Direction) {
	t.Helper()
	if !c.Merge(page, dir) {
		t.Fatalf("seeding merge of page at %d..%d was rejected, want accepted",
			page.Start, page.End())
	}
}

// assertRange asserts exact retained start/end metadata and length.
func assertRange(t *testing.T, c *Cache, wantStart, wantEnd Position) {
	t.Helper()
	gotStart, ok := c.Start()
	if !ok {
		t.Fatalf("cache empty, want retained range %d..%d", wantStart, wantEnd)
	}
	gotEnd, _ := c.End()
	if gotStart != wantStart || gotEnd != wantEnd {
		t.Fatalf("retained range = %d..%d, want exactly %d..%d", gotStart, gotEnd, wantStart, wantEnd)
	}
	if got := c.Len(); got != int(wantEnd-wantStart)+1 {
		t.Fatalf("Len() = %d, want %d for range %d..%d", got, int(wantEnd-wantStart)+1, wantStart, wantEnd)
	}
}

// assertPositions asserts that the cache holds exactly one row per position
// in the ascending position list, that rows are ordered by ascending
// absolute position, and that each row carries the labeled value.
func assertPositions(t *testing.T, c *Cache, want []string) {
	t.Helper()
	rows := c.Rows()
	if len(rows) != len(want) {
		t.Fatalf("Rows() = %d rows, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		var wantPos Position
		if _, err := fmt.Sscanf(want[i], "%d", &wantPos); err != nil {
			t.Fatalf("test bug: bad expected entry %q: %v", want[i], err)
		}
		if row.Position != wantPos {
			t.Fatalf("row %d position = %d, want %d (rows must ascend by absolute position)", i, row.Position, wantPos)
		}
		wantLabel := want[i][len(fmt.Sprint(wantPos))+1:]
		if len(row.Values) != 1 || row.Values[0].Str != wantLabel {
			t.Fatalf("row at position %d = %v, want single value %q", row.Position, row.Values, wantLabel)
		}
	}
}

// assertContiguousBounded asserts the Cache and snapshot invariant for the
// positional cache: at most MaxPositions retained positions, one row per
// position, strictly ascending and gap-free positions, and exact start/end
// metadata matching the rows.
func assertContiguousBounded(t *testing.T, c *Cache) {
	t.Helper()
	rows := c.Rows()
	if len(rows) > MaxPositions {
		t.Fatalf("%d retained positions, want at most %d", len(rows), MaxPositions)
	}
	for i, row := range rows {
		if row.Position != rows[0].Position+Position(i) {
			t.Fatalf("position gap or duplicate at row %d: position %d, want %d", i, row.Position, rows[0].Position+Position(i))
		}
	}
	if len(rows) == 0 {
		if _, ok := c.Start(); ok {
			t.Fatalf("empty cache reports a start position")
		}
		return
	}
	start, _ := c.Start()
	end, _ := c.End()
	if rows[0].Position != start || rows[len(rows)-1].Position != end {
		t.Fatalf("metadata range %d..%d does not match rows %d..%d", start, end, rows[0].Position, rows[len(rows)-1].Position)
	}
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name string
		seed []struct {
			page Page
			dir  Direction
		}
		page         Page
		dir          Direction
		wantAccepted bool
		wantStart    Position
		wantEnd      Position
		want         []string
	}{
		{
			name:         "initial insertion into empty cache accepts any positions",
			page:         page(40, "v40", "v41", "v42"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    40, wantEnd: 42,
			want: []string{"40:v40", "41:v41", "42:v42"},
		},
		{
			name:         "duplicate-valued rows stay distinct positions",
			page:         page(1, "same", "other", "same"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 3,
			want: []string{"1:same", "2:other", "3:same"},
		},
		{
			name: "forward adjacent append",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "v1", "v2"), Forward}},
			page:         page(3, "v3", "v4"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 4,
			want: []string{"1:v1", "2:v2", "3:v3", "4:v4"},
		},
		{
			name: "backward adjacent prepend keeps ascending order",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(3, "v3", "v4"), Forward}},
			page:         page(1, "v1", "v2"),
			dir:          Backward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 4,
			want: []string{"1:v1", "2:v2", "3:v3", "4:v4"},
		},
		{
			name: "exact overlap replacement does not duplicate",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "old1", "old2", "old3"), Forward}},
			page:         page(1, "new1", "new2", "new3"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 3,
			want: []string{"1:new1", "2:new2", "3:new3"},
		},
		{
			name: "partial overlap replacement at high edge",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "v1", "v2", "v3"), Forward}},
			page:         page(2, "new2", "new3", "new4", "new5"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 5,
			want: []string{"1:v1", "2:new2", "3:new3", "4:new4", "5:new5"},
		},
		{
			name: "partial overlap replacement at low edge",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(3, "v3", "v4", "v5"), Forward}},
			page:         page(1, "new1", "new2", "new3"),
			dir:          Backward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 5,
			want: []string{"1:new1", "2:new2", "3:new3", "4:v4", "5:v5"},
		},
		{
			name: "page spanning the whole retained range replaces it",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(3, "v3", "v4", "v5"), Forward}},
			page:         page(1, "new1", "new2", "new3", "new4", "new5", "new6", "new7"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 7,
			want: []string{"1:new1", "2:new2", "3:new3", "4:new4", "5:new5", "6:new6", "7:new7"},
		},
		{
			name: "page spanning the low retained edge prepends and replaces",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(3, "v3", "v4"), Forward}},
			page:         page(1, "new1", "new2", "new3", "new4"),
			dir:          Backward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 4,
			want: []string{"1:new1", "2:new2", "3:new3", "4:new4"},
		},
		{
			name: "page spanning the high retained edge appends and replaces",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "v1", "v2"), Forward}},
			page:         page(2, "new2", "new3", "new4"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 4,
			want: []string{"1:v1", "2:new2", "3:new3", "4:new4"},
		},
		{
			name: "repeated overlap merge is idempotent on range",
			seed: []struct {
				page Page
				dir  Direction
			}{
				{page(1, "v1", "v2", "v3"), Forward},
				{page(2, "n2", "n3", "n4"), Forward},
			},
			page:         page(3, "m3", "m4", "m5"),
			dir:          Forward,
			wantAccepted: true,
			wantStart:    1, wantEnd: 5,
			want: []string{"1:v1", "2:n2", "3:m3", "4:m4", "5:m5"},
		},
		{
			name: "stale low-side gap page rejected",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(5, "v5", "v6", "v7"), Forward}},
			page:         page(1, "stale1", "stale2", "stale3"),
			dir:          Backward,
			wantAccepted: false,
			wantStart:    5, wantEnd: 7,
			want: []string{"5:v5", "6:v6", "7:v7"},
		},
		{
			name: "stale high-side gap page rejected",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "v1", "v2", "v3"), Forward}},
			page:         page(5, "stale5", "stale6"),
			dir:          Forward,
			wantAccepted: false,
			wantStart:    1, wantEnd: 3,
			want: []string{"1:v1", "2:v2", "3:v3"},
		},
		{
			name: "empty page rejected without mutating cache",
			seed: []struct {
				page Page
				dir  Direction
			}{{page(1, "v1", "v2"), Forward}},
			page:         page(3),
			dir:          Forward,
			wantAccepted: false,
			wantStart:    1, wantEnd: 2,
			want: []string{"1:v1", "2:v2"},
		},
		{
			name: "alternating forward and backward traversal stays ascending",
			seed: []struct {
				page Page
				dir  Direction
			}{
				{page(10, "v10", "v11"), Forward},
				{page(8, "v8", "v9"), Backward},
				{page(12, "v12", "v13"), Forward},
				{page(6, "v6", "v7"), Backward},
			},
			page:         page(4, "v4", "v5"),
			dir:          Backward,
			wantAccepted: true,
			wantStart:    4, wantEnd: 13,
			want: []string{
				"4:v4", "5:v5", "6:v6", "7:v7", "8:v8", "9:v9",
				"10:v10", "11:v11", "12:v12", "13:v13",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			for _, step := range tc.seed {
				mustMerge(t, c, step.page, step.dir)
			}
			accepted := c.Merge(tc.page, tc.dir)
			if accepted != tc.wantAccepted {
				t.Fatalf("Merge(%d..%d, %v) accepted = %v, want %v",
					tc.page.Start, tc.page.End(), tc.dir, accepted, tc.wantAccepted)
			}
			assertRange(t, c, tc.wantStart, tc.wantEnd)
			assertPositions(t, c, tc.want)
			assertContiguousBounded(t, c)
		})
	}
}

// TestMergeZeroValueCacheIsUntouched covers that a zero-value cache behaves
// like an empty cache for range metadata and rejects stale pages without
// crashing.
func TestMergeZeroValueCache(t *testing.T) {
	var c Cache
	if _, ok := c.Start(); ok {
		t.Fatalf("zero-value cache Start() ok = true, want false")
	}
	if _, ok := c.End(); ok {
		t.Fatalf("zero-value cache End() ok = true, want false")
	}
	if c.Len() != 0 {
		t.Fatalf("zero-value cache Len() = %d, want 0", c.Len())
	}
	if c.Merge(page(5, "v5"), Forward) {
		t.Fatalf("zero-value cache accepted a page, want rejection so empty caches are populated through New")
	}
}
