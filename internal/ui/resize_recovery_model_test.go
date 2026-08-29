// Scripted Bubble Tea coverage for resize-safe vertical viewport recovery
// orchestration (Issue #32 Tasks 3–4), per the SELECT lifecycle, Cache and
// snapshot invariant, Module Design, and resize Testing Decisions of
// Notes/PRD-sqloid.md. Every visible resize recomputes the exact page size
// from complete visible data rows and applies the pure recovery decision to
// the authoritative dual-cap cache metadata: an idle active SELECT preserves
// or clamps locally with no request when retained metadata suffices, or
// issues exactly one containing-page request at the exact new size when a
// fetch is required. While an old-size page request is pending, the resize
// advances the viewport generation and defers the replacement request until
// that old work truly settles; late success and late failure from the old
// generation are rejected, repeated resizes coalesce to the latest decision,
// the independent count request stays untouched, and inactive, suspended,
// and finalized contexts never fetch. The resultcache row/byte invariants
// (single contiguous range, ≤ MaxPositions, ≤ MaxPayloadBytes) are asserted
// after every accepted response. A deterministic fake executor stands in for
// the Connection boundary so no database access runs here.

package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// assertCacheInvariants asserts the Issue #30/#31 resultcache invariants on
// the active SELECT's cache: a single contiguous inclusive range, at most
// MaxPositions retained positions, and retained payload within
// MaxPayloadBytes. Called after every accepted response in these tests.
func assertCacheInvariants(t *testing.T, m Model) {
	t.Helper()
	c := m.viewportCache
	if c == nil {
		return
	}
	start, ok := c.Start()
	if !ok {
		if c.Len() != 0 {
			t.Fatalf("empty-start cache retains %d rows", c.Len())
		}
		return
	}
	end, _ := c.End()
	if end-start+1 != resultcache.Position(c.Len()) {
		t.Fatalf("cache range %d..%d is not contiguous over %d rows", start, end, c.Len())
	}
	if c.Len() > resultcache.MaxPositions {
		t.Fatalf("cache retains %d positions, cap is %d", c.Len(), resultcache.MaxPositions)
	}
	if c.PayloadBytes() > resultcache.MaxPayloadBytes {
		t.Fatalf("cache retains %d payload bytes, cap is %d", c.PayloadBytes(), resultcache.MaxPayloadBytes)
	}
}

// rangedResultPage builds a display page whose `count` rows start at the
// one-based absolute position firstRow, matching the view's range wording.
func rangedResultPage(firstRow int64, count int) *result.Page {
	rows := make([][]result.Value, count)
	for i := range rows {
		rows[i] = []result.Value{result.NewInteger(firstRow + int64(i))}
	}
	return &result.Page{Columns: []string{"id"}, Rows: rows}
}

// fixtureMidResult moves the displayed window of an idle settled model to
// absolute positions starting at firstRow while leaving the retained cache
// exactly as the settled first page built it: the stand-in for a
// post-eviction state whose retained high end sits below the prior row.
func fixtureMidResult(m Model, firstRow int64) Model {
	m.pageOffset = firstRow - 1
	m.Result = &ResultView{Page: rangedResultPage(firstRow, 3)}
	m.pageExhausted = false
	// The fixture's count is unknown: a settled count would establish the
	// high endpoint instead of leaving the decision to the test's variant.
	m.countState = result.CountState{}
	return m
}

// settleHeldPage applies one held PageSettledMsg to the model, returning the
// updated model plus the command Update returned with it.
func settleHeldPage(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestResizeIdlePreservesPriorFirstRowWithoutRequest(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)
	genBefore := m.viewportGen

	next, cmd := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Fatal("idle resize with the prior row retained issued a request")
	}
	if next.pageOffset != 0 {
		t.Fatalf("pageOffset = %d, want the exact prior first row preserved at offset 0", next.pageOffset)
	}
	if !strings.Contains(next.View(), "rows 1-3") {
		t.Errorf("preserved view missing exact prior range rows 1-3: %q", next.View())
	}
	if next.viewportGen == genBefore {
		t.Error("resize did not advance the viewport generation")
	}
	assertCacheInvariants(t, next)
}

func TestResizeIdleClampsToLowRetainedEndpointAfterEviction(t *testing.T) {
	exec := &fakeSelectExecutor{page: rangedResultPage(1, 11)}
	// The paged page holds 10001 rows: the merge evicts positions 1..12 at
	// the 10000-position cap, leaving the retained range 13..10012.
	pageExec := &fakePageExecutor{rowsShown: 10001}
	m := settledFirstPage(t, exec, pageExec)

	m, fwdCmd := pageDown(m)
	settled := settlePage(t, m, fwdCmd)
	start, _ := settled.viewportCache.Start()
	if start != resultcache.Position(13) {
		t.Fatalf("retained start = %d, want 13 after row-cap eviction", start)
	}

	next, cmd := pressKey(settled, tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatal("clamp-low recovery issued a request; the retained endpoint suffices")
	}
	if !strings.Contains(next.View(), "rows 13-23") {
		t.Errorf("clamp-low view missing the retained low endpoint range rows 13-23: %q", next.View())
	}
	if c := next.viewportCache; c.Len() != resultcache.MaxPositions || c.RowCapEvictions() != 12 {
		t.Fatalf("after recovery cache len=%d evictions=%d, want 10000 retained and 12 evictions",
			c.Len(), c.RowCapEvictions())
	}
	assertCacheInvariants(t, next)
}

func TestResizeIdleClampsToKnownHighEndpointWithoutRequest(t *testing.T) {
	tests := []struct {
		name    string
		short   bool
		countOK bool
		count   int64
	}{
		{name: "established short final page", short: true},
		{name: "known count within retained end", countOK: true, count: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeSelectExecutor{page: threeRowPage()}
			pageExec := &fakePageExecutor{rowsShown: 11}
			m := settledFirstPage(t, exec, pageExec)
			// Fixture: the prior row (51) sits above the retained end (3)
			// while the high boundary is established.
			m = fixtureMidResult(m, 51)
			m.pageExhausted = tt.short
			if tt.countOK {
				m.countState = result.CountState{Status: result.CountSuccess, Total: tt.count}
			}

			next, cmd := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
			if cmd != nil {
				t.Fatalf("%s: clamp-high recovery issued a request", tt.name)
			}
			if !strings.Contains(next.View(), "rows 3-3") {
				t.Errorf("%s: clamp-high view missing the retained end row 3: %q", tt.name, next.View())
			}
			if !next.pageExhausted {
				t.Errorf("%s: clamp-high recovery did not mark the exhausted boundary", tt.name)
			}
			assertCacheInvariants(t, next)
		})
	}
}

func TestResizeIdleFetchesExactNewPageSizeContainingPage(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 15}
	m := settledFirstPage(t, exec, pageExec)
	// Fixture: the prior row (51) sits above the retained end (3) and the
	// high boundary is not established, so the containing page is required.
	m = fixtureMidResult(m, 51)

	next, cmd := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd == nil {
		t.Fatal("idle fetch recovery issued no containing-page request command")
	}
	if pageExec.issued != 0 {
		t.Fatalf("page executor ran %d times before its command was invoked, want 0", pageExec.issued)
	}
	msg := cmd()
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}
	// Containing new-size page of row 51 at size 15: absolute rows 46-60.
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 45`
	if pageExec.sqls[0] != want {
		t.Errorf("recovery page SQL = %q, want %q", pageExec.sqls[0], want)
	}

	settled, _ := settleHeldPage(t, next, msg)
	if !strings.Contains(settled.View(), "rows 46-60") {
		t.Errorf("recovered view missing containing-page range rows 46-60: %q", settled.View())
	}
	assertCacheInvariants(t, settled)
}

func TestResizePendingPageRejectsLateSuccessWithoutReplacement(t *testing.T) {
	exec := &fakeSelectExecutor{page: rangedResultPage(1, 11)}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)

	pending, pageCmd := pageDown(m)
	if pageCmd == nil {
		t.Fatal("Page Down issued no page request command")
	}
	held := pageCmd() // issues the old-size request; its response is held
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}

	next, resizeCmd := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	if resizeCmd != nil {
		t.Fatal("resize with a pending page issued a replacement command before settlement")
	}
	if !next.pagePending {
		t.Fatal("resize cleared the pending slot before the old request settled")
	}

	// The old-generation response settles late: rejected, no row overwrite,
	// no replacement request (the local preserve needed no fetch).
	after, settleCmd := settleHeldPage(t, next, held)
	if settleCmd != nil {
		t.Fatal("old-generation settlement returned an unexpected command")
	}
	if after.pagePending {
		t.Error("old-generation settlement did not release the pending slot")
	}
	if !strings.Contains(after.View(), "rows 1-11") {
		t.Errorf("late old-generation success overwrote the preserved view: %q", after.View())
	}
	if pageExec.issued != 1 {
		t.Errorf("page executor ran %d times after settlement, want no replacement request", pageExec.issued)
	}
	assertCacheInvariants(t, after)
}

func TestResizePendingPageDefersExactlyOneReplacementUntilSettlement(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51) // prior row 51 above retained end 3: fetch required

	pending, pageCmd := pageDown(m)
	held := pageCmd() // issues the old-size request
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}

	next, resizeCmd := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	if resizeCmd != nil {
		t.Fatal("resize with a pending page issued the replacement before settlement")
	}
	if pageExec.issued != 1 {
		t.Fatalf("replacement issued before settlement (%d runs)", pageExec.issued)
	}

	// The old-size response settles late: rejected rows, released slot, and
	// exactly one correctly sized containing-page replacement request.
	after, replacementCmd := settleHeldPage(t, next, held)
	if replacementCmd == nil {
		t.Fatal("settled old page did not dispatch the deferred replacement request")
	}
	replacementMsg := replacementCmd()
	if pageExec.issued != 2 {
		t.Fatalf("page executor ran %d times, want exactly one replacement", pageExec.issued)
	}
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 45`
	if pageExec.sqls[1] != want {
		t.Errorf("replacement SQL = %q, want %q", pageExec.sqls[1], want)
	}

	settled, settleCmd := settleHeldPage(t, after, replacementMsg)
	if settleCmd != nil {
		t.Fatal("replacement settlement issued an unexpected command")
	}
	if !strings.Contains(settled.View(), "rows 46-60") {
		t.Errorf("recovered view missing containing-page range rows 46-60: %q", settled.View())
	}
	assertCacheInvariants(t, settled)
}

func TestRepeatedResizeBeforeSettlementUsesLatestSize(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51)

	pending, pageCmd := pageDown(m)
	held := pageCmd() // issues the old-size request
	next, _ := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ = pressKey(next, tea.WindowSizeMsg{Width: 80, Height: 24}) // coalesce to latest size
	if !next.resizeFetchPending {
		t.Fatal("repeated resize lost the deferred replacement")
	}

	after, replacementCmd := settleHeldPage(t, next, held)
	if replacementCmd == nil {
		t.Fatal("settled old page did not dispatch the deferred replacement request")
	}
	replacementMsg := replacementCmd()
	// The latest decision (size 11) contains row 51 at absolute rows 45-55.
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 11 OFFSET 44`
	if pageExec.sqls[len(pageExec.sqls)-1] != want {
		t.Errorf("replacement SQL = %q, want the latest size %q", pageExec.sqls[len(pageExec.sqls)-1], want)
	}
	final, _ := settleHeldPage(t, after, replacementMsg)
	if !strings.Contains(final.View(), "rows 45-55") {
		t.Errorf("recovered view missing containing-page range rows 45-55: %q", final.View())
	}
	assertCacheInvariants(t, final)
}

func TestLateOldGenerationFailureStillSettlesThenReplaces(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)
	m = fixtureMidResult(m, 51)

	failExec := &fakePageExecutor{err: context.Canceled, rowsShown: 15}
	m.Page = failExec.page
	pending, pageCmd := pageDown(m)
	if pageCmd == nil {
		t.Fatal("Page Down issued no page request command")
	}
	held := pageCmd()
	if failExec.issued != 1 {
		t.Fatalf("failing executor ran %d times, want exactly one request", failExec.issued)
	}

	next, resizeCmd := pressKey(pending, tea.WindowSizeMsg{Width: 100, Height: 30})
	if resizeCmd != nil {
		t.Fatal("resize with a pending page issued the replacement before settlement")
	}

	// The late old-generation failure settles inertly, then the deferred
	// replacement dispatches exactly once.
	after, replacementCmd := settleHeldPage(t, next, held)
	if replacementCmd == nil {
		t.Fatal("late old-generation failure did not dispatch the deferred replacement")
	}
	failExec.err = nil // the connection recovers: the replacement itself succeeds
	replacementMsg := replacementCmd()
	if failExec.issued != 2 {
		t.Fatalf("failing executor ran %d times, want the held request plus one replacement", failExec.issued)
	}
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 45`
	if failExec.sqls[1] != want {
		t.Errorf("replacement SQL = %q, want %q", failExec.sqls[1], want)
	}
	// The rejected late failure changed nothing: the stale page's range never
	// appears and the fixture's rows are still displayed.
	if got := after.View(); strings.Contains(got, "rows 54-64") {
		t.Errorf("late old-generation failure changed the displayed range: %q", got)
	}
	final, _ := settleHeldPage(t, after, replacementMsg)
	if !strings.Contains(final.View(), "rows 46-60") {
		t.Errorf("recovered view missing containing-page range rows 46-60: %q", final.View())
	}
	assertCacheInvariants(t, final)
}

func TestResizeLeavesPendingIndependentCountUntouched(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	count := &fakeCountExecutor{total: 42}
	m := pagingModel(exec, pageExec)
	m.Count = count.count

	execModel, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	withPage, _ := execModel.Update(page)
	counted := withPage.(Model)
	if !counted.countPendingFlag {
		t.Fatal("count request is not pending after its page settled")
	}

	next, resizeCmd := pressKey(counted, tea.WindowSizeMsg{Width: 100, Height: 30})
	if resizeCmd != nil {
		t.Fatal("resize with only the count pending issued a page request")
	}
	if !next.countPendingFlag {
		t.Error("resize disturbed the independent pending count request")
	}
	if count.calls != 1 {
		t.Errorf("count executor ran %d times, want no restart", count.calls)
	}
	if pageExec.issued != 0 {
		t.Errorf("page executor ran %d times, want no request", pageExec.issued)
	}

	// The count settles afterwards into its own state, independent of the
	// resize recovery.
	nextModel, countCmd := next.Update(cnt)
	final := nextModel.(Model)
	if countCmd != nil {
		t.Error("count settlement issued an unexpected command")
	}
	if !strings.Contains(final.View(), "Result count: 42") {
		t.Errorf("settled count view missing exact wording: %q", final.View())
	}
	assertCacheInvariants(t, final)
}

func TestResizeNeverFetchesInactiveOrFinalizedContexts(t *testing.T) {
	// No active result at all: nothing to recover, no fetch.
	idle, _ := sized(New(), 100, 30).(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if idle.(Model).Result != nil {
		t.Fatal("resize created a result on an inactive session")
	}

	// A finalized session never issues database work on resize.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)
	m.terminalState = TerminalReplaced
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30}); cmd != nil {
		t.Fatal("resize in a finalized session issued a request command")
	}
	if pageExec.issued != 0 {
		t.Errorf("page executor ran %d times in a finalized session, want 0", pageExec.issued)
	}
}

func TestTooSmallResizeSuspendsWithoutRecoveryAndRestoresRecovery(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)

	hidden, cmd := pressKey(m, tea.WindowSizeMsg{Width: 79, Height: 30})
	if cmd != nil {
		t.Fatal("too-small resize issued a request command while hidden state is frozen")
	}
	if !hidden.suspended {
		t.Fatal("too-small resize did not suspend the shell")
	}

	// Becoming visible again is the restoring resize: recovery runs locally
	// (the prior row is retained) with no fetch, and the exact new page size
	// governs the next adjacent page request.
	restored, cmd := pressKey(hidden, tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Fatal("restoring resize issued a request command")
	}
	if restored.suspended {
		t.Fatal("restoring resize did not restore the shell")
	}
	_, pageCmd := pageDown(restored)
	if pageCmd == nil {
		t.Fatal("Page Down after restoration issued no command")
	}
	pageCmd()
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 3`
	if got := pageExec.sqls[len(pageExec.sqls)-1]; got != want {
		t.Errorf("page SQL = %q, want the restored 15-row page size %q", got, want)
	}
}

func TestResizeRecomputesExactPageSizeForAdjacentRequests(t *testing.T) {
	tests := []struct {
		name      string
		firstW    int
		firstH    int
		secondW   int
		secondH   int
		wantLimit string
	}{
		{name: "grow", firstW: 80, firstH: 24, secondW: 100, secondH: 30, wantLimit: "15"},
		{name: "shrink", firstW: 100, firstH: 30, secondW: 80, secondH: 24, wantLimit: "11"},
		{name: "unchanged", firstW: 80, firstH: 24, secondW: 80, secondH: 24, wantLimit: "11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeSelectExecutor{page: threeRowPage()}
			pageExec := &fakePageExecutor{rowsShown: 11}
			m0 := sized(pagingModel(exec, pageExec), tt.firstW, tt.firstH).(Model)
			execModel, execCmd := driveToExecutionStart(t, m0)
			m := settleFirstPage(t, execModel, execCmd)

			// The recovery decision itself issues nothing locally.
			next, resizeCmd := pressKey(m, tea.WindowSizeMsg{Width: tt.secondW, Height: tt.secondH})
			if resizeCmd != nil {
				t.Fatalf("%s: resize issued a request command", tt.name)
			}
			_, pageCmd := pageDown(next)
			if pageCmd == nil {
				t.Fatalf("%s: Page Down after resize issued no command", tt.name)
			}
			pageCmd()
			want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT ` +
				tt.wantLimit + ` OFFSET 3`
			if pageExec.sqls[len(pageExec.sqls)-1] != want {
				t.Errorf("%s: page SQL = %q, want %q", tt.name, pageExec.sqls[len(pageExec.sqls)-1], want)
			}
			assertCacheInvariants(t, next)
		})
	}
}

func TestNewExecutionReplacesCacheSoFirstPageMergesFresh(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)
	if got := m.viewportCache.Len(); got != 3 {
		t.Fatalf("first cache length = %d, want 3", got)
	}

	// A second execution's smaller first page must not merge into the stale
	// retained range of the previous result: the fresh cache is exactly 1..2.
	exec.page = rangedResultPage(1, 2)
	newer, newerPage, newerCount := newerExecutionOver(t, m)
	next, _ := newer.Update(newerPage)
	m2 := next.(Model)
	next2, _ := m2.Update(newerCount)
	m2 = next2.(Model)

	start, ok := m2.viewportCache.Start()
	if !ok {
		t.Fatal("fresh execution left no retained cache")
	}
	end, _ := m2.viewportCache.End()
	if start != 1 || end != 2 {
		t.Fatalf("fresh cache = %d..%d, want exactly the new first page 1..2", start, end)
	}
	assertCacheInvariants(t, m2)
}
