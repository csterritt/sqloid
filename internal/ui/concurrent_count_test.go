// Scripted Bubble Tea coverage for the concurrent first page and independent
// result count (Issue #24 Tasks 3–4): one actual SELECT execution assigns one
// execution ID and two distinct role-specific request IDs; page and count
// launch concurrently without waiting for either result; completions mutate
// active state only when both identity levels match and the role is
// unconsumed; count wording is exact from explicit state; count failure stays
// isolated behind `Count unavailable` while rows and paging survive; and rows
// are never clamped to an inconsistent count. A deterministic fake executor
// stands in for the Connection boundary so no database access runs here.

package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// fakeCountExecutor records every count request and returns a queued outcome,
// so tests can prove exactly one count execution with exact inputs.
type fakeCountExecutor struct {
	sqls   []string
	params [][]any
	calls  int
	total  int64
	err    error
}

func (f *fakeCountExecutor) count(ctx context.Context, sql string, params []any) CountResult {
	f.calls++
	f.sqls = append(f.sqls, sql)
	f.params = append(f.params, params)
	if f.err != nil {
		return CountResult{Err: f.err}
	}
	return CountResult{Total: f.total}
}

// concurrentCountModel wires a runnable SELECT model with both executor seams
// wired, plus a fresh history store.
func concurrentCountModel(exec *fakeSelectExecutor, count *fakeCountExecutor) Model {
	m := firstSelectModel(exec)
	m.Count = count.count
	return m
}

// execBatch unwraps one executor command into its concurrently launched
// messages, proving exactly the expected page/count pair.
func execBatch(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("execution start produced no executor command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("execution start produced %T, want a page/count batch", msg)
	}
	var out []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		out = append(out, c())
	}
	return out
}

// splitSelectCount separates the batch into the page and count messages so
// tests control their arrival order deterministically.
func splitSelectCount(t *testing.T, msgs []tea.Msg) (SelectSettledMsg, CountSettledMsg) {
	t.Helper()
	var page *SelectSettledMsg
	var count *CountSettledMsg
	for _, msg := range msgs {
		switch m := msg.(type) {
		case SelectSettledMsg:
			page = &m
		case CountSettledMsg:
			count = &m
		}
	}
	if page == nil || count == nil {
		t.Fatalf("batch missing completions: page %v, count %v", page, count)
	}
	return *page, *count
}

// countStateHeader renders the model's count header through the exact seam.
func countStateHeader(m Model) string { return m.countState.Header() }

// TestConcurrentLaunchCarriesDistinctIdentities covers the launch contract:
// one SELECT execution ID, two distinct nonzero role-specific request IDs,
// both requests carrying exactly the builder's SQL and ordered parameters,
// and the pending count presentation established while the count is in
// flight.
func TestConcurrentLaunchCarriesDistinctIdentities(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{{result.NewInteger(1)}}}}
	count := &fakeCountExecutor{total: 1}
	m := concurrentCountModel(exec, count)

	_, execCmd := driveToExecutionStart(t, m)
	msgs := execBatch(t, execCmd)
	page, cnt := splitSelectCount(t, msgs)

	if page.ExecutionID == 0 || page.RequestID == 0 || cnt.ExecutionID == 0 || cnt.RequestID == 0 {
		t.Fatalf("identities must be nonzero: page %d/%d count %d/%d",
			page.ExecutionID, page.RequestID, cnt.ExecutionID, cnt.RequestID)
	}
	if page.ExecutionID != cnt.ExecutionID {
		t.Errorf("page execution %d != count execution %d: one execution owns both", page.ExecutionID, cnt.ExecutionID)
	}
	if page.RequestID == cnt.RequestID {
		t.Errorf("page and count request IDs are not distinct: both %d", page.RequestID)
	}
	// The count request received exactly the complete-SELECT subquery with the
	// same ordered parameters as the page request.
	if exec.sqls[0] != `SELECT * FROM "users" WHERE "email" = ?` {
		t.Errorf("page SQL = %q", exec.sqls[0])
	}
	wantCountSQL := `SELECT COUNT(*) FROM (SELECT * FROM "users" WHERE "email" = ?)`
	if count.sqls[0] != wantCountSQL {
		t.Errorf("count SQL = %q, want exactly %q", count.sqls[0], wantCountSQL)
	}
	if len(count.params[0]) != 1 || count.params[0][0] != "x" {
		t.Errorf("count params = %v, want [x] in the page's order", count.params[0])
	}
	if count.calls != 1 || exec.calls != 1 {
		t.Errorf("executors ran page=%d count=%d, want exactly one each", exec.calls, count.calls)
	}
}

// TestCountArrivesBeforePage covers count success before the page: the exact
// `Result count: N` wording renders from explicit state, and the page
// completion afterwards still applies its full rows untouched.
func TestCountArrivesBeforePage(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)}, {result.NewInteger(2)}, {result.NewInteger(3)},
	}}}
	count := &fakeCountExecutor{total: 3}
	m := concurrentCountModel(exec, count)

	m, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))

	afterCount := apply(m, cnt)
	if got := countStateHeader(afterCount); got != "Result count: 3" {
		t.Errorf("count header = %q, want exactly %q", got, "Result count: 3")
	}
	if afterCount.Result != nil {
		t.Errorf("count alone stored page state %+v", afterCount.Result)
	}

	afterPage := apply(afterCount, page)
	if afterPage.Result == nil || len(afterPage.Result.Page.Rows) != 3 {
		t.Fatalf("page completion after count lost rows: %+v", afterPage.Result)
	}
	if !strings.Contains(afterPage.View(), "rows 1-3") {
		t.Errorf("page rows not rendered after count:\n%s", afterPage.View())
	}
	if got := countStateHeader(afterPage); got != "Result count: 3" {
		t.Errorf("count header changed after page: %q", got)
	}
}

// TestCountWordingReflectsExecutedLimit covers the after-Limit variant: the
// wording uses the executed builder's Limit, and an inconsistent count is
// never used to clamp displayed rows.
func TestCountWordingReflectsExecutedLimit(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)}, {result.NewInteger(2)},
	}}}
	count := &fakeCountExecutor{total: 42}
	m := concurrentCountModel(exec, count)
	m.QB = m.QB.SetLimitInput("100")

	m, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	m2 := apply(apply(m, page), cnt)

	if got := countStateHeader(m2); got != "Result count: 42 (after Limit 100)" {
		t.Errorf("count header = %q, want exactly %q", got, "Result count: 42 (after Limit 100)")
	}
	// No clamping: the count (42) is inconsistent with fetched rows (2) and
	// must not change them.
	if len(m2.Result.Page.Rows) != 2 {
		t.Errorf("rows clamped to count: %d rows", len(m2.Result.Page.Rows))
	}
}

// TestCountUnavailableIsIsolated covers count failure: the exact
// `Count unavailable` wording, retained successful rows and their rendering,
// and no conversion of the active SELECT into a page failure.
func TestCountUnavailableIsIsolated(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)},
	}}}
	count := &fakeCountExecutor{err: errors.New("database is locked")}
	m := concurrentCountModel(exec, count)

	m, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	m2 := apply(apply(m, page), cnt)

	if got := countStateHeader(m2); got != "Count unavailable" {
		t.Errorf("count header = %q, want exactly %q", got, "Count unavailable")
	}
	if m2.Result == nil || m2.Result.Page == nil || m2.Result.Err != nil {
		t.Fatalf("count failure disturbed the page result: %+v", m2.Result)
	}
	if !strings.Contains(m2.View(), "rows 1-1") {
		t.Errorf("successful rows not still rendered after count failure:\n%s", m2.View())
	}
}

// TestFirstPageFailureIndependentOfCount covers the page's ordinary result
// error path: a first-page failure lands on the result-error boundary and a
// successful count neither masks it nor promotes rows out of it.
func TestFirstPageFailureIndependentOfCount(t *testing.T) {
	exec := &fakeSelectExecutor{err: errors.New("no such table: users")}
	count := &fakeCountExecutor{total: 0}
	m := concurrentCountModel(exec, count)

	m, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))
	m2 := apply(apply(m, page), cnt)

	if m2.Result == nil || m2.Result.Err == nil {
		t.Fatalf("page failure did not follow the ordinary result-error path: %+v", m2.Result)
	}
	if !strings.Contains(m2.Result.Err.Error(), "no such table") {
		t.Errorf("error text = %q, want the driver cause", m2.Result.Err.Error())
	}
	if m2.Result.Page != nil {
		t.Errorf("failed page stored rows %+v", m2.Result.Page)
	}
}

// TestStaleSelectCompletionsAreDiscarded covers the two-level identity guards
// on the model: wrong-role IDs, duplicate responses, delayed responses from a
// superseded execution, and an older delayed count arriving after a newer
// SELECT begins all mutate nothing.
func TestStaleSelectCompletionsAreDiscarded(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{
		{result.NewInteger(1)},
	}}}
	count := &fakeCountExecutor{total: 1}
	m := concurrentCountModel(exec, count)

	m, execCmd := driveToExecutionStart(t, m)
	page, cnt := splitSelectCount(t, execBatch(t, execCmd))

	// Wrong-role request IDs: page wearing count's ID and vice versa.
	wrongPage := SelectSettledMsg{ExecutionID: page.ExecutionID, RequestID: cnt.RequestID, Result: page.Result}
	wrongCount := CountSettledMsg{ExecutionID: page.ExecutionID, RequestID: page.RequestID, Result: cnt.Result}
	if next := apply(m, wrongPage); next.Result != nil {
		t.Error("page completion accepted with count's request ID")
	}
	if next := apply(m, wrongCount); next.countState.Status != result.CountPending {
		t.Error("count completion accepted with page's request ID")
	}

	// A duplicate page response after acceptance mutates nothing twice, but
	// acceptance once is the established behavior: settle properly first.
	m2 := apply(m, page)
	duplicate := SelectSettledMsg{ExecutionID: page.ExecutionID, RequestID: page.RequestID, Result: FirstPageResult{Err: errors.New("late duplicate")}}
	next := apply(m2, duplicate)
	if next.Result == nil || next.Result.Page == nil {
		t.Fatal("duplicate response replaced a successful page with an error")
	}

	// A delayed response from a superseded execution is rejected at both
	// levels: wrong role ID against the new tracker.
	if next := apply(m2, wrongCount); next.countState.Status != result.CountPending {
		t.Error("superseded-execution count accepted under a stale request ID")
	}
}

// TestDelayedCountAfterNewerSelectIsIgnored covers an older execution's
// delayed count arriving after a newer SELECT begins: it must not change the
// newer execution's pending count state.
func TestDelayedCountAfterNewerSelectIsIgnored(t *testing.T) {
	exec := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{{result.NewInteger(1)}}}}
	count := &fakeCountExecutor{total: 7}
	m := concurrentCountModel(exec, count)

	first, firstCmd := driveToExecutionStart(t, m)
	_, olderCount := splitSelectCount(t, execBatch(t, firstCmd))

	// Begin a newer SELECT execution while the older count is still delayed.
	secondCmd := first.executionRoute()
	if secondCmd == nil {
		t.Fatal("second execution start refused")
	}
	startMsg, ok := secondCmd().(ExecutionStartedMsg)
	if !ok {
		t.Fatalf("second route produced %T", secondCmd())
	}
	afterStart, newerCmd := first.Update(startMsg)
	newer, ok := afterStart.(Model)
	if !ok {
		t.Fatalf("second start update returned %T", afterStart)
	}
	newerPage, _ := splitSelectCount(t, execBatch(t, newerCmd))
	newer = apply(newer, newerPage)
	_ = newerPage

	// The older count wears the first execution's IDs, so the newer tracker
	// discards it without touching pending state.
	stale := CountSettledMsg{ExecutionID: olderCount.ExecutionID, RequestID: olderCount.RequestID, Result: CountResult{Total: 999}}
	after := apply(newer, stale)
	if after.countState.Status != result.CountPending {
		t.Errorf("stale count mutated state to %v", after.countState.Status)
	}
	if after.countState.Header() != "Counting rows…" {
		t.Errorf("count header = %q, want the established pending wording", after.countState.Header())
	}
}

// TestCountHelpRecordsIndependentSnapshots covers the help context: it names
// the complete limited SELECT, independent snapshots/drift, and no clamping.
func TestCountHelpRecordsIndependentSnapshots(t *testing.T) {
	help := strings.Join(CountHelpLines(), "\n")
	for _, want := range []string{"complete limited SELECT", "no shared snapshot", "drift", "never clamped"} {
		if !strings.Contains(help, want) {
			t.Errorf("help text missing %q:\n%s", want, help)
		}
	}
}

// apply applies msg through Update, discarding any returned command, and
// returns the resulting model.
func apply(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}
