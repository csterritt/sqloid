// Serialized vertical result paging inside the UI (Issue #25), per the
// Paging consistency decision in Notes/PRD-sqloid.md. Page Down and Page Up
// on an idle active SELECT request exactly one adjacent absolute logical
// range through QueryBuilder's page API, with the page size derived from the
// existing results-area layout arithmetic as the exact count of complete
// data rows. At most one page request is ever pending — tracked
// independently from Issue #24's count request, which may still settle while
// a page is in flight — and repeated or opposite page keys while a page is
// pending are consumed without stacking commands. The user's Limit is never
// read beyond: PageSQL yields no statement past it. Local horizontal
// movement, count settlement, and first-page behavior are untouched, and
// cancellation, stale generation handling, and cache-cap policy stay with
// their owning issues.

package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// PageExecutor performs one cancellable paged-page SELECT execution for the
// given safely rendered page statement (QueryBuilder's PageSQL, whose exact
// LIMIT/OFFSET range replaces the user's Limit clause) and ordered bound
// parameters, mapping the Connection boundary's outcomes onto the returned
// FirstPageResult. It always runs inside a tea.Cmd — never in Update or View.
type PageExecutor func(ctx context.Context, sql string, params []any) FirstPageResult

// PageSettledMsg carries one settled paged-page execution back through
// Update with the full identity (Issues #25 and #26) that guards it: the
// SELECT execution ID it ran under, its page request ID, and the viewport
// generation current at dispatch. Produced only by commands this package
// created; it mutates state only while every applicable identity is still
// current.
type PageSettledMsg struct {
	ExecutionID uint64
	RequestID   uint64
	Generation  uint64
	Result      FirstPageResult
}

// PageLoadingIndicator is the exact page-loading feedback rendered in the
// status/count line while the one page request is pending.
const PageLoadingIndicator = "loading next page…"

// handlePageKey routes a Page Up or Page Down press. At most one page
// request may be pending: a repeated or opposite key while one is pending is
// consumed without issuing any command, keeping the loading feedback visible.
// At the known boundaries — the low boundary at offset zero, the high
// boundary after a page shorter than the requested size, and the user's
// Limit — the key is likewise consumed without a request.
func (m *Model) handlePageKey(up bool) tea.Cmd {
	if m.Page == nil || m.pagePending || m.validating ||
		m.Result == nil || m.Result.Page == nil {
		return nil
	}
	size, offset, ok := m.pageRange(up)
	if !ok {
		return nil
	}
	statement := m.QB.PageSQL(size, offset)
	if statement == "" {
		// Unrenderable snapshot or a range at/beyond the user's Limit: the
		// key is consumed without reading anything.
		return nil
	}
	requestID := result.NextSelectRequestID()
	m.pagePending = true
	m.pageRequestID = requestID
	// Issue #28: the later page gets its own cancellation context so Ctrl+W
	// requests exactly this request's connection-scoped interrupt identity;
	// the handle retires when the request settles.
	pageCtx, pageCancel := context.WithCancel(context.Background())
	m.pageRequestCancel = pageCancel
	// Issue #28: a later-page request is its own cancellable unit, so the
	// generic cancellation seam re-arms for exactly this request; it closes
	// again when the page settles and nothing remains pending.
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return SelectCancelRequestedMsg{} }
	// Issue #26: the request's execution and viewport-generation identities
	// are immutable once captured; the response carries them back verbatim.
	m.pageRequestExecution = m.selectTracker.ExecutionID()
	m.pageRequestGeneration = m.viewportGen
	m.pageRequested = offset
	m.pageRequestedSize = size
	params := m.QB.PageParams()
	exec := m.Page
	execution, generation := m.pageRequestExecution, m.pageRequestGeneration
	return func() tea.Msg {
		return PageSettledMsg{
			ExecutionID: execution,
			RequestID:   requestID,
			Generation:  generation,
			Result:      exec(pageCtx, statement, params),
		}
	}
}

// pageRange computes the exact adjacent logical range: the page size always
// equals the layout's complete visible data rows (PageRows), and the offset
// starts immediately after the displayed page going forward or steps back by
// one page from the displayed start (clamped to offset zero with a
// correspondingly exact size) going backward.
func (m Model) pageRange(up bool) (size, offset int64, ok bool) {
	pageSize := int64(CalculateLayout(m.Height, m.Fields).PageRows)
	if up {
		if m.pageOffset <= 0 {
			return 0, 0, false // known low boundary
		}
		back := pageSize
		if back > m.pageOffset {
			back = m.pageOffset // exact backward range without reading before row 1
		}
		return back, m.pageOffset - back, true
	}
	if m.pageExhausted {
		return 0, 0, false // known high boundary: the last page was short
	}
	if pageSize < 1 {
		return 0, 0, false
	}
	return pageSize, m.pageOffset + int64(len(m.Result.Page.Rows)), true
}

// applyPageSettled applies a matched page completion under the full identity
// rule (Issue #26). Only a response whose request ID matches the one pending
// request settles its guard; a mismatched response — stale, duplicated, or
// from a replaced request — can never clear the newer request's pending
// feedback. Within that, rows, absolute range, and the exhausted boundary
// mutate only when the response's execution ID and viewport generation are
// also still current and the boundary has not classified it cancelled; any
// other settled outcome is inert except for releasing the pending slot.
func (m Model) applyPageSettled(msg PageSettledMsg) Model {
	if !m.pagePending || m.pageRequestID != msg.RequestID {
		return m // stale, duplicated, or wrong-request response: discarded
	}
	requested, requestedSize := m.pageRequested, m.pageRequestedSize
	m.pagePending = false
	m.pageRequestID = 0
	m.pageRequestExecution = 0
	m.pageRequestGeneration = 0
	m.pageRequestCancel = nil // Issue #28: the handle retires with the request
	if msg.Result.Err != nil {
		return m // ordinary failure keeps the previous page displayed
	}
	if msg.Result.Cancelled ||
		msg.ExecutionID != m.selectTracker.ExecutionID() ||
		msg.Generation != m.viewportGen {
		return m // cancelled or stale identity: rows, range, and cache unchanged
	}
	// Issue #31: byte-cap disclosure persists through subsequent traversal —
	// later pages inherit it so the header keeps showing the shared warning
	// even after navigation falls below the cap. A settled typed over-limit
	// failure travels with the view the same way.
	byteTruncated := m.Result != nil && m.Result.ByteTruncated
	prevFailure := (*result.LimitFailure)(nil)
	if m.Result != nil {
		prevFailure = m.Result.LimitFailure
	}
	// Issue #32: the accepted response merges into the authoritative
	// contiguous dual-cap cache by absolute logical position before it
	// becomes display state; the direction follows the serialized request
	// so eviction happens at the standard opposite end.
	forward := requested >= m.pageOffset
	if m.mergePageIntoCache(msg.Result.Page, requested, forward) {
		byteTruncated = byteTruncated || m.viewportCache.TruncatedByByteCap()
	}
	m.Result = &ResultView{Page: msg.Result.Page, Offset: requested, ByteTruncated: byteTruncated, LimitFailure: prevFailure}
	m.pageOffset = requested // the displayed start moves to the requested range
	if int64(len(msg.Result.Page.Rows)) < requestedSize {
		m.pageExhausted = true
	}
	return m
}

// resetPagingState clears the serialized paging bookkeeping for a fresh
// SELECT execution: a new execution displays its first page from offset zero
// with no pending request and no boundary knowledge.
func (m *Model) resetPagingState() {
	// Issue #32: a fresh execution owns a fresh contiguous dual-cap cache —
	// merging its first page into a previous result's retained range would
	// resurrect stale rows and break the cache invariants.
	m.viewportCache = resultcache.New()
	m.pageOffset = 0
	m.pageRequested = 0
	m.pageRequestedSize = 0
	m.pagePending = false
	m.pageRequestID = 0
	m.pageRequestExecution = 0
	m.pageRequestGeneration = 0
	m.pageRequestCancel = nil // Issue #28: no page request owns a handle yet
	m.pageExhausted = false
	// Issue #29: a fresh execution displays its first page from its first
	// whole column again.
	m.firstColumn = 0
}

// deactivateActiveSelect finalizes the active SELECT response window
// (Issue #26): advancing the viewport generation makes every in-flight
// first-page and later-page response inert. The finalization paths — result
// history entry, accepted quit, and any ending cancellation/failure — call
// this when they deactivate the SELECT; starting a new execution finalizes
// it here first.
func (m *Model) deactivateActiveSelect() {
	m.bumpViewportGeneration()
	// Issue #27: finalization releases the generic gate's first-page claim
	// and cancelling handoff; any late settlement cannot re-claim them.
	// Issue #28: every scoped cancellation handle retires with it.
	m.firstPagePending = false
	m.countPendingFlag = false
	m.selectCancelling = false
	m.firstPageCancel = nil
	m.countCancel = nil
	m.pageRequestCancel = nil
	m.resizeFetchPending = false // Issue #32: no replacement fetch outlives finalization
}
