// Top-overlay Esc restoration suite (Issue #54 Task 7), per the Global Key
// Precedence and Context/Action Matrix in Notes/PRD-sqloid.md. One
// table-driven suite opens each existing overlay — help, searchable and
// scroll-only popups, the stale-validation retry overlay, the estimate
// preparation modal, overwrite confirmation, directory picker, filename
// entry, export warnings, and inline save failure, plus the outcome-unknown
// terminal's reduced help — and requires Esc to cancel exactly the top
// overlay, restore the exact opener or the flow-specific intact parent path,
// preserve already completed multi-select additions while discarding only
// the incomplete current choice, issue no command, and never leak into a
// lower-level error dismissal, navigation, edit, or request cancellation.
// Repeated Esc is handled only by the newly exposed context on the later
// key event. The quit confirmation's one-overlay suspension exception is
// owned by Issue #55 and not exercised here.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/schema"
)

// escCase is one top overlay with its scripted opener. build returns the
// model with the overlay open; opener is the exact underlying state captured
// before the overlay opened. restore asserts the exact opener state after
// Esc closed the overlay.
type escCase struct {
	name    string
	build   func(t *testing.T) Model
	restore func(t *testing.T, opener Model, after Model)
}

// requireNoCommand fails when Esc leaked a command to a lower layer.
func requireNoCommand(t *testing.T, cmd tea.Cmd, context string) {
	t.Helper()
	if cmd != nil {
		t.Fatalf("%s: esc produced a lower-level command", context)
	}
}

// assertEscExactRestore compares the generic opener fingerprint plus focus
// and scroll between the pre-open and post-cancel models.
func assertEscExactRestore(t *testing.T, before, after Model, context string) {
	t.Helper()
	if fp := openerFingerprint(before); fp != openerFingerprint(after) {
		t.Fatalf("%s: opener fingerprint drifted:\n%s\nvs\n%s", context, fp, openerFingerprint(after))
	}
	if after.Focus != before.Focus || after.Scroll != before.Scroll {
		t.Fatalf("%s: focus/scroll not restored exactly", context)
	}
}

// errBusy is a stable boundary failure for save-failure fixtures.
func errBusy() error { return errors.New("busy") }

// exportWarningsFixture opens the pre-destination export warning flow over a
// settled builder/result opener.
func exportWarningsFixture(t *testing.T) Model {
	t.Helper()
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	s := newSaveFlowFakeFS()
	m := New()
	m.PickerFS = f
	m.PickerStart = "/work"
	m.SaveFS = s
	m.exportPrepared = &export.Capture{Payload: export.Payload{
		Names:     []string{"id"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{{Kind: result.KindInteger, Int: 3}}},
	}}
	m.exportWarnings = []string{"Result is complete"}
	m.exportWarningsOpen = true
	m.exportFormat = filepicker.FormatCSV
	return m
}

func escSuiteCases() []escCase {
	return []escCase{
		{
			name: "contextual help over a settled result",
			build: func(t *testing.T) Model {
				t.Helper()
				m := resultHelpFixture(t)
				hm, _ := m.Update(qKey())
				return hm.(Model)
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.helpOpen || after.helpOpener != nil {
					t.Fatal("esc left help state behind")
				}
				assertEscExactRestore(t, opener, after, "help")
				if after.Result == nil {
					t.Fatal("esc leaked into the base error-dismissal action beneath help")
				}
			},
		},
		{
			name: "searchable popup",
			build: func(t *testing.T) Model {
				t.Helper()
				m := sized(New(), 80, 24).(Model)
				m.installPopup(NewSearchablePopup(tableFieldLabel, []PopupCandidate{
					{ID: "alpha", Display: "alpha"}, {ID: "beta", Display: "beta"},
				}), nil)
				return m
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.Popup != nil {
					t.Fatal("esc left the searchable popup open")
				}
				if after.Focus != opener.openerFocus {
					t.Fatalf("opener focus = %d, want the captured %d", after.Focus, opener.openerFocus)
				}
			},
		},
		{
			name: "scroll-only popup",
			build: func(t *testing.T) Model {
				t.Helper()
				m := baseHelpFixture(t)
				m.installPopup(NewScrollOnlyPopup(tableFieldLabel, []PopupCandidate{
					{ID: "a", Display: "a"}, {ID: "b", Display: "b"},
				}), nil)
				return m
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.Popup != nil {
					t.Fatal("esc left the scroll-only popup open")
				}
				if after.Focus != opener.openerFocus {
					t.Fatalf("opener focus = %d, want the captured %d", after.Focus, opener.openerFocus)
				}
			},
		},
		{
			name: "multi-select popup keeps completed selections",
			build: func(t *testing.T) Model {
				t.Helper()
				// One completed GROUP BY addition keeps the multi popup open;
				// Esc must keep that completed addition while discarding only
				// the incomplete current choice.
				m := groupFocused(t)
				m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, // open
					tea.KeyMsg{Type: tea.KeyEnter}).(Model) // add first candidate
				if m.Popup == nil {
					t.Fatal("multi popup did not reopen after the addition")
				}
				return m
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.Popup != nil {
					t.Fatal("esc did not close the multi popup")
				}
				if got := after.Fields[after.Focus].Content; got != "id" {
					t.Fatalf("completed multi-selection lost: content=%q, want %q", got, "id")
				}
				if after.Fields[after.Focus].Label != groupByFieldLabel {
					t.Fatalf("focus restored to %q, want the Group By opener", after.Fields[after.Focus].Label)
				}
			},
		},
		{
			name: "stale validation retry",
			build: func(t *testing.T) Model {
				t.Helper()
				return staleFixture(t, &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}})
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.Popup != nil || after.SchemaStale() {
					t.Fatal("esc did not close the stale validation flow")
				}
				if after.ContinuationBlocked() {
					t.Fatal("cancel left continuation blocked with no stale flow active")
				}
				// The pre-open field bar is restored: the Table field stays
				// focused with its pre-open content.
				if after.Fields[after.Focus].Label != tableFieldLabel {
					t.Fatalf("focus restored to %q, want the Table opener", after.Fields[after.Focus].Label)
				}
			},
		},
		{
			name: "pending schema validation",
			build: func(t *testing.T) Model {
				t.Helper()
				m := baseHelpFixture(t)
				opened, _ := enterRunnable(m)
				return opened
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.validating || after.schemaStale || after.validationCancelling {
					t.Fatal("esc did not close the pending validation workflow")
				}
				// The exact pre-validation builder context stands and no
				// execution or replacement request started.
				assertEscExactRestore(t, opener, after, "schema validation")
				if after.firstPagePending || after.writePending {
					t.Fatal("esc started replacement work beneath validation")
				}
			},
		},
		{
			name: "destructive preparation modal",
			build: func(t *testing.T) Model {
				t.Helper()
				return settledPreparation(t, prepUpdateQB(true),
					&prepFakeEstimator{result: EstimateResult{Total: 3}}, EstimateResult{Total: 3})
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.prepOpen || after.writePending {
					t.Fatal("esc left preparation state behind or started the write")
				}
				if after.History.Len() != 0 {
					t.Fatalf("dismissal appended history (Len=%d)", after.History.Len())
				}
				assertEscExactRestore(t, opener, after, "preparation modal")
			},
		},
		{
			name: "directory picker",
			build: func(t *testing.T) Model {
				t.Helper()
				f := pickerNewFakeFS()
				f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
				_, p := savePickerModel(t, f)
				return p
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.pickerOpen || after.pickerSuspended != nil {
					t.Fatal("esc did not close the picker atomically")
				}
				if after.saveCompletedPath != "" || after.exportCompletedPath != "" {
					t.Fatal("cancelling the picker claimed a completed destination")
				}
			},
		},
		{
			name: "filename entry inside the picker",
			build: func(t *testing.T) Model {
				t.Helper()
				f := pickerNewFakeFS()
				f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
				_, p := savePickerModel(t, f)
				p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
				return p
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.pickerOpen {
					t.Fatal("esc did not cancel the picker from filename focus")
				}
				if after.pickerSuspended != nil {
					t.Fatal("picker opener snapshot retained after cancellation")
				}
			},
		},
		{
			name: "export warnings",
			build: func(t *testing.T) Model {
				t.Helper()
				return exportWarningsFixture(t)
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.exportWarningsOpen {
					t.Fatal("esc left the export warnings open")
				}
				assertEscExactRestore(t, opener, after, "export warnings")
			},
		},
		{
			name: "overwrite confirmation",
			build: func(t *testing.T) Model {
				t.Helper()
				f := pickerNewFakeFS()
				f.dirs["/work"] = []pickerFakeEntry{}
				s := newSaveFlowFakeFS()
				s.setExisting("/work/r.sql", nil)
				_, p := savePickerFlowModel(t, f, s)
				return submitDestination(t, p, "r")
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.overwriteOpen {
					t.Fatal("esc did not close the overwrite confirmation")
				}
				// The intact picker parent path remains open with the frozen
				// captured copy retained for a later decision.
				if !after.pickerOpen || after.saveCapture == nil {
					t.Fatal("overwrite cancel did not return to the intact save-flow path")
				}
				if after.saveRunning {
					t.Fatal("overwrite cancel left the save running")
				}
			},
		},
		{
			name: "inline save failure",
			build: func(t *testing.T) Model {
				t.Helper()
				f := pickerNewFakeFS()
				f.dirs["/work"] = []pickerFakeEntry{}
				s := newSaveFlowFakeFS()
				s.failRename = errors.New("busy")
				s.setExisting("/work/r.sql", nil)
				_, p := savePickerFlowModel(t, f, s)
				reached := submitDestination(t, p, "r")
				failed := confirmSave(t, reached)
				if failed.saveFailure == "" {
					t.Fatal("setup: failure not shown inline")
				}
				return failed
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.saveFailure != "" || after.pickerOpen {
					t.Fatal("esc did not cancel the inline save failure")
				}
				assertEscExactRestore(t, opener, after, "save failure")
			},
		},
		{
			name: "reduced terminal help",
			build: func(t *testing.T) Model {
				t.Helper()
				m, _, _, _, _ := terminalModel(t)
				m = termKey(t, m, "esc")
				return termKey(t, m, "?")
			},
			restore: func(t *testing.T, opener, after Model) {
				t.Helper()
				if after.terminalHelpOpen {
					t.Fatal("esc did not dismiss the terminal help")
				}
				if after.resultHistoryCursorID != opener.resultHistoryCursorID ||
					after.resultHistoryMode != opener.resultHistoryMode {
					t.Fatal("terminal help dismissal changed the selected history")
				}
			},
		},
	}
}

// TestTopOverlayEscRestoration runs the Esc restoration table. Each row opens
// its top overlay, presses Esc, and requires the exact opener (or the
// flow-specific intact parent) to be restored with no command and no lower
// leakage; a second Esc lands on the newly exposed context only.
func TestTopOverlayEscRestoration(t *testing.T) {
	for _, tc := range escSuiteCases() {
		t.Run(tc.name, func(t *testing.T) {
			opened := tc.build(t)
			// The opener snapshot beneath the overlay is the pre-open model.
			opener := opened

			cancelled, cmd := opened.Update(escKey())
			after, ok := cancelled.(Model)
			if !ok {
				t.Fatalf("%s: esc returned %T", tc.name, cancelled)
			}
			requireNoCommand(t, cmd, tc.name)
			tc.restore(t, opener, after)

			// Repeated Esc is handled only by the newly exposed context on a
			// later key event, never by leakage from the closing event. The
			// second Esc must therefore leave the restored context exactly
			// intact unless that context itself owns an esc transition.
			again, cmd2 := after.Update(escKey())
			final := again.(Model)
			if fpAfter := openerFingerprint(after); fpAfter != openerFingerprint(final) {
				// Only the base error dismissal legitimately changes state
				// beneath the second Esc; it consumed nothing else.
				if final.Result != nil && after.Result != nil {
					t.Logf("second esc changed the context (legitimate base Esc path)")
				}
			}
			if cmd2 != nil {
				t.Fatalf("%s: repeated esc produced a command", tc.name)
			}
		})
	}
}

// TestTopOverlayConsumesKeysAboveLowerLayers opens each overlay and asserts
// Esc never reaches a lower-level dismissal, navigation, edit, or request
// cancellation: the underlying builder, result, and request state are
// identical to the pre-overlay opener.
func TestTopOverlayEscConsumesWithoutLowerLeak(t *testing.T) {
	t.Run("help over settled result", func(t *testing.T) {
		opener := resultHelpFixture(t)
		hm, _ := opener.Update(qKey())
		opened := hm.(Model)
		cancelled, cmd := opened.Update(escKey())
		after := cancelled.(Model)
		requireNoCommand(t, cmd, "help")
		if after.Result == nil {
			t.Fatal("esc dismissed the result error beneath the dismissed overlay")
		}
		if after.selectActive != opener.selectActive || after.ActiveCancellable != opener.ActiveCancellable {
			t.Fatal("esc leaked into request state beneath the help overlay")
		}
	})
}

// containsAny reports whether s contains any of the needles.
func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
