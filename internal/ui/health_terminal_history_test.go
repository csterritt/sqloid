// Terminal history navigation coverage for Issue #46, per the History Module
// Design and Context/Action matrix decisions in Notes/PRD-sqloid.md. Entering
// either health terminal selects the newest stable-backed immutable result
// only when one exists; with an empty result history the selection stays
// empty and the exact terminal message remains the primary view — no
// synthetic entry, no absent stable ID, no stale columns/rows, and no
// missing-backed rendering. Ctrl+P/N navigate complete query-history states
// and Ctrl+E/Y navigate immutable results entirely in memory with
// deterministic boundaries; both terminal variants behave identically.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// healthTerminalModel enters one of the two health terminals through a typed
// first-page classification at the SELECT boundary, seeding immutable history
// stores beforehand. Every normally database-capable seam stays wired to a
// counting fake so navigation can be proven database-free.
func healthTerminalModel(t *testing.T, err error, want TerminalState) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
	t.Helper()
	exec := &fakeSelectExecutor{err: err}
	m := firstSelectModel(exec)
	m.ResultHistory.AppendFinalized(browseEntry(11, 1, 2))
	m.ResultHistory.AppendFinalized(browseEntry(12, 1, 1))

	m, execCmd := driveToExecutionStart(t, m)
	next, _ := m.Update(execCmd())
	m = next.(Model)
	if m.terminalState != want {
		t.Fatalf("setup: terminal state = %v, want %v", m.terminalState, want)
	}

	sel := &fakeSelectExecutor{page: threeRowFirstPage()}
	count := &fakeCountExecutor{total: 3}
	page := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m.Select = sel.selectPage
	m.Count = count.count
	m.Page = page.page
	m.Refresher = refresh
	return m, sel, count, page, refresh
}

// requireNoDatabaseWork asserts no seam issued a request.
func requireNoDatabaseWork(t *testing.T, sel *fakeSelectExecutor, count *fakeCountExecutor, page *fakePageExecutor, refresh *fakeRefresher, context string) {
	t.Helper()
	if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 {
		t.Fatalf("%s started database work: select=%d count=%d page=%d refresh=%d",
			context, sel.calls, count.calls, page.issued, refresh.calls)
	}
}

// TestHealthTerminalSelectsNewestResultOnEntry proves both terminal variants
// select the newest stable-backed result entry at entry.
func TestHealthTerminalSelectsNewestResultOnEntry(t *testing.T) {
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
			if !m.resultHistoryMode {
				t.Fatal("terminal entry did not select a result entry")
			}
			newest, ok := m.ResultHistory.Newest()
			if !ok || newest.ID != m.resultHistoryCursorID {
				t.Fatalf("selection = %v, want the newest entry %v", m.resultHistoryCursorID, newest.ID)
			}
			if m.resultHistoryView == nil || len(m.resultHistoryView.Page.Rows) == 0 {
				t.Fatal("newest entry projected no view")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "terminal entry")
		})
	}
}

// TestHealthTerminalResultNavigationBoundaries proves Ctrl+E/Y navigate the
// immutable entries older/newer with deterministic no-op boundaries in both
// terminal variants.
func TestHealthTerminalResultNavigationBoundaries(t *testing.T) {
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
			newest, _ := m.ResultHistory.Newest()
			older, ok := m.ResultHistory.OlderThan(newest.ID)
			if !ok {
				t.Fatal("setup: no older entry retained")
			}

			m = termKey(t, m, "ctrl+e")
			if m.resultHistoryCursorID != older.ID {
				t.Fatalf("ctrl+e cursor = %v, want older %v", m.resultHistoryCursorID, older.ID)
			}
			if got := termKey(t, m, "ctrl+e"); got.resultHistoryCursorID != older.ID {
				t.Error("ctrl+e crossed the oldest boundary")
			}
			m = termKey(t, m, "ctrl+y")
			if m.resultHistoryCursorID != newest.ID {
				t.Fatalf("ctrl+y cursor = %v, want back at the newest entry", m.resultHistoryCursorID)
			}
			if got := termKey(t, m, "ctrl+y"); got.resultHistoryCursorID != newest.ID || !got.resultHistoryMode {
				t.Error("ctrl+y at the newest boundary must be a no-op inside the terminal")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "result navigation")
		})
	}
}

// TestHealthTerminalQueryHistoryNavigation proves Ctrl+P/N navigate complete
// query-history states in memory with deterministic boundaries, preserving
// the selected result entry.
func TestHealthTerminalQueryHistoryNavigation(t *testing.T) {
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
			// Seed a second, distinct query state so navigation has two stops.
			m.History.Append(columnedSelectQB("id").HistoryState())
			newest, _ := m.History.Newest()
			older, ok := m.History.OlderThan(newest.ID)
			if !ok {
				t.Fatal("setup: no older query entry retained")
			}

			m = termKey(t, m, "ctrl+p")
			if !m.historyMode || m.historyCursorID != newest.ID {
				t.Fatalf("ctrl+p opened browsing at cursor=%v (mode=%v), want the newest entry", m.historyCursorID, m.historyMode)
			}
			m = termKey(t, m, "ctrl+p")
			if m.historyCursorID != older.ID {
				t.Fatalf("cursor = %v, want the older entry %v", m.historyCursorID, older.ID)
			}
			if got := termKey(t, m, "ctrl+p"); got.historyCursorID != older.ID {
				t.Error("ctrl+p crossed the oldest boundary")
			}
			m = termKey(t, m, "ctrl+n")
			if m.historyCursorID != newest.ID {
				t.Fatalf("cursor = %v, want back at the newest entry", m.historyCursorID)
			}
			if got := termKey(t, m, "ctrl+n"); got.historyMode {
				t.Error("ctrl+n at the newest boundary did not end browsing deterministically")
			}
			// The selected result entry survived query-history navigation.
			if !m.resultHistoryMode || m.resultHistoryCursorID == 0 {
				t.Fatal("result selection was lost during query navigation")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "query navigation")
		})
	}
}

// TestHealthTerminalEmptyResultHistoryFallback proves that with an empty
// result history the selection stays empty, Ctrl+E/Y are no-ops, and the
// exact terminal message remains the primary view — no synthetic entry, no
// absent stable ID, no stale columns or rows, and no missing-backed entry.
func TestHealthTerminalEmptyResultHistoryFallback(t *testing.T) {
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
			m.ResultHistory = history.NewResultStore()
			// Reset any projection left by entry selection.
			m.resultHistoryMode = false
			m.resultHistoryCursorID = 0
			m.resultHistoryView = nil

			wantMessage := DeletedSessionEndedMessage
			if tc.want == TerminalReplaced {
				wantMessage = ReplacedSessionEndedMessage
			}
			m = termKey(t, m, "ctrl+e")
			m = termKey(t, m, "ctrl+y")
			if m.resultHistoryMode || m.resultHistoryCursorID != 0 || m.resultHistoryView != nil {
				t.Fatalf("empty fallback selected an entry (mode=%v cursor=%v view=%v)",
					m.resultHistoryMode, m.resultHistoryCursorID, m.resultHistoryView)
			}
			if m.ResultHistory.Len() != 0 {
				t.Fatalf("empty fallback synthesized %d entries", m.ResultHistory.Len())
			}
			if view := m.View(); view != wantMessage {
				t.Fatalf("empty fallback view =\n%s\nwant exactly the primary terminal message", view)
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "empty result fallback")
		})
	}
}

// TestHealthTerminalEmptyQueryHistoryFallback proves Ctrl+P/N against an
// independently empty query history are deterministic no-ops.
func TestHealthTerminalEmptyQueryHistoryFallback(t *testing.T) {
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
			m.History = history.NewStore()
			m = termKey(t, m, "ctrl+p")
			if m.historyMode {
				t.Fatal("empty query history opened browsing via ctrl+p")
			}
			m = termKey(t, m, "ctrl+n")
			if m.historyMode {
				t.Fatal("empty query history opened browsing via ctrl+n")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "empty query fallback")
		})
	}
}

// TestHealthTerminalBothHistoriesEmptyPrimaryMessage proves that with both
// histories empty the terminal view stays exactly the primary message and
// every navigation key is a no-op — no stale columns/rows may render.
func TestHealthTerminalBothHistoriesEmptyPrimaryMessage(t *testing.T) {
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
			m.ResultHistory = history.NewResultStore()
			m.History = history.NewStore()
			m = termKey(t, m, "ctrl+e")
			m = termKey(t, m, "ctrl+y")
			m = termKey(t, m, "ctrl+p")
			m = termKey(t, m, "ctrl+n")
			m = termKey(t, m, "esc")

			wantMessage := DeletedSessionEndedMessage
			if tc.want == TerminalReplaced {
				wantMessage = ReplacedSessionEndedMessage
			}
			if view := m.View(); view != wantMessage {
				t.Fatalf("empty-histories view =\n%s\nwant exactly the primary terminal message", view)
			}
			if m.resultHistoryView != nil || m.resultHistoryCursorID != 0 {
				t.Error("stale projection survived the empty fallback")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "both empty")
		})
	}
}

// TestHealthTerminalResizeIssuesNoRequests proves entry, navigation, and
// resize produce no connection or database requests and keep the message.
func TestHealthTerminalResizeIssuesNoRequests(t *testing.T) {
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
			next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			if cmd != nil {
				t.Fatal("resize inside the terminal dispatched a command")
			}
			nm := next.(Model)
			if nm.terminalState != tc.want {
				t.Fatalf("resize left the terminal state for %v", nm.terminalState)
			}
			if !strings.Contains(nm.View(), DeletedSessionEndedMessage) && !strings.Contains(nm.View(), ReplacedSessionEndedMessage) {
				t.Fatal("resize lost the terminal message")
			}
			requireNoDatabaseWork(t, sel, count, page, refresh, "resize")
		})
	}
}
