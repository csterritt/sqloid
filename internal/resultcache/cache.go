// Package resultcache is Sqloid's UI-independent positional result cache for
// one active SELECT: a single contiguous inclusive range of absolute logical
// result positions, per the Cache and snapshot invariant of
// Notes/PRD-sqloid.md and Issue #30. The cache merges fetched pages by
// absolute position — adjacent pages append or prepend, overlapping pages
// replace rows at their same positions without duplication, and nonadjacent
// stale pages are rejected atomically rather than creating a gap — and
// independently evicts from the opposite end of traversal when the retained
// range exceeds the hard limit of MaxPositions logical positions.
//
// The package is independent of Bubble Tea and database-driver concerns: it
// consumes only result.Value payloads and positional range metadata. The byte
// cap of the invariant belongs to Issue #31 and is not accounted here.
package resultcache

import (
	"strconv"

	"github.com/chris/sqloid/internal/result"
)

// MaxPositions is the independent hard limit on retained logical positions.
// The 64 MiB payload cap is accounted by Issue #31 and never by this package.
const MaxPositions = 10000

// Position is an absolute logical result row position, one-based: position 1
// is the first row of the limited logical result. Positions are independent
// of row values and of any slice index, so duplicate-valued rows remain
// distinct positions.
type Position int64

// String renders the position in decimal for tests and diagnostics.
func (p Position) String() string { return strconv.FormatInt(int64(p), 10) }

// Direction is the traversal direction of an incoming page request.
type Direction int

const (
	// Forward is traversal toward higher positions (Page Down). Accepted
	// forward merges evict from the low end of the retained range.
	Forward Direction = iota
	// Backward is traversal toward lower positions (Page Up). Accepted
	// backward merges evict from the high end of the retained range.
	Backward
)

// String renders the direction name for diagnostics.
func (d Direction) String() string {
	if d == Backward {
		return "backward"
	}
	return "forward"
}

// Row is one cached result row at an absolute logical position. Values are
// the typed result payload for that position; the cache copies them on
// acceptance so later caller mutation cannot corrupt retained rows.
type Row struct {
	Position Position
	Values   []result.Value
}

// Page is one fetched page of rows occupying consecutive absolute positions
// starting at Start. Rows must be in ascending position order with no gaps;
// a page is the fetch-side unit and carries no direction itself.
type Page struct {
	Start Position
	Rows  []Row
}

// End returns the inclusive last position covered by the page, or Start-1 for
// an empty page.
func (p Page) End() Position {
	return p.Start + Position(len(p.Rows)) - 1
}

// Cache is the single contiguous positional result cache of one active
// SELECT. The zero value is an empty cache that rejects merges; construct
// caches with New. Rows are always retained in ascending position order
// regardless of the traversal direction that fetched them.
type Cache struct {
	rows []Row
	// initialized is set only by New; a zero-value cache rejects merges so an
	// accidentally unmanaged cache never silently admits rows.
	initialized bool
	// rowCapEvictions is the cumulative count of positions evicted by the
	// MaxPositions cap, exposed as eviction metadata for snapshots and the UI.
	rowCapEvictions int
}

// New returns an empty positional result cache.
func New() *Cache { return &Cache{initialized: true} }

// Start returns the inclusive first retained position and false when the
// cache is empty.
func (c *Cache) Start() (Position, bool) {
	if len(c.rows) == 0 {
		return 0, false
	}
	return c.rows[0].Position, true
}

// End returns the inclusive last retained position and false when the cache
// is empty.
func (c *Cache) End() (Position, bool) {
	if len(c.rows) == 0 {
		return 0, false
	}
	return c.rows[len(c.rows)-1].Position, true
}

// Len returns the number of retained positions.
func (c *Cache) Len() int { return len(c.rows) }

// RowCapEvictions returns the cumulative number of positions evicted by the
// MaxPositions hard cap across the cache's lifetime.
func (c *Cache) RowCapEvictions() int { return c.rowCapEvictions }

// Rows returns the retained rows in ascending absolute-position order as a
// fresh slice; mutating it does not affect the cache.
func (c *Cache) Rows() []Row {
	out := make([]Row, len(c.rows))
	copy(out, c.rows)
	return out
}

// Merge merges page in traversal direction dir and reports whether the cache
// changed. An empty cache accepts any nonempty page. Otherwise the page is
// accepted only when its union with the retained range stays contiguous
// after accounting for overlap: positions the page shares with the retained
// range replace those rows, and remaining page positions must be adjacent to
// the retained range. A page whose remaining positions are nonadjacent — a
// stale response that would create a low-side or high-side gap — is rejected
// and neither rows nor metadata change. On acceptance the cache stays
// contiguous and ascending, and when it would exceed MaxPositions the
// opposite end of the incoming direction is evicted (low end for Forward,
// high end for Backward) until at most MaxPositions positions remain.
func (c *Cache) Merge(page Page, dir Direction) bool {
	if !c.initialized || len(page.Rows) == 0 {
		return false
	}
	pageEnd := page.End()
	if len(c.rows) > 0 {
		start, _ := c.Start()
		end, _ := c.End()
		if pageEnd < start-1 || page.Start > end+1 {
			return false
		}
	}

	// Both the retained rows and the page rows are sorted by ascending
	// position; merge them so the result stays ascending regardless of
	// traversal direction. Rows shared by both sides keep the page's payload,
	// which replaces the retained overlap.
	merged := make([]Row, 0, len(c.rows)+len(page.Rows))
	pageIdx := 0
	for _, row := range c.rows {
		for pageIdx < len(page.Rows) && page.Rows[pageIdx].Position < row.Position {
			merged = appendPageRow(merged, page.Rows[pageIdx])
			pageIdx++
		}
		if pageIdx < len(page.Rows) && page.Rows[pageIdx].Position == row.Position {
			merged = appendPageRow(merged, page.Rows[pageIdx])
			pageIdx++
			continue
		}
		merged = append(merged, row)
	}
	for ; pageIdx < len(page.Rows); pageIdx++ {
		merged = appendPageRow(merged, page.Rows[pageIdx])
	}
	c.rows = merged

	c.evict(dir)
	return true
}

// appendPageRow appends one page row, copying its payload so the cache never
// aliases caller storage.
func appendPageRow(dst []Row, row Row) []Row {
	return append(dst, Row{Position: row.Position, Values: copyValues(row.Values)})
}

// evict enforces the MaxPositions hard cap: after an accepted forward merge
// the low end is evicted first, after an accepted backward merge the high
// end is evicted first, until at most MaxPositions positions remain.
func (c *Cache) evict(dir Direction) {
	if len(c.rows) <= MaxPositions {
		return
	}
	excess := len(c.rows) - MaxPositions
	c.rowCapEvictions += excess
	if dir == Backward {
		c.rows = c.rows[:len(c.rows)-excess]
		return
	}
	c.rows = append([]Row(nil), c.rows[excess:]...)
}

// copyValues copies one row's payload so the cache never aliases caller
// storage.
func copyValues(values []result.Value) []result.Value {
	out := make([]result.Value, len(values))
	copy(out, values)
	return out
}
