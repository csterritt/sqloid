// Scripted Bubble Tea coverage for scoped Ctrl+W cancellation of active
// SELECT work (Issue #28 Task 1), per the Global Key Precedence and Errors
// and cancellation bounds sections of Notes/PRD-sqloid.md. Barrier-controlled
// fake executors hold each in-flight first-page, later-page, and count
// request; Ctrl+W must request one scoped cancellation for each currently
// active request through independent per-request identities, render the exact
// `cancelling…` feedback until every targeted request settles, dispatch no
// replacement SELECT/page/count work before that settlement, reject
// cancellation-classified late results inertly through Issue #26's guards,
// and accept healthy replacement work afterwards. Cancellation is idempotent
// and never touches inactive or unrelated work.

package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
)

// twoVersionSelectModel wires a runnable SELECT model whose validation seam
// serves two successful version reads, so a second execution can open
// validation after the first one settles.
func twoVersionSelectModel() Model {
	return validationModel(whereUICatalog(), &fakeVersionReader{
		queued: []schema.VersionAttempt{versionOK(17), versionOK(17)},
	}, &fakeRefresher{}, validSelectQB())
}

// blockingSelectExecutor holds its request behind two deterministic barriers:
// the first signals that the command ran and hands the caller the request's
// own context (its cancellation identity); the work stays blocked until ctx
// is done — mirroring an interrupted in-flight query — and settlement is
// held back until the test releases the hold channel.
type blockingSelectExecutor struct {
	started chan context.Context
	hold    chan struct{}
	result  FirstPageResult
	calls   int
}

func newBlockingSelect(result FirstPageResult) *blockingSelectExecutor {
	return &blockingSelectExecutor{started: make(chan context.Context, 8), hold: make(chan struct{}), result: result}
}

func (f *blockingSelectExecutor) selectPage(ctx context.Context, sql string, params []any) FirstPageResult {
	f.calls++
	f.started <- ctx
	<-ctx.Done() // the request runs until its cancellation identity fires
	<-f.hold     // settlement stays behind the test barrier
	return f.result
}

// blockingCountExecutor mirrors blockingSelectExecutor for the count role.
type blockingCountExecutor struct {
	started chan context.Context
	hold    chan struct{}
	result  CountResult
	calls   int
}

func newBlockingCount(result CountResult) *blockingCountExecutor {
	return &blockingCountExecutor{started: make(chan context.Context, 8), hold: make(chan struct{}), result: result}
}

func (f *blockingCountExecutor) count(ctx context.Context, sql string, params []any) CountResult {
	f.calls++
	f.started <- ctx
	<-ctx.Done()
	<-f.hold
	return f.result
}

// blockingPageExecutor mirrors them for the later-page role.
type blockingPageExecutor struct {
	started chan context.Context
	hold    chan struct{}
	result  FirstPageResult
	calls   int
}

func newBlockingPage(result FirstPageResult) *blockingPageExecutor {
	return &blockingPageExecutor{started: make(chan context.Context, 8), hold: make(chan struct{}), result: result}
}

func (f *blockingPageExecutor) page(ctx context.Context, sql string, params []any) FirstPageResult {
	f.calls++
	f.started <- ctx
	<-ctx.Done()
	<-f.hold
	return f.result
}

// runCmd executes one command in its own goroutine so a blocking fake can be
// held behind its barriers without deadlocking the test.
func runCmd(cmd tea.Cmd) <-chan tea.Msg {
	msgs := make(chan tea.Msg, 1)
	go func() { msgs <- cmd() }()
	return msgs
}

// ctrlW presses Ctrl+W and applies the dispatched cancellation message,
// returning the model after the scoped cancellation has been requested.
func ctrlW(m Model) Model {
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if cmd == nil {
		return next
	}
	msg := cmd()
	after, _ := next.Update(msg)
	return after.(Model)
}

// runBatchAsync unwraps one execution-start batch and dispatches both
// commands in their own goroutines, so blocking page/count fakes run
// concurrently while the test holds them behind their barriers.
func runBatchAsync(t *testing.T, cmd tea.Cmd) (chan tea.Msg, chan tea.Msg) {
	t.Helper()
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("execution start produced %T, want a page/count batch", msg)
	}
	pageMsgs := make(chan tea.Msg, 1)
	countMsgs := make(chan tea.Msg, 1)
	for _, c := range batch {
		if c == nil {
			continue
		}
		probe := c
		go func() {
			m := probe()
			switch m.(type) {
			case SelectSettledMsg:
				pageMsgs <- m
			case CountSettledMsg:
				countMsgs <- m
			default:
				pageMsgs <- m
			}
		}()
	}
	return pageMsgs, countMsgs
}

// applyMsgs drains one settled-message channel into the model.
func applyMsg(m Model, msgs <-chan tea.Msg) Model {
	next, _ := m.Update(<-msgs)
	return next.(Model)
}

// TestCtrlWCancelsFirstPageAndCountUntilAllSettle covers the concurrent
// first-page/count scope: Ctrl+W requests one independent cancellation for
// each active request, `cancelling…` stays visible until both settle, no
// replacement execution or page work dispatches before settlement, the
// cancellation-classified late page is inert, and the settled gate accepts a
// healthy replacement execution.
func TestCtrlWCancelsFirstPageAndCountUntilAllSettle(t *testing.T) {
	pageExec := newBlockingSelect(FirstPageResult{Cancelled: true})
	countExec := newBlockingCount(CountResult{Cancelled: true})
	m := twoVersionSelectModel()
	m.Select = pageExec.selectPage
	m.Count = countExec.count

	execModel, execCmd := driveToExecutionStart(t, m)
	pageMsgs, countMsgs := runBatchAsync(t, execCmd)
	pageCtx := <-pageExec.started
	countCtx := <-countExec.started
	if pageCtx == countCtx {
		t.Fatal("first-page and count requests shared one cancellation identity")
	}

	// Scope: both concurrently active requests are cancelled by one Ctrl+W.
	m = ctrlW(execModel)
	if !m.selectCancelling {
		t.Fatal("Ctrl+W did not enter the cancelling state")
	}
	if err := pageCtx.Err(); err == nil {
		t.Fatal("first-page request context not cancelled")
	}
	if err := countCtx.Err(); err == nil {
		t.Fatal("count request context not cancelled")
	}

	// `cancelling…` remains visible while both requests are unsettled.
	if got := m.View(); !containsStatus(got, SelectCancellingIndicator) {
		t.Fatalf("view before settlement = %q, want the cancelling indicator", got)
	}
	if m.inFlightEnterFeedback() == "" {
		t.Error("Enter feedback lost while cancelling")
	}

	// No replacement SELECT or page work dispatches before settlement:
	// Enter is consumed, Page Down issues nothing, and no executor is re-hit.
	blocked, enterCmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if enterCmd != nil {
		t.Fatal("replacement execution dispatched before settlement")
	}
	if blocked.inFlightNotice == "" {
		t.Error("Enter consumed without the in-flight explanation")
	}
	if pageExec.calls != 1 || countExec.calls != 1 {
		t.Fatalf("executor calls during cancelling = %d/%d, want 1/1", pageExec.calls, countExec.calls)
	}
	if _, cmd := pageDown(m); cmd != nil {
		t.Fatal("replacement page request dispatched before settlement")
	}

	// Idempotency: pressing Ctrl+W again while cancelling changes nothing.
	again := ctrlW(m)
	if pageExec.calls != 1 || countExec.calls != 1 {
		t.Fatalf("executor calls after repeat Ctrl+W = %d/%d, want 1/1", pageExec.calls, countExec.calls)
	}
	if !again.selectCancelling {
		t.Error("repeat Ctrl+W dropped the cancelling state early")
	}
	m = again

	// The count settles first: its role is inert, and `cancelling…` persists
	// because the first page is still unsettled.
	close(countExec.hold) // release the count settlement barrier
	m = applyMsg(m, countMsgs)
	if m.countPendingFlag {
		t.Error("cancelled count did not settle its pending slot")
	}
	if !m.selectCancelling {
		t.Fatal("cancelling feedback cleared before every targeted request settled")
	}
	if got := countStateHeader(m); got != "Counting rows…" {
		t.Errorf("cancelled count header = %q, want the pending wording", got)
	}

	// The page settles last: classified cancelled, it never mutates rows.
	close(pageExec.hold)
	m = applyMsg(m, pageMsgs)
	if m.selectCancelling || m.firstPagePending {
		t.Fatal("gate not settled after every targeted request settled")
	}
	if m.Result != nil {
		t.Fatalf("cancelled late page stored rows: %+v", m.Result)
	}

	// After all-request settlement, a healthy replacement execution is
	// accepted: swap in normal executors and run the second execution —
	// through validation — to completion.
	healthy := &fakeSelectExecutor{page: threeRowFirstPage()}
	m.Select = healthy.selectPage
	m.Count = (&fakeCountExecutor{}).count
	next, execCmd2 := driveToExecutionStart(t, m)
	if execCmd2 == nil {
		t.Fatal("replacement execution refused after settlement")
	}
	final := settleFirstPage(t, next, execCmd2)
	if healthy.calls != 1 {
		t.Fatalf("replacement executor calls = %d, want 1", healthy.calls)
	}
	if final.Result == nil || len(final.Result.Page.Rows) != 3 {
		t.Fatalf("healthy replacement execution did not apply: %+v", final.Result)
	}
}

// TestCtrlWCancelsCountOnlyScope covers the count-only scope: with the first
// page settled, Ctrl+W cancels exactly the active count request and nothing
// else, and the inert cancelled count leaves the pending presentation and
// displayed rows untouched.
func TestCtrlWCancelsCountOnlyScope(t *testing.T) {
	countExec := newBlockingCount(CountResult{Cancelled: true})
	m := twoVersionSelectModel()
	m.Select = (&fakeSelectExecutor{page: threeRowFirstPage()}).selectPage
	m.Count = countExec.count

	execModel, execCmd := driveToExecutionStart(t, m)
	pageMsgs, countMsgs := runBatchAsync(t, execCmd)
	countCtx := <-countExec.started

	// The first page is already settled: the active scope is count-only.
	afterPage := applyMsg(execModel, pageMsgs)
	if afterPage.firstPagePending || !afterPage.countPendingFlag {
		t.Fatalf("setup: want count-only pending, firstPage=%v count=%v", afterPage.firstPagePending, afterPage.countPendingFlag)
	}

	m2 := ctrlW(afterPage)
	if err := countCtx.Err(); err == nil {
		t.Fatal("active count request context not cancelled by Ctrl+W")
	}
	if !m2.selectCancelling {
		t.Fatal("cancelling state not entered for count-only scope")
	}

	// No replacement count or page work dispatches while unsettled.
	if _, cmd := pageDown(m2); cmd != nil {
		t.Fatal("page request dispatched while a cancelled request is unsettled")
	}

	close(countExec.hold)
	m2 = applyMsg(m2, countMsgs)
	if m2.selectCancelling {
		t.Fatal("cancelling feedback not cleared after the count settled")
	}
	if got := countStateHeader(m2); got != "Counting rows…" {
		t.Errorf("cancelled count header = %q, want the pending wording", got)
	}
	if m2.Result == nil || len(m2.Result.Page.Rows) != 3 {
		t.Fatalf("count-only cancellation disturbed rows: %+v", m2.Result)
	}
}

// TestCtrlWCancelsLaterPageOnlyScope covers the later-page-only scope: with
// first page and count settled, Ctrl+W cancels exactly the one active later-
// page request, `cancelling…` holds until it settles, no replacement page
// dispatches before settlement, and the cancelled page's rows, range, and
// cache stay unchanged.
func TestCtrlWCancelsLaterPageOnlyScope(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowFirstPage()}
	m := firstSelectModel(exec)
	m.Page = (&fakePageExecutor{rowsShown: 3}).page
	base := settledFirstPage(t, exec, &fakePageExecutor{rowsShown: 3})

	pageExec := newBlockingPage(FirstPageResult{Cancelled: true})
	base.Page = pageExec.page

	next, cmd := pageDown(base)
	if cmd == nil {
		t.Fatal("page down issued no request")
	}
	pageMsgs := runCmd(cmd)
	pageCtx := <-pageExec.started

	m2 := ctrlW(next)
	if err := pageCtx.Err(); err == nil {
		t.Fatal("later-page request context not cancelled by Ctrl+W")
	}
	if !m2.selectCancelling {
		t.Fatal("cancelling state not entered for later-page scope")
	}
	if got := m2.View(); !containsStatus(got, SelectCancellingIndicator) {
		t.Fatalf("view while later page cancels = %q, want the cancelling indicator", got)
	}

	// No replacement page dispatches before settlement.
	if _, cmd := pageDown(m2); cmd != nil {
		t.Fatal("replacement page request dispatched before settlement")
	}

	close(pageExec.hold)
	m2 = applyMsg(m2, pageMsgs)
	if m2.selectCancelling || m2.pagePending {
		t.Fatal("gate not settled after the later page settled")
	}
	if m2.pageOffset != base.pageOffset || m2.pageExhausted != base.pageExhausted {
		t.Error("cancelled later page altered cache metadata")
	}
	if m2.Result == nil || m2.Result.Offset != 0 {
		t.Fatalf("cancelled later page mutated rows/range: %+v", m2.Result)
	}

	// Settlement reopens the gate: a healthy replacement page applies.
	healthy := &fakePageExecutor{rowsShown: 3}
	m2.Page = healthy.page
	final, cmd2 := pageDown(m2)
	if cmd2 == nil {
		t.Fatal("replacement page request refused after settlement")
	}
	applied := settlePage(t, final, cmd2)
	if applied.Result == nil || applied.Result.Offset == 0 {
		t.Fatalf("post-cancellation replacement page did not apply: %+v", applied.Result)
	}
}

// containsStatus reports whether the rendered view carries the exact status
// text; status rendering keeps content understandable without color.
func containsStatus(view, status string) bool {
	return len(view) > 0 && indexOf(view, status) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
