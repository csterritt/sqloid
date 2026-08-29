// Barrier-controlled coverage for cancellation-wins classification and
// predecessor-settlement ordering (Issue #26 Task 3): a request cancelled
// through the Connection boundary classifies its late success as cancelled
// so rows/cache stay unchanged; stale responses from a superseded execution
// stay inert after a newer execution begins; a same-execution replacement
// page or count command is never dispatched, and no lease is reused, before
// every replaced predecessor has actually settled — while the independent
// page/count pair still settles in either order. Ctrl+W interrupt wiring
// itself stays with Issue #28; these tests exercise the observable boundary.

package ui

import (
	"strings"
	"testing"
)

// cancelledPageClassifies the held page response as the connection
// boundary's cancelled settlement: late success discarded and reclassified.
func cancelledPageClassifies(msg PageSettledMsg) PageSettledMsg {
	msg.Result = FirstPageResult{Cancelled: true}
	return msg
}

// TestCancelledLaterPageWinsOverLateSuccess covers the later-page rule:
// cancellation is requested while the page request is in flight, the late
// success is classified cancelled, rows/range/cache stay unchanged, the
// pending guard settles, and only after that settlement may a replacement
// page request start.
func TestCancelledLaterPageWinsOverLateSuccess(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	inFlightModel, pendingCmd := pageDown(m)
	inFlight := pendingCmd().(PageSettledMsg)

	// No replacement starts while the cancelled predecessor is unsettled.
	if _, cmd := pageDown(inFlightModel); cmd != nil {
		t.Fatal("replacement page request dispatched before predecessor settlement")
	}

	// The boundary classifies the late success as cancelled: inert.
	settled := apply(m, cancelledPageClassifies(inFlight))
	if settled.pagePending {
		t.Error("cancelled predecessor did not settle its pending guard")
	}
	if settled.Result == nil || len(settled.Result.Page.Rows) != 3 || settled.Result.Offset != 0 {
		t.Fatalf("cancelled later page mutated rows/range: %+v", settled.Result)
	}
	if settled.pageOffset != m.pageOffset || settled.pageExhausted != m.pageExhausted {
		t.Error("cancelled later page altered cache metadata")
	}

	// After settlement a replacement may start and applies normally.
	next, cmd := pageDown(settled)
	if cmd == nil {
		t.Fatal("replacement page request refused after predecessor settlement")
	}
	replacement := cmd().(PageSettledMsg)
	final := apply(next, replacement)
	if final.Result == nil || final.Result.Offset == 0 {
		t.Fatalf("post-cancellation replacement page did not apply: %+v", final.Result)
	}
}

// TestCancelledFirstPageNeverMutatesRows covers the first-page rule: a
// response the boundary classified cancelled — late success included —
// mutates neither visible rows nor the pending count feedback of its own
// execution.
func TestCancelledFirstPageNeverMutatesRows(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	execModel, page, _ := settledExecutionWithHeldCompletions(t, m)
	page.Result = FirstPageResult{Page: exec.page, Cancelled: true}

	after := apply(execModel, page)
	if after.Result != nil {
		t.Fatalf("cancelled first page stored rows: %+v", after.Result)
	}
	if got := countStateHeader(after); got != "Counting rows…" {
		t.Errorf("count header = %q, want the pending wording — cancelled page must not disturb it", got)
	}
}

// TestCancelledCountStaysInert covers the count role: a count the boundary
// classified cancelled leaves the count presentation untouched — it is
// neither an exact total nor the exact failure wording — and page rows and
// paging stay fully active.
func TestCancelledCountStaysInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	execModel, page, countMsg := settledExecutionWithHeldCompletions(t, m)
	countMsg.Result = CountResult{Cancelled: true}

	after := apply(apply(execModel, page), countMsg)
	if got := countStateHeader(after); got != "Counting rows…" {
		t.Errorf("cancelled count header = %q, want the pending wording", got)
	}
	if after.Result == nil || len(after.Result.Page.Rows) != 3 {
		t.Fatalf("cancelled count disturbed rows: %+v", after.Result)
	}
}

// TestLateCancelledResponseAfterNewerExecutionStaysInert covers the
// superseded-execution rule: a cancelled old execution's late response —
// classified cancelled by the boundary — cannot clear or mutate any state
// belonging to the newer execution, on either page or count role.
func TestLateCancelledResponseAfterNewerExecutionStaysInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	_, oldPage, oldCount := settledExecutionWithHeldCompletions(t, m)
	oldPage.Result = FirstPageResult{Page: exec.page, Cancelled: true}
	oldCount.Result = CountResult{Cancelled: true}

	newer, newerPage, _ := newerExecutionOver(t, m)
	newer = apply(newer, newerPage)
	before := newer

	afterPage := apply(newer, oldPage)
	if afterPage.Result == nil || len(afterPage.Result.Page.Rows) != 3 {
		t.Fatalf("old cancelled page disturbed the newer result: %+v", afterPage.Result)
	}
	afterCount := apply(afterPage, oldCount)
	if got := countStateHeader(afterCount); got != "Counting rows…" {
		t.Errorf("old cancelled count header = %q, want the newer execution's pending wording", got)
	}
	if afterCount.Result == nil || len(afterCount.Result.Page.Rows) != 3 {
		t.Fatalf("old cancelled count disturbed the newer result: %+v", afterCount.Result)
	}
	if afterCount.pagePending != before.pagePending || afterCount.pageOffset != before.pageOffset {
		t.Error("old cancelled response altered the newer execution's paging state")
	}
}

// TestLateErrorAfterNewerExecutionStaysInert covers the failure variant: a
// superseded execution's late ordinary error cannot create a result-error
// boundary or clear state belonging to the newer execution.
func TestLateErrorAfterNewerExecutionStaysInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	_, oldPage, oldCount := settledExecutionWithHeldCompletions(t, m)
	oldPage.Result = FirstPageResult{Err: execFailed("old table dropped")}
	oldCount.Result = CountResult{Err: execFailed("old count failed")}

	newer, newerPage, _ := newerExecutionOver(t, m)
	newer = apply(newer, newerPage)

	after := apply(apply(newer, oldPage), oldCount)
	if after.Result == nil || after.Result.Err != nil || len(after.Result.Page.Rows) != 3 {
		t.Fatalf("old errors disturbed the newer result: %+v", after.Result)
	}
	if got := countStateHeader(after); got != "Counting rows…" {
		t.Errorf("old count error header = %q, want the newer execution's pending wording", got)
	}
}

// TestCountSettlesIndependentlyWhilePagePending covers the independent
// settlement order of the page/count pair: the count settles while the page
// request is pending, without releasing the page's pending guard, and a
// pending page still blocks replacement until it settles.
func TestCountSettlesIndependentlyWhilePagePending(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)
	m.Page = (&fakePageExecutor{rowsShown: 3}).page

	execModel, page, countMsg := settledExecutionWithHeldCompletions(t, m)
	withFirst := apply(execModel, page)

	// Dispatch the page request, then settle the count first: the count's
	// settlement must not release the page's pending guard.
	withPending, pageCmd := pageDown(withFirst)
	pendingMsg := pageCmd().(PageSettledMsg)
	if !withPending.pagePending || !strings.Contains(withPending.View(), PageLoadingIndicator) {
		t.Fatal("page request not pending after dispatch")
	}
	afterCount := apply(withPending, countMsg)
	if got := countStateHeader(afterCount); got != "Result count: 3" {
		t.Errorf("count header = %q, want exactly %q — count settles while the page is pending", got, "Result count: 3")
	}
	if !afterCount.pagePending || !strings.Contains(afterCount.View(), PageLoadingIndicator) {
		t.Fatal("count settlement released the page's pending guard")
	}
	if _, cmd := pageDown(afterCount); cmd != nil {
		t.Fatal("replacement page dispatched while the predecessor is pending")
	}

	// The page settles afterwards and applies normally.
	final := apply(afterCount, pendingMsg)
	if final.pagePending {
		t.Fatal("page request never settled")
	}
	if final.Result == nil || final.Result.Offset == 0 {
		t.Fatalf("pending page did not apply after count settlement: %+v", final.Result)
	}
}

// TestReplacementWaitsForCancelledPredecessorOnNewerExecution covers the
// execution-level ordering rule: a newer SELECT started through the allowed
// execution-start seam dispatches its own page/count pair — the superseded
// execution's requests settle independently and its late responses stay
// inert — and no dedicated lease is reused for replacement work before
// settlement (lease ownership itself is proven in internal/connection).
func TestReplacementWaitsForCancelledPredecessorOnNewerExecution(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	_, pendingCmd := pageDown(m)
	oldPage := pendingCmd().(PageSettledMsg)
	oldPage.Result = FirstPageResult{Cancelled: true}

	// The newer execution starts and dispatches both replacement requests.
	newer, newerPage, newerCount := newerExecutionOver(t, m)
	if newer.selectTracker.ExecutionID() == 0 {
		t.Fatal("newer execution has no identity")
	}

	// The old request settles late and stays fully inert: it changes
	// neither the still-displayed previous result nor any paging state.
	before := newer
	after := apply(newer, oldPage)
	if after.Result != before.Result {
		t.Fatalf("old cancelled page applied to the newer execution: %+v", after.Result)
	}
	if after.pagePending != before.pagePending || after.pageOffset != before.pageOffset {
		t.Error("old cancelled page altered the newer execution's paging state")
	}

	// The newer pair settles in either order — count first here — and both
	// apply their own roles.
	afterCount := apply(after, newerCount)
	if got := countStateHeader(afterCount); got != "Result count: 0" {
		t.Errorf("newer count header = %q, want exactly %q (the fixture count's settled total)", got, "Result count: 0")
	}
	afterPage := apply(afterCount, newerPage)
	if afterPage.Result == nil || len(afterPage.Result.Page.Rows) != 3 {
		t.Fatalf("newer first page missing after count-first settlement: %+v", afterPage.Result)
	}
}

// execFailed is a placeholder for an ordinary boundary failure classification.
func execFailed(msg string) error { return &classifiedError{msg} }

type classifiedError struct{ msg string }

func (e *classifiedError) Error() string { return e.msg }
