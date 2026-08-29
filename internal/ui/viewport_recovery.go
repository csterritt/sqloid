// Pure resize-safe vertical viewport recovery decisions (Issue #32 Task 2),
// per the SELECT lifecycle, Cache and snapshot invariant, and Module Design
// sections of Notes/PRD-sqloid.md. The seam consumes authoritative
// post-eviction metadata from internal/resultcache — the single contiguous
// retained range, dual row/byte cap-eviction disclosure, and known endpoint
// state — and requires exactly one explicit decision for the first displayed
// logical row after a page-size change: preserve the exact prior row while it
// remains valid and retained; clamp only to a known retained low/high
// endpoint at an established result boundary; or request the absolute
// containing page of the target. No Bubble Tea command or database dispatch
// lives here: the returned typed decision lets orchestration distinguish a
// local preserve/clamp from a fetch.

package ui

import (
	"github.com/chris/sqloid/internal/resultcache"
)

// RecoveryAction is the one explicit viewport-recovery decision.
type RecoveryAction int

const (
	// RecoveryPreserve keeps the exact prior first logical row: it remains
	// valid and retained in the post-eviction contiguous range.
	RecoveryPreserve RecoveryAction = iota
	// RecoveryClampLow moves the first displayed row to the known retained
	// low endpoint because the prior row sits below the established range.
	RecoveryClampLow
	// RecoveryClampHigh moves the first displayed row to the known retained
	// high endpoint because the prior row sits above an established boundary.
	RecoveryClampHigh
	// RecoveryFetch requests the absolute containing page of the required
	// row: it is not retained and cannot be safely clamped.
	RecoveryFetch
)

// String renders the action name for tests and diagnostics.
func (a RecoveryAction) String() string {
	switch a {
	case RecoveryPreserve:
		return "preserve"
	case RecoveryClampLow:
		return "clamp-low"
	case RecoveryClampHigh:
		return "clamp-high"
	default:
		return "fetch"
	}
}

// ViewportMeta is the authoritative post-eviction cache metadata a recovery
// decision consumes: the contiguous inclusive retained range, the dual-cap
// eviction disclosure, and the known endpoint state. Build it from the
// resultcache with ViewportMetaFromCache; the zero value is a valid empty
// cache whose recovery decision stays deterministic.
type ViewportMeta struct {
	// Start and End are the inclusive first and last retained logical
	// positions (one-based); meaningful only while HasRows is set.
	Start, End resultcache.Position
	// HasRows reports whether any row is retained at all.
	HasRows bool
	// HighEndpointEstablished reports an established short/empty final page:
	// the retained end is known to be the true end of the logical result.
	// A count is honored for this only when it does not exceed the retained
	// end (set by the caller), so an inconsistent count never clamps pages.
	HighEndpointEstablished bool
	// HasKnownCount and KnownCount carry the complete limited-SELECT count
	// metadata; RecoverViewport honors it only within the retained range.
	HasKnownCount bool
	KnownCount    int64
	// RowCapEvictions, PayloadBytes, and ByteTruncated are the Issue #30/#31
	// dual-cap disclosure metadata carried beside every decision so callers
	// see eviction context without it affecting the decision itself.
	RowCapEvictions int
	PayloadBytes    int64
	ByteTruncated   bool
}

// ViewportMetaFromCache derives recovery metadata from the authoritative
// contiguous resultcache plus the model's endpoint knowledge: shortPage is
// the exhausted-final-page boundary and countKnown/count report whether a
// settled complete-limited count exists and its total.
func ViewportMetaFromCache(c *resultcache.Cache, shortPage bool, countKnown bool, count int64) ViewportMeta {
	meta := ViewportMeta{
		HighEndpointEstablished: shortPage,
		HasKnownCount:           countKnown,
		KnownCount:              count,
	}
	if c != nil {
		meta.RowCapEvictions = c.RowCapEvictions()
		meta.PayloadBytes = c.PayloadBytes()
		meta.ByteTruncated = c.TruncatedByByteCap()
		if start, ok := c.Start(); ok {
			end, _ := c.End()
			meta.Start, meta.End, meta.HasRows = start, end, true
		}
	}
	return meta
}

// ViewportRecovery is one decided recovery: Action picks the branch,
// FirstRow is the absolute one-based logical position to display (preserve/
// clamp) or to request (fetch), and Size is the request limit for a fetch —
// always the exact newly computed page size, floored at one complete row.
type ViewportRecovery struct {
	Action   RecoveryAction
	FirstRow int64
	Size     int64
}

// RecoverViewport decides how the viewport recovers the prior first logical
// row after the page size changed to newSize, against the post-eviction
// retained range in meta. Decision order: preserve the exact prior row when
// it remains valid and retained; clamp to the known retained low endpoint
// when the target is below the range; clamp to the retained high endpoint
// only when that endpoint is established by a short final page or by a count
// that does not exceed the retained end (an inconsistent count is never used
// to clamp); otherwise request the new-size page containing the target — its
// absolute one-based first row is the page-grid boundary at or below the
// target, so the response both contains the required row and merges
// contiguously wherever possible. Empty or unknown metadata yields the
// deterministic safe fetch of the target's containing page.
func RecoverViewport(meta ViewportMeta, prior, newSize int64) ViewportRecovery {
	size := newSize
	if size < 1 {
		size = 1 // no complete data row fits: recover at one row, deterministically
	}
	containing := containingPageStart(prior, size)
	if !meta.HasRows {
		return ViewportRecovery{Action: RecoveryFetch, FirstRow: containing, Size: size}
	}
	if prior >= int64(meta.Start) && prior <= int64(meta.End) {
		return ViewportRecovery{Action: RecoveryPreserve, FirstRow: prior, Size: size}
	}
	if prior < int64(meta.Start) {
		return ViewportRecovery{Action: RecoveryClampLow, FirstRow: int64(meta.Start), Size: size}
	}
	if meta.HighEndpointEstablished || (meta.HasKnownCount && meta.KnownCount <= int64(meta.End)) {
		return ViewportRecovery{Action: RecoveryClampHigh, FirstRow: int64(meta.End), Size: size}
	}
	return ViewportRecovery{Action: RecoveryFetch, FirstRow: containing, Size: size}
}

// containingPageStart returns the absolute one-based first logical row of
// the new-size page-grid page that contains the target row: boundaries fall
// at 1, 1+size, 1+2·size, … in absolute logical positions.
func containingPageStart(target, size int64) int64 {
	if target < 1 {
		target = 1
	}
	return (target-1)/size*size + 1
}
