// Explicit active-SELECT lifecycle inside the UI (Issue #34), per the
// Identities and state, SELECT, and active-finalization decisions in
// Notes/PRD-sqloid.md. The active SELECT is distinct from any individual
// request in flight: it stays active between requests and owns future
// serialized page requests across builder edits, overlays, save/export,
// estimates, query-history navigation, resize, count/page settlement, count
// failure, and idle periods. It is finalized only by the exhaustive list of
// finalizing events — an actual new execution, entering result history, a
// cancellation or failure that ends the SELECT, and an accepted quit — and
// finalization is idempotent per execution ID: late or duplicate messages
// from a finalized execution can never reactivate or mutate it, and exactly
// one immutable result-history entry is created for each actual execution.

package ui

import (
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// SelectIsActive reports whether the model currently owns an active SELECT
// execution. The active SELECT is separate from request state: it survives
// every non-finalizing event and every individual request settlement, and
// only the finalizing events end it. Nothing here reads rendered text.
func (m Model) SelectIsActive() bool { return m.selectActive }

// ActiveSelectExecutionID returns the execution ID of the currently active
// SELECT, or zero when no SELECT is active.
func (m Model) ActiveSelectExecutionID() uint64 {
	if !m.selectActive {
		return 0
	}
	return m.activeExecID
}

// FinalizedSelectExecutionID returns the execution ID most recently
// finalized, or zero when no SELECT has been finalized yet. Late or duplicate
// finalization of the same execution is a deterministic no-op.
func (m Model) FinalizedSelectExecutionID() uint64 { return m.finalizedExecID }

// activateSelect begins the active-SELECT lifetime for a freshly started
// execution. The previous active SELECT — if any — is finalized first, so an
// actual new execution is the first finalizer: starting a real execution
// finalizes before replacing the active execution.
func (m *Model) activateSelect(execID uint64) {
	m.finalizeActiveSelect()
	m.selectActive = true
	m.activeExecID = execID
}

// enterResultHistory is the result-history finalizer seam (Issue #34): the
// result-history navigation owned by Issue #36 invokes it when it displays a
// retained entry, which deactivates the active SELECT and invalidates future
// page mutation. Repeated invocation for an already-finalized execution is a
// deterministic no-op.
func (m *Model) enterResultHistory() {
	m.finalizeActiveSelect()
}

// acceptedQuitCleanup performs the cleanup an accepted quit requires: any
// still-owned read requests are cancelled and the active SELECT is finalized
// exactly once before teardown. It is invoked only from the accepted-quit
// path; repeated invocation is a deterministic no-op.
func (m *Model) acceptedQuitCleanup() {
	m.finalizeActiveSelect()
}

// finalizeActiveSelect finalizes the active SELECT exactly once for its
// execution ID. It records the finalized identity before anything else, so a
// duplicate or late finalizer message cannot append a second result entry or
// rewrite the first, then deactivates the response window, cancels any
// still-owned read requests, and appends the execution's one immutable
// snapshot to the result-history store. A model with no active SELECT, or one
// whose execution was already finalized, is an unchanged no-op.
func (m *Model) finalizeActiveSelect() {
	if !m.selectActive || m.activeExecID == 0 || m.activeExecID == m.finalizedExecID {
		return
	}
	m.finalizedExecID = m.activeExecID
	m.deactivateActiveSelect()
	m.selectActive = false
	m.activeExecID = 0
	m.selectCancelling = false
	m.ActiveCancellable = false
	m.CancelCommand = nil
	m.appendFinalizedResultEntry()
	// The recorded ending has been consumed into the snapshot; a later
	// execution's finalization must never be colored by it.
	m.clearPendingEnding()
}

// appendFinalizedResultEntry converts the finalized execution's authoritative
// state into exactly one immutable history result entry. A tabular snapshot
// (success, count failure with rows, or partial page failure after rows)
// carries the cache's retained rows in ascending logical position order with
// the Issue #33 metadata and truthful completeness classification; when
// cancellation or a first-page failure ended the SELECT before any row was
// retained, the defined non-tabular Cancelled or error entry is created
// instead. The snapshot is independent of the model: later cache or source
// mutation cannot change it.
func (m *Model) appendFinalizedResultEntry() {
	if m.suppressFinalizedAppend {
		// Issue #46: a health-failed execution has no truthful snapshot — the
		// database is gone or was replaced — so finalization deactivates the
		// SELECT without appending an entry. Consumed exactly once.
		m.suppressFinalizedAppend = false
		return
	}
	if m.ResultHistory == nil {
		return
	}
	failure := m.pendingFailure
	cancelReason := m.pendingCancelReason
	outcome := history.OutcomeSuccess
	reason := ""
	if cancelReason != "" {
		outcome = history.OutcomeCancelled
		reason = cancelReason
	} else if failure != nil {
		outcome = history.OutcomeFailed
		reason = failure.Error()
	}

	// Retained rows in ascending logical position order from the
	// authoritative dual-cap cache.
	var rows [][]result.Value
	var columns []string
	if m.viewportCache != nil {
		for _, r := range m.viewportCache.Rows() {
			rows = append(rows, r.Values)
		}
	}
	hasRows := len(rows) > 0
	if m.Result != nil && m.Result.Page != nil {
		columns = m.Result.Page.Columns
	}

	// Endpoint observations: the low end is reached when position 1 was
	// retained or the low end was evicted by cap traversal; the high end is
	// reached through a short/empty observed final page or a settled count
	// not exceeding the retained end. Issue #73: an observed short or empty
	// first page (pageExhausted) establishes the high endpoint even when the
	// cache retained no rows; an empty observed page also establishes the
	// low endpoint because the result is empty and both endpoints sit at
	// position 0.
	reachedLow, reachedHigh := false, false
	if m.viewportCache != nil {
		if start, ok := m.viewportCache.Start(); ok {
			reachedLow = start == 1 || m.viewportCache.RowCapEvictions() > 0
		}
		if end, ok := m.viewportCache.End(); ok {
			reachedHigh = m.pageExhausted ||
				(m.countState.Status == result.CountSuccess && m.countState.Total <= int64(end))
		}
	}
	if m.pageExhausted {
		reachedHigh = true
		if !hasRows {
			reachedLow = true
		}
	}
	// Issue #78: derive count/cache inconsistency from the same authoritative
	// cache snapshot used for rows and range, before SnapshotFacts. Count and
	// cache are independent autocommit facts: a successful limited-result
	// count whose total falls below the retained cache end contradicts the
	// cache. The contradiction is recorded without rewriting the total,
	// retained range, endpoint observations, rows, or count state; the
	// corrected history.Classify from Issue #77 then rejects complete.
	countCacheInconsistent := false
	if m.countState.Status == result.CountSuccess && m.viewportCache != nil {
		if end, ok := m.viewportCache.End(); ok && int64(end) > m.countState.Total {
			countCacheInconsistent = true
		}
	}
	// Issue #75: source invalid-UTF truth from the accepted active page so
	// it enters the immutable snapshot through Finalization. The persistent
	// byte-cap truth is already sourced from the authoritative cache via
	// FactsFromCache in SnapshotFacts, never re-derived from payload size.
	// Issue #76: source the typed limit-failure kind and one-based position
	// from the accepted active ResultView so they enter the immutable
	// snapshot as typed facts independent of the terminal outcome.
	invalidUTF := false
	if m.Result != nil && m.Result.Page != nil {
		invalidUTF = m.Result.Page.InvalidUTF
	}
	var limitKind result.LimitKind
	var limitPos int64
	if m.Result != nil && m.Result.LimitFailure != nil {
		limitKind = m.Result.LimitFailure.Kind
		limitPos = m.Result.LimitFailure.Position
	}
	final := Finalization{
		Outcome:                outcome,
		Reason:                 reason,
		ReachedLow:             reachedLow,
		ReachedHigh:            reachedHigh,
		InvalidUTF:             invalidUTF,
		LimitFailureKind:       limitKind,
		LimitFailurePosition:   limitPos,
		CountWorkFinished:      m.countState.Status != result.CountPending,
		PageWorkFinished:       !m.pagePending,
		ObservedShortFinalPage: m.pageExhausted,
		CountCacheInconsistent: countCacheInconsistent,
	}
	meta, traversal, err := m.SnapshotFacts(final)
	if err != nil {
		// Snapshot facts come from validated model state; a construction
		// error must not create a second entry, so the execution stays
		// finalized with no entry rather than a malformed one.
		return
	}
	if !hasRows {
		kind := history.KindTabular // an observed empty result
		if outcome == history.OutcomeCancelled {
			kind = history.KindCancelled
		} else if outcome == history.OutcomeFailed {
			kind = history.KindError
		}
		m.ResultHistory.AppendFinalized(history.ResultEntry{
			ExecutionID:  m.finalizedExecID,
			Kind:         kind,
			Reason:       reason,
			Metadata:     meta,
			Completeness: history.Classify(meta, traversal),
			QueryEntryID: m.lastExecQueryEntryID,
		})
		return
	}
	m.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID:  m.finalizedExecID,
		Kind:         history.KindTabular,
		Columns:      columns,
		Rows:         rows,
		Metadata:     meta,
		Completeness: history.Classify(meta, traversal),
		QueryEntryID: m.lastExecQueryEntryID,
	})
}

// noteSelectCancelled records a cancellation that ends the SELECT or, when
// rows are already retained, the ending of one interrupted page request whose
// settlement classified it cancelled. The reason is never overwritten by a
// later note, so the first recorded ending wins and a subsequent healthy page
// settlement clears it (execution continues).
func (m *Model) noteSelectCancelled() {
	if m.pendingCancelReason == "" && m.pendingFailure == nil {
		m.pendingCancelReason = "cancelled"
	}
}

// noteSelectFailed records a failure that ends the SELECT: a first-page
// ordinary failure (before rows) or a later-page failure after retained rows.
// The reason is never overwritten by a later note; a healthy later page
// settlement clears it, because the execution continues after the failure.
func (m *Model) noteSelectFailed(reason string) {
	if m.pendingFailure == nil && m.pendingCancelReason == "" {
		m.pendingFailure = &selectFailure{reason: reason}
	}
}

// clearPendingEnding clears the recorded terminal ending once finalization
// consumed it, so a later execution's finalization is never colored by it.
func (m *Model) clearPendingEnding() {
	m.pendingCancelReason = ""
	m.pendingFailure = nil
}

// selectFailure carries one recorded terminal failure reason.
type selectFailure struct {
	reason string
}

func (f *selectFailure) Error() string { return f.reason }
