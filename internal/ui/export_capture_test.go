// Ctrl+X immutable export-capture model coverage for Issue #49, per the
// Result export scope, Export warnings, Cache and snapshot invariant,
// Context/Action matrix, and Testing Decisions in Notes/PRD-sqloid.md.
// Ctrl+X targeting follows the current in-memory selection — the idle
// active tabular result, each selected retained history snapshot, and the
// terminal in-memory selection including Ctrl+E/Y changes — and captures
// synchronously inside Update: deduplicated names, ascending logical
// positions, every typed value with exact BLOB bytes, and snapshot
// metadata, before any picker command or later mutation can run. Capture
// never finalizes or deactivates an active SELECT, never changes its
// execution/request/generation/cache/viewport/history state, and starts no
// database work. Eligibility rejection and warnings belong to later tasks.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// TestIdleActiveExportCapture pins the idle active SELECT contract: Ctrl+X
// deep-copies the viewed tabular result synchronously, returns no command,
// preserves every active-SELECT and request/generation state, and issues
// zero database work.
func TestIdleActiveExportCapture(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 3}
	pageExec := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m := settledFirstPage(t, exec, pageExec)
	m.Count = count.count
	m.Refresher = refresh

	beforeCalls := [4]int{exec.calls, count.calls, pageExec.issued, refresh.calls}
	beforeTracker := m.selectTracker
	beforeGen := m.viewportGen
	beforeCache := m.viewportCache
	beforeExec := m.activeExecID

	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil {
		t.Fatal("idle active Ctrl+X issued a command")
	}
	if nm.exportPrepared == nil {
		t.Fatalf("no capture prepared: %q", nm.exportNotice)
	}
	cap := *nm.exportPrepared
	if !reflectNames(cap.Payload.Names, []string{"id"}) {
		t.Errorf("capture names = %q, want [id]", cap.Payload.Names)
	}
	if len(cap.Payload.Rows) != 3 {
		t.Fatalf("capture rows = %d, want the three cached rows", len(cap.Payload.Rows))
	}
	for i, p := range cap.Payload.Positions {
		if p != int64(1+i) {
			t.Errorf("position %d = %d, want ascending from 1", i, p)
		}
	}
	for i, row := range cap.Payload.Rows {
		if row[0].Int != int64(1+i) {
			t.Errorf("captured row %d = %v, want the active INTEGER %d", i, row[0], 1+i)
		}
	}
	if !nm.SelectIsActive() {
		t.Error("capture deactivated the active SELECT lifetime")
	}
	if nm.activeExecID != beforeExec {
		t.Error("capture changed the active execution identity")
	}
	if nm.finalizedExecID != 0 || nm.finalizedExecID == nm.activeExecID {
		t.Errorf("capture finalized the active SELECT: finalized=%d active=%d", nm.finalizedExecID, nm.activeExecID)
	}
	if nm.selectTracker != beforeTracker {
		t.Error("capture mutated the execution/request tracker")
	}
	if nm.viewportGen != beforeGen || nm.viewportCache != beforeCache {
		t.Error("capture advanced the viewport generation or replaced the cache")
	}
	if nm.firstPagePending || nm.pagePending || nm.countPendingFlag {
		t.Error("capture left or created pending request state")
	}
	if [4]int{exec.calls, count.calls, pageExec.issued, refresh.calls} != beforeCalls {
		t.Fatalf("capture started database work: select=%d->%d count=%d->%d page=%d->%d refresh=%d->%d",
			beforeCalls[0], exec.calls, beforeCalls[1], count.calls, beforeCalls[2], pageExec.issued, beforeCalls[3], refresh.calls)
	}
}

// reflectNames compares a captured name set with the expected strings.
func reflectNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// reflectPositions compares a captured position sequence with the expected
// ascending values.
func reflectPositions(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestExportCaptureImmuneToLiveMutation proves the captured payload is fully
// independent of the live result: mutating the displayed rows, the typed
// cells, and the original BLOB byte slices after capture leaves the capture
// byte-identical, still in ascending logical-position order.
func TestExportCaptureImmuneToLiveMutation(t *testing.T) {
	page := result.FromDriver([]string{"id", "blob", "id"}, [][]any{
		{int64(1), []byte{0xDE, 0xAD}, int64(9)},
		{int64(2), []byte{0xBE}, int64(8)},
	})
	m := resultModel(t, &page, nil)
	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil || nm.exportPrepared == nil {
		t.Fatalf("Ctrl+X did not capture: cmd=%v notice=%q", cmd, nm.exportNotice)
	}
	cap := *nm.exportPrepared

	// Mutate every live source afterward: cells, BLOB payloads, row and page
	// slices, and the driver metadata.
	page.Rows[0][0] = result.NewInteger(999)
	page.Rows[0][1].Bytes[0] = 0x00
	page.Rows[1][1].Bytes = []byte{0xFF, 0xFF}
	page.Columns[0] = "mutated"
	page.Rows = append(page.Rows, []result.Value{result.NewInteger(3), result.NewNull(), result.NewNull()})

	if !reflectNames(cap.Payload.Names, []string{"id", "blob", "id_2"}) {
		t.Errorf("capture names = %q, want deduplicated [id blob id_2]", cap.Payload.Names)
	}
	if got := cap.Payload.Rows[0][0].Int; got != 1 {
		t.Errorf("captured first cell = %d, want 1 after live mutation", got)
	}
	if got := cap.Payload.Rows[1][0].Int; got != 2 {
		t.Errorf("captured second-row cell = %d, want 2 after live mutation", got)
	}
	if string(cap.Payload.Rows[0][1].Bytes) != "\xDE\xAD" || string(cap.Payload.Rows[1][1].Bytes) != "\xBE" {
		t.Errorf("captured BLOB bytes mutated: %x %x", cap.Payload.Rows[0][1].Bytes, cap.Payload.Rows[1][1].Bytes)
	}
	if len(cap.Payload.Rows) != 2 {
		t.Errorf("capture gained the appended row: %d rows", len(cap.Payload.Rows))
	}
	if !reflectPositions(cap.Payload.Positions, []int64{1, 2}) {
		t.Errorf("positions = %v, want ascending [1 2]", cap.Payload.Positions)
	}
}

// TestHistorySelectionExportCapture pins historical targeting: Ctrl+X
// captures the selected retained snapshot, and after a Ctrl+E selection
// change the already-captured payload stays exactly what the first selection
// held, with zero database work.
func TestHistorySelectionExportCapture(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	pageExec := &fakePageExecutor{rowsShown: 3}
	m := settledFirstPage(t, exec, pageExec)
	// Seed a distinct retained tabular snapshot (with a BLOB and its own
	// retained-range metadata) as the newest entry so the selection and the
	// capture inputs are fully known.
	if _, ok := m.ResultHistory.AppendFinalized(browseEntry(5, 1, 2)); !ok {
		t.Fatal("setup: could not seed a tabular snapshot")
	}
	m.enterResultHistoryMode()
	if !m.resultHistoryMode {
		t.Fatal("setup: result-history mode did not engage")
	}
	newest, ok := m.ResultHistory.Newest()
	if !ok || newest.ID != m.resultHistoryCursorID {
		t.Fatal("setup: selection is not the newest entry")
	}

	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil {
		t.Fatal("historical Ctrl+X issued a command")
	}
	if nm.exportPrepared == nil {
		t.Fatalf("no historical capture: %q", nm.exportNotice)
	}
	cap := *nm.exportPrepared
	// Entering result history first finalized the active SELECT, so the
	// newest selected entry is that tabular snapshot; the capture must match
	// its immutable contents exactly, ascending from its retained range.
	if !reflectNames(cap.Payload.Names, result.DeduplicateNames(newest.Columns)) {
		t.Errorf("capture names = %q, want %q", cap.Payload.Names, newest.Columns)
	}
	if len(cap.Payload.Rows) != len(newest.Rows) || len(newest.Rows) == 0 {
		t.Fatalf("capture rows = %d, want the snapshot's %d", len(cap.Payload.Rows), len(newest.Rows))
	}
	wantStart := int64(1)
	if newest.Metadata.HasRetainedRange {
		wantStart = int64(newest.Metadata.RetainedStart)
	}
	for i, p := range cap.Payload.Positions {
		if p != wantStart+int64(i) {
			t.Fatalf("position %d = %d, want ascending from %d", i, p, wantStart)
		}
	}
	if cap.Metadata.Outcome != history.OutcomeSuccess {
		t.Errorf("capture metadata outcome = %v, want the snapshot's success", cap.Metadata.Outcome)
	}

	beforeCalls := [2]int{exec.calls, pageExec.issued}
	// Change the in-memory selection afterward; the earlier capture must not
	// follow it, whatever later selection mutations do.
	if _, ok := m.ResultHistory.OlderThan(m.resultHistoryCursorID); !ok {
		t.Fatal("setup: no older entry to select")
	}
	step := m
	step.resultHistoryStep(false)
	if step.resultHistoryCursorID == nm.resultHistoryCursorID {
		t.Fatal("setup: selection change did not move the cursor")
	}
	if nm.exportPrepared == nil {
		t.Fatal("selection change destroyed the captured payload")
	}
	if !reflectPositions((*nm.exportPrepared).Payload.Positions, []int64{1, 2, 3}) {
		t.Errorf("capture followed the selection change: %v", (*nm.exportPrepared).Payload.Positions)
	}
	if [2]int{exec.calls, pageExec.issued} != beforeCalls {
		t.Fatalf("historical capture started database work: select=%d->%d page=%d->%d",
			beforeCalls[0], exec.calls, beforeCalls[1], pageExec.issued)
	}
}

// TestTerminalSelectionExportCapture pins terminal targeting across all
// three terminal workflows: Ctrl+X targets the current in-memory selection
// (newest entry initially, then Ctrl+E/Y changes) without consulting the
// database, capturing only backed tabular snapshots.
func TestTerminalSelectionExportCapture(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want TerminalState
	}{
		{name: "deletion", err: deletedHealthErr(), want: TerminalDeleted},
		{name: "replacement", err: replacedHealthErr(), want: TerminalReplaced},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, sel, count, page, refresh := healthTerminalModel(t, tc.err, tc.want)
			// Entry 12 (newest, selected on terminal entry) has one row at
			// position 1; Ctrl+E selects entry 11 with rows 1-2.
			newestCap, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
			if cmd != nil {
				t.Fatal("terminal Ctrl+X issued a command")
			}
			if newestCap.exportPrepared == nil {
				t.Fatalf("terminal tabular selection not captured: %q", newestCap.exportNotice)
			}
			if !reflectPositions((*newestCap.exportPrepared).Payload.Positions, []int64{1}) {
				t.Errorf("newest-entry capture positions = %v, want [1]", (*newestCap.exportPrepared).Payload.Positions)
			}
			// Esc closes the warning flow, then Ctrl+E/Y changes the in-memory
			// selection and a second Ctrl+X follows it — all without database
			// work.
			closed, _ := pressKey(newestCap, tea.KeyMsg{Type: tea.KeyEsc})
			if closed.exportPrepared != nil {
				t.Fatal("Esc did not close the export flow")
			}
			older, _ := m.ResultHistory.OlderThan(m.resultHistoryCursorID)
			step, _ := pressKey(closed, ctrlKey(tea.KeyCtrlE))
			if step.resultHistoryCursorID != older.ID {
				t.Fatal("setup: Ctrl+E did not move the terminal selection")
			}
			after, _ := pressKey(step, ctrlKey(tea.KeyCtrlX))
			if after.exportPrepared == nil || !reflectPositions((*after.exportPrepared).Payload.Positions, []int64{1, 2}) {
				t.Fatalf("Ctrl+E selection not captured: %+v %q", after.exportPrepared, after.exportNotice)
			}
			if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 {
				t.Fatalf("terminal capture started database work: select=%d count=%d page=%d refresh=%d",
					sel.calls, count.calls, page.issued, refresh.calls)
			}
		})
	}
}

// TestOutcomeUnknownTerminalExportTargetsSelection pins the outcome-unknown
// terminal: the initially selected outcome-unknown entry is non-tabular, and
// Ctrl+E to an older retained tabular snapshot makes Ctrl+X capture it —
// always without database work and with no terminal-state disturbance.
func TestOutcomeUnknownTerminalExportTargetsSelection(t *testing.T) {
	m, fake := updateUnresolvedUpdate(t)
	m.catalog = prepCatalog()
	// A retained tabular snapshot exists behind the newest outcome-unknown
	// entry so Ctrl+E can select it.
	if _, ok := m.ResultHistory.AppendFinalized(browseEntry(3, 1, 1)); !ok {
		t.Fatal("setup: could not seed a tabular entry")
	}
	// The seeded snapshot becomes the newest entry; Ctrl+Y selects it from
	// the initially selected outcome-unknown entry.
	stepped, _ := pressKey(m, ctrlKey(tea.KeyCtrlY))
	if stepped.resultHistoryCursorID == m.resultHistoryCursorID {
		t.Fatal("setup: Ctrl+Y did not leave the outcome-unknown entry")
	}
	nm, cmd := pressKey(stepped, ctrlKey(tea.KeyCtrlX))
	if cmd != nil {
		t.Fatal("terminal Ctrl+X issued a command")
	}
	if nm.exportPrepared == nil {
		t.Fatalf("terminal tabular snapshot not captured: %q", nm.exportNotice)
	}
	if !reflectNames((*nm.exportPrepared).Payload.Names, []string{"id", "v"}) {
		t.Errorf("capture names = %q, want the snapshot's [id v]", (*nm.exportPrepared).Payload.Names)
	}
	if nm.terminalState != TerminalOutcomeUnknown || nm.writePending {
		t.Errorf("capture disturbed the terminal state: %v pending=%v", nm.terminalState, nm.writePending)
	}
	_ = fake
}
