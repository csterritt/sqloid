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
)

// PageExecutor performs one cancellable paged-page SELECT execution for the
// given safely rendered page statement (QueryBuilder's PageSQL, whose exact
// LIMIT/OFFSET range replaces the user's Limit clause) and ordered bound
// parameters, mapping the Connection boundary's outcomes onto the returned
// FirstPageResult. It always runs inside a tea.Cmd — never in Update or View.
type PageExecutor func(ctx context.Context, sql string, params []any) FirstPageResult

// PageSettledMsg carries one settled paged-page execution back through
// Update. Produced only by commands this package created; it mutates state
// only while the one pending page request's ID still matches.
type PageSettledMsg struct {
	RequestID uint64
	Result    FirstPageResult
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
	m.pageRequested = offset
	m.pageRequestedSize = size
	params := m.QB.PageParams()
	exec := m.Page
	return func() tea.Msg {
		return PageSettledMsg{RequestID: requestID, Result: exec(context.Background(), statement, params)}
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

// applyPageSettled applies a matched page completion. Success installs the
// page with its absolute logical range; a page shorter than the requested
// size marks the known high boundary. Ordinary failures keep the previous
// page displayed (their error boundary is owned by later issues). Any
// settled outcome, matched or not, clears exactly one pending slot: a
// mismatched ID can never release the current request's guard, so the
// pending flag is only cleared by the request it was issued under.
func (m Model) applyPageSettled(requestID uint64, res FirstPageResult) Model {
	if !m.pagePending || m.pageRequestID != requestID {
		return m // stale, duplicated, or wrong-request response: discarded
	}
	m.pagePending = false
	m.pageRequestID = 0
	if res.Err != nil {
		return m
	}
	m.Result = &ResultView{Page: res.Page, Offset: m.pageRequested}
	m.pageOffset = m.pageRequested // the displayed start moves to the requested range
	if int64(len(res.Page.Rows)) < m.pageRequestedSize {
		m.pageExhausted = true
	}
	return m
}

// resetPagingState clears the serialized paging bookkeeping for a fresh
// SELECT execution: a new execution displays its first page from offset zero
// with no pending request and no boundary knowledge.
func (m *Model) resetPagingState() {
	m.pageOffset = 0
	m.pageRequested = 0
	m.pageRequestedSize = 0
	m.pagePending = false
	m.pageRequestID = 0
	m.pageExhausted = false
}
