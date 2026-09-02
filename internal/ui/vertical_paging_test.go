// Scripted Bubble Tea coverage for serialized vertical result paging (Issue
// #25 Tasks 3–4): Page Down and Page Up on an idle active SELECT request
// exactly the adjacent absolute logical range through QueryBuilder's page
// API; navigation stops at the known low/high boundaries; the user's Limit
// is never read beyond; at most one page request stays pending while only
// the independent count request may coexist; repeated and opposite page keys
// are consumed without stacking commands or issuing another request and
// keep page-loading feedback visible while horizontal movement stays local;
// and page size always equals all complete visible data rows after the
// results border, status/count line, and frozen header — no partially
// visible row counts. A deterministic fake executor stands in for the
// Connection boundary so no database access runs here.

package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// fakePageExecutor records every paged-page request and returns a queued
// outcome. issued counts requests whose command was invoked; settled counts
// outcomes actually applied to the model, so tests can hold a response
// behind a barrier simply by deferring the Update of the settled message.
// offsets records the structured logical offset passed to each request
// (Issue #71), in addition to SQL and parameters.
type fakePageExecutor struct {
	issued     int
	settled    int
	sqls       []string
	params     [][]any
	offsets    []int64
	rowsShown  int64 // rows in each returned page
	honorLimit bool  // parse the statement's LIMIT and return exactly that many rows
	err        error
	// limitFailure, when non-nil, is returned as the typed FirstPageResult
	// LimitFailure at the given page-relative index (Issue #71) — but only
	// when the request's offset matches limitFailureOffset. The fake
	// computes the absolute position from offset + relativeIdx + 1.
	limitFailure       *result.LimitFailure
	limitFailureAt     int64 // 0-based page-relative index of the failing row
	limitFailureOffset int64 // absolute offset at which to return the failure
}

func (f *fakePageExecutor) page(ctx context.Context, sql string, params []any, offset int64) FirstPageResult {
	f.issued++
	f.sqls = append(f.sqls, sql)
	f.params = append(f.params, params)
	f.offsets = append(f.offsets, offset)
	if f.err != nil {
		return FirstPageResult{Err: f.err}
	}
	if f.limitFailure != nil && offset == f.limitFailureOffset {
		// Issue #71: the fake mirrors the connection scanner's contract —
		// the absolute position is offset + page-relative index + 1 — so the
		// UI-visible message names the exact absolute row N.
		absPos := offset + f.limitFailureAt + 1
		lf := &result.LimitFailure{Kind: f.limitFailure.Kind, Position: absPos}
		// Return the complete leading rows before the failing row.
		out := make([][]result.Value, f.limitFailureAt)
		for i := range out {
			out[i] = []result.Value{result.NewInteger(int64(i+1) + offset)}
		}
		return FirstPageResult{Page: &result.Page{Columns: []string{"id"}, Rows: out}, LimitFailure: lf}
	}
	rows := f.rowsShown
	if f.honorLimit {
		// Realistic: the page holds exactly the requested LIMIT rows.
		if i := strings.Index(sql, "LIMIT "); i >= 0 {
			if v, err := strconv.Atoi(strings.Fields(sql[i+6:])[0]); err == nil {
				rows = int64(v)
			}
		}
	}
	out := make([][]result.Value, rows)
	for i := range out {
		out[i] = []result.Value{result.NewInteger(int64(i + 1))}
	}
	return FirstPageResult{Page: &result.Page{Columns: []string{"id"}, Rows: out}}
}

// pagingModel wires a runnable SELECT model with a first-page executor, a
// paged-page executor, and a fresh history store.
func pagingModel(exec *fakeSelectExecutor, pageExec *fakePageExecutor) Model {
	m := firstSelectModel(exec)
	m.Page = pageExec.page
	// Issue #24 wiring: paging coexists with the independent count request,
	// so the default paging model carries both executor seams.
	m.Count = (&fakeCountExecutor{}).count
	return m
}

// threeRowPage returns a first-page fixture displaying three rows.
func threeRowPage() *result.Page {
	return &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)}, {result.NewInteger(2)}, {result.NewInteger(3)},
	}}
}

// settleFirstPage applies the page and count completions of one execution
// batch, returning the idle model with the first page displayed.
func settleFirstPage(t *testing.T, m Model, execCmd tea.Cmd) Model {
	t.Helper()
	msgs := execBatch(t, execCmd)
	page, count := splitSelectCount(t, msgs)
	next, _ := m.Update(page)
	m2 := next.(Model)
	// The count settlement mutates the model (it clears the count's pending
	// slot), so its updated model must be kept; only its command is checked.
	next2, nextCmd := m2.Update(count)
	m2 = next2.(Model)
	if nextCmd != nil {
		t.Fatal("count settlement issued an unexpected command")
	}
	return m2
}

// settledFirstPage drives a paging model through validation, execution, and
// first-page settlement, returning the idle model with that page displayed.
func settledFirstPage(t *testing.T, exec *fakeSelectExecutor, pageExec *fakePageExecutor) Model {
	t.Helper()
	m := pagingModel(exec, pageExec)
	execModel, execCmd := driveToExecutionStart(t, m)
	return settleFirstPage(t, execModel, execCmd)
}

// settlePage invokes a page request command and applies its settled message,
// returning the updated model.
func settlePage(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("page key produced no command")
	}
	msg := cmd()
	settled, ok := msg.(PageSettledMsg)
	if !ok {
		t.Fatalf("page command produced %T, want PageSettledMsg", msg)
	}
	next, _ := m.Update(settled)
	return next.(Model)
}

// pressKey applies one key press and returns the model plus its command.
func pressKey(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// pageDown and pageUp press the adjacent-page keys.
func pageDown(m Model) (Model, tea.Cmd) { return pressKey(m, tea.KeyMsg{Type: tea.KeyPgDown}) }
func pageUp(m Model) (Model, tea.Cmd)   { return pressKey(m, tea.KeyMsg{Type: tea.KeyPgUp}) }

func TestPageDownRequestsAdjacentRange(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)

	next, cmd := pageDown(m)
	if cmd == nil {
		t.Fatal("idle Page Down issued no page request command")
	}
	if pageExec.issued != 0 {
		t.Fatalf("page executor ran %d times before its command was invoked, want 0", pageExec.issued)
	}
	cmd() // issues the request
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 11 OFFSET 3`
	if pageExec.sqls[0] != want {
		t.Errorf("page SQL = %q, want %q", pageExec.sqls[0], want)
	}
	if len(pageExec.params[0]) != 1 || pageExec.params[0][0] != "x" {
		t.Errorf("page params = %v, want [x] in the first page's order", pageExec.params[0])
	}
	// The pending request keeps the displayed page but shows loading feedback.
	if got := next.View(); !strings.Contains(got, PageLoadingIndicator) {
		t.Errorf("pending page request missing loading feedback %q: %q", PageLoadingIndicator, got)
	}

	settled := settlePage(t, next, cmd)
	if !strings.Contains(settled.View(), "rows 4-14") {
		t.Errorf("settled Page Down view missing absolute range rows 4-14: %q", settled.View())
	}
}

func TestPageUpSuppressedAtLowBoundary(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)

	_, cmd := pageUp(m)
	if cmd != nil {
		t.Fatal("Page Up at the low boundary issued a command")
	}
	if pageExec.issued != 0 {
		t.Fatalf("page executor ran %d times, want 0", pageExec.issued)
	}
}

func TestPageUpRequestsAdjacentBackwardRange(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
	m := settledFirstPage(t, exec, pageExec)

	// Page forward to offset 3, then back: the request is the exact
	// backwards range of 11 rows ending at the displayed start.
	m, fwdCmd := pageDown(m)
	settled := settlePage(t, m, fwdCmd)

	next, cmd := pageUp(settled)
	if cmd == nil {
		t.Fatal("Page Up below the low boundary issued no command")
	}
	cmd()
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 3 OFFSET 0`
	if pageExec.sqls[1] != want {
		t.Errorf("page up SQL = %q, want %q", pageExec.sqls[1], want)
	}
	settled = settlePage(t, next, cmd)
	if !strings.Contains(settled.View(), "rows 1-3") {
		t.Errorf("settled Page Up view missing absolute range rows 1-3: %q", settled.View())
	}
}

func TestPageDownStopsAtHighBoundary(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 2} // shorter than any page size
	m := settledFirstPage(t, exec, pageExec)

	next, cmd := pageDown(m)
	if cmd == nil {
		t.Fatal("Page Down after the first page issued no command")
	}
	// A page shorter than the requested size is the last page: the next
	// Page Down is consumed at the known high boundary.
	m2, cmd2 := pageDown(settlePage(t, next, cmd))
	_ = m2
	if cmd2 != nil {
		t.Fatal("Page Down at the high boundary issued a command")
	}
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}
}

func TestPageDownRespectsUserLimitBoundary(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := pagingModel(exec, pageExec)
	// A user Limit of 3 bounds the logical result at the 3 displayed rows.
	m.QB = m.QB.SetLimitInput("3")
	m.applyBuilder(m.QB)

	started, execCmd := driveToExecutionStart(t, m)
	m2 := settleFirstPage(t, started, execCmd)

	_, cmd := pageDown(m2)
	if cmd != nil {
		t.Fatal("Page Down beyond the user's Limit issued a command")
	}
	if pageExec.issued != 0 {
		t.Fatalf("page executor ran %d times, want 0", pageExec.issued)
	}
}

func TestRepeatedAndOppositeKeysSuppressedWhilePending(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)

	next, cmd := pageDown(m)
	if cmd == nil {
		t.Fatal("Page Down issued no command")
	}
	// Hold the response: the request is issued but not yet settled.
	cmd() // issues the request

	// Repeated Page Down while pending: consumed, no stacked command.
	_, repCmd := pageDown(next)
	if repCmd != nil {
		t.Fatal("repeated Page Down while pending stacked a command")
	}
	// Opposite Page Up while pending: consumed as well.
	_, oppCmd := pageUp(next)
	if oppCmd != nil {
		t.Fatal("opposite Page Up while pending stacked a command")
	}
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times while pending, want exactly one request", pageExec.issued)
	}
	if got := next.View(); !strings.Contains(got, PageLoadingIndicator) {
		t.Errorf("pending page view missing loading feedback %q: %q", PageLoadingIndicator, got)
	}

	// The request keeps its exact range: the executor saw only the original
	// offset.
	if want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 11 OFFSET 3`; pageExec.sqls[0] != want {
		t.Errorf("page SQL = %q, want preserved %q", pageExec.sqls[0], want)
	}

	settled := settlePage(t, next, cmd)
	if got := settled.View(); strings.Contains(got, PageLoadingIndicator) {
		t.Errorf("settled page view still shows loading feedback: %q", got)
	}
}

func TestHorizontalMovementStaysLocalWhilePending(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	m := settledFirstPage(t, exec, pageExec)

	next, cmd := pageDown(m)
	cmd() // issues the request

	for _, k := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
		if _, moveCmd := pressKey(next, tea.KeyMsg{Type: k}); moveCmd != nil {
			t.Fatalf("horizontal key %v issued a command while a page was pending", k)
		}
	}
	if pageExec.issued != 1 {
		t.Fatalf("horizontal movement issued page requests, want only the one held pending")
	}
	if got := next.View(); !strings.Contains(got, PageLoadingIndicator) {
		t.Errorf("pending page view missing loading feedback after local movement: %q", got)
	}
}

func TestCountCoexistsWithPendingPage(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 11}
	count := &fakeCountExecutor{total: 42}
	m := pagingModel(exec, pageExec)
	m.Count = count.count

	execModel, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	// The first page settles while the count stays pending.
	withPage, _ := execModel.Update(page)
	execDone := withPage.(Model)

	// With the count still pending, the user pages: exactly one page request
	// may coexist with it.
	pending, pageCmd := pageDown(execDone)
	if pageCmd == nil {
		t.Fatal("Page Down while the count is pending issued no command")
	}
	pageCmd() // issues the page request
	if pageExec.issued != 1 {
		t.Fatalf("page executor ran %d times, want exactly one request", pageExec.issued)
	}
	// The count settles while the page is pending, into its own state.
	withCount, _ := pending.Update(cnt)
	counted := withCount.(Model)
	if !strings.Contains(counted.View(), "Result count: 42") {
		t.Errorf("count settlement view missing exact wording: %q", counted.View())
	}
	if got := counted.View(); !strings.Contains(got, PageLoadingIndicator) {
		t.Errorf("count settlement cleared the pending page's loading feedback: %q", got)
	}
	// The page settles afterwards and installs its rows.
	settled := settlePage(t, counted, pageCmd)
	if strings.Contains(settled.View(), PageLoadingIndicator) {
		t.Errorf("settled page view still shows loading feedback: %q", settled.View())
	}
}

func TestPageSizeEqualsCompleteVisibleRows(t *testing.T) {
	tests := []struct {
		name     string
		height   int
		pageRows int
	}{
		{name: "80x24", height: 24, pageRows: 11},
		{name: "100x30", height: 30, pageRows: 15},
		{name: "160x50", height: 50, pageRows: 34},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeSelectExecutor{page: threeRowPage()}
			pageExec := &fakePageExecutor{rowsShown: 100}
			m := settledFirstPage(t, exec, pageExec)
			resized := sized(m, 80, tt.height).(Model)

			_, cmd := pageDown(resized)
			if cmd == nil {
				t.Fatalf("Page Down at %dx%d issued no command", 80, tt.height)
			}
			cmd()
			want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT ` +
				strconv.Itoa(tt.pageRows) + ` OFFSET 3`
			if pageExec.sqls[len(pageExec.sqls)-1] != want {
				t.Errorf("page SQL = %q, want LIMIT exactly %d complete visible rows",
					pageExec.sqls[len(pageExec.sqls)-1], tt.pageRows)
			}
		})
	}
}

func TestPageSizeAfterResizeUsesNewValue(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 100}
	m := settledFirstPage(t, exec, pageExec)

	resized, _ := pressKey(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	_, cmd := pageDown(resized)
	if cmd == nil {
		t.Fatal("Page Down after resize issued no command")
	}
	cmd()
	want := `SELECT * FROM "users" WHERE "email" = ? ORDER BY rowid LIMIT 15 OFFSET 3`
	if pageExec.sqls[len(pageExec.sqls)-1] != want {
		t.Errorf("page SQL = %q, want the resized 15 complete visible rows", pageExec.sqls[len(pageExec.sqls)-1])
	}
}

// pageSQLOffset extracts the integer OFFSET value from a rendered PageSQL
// statement, matching QueryBuilder's exact `LIMIT N OFFSET O` range.
func pageSQLOffset(sql string) int64 {
	i := strings.Index(sql, "OFFSET ")
	if i < 0 {
		return 0
	}
	v, err := strconv.ParseInt(strings.Fields(sql[i+7:])[0], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// collapseWhitespace replaces all runs of whitespace (including newlines from
// terminal line wrapping) with single spaces, so a message wrapped across
// rendered lines can be checked with strings.Contains. Border box-drawing
// characters are removed first so a message split at a border joins cleanly.
func collapseWhitespace(s string) string {
	for _, ch := range []string{"│", "╭", "╮", "╰", "╯", "─"} {
		s = strings.ReplaceAll(s, ch, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

// TestPageExecutorReceivesOffsetMatchingPageSQL proves Issue #71's contract:
// the logical offset passed to the page executor equals the OFFSET rendered
// into QueryBuilder's PageSQL, for forward, backward, and Limit-clamped
// ranges, nonzero current pages, and the resize/refetch path. The executor
// fake records the structured offset alongside SQL and parameters.
func TestPageExecutorReceivesOffsetMatchingPageSQL(t *testing.T) {
	t.Run("forward from first page", func(t *testing.T) {
		exec := &fakeSelectExecutor{page: threeRowPage()}
		pageExec := &fakePageExecutor{rowsShown: 11}
		m := settledFirstPage(t, exec, pageExec)

		_, cmd := pageDown(m)
		cmd()
		if len(pageExec.offsets) != 1 {
			t.Fatalf("recorded offsets = %v, want one", pageExec.offsets)
		}
		sqlOffset := pageSQLOffset(pageExec.sqls[0])
		if pageExec.offsets[0] != sqlOffset {
			t.Fatalf("executor offset = %d, want PageSQL OFFSET %d", pageExec.offsets[0], sqlOffset)
		}
		if pageExec.offsets[0] != 3 {
			t.Fatalf("forward offset = %d, want 3 (after 3 displayed rows)", pageExec.offsets[0])
		}
	})

	t.Run("backward from nonzero page", func(t *testing.T) {
		exec := &fakeSelectExecutor{page: threeRowPage()}
		pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
		m := settledFirstPage(t, exec, pageExec)

		// Page forward to offset 3, then back.
		m, fwdCmd := pageDown(m)
		settled := settlePage(t, m, fwdCmd)

		_, cmd := pageUp(settled)
		cmd()
		if len(pageExec.offsets) < 2 {
			t.Fatalf("recorded offsets = %v, want at least two", pageExec.offsets)
		}
		sqlOffset := pageSQLOffset(pageExec.sqls[1])
		if pageExec.offsets[1] != sqlOffset {
			t.Fatalf("backward executor offset = %d, want PageSQL OFFSET %d", pageExec.offsets[1], sqlOffset)
		}
		if pageExec.offsets[1] != 0 {
			t.Fatalf("backward offset = %d, want 0 (clamped to low boundary)", pageExec.offsets[1])
		}
	})

	t.Run("forward from nonzero page", func(t *testing.T) {
		exec := &fakeSelectExecutor{page: threeRowPage()}
		pageExec := &fakePageExecutor{rowsShown: 11, honorLimit: true}
		m := settledFirstPage(t, exec, pageExec)

		// Page forward twice: first to offset 3, then to offset 14.
		m, fwdCmd := pageDown(m)
		settled := settlePage(t, m, fwdCmd)

		_, cmd := pageDown(settled)
		cmd()
		if len(pageExec.offsets) < 2 {
			t.Fatalf("recorded offsets = %v, want at least two", pageExec.offsets)
		}
		sqlOffset := pageSQLOffset(pageExec.sqls[len(pageExec.sqls)-1])
		if pageExec.offsets[len(pageExec.offsets)-1] != sqlOffset {
			t.Fatalf("second forward executor offset = %d, want PageSQL OFFSET %d",
				pageExec.offsets[len(pageExec.offsets)-1], sqlOffset)
		}
		if pageExec.offsets[len(pageExec.offsets)-1] != 14 {
			t.Fatalf("second forward offset = %d, want 14 (3 + 11 displayed)", pageExec.offsets[len(pageExec.offsets)-1])
		}
	})

	t.Run("resize refetch uses new offset", func(t *testing.T) {
		exec := &fakeSelectExecutor{page: threeRowPage()}
		pageExec := &fakePageExecutor{rowsShown: 11}
		m := settledFirstPage(t, exec, pageExec)

		// Page forward to offset 3, then resize triggers a refetch.
		m, fwdCmd := pageDown(m)
		settled := settlePage(t, m, fwdCmd)

		resized, _ := pressKey(settled, tea.WindowSizeMsg{Width: 100, Height: 30})
		// After resize, page down should use the current displayed offset.
		_, cmd := pageDown(resized)
		if cmd == nil {
			t.Fatal("Page Down after resize issued no command")
		}
		cmd()
		last := len(pageExec.offsets) - 1
		sqlOffset := pageSQLOffset(pageExec.sqls[last])
		if pageExec.offsets[last] != sqlOffset {
			t.Fatalf("resize refetch executor offset = %d, want PageSQL OFFSET %d", pageExec.offsets[last], sqlOffset)
		}
	})
}

// TestPageExecutorValueLimitFailureShowsAbsoluteRow proves Issue #71's
// UI-visible contract: when the page executor returns a typed value
// LimitFailure at a known page-relative index on a request with a nonzero
// offset, the rendered view shows the exact absolute row-N message where
// N = offset + relativeIdx + 1.
func TestPageExecutorValueLimitFailureShowsAbsoluteRow(t *testing.T) {
	cases := []struct {
		name        string
		offset      int64
		relativeIdx int64
	}{
		{"first relative row at offset 3", 3, 0},
		{"second relative row at offset 3", 3, 1},
		{"third relative row at offset 14", 14, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fakeSelectExecutor{page: threeRowPage()}
			pageExec := &fakePageExecutor{
				rowsShown:          11,
				honorLimit:         true,
				limitFailure:       &result.LimitFailure{Kind: result.KindValue},
				limitFailureAt:     tc.relativeIdx,
				limitFailureOffset: tc.offset,
			}
			m := settledFirstPage(t, exec, pageExec)

			// Page forward to the target offset. The first page shows 3 rows
			// (offset 0); one page down reaches offset 3, and a second
			// reaches offset 14. The fake returns the limit failure when the
			// offset matches limitFailureOffset, so the final page down
			// produces the failure.
			cur := m
			pageDowns := 1 // always at least one to reach offset 3
			if tc.offset > 3 {
				pageDowns = 2 // second page down reaches offset 14
			}
			for i := 0; i < pageDowns; i++ {
				next, cmd := pageDown(cur)
				if cmd == nil {
					t.Fatalf("page down %d issued no command", i+1)
				}
				cur = settlePage(t, next, cmd)
			}

			// The executor recorded the offset it received on the last call.
			last := len(pageExec.offsets) - 1
			if pageExec.offsets[last] != tc.offset {
				t.Fatalf("executor offset = %d, want %d", pageExec.offsets[last], tc.offset)
			}

			// The view shows the exact absolute row-N message. The terminal
			// renderer may wrap the message across lines, so collapse
			// whitespace before checking.
			wantPos := tc.offset + tc.relativeIdx + 1
			wantMsg := "result value exceeds the 64 MiB v1 limit at row " + strconv.FormatInt(wantPos, 10)
			if got := collapseWhitespace(cur.View()); !strings.Contains(got, wantMsg) {
				t.Fatalf("view missing exact absolute message %q:\n%s", wantMsg, cur.View())
			}
		})
	}
}

// TestPageExecutorPageLimitFailureShowsAbsoluteRow proves Issue #71's
// UI-visible contract for page-envelope failures: when the page executor
// returns a typed page LimitFailure at a known page-relative index on a
// request with a nonzero offset, the rendered view shows the exact absolute
// row-N message where N = offset + relativeIdx + 1.
func TestPageExecutorPageLimitFailureShowsAbsoluteRow(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{
		rowsShown:          11,
		honorLimit:         true,
		limitFailure:       &result.LimitFailure{Kind: result.KindPage},
		limitFailureAt:     2,  // third page-relative row
		limitFailureOffset: 14, // after paging forward twice from offset 3
	}
	m := settledFirstPage(t, exec, pageExec)

	// Page forward to offset 3 (first page shows 3 rows).
	next, cmd := pageDown(m)
	settled := settlePage(t, next, cmd)

	// Page forward to offset 14 — the fake returns the page-envelope failure.
	next, cmd = pageDown(settled)
	if cmd == nil {
		t.Fatal("page key issued no command")
	}
	failSettled := settlePage(t, next, cmd)

	// The executor recorded offset 14 (3 + 11 displayed rows).
	last := len(pageExec.offsets) - 1
	if pageExec.offsets[last] != 14 {
		t.Fatalf("executor offset = %d, want 14", pageExec.offsets[last])
	}

	// The view shows the exact absolute row-N message: 14 + 2 + 1 = 17.
	// The terminal renderer may wrap the message across lines, so collapse
	// whitespace before checking.
	wantMsg := "result page exceeds the 64 MiB v1 limit at row 17"
	if got := collapseWhitespace(failSettled.View()); !strings.Contains(got, wantMsg) {
		t.Fatalf("view missing exact absolute page-limit message %q:\n%s", wantMsg, failSettled.View())
	}
}
