// Destructive preparation modal inside the UI (Issue #40), per the Estimate
// SQL and modal decision in Notes/PRD-sqloid.md. A settled successful
// pre-execution validation of a runnable UPDATE or DELETE opens this modal
// immediately instead of the SELECT execution route, dispatches exactly one
// independent matching-target estimate, and keeps operation, table,
// QueryBuilder's canonical rendered SQL, and any no-WHERE all-rows warning
// continuously visible. Enter/y confirmation stays disabled through estimate
// settlement (Issue #41 owns enabling it); Esc/n dismisses, cancellation
// waits for settlement, and every preparation stage appends neither query
// nor result history and never starts the write.

package ui

import (
	"context"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/result"
)

// DestructivePrepPendingStatus is the exact status rendered while the
// matching-target estimate is in flight.
const DestructivePrepPendingStatus = "Estimating matching target rows…"

// DestructivePrepCancellingStatus is the exact status rendered from a Ctrl+W
// cancellation request until the estimate settles; settlement then dismisses
// preparation without history.
const DestructivePrepCancellingStatus = "cancelling…"

// EstimateResult is one settled matching-target estimate: Total is
// meaningful exactly on success (Err nil); Err is non-nil exactly on failure
// with its cause preserved. Cancelled records the Connection boundary's
// cancellation classification and stays fully inert at the response boundary.
type EstimateResult struct {
	Total     int64
	Err       error
	Cancelled bool
}

// EstimateExecutor performs one cancellable destructive estimate for the
// given safely rendered estimate statement and its WHERE-only ordered
// parameters, mapping the Connection boundary's outcomes onto the returned
// EstimateResult. It always runs inside a tea.Cmd.
type EstimateExecutor func(ctx context.Context, sql string, params []any) EstimateResult

// EstimateSettledMsg carries one settled estimate back through Update with
// the preparation identity that issued it, so late or superseded completions
// cannot mutate the modal. Produced only by commands this package created.
type EstimateSettledMsg struct {
	Preparation uint64
	Result      EstimateResult
}

// CancelEstimateMsg is produced by the cancellation closure dispatched at
// Ctrl+W over a pending estimate. The model already entered its cancelling
// state before dispatching; the modal dismisses at true settlement.
type CancelEstimateMsg struct{}

// WriteConfirmedMsg is the deliberate-confirmation seam (Issue #41): one
// message carrying the confirmed operation and the exact retained rendered
// write statement, bound to the consumed preparation identity and the newly
// allocated actual-write execution identity. Issue #42 owns consuming it for
// transactional execution and the execution-start history seam; re-delivery
// of an already-recorded execution identity is an inert no-op.
type WriteConfirmedMsg struct {
	Preparation uint64
	Execution   uint64
	Operation   string
	SQL         string
}

// destructivePrepLines returns the modal's content lines: the operation and
// table header, the canonical rendered write SQL, the retained estimate
// state, and the prominent all-rows warning only when the statement has no
// WHERE clause.
func (m Model) destructivePrepLines() []string {
	lines := []string{
		"DESTRUCTIVE " + m.prepOperation + " — table " + m.prepTable,
		"",
		m.prepSQL,
	}
	if m.prepNoWhere {
		lines = append(lines,
			"",
			"WARNING: no WHERE clause — this statement targets every row of "+m.prepTable)
	}
	lines = append(lines, "")
	switch {
	case m.prepCancelling:
		lines = append(lines, DestructivePrepCancellingStatus)
	case m.prepPending:
		lines = append(lines, DestructivePrepPendingStatus)
	case m.prepErr != "":
		lines = append(lines, "Estimate failed: "+m.prepErr)
	default:
		lines = append(lines, "Estimated matching target rows: "+strconv.Itoa(int(m.prepEstimate)))
	}
	if m.prepConfirmationReady() {
		lines = append(lines, "", "Enter/y confirms the write — Esc/n dismisses")
	} else {
		lines = append(lines, "", "Esc/n dismisses — Enter/y confirmation disabled while estimating")
	}
	return lines
}

// drawPrepOverlay composites the preparation modal box over the composed
// shell inside the results region, following the Issue #8 overlay pattern.
func (m Model) drawPrepOverlay(base string) string {
	lines := m.destructivePrepLines()
	longest := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > longest {
			longest = w
		}
	}
	w := longest + 2
	if w > m.Width-popupBorderCols {
		w = m.Width - popupBorderCols
	}
	if w < 4 {
		w = 4
	}
	box := valuePromptStyle.Width(w).Height(len(lines)).Render(strings.Join(lines, "\n"))
	return composeOverlay(base, box, 1, 1)
}

// beginPreparation opens the destructive modal under a fresh preparation
// identity and issues exactly one estimate command carrying the estimate
// statement and WHERE-only parameters captured from the builder at issue
// time. It is refused while a preparation is already open or the session is
// terminal; without a wired estimator nothing stays pending.
func (m *Model) beginPreparation() tea.Cmd {
	if m.terminalState != TerminalNone || m.prepOpen {
		return nil
	}
	m.prepAttempt++
	m.prepOpen = true
	m.prepOperation = m.QB.Command().String()
	if table, ok := m.QB.SelectedTable(); ok {
		m.prepTable = table
	}
	m.prepNoWhere = !m.QB.HasWhere()
	switch m.QB.Command() {
	case qb.CommandUpdate:
		m.prepSQL = m.QB.UpdateRenderedSQL()
	case qb.CommandDelete:
		m.prepSQL = m.QB.DeleteRenderedSQL()
	default:
		return nil
	}
	if m.prepSQL == "" {
		return nil
	}
	m.prepPending = true
	m.prepCancelling = false
	m.prepErr = ""
	m.ActiveCancellable = true
	statement := m.QB.EstimateSQL()
	params := m.QB.EstimateParams()
	if m.Estimator == nil {
		m.prepPending = false
		m.ActiveCancellable = false
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.prepCancel = cancel
	m.CancelCommand = func() tea.Msg { cancel(); return CancelEstimateMsg{} }
	estimate := m.Estimator
	attempt := m.prepAttempt
	return func() tea.Msg {
		return EstimateSettledMsg{Preparation: attempt, Result: estimate(ctx, statement, params)}
	}
}

// applyEstimateSettled transitions the model on one settled estimate.
// Superseded identities and terminal arrivals are discarded. With a current
// identity the pending slot releases; a requested cancellation dismisses
// preparation (cancellation wins over any retained outcome), while success
// retains the count and failure retains the cause — both preserving SQL,
// warning, and the confirmation-ready seam for Issue #41.
func (m *Model) applyEstimateSettled(msg EstimateSettledMsg) tea.Cmd {
	if msg.Preparation != m.prepAttempt || msg.Preparation == 0 || !m.prepPending || m.terminalState != TerminalNone {
		return nil
	}
	m.prepPending = false
	m.ActiveCancellable = false
	m.prepCancel = nil
	m.CancelCommand = nil
	if m.prepCancelling || msg.Result.Cancelled {
		// Cancellation requested before settlement, or a cancellation-
		// classified response: the estimate is discarded wholesale and
		// preparation dismisses without history.
		m.dismissPreparation()
		return nil
	}
	if msg.Result.Err != nil {
		m.prepErr = msg.Result.Err.Error()
		return nil
	}
	m.prepErr = ""
	m.prepEstimate = msg.Result.Total
	return nil
}

// dismissPreparation closes the modal and restores the exact pre-open
// builder context, advancing the identity so any still-outstanding response
// can never mutate the restored state. Nothing executes and nothing appends.
func (m *Model) dismissPreparation() {
	if !m.prepOpen {
		return
	}
	m.prepAttempt++
	if m.prepCancel != nil {
		m.prepCancel()
		m.prepCancel = nil
	}
	m.prepOpen = false
	m.prepPending = false
	m.prepCancelling = false
	m.ActiveCancellable = false
	m.CancelCommand = nil
}

// requestEstimateCancellation handles Ctrl+W over a pending estimate: it
// marks the modal cancelling (exact `cancelling…` until settlement) and
// dispatches the connection-scoped interrupt exactly once.
func (m *Model) requestEstimateCancellation() tea.Cmd {
	if !m.prepOpen || !m.prepPending || m.prepCancelling || m.CancelCommand == nil {
		return nil
	}
	m.prepCancelling = true
	return m.CancelCommand
}

// prepConfirmationReady reports whether the current preparation has settled
// — successfully or in failure — with the modal still open. Only then does
// Enter/y deliberately confirm the write; pending, cancelling, and dismissed
// preparations never confirm.
func (m Model) prepConfirmationReady() bool {
	return m.prepOpen && !m.prepPending && !m.prepCancelling && m.prepSQL != ""
}

// confirmPreparation performs the exactly-once confirmation transition
// (Issue #41): the first accepted Enter/y from a settled preparation closes
// it through its authoritative dismissal and emits one command producing a
// WriteConfirmedMsg carrying the retained operation and rendered statement
// bound to the consumed preparation identity and a fresh actual-write
// execution identity from the dedicated write identity space. Pending,
// cancelling, and dismissed preparations reject confirmation; the execution
// identity is allocated here and never re-used, so duplicate keys or
// messages can never start a second write.
func (m *Model) confirmPreparation() tea.Cmd {
	if !m.prepConfirmationReady() || m.terminalState != TerminalNone {
		return nil
	}
	m.writeAttempt++
	preparation := m.prepAttempt
	operation, sql := m.prepOperation, m.prepSQL
	m.dismissPreparation()
	return func() tea.Msg {
		return WriteConfirmedMsg{
			Preparation: preparation,
			Execution:   result.NextWriteExecutionID(),
			Operation:   operation,
			SQL:         sql,
		}
	}
}

// applyWriteConfirmed records the execution identity of one delivered
// confirmation. The preparation has already closed, so no key can confirm
// again; a duplicate or stale delivery is an inert no-op that can neither
// re-open anything nor touch history.
func (m *Model) applyWriteConfirmed(msg WriteConfirmedMsg) {
	if msg.Execution == 0 || msg.Execution <= m.confirmedExecution {
		return
	}
	m.confirmedExecution = msg.Execution
}

// handlePreparationKey consumes one key press while the modal is open.
// Enter/y confirm only from a settled success or settled failure (Issue
// #41); pending and cancelling preparations keep them consumed no-ops. Esc/n
// dismisses; everything else is inert so builder state cannot leak through
// the modal.
func (m *Model) handlePreparationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+w":
		// Scoped estimate cancellation: exact `cancelling…` until settlement.
		return *m, m.requestEstimateCancellation()
	case "esc", "n":
		m.dismissPreparation()
		return *m, nil
	case "enter", "y":
		if cmd := m.confirmPreparation(); cmd != nil {
			return *m, cmd
		}
		return *m, nil
	default:
		return *m, nil
	}
}
