// Typed health-to-terminal mapping coverage for Issue #46, per the Session
// health and Context/Action matrix decisions in Notes/PRD-sqloid.md. Issue
// #7's typed deletion and same-path-replacement classifications — injected at
// every request boundary and during ordinary activity — enter the exact
// terminal states whose primary messages are `Database file no longer exists
// — session ended` and `Database file was replaced — session ended`. The
// strings are owned by internal/ui only: the health/connection layer carries
// no terminal copy, and typed classification (never error-text matching)
// selects the state. Terminal entry leaves no transaction or driver work
// pending, and from either state every database-capable key is suppressed
// before any command can be built.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/schema"
)

// deletedHealthErr and replacedHealthErr build the typed Issue #7 request
// boundary classifications for injection at every seam.
func deletedHealthErr() error {
	return &connection.HealthError{Path: "/tmp/sqloid.db", Kind: connection.HealthDeleted}
}

func replacedHealthErr() error {
	return &connection.HealthError{Path: "/tmp/sqloid.db", Kind: connection.HealthReplaced}
}

// applyExecutorCmd runs one executor command's message through Update, so
// tests drive the same settled seam the Bubble Tea runtime would deliver.
func applyExecutorCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("setup: no executor command to run")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

// assertHealthTerminal asserts the model sits in exactly one of the two
// health terminal states with the exact primary message rendered, no other
// database lifecycle pending, and all continuation blocked.
func assertHealthTerminal(t *testing.T, m Model, want TerminalState, context string) {
	t.Helper()
	if m.terminalState != want {
		t.Fatalf("%s: terminal state = %v, want %v", context, m.terminalState, want)
	}
	wantMessage := DeletedSessionEndedMessage
	other := ReplacedSessionEndedMessage
	if want == TerminalReplaced {
		wantMessage = ReplacedSessionEndedMessage
		other = DeletedSessionEndedMessage
	}
	view := m.View()
	if !strings.Contains(view, wantMessage) {
		t.Fatalf("%s: terminal view lacks the exact primary message:\n%s", context, view)
	}
	if strings.Contains(view, other) {
		t.Errorf("%s: terminal view leaked the other classification's message:\n%s", context, view)
	}
	if m.ContinuationBlocked() == false {
		t.Errorf("%s: terminal state did not block continuation", context)
	}
	// No transaction or driver work may remain pending on entry.
	if m.firstPagePending || m.countPendingFlag || m.pagePending || m.writePending ||
		m.validationPending || m.refreshPending || m.prepPending {
		t.Errorf("%s: database lifecycle state still pending on terminal entry", context)
	}
	if m.writePhases != nil || m.writeCancel != nil || m.CancelCommand != nil {
		t.Errorf("%s: pending cancellation handles survived terminal entry", context)
	}
}

// TestSelectBoundaryHealthErrorEntersDeletionTerminal proves a typed deletion
// classification at the first-page SELECT request boundary enters the exact
// deletion terminal with no pending work.
func TestSelectBoundaryHealthErrorEntersDeletionTerminal(t *testing.T) {
	exec := &fakeSelectExecutor{err: deletedHealthErr()}
	m := firstSelectModel(exec)

	m, execCmd := driveToExecutionStart(t, m)
	if execCmd == nil {
		t.Fatal("setup: execution start produced no executor command")
	}
	end := applyExecutorCmd(t, m, execCmd)
	assertHealthTerminal(t, end, TerminalDeleted, "select first-page deletion")
	if exec.calls != 1 {
		t.Errorf("executor calls = %d, want exactly 1", exec.calls)
	}
}

// TestSelectBoundaryHealthErrorEntersReplacementTerminal mirrors deletion
// through the replacement classification at the same boundary.
func TestSelectBoundaryHealthErrorEntersReplacementTerminal(t *testing.T) {
	exec := &fakeSelectExecutor{err: replacedHealthErr()}
	m := firstSelectModel(exec)

	m, execCmd := driveToExecutionStart(t, m)
	end := applyExecutorCmd(t, m, execCmd)
	assertHealthTerminal(t, end, TerminalReplaced, "select first-page replacement")
}

// TestCountBoundaryHealthErrorDuringOrdinaryActivity proves a typed
// classification on the independent count request — an ordinary-activity
// boundary after a settled first page — ends the session in the exact
// terminal state.
func TestCountBoundaryHealthErrorDuringOrdinaryActivity(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want TerminalState
	}{
		{name: "deleted", err: deletedHealthErr(), want: TerminalDeleted},
		{name: "replaced", err: replacedHealthErr(), want: TerminalReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, pageMsg, countMsg := startActiveSelect(t)
			m = apply(m, pageMsg)
			countMsg.Result = CountResult{Err: tc.err}
			end := apply(m, countMsg)
			assertHealthTerminal(t, end, tc.want, "count boundary "+tc.name)
		})
	}
}

// TestPageBoundaryHealthErrorDuringOrdinaryActivity proves a typed
// classification on a later page request during ordinary paging activity
// enters the exact terminal state.
func TestPageBoundaryHealthErrorDuringOrdinaryActivity(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want TerminalState
	}{
		{name: "deleted", err: deletedHealthErr(), want: TerminalDeleted},
		{name: "replaced", err: replacedHealthErr(), want: TerminalReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, pageNext, _ := fixtureFor(t, activeState{name: "page pending", pagePending: true})
			pageNext.Result = FirstPageResult{Err: tc.err}
			end := apply(m, pageNext)
			assertHealthTerminal(t, end, tc.want, "page boundary "+tc.name)
		})
	}
}

// TestEstimateBoundaryHealthError proves a typed classification on the
// destructive-estimate request enters the exact terminal state instead of an
// estimate failure.
func TestEstimateBoundaryHealthError(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m, cmd := openPreparation(t, prepUpdateQB(true), est)
	msg, ok := cmd().(EstimateSettledMsg)
	if !ok {
		t.Fatalf("estimate command produced %T, want EstimateSettledMsg", cmd())
	}
	// The estimate request boundary classifies deletion; inject the typed
	// classification the boundary would map onto the settled result.
	msg.Result = EstimateResult{Err: deletedHealthErr()}
	end := apply(m, msg)
	assertHealthTerminal(t, end, TerminalDeleted, "estimate boundary")
}

// TestWriteBoundaryHealthError proves the typed health classification carried
// by a settled write ends the session in the exact terminal state with no
// write lifecycle pending.
func TestWriteBoundaryHealthError(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting},
		result: connection.WriteResult{Outcome: connection.WriteFailed, Err: deletedHealthErr(), Health: &connection.HealthError{Path: "/tmp/db", Kind: connection.HealthDeleted}},
	}
	m.Write = fake.Write
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("setup: confirmation produced no command")
	}
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("setup: confirmation command produced %T", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	nm := dispatchWriteBatch(t, next.(Model), cmd, false)
	settled, _ := nm.Update(WriteSettledMsg{Execution: fake.execution, Result: fake.result})
	end := settled.(Model)
	assertHealthTerminal(t, end, TerminalDeleted, "write boundary")
	if end.ResultHistory.Len() != 0 {
		t.Errorf("health-failed write appended %d entries, want none", end.ResultHistory.Len())
	}
}

// TestValidationBoundaryHealthClassification proves the typed version-read
// classifications (mapped by the schema boundary from Issue #7 kinds) enter
// the exact terminal states during pre-execution validation.
func TestValidationBoundaryHealthClassification(t *testing.T) {
	cases := []struct {
		name   string
		reader *fakeVersionReader
		want   TerminalState
	}{
		{name: "deleted", reader: &fakeVersionReader{queued: []schema.VersionAttempt{versionDeleted()}}, want: TerminalDeleted},
		{name: "replaced", reader: &fakeVersionReader{queued: []schema.VersionAttempt{versionReplaced()}}, want: TerminalReplaced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := selectModel(tc.reader, &fakeRefresher{})
			opened, cmd := enterRunnable(m)
			settled, _ := opened.Update(cmd())
			end := settled.(Model)
			assertHealthTerminal(t, end, tc.want, "validation boundary "+tc.name)
		})
	}
}

// TestRefreshBoundaryHealthClassification proves the typed refresh
// classification at the Table-popup open boundary enters the exact terminal
// states.
func TestRefreshBoundaryHealthClassification(t *testing.T) {
	cases := []struct {
		name string
		kind connection.HealthKind
		want TerminalState
	}{
		{name: "deleted", kind: connection.HealthDeleted, want: TerminalDeleted},
		{name: "replaced", kind: connection.HealthReplaced, want: TerminalReplaced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := schema.RefreshDeleted
			if tc.kind == connection.HealthReplaced {
				status = schema.RefreshReplaced
			}
			end := terminalFixture(t, status)
			assertHealthTerminal(t, end, tc.want, "refresh boundary "+tc.name)
		})
	}
}

// TestHealthTerminalIsTypedNotErrorText proves the terminal states are
// selected by typed classification only: a plain error whose text is exactly
// the terminal UI message never enters a terminal state.
func TestHealthTerminalIsTypedNotErrorText(t *testing.T) {
	exec := &fakeSelectExecutor{err: errors.New(DeletedSessionEndedMessage)}
	m := firstSelectModel(exec)

	m, execCmd := driveToExecutionStart(t, m)
	end := applyExecutorCmd(t, m, execCmd)
	if end.terminalState != TerminalNone {
		t.Fatalf("error-text decoy entered terminal state %v", end.terminalState)
	}
	// The ordinary failure path ran: the SELECT finalized with exactly one
	// ordinary error entry carrying the decoy cause as a driver error —
	// classification never consumed the terminal UI text.
	if end.ResultHistory.Len() != 1 {
		t.Fatalf("ordinary failure finalized %d entries, want 1", end.ResultHistory.Len())
	}
	if e, ok := end.ResultHistory.Newest(); !ok || e.Kind != history.KindError {
		t.Fatalf("decoy failure produced kind=%v, want an ordinary error entry", e.Kind)
	}
}

// TestHealthTerminalStringsOwnedByUIOnly proves the exact terminal messages
// live only in internal/ui: the health/connection layer's typed diagnostic
// strings carry no session-ended copy.
func TestHealthTerminalStringsOwnedByUIOnly(t *testing.T) {
	if DeletedSessionEndedMessage != "Database file no longer exists — session ended" {
		t.Errorf("deleted terminal message changed: %q", DeletedSessionEndedMessage)
	}
	if ReplacedSessionEndedMessage != "Database file was replaced — session ended" {
		t.Errorf("replaced terminal message changed: %q", ReplacedSessionEndedMessage)
	}
	he := &connection.HealthError{Path: "/tmp/db", Kind: connection.HealthDeleted}
	if strings.Contains(he.Error(), "session ended") {
		t.Errorf("connection HealthError leaked terminal copy: %q", he.Error())
	}
	if strings.Contains(connection.HealthDeleted.String(), "session ended") ||
		strings.Contains(connection.HealthReplaced.String(), "session ended") {
		t.Error("connection HealthKind leaked terminal copy")
	}
}

// TestHealthTerminalForbidsDatabaseWork exercises every database-capable
// key/path from both terminal states and requires no validation, execution,
// paging, refresh, rerun, health request, or any other database command.
func TestHealthTerminalForbidsDatabaseWork(t *testing.T) {
	for _, state := range []struct {
		name string
		err  error
		want TerminalState
	}{
		{name: "deleted", err: deletedHealthErr(), want: TerminalDeleted},
		{name: "replaced", err: replacedHealthErr(), want: TerminalReplaced},
	} {
		t.Run(state.name, func(t *testing.T) {
			exec := &fakeSelectExecutor{err: state.err}
			m := firstSelectModel(exec)
			m, execCmd := driveToExecutionStart(t, m)
			m = applyExecutorCmd(t, m, execCmd)
			if m.terminalState != state.want {
				t.Fatalf("setup: terminal state = %v", m.terminalState)
			}

			// Wire every normally database-capable seam to counting fakes.
			sel := &fakeSelectExecutor{page: threeRowFirstPage()}
			count := &fakeCountExecutor{total: 3}
			page := &fakePageExecutor{rowsShown: 3}
			refresh := &fakeRefresher{queued: []schema.Attempt{successAttempt(prepCatalog())}}
			reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}
			est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
			m.Select = sel.selectPage
			m.Count = count.count
			m.Page = page.page
			m.Refresher = refresh
			m.VersionReader = reader
			m.Estimator = est.ExecuteEstimate

			// Enter, execution, paging, rerun, cancellation, help, history,
			// navigation, and dismissal keys — every normally database-capable
			// path — must all be consumed with no command at all.
			for _, k := range []string{"enter", "s", "u", "d", "i", "x", "r", "pgup", "pgdown", "ctrl+w", "ctrl+p", "ctrl+n", "ctrl+e", "ctrl+y", "?", "esc", "tab", "backspace"} {
				next := termKey(t, m, k)
				if next.terminalState != state.want {
					t.Fatalf("key %q left the terminal state for %v", k, next.terminalState)
				}
				m = next
			}
			if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 || reader.calls != 0 || est.requests != 0 {
				t.Fatalf("terminal state started database work: select=%d count=%d page=%d refresh=%d version=%d estimate=%d",
					sel.calls, count.calls, page.issued, refresh.calls, reader.calls, est.requests)
			}
			if m.Popup != nil || m.ValuePrompt != nil {
				t.Error("terminal state opened an overlay")
			}
			if m.inFlightNotice != "" || m.writePending {
				t.Error("terminal state shows in-flight feedback or pending work")
			}
		})
	}
}
