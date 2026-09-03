// UI lifecycle coverage for Issue #76: typed page/value over-limit failures
// round-trip from accepted active-page settlement through finalization into
// immutable history.SnapshotMetadata, through local historical projection at
// multiple terminal page sizes, through browsing and pre-destination export
// presentation, and through serializer-spy exclusion proofs. The typed
// LimitFailure kind and one-based position are preserved as immutable facts
// independent of byte-cap eviction disclosure and terminal outcome; a
// no-failure snapshot stays unset through finalization and projection.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// limitFailureRoundtripModel wires a runnable SELECT model with both executor
// seams and a fresh result-history store so the real settlement and
// finalization paths run end-to-end.
func limitFailureRoundtripModel(exec *fakeSelectExecutor) Model {
	m := pagingModel(exec, &fakePageExecutor{rowsShown: int64(defaultPageRows)})
	m.ResultHistory = history.NewResultStore()
	return m
}

// finalizeWithLimitFailure drives a real first-page execution, then pages
// forward to trigger a typed limit failure at the given one-based absolute
// position via the fake page executor, then finalizes via enterResultHistory
// and returns the model plus the one retained tabular entry. The leading
// rows before the failing row enter the contiguous cache through the real
// merge seam.
func finalizeWithLimitFailure(t *testing.T, kind result.LimitKind, failureOffset int64, leadingRows int64) (Model, history.ResultEntry) {
	t.Helper()
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	pageExec := &fakePageExecutor{
		rowsShown:          int64(defaultPageRows),
		honorLimit:         true,
		limitFailure:       &result.LimitFailure{Kind: kind},
		limitFailureAt:     leadingRows,   // 0-based index of failing row = leading row count
		limitFailureOffset: failureOffset, // absolute offset at which to fail
	}
	m := limitFailureRoundtripModel(exec)
	m.Page = pageExec.page
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)

	// Page forward to the target offset. Each page down advances by
	// defaultPageRows rows; keep paging until the fake executor returns the
	// limit failure at the requested offset.
	cur := m
	for {
		next, cmd := pageDown(cur)
		if cmd == nil {
			t.Fatalf("page down produced no command at offset %d", failureOffset)
		}
		settled := settlePage(t, next, cmd)
		if settled.Result != nil && settled.Result.LimitFailure != nil {
			cur = settled
			break
		}
		cur = settled
	}
	if cur.Result == nil || cur.Result.LimitFailure == nil {
		t.Fatalf("limit failure not triggered at offset %d", failureOffset)
	}
	cur.enterResultHistory()
	entries := cur.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	return cur, entries[0]
}

// TestFinalizationPreservesTypedLimitFailureKindAndPosition proves the
// accepted active ResultView's typed LimitFailure kind and one-based
// position become immutable snapshot metadata through
// appendFinalizedResultEntry, for both page and value failure kinds (AC1).
func TestFinalizationPreservesTypedLimitFailureKindAndPosition(t *testing.T) {
	cases := []struct {
		name    string
		kind    result.LimitKind
		offset  int64
		leading int64
		wantPos int64
	}{
		{
			name:    "page failure at row 26",
			kind:    result.KindPage,
			offset:  22, // page down twice from offset 0 (11 rows per page)
			leading: 3,  // 3 leading rows before the failing row
			wantPos: 26, // offset 22 + 0-based index 3 + 1 = position 26
		},
		{
			name:    "value failure at row 15",
			kind:    result.KindValue,
			offset:  11, // page down once from offset 0
			leading: 3,  // 3 leading rows before the failing row
			wantPos: 15, // offset 11 + 0-based index 3 + 1 = position 15
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, entry := finalizeWithLimitFailure(t, tc.kind, tc.offset, tc.leading)
			if entry.Metadata.LimitFailureKind != tc.kind {
				t.Fatalf("stored LimitFailureKind = %v, want %v",
					entry.Metadata.LimitFailureKind, tc.kind)
			}
			if entry.Metadata.LimitFailurePosition != tc.wantPos {
				t.Fatalf("stored LimitFailurePosition = %d, want %d",
					entry.Metadata.LimitFailurePosition, tc.wantPos)
			}
		})
	}
}

// TestFinalizationPreservesLeadingRowsAndAbsolutePositions proves the
// finalized snapshot retains the complete leading rows that entered the
// contiguous cache before the limit failure, with their absolute positions
// and typed cells unchanged (AC1).
func TestFinalizationPreservesLeadingRowsAndAbsolutePositions(t *testing.T) {
	_, entry := finalizeWithLimitFailure(t, result.KindPage, 22, 3)
	if len(entry.Rows) == 0 {
		t.Fatalf("no leading rows retained: %+v", entry)
	}
	if !entry.Metadata.HasRetainedRange {
		t.Fatalf("snapshot missing retained range metadata")
	}
	if entry.Metadata.RetainedStart != 1 {
		t.Errorf("RetainedStart = %d, want 1", entry.Metadata.RetainedStart)
	}
	for i, row := range entry.Rows {
		if len(row) == 0 || row[0].Kind != result.KindInteger {
			t.Errorf("retained row %d is not a typed INTEGER: %+v", i, row)
		}
	}
}

// TestFinalizationLimitFailureImmutableAfterMutation proves stored
// limit-failure metadata and rows remain unchanged after finalization when
// the live result view, projected view, and cache state are mutated (AC2).
func TestFinalizationLimitFailureImmutableAfterMutation(t *testing.T) {
	m, entry := finalizeWithLimitFailure(t, result.KindValue, 11, 3)
	before, ok := m.ResultHistory.Lookup(entry.ID)
	if !ok {
		t.Fatal("baseline lookup failed")
	}
	// Mutate the live result view's LimitFailure.
	if m.Result != nil && m.Result.LimitFailure != nil {
		m.Result.LimitFailure.Kind = result.KindPage
		m.Result.LimitFailure.Position = 999
	}
	// Mutate the projected view if one exists.
	if m.resultHistoryView != nil {
		m.resultHistoryView.LimitFailure = &result.LimitFailure{Kind: result.KindPage, Position: 999}
	}
	// Mutate the retrieved entry (a deep copy from the store).
	entry.Metadata.LimitFailureKind = result.KindPage
	entry.Metadata.LimitFailurePosition = 999
	// Merge more rows into the live cache; the snapshot is independent.
	if m.viewportCache != nil {
		mergeRowsIntoCache(t, &m, int64(len(entry.Rows)+1), 5)
	}
	after, ok := m.ResultHistory.Lookup(entry.ID)
	if !ok {
		t.Fatal("finalized entry was evicted by post-finalization mutation")
	}
	if after.Metadata.LimitFailureKind != before.Metadata.LimitFailureKind {
		t.Errorf("stored LimitFailureKind changed: got %v want %v",
			after.Metadata.LimitFailureKind, before.Metadata.LimitFailureKind)
	}
	if after.Metadata.LimitFailurePosition != before.Metadata.LimitFailurePosition {
		t.Errorf("stored LimitFailurePosition changed: got %d want %d",
			after.Metadata.LimitFailurePosition, before.Metadata.LimitFailurePosition)
	}
	if len(after.Rows) != len(before.Rows) {
		t.Errorf("stored row count changed: got %d want %d",
			len(after.Rows), len(before.Rows))
	}
}

// TestProjectHistoryEntryRestoresLimitFailure proves local historical
// projection restores ResultView.LimitFailure from the immutable metadata
// at multiple terminal page sizes, preserving offset, rows, columns, and
// the exact typed kind and position (AC1).
func TestProjectHistoryEntryRestoresLimitFailure(t *testing.T) {
	cases := []struct {
		name    string
		kind    result.LimitKind
		offset  int64
		leading int64
		wantPos int64
	}{
		{name: "page failure", kind: result.KindPage, offset: 22, leading: 3, wantPos: 26},
		{name: "value failure", kind: result.KindValue, offset: 11, leading: 3, wantPos: 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, entry := finalizeWithLimitFailure(t, tc.kind, tc.offset, tc.leading)
			for _, pageRows := range []int{0, 2, 5, 10, 50} {
				view := projectHistoryEntry(entry, pageRows)
				if view == nil || view.Page == nil {
					t.Fatalf("pageRows=%d: projection lost page", pageRows)
				}
				if view.LimitFailure == nil {
					t.Fatalf("pageRows=%d: projection lost LimitFailure", pageRows)
				}
				if view.LimitFailure.Kind != tc.kind {
					t.Errorf("pageRows=%d: LimitFailure.Kind = %v, want %v",
						pageRows, view.LimitFailure.Kind, tc.kind)
				}
				if view.LimitFailure.Position != tc.wantPos {
					t.Errorf("pageRows=%d: LimitFailure.Position = %d, want %d",
						pageRows, view.LimitFailure.Position, tc.wantPos)
				}
				want := pageRows
				if want > len(entry.Rows) {
					want = len(entry.Rows)
				}
				if len(view.Page.Rows) != want {
					t.Errorf("pageRows=%d: resliced rows = %d want %d",
						pageRows, len(view.Page.Rows), want)
				}
			}
		})
	}
}

// TestHistoricalBrowsingRendersExactLimitFailureMessage proves browsing
// renders the exact shared result.LimitFailure.Error line for page versus
// value failures at multiple terminal heights (AC2).
func TestHistoricalBrowsingRendersExactLimitFailureMessage(t *testing.T) {
	cases := []struct {
		name    string
		kind    result.LimitKind
		offset  int64
		leading int64
		wantPos int64
	}{
		{name: "page failure", kind: result.KindPage, offset: 22, leading: 3, wantPos: 26},
		{name: "value failure", kind: result.KindValue, offset: 11, leading: 3, wantPos: 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, entry := finalizeWithLimitFailure(t, tc.kind, tc.offset, tc.leading)
			store := history.NewResultStore()
			store.AppendFinalized(entry)
			m.ResultHistory = store
			m.enterResultHistoryMode()
			wantMsg := result.LimitFailureMessage(tc.kind, tc.wantPos)
			for _, height := range []int{24, 30, 40} {
				next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: height})
				m = next.(Model)
				view := m.View()
				if !strings.Contains(view, wantMsg) {
					t.Errorf("height=%d: browsing view missing exact limit-failure message %q:\n%s",
						height, wantMsg, view)
				}
			}
		})
	}
}

// TestHistoricalExportPreservesLimitFailureMetadata proves the pre-destination
// export flow for a historical snapshot carrying a typed limit failure
// preserves the immutable snapshot metadata without injecting a partial row
// (AC3).
func TestHistoricalExportPreservesLimitFailureMetadata(t *testing.T) {
	m, entry := finalizeWithLimitFailure(t, result.KindPage, 22, 3)
	store := history.NewResultStore()
	store.AppendFinalized(entry)
	m.ResultHistory = store
	m.enterResultHistoryMode()
	opened, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil {
		t.Fatal("Ctrl+X issued a command")
	}
	if opened.exportPrepared == nil {
		t.Fatalf("historical export flow did not open: %q", opened.exportNotice)
	}
	captured := opened.exportPrepared
	if captured.Metadata.LimitFailureKind != result.KindPage {
		t.Errorf("export capture lost LimitFailureKind: got %v want %v",
			captured.Metadata.LimitFailureKind, result.KindPage)
	}
	if captured.Metadata.LimitFailurePosition != entry.Metadata.LimitFailurePosition {
		t.Errorf("export capture lost LimitFailurePosition: got %d want %d",
			captured.Metadata.LimitFailurePosition, entry.Metadata.LimitFailurePosition)
	}
	if len(captured.Payload.Rows) != len(entry.Rows) {
		t.Errorf("export payload rows = %d, want %d (no partial failure row)",
			len(captured.Payload.Rows), len(entry.Rows))
	}
	if len(captured.Payload.Names) != len(entry.Columns) {
		t.Errorf("export payload names = %d, want %d",
			len(captured.Payload.Names), len(entry.Columns))
	}
}

// TestHistoricalExportPayloadExcludesLimitFailureMessage proves the typed
// limit-failure message never becomes a CSV record, JSON object property,
// or data value: the serializer-visible Payload carries only names, rows,
// and positions (AC3).
func TestHistoricalExportPayloadExcludesLimitFailureMessage(t *testing.T) {
	m, entry := finalizeWithLimitFailure(t, result.KindValue, 11, 3)
	store := history.NewResultStore()
	store.AppendFinalized(entry)
	m.ResultHistory = store
	m.enterResultHistoryMode()
	opened, _ := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if opened.exportPrepared == nil {
		t.Fatal("historical export flow did not open")
	}
	payload := opened.exportPrepared.Payload
	wantMsg := result.LimitFailureMessage(result.KindValue, entry.Metadata.LimitFailurePosition)
	for _, name := range payload.Names {
		if strings.Contains(name, "exceeds") || strings.Contains(name, "64 MiB") {
			t.Errorf("payload name %q carries limit-failure text", name)
		}
	}
	for _, row := range payload.Rows {
		for _, v := range row {
			tok := v.Display()
			if strings.Contains(tok, "exceeds") || strings.Contains(tok, wantMsg) {
				t.Errorf("payload value token %q carries limit-failure text", tok)
			}
		}
	}
	csvOut := string(export.CSV(payload))
	if strings.Contains(csvOut, wantMsg) {
		t.Errorf("CSV output carries limit-failure text:\n%s", csvOut)
	}
	jsonOut := string(export.JSON(payload))
	if strings.Contains(jsonOut, wantMsg) {
		t.Errorf("JSON output carries limit-failure text:\n%s", jsonOut)
	}
}

// TestNoFailureSnapshotRemainsUnset proves a snapshot with no limit failure
// keeps the typed limit-failure metadata absent through finalization and
// projection: no LimitFailure is synthesized on the projected view (AC4).
func TestNoFailureSnapshotRemainsUnset(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	m := limitFailureRoundtripModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Metadata.LimitFailureKind != 0 {
		t.Errorf("no-failure snapshot has LimitFailureKind = %v, want 0 (unset)",
			entry.Metadata.LimitFailureKind)
	}
	if entry.Metadata.LimitFailurePosition != 0 {
		t.Errorf("no-failure snapshot has LimitFailurePosition = %d, want 0 (unset)",
			entry.Metadata.LimitFailurePosition)
	}
	for _, pageRows := range []int{0, 3, 11, 50} {
		view := projectHistoryEntry(entry, pageRows)
		if view == nil || view.Page == nil {
			t.Fatalf("pageRows=%d: no-failure projection lost page", pageRows)
		}
		if view.LimitFailure != nil {
			t.Errorf("pageRows=%d: no-failure projection synthesized LimitFailure: %+v",
				pageRows, view.LimitFailure)
		}
	}
}
