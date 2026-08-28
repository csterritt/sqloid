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
// Update. Produced only by commands this package created.
type SelectSettledMsg struct {
	Result FirstPageResult
}

// ResultView is the model's complete settled first-page state. Page is
// non-nil exactly on success; Err is non-nil exactly on ordinary failure.
// Render helpers read these through internal/result only.
type ResultView struct {
	Page *result.Page
	Err  error
}

// startSelectPage returns the command that issues the one first-page request
// at the actual-execution boundary. The builder's SQL and parameters are
// captured here, after history has already appended for this execution, so
// the request carries exactly the validated builder state. A nil executor
// yields no command because there is no database work to do.
func (m *Model) startSelectPage() tea.Cmd {
	if m.Select == nil {
		return nil
	}
	exec := m.Select
	sql := m.QB.SelectSQL()
	params := m.QB.SelectParams()
	return func() tea.Msg {
		return SelectSettledMsg{Result: exec(context.Background(), sql, params)}
	}
}

// applySelectSettled stores the settled completion as fresh result state,
// replacing any previous result outright. Ordinary failures land on the
// result-error boundary exactly like successes; no history entry is undone
// and no builder state changes.
func (m Model) applySelectSettled(res FirstPageResult) Model {
	m.Result = &ResultView{Page: res.Page, Err: res.Err}
	return m
}
