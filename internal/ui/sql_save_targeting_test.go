// Ctrl+S save-targeting model coverage for Issue #48, per the Query save
// targeting decision in Notes/PRD-sqloid.md. Ordinary states resolve in
// exact viewed-result-query, runnable-builder, last-execution priority, with
// the viewed result's query obtained through its backing immutable history
// entry (never visible text); terminal states use only the Ctrl+P/N-selected
// immutable query, then the last actual execution. A failed resolution shows
// exactly `no runnable query to save`, opens no picker, and serializes
// nothing. Every resolution path issues zero validation, schema, connection,
// or database work.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// saveModel builds an ordinary-state model with counting fake database seams
// wired, so every Ctrl+S press can be proven database-free.
func saveModel(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
	t.Helper()
	m := modelWithQB(qb.NewQuery())
	sel := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 1}
	page := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m.Select = sel.selectPage
	m.Count = count.count
	m.Page = page.page
	m.Refresher = refresh
	return m, sel, count, page, refresh
}

// requireSaveZeroDatabaseWork asserts no seam issued a request and no
// validation workflow opened during save targeting.
func requireSaveZeroDatabaseWork(t *testing.T, sel *fakeSelectExecutor, count *fakeCountExecutor, page *fakePageExecutor, refresh *fakeRefresher, context string) {
	t.Helper()
	if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 {
		t.Fatalf("%s started database work: select=%d count=%d page=%d refresh=%d",
			context, sel.calls, count.calls, page.issued, refresh.calls)
	}
}

// TestOrdinarySaveTargetingPriority walks every pairwise and all-present
// combination of viewed-result query, runnable builder, and last execution,
// plus absent and non-runnable builders, proving the exact ordinary priority
// and that no press ever touches a database seam.
func TestOrdinarySaveTargetingPriority(t *testing.T) {
	lastState := qb.HistoryState{Command: qb.CommandDelete, Table: "last_table", TableSet: true}

	for _, tc := range []struct {
		name       string
		viewed     bool
		builder    bool
		runnable   bool
		last       bool
		wantSource export.SQLSaveSource
	}{
		{name: "all present chooses viewed result query", viewed: true, builder: true, runnable: true, last: true, wantSource: export.SaveFromViewedResult},
		{name: "viewed beats runnable builder", viewed: true, builder: true, runnable: true, wantSource: export.SaveFromViewedResult},
		{name: "viewed beats last execution", viewed: true, last: true, wantSource: export.SaveFromViewedResult},
		{name: "viewed alone", viewed: true, wantSource: export.SaveFromViewedResult},
		{name: "runnable builder beats last execution", builder: true, runnable: true, last: true, wantSource: export.SaveFromRunnableBuilder},
		{name: "runnable builder alone", builder: true, runnable: true, wantSource: export.SaveFromRunnableBuilder},
		{name: "non-runnable builder falls to last execution", builder: true, last: true, wantSource: export.SaveFromLastExecution},
		{name: "last execution alone", last: true, wantSource: export.SaveFromLastExecution},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := history.NewStore()
			viewedID := store.Append(viewedState())
			lastID := store.Append(lastState)

			m, sel, count, page, refresh := saveModel(t)
			m.History = store
			m.ResultHistory = history.NewResultStore()
			if tc.last {
				m.lastExecQueryEntryID = lastID
			}
			if tc.viewed {
				retained, ok := m.ResultHistory.AppendFinalized(history.ResultEntry{
					ExecutionID:  11,
					Kind:         history.KindTabular,
					Columns:      []string{"id"},
					QueryEntryID: viewedID,
				})
				if !ok {
					t.Fatal("setup: result entry rejected")
				}
				m.resultHistoryMode = true
				m.resultHistoryCursorID = retained.ID
				m.resultHistoryView = projectHistoryEntry(retained, 5)
			}
			if tc.builder {
				q := qb.NewQuery().RefreshSchema(whereUICatalog()).
					SelectCommand(qb.CommandSelect).SelectTable("users")
				if tc.runnable {
					q = q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
				}
				m.applyBuilder(q)
			}

			nm, _ := pressKey(m, ctrlKey(tea.KeyCtrlS))
			if nm.savePrepared == nil {
				t.Fatalf("no target prepared; notice %q", nm.saveNotice)
			}
			if nm.savePrepared.Source != tc.wantSource {
				t.Fatalf("source = %v, want %v", nm.savePrepared.Source, tc.wantSource)
			}
			switch tc.wantSource {
			case export.SaveFromViewedResult:
				if !nm.savePrepared.State.Equal(wantState(t, store, viewedID)) {
					t.Fatalf("target state = %+v, want the viewed result's query", nm.savePrepared.State)
				}
			case export.SaveFromLastExecution:
				if !nm.savePrepared.State.Equal(wantState(t, store, lastID)) {
					t.Fatalf("target state = %+v, want the last execution", nm.savePrepared.State)
				}
			}
			if nm.saveNotice != "" {
				t.Fatalf("unexpected save feedback %q", nm.saveNotice)
			}
			requireSaveZeroDatabaseWork(t, sel, count, page, refresh, "Ctrl+S target resolution")
		})
	}
}

// viewedState returns one immutable viewed-result query state.
func viewedState() qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandSelect, Table: "viewed_table", TableSet: true}
}

// lastState returns one distinct last-execution history state.
func lastState() qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandDelete, Table: "last_table", TableSet: true}
}

// wantState returns the retained immutable state behind an entry ID.
func wantState(t *testing.T, store *history.Store, id history.EntryID) qb.HistoryState {
	t.Helper()
	e, ok := store.Lookup(id)
	if !ok {
		t.Fatalf("setup: entry %d not retained", id)
	}
	return e.State
}

// TestOrdinarySaveNoTargetFeedback requires the exact inline feedback, no
// picker, and no prepared target when nothing exists to save.
func TestOrdinarySaveNoTargetFeedback(t *testing.T) {
	m, sel, count, page, refresh := saveModel(t)
	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlS))
	if cmd != nil {
		t.Fatal("failed Ctrl+S resolution issued a command")
	}
	if nm.saveNotice != NoRunnableQueryFeedback {
		t.Fatalf("save feedback = %q, want exactly %q", nm.saveNotice, NoRunnableQueryFeedback)
	}
	if nm.savePrepared != nil {
		t.Fatal("failed Ctrl+S prepared a picker target")
	}
	if nm.Popup != nil {
		t.Fatal("failed Ctrl+S opened a picker")
	}
	if !strings.Contains(nm.View(), NoRunnableQueryFeedback) {
		t.Fatalf("feedback not rendered inline:\n%s", nm.View())
	}
	requireSaveZeroDatabaseWork(t, sel, count, page, refresh, "no-target Ctrl+S")
}

// TestOrdinarySaveViewedResultAssociation proves the viewed-result query
// comes from the result entry's backing immutable history entry, not from
// any rendered text: one real executed SELECT finalizes a snapshot whose
// stable association resolves the exact executed query state.
func TestOrdinarySaveViewedResultAssociation(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 3}
	page := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m := firstSelectModel(exec)
	m.Select = exec.selectPage
	m.Count = count.count
	m.Page = page.page
	m.Refresher = refresh

	execModel, execCmd := driveToExecutionStart(t, m)
	next, _ := execModel.Update(execCmd())
	m = next.(Model)
	// Finalize the execution by entering result history, then view it.
	m.enterResultHistoryMode()
	if !m.resultHistoryMode {
		t.Fatal("setup: result history mode did not engage")
	}
	if m.savePrepared != nil {
		t.Fatal("no Ctrl+S press must not prepare a target")
	}

	nm, _ := pressKey(m, ctrlKey(tea.KeyCtrlS))
	if nm.savePrepared == nil {
		t.Fatalf("viewed result's query not targeted: notice %q", nm.saveNotice)
	}
	if nm.savePrepared.Source != export.SaveFromViewedResult {
		t.Fatalf("source = %v, want viewed-result-query", nm.savePrepared.Source)
	}
	// The target is the executed query's immutable history state — the exact
	// normalized snapshot the execution started with, never a text rebuild.
	if !nm.savePrepared.State.Equal(validSelectQB().HistoryState()) {
		t.Fatalf("target = %+v, want the executed SELECT's immutable state", nm.savePrepared.State)
	}
	// The viewed result's snapshot carries the stable association.
	viewed := wantNewestResult(t, nm)
	if viewed.QueryEntryID == 0 {
		t.Fatal("finalized snapshot carries no query-history association")
	}
}

// wantNewestResult returns the newest retained result entry.
func wantNewestResult(t *testing.T, m Model) history.ResultEntry {
	t.Helper()
	e, ok := m.ResultHistory.Newest()
	if !ok {
		t.Fatal("setup: no result entries retained")
	}
	return e
}

// TestSaveBlockedDuringInFlightStaysExact re-checks that the in-flight gate
// still owns Ctrl+S during requests: the resolution seam is never reached.
func TestSaveBlockedDuringInFlight(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	nm, _ := pressKey(m, ctrlKey(tea.KeyCtrlS))
	if nm.inFlightNotice != SaveBlockedFeedback || nm.savePrepared != nil {
		t.Fatalf("in-flight gate leaked Ctrl+S resolution: %q %v", nm.saveNotice, nm.savePrepared)
	}
}

// TestTerminalSaveTargetingOutcomeUnknown drives the outcome-unknown
// terminal: selected-query priority, then last-execution fallback, with the
// builder and viewed-result candidates ignored and zero database work.
func TestTerminalSaveTargetingOutcomeUnknown(t *testing.T) {
	m, fake := updateUnresolvedUpdate(t)
	m.catalog = prepCatalog() // the prep fixtures build QBs against this catalog
	if m.terminalState != TerminalOutcomeUnknown {
		t.Fatal("setup: outcome-unknown terminal not entered")
	}
	sel := &fakeSelectExecutor{}
	count := &fakeCountExecutor{total: 1}
	page := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m.Select = sel.selectPage
	m.Count = count.count
	m.Page = page.page
	m.Refresher = refresh

	// With no Ctrl+P/N selection the last actual execution — the resolved
	// write's own query-history entry — is the terminal target.
	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlS))
	if cmd == nil || !nm.pickerOpen {
		t.Fatal("terminal Ctrl+S did not open the destination picker")
	}
	// Issue #52: cancel the picker to continue scripted targeting; exact
	// restoration keeps the prepared target.
	nm, _ = pressKey(nm, tea.KeyMsg{Type: tea.KeyEscape})
	if nm.pickerOpen {
		t.Fatal("Esc did not cancel the picker")
	}
	if nm.savePrepared == nil {
		t.Fatalf("terminal fallback missing: %q", nm.saveNotice)
	}
	if nm.savePrepared.Source != export.SaveFromLastExecution {
		t.Fatalf("source = %v, want last-execution", nm.savePrepared.Source)
	}
	// The executed write's query resolves through the outcome-unknown
	// entry's stable query-history association, proving the write recorded
	// its own backing entry at execution start.
	entry, ok := m.ResultHistory.Newest()
	if !ok || entry.QueryEntryID == 0 {
		t.Fatal("setup: outcome-unknown entry lacks its query association")
	}
	qe, ok := m.History.Lookup(entry.QueryEntryID)
	if !ok {
		t.Fatal("setup: write's query entry not retained")
	}
	want := qe.State
	if !nm.savePrepared.State.Equal(want) {
		t.Fatalf("target = %+v, want the executed write's immutable query state", nm.savePrepared.State)
	}

	// Select a different immutable query with Ctrl+P: it becomes the only
	// target, deliberately overriding the builder and last execution.
	tm, _ := pressKey(nm, tea.KeyMsg{Type: tea.KeyCtrlP})
	if !tm.historyMode {
		t.Fatal("setup: Ctrl+P did not enter query history")
	}
	after, _ := pressKey(tm, ctrlKey(tea.KeyCtrlS))
	if after.savePrepared == nil || after.savePrepared.Source != export.SaveFromTerminalSelection {
		t.Fatalf("terminal selection not used: %+v %q", after.savePrepared, after.saveNotice)
	}
	requireSaveZeroDatabaseWork(t, sel, count, page, refresh, "outcome-unknown Ctrl+S")
	_ = fake
}

// TestTerminalSaveTargetingHealth covers the two health terminals: the
// selected immutable query wins; with history browsing left, the last actual
// execution falls back. Both presses start zero work.
func TestTerminalSaveTargetingHealth(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want TerminalState
	}{
		{name: "deleted", err: deletedHealthErr(), want: TerminalDeleted},
		{name: "replaced", err: replacedHealthErr(), want: TerminalReplaced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, sel, count, page, refresh := healthTerminalModel(t, tc.err, tc.want)
			// No selection: the last actual execution (the SELECT that ended
			// into this terminal) is the fallback target.
			nm, openCmd := pressKey(m, ctrlKey(tea.KeyCtrlS))
			if openCmd == nil || !nm.pickerOpen {
				t.Fatal("terminal Ctrl+S did not open the destination picker")
			}
			nm, _ = pressKey(nm, tea.KeyMsg{Type: tea.KeyEscape})
			if nm.savePrepared == nil || nm.savePrepared.Source != export.SaveFromLastExecution {
				t.Fatalf("terminal fallback = %+v (%q)", nm.savePrepared, nm.saveNotice)
			}
			// Select a query-history entry with Ctrl+P; it must now win.
			tm, _ := pressKey(nm, tea.KeyMsg{Type: tea.KeyCtrlP})
			if !tm.historyMode {
				t.Fatal("setup: Ctrl+P did not enter query history")
			}
			after, _ := pressKey(tm, ctrlKey(tea.KeyCtrlS))
			if after.savePrepared == nil || after.savePrepared.Source != export.SaveFromTerminalSelection {
				t.Fatalf("selected query not targeted: %+v", after.savePrepared)
			}
			requireSaveZeroDatabaseWork(t, sel, count, page, refresh, "health terminal Ctrl+S")
		})
	}
}

// TestTerminalSaveNoTargetFeedback requires the exact no-target feedback in
// a terminal state with neither a selected query nor a last execution, with
// no picker and no serialization.
func TestTerminalSaveNoTargetFeedback(t *testing.T) {
	m, fake := updateUnresolvedUpdate(t)
	m.catalog = prepCatalog()
	// Model a session whose resolved outcome left no last-execution query
	// entry to fall back to.
	m.lastExecQueryEntryID = 0
	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlS))
	if cmd != nil {
		t.Fatal("failed terminal save issued a command")
	}
	if nm.saveNotice != NoRunnableQueryFeedback {
		t.Fatalf("feedback = %q, want exact %q", nm.saveNotice, NoRunnableQueryFeedback)
	}
	if nm.savePrepared != nil || nm.Popup != nil {
		t.Fatal("failed terminal Ctrl+S opened a picker or prepared a target")
	}
	_ = fake
}
