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
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
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
}

func (f *fakeSelectExecutor) selectPage(ctx context.Context, sql string, params []any) FirstPageResult {
	f.calls++
	f.sqls = append(f.sqls, sql)
	f.params = append(f.params, params)
	if f.err != nil {
		return FirstPageResult{Err: f.err}
	}
	return FirstPageResult{Page: f.page}
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
