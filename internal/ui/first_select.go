// Production first-page SELECT orchestration inside the UI (Issue #22), per
// the Execution and Result Lifecycle decisions in Notes/PRD-sqloid.md. Only
// the successful current validation handoff from Issue #21 reaches the
// execution route; the actual-execution identity is that boundary, and query
// history appends exactly when the ExecutionStartedMsg is handled — before
// any database work is issued. The wired SelectExecutor seam runs the one
// first-page request inside a returned tea.Cmd, receives exactly the SQL and
// ordered parameters rendered by QueryBuilder, and settles into Result
// state: a typed result.Page on success, or the ordinary result-error
// boundary on failure. No database call, driver type, or count/paging/cache
// behavior appears in Bubble Tea state.

package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// SelectExecutor performs one cancellable first-page SELECT execution for
// the given safely rendered SQL and ordered bound parameters, mapping the
// Connection boundary's outcomes (including Issue #7 health classification)
// onto the returned FirstPageResult. It always runs inside a tea.Cmd.
type SelectExecutor func(ctx context.Context, sql string, params []any) FirstPageResult

// FirstPageResult is one settled first-page execution: exactly one of Page
// (non-nil on OutcomeSuccess) or Err (non-nil on failure, cause preserved)
// is meaningful. Cancelled records the Connection boundary's cancellation
// classification (Issue #26): a success arriving after cancellation was
// requested, or the work failing with a cancellation error, is classified
// cancelled and stays fully inert at the response boundary — it can never
// mutate rows, range, or retained cache.
type FirstPageResult struct {
	Page      *result.Page
	Err       error
	Cancelled bool
	// ByteTruncated carries persistent `truncated-by-byte-cap` disclosure
	// (Issue #31): it is set once byte-cap eviction has occurred and stays
	// set through later navigation and finalization.
	ByteTruncated bool
	// LimitFailure is the typed Issue #31 over-limit failure (page envelope
	// or connection-local value) with its one-based logical position, if the
	// page settled as such a failure. Its Error renders the exact shared
	// message from internal/result.
	LimitFailure *result.LimitFailure
}

// SelectSettledMsg carries one settled first-page execution back through
// Update with the full identity (Issues #24 and #26) that guards it: the
// SELECT execution ID that produced it, the first-page request ID, and the
// viewport generation current at dispatch. Produced only by commands this
// package created.
type SelectSettledMsg struct {
	ExecutionID uint64
	RequestID   uint64
	Generation  uint64
	Result      FirstPageResult
}

// ResultView is the model's complete settled first-page state. Page is
// non-nil exactly on success; Err is non-nil exactly on ordinary failure.
// Offset is the absolute logical offset of the displayed page's first row
// (Issue #25): zero for every first page, the requested offset for paged
// pages. Render helpers read these through internal/result only.
type ResultView struct {
	Page   *result.Page
	Err    error
	Offset int64
	// ByteTruncated is the persistent Issue #31 `truncated-by-byte-cap`
	// disclosure; once set it survives subsequent page traversal so the
	// header keeps showing the shared warning.
	ByteTruncated bool
	// LimitFailure is the typed Issue #31 over-limit failure carried from
	// the settled page; it is rendered through its single Error definition.
	LimitFailure *result.LimitFailure
}

// startSelectPage returns the commands that issue the two concurrent
// requests of one actual SELECT execution (Issue #24): the first page and the
// complete-limited-result count. One fresh nonzero execution ID and two
// distinct role-specific request IDs are assigned here, the tracker guarding
// both completions is installed, and the executed builder's user Limit is
// captured as count metadata for the exact after-Limit wording. The page and
// count requests are launched in one batch without waiting for either result;
// each acquires its own dedicated lease and runs as an independent autocommit
// read. The builder's SQL and parameters are captured here, after history has
// already appended for this execution, so the requests carry exactly the
// validated builder state. A nil page executor yields no command because
// there is no database work to do; a nil count executor leaves the count
// state unset (page-only fixtures from Issue #22 are unaffected). Issue
// #27: opening the execution claims the first-page slot for the generic
// in-flight gate (firstPagePending) and marks the model as owning
// cancellable work so Ctrl+W routes to the scoped cancellation seam.
func (m *Model) startSelectPage() tea.Cmd {
	if m.Select == nil {
		return nil
	}
	exec := result.NextSelectExecutionID()
	// Issue #34: starting an actual new execution finalizes the previous
	// active SELECT (if any) before the active lifetime moves to the new ID.
	m.activateSelect(exec)
	m.resetPagingState() // a fresh execution pages from its first page again
	pageID := result.NextSelectRequestID()
	countID := result.NextSelectRequestID()
	m.selectTracker = result.NewSelectTracker(exec, pageID, countID)
	generation := m.viewportGen
	// Issue #27: claim the first-page slot and the generic cancellation seam
	// before anything dispatches; both release at their request's settlement.
	// Issue #28: each request gets its own derived cancellation context so
	// Ctrl+W requests one independent connection-scoped interrupt identity
	// per active request; each handle retires exactly at its settlement.
	m.firstPagePending = true
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return SelectCancelRequestedMsg{} }

	pageCtx, pageCancel := context.WithCancel(context.Background())
	countCtx, countCancel := context.WithCancel(context.Background())
	m.firstPageCancel = pageCancel
	m.countCancel = countCancel

	pageFn := m.Select
	sql := m.QB.SelectSQL()
	params := m.QB.SelectParams()
	pageCmd := func() tea.Msg {
		return SelectSettledMsg{
			ExecutionID: exec,
			RequestID:   pageID,
			Generation:  generation,
			Result:      pageFn(pageCtx, sql, params),
		}
	}
	countFn := m.Count
	if countFn == nil {
		// No count executor wired: page-only execution as in Issue #22.
		return pageCmd
	}
	countSQL := m.QB.CountSQL()
	countCmd := func() tea.Msg {
		return CountSettledMsg{
			ExecutionID: exec,
			RequestID:   countID,
			Result:      countFn(countCtx, countSQL, params),
		}
	}
	m.countState = result.CountState{Status: result.CountPending}
	m.countPendingFlag = true // Issue #27: the gate owns the count claim too
	if limit, has := m.QB.LimitValue(); has {
		m.countState.HasLimit, m.countState.Limit = true, limit
	}
	// Issue #24: both requests launch now, neither waiting for the other.
	return tea.Batch(pageCmd, countCmd)
}

// applySelectSettled stores the settled completion as fresh result state,
// replacing any previous result outright. Ordinary failures land on the
// result-error boundary exactly like successes; no history entry is undone
// and no builder state changes. Responses classified cancelled by the
// Connection boundary never reach this seam: the Update guard rejects them
// so rows, cache, and pending feedback stay untouched.
func (m Model) applySelectSettled(res FirstPageResult) Model {
	if res.Err == nil && res.Cancelled {
		return m // defensive: cancellation classification is fully inert here
	}
	m.Result = &ResultView{Page: res.Page, Err: res.Err}
	// Issue #32: the first page of the fresh execution seeds the active
	// contiguous dual-cap cache at absolute positions 1..len before it
	// becomes display state.
	if res.Page != nil {
		m.mergePageIntoCache(res.Page, 0, true)
	}
	return m
}
