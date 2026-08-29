// Package resultcache is Sqloid's UI-independent positional result cache for
// one active SELECT: a single contiguous inclusive range of absolute logical
// result positions, per the Cache and snapshot invariant of
// Notes/PRD-sqloid.md and Issue #30. The cache merges fetched pages by
// absolute position — adjacent pages append or prepend, overlapping pages
// replace rows at their same positions without duplication, and nonadjacent
// stale pages are rejected atomically rather than creating a gap — and
// independently evicts from the opposite end of traversal when the retained
// range exceeds either hard limit: MaxPositions logical positions or
// MaxPayloadBytes (64 MiB) of retained payload (Issue #31). A fetched page
// whose own rows collectively exceed the byte envelope admits only its
// complete leading rows and fails typed at the first nonfitting position.
//
// The package is independent of Bubble Tea and database-driver concerns: it
// consumes only result.Value payloads and positional range metadata.
package resultcache

import (
	"strconv"

	"github.com/chris/sqloid/internal/result"
)

// MaxPositions is the independent hard limit on retained logical positions.
const MaxPositions = 10000

// MaxPayloadBytes is the independent hard limit on retained payload in bytes
// (Issue #31): 64 MiB counted by ValuePayload accounting — raw TEXT/BLOB
// bytes, 8 per INTEGER/REAL, 0 for NULL — never model overhead or metadata.
// It is cumulative with, and independent of, MaxPositions.
const MaxPayloadBytes int64 = 64 << 20

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
	// payloadBytes is the exact retained-payload total (Issue #31 accounting:
	// raw TEXT/BLOB bytes, 8 per INTEGER/REAL, 0 for NULL) of all retained
	// rows. It is maintained by every merge, replacement, and eviction.
	payloadBytes int64
	// byteTruncated is sticky byte-cap truncation disclosure: set once any
	// MaxPayloadBytes eviction has occurred and never cleared, so the warning
	// persists after later navigation falls below the cap.
	byteTruncated bool
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

// PayloadBytes returns the exact retained-payload total in bytes across all
// retained rows, per the Issue #31 accounting: raw TEXT and BLOB byte length,
// exactly 8 bytes for each INTEGER or REAL, and 0 for NULL. The total excludes
// model, slice, string-header, metadata, and other implementation overhead.
func (c *Cache) PayloadBytes() int64 { return c.payloadBytes }

// TruncatedByByteCap reports whether any MaxPayloadBytes byte-cap eviction has
// ever occurred on this cache. It is persistent typed `truncated-by-byte-cap`
// metadata: once set it stays set, including after later navigation brings the
// retained payload back below the cap, so disclosure survives traversal and
// result finalization.
func (c *Cache) TruncatedByByteCap() bool { return c.byteTruncated }

// Rows returns the retained rows in ascending absolute-position order as a
// fresh slice; mutating it does not affect the cache.
func (c *Cache) Rows() []Row {
	out := make([]Row, len(c.rows))
	copy(out, c.rows)
	return out
}

// Merge merges page in traversal direction dir. An empty cache accepts any
// nonempty page. Otherwise the page is accepted only when its union with the
// retained range stays contiguous after accounting for overlap: positions the
// page shares with the retained range replace those rows, and remaining page
// positions must be adjacent to the retained range. A page whose remaining
// positions are nonadjacent — a stale response that would create a low-side or
// high-side gap — is rejected with (false, nil) and neither rows nor metadata
// change. On acceptance the cache stays contiguous and ascending.
//
// The hard caps are enforced together after an accepted merge: when the page
// exceeds MaxPositions the opposite end of the incoming direction is evicted
// (low end for Forward, high end for Backward) until at most MaxPositions
// positions remain, and when the retained payload exceeds MaxPayloadBytes
// complete rows are likewise evicted from that same opposite end until the
// payload total fits. Byte-cap eviction sets the persistent
// TruncatedByByteCap disclosure.
//
// Page-envelope admission (Issue #31): a page whose own retained rows
// collectively exceed MaxPayloadBytes can never fit the envelope. Only the
// complete leading rows whose cumulative payload fits are admitted — no
// partial row, no bytes or fields from the first nonfitting row — and the
// returned error is exactly the *result.LimitFailure{Kind: KindPage} naming
// that first nonfitting absolute logical position. Earlier complete rows and
// all prior valid cache content remain retained under both caps.
func (c *Cache) Merge(page Page, dir Direction) (bool, error) {
	if !c.initialized || len(page.Rows) == 0 {
		return false, nil
	}
	// Contiguity is judged on the page exactly as fetched: admission trims
	// only trailing rows, so a page that was contiguous before admission is
	// still contiguous with its retained leading rows afterwards.
	pageEnd := page.End()
	if len(c.rows) > 0 {
		start, _ := c.Start()
		end, _ := c.End()
		if pageEnd < start-1 || page.Start > end+1 {
			return false, nil
		}
	}
	page, admitErr := admitPage(page, dir)
	var limitFailure error
	if admitErr != nil {
		limitFailure = admitErr
	}
	if len(page.Rows) == 0 {
		// Even the page's first row exceeds the envelope: nothing is admitted
		// and no bytes or fields of that row reach the cache.
		return false, limitFailure
	}

	// Both the retained rows and the page rows are sorted by ascending
	// position; merge them so the result stays ascending regardless of
	// traversal direction. Rows shared by both sides keep the page's payload,
	// which replaces the retained overlap. The retained-payload total is
	// recomputed from the merged rows so replacement is re-priced exactly.
	merged := make([]Row, 0, len(c.rows)+len(page.Rows))
	var payload int64
	pageIdx := 0
	for _, row := range c.rows {
		for pageIdx < len(page.Rows) && page.Rows[pageIdx].Position < row.Position {
			merged, payload = appendPageRow(merged, page.Rows[pageIdx], payload)
			pageIdx++
		}
		if pageIdx < len(page.Rows) && page.Rows[pageIdx].Position == row.Position {
			merged, payload = appendPageRow(merged, page.Rows[pageIdx], payload)
			pageIdx++
			continue
		}
		merged = append(merged, row)
		payload += RowPayload(row.Values)
	}
	for ; pageIdx < len(page.Rows); pageIdx++ {
		merged, payload = appendPageRow(merged, page.Rows[pageIdx], payload)
	}
	c.rows = merged
	c.payloadBytes = payload

	// Issue #31: a page-envelope trim can leave a gap between the admitted
	// leading rows and the retained range (backward pages especially). The
	// contiguity invariant is hard, so complete rows leave at the standard
	// opposite end of the incoming direction until one contiguous range
	// remains. The failing row itself never contributes anything.
	if limitFailure != nil {
		for len(c.rows) > 0 {
			span := c.rows[len(c.rows)-1].Position - c.rows[0].Position + 1
			if span == Position(len(c.rows)) {
				break
			}
			c.drop(dir, 1)
		}
	}

	c.evict(dir)
	return true, limitFailure
}

// admitPage performs the Issue #31 page-envelope admission: a page whose own
// rows collectively exceed MaxPayloadBytes can never fit, so only the
// complete rows nearest the retained range — leading rows for Forward,
// trailing rows for Backward, i.e. the page's rows in traversal order — are
// admitted while their cumulative payload fits, and the returned typed
// failure names the one-based absolute logical position of the first
// nonfitting row in traversal order. No partial row and no bytes or fields
// of a nonfitting row ever reach the cache; earlier complete rows and prior
// valid cache content remain retained under both caps, and the retained
// range stays contiguous.
func admitPage(page Page, dir Direction) (Page, error) {
	var failure error
	if dir == Backward {
		kept := len(page.Rows)
		var total int64
		for i := len(page.Rows) - 1; i >= 0; i-- {
			total += RowPayload(page.Rows[i].Values)
			if total > MaxPayloadBytes {
				kept = i + 1
				var f error = &result.LimitFailure{Kind: result.KindPage, Position: int64(page.Start) + int64(i)}
				failure = f
				break
			}
		}
		if failure == nil {
			return page, nil
		}
		rows := append([]Row(nil), page.Rows[kept:]...)
		return Page{Start: page.Start + Position(kept), Rows: rows}, failure
	}
	var total int64
	for i, row := range page.Rows {
		total += RowPayload(row.Values)
		if total > MaxPayloadBytes {
			rows := append([]Row(nil), page.Rows[:i]...)
			var f error = &result.LimitFailure{Kind: result.KindPage, Position: int64(page.Start) + int64(i)}
			return Page{Start: page.Start, Rows: rows}, f
		}
	}
	return page, nil
}

// appendPageRow appends one page row, copying its payload so the cache never
// aliases caller storage, and returns the updated payload total.
func appendPageRow(dst []Row, row Row, payload int64) ([]Row, int64) {
	copied := Row{Position: row.Position, Values: copyValues(row.Values)}
	return append(dst, copied), payload + RowPayload(copied.Values)
}

// evict enforces both hard caps: after an accepted forward merge the low end
// is evicted first, after an accepted backward merge the high end is evicted
// first, until at most MaxPositions positions remain AND the retained payload
// fits MaxPayloadBytes. Any byte eviction sets the persistent byte-cap
// disclosure.
func (c *Cache) evict(dir Direction) {
	if excess := len(c.rows) - MaxPositions; excess > 0 {
		c.rowCapEvictions += excess
		c.drop(dir, excess)
	}
	for c.payloadBytes > MaxPayloadBytes && len(c.rows) > 0 {
		c.byteTruncated = true
		c.drop(dir, 1)
	}
}

// drop removes exactly n complete rows from the opposite end of the incoming
// traversal direction — the low end for Forward, the high end for Backward —
// subtracting their payload from the retained total.
func (c *Cache) drop(dir Direction, n int) {
	if n <= 0 || n > len(c.rows) {
		return
	}
	if dir == Backward {
		for i := len(c.rows) - n; i < len(c.rows); i++ {
			c.payloadBytes -= RowPayload(c.rows[i].Values)
		}
		c.rows = c.rows[:len(c.rows)-n]
		return
	}
	for i := 0; i < n; i++ {
		c.payloadBytes -= RowPayload(c.rows[i].Values)
	}
	c.rows = append([]Row(nil), c.rows[n:]...)
}

// copyValues copies one row's payload so the cache never aliases caller
// storage.
func copyValues(values []result.Value) []result.Value {
	out := make([]result.Value, len(values))
	copy(out, values)
	return out
}
