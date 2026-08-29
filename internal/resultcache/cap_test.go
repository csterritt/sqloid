package resultcache

import "testing"

// capPage builds a page of the given size occupying consecutive absolute
// positions starting at start. Each row's payload encodes its position as
// "<prefix><N>", so retained overlap values are verifiable after replacement.
func capPage(prefix string, start Position, size int) Page {
	rows := make([]Row, size)
	for i := range rows {
		pos := start + Position(i)
		rows[i] = Row{Position: pos, Values: vals(prefix + pos.String())}
	}
	return Page{Start: start, Rows: rows}
}

// seedStep is one seeding merge before the merge under test.
type seedStep struct {
	start Position
	size  int
	dir   Direction
}

// seedCache applies seed merges; every step must be accepted.
func seedCache(t *testing.T, steps []seedStep) *Cache {
	t.Helper()
	c := New()
	for _, s := range steps {
		mustMerge(t, c, capPage("p", s.start, s.size), s.dir)
	}
	return c
}

// mustMergeChecked merges page in dir and requires acceptance with no typed
// failure, for byte-cap fixtures.
func mustMergeChecked(t *testing.T, c *Cache, page Page, dir Direction) {
	t.Helper()
	accepted, err := c.Merge(page, dir)
	if err != nil {
		t.Fatalf("merge of page at %d..%d failed: %v", page.Start, page.End(), err)
	}
	if !accepted {
		t.Fatalf("merge of page at %d..%d rejected, want accepted", page.Start, page.End())
	}
}

// TestPositionCapEviction specifies the independent MaxPositions hard cap:
// after every accepted merge the retained interval is contiguous with at most
// MaxPositions positions, eviction is deterministic at the standard opposite
// end of the incoming traversal direction (low end for Forward, high end for
// Backward), start/end metadata is exact, no position is duplicated, and
// values at retained overlaps are unaffected. Rejection of stale gap pages
// after eviction stays atomic.
func TestPositionCapEviction(t *testing.T) {
	tests := []struct {
		name        string
		seed        []seedStep
		page        Page
		dir         Direction
		wantStart   Position
		wantEnd     Position
		wantEvicted int
		// replaced lists positions that must carry the page's replaced value
		// ("r<N>"); all other retained positions keep their original "p<N>".
		replaced []Position
	}{
		{
			name:      "forward merge landing exactly at the cap evicts nothing",
			seed:      []seedStep{{1, MaxPositions - 1, Forward}},
			page:      capPage("p", MaxPositions-1, 2),
			dir:       Forward,
			wantStart: 1, wantEnd: Position(MaxPositions),
			wantEvicted: 0,
		},
		{
			name:      "forward append one past the cap evicts one low position",
			seed:      []seedStep{{1, MaxPositions - 1, Forward}},
			page:      capPage("p", MaxPositions-1, 3),
			dir:       Forward,
			wantStart: 2, wantEnd: Position(MaxPositions + 1),
			wantEvicted: 1,
		},
		{
			name:      "forward append crossing the cap by several page sizes evicts exactly the excess",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("p", MaxPositions+1, 2500),
			dir:       Forward,
			wantStart: 2501, wantEnd: Position(MaxPositions + 2500),
			wantEvicted: 2500,
		},
		{
			name:      "single page larger than the cap retains its last MaxPositions positions forward",
			page:      capPage("p", 1, MaxPositions+5000),
			dir:       Forward,
			wantStart: 5001, wantEnd: Position(MaxPositions + 5000),
			wantEvicted: 5000,
		},
		{
			name:      "single page larger than the cap retains its first MaxPositions positions backward",
			page:      capPage("p", 1, MaxPositions+5000),
			dir:       Backward,
			wantStart: 1, wantEnd: Position(MaxPositions),
			wantEvicted: 5000,
		},
		{
			name:      "backward append one past the cap evicts one high position",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("p", -2, 3),
			dir:       Backward,
			wantStart: -2, wantEnd: Position(MaxPositions - 3),
			wantEvicted: 3,
		},
		{
			name:      "backward page crossing the cap by several page sizes evicts exactly the excess",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("p", -2499, 2500),
			dir:       Backward,
			wantStart: -2499, wantEnd: Position(MaxPositions - 2500),
			wantEvicted: 2500,
		},
		{
			name:      "forward overlap near the low edge replaces then evicts the low end",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("r", 500, 11501),
			dir:       Forward,
			wantStart: 2001, wantEnd: Position(MaxPositions + 2000),
			wantEvicted: 2000,
			replaced:    []Position{2500, Position(MaxPositions) + 1000},
		},
		{
			name:      "backward overlap near the high edge replaces then evicts the high end",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("r", MaxPositions-2500, 3000),
			dir:       Backward,
			wantStart: 1, wantEnd: Position(MaxPositions),
			wantEvicted: 499,
			replaced:    []Position{MaxPositions - 2500, MaxPositions},
		},
		{
			name:      "page spanning the retained low edge under cap pressure replaces the overlap",
			seed:      []seedStep{{1, MaxPositions, Forward}},
			page:      capPage("r", MaxPositions-1000, 2000),
			dir:       Forward,
			wantStart: 1000, wantEnd: Position(MaxPositions + 999),
			wantEvicted: 999,
			replaced:    []Position{MaxPositions - 1000, MaxPositions - 500, Position(MaxPositions) + 999},
		},
		{
			name: "alternating direction after prior eviction evicts the other end",
			seed: []seedStep{
				{1, MaxPositions, Forward},
				{MaxPositions + 1, 1000, Forward},
			},
			page:      capPage("p", 1, 1000),
			dir:       Backward,
			wantStart: 1, wantEnd: Position(MaxPositions),
			wantEvicted: 2000,
		},
		{
			name: "alternating direction again evicts the low end once more",
			seed: []seedStep{
				{1, MaxPositions, Forward},
				{MaxPositions + 1, 1000, Forward},
				{1, 1000, Backward},
			},
			page:      capPage("p", Position(MaxPositions)+1, 900),
			dir:       Forward,
			wantStart: 901, wantEnd: Position(MaxPositions + 900),
			wantEvicted: 2900,
		},
		{
			name: "stale gap page after eviction rejected atomically",
			seed: []seedStep{
				{1, MaxPositions, Forward},
				{MaxPositions + 1, 1000, Forward},
			},
			page:      capPage("p", MaxPositions+2500, 10),
			dir:       Forward,
			wantStart: 1001, wantEnd: Position(MaxPositions + 1000),
			wantEvicted: 1000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := seedCache(t, tc.seed)
			before := c.Rows()
			accepted, err := c.Merge(tc.page, tc.dir)
			if tc.name == "stale gap page after eviction rejected atomically" {
				if accepted || err != nil {
					t.Fatalf("Merge of nonadjacent stale page accepted (%v, %v), want rejection", accepted, err)
				}
				after := c.Rows()
				if len(before) != len(after) {
					t.Fatalf("rejected merge changed cache size: %d -> %d rows", len(before), len(after))
				}
				for i := range before {
					if before[i].Position != after[i].Position || before[i].Values[0].Str != after[i].Values[0].Str {
						t.Fatalf("rejected merge mutated row %d: %d/%q -> %d/%q",
							i, before[i].Position, before[i].Values[0].Str, after[i].Position, after[i].Values[0].Str)
					}
				}
				assertRange(t, c, tc.wantStart, tc.wantEnd)
				if got := c.RowCapEvictions(); got != tc.wantEvicted {
					t.Fatalf("RowCapEvictions() = %d, want unchanged %d", got, tc.wantEvicted)
				}
				assertContiguousBounded(t, c)
				return
			}
			if err != nil {
				t.Fatalf("Merge(%d..%d, %v) returned error %v, want none", tc.page.Start, tc.page.End(), tc.dir, err)
			}
			if !accepted {
				t.Fatalf("Merge(%d..%d, %v) rejected, want accepted", tc.page.Start, tc.page.End(), tc.dir)
			}
			assertRange(t, c, tc.wantStart, tc.wantEnd)
			assertContiguousBounded(t, c)
			if got := c.RowCapEvictions(); got != tc.wantEvicted {
				t.Fatalf("RowCapEvictions() = %d, want %d", got, tc.wantEvicted)
			}
			rows := c.Rows()
			byPos := make(map[Position]string, len(rows))
			for _, row := range rows {
				if _, dup := byPos[row.Position]; dup {
					t.Fatalf("duplicate position %d retained", row.Position)
				}
				byPos[row.Position] = row.Values[0].Str
			}
			for _, pos := range tc.replaced {
				want := "r" + pos.String()
				if got, ok := byPos[pos]; !ok || got != want {
					t.Fatalf("overlap position %d = (%q, %v), want replaced value %q", pos, got, ok, want)
				}
			}
			// Sample retained originals at both retained ends to prove
			// eviction removed the opposite end's values, not the retained
			// ones.
			if len(tc.replaced) > 0 || tc.name == "stale gap page after eviction rejected atomically" {
				return
			}
			if got := byPos[tc.wantStart]; got != "p"+tc.wantStart.String() {
				t.Fatalf("retained low end position %d = %q, want %q", tc.wantStart, got, "p"+tc.wantStart.String())
			}
			if got := byPos[tc.wantEnd]; got != "p"+tc.wantEnd.String() {
				t.Fatalf("retained high end position %d = %q, want %q", tc.wantEnd, got, "p"+tc.wantEnd.String())
			}
		})
	}
}
