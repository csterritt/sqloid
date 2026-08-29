// Independent result-count lifecycle inside the UI (Issue #24), per the
// Paging consistency decision in Notes/PRD-sqloid.md. The count of one SELECT
// execution is a second concurrent request with its own role-specific
// identity: it launches alongside the first page without waiting for either
// result, runs as an independent autocommit read with no shared snapshot, and
// settles into its own presentation state — exact successful variants, or
// exact `Count unavailable` on failure — without ever becoming a page failure
// or clamping displayed rows. Help records that the count covers the complete
// limited SELECT result and may drift independently of the displayed rows.

package ui

import (
	"context"

	"github.com/chris/sqloid/internal/result"
)

// CountExecutor performs one cancellable complete-SELECT count execution for
// the given safely rendered count statement and ordered bound parameters,
// mapping the Connection boundary's outcomes (including Issue #7 health
// classification) onto the returned CountResult. It always runs inside a
// tea.Cmd, concurrently with the first page and never after it.
type CountExecutor func(ctx context.Context, sql string, params []any) CountResult

// CountResult is one settled count execution: Total is meaningful exactly on
// success (Err nil); Err is non-nil exactly on failure with its cause
// preserved. Cancelled records the Connection boundary's cancellation
// classification (Issue #26): a post-cancellation success or cancellation
// error is classified cancelled and stays fully inert at the response
// boundary — it is neither an exact total nor the exact failure wording.
type CountResult struct {
	Total     int64
	Err       error
	Cancelled bool
}

// CountSettledMsg carries one settled count execution back through Update
// with the two-level identity that guards it. Produced only by commands this
// package created.
type CountSettledMsg struct {
	ExecutionID uint64
	RequestID   uint64
	Result      CountResult
}

// CountHelpLines returns the exact user-facing help for the independent
// result count: it counts the complete limited SELECT (the user's Limit stays
// inside the counted subquery), not the table size or a pre-Limit size, and
// may drift independently of the displayed rows because page and count are
// separate autocommit reads with no shared snapshot. Rows are never clamped
// to an inconsistent count.
func CountHelpLines() []string {
	return []string{
		"The result count covers the complete limited SELECT result — the user's",
		"Limit stays inside the counted subquery — not the table size or a",
		"pre-Limit size. The count and the first page run concurrently as",
		"independent autocommit reads with no shared snapshot, so the count may",
		"drift independently of the displayed rows; rows are never clamped to an",
		"inconsistent count.",
	}
}

// applyCountSettled stores a tracker-accepted count completion as the count's
// own presentation state. Success records the total beside the executed
// Limit metadata captured at launch; failure records the exact
// `Count unavailable` state. Neither outcome ever touches page rows, errors,
// history, or builder state, and rows are never clamped to the total.
// Responses classified cancelled by the Connection boundary never reach this
// seam: the Update guard rejects them so the pending presentation stays.
func (m Model) applyCountSettled(res CountResult) Model {
	if res.Err == nil && res.Cancelled {
		return m // defensive: cancellation classification is fully inert here
	}
	if res.Err != nil {
		m.countState.Status = result.CountUnavailable
		return m
	}
	m.countState.Status = result.CountSuccess
	m.countState.Total = res.Total
	return m
}
