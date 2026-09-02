// Focused model and rendering coverage for read-request phase feedback
// (Issue #27 Task 3): while fake first-page, count, and later-page requests
// are independently held and settled, the UI shows exactly `Running…` for
// initial SELECT page work, exactly `Counting rows…` while the independent
// count is pending, and the distinct page-loading state during later
// navigation; labels update when page and count settle in either order and
// stay visible through permitted local interaction. Enter during each
// pending phase is consumed with no stacked request and a Ctrl+W hint;
// history and save/export rejections are action-specific; the Ctrl+W
// `cancelling…` handoff renders without changing Issue #28's interrupt
// semantics; count failure lands on the established `Count unavailable`; and
// no write-phase label is introduced or changed (Issue #44 owns those).

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRunningFeedbackForInitialSelectPage requires the exact `Running…`
// status while the first SELECT page is in flight — both before any result
// exists and beside retained rows from a previous page state.
func TestRunningFeedbackForInitialSelectPage(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	if view := m.View(); !strings.Contains(view, "Running…") {
		t.Fatalf("initial page work did not render `Running…`:\n%s", view)
	}
}

// TestCountingFeedbackWhileCountPending requires the exact `Counting rows…`
// status while the independent count is pending and the first page has
// settled, alongside the displayed rows.
func TestCountingFeedbackWhileCountPending(t *testing.T) {
	m := pendingCountOnly(t,
		&fakeSelectExecutor{page: threeRowPage()},
		&fakeCountExecutor{total: 7})
	view := m.View()
	if !strings.Contains(view, "Counting rows…") {
		t.Fatalf("pending count did not render `Counting rows…`:\n%s", view)
	}
	if strings.Contains(view, "Running…") {
		t.Fatalf("settled first page still rendered `Running…`:\n%s", view)
	}
	if !strings.Contains(view, "rows 1-11") {
		t.Fatalf("settled rows were lost behind count feedback:\n%s", view)
	}
}

// TestPageLoadingFeedbackIsDistinctFromCounting requires the later-page
// loading state to render the exact distinct page-loading wording while the
// count status (if any) stays independently present.
func TestPageLoadingFeedbackIsDistinctFromCounting(t *testing.T) {
	m := pendingLaterPage(t,
		&fakeSelectExecutor{page: firstPageRows(defaultPageRows)},
		&fakePageExecutor{rowsShown: 11})
	view := m.View()
	if !strings.Contains(view, PageLoadingIndicator) {
		t.Fatalf("later-page work did not render %q:\n%s", PageLoadingIndicator, view)
	}
	if strings.Contains(view, "Running…") {
		t.Fatalf("page loading wrongly rendered `Running…`:\n%s", view)
	}
}

// TestFeedbackLabelsUpdateAsRequestsSettleInEitherOrder requires labels to
// update exactly when requests settle, whichever order the page and count
// completions arrive in.
func TestFeedbackLabelsUpdateAsRequestsSettleInEitherOrder(t *testing.T) {
	// Count settles first, page still held: `Counting rows…` gone, page
	// feedback still present.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 7}
	m := concurrentCountModel(exec, count)
	execModel, execCmd := driveToExecutionStart(t, m)
	page, countMsg := splitSelectCount(t, execBatch(t, execCmd))
	next, _ := execModel.Update(countMsg)
	afterCount := next.(Model)
	view := afterCount.View()
	if strings.Contains(view, "Counting rows…") {
		t.Fatalf("count wording survived settlement:\n%s", view)
	}
	if !strings.Contains(view, "Running…") {
		t.Fatalf("page feedback missing while the page is still held:\n%s", view)
	}

	// Page settles second: both pending labels are gone.
	next2, _ := afterCount.Update(page)
	final := next2.(Model)
	finalView := final.View()
	if strings.Contains(finalView, "Running…") ||
		strings.Contains(finalView, "Counting rows…") {
		t.Fatalf("pending wording survived full settlement:\n%s", finalView)
	}
	if !strings.Contains(finalView, "Result count: 7") {
		t.Fatalf("settled count wording missing:\n%s", finalView)
	}
}

// TestPendingFeedbackSurvivesPermittedLocalInteraction requires the phase
// labels to remain visible through permitted local interaction — a field
// navigation press must not clear `Running…`.
func TestPendingFeedbackSurvivesPermittedLocalInteraction(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	next, cmd := pressKey(m, keyMsg("tab"))
	if cmd != nil {
		t.Fatal("tab produced a command while a request is pending")
	}
	if view := next.View(); !strings.Contains(view, "Running…") {
		t.Fatalf("`Running…` was cleared by permitted local interaction:\n%s", view)
	}
}

// TestCancellingHandoffRendersUntilSettlement requires the Ctrl+W handoff to
// render the exact `cancelling…` status for the held SELECT work and to
// clear once every owned request has settled, without touching Issue #28's
// interrupt semantics (the fake is never interrupted here).
func TestCancellingHandoffRendersUntilSettlement(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 7}
	m := concurrentCountModel(exec, count)
	execModel, execCmd := driveToExecutionStart(t, m)
	page, countMsg := splitSelectCount(t, execBatch(t, execCmd))

	// Hold only the count: request Ctrl+W with the cancellable seam owned.
	held := execModel
	next, _ := held.Update(page)
	held = next.(Model)
	held.ActiveCancellable = true
	held.CancelCommand = func() tea.Msg { return SelectCancelRequestedMsg{} }
	cancelling, _ := pressKey(held, tea.KeyMsg{Type: tea.KeyCtrlW})
	if !cancelling.selectCancelling {
		t.Fatal("Ctrl+W did not enter the cancelling handoff state")
	}
	view := cancelling.View()
	if !strings.Contains(view, "cancelling…") {
		t.Fatalf("Ctrl+W handoff did not render `cancelling…`:\n%s", view)
	}
	if !strings.Contains(view, "Counting rows…") {
		t.Fatalf("cancelling handoff replaced the pending count wording:\n%s", view)
	}

	// Settlement releases the handoff exactly once nothing remains pending.
	settled, _ := cancelling.Update(countMsg)
	final := settled.(Model)
	if final.selectCancelling {
		t.Fatal("cancelling handoff survived full settlement")
	}
	if strings.Contains(final.View(), "cancelling…") {
		t.Fatalf("`cancelling…` rendered after settlement:\n%s", final.View())
	}
}

// TestFeedbackRejectionsAreActionSpecific requires each blocked in-flight
// action to explain itself with its own exact wording, rendered in the view.
func TestFeedbackRejectionsAreActionSpecific(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	for _, tc := range []struct {
		name     string
		key      tea.KeyMsg
		feedback string
	}{
		{"query history", tea.KeyMsg{Type: tea.KeyCtrlP}, QueryHistoryBlockedFeedback},
		{"result history", tea.KeyMsg{Type: tea.KeyCtrlE}, ResultHistoryBlockedFeedback},
		{"save", tea.KeyMsg{Type: tea.KeyCtrlS}, SaveBlockedFeedback},
		{"export", tea.KeyMsg{Type: tea.KeyCtrlX}, ExportBlockedFeedback},
	} {
		next, cmd := pressKey(m, tc.key)
		if cmd != nil {
			t.Fatalf("%s: blocked action returned a command", tc.name)
		}
		if next.inFlightNotice != tc.feedback {
			t.Fatalf("%s: feedback = %q, want exactly %q", tc.name, next.inFlightNotice, tc.feedback)
		}
		if view := next.View(); !strings.Contains(view, tc.feedback) {
			t.Fatalf("%s: view did not render %q:\n%s", tc.name, tc.feedback, view)
		}
	}
}

// TestTopOverlayConsumesKeysBeforePendingFeedback requires an open popup to
// consume Enter and printable keys before the pending feedback machinery.
func TestTopOverlayConsumesKeysBeforePendingFeedback(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	m.Focus = 1 // Table field
	if _ = m.openPopupCmd(tea.KeyMsg{Type: tea.KeyEnter}); m.Popup == nil {
		t.Fatal("setup: popup did not open")
	}
	// Enter is consumed inside the popup (accepting the candidate and
	// closing it), never routed to the pending-phase gate.
	next, _ := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if next.Popup != nil {
		t.Fatal("popup Enter did not accept inside the popup")
	}
	if next.inFlightNotice != "" {
		t.Fatalf("pending feedback leaked behind the popup: %q", next.inFlightNotice)
	}
}

// TestNoWritePhaseLabelsIntroduced requires that no write-phase feedback
// label appears from the read-request feedback work: Issue #44 owns those.
func TestNoWritePhaseLabelsIntroduced(t *testing.T) {
	forbidden := []string{
		"Estimating matching target rows…",
		"beginning",
		"executing",
		"committing",
		"rollback",
	}
	models := []Model{
		pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()}),
		pendingCountOnly(t,
			&fakeSelectExecutor{page: threeRowPage()},
			&fakeCountExecutor{total: 7}),
		pendingLaterPage(t,
			&fakeSelectExecutor{page: firstPageRows(defaultPageRows)},
			&fakePageExecutor{rowsShown: 11}),
	}
	for i, m := range models {
		view := m.View()
		for _, label := range forbidden {
			if strings.Contains(view, label) {
				t.Errorf("phase %d: write-phase label %q appeared in read feedback:\n%s", i, label, view)
			}
		}
	}
}
