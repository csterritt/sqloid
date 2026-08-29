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
// Ctrl+W cancellation hint.
func (m Model) inFlightEnterFeedback() string {
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

// handleInFlightGate applies the generic request-in-flight gate to one key
// that reached the otherwise-base context with SELECT work pending. It
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
		m.inFlightNotice = QueryHistoryBlockedFeedback
		return *m, nil, true
	case "ctrl+e", "ctrl+y":
		m.inFlightNotice = ResultHistoryBlockedFeedback
		return *m, nil, true
	case "ctrl+s":
		m.inFlightNotice = SaveBlockedFeedback
		return *m, nil, true
	case "ctrl+x":
		m.inFlightNotice = ExportBlockedFeedback
		return *m, nil, true
	case "q", "ctrl+c":
		return m.openQuitConfirmation(), nil, true
	case "ctrl+w":
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
