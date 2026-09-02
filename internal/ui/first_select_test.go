// Scripted Bubble Tea coverage for the first production SELECT path (Issue
// #22 Tasks 3–4): runnable Enter completes Issue #21 validation first, then
// one actual SELECT execution starts; query history appends exactly at that
// actual-execution boundary with consecutive-identical suppression; the
// wired executor seam receives exactly the builder's SQL and ordered
// parameters; typed rows cross into internal/result without coercion; and
// execution failures follow the ordinary result-error boundary. A
// deterministic fake executor stands in for the Connection boundary so no
// database access runs here.

package ui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
	"github.com/chris/sqloid/internal/schema"
)

// fakeSelectExecutor records every first-page request and returns a queued
// outcome, so tests can prove exactly one execution with exact inputs.
type fakeSelectExecutor struct {
	sqls   []string
	params [][]any
	calls  int
	page   *result.Page
	err    error
	// byteTruncated and limitFailure carry Issue #31/#72 settlement
	// metadata through the real FirstPageResult path so tests never
	// assign ResultView fields directly.
	byteTruncated bool
	limitFailure  *result.LimitFailure
}

func (f *fakeSelectExecutor) selectPage(ctx context.Context, sql string, params []any) FirstPageResult {
	f.calls++
	f.sqls = append(f.sqls, sql)
	f.params = append(f.params, params)
	if f.err != nil {
		return FirstPageResult{Err: f.err}
	}
	return FirstPageResult{
		Page:          f.page,
		ByteTruncated: f.byteTruncated,
		LimitFailure:  f.limitFailure,
	}
}

// firstSelectModel wires a runnable SELECT model with the validation fakes
// and the given executor, plus a fresh history store.
func firstSelectModel(exec *fakeSelectExecutor) Model {
	m := selectModel(&fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}, &fakeRefresher{})
	m.Select = exec.selectPage
	return m
}

// driveToExecutionStart presses Enter on runnable data, opens validation,
// settles the unchanged version, and applies the resulting
// execution-start lifecycle message, returning the model after the
// ExecutionStartedMsg update plus the executor command it returned.
func driveToExecutionStart(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()

	opened, cmd := enterRunnable(m)
	if cmd == nil {
		t.Fatal("validation issued no version-read command")
	}
	settled, nextCmd := opened.Update(cmd())
	opened, ok := settled.(Model)
	if !ok {
		t.Fatalf("validation settle returned %T", settled)
	}
	if nextCmd == nil {
		t.Fatal("successful validation issued no execution-start command")
	}
	started := nextCmd()
	startMsg, ok := started.(ExecutionStartedMsg)
	if !ok {
		t.Fatalf("execution route produced %T, want ExecutionStartedMsg", started)
	}
	if opened.validating {
		t.Fatal("validation workflow still open when the start message arrived")
	}
	afterStart, execCmd := opened.Update(startMsg)
	execModel, ok := afterStart.(Model)
	if !ok {
		t.Fatalf("execution-start update returned %T", afterStart)
	}
	return execModel, execCmd
}

func TestFirstSelectRunsOnePageAfterValidation(t *testing.T) {
	exec := &fakeSelectExecutor{
		page: &result.Page{Columns: []string{"id", "email"}, Rows: [][]result.Value{{
			result.NewInteger(1), result.NewText("a@b"),
		}}},
	}
	m := firstSelectModel(exec)

	execModel, execCmd := driveToExecutionStart(t, m)
	if execModel.validating {
		t.Fatal("validation workflow still open when the start message was handled")
	}
	// History appended exactly at the actual-execution boundary.
	if execModel.History.Len() != 1 {
		t.Fatalf("history length = %d at execution start, want 1", execModel.History.Len())
	}
	if exec.calls != 0 {
		t.Fatalf("executor ran %d times before its command was invoked, want 0", exec.calls)
	}
	if execCmd == nil {
		t.Fatal("execution start produced no executor command")
	}

	settled := execCmd()
	msg, ok := settled.(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", settled)
	}
	if msg.Result.Page == nil || msg.Result.Err != nil {
		t.Fatalf("settled result = %+v, want successful page", msg.Result)
	}
	final := asModel(execModel.Update(msg))
	if exec.calls != 1 {
		t.Errorf("executor ran %d times, want exactly 1", exec.calls)
	}
	wantSQL := `SELECT * FROM "users" WHERE "email" = ?`
	if len(exec.sqls) != 1 || exec.sqls[0] != wantSQL {
		t.Errorf("executor SQL = %q, want exactly %q", exec.sqls, wantSQL)
	}
	if len(exec.params) != 1 || len(exec.params[0]) != 1 || exec.params[0][0] != "x" {
		t.Errorf("executor params = %v, want [x]", exec.params)
	}
	// Typed rows crossed into the shared representation without coercion.
	if final.Result == nil || final.Result.Page == nil {
		t.Fatal("settled success stored no result")
	}
	cell := final.Result.Page.Rows[0][0]
	if cell.Kind != result.KindInteger || cell.Int != 1 {
		t.Errorf("stored cell = %+v, want typed Integer 1", cell)
	}
}

func TestFirstSelectHistorySuppressionAtBoundary(t *testing.T) {
	exec := &fakeSelectExecutor{}
	m := firstSelectModel(exec)

	execModel, execCmd := driveToExecutionStart(t, m)
	if execModel.History.Len() != 1 {
		t.Fatalf("history length = %d, want 1", execModel.History.Len())
	}
	// A second consecutive identical start (same builder state) suppresses
	// its append while still issuing its own execution.
	startCmd := execModel.executionRoute()
	if startCmd == nil {
		t.Fatal("execution route refused a second identical start")
	}
	startMsg, ok := startCmd().(ExecutionStartedMsg)
	if !ok {
		t.Fatalf("second route produced %T, want ExecutionStartedMsg", startCmd())
	}
	afterSecond, secondCmd := execModel.Update(startMsg)
	second, ok := afterSecond.(Model)
	if !ok {
		t.Fatalf("second start update returned %T", afterSecond)
	}
	if second.History.Len() != 1 {
		t.Errorf("history length = %d after identical re-execution, want still 1", second.History.Len())
	}
	if exec.calls != 0 {
		t.Errorf("executor ran before its command was invoked: %d", exec.calls)
	}
	if secondCmd == nil {
		t.Fatal("second identical start produced no executor command")
	}
	if execCmd == nil {
		t.Fatal("first execution start produced no executor command")
	}
}

func TestFirstSelectErrorFollowsOrdinaryBoundary(t *testing.T) {
	exec := &fakeSelectExecutor{err: errors.New("no such table: users")}
	m := firstSelectModel(exec)

	execModel, execCmd := driveToExecutionStart(t, m)
	if execModel.History.Len() != 1 {
		t.Fatalf("history length = %d at execution start, want 1 (append not undone by failure)", execModel.History.Len())
	}
	settled := execCmd()
	final := asModel(execModel.Update(settled))
	if final.Result == nil || final.Result.Err == nil {
		t.Fatal("ordinary failure not routed to the result-error boundary")
	}
	if !strings.Contains(final.Result.Err.Error(), "no such table") {
		t.Errorf("error text = %q, want the driver cause", final.Result.Err.Error())
	}
}

func TestFailedValidationRunsNoExecutionOrHistory(t *testing.T) {
	exec := &fakeSelectExecutor{}
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionFailed("disk I/O error")}}
	m := selectModel(reader, &fakeRefresher{})
	m.Select = exec.selectPage
	m.History = history.NewStore()

	opened, cmd := enterRunnable(m)
	settled, _ := opened.Update(cmd())
	opened, ok := settled.(Model)
	if !ok {
		t.Fatalf("validation settle returned %T", settled)
	}
	if opened.History.Len() != 0 {
		t.Errorf("failed validation appended history: %d entries", opened.History.Len())
	}
	if exec.calls != 0 {
		t.Errorf("failed validation ran the executor %d times", exec.calls)
	}
}

// settleFirstPageResult drives a first-page executor through validation and
// execution start, then applies the settled SelectSettledMsg, returning the
// final model. It follows the real identity/update path — no direct
// ResultView mutation.
func settleFirstPageResult(t *testing.T, exec *fakeSelectExecutor) Model {
	t.Helper()
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	settled := execCmd()
	return asModel(execModel.Update(settled))
}

// TestFirstSelectRetainsByteTruncatedFromResult requires accepted first-page
// settlement to copy FirstPageResult.ByteTruncated into ResultView and render
// the exact shared 64 MiB warning (Issue #72 AC1).
func TestFirstSelectRetainsByteTruncatedFromResult(t *testing.T) {
	exec := &fakeSelectExecutor{
		page:          threeRowPage(),
		byteTruncated: true,
	}
	m := settleFirstPageResult(t, exec)
	if m.Result == nil || !m.Result.ByteTruncated {
		t.Fatal("settled first page did not retain ByteTruncated from FirstPageResult")
	}
	if got := m.View(); !strings.Contains(got, result.ByteCapWarning) {
		t.Fatalf("view missing the shared byte-cap warning:\n%s", got)
	}
}

// TestFirstSelectRetainsValueLimitFailure requires accepted first-page
// settlement to copy a typed KindValue LimitFailure at a known one-based
// position into ResultView and render the exact shared row-N message
// (Issue #72 AC2).
func TestFirstSelectRetainsValueLimitFailure(t *testing.T) {
	exec := &fakeSelectExecutor{
		page: threeRowPage(),
		limitFailure: &result.LimitFailure{
			Kind:     result.KindValue,
			Position: 7,
		},
	}
	m := settleFirstPageResult(t, exec)
	if m.Result == nil || m.Result.LimitFailure == nil {
		t.Fatal("settled first page did not retain the LimitFailure")
	}
	if m.Result.LimitFailure.Kind != result.KindValue {
		t.Errorf("retained kind = %v, want KindValue", m.Result.LimitFailure.Kind)
	}
	if m.Result.LimitFailure.Position != 7 {
		t.Errorf("retained position = %d, want 7", m.Result.LimitFailure.Position)
	}
	want := "result value exceeds the 64 MiB v1 limit at row 7"
	if got := collapseWhitespace(m.View()); !strings.Contains(got, want) {
		t.Fatalf("view missing exact value-limit message %q:\n%s", want, m.View())
	}
}

// TestFirstSelectRetainsPageLimitFailure requires accepted first-page
// settlement to copy a typed KindPage LimitFailure at a known one-based
// position into ResultView and render the exact shared row-N message
// (Issue #72 AC2).
func TestFirstSelectRetainsPageLimitFailure(t *testing.T) {
	exec := &fakeSelectExecutor{
		page: threeRowPage(),
		limitFailure: &result.LimitFailure{
			Kind:     result.KindPage,
			Position: 12,
		},
	}
	m := settleFirstPageResult(t, exec)
	if m.Result == nil || m.Result.LimitFailure == nil {
		t.Fatal("settled first page did not retain the LimitFailure")
	}
	if m.Result.LimitFailure.Kind != result.KindPage {
		t.Errorf("retained kind = %v, want KindPage", m.Result.LimitFailure.Kind)
	}
	if m.Result.LimitFailure.Position != 12 {
		t.Errorf("retained position = %d, want 12", m.Result.LimitFailure.Position)
	}
	want := "result page exceeds the 64 MiB v1 limit at row 12"
	if got := collapseWhitespace(m.View()); !strings.Contains(got, want) {
		t.Fatalf("view missing exact page-limit message %q:\n%s", want, m.View())
	}
}

// TestFirstSelectCacheDerivedByteTruncation requires that when the viewport
// cache already records byte-cap truncation, accepted first-page settlement
// ORs the cache-derived fact into ResultView.ByteTruncated even when the
// incoming FirstPageResult.ByteTruncated flag is false (Issue #72 AC1 —
// cache-derived truncation cannot be lost).
func TestFirstSelectCacheDerivedByteTruncation(t *testing.T) {
	exec := &fakeSelectExecutor{
		page:          threeRowPage(),
		byteTruncated: false,
	}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Pre-seed the fresh viewport cache with content that triggers byte-cap
	// eviction, simulating a cache state where TruncatedByByteCap is already
	// true when the first page settles. Three 22 MiB rows exceed the 64 MiB
	// envelope, so evict drops the low end and sets the persistent disclosure.
	third := int(resultcache.MaxPayloadBytes/3) + 1
	c := resultcache.New()
	for i := int64(1); i <= 3; i++ {
		page := resultcache.Page{
			Start: resultcache.Position(i),
			Rows: []resultcache.Row{{
				Position: resultcache.Position(i),
				Values:   []result.Value{result.NewBlob(make([]byte, third))},
			}},
		}
		if accepted, _ := c.Merge(page, resultcache.Forward); !accepted {
			t.Fatalf("pre-seed merge %d not accepted", i)
		}
	}
	if !c.TruncatedByByteCap() {
		t.Fatal("pre-seeded cache did not record byte-cap truncation")
	}
	execModel.viewportCache = c

	settled := execCmd()
	final := asModel(execModel.Update(settled))
	if !final.viewportCache.TruncatedByByteCap() {
		t.Fatal("viewport cache lost byte-cap truncation after first-page settlement")
	}
	if final.Result == nil || !final.Result.ByteTruncated {
		t.Fatal("settled first page did not OR cache-derived byte truncation into ResultView")
	}
	if got := final.View(); !strings.Contains(got, result.ByteCapWarning) {
		t.Fatalf("view missing the shared byte-cap warning after cache-derived truncation:\n%s", got)
	}
}

// TestFirstSelectCancelledSettlementInert requires that a cancelled
// first-page settlement carrying ByteTruncated and LimitFailure never
// mutates ResultView — cancellation is fully inert at the response boundary
// (Issue #72).
func TestFirstSelectCancelledSettlementInert(t *testing.T) {
	exec := &fakeSelectExecutor{
		page:          threeRowPage(),
		byteTruncated: true,
		limitFailure:  &result.LimitFailure{Kind: result.KindValue, Position: 3},
	}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Replace the settled message's result with a cancelled outcome that
	// still carries metadata, proving the metadata cannot leak through.
	settled := execCmd()
	msg, ok := settled.(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", settled)
	}
	msg.Result = FirstPageResult{
		Cancelled:     true,
		ByteTruncated: true,
		LimitFailure:  &result.LimitFailure{Kind: result.KindValue, Position: 3},
	}
	final := asModel(execModel.Update(msg))
	if final.Result != nil {
		t.Fatalf("cancelled settlement mutated ResultView: %+v", final.Result)
	}
}

// TestFirstSelectStaleIdentitySettlementInert requires that a stale-identity
// first-page settlement carrying ByteTruncated and LimitFailure never
// mutates ResultView (Issue #72).
func TestFirstSelectStaleIdentitySettlementInert(t *testing.T) {
	exec := &fakeSelectExecutor{
		page:          threeRowPage(),
		byteTruncated: true,
		limitFailure:  &result.LimitFailure{Kind: result.KindPage, Position: 5},
	}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Drive a real settlement first to establish a ResultView.
	realSettled := execCmd()
	realMsg, ok := realSettled.(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", realSettled)
	}
	settled := asModel(execModel.Update(realMsg))
	if settled.Result == nil {
		t.Fatal("real settlement produced no ResultView")
	}
	wantByte := settled.Result.ByteTruncated
	wantLF := settled.Result.LimitFailure

	// A second settlement with the same request identity is stale: the
	// tracker already consumed that role. It must not mutate ResultView,
	// even if it carries different metadata.
	staleMsg := realMsg
	staleMsg.Result = FirstPageResult{
		Page:          threeRowPage(),
		ByteTruncated: false,
		LimitFailure:  &result.LimitFailure{Kind: result.KindValue, Position: 99},
	}
	final := asModel(settled.Update(staleMsg))
	if final.Result == nil {
		t.Fatal("stale settlement cleared ResultView")
	}
	if final.Result.ByteTruncated != wantByte {
		t.Errorf("stale settlement changed ByteTruncated: got %v, want %v", final.Result.ByteTruncated, wantByte)
	}
	if final.Result.LimitFailure != wantLF {
		t.Errorf("stale settlement changed LimitFailure: got %+v, want %+v", final.Result.LimitFailure, wantLF)
	}
}

// TestFirstSelectFreshExecutionReplacesMetadata requires that starting a
// fresh execution after a metadata-carrying first page replaces the old
// ResultView outright, including its ByteTruncated and LimitFailure
// (Issue #72).
func TestFirstSelectFreshExecutionReplacesMetadata(t *testing.T) {
	exec := &fakeSelectExecutor{
		page:          threeRowPage(),
		byteTruncated: true,
		limitFailure:  &result.LimitFailure{Kind: result.KindValue, Position: 4},
	}
	m := settleFirstPageResult(t, exec)
	if m.Result == nil || !m.Result.ByteTruncated || m.Result.LimitFailure == nil {
		t.Fatal("initial settlement did not retain metadata")
	}

	// Start a fresh execution with no metadata and settle it.
	exec.byteTruncated = false
	exec.limitFailure = nil
	exec.calls = 0
	startCmd := m.executionRoute()
	if startCmd == nil {
		t.Fatal("fresh execution route refused to start")
	}
	startMsg, ok := startCmd().(ExecutionStartedMsg)
	if !ok {
		t.Fatalf("fresh route produced %T, want ExecutionStartedMsg", startCmd())
	}
	afterStart, execCmd := m.Update(startMsg)
	started := afterStart.(Model)
	if execCmd == nil {
		t.Fatal("fresh execution start produced no executor command")
	}
	settled := execCmd()
	final := asModel(started.Update(settled))
	if final.Result == nil {
		t.Fatal("fresh execution produced no ResultView")
	}
	if final.Result.ByteTruncated {
		t.Error("fresh execution retained stale ByteTruncated from the previous execution")
	}
	if final.Result.LimitFailure != nil {
		t.Errorf("fresh execution retained stale LimitFailure: %+v", final.Result.LimitFailure)
	}
}

// TestFirstSelectRetainsByteTruncatedAndLimitFailureTogether requires that
// accepted first-page settlement retains both ByteTruncated and
// LimitFailure simultaneously and renders both the 64 MiB warning and the
// exact row-N diagnostic (Issue #72 AC1–AC2).
func TestFirstSelectRetainsByteTruncatedAndLimitFailureTogether(t *testing.T) {
	exec := &fakeSelectExecutor{
		page: threeRowPage(),
		limitFailure: &result.LimitFailure{
			Kind:     result.KindPage,
			Position: 9,
		},
		byteTruncated: true,
	}
	m := settleFirstPageResult(t, exec)
	if m.Result == nil || !m.Result.ByteTruncated || m.Result.LimitFailure == nil {
		t.Fatalf("settled first page lost metadata: %+v", m.Result)
	}
	view := collapseWhitespace(m.View())
	if !strings.Contains(view, result.ByteCapWarning) {
		t.Fatalf("view missing the shared byte-cap warning:\n%s", m.View())
	}
	wantRow := "result page exceeds the 64 MiB v1 limit at row " + strconv.FormatInt(9, 10)
	if !strings.Contains(view, wantRow) {
		t.Fatalf("view missing exact page-limit message %q:\n%s", wantRow, m.View())
	}
}

// firstPageRows returns a successful FirstPageResult carrying a page with the
// given number of integer rows, suitable for first-page endpoint tests.
func firstPageRows(n int) *result.Page {
	rows := make([][]result.Value, n)
	for i := range rows {
		rows[i] = []result.Value{result.NewInteger(int64(i + 1))}
	}
	return &result.Page{Columns: []string{"id"}, Rows: rows}
}

// defaultPageRows is the exact count of complete visible data rows at the
// default 80x24 test layout (Issue #25 layout arithmetic).
const defaultPageRows = 11

// settleFirstPageWithCount drives a first-page execution with a count
// executor wired, settles both the page and count completions, and returns
// the idle model. The count executor's err controls whether the count
// settles as unavailable.
func settleFirstPageWithCount(t *testing.T, page *result.Page, countErr error) Model {
	t.Helper()
	exec := &fakeSelectExecutor{page: page}
	count := &fakeCountExecutor{err: countErr}
	m := firstSelectModel(exec)
	m.Count = count.count
	execModel, execCmd := driveToExecutionStart(t, m)
	return settleFirstPage(t, execModel, execCmd)
}

// TestFirstSelectRetainsRequestedFirstPageSize proves the layout-derived
// requested first-page size is retained at dispatch and bound to the
// request identity: an accepted page with exactly that many rows does not
// set pageExhausted, while a shorter page does (Issue #73 AC1, AC4).
func TestFirstSelectRetainsRequestedFirstPageSize(t *testing.T) {
	cases := []struct {
		name        string
		rows        int
		wantExhaust bool
	}{
		{"exactly full does not exhaust", defaultPageRows, false},
		{"short exhausts", defaultPageRows - 1, true},
		{"one row exhausts", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := settleFirstPageResult(t, &fakeSelectExecutor{page: firstPageRows(tc.rows)})
			if m.pageExhausted != tc.wantExhaust {
				t.Errorf("pageExhausted = %v, want %v (rows=%d, requestedSize=%d)",
					m.pageExhausted, tc.wantExhaust, tc.rows, defaultPageRows)
			}
		})
	}
}

// TestFirstSelectEmptyPageSetsPageExhausted proves an accepted zero-row
// first page sets pageExhausted so the high endpoint is established
// (Issue #73 AC1, AC2).
func TestFirstSelectEmptyPageSetsPageExhausted(t *testing.T) {
	m := settleFirstPageResult(t, &fakeSelectExecutor{page: firstPageRows(0)})
	if !m.pageExhausted {
		t.Fatal("empty first page did not set pageExhausted")
	}
	if m.Result == nil || m.Result.Page == nil {
		t.Fatal("empty first page produced no ResultView")
	}
	if len(m.Result.Page.Rows) != 0 {
		t.Errorf("empty first page stored %d rows, want 0", len(m.Result.Page.Rows))
	}
}

// TestFirstSelectFullPageDoesNotSetPageExhausted proves an accepted first
// page with exactly the requested size leaves pageExhausted false — the
// remainder is unknown (Issue #73 AC4).
func TestFirstSelectFullPageDoesNotSetPageExhausted(t *testing.T) {
	m := settleFirstPageResult(t, &fakeSelectExecutor{page: firstPageRows(defaultPageRows)})
	if m.pageExhausted {
		t.Fatal("exactly-full first page set pageExhausted, want false (remainder unknown)")
	}
}

// TestFirstSelectShortPageFeedsObservedShortFinalPage proves a short
// first page sets pageExhausted which feeds ObservedShortFinalPage into
// the finalization facts (Issue #73 AC3).
func TestFirstSelectShortPageFeedsObservedShortFinalPage(t *testing.T) {
	m := settleFirstPageResult(t, &fakeSelectExecutor{page: firstPageRows(3)})
	if !m.pageExhausted {
		t.Fatal("short first page did not set pageExhausted")
	}
	final := Finalization{
		Outcome:                history.OutcomeSuccess,
		ReachedLow:             true,
		ReachedHigh:            true,
		CountWorkFinished:      true,
		PageWorkFinished:       true,
		ObservedShortFinalPage: m.pageExhausted,
	}
	_, traversal, err := m.SnapshotFacts(final)
	if err != nil {
		t.Fatalf("SnapshotFacts: %v", err)
	}
	if !traversal.ObservedShortFinalPage {
		t.Error("ObservedShortFinalPage = false, want true (pageExhausted should feed it)")
	}
}

// TestFirstSelectStaleExecutionShortSettlementInert proves a stale
// execution ID settlement carrying a short page does not set pageExhausted
// or mutate cache/result state (Issue #73 AC4).
func TestFirstSelectStaleExecutionShortSettlementInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Capture the pre-settlement state.
	wantExhaust := execModel.pageExhausted
	wantCacheLen := 0
	if execModel.viewportCache != nil {
		wantCacheLen = execModel.viewportCache.Len()
	}

	// A settlement with a wrong execution ID and short data must be inert.
	realMsg, ok := execCmd().(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", execCmd())
	}
	realMsg.ExecutionID = realMsg.ExecutionID + 999
	realMsg.Result = FirstPageResult{Page: firstPageRows(1)}
	final := asModel(execModel.Update(realMsg))
	if final.pageExhausted != wantExhaust {
		t.Errorf("stale execution settlement changed pageExhausted: got %v, want %v",
			final.pageExhausted, wantExhaust)
	}
	if final.Result != nil {
		t.Errorf("stale execution settlement stored a ResultView: %+v", final.Result)
	}
	if final.viewportCache != nil && final.viewportCache.Len() != wantCacheLen {
		t.Errorf("stale execution settlement changed cache len: got %d, want %d",
			final.viewportCache.Len(), wantCacheLen)
	}
}

// TestFirstSelectStaleRequestShortSettlementInert proves a stale request
// ID settlement (already-consumed role) carrying a short page does not set
// pageExhausted or mutate cache/result state (Issue #73 AC4).
func TestFirstSelectStaleRequestShortSettlementInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Drive a real settlement first to establish state.
	realSettled := execCmd()
	realMsg, ok := realSettled.(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", realSettled)
	}
	settled := asModel(execModel.Update(realMsg))
	if settled.Result == nil {
		t.Fatal("real settlement produced no ResultView")
	}
	wantExhaust := settled.pageExhausted
	wantCacheLen := 0
	if settled.viewportCache != nil {
		wantCacheLen = settled.viewportCache.Len()
	}

	// A second settlement with the same (already-consumed) request identity
	// carrying short data must be inert.
	staleMsg := realMsg
	staleMsg.Result = FirstPageResult{Page: firstPageRows(1)}
	final := asModel(settled.Update(staleMsg))
	if final.pageExhausted != wantExhaust {
		t.Errorf("stale request settlement changed pageExhausted: got %v, want %v",
			final.pageExhausted, wantExhaust)
	}
	if final.viewportCache != nil && final.viewportCache.Len() != wantCacheLen {
		t.Errorf("stale request settlement changed cache len: got %d, want %d",
			final.viewportCache.Len(), wantCacheLen)
	}
}

// TestFirstSelectStaleGenerationShortSettlementInert proves a stale
// viewport-generation settlement carrying a short page does not set
// pageExhausted or mutate cache/result state (Issue #73 AC4).
func TestFirstSelectStaleGenerationShortSettlementInert(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)

	// Capture the generation at dispatch.
	settled := execCmd()
	realMsg, ok := settled.(SelectSettledMsg)
	if !ok {
		t.Fatalf("executor command produced %T, want SelectSettledMsg", settled)
	}

	// Bump the viewport generation to make the dispatched settlement stale.
	execModel.viewportGen = realMsg.Generation + 999

	final := asModel(execModel.Update(realMsg))
	// The generation mismatch prevents applySelectSettled from running,
	// so pageExhausted stays false (from resetPagingState) and no result
	// is stored.
	if final.pageExhausted {
		t.Error("stale generation settlement set pageExhausted, want false")
	}
	if final.Result != nil {
		t.Errorf("stale generation settlement stored a ResultView: %+v", final.Result)
	}
}
