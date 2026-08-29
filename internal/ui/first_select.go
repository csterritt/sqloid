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
// is meaningful. Cancelled requests are not yet producible: Ctrl+W routing
// for SELECT pages arrives with later issues.
type FirstPageResult struct {
	Page *result.Page
	Err  error
}

// SelectSettledMsg carries one settled first-page execution back through
// Update with the two-level identity (Issue #24) that guards it: the SELECT
// execution ID that produced it and the first-page request ID. Produced only
// by commands this package created.
type SelectSettledMsg struct {
	ExecutionID uint64
	RequestID   uint64
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
// state unset (page-only fixtures from Issue #22 are unaffected).
func (m *Model) startSelectPage() tea.Cmd {
	if m.Select == nil {
		return nil
	}
	m.resetPagingState() // a fresh execution pages from its first page again
	exec := result.NextSelectExecutionID()
	pageID := result.NextSelectRequestID()
	countID := result.NextSelectRequestID()
	m.selectTracker = result.NewSelectTracker(exec, pageID, countID)

	pageFn := m.Select
	sql := m.QB.SelectSQL()
	params := m.QB.SelectParams()
	pageCmd := func() tea.Msg {
		return SelectSettledMsg{
			ExecutionID: exec,
			RequestID:   pageID,
			Result:      pageFn(context.Background(), sql, params),
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
			Result:      countFn(context.Background(), countSQL, params),
		}
	}
	m.countState = result.CountState{Status: result.CountPending}
	if limit, has := m.QB.LimitValue(); has {
		m.countState.HasLimit, m.countState.Limit = true, limit
	}
	// Issue #24: both requests launch now, neither waiting for the other.
	return tea.Batch(pageCmd, countCmd)
}

// applySelectSettled stores the settled completion as fresh result state,
// replacing any previous result outright. Ordinary failures land on the
// result-error boundary exactly like successes; no history entry is undone
// and no builder state changes.
func (m Model) applySelectSettled(res FirstPageResult) Model {
	m.Result = &ResultView{Page: res.Page, Err: res.Err}
	return m
}
