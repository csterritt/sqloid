// Generic request-in-flight action gating inside the UI (Issue #27), per the
// Global Key Precedence and Context/Action Matrix in Notes/PRD-sqloid.md. The
// gate sits at the authoritative precedence point between focused
// input/overlays and base-context handling. It derives pending state purely
// from active request ownership and settlement flags maintained by the
// SELECT/page/count seams — never from rendered phase-label strings — and
// consumes execution, query-history, result-history, save, and export actions
// with contextual feedback and no command dispatch while any SELECT read
// request is in flight. Permitted local horizontal one-column movement stays
// ungated (Issue #29 owns it), `q`/Ctrl+C open the shared quit confirmation,
// and Ctrl+W routes to scoped cancellation only while a cancellable request
// is owned. Page keys remain governed by Issue #25's page-pending rule and
// write-phase integration stays with Issue #44.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// Exact in-flight gate feedback strings (Issue #27). The Enter hint names the
// Ctrl+W cancellation route; every blocked action explains why it was
// rejected. These are presentation-only: the gate itself never inspects them.
const (
	// SelectRunningIndicator is the exact `Running…` feedback rendered while
	// the first-page request of an actual SELECT execution is in flight.
	SelectRunningIndicator = "Running…"

	// QueryHistoryBlockedFeedback is the exact rejection for Ctrl+P/N while a
	// request is in flight.
	QueryHistoryBlockedFeedback = "query history is unavailable while a request is in flight"
	// ResultHistoryBlockedFeedback is the exact rejection for Ctrl+E/Y while
	// a request is in flight.
	ResultHistoryBlockedFeedback = "result history is unavailable while a request is in flight"
	// SaveBlockedFeedback is the exact rejection for Ctrl+S while a request
	// is in flight.
	SaveBlockedFeedback = "saving is unavailable while a request is in flight"
	// ExportBlockedFeedback is the exact rejection for Ctrl+X while a request
	// is in flight.
	ExportBlockedFeedback = "export is unavailable while a request is in flight"
	// CancelHintSuffix is the exact Ctrl+W hint appended to Enter feedback in
	// every pending read phase.
	CancelHintSuffix = "press Ctrl+W to cancel"
)

// SelectCancellingIndicator is the exact status rendered from the moment a
// Ctrl+W cancellation has been requested for SELECT work until every owned
// read request settles (mirroring the established validation wording).
const SelectCancellingIndicator = "cancelling…"

// SelectCancelRequestedMsg is the message produced by the generic
// cancellation closure dispatched at Ctrl+W for SELECT work. The model has
// already entered its cancelling state before dispatching, so the message
// itself settles nothing — real interrupt semantics stay with Issue #28.
type SelectCancelRequestedMsg struct{}

// selectRequestPending reports whether any SELECT read request is currently
// in flight: the first page, one later page, or the independent count. It is
// the single authoritative pending source for the generic gate; nothing here
// reads rendered phase-label text.
func (m Model) selectRequestPending() bool {
	return m.firstPagePending || m.pagePending || m.countPendingFlag
}

// inFlightEnterFeedback composes the contextual Enter-rejection feedback for
// the current pending phase: the phase's status wording plus the exact
// Ctrl+W cancellation hint. Issue #44: a pending write feeds the same
// composition — the typed write-phase status for the cancellable phases, or
// the exact post-boundary message once rollback cleanup or committing has
// begun.
func (m Model) inFlightEnterFeedback() string {
	if m.writePending {
		if m.writeNoncancellable {
			return CommitBoundaryFeedback
		}
		return m.writePhaseStatus() + " — " + CancelHintSuffix
	}
	var phase string
	switch {
	case m.firstPagePending:
		phase = SelectRunningIndicator
	case m.pagePending:
		phase = PageLoadingIndicator
	case m.countPendingFlag:
		phase = result.CountState{Status: result.CountPending}.Header()
	}
	return phase + " — " + CancelHintSuffix
}

// inFlightBlockedFeedback returns the exact explanatory rejection for one of
// the gate's blocked actions, keyed by the key string. It is the single
// shared mapping used by both the base-context gate and the Issue #44
// estimate modal; the gate itself never inspects these strings.
func inFlightBlockedFeedback(key string) string {
	switch key {
	case "ctrl+p", "ctrl+n":
		return QueryHistoryBlockedFeedback
	case "ctrl+e", "ctrl+y":
		return ResultHistoryBlockedFeedback
	case "ctrl+s":
		return SaveBlockedFeedback
	case "ctrl+x":
		return ExportBlockedFeedback
	default:
		return ""
	}
}

// handleInFlightGate applies the generic request-in-flight gate to one key
// that reached the otherwise-base context with SELECT work pending or, since
// Issue #44, a write pending in any of its typed phases. It
// returns (handled model, command) for every key the gate consumes; callers
// fall through to base handling for permitted local interaction such as
// horizontal one-column movement. Feedback is recorded in the model so View
// can render it without re-deriving anything.
func (m *Model) handleInFlightGate(msg tea.KeyMsg) (next tea.Model, cmd tea.Cmd, handled bool) {
	switch msg.String() {
	case "enter":
		// Enter is consumed without another execution: requests never stack.
		m.inFlightNotice = m.inFlightEnterFeedback()
		return *m, nil, true
	case "ctrl+p", "ctrl+n":
		m.inFlightNotice = inFlightBlockedFeedback(msg.String())
		return *m, nil, true
	case "ctrl+e", "ctrl+y":
		m.inFlightNotice = inFlightBlockedFeedback(msg.String())
		return *m, nil, true
	case "ctrl+s":
		m.inFlightNotice = inFlightBlockedFeedback(msg.String())
		return *m, nil, true
	case "ctrl+x":
		m.inFlightNotice = inFlightBlockedFeedback(msg.String())
		return *m, nil, true
	case "q", "ctrl+c":
		return m.openQuitConfirmation(), nil, true
	case "ctrl+w":
		if m.writePending {
			// Issue #44: the write's typed state routes Ctrl+W. In the
			// noncancellable rollback-cleanup/committing phases the key is
			// ignored with the exact boundary feedback and the work is never
			// mutated; in the cancellable phases the cancellation request is
			// deduplicated to exactly one per write and exact `cancelling…`
			// holds until settlement. Routing never inspects label text.
			if m.writeNoncancellable {
				m.inFlightNotice = CommitBoundaryFeedback
				return *m, nil, true
			}
			if !m.writeCancelling && m.CancelCommand != nil {
				m.writeCancelling = true
				return *m, m.CancelCommand, true
			}
			return *m, nil, true
		}
		if m.ActiveCancellable && m.CancelCommand != nil {
			// Scoped cancellation: mark SELECT work cancelling before the
			// command dispatches, exactly like the validation workflow. With
			// no cancellable request the key is ignored with no state change.
			if m.selectRequestPending() {
				m.selectCancelling = true
			}
			return *m, m.CancelCommand, true
		}
		return *m, nil, true
	}
	// Permitted local interaction (horizontal one-column movement, serialized
	// page keys, and every key the gate does not own) falls through to base
	// handling unchanged; the notice clears so stale explanations never
	// persist across permitted actions.
	m.inFlightNotice = ""
	return nil, nil, false
}

// openQuitConfirmation suspends the exact current model behind the shared
// quit confirmation, capturing a full copy so Esc/n can restore that exact
// context with no key leakage.
func (m *Model) openQuitConfirmation() tea.Model {
	copied := *m
	m.quitConfirm = true
	m.quitSuspended = &copied
	return *m
}

// handleQuitConfirmKey dispatches one key inside the shared quit
// confirmation, which sits above every other context: Enter/y/Ctrl+C
// confirms, Esc/n restores the exact suspended context, and every other key
// is consumed with no leakage.
func (m *Model) handleQuitConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y", "ctrl+c":
		// Issue #43: an accepted quit during a pending write enters the write
		// settlement coordinator instead of exiting; every other context
		// finalizes its cleanup and exits immediately, as before.
		if m.writePending {
			return m.acceptQuitWithWrite()
		}
		// Issue #34: accepted quit finalizes the active SELECT once — with
		// required cancellation of still-owned read requests — before exit.
		m.acceptedQuitCleanup()
		return *m, tea.Quit
	case "esc", "n":
		restored := *m.quitSuspended
		restored.quitConfirm = false
		restored.quitSuspended = nil
		return restored, nil
	}
	return *m, nil
}

// acceptQuitWithWrite is the write-side accepted-quit coordinator (Issue
// #43): for cancellable work it requests cancellation exactly once and waits
// through rollback resolution; for the noncancellable rollback-cleanup or
// committing phases it issues no interrupt and waits for the existing
// operation. In both cases the model stays alive with no exit command while
// the transaction or driver work remains pending; the exit command is
// emitted only when settlement finalizes (Update's WriteSettledMsg case).
// Repeated acceptance is an idempotent no-op that neither exits early nor
// requests a second cancellation.
func (m *Model) acceptQuitWithWrite() (tea.Model, tea.Cmd) {
	if m.quitWaitWrite {
		return *m, nil
	}
	m.quitWaitWrite = true
	m.quitConfirm = false
	m.quitSuspended = nil
	var cmd tea.Cmd
	if !m.writeNoncancellable && !m.writeCancelling && m.CancelCommand != nil {
		m.writeCancelling = true
		cmd = m.CancelCommand
	}
	return *m, cmd
}
