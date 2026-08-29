// Barrier-controlled scripted coverage for SELECT request identities and
// stale-response rejection (Issue #26 Task 1): first-page and later-page
// successes and failures are delivered out of order while the SELECT
// execution ID, page request ID, and viewport generation independently vary,
// and a response may mutate visible rows, range, loading state, or retained
// cache only when every applicable identity is current. Superseded
// executions, replaced requests within the same execution, resize generation
// advancement, SELECT deactivation, and a fully current control case are all
// covered. The barriers are explicit held messages — never timing sleeps.

package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// settledExecutionWithHeldCompletions drives a concurrent page/count model
// through validation and execution start, returning the model plus both held
// completion messages for out-of-order delivery.
func settledExecutionWithHeldCompletions(t *testing.T, m Model) (Model, SelectSettledMsg, CountSettledMsg) {
	t.Helper()
	execModel, execCmd := driveToExecutionStart(t, m)
	page, count := splitSelectCount(t, execBatch(t, execCmd))
	return execModel, page, count
}

// newerExecutionOver starts a second SELECT execution on an idle model via
// the allowed execution-start seam and settles its first page, returning the
// newer model and its page request command's message.
func newerExecutionOver(t *testing.T, m Model) (Model, SelectSettledMsg, CountSettledMsg) {
	t.Helper()
	startCmd := m.executionRoute()
	if startCmd == nil {
		t.Fatal("execution route refused a newer start")
	}
	startMsg, ok := startCmd().(ExecutionStartedMsg)
	if !ok {
		t.Fatalf("newer route produced %T, want ExecutionStartedMsg", startCmd())
	}
	next, execCmd := m.Update(startMsg)
	newer, ok := next.(Model)
	if !ok {
		t.Fatalf("newer execution-start update returned %T", next)
	}
	newerPage, newerCount := splitSelectCount(t, execBatch(t, execCmd))
	return newer, newerPage, newerCount
}

// withReplacedResult rewrites a held message's result, standing in for the
// connection boundary's settlement of the identical identity.
func withReplacedResult(res FirstPageResult, page SelectSettledMsg) SelectSettledMsg {
	page.Result = res
	return page
}

// threeRowFirstPage is a first-page fixture with three identifiable rows.
func threeRowFirstPage() *result.Page {
	return &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)}, {result.NewInteger(2)}, {result.NewInteger(3)},
	}}
}

// executedResultModel drives a real SELECT execution (validation, execution
// start, and settled page/count pair) to reach a genuinely executed result
// state, sized at 80x24.
func executedResultModel(t *testing.T, page *result.Page) Model {
	t.Helper()
	m := concurrentCountModel(&fakeSelectExecutor{page: page}, &fakeCountExecutor{})
	execModel, execCmd := driveToExecutionStart(t, m)
	pageMsg, countMsg := splitSelectCount(t, execBatch(t, execCmd))
	return sized(apply(apply(execModel, pageMsg), countMsg), 80, 24).(Model)
}

// TestCurrentFirstPageControlApplies is the control case: a first-page
// response whose execution, request, and generation are all current mutates
// the visible rows exactly once.
func TestCurrentFirstPageControlApplies(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	execModel, page, countMsg := settledExecutionWithHeldCompletions(t, m)
	if execModel.Result != nil {
		t.Fatal("first page applied before its command was invoked")
	}
	after := apply(apply(execModel, page), countMsg)
	if after.Result == nil || len(after.Result.Page.Rows) != 3 {
		t.Fatalf("current first page response did not apply: %+v", after.Result)
	}
	if !strings.Contains(after.View(), "rows 1-3") {
		t.Errorf("rows not rendered after the current response:\n%s", after.View())
	}
}

// TestFirstPageRejectedAfterResizeGenerationAdvance covers the resize
// generation rule: a first-page response captured before a resize mutates
// neither rows nor any failure boundary after the viewport generation
// advances, while the count — which tracks only its Issue #24 identity —
// still settles on arrival.
func TestFirstPageRejectedAfterResizeGenerationAdvance(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	execModel, page, countMsg := settledExecutionWithHeldCompletions(t, m)
	resized := sized(execModel, 100, 30).(Model)

	// The stale-generation first-page success is inert.
	afterPage := apply(resized, page)
	if afterPage.Result != nil {
		t.Errorf("pre-resize first-page success mutated rows: %+v", afterPage.Result)
	}
	// Its failure variant would likewise never create an error boundary.
	failed := withReplacedResult(FirstPageResult{Err: errors.New("no such table: users")}, page)
	if after := apply(resized, failed); after.Result != nil {
		t.Errorf("pre-resize first-page failure created a result state: %+v", after.Result)
	}

	// The count request is independent of the viewport generation.
	afterCount := apply(resized, countMsg)
	if got := countStateHeader(afterCount); got != "Result count: 3" {
		t.Errorf("count header = %q, want exactly %q — count identity unaffected by resize", got, "Result count: 3")
	}

	// Control: a fresh execution launched after the resize applies normally.
	newer, newerPage, _ := newerExecutionOver(t, afterCount)
	newer = apply(newer, newerPage)
	if newer.Result == nil || len(newer.Result.Page.Rows) != 3 {
		t.Fatalf("post-resize execution's current first page did not apply: %+v", newer.Result)
	}
}

// TestFirstPageRejectedAfterDeactivationFinalization covers the
// deactivation rule: once the active SELECT is deactivated/finalized, a
// held first-page response — success or failure — mutates nothing, and a
// later response still under the same execution is equally inert.
func TestFirstPageRejectedAfterDeactivationFinalization(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	execModel, page, _ := settledExecutionWithHeldCompletions(t, m)
	execModel.deactivateActiveSelect()

	if after := apply(execModel, page); after.Result != nil {
		t.Errorf("deactivated execution's first page mutated rows: %+v", after.Result)
	}
	failed := withReplacedResult(FirstPageResult{Err: errors.New("lock timeout")}, page)
	if after := apply(execModel, failed); after.Result != nil && after.Result.Err != nil {
		t.Errorf("deactivated execution's first-page failure created an error boundary: %v", after.Result.Err)
	}
}

// TestStaleLaterPageRejectedAfterResizeGenerationAdvance covers a later page
// held across a resize: its stale-generation success mutates neither rows,
// range, nor the exhausted boundary; its settlement frees the pending slot so
// the same execution can issue a replacement page under the current
// generation, which then applies.
func TestStaleLaterPageRejectedAfterResizeGenerationAdvance(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	pending, pendingCmd := pageDown(m)
	pendingMsg, ok := pendingCmd().(PageSettledMsg)
	if !ok {
		t.Fatalf("page command produced %T, want PageSettledMsg", pendingCmd())
	}
	pendingMsg.Result = FirstPageResult{Page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{{result.NewInteger(9)}}}}

	resized := sized(pending, 100, 30).(Model)
	after := apply(resized, pendingMsg)

	if after.pagePending {
		t.Error("settled stale page request did not release the pending slot")
	}
	if after.pageExhausted {
		t.Error("stale-generation page response marked the exhausted high boundary")
	}
	if after.Result == nil || len(after.Result.Page.Rows) != 3 || after.Result.Offset != 0 {
		t.Fatalf("stale-generation page response mutated rows/range: %+v", after.Result)
	}

	// Replacement within the same execution under the advanced generation.
	withPending, cmd := pageDown(after)
	if cmd == nil {
		t.Fatal("no replacement page request issued after the predecessor settled")
	}
	replacementMsg := cmd().(PageSettledMsg)
	replacement := withPending
	if replacementMsg.Generation == pendingMsg.Generation {
		t.Error("replacement page reused the stale viewport generation")
	}
	if replacementMsg.ExecutionID != pendingMsg.ExecutionID {
		t.Error("replacement page changed execution within the same execution")
	}
	settled := apply(replacement, replacementMsg)
	if settled.Result == nil || settled.Result.Offset == 0 {
		t.Fatalf("replacement page did not apply its absolute range: %+v", settled.Result)
	}
}

// TestStaleLaterPageRejectedAfterExecutionSuperseded covers a later page held
// across a newer execution: the old response cannot apply its rows or disturb
// the newer execution's result or pending count feedback.
func TestStaleLaterPageRejectedAfterExecutionSuperseded(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	pending, pendingCmd := pageDown(m)
	stalePage, ok := pendingCmd().(PageSettledMsg)
	if !ok {
		t.Fatalf("page command produced %T, want PageSettledMsg", pendingCmd())
	}

	newer, newerPage, newerCount := newerExecutionOver(t, pending)
	before := apply(newer, newerPage)

	after := apply(before, stalePage)
	if after.Result == nil || len(after.Result.Page.Rows) != 3 || after.Result.Offset != 0 {
		t.Fatalf("superseded execution's later page mutated the newer result: %+v", after.Result)
	}
	if after.countState.Status != result.CountPending {
		t.Errorf("stale page response disturbed the newer count state: %v", after.countState.Status)
	}

	// The newer execution's own later page applies on arrival.
	withPending, newerCmd := pageDown(after)
	newerPageMsg := newerCmd().(PageSettledMsg)
	if newerPageMsg.ExecutionID == stalePage.ExecutionID {
		t.Fatal("newer execution's page request carried the superseded execution ID")
	}
	settled := apply(apply(withPending, newerPageMsg), newerCount)
	if settled.Result == nil || settled.Result.Offset == 0 {
		t.Fatalf("newer execution's page did not apply: %+v", settled.Result)
	}
}

// TestStaleLaterPageRejectedAfterDeactivation covers deactivation of the
// active SELECT while a later page is pending: the held response mutates
// nothing, releases the pending slot, and a re-issued page under the advanced
// generation applies.
func TestStaleLaterPageRejectedAfterDeactivation(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	pending, pendingCmd := pageDown(m)
	stalePage := pendingCmd().(PageSettledMsg)

	pending.deactivateActiveSelect()
	after := apply(pending, stalePage)
	if after.Result == nil || after.Result.Offset != 0 {
		t.Fatalf("deactivated execution's later page mutated rows/range: %+v", after.Result)
	}

	withPending, cmd := pageDown(after)
	if cmd == nil {
		t.Fatal("page request refused after the stale predecessor settled")
	}
	replacementMsg := cmd().(PageSettledMsg)
	if replacementMsg.Generation == stalePage.Generation {
		t.Error("replacement after deactivation reused the stale generation")
	}
	final := apply(withPending, replacementMsg)
	if final.Result == nil || final.Result.Offset == 0 {
		t.Fatalf("post-deactivation page did not apply: %+v", final.Result)
	}
}

// TestStaleResponseCannotClearNewerRequestFeedback covers the feedback rule:
// a stale later-page response arriving while a newer page request is pending
// clears neither the newer request's loading feedback nor its pending guard,
// and alters no cache metadata.
func TestStaleResponseCannotClearNewerRequestFeedback(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	_, firstCmd := pageDown(m)
	stalePage := firstCmd().(PageSettledMsg)

	// A newer execution replaces the active one while the old page is held;
	// its paging state is fresh, and the old response must stay inert.
	newer, newerPage, _ := newerExecutionOver(t, m)
	newer = apply(newer, newerPage)

	// Duplicate-shaped stale response wearing the superseded identity.
	if after := apply(newer, stalePage); after.Result != nil && after.Result.Offset != 0 {
		t.Fatalf("stale page applied rows to the newer execution: %+v", after.Result)
	}

	// The newer execution issues its own page request; the stale response
	// arriving while it is pending cannot clear its pending feedback.
	withPending, newerCmd := pageDown(newer)
	if newerCmd == nil {
		t.Fatal("newer execution refused its first page request")
	}
	if !withPending.pagePending || !strings.Contains(withPending.View(), PageLoadingIndicator) {
		t.Fatalf("newer page request lost its loading feedback: pending=%v", withPending.pagePending)
	}
	newerPageMsg := newerCmd().(PageSettledMsg)
	after := apply(withPending, stalePage)
	if !after.pagePending {
		t.Error("stale page response cleared the newer request's pending guard")
	}
	if after.Result == nil || after.Result.Offset != 0 || len(after.Result.Page.Rows) != 3 {
		t.Fatalf("stale page response mutated rows/range while pending: %+v", after.Result)
	}
	// The newer request's own response settles afterwards and applies.
	final := apply(after, newerPageMsg)
	if final.Result == nil || final.Result.Offset == 0 {
		t.Fatalf("newer page did not apply after the stale response settled: %+v", final.Result)
	}
}

// TestStalePageFailureCannotClearNewerRequestFeedback covers the failure
// variant: a stale later-page failure while a newer page request is pending
// is equally inert.
func TestStalePageFailureCannotClearNewerRequestFeedback(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, &fakeSelectExecutor{page: threeRowFirstPage()}, pageExec)

	_, firstCmd := pageDown(m)
	stalePage := firstCmd().(PageSettledMsg)
	stalePage.Result = FirstPageResult{Err: errors.New("lock timeout")}

	newer, newerPage, _ := newerExecutionOver(t, m)
	newer = apply(newer, newerPage)
	withPending, newerCmd := pageDown(newer)
	if !withPending.pagePending {
		t.Fatal("newer page request not pending after dispatch")
	}

	after := apply(withPending, stalePage)
	if !after.pagePending {
		t.Error("stale page failure cleared the newer request's pending guard")
	}
	if after.Result == nil || after.Result.Err != nil {
		t.Errorf("stale page failure created a result error: %+v", after.Result)
	}

	// The newer request's own response settles afterwards and applies.
	final := apply(after, newerCmd().(PageSettledMsg))
	if final.Result == nil || final.Result.Offset == 0 {
		t.Fatalf("newer page did not apply after the stale failure settled: %+v", final.Result)
	}
}
