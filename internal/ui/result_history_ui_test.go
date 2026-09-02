// Result-history UI and query-error recovery tests (Issue #36 Task 3), per
// the Execution and Result Lifecycle, History Module Design, and history
// Testing Decisions in Notes/PRD-sqloid.md. Ctrl+E/Y enter and traverse
// result history with zero database work; an actual execution exits history
// before finalization; an ordinary query error (including the five-second
// `database is locked` busy timeout) finalizes one lifecycle-defined error
// entry that replaces the visible result area; Esc dismisses only the visible
// error while older entries stay reachable; and an authoritative terminal
// health classification overrides the lock error.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// sendKeys applies key messages through Update and returns the final model.
func sendKeys(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// seededBrowseModel builds a validated-select-free model with three finalized
// entries (tabular, tabular, error) at a known size, wired with counting
// executors.
func seededBrowseModel(t *testing.T, execs *countingExecutors) Model {
	t.Helper()
	return browseModel(t, execs, []history.ResultEntry{
		browseEntry(1, 1, 12),
		browseEntry(2, 1, 8),
		browseErrorEntry(3, "no such table: gone"),
	})
}

// TestResultHistoryKeysEnterAndTraverse walks the key seam: Ctrl+E and Ctrl+Y
// enter result history at the newest entry, Ctrl+E steps older, Ctrl+Y steps
// newer with the newest boundary exiting, boundaries are no-ops, Esc leaves
// result history, and no database work happens anywhere.
func TestResultHistoryKeysEnterAndTraverse(t *testing.T) {
	execs := &countingExecutors{}
	m := seededBrowseModel(t, execs)

	// Ctrl+E enters at the newest entry (the error snapshot).
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if !m.resultHistoryMode {
		t.Fatal("ctrl+e did not enter result-history mode")
	}
	if m.resultHistoryCursorID != 3 || m.resultHistoryView == nil || m.resultHistoryView.Err == nil {
		t.Fatalf("entry selection wrong: id=%d view=%+v", m.resultHistoryCursorID, m.resultHistoryView)
	}
	requireZeroRequests(t, execs, nil, "ctrl+e entry")

	// Ctrl+E steps older through both tabular snapshots.
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if m.resultHistoryCursorID != 2 || m.resultHistoryView == nil || m.resultHistoryView.Page == nil {
		t.Fatalf("ctrl+e did not reach the older tabular snapshot: id=%d view=%+v", m.resultHistoryCursorID, m.resultHistoryView)
	}
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if m.resultHistoryCursorID != 1 {
		t.Fatalf("second ctrl+e cursor = %d, want 1", m.resultHistoryCursorID)
	}
	// Boundary: further Ctrl+E is a deterministic no-op.
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if m.resultHistoryCursorID != 1 {
		t.Fatalf("older boundary moved the cursor: %d", m.resultHistoryCursorID)
	}
	// Ctrl+Y steps newer; at the newest entry it exits the mode.
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlY))
	if m.resultHistoryCursorID != 2 || m.resultHistoryNotice != "" {
		t.Fatalf("ctrl+y step wrong: id=%d notice=%q", m.resultHistoryCursorID, m.resultHistoryNotice)
	}
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlY))
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlY))
	if m.resultHistoryMode {
		t.Fatal("ctrl+y at the newest entry did not exit result history")
	}
	requireZeroRequests(t, execs, nil, "traversal")

	// Esc inside result history returns to the base builder/result context.
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE), tea.KeyMsg{Type: tea.KeyEsc})
	if m.resultHistoryMode || m.resultHistoryView != nil {
		t.Fatal("esc did not leave result history cleanly")
	}
	// All three entries stay reachable afterwards.
	if m.ResultHistory.Len() != 3 {
		t.Fatalf("history retained %d entries, want 3", m.ResultHistory.Len())
	}
	requireZeroRequests(t, execs, nil, "esc exit")
}

// TestExecutionStartExitsResultHistory proves an actual execution first exits
// result-history mode — clearing the historical selection and stale displayed
// rows — and only then lets the Issue #34 finalization of the new execution
// proceed exactly once.
func TestExecutionStartExitsResultHistory(t *testing.T) {
	execs := &countingExecutors{}
	m := seededBrowseModel(t, execs)
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if !m.resultHistoryMode {
		t.Fatal("fixture did not enter result history")
	}

	m = sendKeys(t, m, ExecutionStartedMsg{})
	if m.resultHistoryMode || m.resultHistoryView != nil || m.resultHistoryCursorID != 0 {
		t.Fatalf("execution start did not clear the result-history selection: mode=%v view=%+v",
			m.resultHistoryMode, m.resultHistoryView)
	}
	if !m.SelectIsActive() {
		t.Fatal("new execution did not become the active SELECT")
	}
	requireZeroRequests(t, execs, nil, "history exit before execution")
}

// firstPageLockErrorMsg builds a settled first-page failure with the exact
// busy-timeout wording.
func firstPageLockErrorMsg() SelectSettledMsg {
	return SelectSettledMsg{
		ExecutionID: 9, RequestID: 1, Generation: 0,
		Result: FirstPageResult{Err: errors.New("database is locked")},
	}
}

var _ = firstPageLockErrorMsg

// TestQueryErrorReplacesResultAndDismisses covers ordinary query-error
// recovery: the first-page failure finalizes one error entry that becomes the
// newest result and replaces the visible result area; Esc dismisses only the
// displayed error without deleting history; older successful entries remain
// reachable through Ctrl+E/Y; no stale selected rows survive.
func TestQueryErrorReplacesResultAndDismisses(t *testing.T) {
	execs := &countingExecutors{}
	m := twoVersionSelectModel()
	m.ResultHistory = history.NewResultStore()
	// A far-from-zero execution identity so the seeded snapshot can never
	// collide with the global execution counter's fresh execution ID.
	if _, ok := m.ResultHistory.AppendFinalized(browseEntry(1<<32, 1, 5)); !ok {
		t.Fatal("seeding result history failed")
	}
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = resized.(Model)

	// A fresh execution whose first page fails with an ordinary error.
	lockExec := &fakeSelectExecutor{err: errors.New("database is locked")}
	m.Select = lockExec.selectPage
	started, execCmd := driveToExecutionStart(t, m)
	m = started
	if execCmd == nil {
		t.Fatal("execution start issued no command")
	}
	pageMsg, _ := m.Update(execCmd())
	m = pageMsg.(Model)

	// The finalized error entry became the newest result and replaced the
	// visible result area; no stale selected rows survive.
	if m.ResultHistory.Len() != 2 {
		t.Fatalf("history = %d entries, want the old entry plus one error entry", m.ResultHistory.Len())
	}
	newest, _ := m.ResultHistory.Newest()
	if newest.Kind != history.KindError || newest.Reason != "database is locked" || len(newest.Rows) != 0 {
		t.Fatalf("newest entry is not the finalized error: kind=%v reason=%q", newest.Kind, newest.Reason)
	}
	if m.Result == nil || m.Result.Err == nil || m.Result.Err.Error() != "database is locked" {
		t.Fatalf("visible result area not replaced by the error: %+v", m.Result)
	}
	if m.resultHistoryMode || m.resultHistoryView != nil {
		t.Fatal("stale result-history selection survived execution/error")
	}
	if m.terminalState != TerminalNone {
		t.Fatalf("ordinary lock error entered a terminal state: %v", m.terminalState)
	}

	// Esc dismisses only the displayed error; history is intact and older
	// successful entries remain reachable.
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Result != nil {
		t.Fatalf("esc did not dismiss the displayed error: %+v", m.Result)
	}
	if m.ResultHistory.Len() != 2 {
		t.Fatal("esc deleted retained history")
	}
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if !m.resultHistoryMode || m.resultHistoryCursorID != 2 {
		t.Fatalf("ctrl+e after dismissal did not reach the newest (error) entry: %+v", m.resultHistoryView)
	}
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	if m.resultHistoryCursorID != 1 || m.resultHistoryView == nil || m.resultHistoryView.Page == nil {
		t.Fatal("older successful entry not reachable through ctrl+e")
	}
	requireZeroRequests(t, execs, nil, "error recovery browsing")
}

// TestLaterPageErrorFinalizesFailedSnapshot covers a later execution error
// after retained rows: the recorded ending finalizes into a tabular failed
// snapshot preserving the captured rows, reachable as the newest entry.
func TestLaterPageErrorFinalizesFailedSnapshot(t *testing.T) {
	m, _, page, _ := fixtureFor(t, activeState{name: "page pending", pagePending: true})
	page.Result = FirstPageResult{Err: errors.New("disk I/O error")}
	m = apply(m, page)
	m.enterResultHistory()
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("history = %d entries, want exactly one", m.ResultHistory.Len())
	}
	entry := m.ResultHistory.Entries()[0]
	if entry.Kind != history.KindTabular || entry.Metadata.Outcome != history.OutcomeFailed ||
		entry.Metadata.Reason != "disk I/O error" || len(entry.Rows) != defaultPageRows {
		t.Fatalf("later-page failure snapshot wrong: kind=%v outcome=%v rows=%d",
			entry.Kind, entry.Metadata.Outcome, len(entry.Rows))
	}
}

// TestDatabaseIsLockedIsOrdinaryQueryError proves a request that exceeded the
// five-second busy timeout with `database is locked` is classified as an
// ordinary query error — one finalized error entry, the ordinary result-error
// boundary, and no terminal state.
func TestDatabaseIsLockedIsOrdinaryQueryError(t *testing.T) {
	m, page, _ := startActiveSelect(t)
	execID := m.ActiveSelectExecutionID()
	page.Result = FirstPageResult{Err: errors.New("database is locked")}
	ended := apply(m, page)
	requireFinalized(t, ended, execID, "database is locked failure")
	entry := requireEntryAt(t, ended, history.KindError, "database is locked failure")
	if entry.Metadata.Reason != "database is locked" {
		t.Fatalf("reason = %q, want the exact busy-timeout cause", entry.Metadata.Reason)
	}
	if ended.terminalState != TerminalNone {
		t.Fatalf("lock failure entered terminal state %v", ended.terminalState)
	}
	view := ended.View()
	if !strings.Contains(view, "database is locked") || strings.Contains(view, DeletedSessionEndedMessage) {
		t.Fatalf("locked failure must render the ordinary error boundary, got:\n%s", view)
	}
}

// TestTerminalHealthOverridesLockError proves that where an authoritative
// health classification is present — path deletion/replacement — the terminal
// state overrides the lock/query error: the terminal message replaces the
// result area and no lock-error rendering survives.
func TestTerminalHealthOverridesLockError(t *testing.T) {
	m, page, _ := startActiveSelect(t)
	m.terminalState = TerminalDeleted
	page.Result = FirstPageResult{Err: errors.New("database is locked")}
	ended := apply(m, page)
	if ended.terminalState == TerminalNone {
		t.Fatal("terminal health classification was lost")
	}
	view := ended.View()
	if strings.Contains(view, "database is locked") {
		t.Fatalf("lock error leaked past the terminal override:\n%s", view)
	}
	if !strings.Contains(view, DeletedSessionEndedMessage) {
		t.Fatalf("terminal message not rendered over the lock error:\n%s", view)
	}
}
