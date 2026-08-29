// Resize-safe vertical viewport recovery orchestration inside the UI (Issue
// #32 Tasks 2 and 4), per the SELECT lifecycle, Cache and snapshot invariant,
// Module Design, and resize Testing Decisions in Notes/PRD-sqloid.md. Every
// visible resize (including suspension restoration) recomputes the exact page
// size from complete visible data rows and applies the pure decision seam to
// the authoritative dual-cap cache metadata: preserve or clamp locally, or
// dispatch exactly one containing-page request at the exact new size. While
// an old-size page request is pending, the resize advances the viewport
// generation, cancels/invalidates that old work, and defers the replacement
// request until true settlement; late success and late failure from the old
// generation are rejected, repeated resizes coalesce to the latest decision,
// count work stays independent, and inactive/finalized contexts never fetch.

package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// viewportCacheOf returns the active SELECT's cache, constructing it lazily
// so page-only fixtures and pre-execution models keep a safe empty cache.
func (m *Model) viewportCacheOf() *resultcache.Cache {
	if m.viewportCache == nil {
		m.viewportCache = resultcache.New()
	}
	return m.viewportCache
}

// applyResizeRecovery runs the Issue #32 vertical recovery after resize set
// the new dimensions: with no active successful result, no work runs; with
// one pending old-size page request, the pure decision still resolves the
// required row, the old request is cancelled/invalidated, and a fetch is
// deferred to true settlement; an idle fetch dispatches immediately through
// the returned command. Inactive, finalized, suspended, and validating
// contexts never fetch.
func (m *Model) applyResizeRecovery() tea.Cmd {
	if m.suspended || m.terminalState != TerminalNone || m.validating {
		return nil
	}
	// Issue #34: inactive/finalized SELECTs never fetch recovery pages.
	if !m.selectActive || m.Result == nil || m.Result.Page == nil {
		return nil
	}
	pageSize := int64(CalculateLayout(m.Height, m.Fields).PageRows)
	meta := ViewportMetaFromCache(
		m.viewportCache,
		m.pageExhausted,
		m.countState.Status == result.CountSuccess,
		m.countState.Total,
	)
	// The viewport's required row is the prior first logical row of the
	// displayed page (absolute one-based logical position).
	decision := RecoverViewport(meta, m.pageOffset+1, pageSize)
	switch decision.Action {
	case RecoveryPreserve, RecoveryClampLow, RecoveryClampHigh:
		if m.pagePending {
			// Local recovery keeps the exact row or a known endpoint while the
			// old-size request becomes stale: advance-style invalidation has
			// already happened via the resize generation bump; the scoped
			// cancellation handle stops the connection work itself.
			if m.pageRequestCancel != nil {
				m.pageRequestCancel()
			}
		}
		m.rebuildRetainedView(decision.Action, decision.FirstRow, pageSize)
		m.resizeFetchPending = false
		return nil
	default:
		m.resizeFetchRow = decision.FirstRow
		m.resizeFetchSize = decision.Size
		if m.pagePending {
			// The old-size request must settle before any replacement: cancel
			// its scoped handle and defer exactly one correctly sized request.
			if m.pageRequestCancel != nil {
				m.pageRequestCancel()
			}
			m.resizeFetchPending = true
			return nil
		}
		m.resizeFetchPending = false
		return m.requestRecoveryPage(decision.FirstRow, decision.Size)
	}
}

// rebuildRetainedView re-slices the displayed result from the retained cache
// rows so the viewport shows exactly the decided first logical row and as
// many complete rows of the exact new page size as remain retained. Clamp
// decisions to the high endpoint also mark the exhausted boundary known.
func (m *Model) rebuildRetainedView(action RecoveryAction, firstRow, size int64) {
	c := m.viewportCache
	if c == nil || m.Result == nil || m.Result.Page == nil {
		return
	}
	start, ok := c.Start()
	if !ok {
		return
	}
	end, _ := c.End()
	lo, hi := firstRow, firstRow+size-1
	if hi > int64(end) {
		hi = int64(end) // show the final retained page ending at the boundary
	}
	if lo > hi {
		lo = max64(int64(start), hi-size+1)
	}
	var rows [][]result.Value
	for _, r := range c.Rows() {
		if int64(r.Position) < lo || int64(r.Position) > hi {
			continue
		}
		rows = append(rows, r.Values)
	}
	if len(rows) == 0 {
		return // nothing retained at the decided position: keep the current view
	}
	byteTruncated := m.Result.ByteTruncated || c.TruncatedByByteCap()
	m.Result = &ResultView{
		Page:          &result.Page{Columns: m.Result.Page.Columns, Rows: rows},
		Offset:        lo - 1,
		ByteTruncated: byteTruncated,
		LimitFailure:  m.Result.LimitFailure,
	}
	m.pageOffset = lo - 1
	if action == RecoveryClampHigh {
		m.pageExhausted = true // the clamped endpoint is the known final row
	}
}

// requestRecoveryPage dispatches exactly one cancellable containing-page
// request for the page of `size` rows whose first absolute logical row is
// firstRow, through the same identity/generation/cancellation lifecycle as
// ordinary serialized paging (Issues #25/#26/#28).
func (m *Model) requestRecoveryPage(firstRow, size int64) tea.Cmd {
	// Issue #34: a finalized or inactive SELECT issues no further page work.
	if !m.selectActive || size < 1 {
		return nil
	}
	offset := firstRow - 1 // the statement is LIMIT size OFFSET firstRow-1
	statement := m.QB.PageSQL(size, offset)
	if statement == "" {
		// Unrenderable snapshot or a range at/beyond the user's Limit: no
		// database work is issued and the display keeps its local decision.
		return nil
	}
	requestID := result.NextSelectRequestID()
	m.pagePending = true
	m.pageRequestID = requestID
	pageCtx, pageCancel := context.WithCancel(context.Background())
	m.pageRequestCancel = pageCancel
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return SelectCancelRequestedMsg{} }
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

// mergePageIntoCache merges one accepted settled page into the authoritative
// contiguous dual-cap cache by absolute logical position. requestedOffset is
// the page's zero-based absolute offset; the traversal direction chooses the
// eviction end exactly as serialized paging does (forward evicts low,
// backward evicts high). A nonadjacent stale page is rejected atomically and
// changes nothing. The returned byte-truncation disclosure includes any
// eviction the merge itself caused.
func (m *Model) mergePageIntoCache(p *result.Page, requestedOffset int64, requestedForward bool) bool {
	if p == nil || len(p.Rows) == 0 {
		return false
	}
	page := resultcache.Page{Start: resultcache.Position(requestedOffset + 1)}
	for _, values := range p.Rows {
		page.Rows = append(page.Rows, resultcache.Row{
			Position: resultcache.Position(requestedOffset + 1 + int64(len(page.Rows))),
			Values:   values,
		})
	}
	dir := resultcache.Forward
	if !requestedForward {
		dir = resultcache.Backward
	}
	accepted, _ := m.viewportCacheOf().Merge(page, dir)
	return accepted
}

// max64 returns the larger of two int64 values.
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
