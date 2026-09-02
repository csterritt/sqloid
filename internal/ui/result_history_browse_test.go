// Local result-history projection and zero-refetch browsing tests (Issue #36
// Task 1), per the Execution and Result Lifecycle, History Module Design, and
// history Testing Decisions in Notes/PRD-sqloid.md. The projection is a pure
// local reslice of the selected immutable snapshot for the current terminal
// height's complete-row capacity: it never mutates the stored snapshot and
// never consults internal/resultcache as live backing state. Entry selection,
// repeated navigation, resize/reslicing, and rendering issue zero database,
// page, or count requests — the only fresh-data path remains an actual rerun.

package ui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// browseEntry builds a finalized tabular snapshot with n rows at absolute
// positions start..start+n-1, carrying one BLOB to prove exact retention
// through projection. The facts come from a real resultcache-shaped merge.
func browseEntry(execID uint64, start, n int) history.ResultEntry {
	cache := resultcache.New()
	page := resultcache.Page{Start: resultcache.Position(start)}
	for i := 0; i < n; i++ {
		row := []result.Value{result.NewInteger(int64(start + i)), result.NewText("v")}
		if i == 0 {
			row[1] = result.NewBlob([]byte{0xDE, 0xAD})
		}
		page.Rows = append(page.Rows, resultcache.Row{
			Position: resultcache.Position(start + i),
			Values:   row,
		})
	}
	if _, err := cache.Merge(page, resultcache.Forward); err != nil {
		panic(err)
	}
	meta, err := history.NewSnapshotMetadata(history.FactsFromCache(cache), history.Lifecycle{
		Outcome:    history.OutcomeSuccess,
		ReachedLow: true,
	})
	if err != nil {
		panic(err)
	}
	rows := make([][]result.Value, n)
	for i, r := range cache.Rows() {
		rows[i] = r.Values
	}
	return history.ResultEntry{
		ExecutionID:  execID,
		Kind:         history.KindTabular,
		Columns:      []string{"id", "v"},
		Rows:         rows,
		Metadata:     meta,
		Completeness: history.Completeness{Partial: true},
	}
}

// browseErrorEntry builds a finalized non-tabular error entry.
func browseErrorEntry(execID uint64, reason string) history.ResultEntry {
	meta, err := history.NewSnapshotMetadata(history.CacheFacts{},
		history.Lifecycle{Outcome: history.OutcomeFailed, Reason: reason})
	if err != nil {
		panic(err)
	}
	return history.ResultEntry{
		ExecutionID:  execID,
		Kind:         history.KindError,
		Reason:       reason,
		Metadata:     meta,
		Completeness: history.Completeness{Partial: true},
	}
}

// TestProjectHistoryEntryReslicesAtTerminalHeight walks the pure local
// projection: rows are resliced from the immutable snapshot using the current
// complete-row layout capacity, with the absolute offset preserved from the
// entry metadata and the stored snapshot untouched.
func TestProjectHistoryEntryReslicesAtTerminalHeight(t *testing.T) {
	cases := []struct {
		name       string
		entryRows  int
		start      int
		pageRows   int
		wantRows   int
		wantOffset int64
	}{
		{name: "capacity smaller than snapshot reslices locally", entryRows: 5, start: 1, pageRows: 3, wantRows: 3, wantOffset: 0},
		{name: "capacity larger than snapshot shows all rows", entryRows: 3, start: 1, pageRows: 8, wantRows: 3, wantOffset: 0},
		{name: "snapshot starting later keeps absolute offset", entryRows: 6, start: 9, pageRows: 4, wantRows: 4, wantOffset: 8},
		{name: "zero capacity shows no rows", entryRows: 4, start: 1, pageRows: 0, wantRows: 0, wantOffset: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := browseEntry(1, tc.start, tc.entryRows)
			view := projectHistoryEntry(entry, tc.pageRows)
			if view == nil || view.Page == nil {
				t.Fatalf("tabular entry projected to nil page: %+v", view)
			}
			if len(view.Page.Rows) != tc.wantRows {
				t.Fatalf("resliced rows = %d, want %d", len(view.Page.Rows), tc.wantRows)
			}
			if view.Offset != tc.wantOffset {
				t.Fatalf("offset = %d, want %d", view.Offset, tc.wantOffset)
			}
			for i, row := range view.Page.Rows {
				if row[0].Kind != result.KindInteger || row[0].Int != int64(tc.start+i) {
					t.Fatalf("resliced row %d wrong: %+v", i, row[0])
				}
			}
			// The projection must not rewrite the stored entry: the full
			// snapshot is still intact afterwards.
			if len(entry.Rows) != tc.entryRows {
				t.Fatalf("stored entry rows changed by projection: %d", len(entry.Rows))
			}
			if entry.Metadata.RetainedStart != resultcache.Position(tc.start) ||
				entry.Metadata.RetainedEnd != resultcache.Position(tc.start+tc.entryRows-1) {
				t.Fatalf("stored entry metadata changed by projection: %+v", entry.Metadata)
			}
		})
	}
}

// TestProjectHistoryEntryNonTabular walks non-tabular projections: an error
// entry displays its exact reason, a cancelled entry likewise, and neither
// produces rows.
func TestProjectHistoryEntryNonTabular(t *testing.T) {
	errView := projectHistoryEntry(browseErrorEntry(1, "database is locked"), 5)
	if errView == nil || errView.Err == nil || errView.Err.Error() != "database is locked" {
		t.Fatalf("error entry projected wrong: %+v", errView)
	}
	if errView.Page != nil {
		t.Fatal("error entry projected tabular rows")
	}

	meta, _ := history.NewSnapshotMetadata(history.CacheFacts{},
		history.Lifecycle{Outcome: history.OutcomeCancelled, Reason: "cancelled"})
	cancelled := history.ResultEntry{ExecutionID: 2, Kind: history.KindCancelled, Reason: "cancelled", Metadata: meta}
	cancelView := projectHistoryEntry(cancelled, 5)
	if cancelView == nil || cancelView.Err == nil || cancelView.Page != nil {
		t.Fatalf("cancelled entry projected wrong: %+v", cancelView)
	}
}

// TestProjectHistoryEntryImmutable proves mutation of the projected view or
// of retrieved data can never alter the stored history entry: BLOB bytes are
// copied exactly and survive both directions of mutation.
func TestProjectHistoryEntryImmutable(t *testing.T) {
	s := history.NewResultStore()
	retained, _ := s.AppendFinalized(browseEntry(1, 1, 5))
	entry, _ := s.Lookup(retained.ID)

	view := projectHistoryEntry(entry, 3)
	if view.Page == nil || len(view.Page.Rows) == 0 {
		t.Fatal("projection lost rows")
	}
	// Mutate the projected rows and the BLOB bytes in the view.
	view.Page.Rows[0][1].Bytes[0] = 0x00
	view.Page.Rows[0][0] = result.NewText("mutated")
	// Re-retrieve from the store: nothing may have changed.
	after, _ := s.Lookup(retained.ID)
	if after.Rows[0][0].Kind != result.KindInteger || after.Rows[0][0].Int != 1 {
		t.Fatalf("stored row mutated through projection: %+v", after.Rows[0][0])
	}
	if after.Rows[0][1].Kind != result.KindBlob || after.Rows[0][1].Bytes[0] != 0xDE {
		t.Fatalf("stored BLOB bytes mutated through projection: %x", after.Rows[0][1].Bytes)
	}
}

// countingExecutors counts database executor invocations so tests can prove
// browsing issues zero requests.
type countingExecutors struct {
	selects int
	pages   int
	counts  int
}

func (c *countingExecutors) selectPage(_ context.Context, _ string, _ []any) FirstPageResult {
	c.selects++
	return FirstPageResult{Err: errors.New("no database work may happen while browsing")}
}
func (c *countingExecutors) page(_ context.Context, _ string, _ []any, _ int64) FirstPageResult {
	c.pages++
	return FirstPageResult{Err: errors.New("no database work may happen while browsing")}
}
func (c *countingExecutors) count(_ context.Context, _ string, _ []any) CountResult {
	c.counts++
	return CountResult{Err: errors.New("no database work may happen while browsing")}
}

// requireZeroRequests asserts no executor ran and no command carrying a
// page/count/first-page settlement exists.
func requireZeroRequests(t *testing.T, c *countingExecutors, cmd tea.Cmd, context string) {
	t.Helper()
	if c.selects != 0 || c.pages != 0 || c.counts != 0 {
		t.Fatalf("%s: database requests issued while browsing (select=%d page=%d count=%d)",
			context, c.selects, c.pages, c.counts)
	}
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg.(type) {
	case SelectSettledMsg, PageSettledMsg, CountSettledMsg:
		t.Fatalf("%s: command issued a database request: %T", context, msg)
	}
}

// browseModel builds a model seeded with finalized result-history entries and
// wired with counting executors, sized to a known terminal height.
func browseModel(t *testing.T, execs *countingExecutors, entries []history.ResultEntry) Model {
	t.Helper()
	m := New()
	m.Select = execs.selectPage
	m.Page = execs.page
	m.Count = execs.count
	m.ResultHistory = history.NewResultStore()
	for _, e := range entries {
		if _, ok := m.ResultHistory.AppendFinalized(e); !ok {
			t.Fatal("seeding result history failed")
		}
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

// TestResultHistoryBrowsingIssuesZeroRequests covers entry selection,
// repeated navigation, resize/reslicing, and rendering: none may issue any
// database, page, or count request, and every displayed row must come from
// the selected immutable snapshot.
func TestResultHistoryBrowsingIssuesZeroRequests(t *testing.T) {
	execs := &countingExecutors{}
	m := browseModel(t, execs, []history.ResultEntry{
		browseEntry(1, 1, 30),
		browseEntry(2, 1, 10),
		browseErrorEntry(3, "no such table: gone"),
	})

	// Enter result-history browsing at the newest entry.
	m.enterResultHistoryMode()
	if !m.resultHistoryMode {
		t.Fatal("entering result history failed")
	}
	entry3, _ := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	if entry3.Kind != history.KindError {
		t.Fatalf("selection is not the newest entry: %+v", entry3)
	}
	requireZeroRequests(t, execs, nil, "entry selection")

	pageRows := int64(CalculateLayout(m.Height, m.Fields).PageRows)
	if m.resultHistoryView == nil || m.resultHistoryView.Err == nil ||
		m.resultHistoryView.Err.Error() != "no such table: gone" {
		t.Fatalf("error entry not displayed as the selected snapshot: %+v", m.resultHistoryView)
	}

	// Repeated navigation down to the oldest entry and back, including the
	// deterministic boundaries and the newest-boundary mode exit.
	m.resultHistoryStep(false) // older
	m.resultHistoryStep(false) // older → oldest entry (ID 1)
	if id := m.resultHistoryCursorID; id != 1 {
		t.Fatalf("navigation did not reach the oldest entry: cursor %d", id)
	}
	m.resultHistoryStep(false) // older at the oldest boundary: no-op
	if id := m.resultHistoryCursorID; id != 1 {
		t.Fatalf("older boundary moved the cursor: %d", id)
	}
	requireZeroRequests(t, execs, nil, "repeated navigation")
	tabEntry, _ := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	if len(tabEntry.Rows) != 30 {
		t.Fatalf("backing snapshot rows changed: %d", len(tabEntry.Rows))
	}
	if m.resultHistoryView == nil || len(m.resultHistoryView.Page.Rows) != int(pageRows) {
		t.Fatalf("displayed slice wrong for height %d: %+v", m.Height, m.resultHistoryView)
	}
	m.resultHistoryStep(true) // newer
	m.resultHistoryStep(true) // newer → newest entry (ID 3)
	if id := m.resultHistoryCursorID; id != 3 {
		t.Fatalf("navigation did not reach the newest entry: cursor %d", id)
	}
	m.resultHistoryStep(true) // newer at the newest boundary exits mode
	if m.resultHistoryMode {
		t.Fatal("newer step at the newest boundary did not exit result history")
	}
	if m.resultHistoryView != nil {
		t.Fatal("historical rows survived the mode exit")
	}

	// Re-enter and step to the middle tabular entry for the resize check.
	m.enterResultHistoryMode()
	m.resultHistoryStep(false)
	if id := m.resultHistoryCursorID; id != 2 {
		t.Fatalf("re-entry did not select the newest then step older: cursor %d", id)
	}

	// Resize while browsing: rows reslice locally from the same snapshot for
	// the new complete-row capacity, with no stored mutation and no fetch.
	before, _ := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	requireZeroRequests(t, execs, cmd, "resize")
	newPageRows := int64(CalculateLayout(m.Height, m.Fields).PageRows)
	wantRows := int(newPageRows)
	if wantRows > 10 {
		wantRows = 10
	}
	if m.resultHistoryView == nil || len(m.resultHistoryView.Page.Rows) != wantRows {
		t.Fatalf("resize did not reslice to the new capacity: rows=%d want %d",
			len(m.resultHistoryView.Page.Rows), wantRows)
	}
	after, _ := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	if len(after.Rows) != len(before.Rows) || after.Metadata.RetainedEnd != before.Metadata.RetainedEnd {
		t.Fatal("resize mutated the stored snapshot")
	}

	// Rendering reads only the projection: zero requests.
	_ = m.View()
	requireZeroRequests(t, execs, nil, "rendering")

	// The only fresh-data path remains an actual rerun: the active SELECT was
	// finalized by entering history and no page request can be issued.
	if m.SelectIsActive() {
		t.Fatal("entering result history did not finalize the active SELECT")
	}
}
