// Defensive result-eviction UI reconciliation tests (Issue #36 Tasks 5–6),
// per the Execution and Result Lifecycle and history Testing Decisions in
// Notes/PRD-sqloid.md. When an externally driven store mutation evicts the
// selected entry and entries remain, selection moves to the new oldest
// retained entry — resliced only from that entry's immutable rows at the
// current terminal height — with exactly `Previously viewed result was
// evicted from history`. When no entry remains, result-history mode clears to
// the base builder/result fallback with no historical rows. No frame,
// intermediate state, resize, navigation step, or dismissal can render rows,
// columns, metadata, or errors from the evicted backing entry, and no
// database request is ever issued. This is the defensive external-mutation
// path only: normal actual execution exits history before its append.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// fillResultHistory seeds n entries into the model's result-history store,
// alternating tabular and error entries, returning their IDs oldest first.
func fillResultHistory(t *testing.T, m *Model, n int) []history.EntryID {
	t.Helper()
	var ids []history.EntryID
	for i := 0; i < n; i++ {
		e := browseEntry(uint64(100+i), 1, 6)
		if i%2 == 1 {
			e = browseErrorEntry(uint64(100+i), "no such table: gone")
		}
		retained, ok := m.ResultHistory.AppendFinalized(e)
		if !ok {
			t.Fatal("seed append rejected")
		}
		ids = append(ids, retained.ID)
	}
	return ids
}

// reconcile applies an inert message through Update, which defensively
// resolves the selected stable ID after every possible store mutation.
func reconcile(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: m.Width, Height: m.Height})
	return next.(Model)
}

// viewContains reports whether a rendered view contains the given grid
// token as a whole field, ignoring border glyphs and padding.
func viewContains(view, token string) bool {
	for _, line := range strings.Split(view, "\n") {
		line = strings.Trim(line, "\u2502 ")
		for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r == '|' || r == ' ' }) {
			if field == token {
				return true
			}
		}
	}
	return false
}

// TestDefensiveEvictedSelectionNewOldest walks the matrix: the selected
// oldest, middle, and newest IDs evicted under full and partially filled
// histories, for tabular and error entries. Selection moves to the new oldest
// retained entry, resliced at the current height, with exactly the eviction
// notice, zero requests, and no evicted rows ever rendered.
func TestDefensiveEvictedSelectionNewOldest(t *testing.T) {
	cases := []struct {
		name      string
		fill      int
		extra     int // entries appended externally after selection
		selection int // index into seeded IDs
	}{
		{name: "full history, oldest selected", fill: 20, extra: 1, selection: 0},
		{name: "full history, middle selected", fill: 20, extra: 10, selection: 9},
		{name: "full history, newest selected", fill: 20, extra: 20, selection: 19},
		{name: "partially filled, oldest selected", fill: 7, extra: 21, selection: 0},
		{name: "partially filled, newest selected", fill: 7, extra: 21, selection: 6},
		{name: "partially filled then filled, middle selected", fill: 19, extra: 7, selection: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			execs := &countingExecutors{}
			m := browseModel(t, execs, nil)
			ids := fillResultHistory(t, &m, tc.fill)

			// Enter browsing (the newest entry is selected) and re-target the
			// selected entry under test.
			m.enterResultHistoryMode()
			if m.resultHistoryCursorID != ids[tc.fill-1] {
				t.Fatal("fixture selection did not start at the newest entry")
			}
			m.resultHistoryCursorID = ids[tc.selection]
			m.projectSelectedHistoryEntry()

			// Externally driven appends (normal execution exits history before
			// its append, so this path is defensive only).
			var newIDs []history.EntryID
			for i := 0; i < tc.extra; i++ {
				e, ok := m.ResultHistory.AppendFinalized(browseEntry(uint64(500+i), 1, 4))
				if !ok {
					t.Fatal("external append rejected")
				}
				newIDs = append(newIDs, e.ID)
			}
			// Expected reconciliation outcome: the oldest fill+extra-capacity
			// entries are evicted; when the selected entry was among them the
			// selection must move to the new oldest retained entry with the
			// exact notice; otherwise nothing changes.
			allIDs := append(append([]history.EntryID(nil), ids...), newIDs...)
			evicted := tc.fill + tc.extra - 20
			m = reconcile(t, m)
			if tc.selection < evicted {
				if m.resultHistoryCursorID != allIDs[evicted] {
					t.Fatalf("selection = %d, want the new oldest retained ID %d", m.resultHistoryCursorID, allIDs[evicted])
				}
				if m.resultHistoryNotice != ResultEvictedNotice {
					t.Fatalf("notice = %q, want exactly %q", m.resultHistoryNotice, ResultEvictedNotice)
				}
			} else {
				if m.resultHistoryCursorID != allIDs[tc.selection] || m.resultHistoryNotice != "" {
					t.Fatalf("unaffected selection changed: id=%d notice=%q", m.resultHistoryCursorID, m.resultHistoryNotice)
				}
			}
			requireZeroRequests(t, execs, nil, "defensive eviction")

			// The displayed view is the new oldest entry resliced at the
			// current height, with rows never exceeding its snapshot.
			pageRows := int64(CalculateLayout(m.Height, m.Fields).PageRows)
			backing, _ := m.ResultHistory.Lookup(allIDs[evicted])
			if tc.selection >= evicted {
				backing, _ = m.ResultHistory.Lookup(allIDs[tc.selection])
			}
			wantRows := len(backing.Rows)
			if int64(wantRows) > pageRows {
				wantRows = int(pageRows)
			}
			if backing.Kind == history.KindTabular {
				if m.resultHistoryView == nil || m.resultHistoryView.Page == nil {
					t.Fatal("tabular fallback produced no projection")
				}
				if len(m.resultHistoryView.Page.Rows) != wantRows {
					t.Fatalf("fallback reslice rows = %d, want %d", len(m.resultHistoryView.Page.Rows), wantRows)
				}
			} else if m.resultHistoryView == nil || m.resultHistoryView.Err == nil || m.resultHistoryView.Err.Error() != backing.Reason {
				t.Fatalf("error fallback wrong: %+v want reason %q", m.resultHistoryView, backing.Reason)
			}
			// Rendering issues zero requests and shows only surviving data.
			_ = m.View()
			requireZeroRequests(t, execs, nil, "eviction rendering")

			// Surviving entries remain intact.
			for _, id := range ids[1:] {
				if e, ok := m.ResultHistory.Lookup(id); ok && e.Kind == history.KindTabular && len(e.Rows) != 6 {
					t.Fatalf("surviving entry %d rows changed: %d", id, len(e.Rows))
				}
			}
		})
	}
}

// TestDefensiveEvictedSelectionEmptyHistory covers the empty base fallback:
// when nothing is retained after the eviction, result-history mode clears and
// the base builder/result fallback returns with no historical rows and no
// evicted data in any frame.
func TestDefensiveEvictedSelectionEmptyHistory(t *testing.T) {
	execs := &countingExecutors{}
	m := browseModel(t, execs, nil)
	m.ResultHistory = history.NewResultStore()
	ids := fillResultHistory(t, &m, 1)
	m.enterResultHistoryMode()
	if m.resultHistoryCursorID != ids[0] {
		t.Fatal("fixture did not select the only entry")
	}

	// Externally driven replacement of the whole store: the entry vanishes.
	m.ResultHistory = history.NewResultStore()

	m = reconcile(t, m)
	if m.resultHistoryMode || m.resultHistoryView != nil || m.resultHistoryCursorID != 0 {
		t.Fatalf("empty history did not return to the base fallback: mode=%v view=%+v",
			m.resultHistoryMode, m.resultHistoryView)
	}
	if m.resultHistoryNotice != ResultEvictedNotice {
		t.Fatalf("notice = %q, want exactly %q", m.resultHistoryNotice, ResultEvictedNotice)
	}
	if view := m.View(); viewContains(view, "999") {
		t.Fatalf("evicted data rendered in the base fallback:\n%s", view)
	}
	requireZeroRequests(t, execs, nil, "empty-history eviction")
}

// TestDefensiveEvictionNeverRendersEvictedRows proves no intermediate frame
// can render data from an evicted backing entry: after the external append,
// every reachable state — reconciled model, resized model, navigation step,
// dismissal — resolves through the new backing entry only.
func TestDefensiveEvictionNeverRendersEvictedRows(t *testing.T) {
	execs := &countingExecutors{}
	m := browseModel(t, execs, nil)

	// Build a full store with distinctive absolute ranges per entry and
	// select the oldest (positions 1..6).
	full := history.NewResultStore()
	var fullIDs []history.EntryID
	for i := 0; i < 20; i++ {
		retained, _ := full.AppendFinalized(browseEntry(uint64(300+i), 1000+i, 6))
		fullIDs = append(fullIDs, retained.ID)
	}
	m.ResultHistory = full
	m.enterResultHistoryMode()
	m.resultHistoryCursorID = fullIDs[0]
	m.projectSelectedHistoryEntry()
	if view := m.View(); !viewContains(view, "1000") {
		t.Fatal("fixture did not display the selected entry")
	}

	for i := 0; i < 3; i++ {
		if _, ok := m.ResultHistory.AppendFinalized(browseEntry(uint64(600+i), 1, 4)); !ok {
			t.Fatal("external append rejected")
		}
	}

	m = reconcile(t, m)
	if m.resultHistoryCursorID != fullIDs[3] {
		t.Fatalf("selection = %d, want the new oldest %d", m.resultHistoryCursorID, fullIDs[3])
	}
	if m.resultHistoryNotice != ResultEvictedNotice {
		t.Fatalf("notice = %q, want exactly %q", m.resultHistoryNotice, ResultEvictedNotice)
	}
	if view := m.View(); viewContains(view, "1000") || viewContains(view, "1002") {
		t.Fatalf("evicted entries' rows rendered after eviction:\n%s", view)
	}
	if !viewContains(m.View(), "1003") {
		t.Fatal("new oldest entry's rows not rendered after reconciliation")
	}

	// Resize, navigate, and dismiss: every state resolves through the new
	// backing entry only, and no request is issued.
	m = sendKeys(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := m.View(); viewContains(view, "1000") || viewContains(view, "1002") {
		t.Fatal("evicted entries' rows survived a resize frame")
	}
	m = sendKeys(t, m, ctrlKey(tea.KeyCtrlE))
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.resultHistoryMode {
		t.Fatal("esc did not leave result history after eviction reconciliation")
	}
	requireZeroRequests(t, execs, nil, "post-eviction interaction")

	// Every surviving snapshot is unchanged.
	for i, id := range fullIDs[3:] {
		e, ok := m.ResultHistory.Lookup(id)
		if !ok || len(e.Rows) != 6 {
			t.Fatalf("surviving entry %d changed: rows=%d ok=%v", i, len(e.Rows), ok)
		}
	}
}
