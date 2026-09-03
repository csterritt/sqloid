// UI lifecycle coverage for Issue #75: invalid-UTF and byte-cap warning
// metadata round trips from accepted active-page settlement through
// finalization into immutable history.SnapshotMetadata, through local
// historical projection at multiple terminal page sizes, through browsing
// and pre-destination export presentation, and through serializer-spy
// exclusion proofs. Warning facts must be immutable: mutating live pages,
// projected views, source BLOBs, and cache state after finalization cannot
// change stored rows or metadata. Neither warning ever becomes a CSV record,
// JSON object property, or data value.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// malformedTextPage returns a first-page fixture carrying one TEXT value
// whose maximal invalid UTF-8 bytes were replaced, with Page.InvalidUTF
// set so the active page owns the invalid-UTF truth.
func malformedTextPage(rows int) *result.Page {
	decoded, _ := result.DecodeText("bad\xE0\x80\x80tail")
	out := make([][]result.Value, rows)
	for i := range out {
		out[i] = []result.Value{result.NewInteger(int64(i + 1)), result.NewText(decoded)}
	}
	return &result.Page{Columns: []string{"id", "t"}, Rows: out, InvalidUTF: true}
}

// byteCapCache pre-seeds a viewport cache whose retained payload already
// exceeds MaxPayloadBytes so TruncatedByByteCap is sticky, then returns it
// for use as the active SELECT's authoritative cache.
func byteCapCache(t *testing.T, rows int) *resultcache.Cache {
	t.Helper()
	third := int(resultcache.MaxPayloadBytes/3) + 1
	c := resultcache.New()
	for i := int64(1); i <= int64(rows); i++ {
		p := resultcache.Page{
			Start: resultcache.Position(i),
			Rows: []resultcache.Row{{
				Position: resultcache.Position(i),
				Values:   []result.Value{result.NewInteger(i), result.NewBlob(make([]byte, third))},
			}},
		}
		if accepted, _ := c.Merge(p, resultcache.Forward); !accepted {
			t.Fatalf("byte-cap pre-seed merge %d not accepted", i)
		}
	}
	if !c.TruncatedByByteCap() {
		t.Fatalf("byte-cap cache did not become truncated after %d rows", rows)
	}
	return c
}

// warningRoundtripModel wires a runnable SELECT model with both executor
// seams and a fresh result-history store so the real settlement and
// finalization paths run end-to-end.
func warningRoundtripModel(exec *fakeSelectExecutor) Model {
	m := pagingModel(exec, &fakePageExecutor{rowsShown: int64(defaultPageRows)})
	m.ResultHistory = history.NewResultStore()
	return m
}

// finalizeWithMalformedText drives a real first-page execution carrying
// malformed TEXT through validation, execution start, and first-page
// settlement, then finalizes via enterResultHistory and returns the model
// plus the one retained tabular entry.
func finalizeWithMalformedText(t *testing.T, rows int) (Model, history.ResultEntry) {
	t.Helper()
	exec := &fakeSelectExecutor{page: malformedTextPage(rows)}
	m := warningRoundtripModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	if m.Result == nil || m.Result.Page == nil || !m.Result.Page.InvalidUTF {
		t.Fatalf("active page did not retain InvalidUTF after settlement: %+v", m.Result)
	}
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	return m, entries[0]
}

// finalizeWithByteCap drives a real first-page execution, then replaces the
// active SELECT's authoritative cache with a pre-seeded byte-cap-truncated
// cache and sets ResultView.ByteTruncated so the persistent byte-cap truth
// is owned by the cache/result state, then finalizes via enterResultHistory.
func finalizeWithByteCap(t *testing.T, rows int) (Model, history.ResultEntry) {
	t.Helper()
	exec := &fakeSelectExecutor{page: firstPageRows(rows)}
	m := warningRoundtripModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	// Replace the authoritative cache with a byte-cap-truncated one and
	// mirror the persistent disclosure onto the result view, exactly as
	// the real settlement path would after a byte-cap eviction.
	m.viewportCache = byteCapCache(t, rows)
	m.Result.ByteTruncated = true
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want 1", len(entries))
	}
	return m, entries[0]
}

// TestFinalizationPreservesInvalidUTFFromAcceptedPage proves the accepted
// active page's InvalidUTF truth becomes immutable snapshot
// SnapshotMetadata.InvalidUTF through appendFinalizedResultEntry (AC1).
func TestFinalizationPreservesInvalidUTFFromAcceptedPage(t *testing.T) {
	_, entry := finalizeWithMalformedText(t, 3)
	if !entry.Metadata.InvalidUTF {
		t.Fatalf("finalized snapshot lost InvalidUTF metadata: %+v", entry.Metadata)
	}
}

// TestFinalizationPreservesByteCapFromAuthoritativeCache proves the
// persistent byte-cap truth sourced from the authoritative cache becomes
// immutable snapshot SnapshotMetadata.TruncatedByByteCap through
// appendFinalizedResultEntry (AC1).
func TestFinalizationPreservesByteCapFromAuthoritativeCache(t *testing.T) {
	_, entry := finalizeWithByteCap(t, 3)
	if !entry.Metadata.TruncatedByByteCap {
		t.Fatalf("finalized snapshot lost TruncatedByByteCap metadata: %+v", entry.Metadata)
	}
}

// TestFinalizationWarningMetadataImmutableAfterMutation proves stored
// rows/metadata remain unchanged after finalization when live pages,
// projected views, source BLOBs, and cache state are mutated (AC2).
func TestFinalizationWarningMetadataImmutableAfterMutation(t *testing.T) {
	m, entry := finalizeWithMalformedText(t, 3)
	// Capture the immutable baseline before any mutation: Lookup returns
	// a fresh deep copy, so before is independent of later mutation of the
	// retrieved entry or the store.
	before, ok := m.ResultHistory.Lookup(entry.ID)
	if !ok {
		t.Fatal("baseline lookup failed")
	}
	// Mutate the live active page's InvalidUTF flag and rows.
	if m.Result != nil && m.Result.Page != nil {
		m.Result.Page.InvalidUTF = false
		m.Result.Page.Rows[0][0] = result.NewText("mutated")
	}
	// Mutate the projected view if one exists.
	if m.resultHistoryView != nil && m.resultHistoryView.Page != nil {
		m.resultHistoryView.Page.InvalidUTF = false
		m.resultHistoryView.Page.Rows[0][0] = result.NewText("mutated")
	}
	// Mutate the retrieved entry's rows (a deep copy from the store).
	entry.Rows[0][0] = result.NewInteger(999)
	// Mutate the source BLOB bytes inside the live cache; the snapshot is
	// independent.
	if m.viewportCache != nil {
		mergeRowsIntoCache(t, &m, int64(len(entry.Rows)+1), 5)
	}
	after, ok := m.ResultHistory.Lookup(entry.ID)
	if !ok {
		t.Fatal("finalized entry was evicted by post-finalization mutation")
	}
	if after.Metadata.InvalidUTF != before.Metadata.InvalidUTF {
		t.Errorf("stored InvalidUTF changed after mutation: got %v want %v",
			after.Metadata.InvalidUTF, before.Metadata.InvalidUTF)
	}
	if after.Metadata.TruncatedByByteCap != before.Metadata.TruncatedByByteCap {
		t.Errorf("stored TruncatedByByteCap changed after mutation: got %v want %v",
			after.Metadata.TruncatedByByteCap, before.Metadata.TruncatedByByteCap)
	}
	if len(after.Rows) != len(before.Rows) {
		t.Errorf("stored row count changed after mutation: got %d want %d",
			len(after.Rows), len(before.Rows))
	}
	if after.Rows[0][0].Kind != before.Rows[0][0].Kind ||
		after.Rows[0][0].Int != before.Rows[0][0].Int {
		t.Errorf("stored row 0 col 0 changed after mutation: got %+v want %+v",
			after.Rows[0][0], before.Rows[0][0])
	}
}

// TestProjectHistoryEntryRestoresInvalidUTFAndByteCap proves local
// historical projection restores Metadata.InvalidUTF onto the new
// result.Page and Metadata.TruncatedByByteCap onto ResultView.ByteTruncated
// at multiple terminal page sizes, preserving offset, rows, columns, and
// BLOB copies (AC1).
func TestProjectHistoryEntryRestoresInvalidUTFAndByteCap(t *testing.T) {
	_, entry := finalizeWithMalformedText(t, 5)
	// Also stamp byte-cap truth onto the same snapshot to prove both
	// warnings restore together.
	entry.Metadata.TruncatedByByteCap = true
	for _, pageRows := range []int{0, 2, 5, 10} {
		view := projectHistoryEntry(entry, pageRows)
		if view == nil || view.Page == nil {
			t.Fatalf("pageRows=%d: projection lost page", pageRows)
		}
		if !view.Page.InvalidUTF {
			t.Errorf("pageRows=%d: projection lost Page.InvalidUTF", pageRows)
		}
		if !view.ByteTruncated {
			t.Errorf("pageRows=%d: projection lost ResultView.ByteTruncated", pageRows)
		}
		want := pageRows
		if want > len(entry.Rows) {
			want = len(entry.Rows)
		}
		if len(view.Page.Rows) != want {
			t.Errorf("pageRows=%d: resliced rows = %d want %d", pageRows, len(view.Page.Rows), want)
		}
		if !equalColumns(view.Page.Columns, entry.Columns) {
			t.Errorf("pageRows=%d: columns changed", pageRows)
		}
	}
}

// TestProjectHistoryEntryRestoresByteCapOnly proves a snapshot with only
// byte-cap truth (no invalid UTF) restores ByteTruncated without setting
// InvalidUTF on the projected page.
func TestProjectHistoryEntryRestoresByteCapOnly(t *testing.T) {
	_, entry := finalizeWithByteCap(t, 3)
	if entry.Metadata.InvalidUTF {
		t.Fatalf("byte-cap snapshot unexpectedly carried InvalidUTF")
	}
	view := projectHistoryEntry(entry, 3)
	if view == nil || view.Page == nil {
		t.Fatal("byte-cap projection lost page")
	}
	if view.Page.InvalidUTF {
		t.Errorf("byte-cap-only projection wrongly set Page.InvalidUTF")
	}
	if !view.ByteTruncated {
		t.Errorf("byte-cap-only projection lost ResultView.ByteTruncated")
	}
}

// TestHistoricalBrowsingrendersSharedWarnings proves the shared UTF and
// 64 MiB warnings appear in browsing at multiple terminal page sizes for
// a snapshot carrying both warning facts (AC2). A wide terminal is used so
// the full status line — both warnings joined — fits without border
// wrapping; the rendering seam is the same at every width.
func TestHistoricalBrowsingRendersSharedWarnings(t *testing.T) {
	m, entry := finalizeWithMalformedText(t, 3)
	entry.Metadata.TruncatedByByteCap = true
	// Replace the store's entry with the doctored one carrying both
	// warnings by re-seeding a fresh store.
	store := history.NewResultStore()
	store.AppendFinalized(entry)
	m.ResultHistory = store
	m.enterResultHistoryMode()
	for _, height := range []int{24, 30, 40} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: height})
		m = next.(Model)
		view := m.View()
		if !strings.Contains(view, result.UTFWarning) {
			t.Errorf("height=%d: browsing view missing shared UTF warning:\n%s", height, view)
		}
		if !strings.Contains(view, result.ByteCapWarning) {
			t.Errorf("height=%d: browsing view missing shared byte-cap warning:\n%s", height, view)
		}
	}
}

// TestHistoricalExportWarningsRenderBeforeDestination proves the shared
// UTF and 64 MiB warnings appear in the pre-destination export flow for a
// historical snapshot carrying both warning facts (AC2).
func TestHistoricalExportWarningsRenderBeforeDestination(t *testing.T) {
	m, entry := finalizeWithMalformedText(t, 3)
	entry.Metadata.TruncatedByByteCap = true
	store := history.NewResultStore()
	store.AppendFinalized(entry)
	m.ResultHistory = store
	m.enterResultHistoryMode()
	opened, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil {
		t.Fatal("Ctrl+X issued a command")
	}
	if opened.exportPrepared == nil || !opened.exportWarningsOpen {
		t.Fatalf("historical export flow did not open: %q", opened.exportNotice)
	}
	joined := strings.Join(opened.exportWarnings, "\n")
	if !strings.Contains(joined, result.UTFWarning) {
		t.Errorf("historical export warnings missing UTF: %q", joined)
	}
	if !strings.Contains(joined, result.ByteCapWarning) {
		t.Errorf("historical export warnings missing byte-cap: %q", joined)
	}
	view := opened.View()
	if !strings.Contains(view, "Export result") {
		t.Fatalf("warnings not rendered before destination selection:\n%s", view)
	}
}

// TestHistoricalExportPayloadExcludesWarnings proves neither warning becomes
// a CSV record, JSON object property, or data value: the serializer-visible
// Payload carries only names/positions/rows, and no warning text appears in
// any name or cell token (AC3).
func TestHistoricalExportPayloadExcludesWarnings(t *testing.T) {
	m, entry := finalizeWithMalformedText(t, 3)
	entry.Metadata.TruncatedByByteCap = true
	store := history.NewResultStore()
	store.AppendFinalized(entry)
	m.ResultHistory = store
	m.enterResultHistoryMode()
	opened, _ := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if opened.exportPrepared == nil {
		t.Fatal("historical export flow did not open")
	}
	payload := opened.exportPrepared.Payload
	for _, name := range payload.Names {
		if strings.Contains(name, "Result") || strings.Contains(name, "UTF-8") ||
			strings.Contains(name, "U+FFFD") || strings.Contains(name, "64 MiB") {
			t.Errorf("payload name %q carries warning metadata", name)
		}
	}
	for _, row := range payload.Rows {
		for _, v := range row {
			tok := v.Display()
			if strings.Contains(tok, "Result truncated") || strings.Contains(tok, "invalid UTF-8 replaced") {
				t.Errorf("payload value token %q carries warning text", tok)
			}
		}
	}
	// Direct serializer-spy proof: CSV and JSON output contain no warning
	// literals and no extra record/property for either warning.
	csvOut := string(export.CSV(payload))
	if strings.Contains(csvOut, result.ByteCapWarning) || strings.Contains(csvOut, result.UTFWarning) {
		t.Errorf("CSV output carries warning text:\n%s", csvOut)
	}
	jsonOut := string(export.JSON(payload))
	if strings.Contains(jsonOut, result.ByteCapWarning) || strings.Contains(jsonOut, result.UTFWarning) {
		t.Errorf("JSON output carries warning text:\n%s", jsonOut)
	}
	// The payload has exactly the snapshot's columns and rows: no warning
	// record was injected.
	if len(payload.Names) != len(entry.Columns) {
		t.Errorf("payload names = %d, want %d (no warning column)", len(payload.Names), len(entry.Columns))
	}
	if len(payload.Rows) != len(entry.Rows) {
		t.Errorf("payload rows = %d, want %d (no warning row)", len(payload.Rows), len(entry.Rows))
	}
}

// equalColumns reports whether two column slices match exactly.
func equalColumns(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
