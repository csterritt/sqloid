// Issue #69 Task 1 (REFACTOR): behavioral safety net locking every reachable
// picker key-routing path before dead filename arrow branches are removed.
// Left and Right are consumed at picker scope in both directory and filename
// focus to toggle export format (export flow only; save flow no-ops the
// toggle) and therefore never reach filename mutation. Every remaining
// reachable filename editing route — printable runes, Backspace/Ctrl+H,
// Delete, Home/Ctrl+A, End/Ctrl+E, Ctrl+U — plus Tab, Enter, and Esc at the
// picker dispatcher is exercised with exact state assertions. This safety
// net must be green before and after the dead-code removal.

package ui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
)

// exportPickerModel builds an export-flow picker seeded against the fake
// boundary at /work with the given initial format, the start listing
// settled, and filename focus if focusFilename is true.
func exportPickerModel(t *testing.T, f *pickerFakeFS, format filepicker.Format, focusFilename bool) Model {
	t.Helper()
	if f.fail == nil {
		f.fail = map[string]error{}
	}
	if f.dirs == nil {
		f.dirs = map[string][]pickerFakeEntry{}
	}
	m := New()
	m.PickerFS = f
	m.PickerStart = "/work"
	m.SaveFS = newSaveFlowFakeFS()
	m.exportPrepared = &export.Capture{Payload: export.Payload{Names: []string{"c"}}}
	m.exportFormat = format
	m.exportWarnings = []string{"Result is complete"}
	m.exportWarningsOpen = true
	opened, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !opened.pickerOpen || opened.picker.Format() != format {
		t.Fatalf("export picker not opened with format %q: open=%v format=%q",
			format, opened.pickerOpen, opened.picker.Format())
	}
	settled, _ := runList(t, opened, cmd)
	if focusFilename {
		settled, _ = pressKey(settled, tea.KeyMsg{Type: tea.KeyTab})
		if settled.picker.Focus() != filepicker.FocusFilename {
			t.Fatal("Tab did not move focus to filename")
		}
	}
	return settled
}

// TestPickerLeftRightToggleExportFormatInBothFocuses requires Left and Right
// to toggle the export format through pickerToggleFormat at picker scope in
// both directory and filename focus, preserving filename text/cursor and
// directory state, and never reaching filename mutation.
func TestPickerLeftRightToggleExportFormatInBothFocuses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		focus     filepicker.Focus
		focusName string
	}{
		{"directory", filepicker.FocusDir, "dir"},
		{"filename", filepicker.FocusFilename, "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := pickerNewFakeFS()
			f.dirs["/work"] = []pickerFakeEntry{{"out", true}, {"sub", true}}
			// Seed filename text so we can prove Left/Right never mutate it.
			p := exportPickerModel(t, f, filepicker.FormatCSV, false)
			p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
			p, _ = pressKey(p, runeKey("draft"))
			if tc.focus == filepicker.FocusDir {
				p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // back to dir
			}
			if got := p.picker.Focus(); got != tc.focus {
				t.Fatalf("setup focus = %v, want %v", got, tc.focus)
			}
			filenameBefore := p.picker.Filename()
			cursorBefore := p.picker.Cursor()
			dirBefore := p.picker.CurrentDir()
			highlightBefore := p.picker.Highlight()
			formatBefore := p.exportFormat

			// Left toggles CSV → JSON.
			p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyLeft})
			if got := p.exportFormat; got != filepicker.FormatJSON {
				t.Fatalf("Left: exportFormat = %q, want json (toggled from csv)", got)
			}
			if got := p.picker.Filename(); got != filenameBefore {
				t.Errorf("Left: filename drifted %q → %q", filenameBefore, got)
			}
			if got := p.picker.Cursor(); got != cursorBefore {
				t.Errorf("Left: cursor drifted %d → %d", cursorBefore, got)
			}
			if got := p.picker.CurrentDir(); got != dirBefore {
				t.Errorf("Left: directory drifted %q → %q", dirBefore, got)
			}
			if got := p.picker.Highlight(); got != highlightBefore {
				t.Errorf("Left: highlight drifted %d → %d", highlightBefore, got)
			}

			// Right toggles JSON → CSV.
			p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyRight})
			if got := p.exportFormat; got != filepicker.FormatCSV {
				t.Fatalf("Right: exportFormat = %q, want csv (toggled back from json)", got)
			}
			if got := p.picker.Filename(); got != filenameBefore {
				t.Errorf("Right: filename drifted %q → %q", filenameBefore, got)
			}
			if got := p.picker.Cursor(); got != cursorBefore {
				t.Errorf("Right: cursor drifted %d → %d", cursorBefore, got)
			}

			// Right again toggles CSV → JSON.
			p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyRight})
			if got := p.exportFormat; got != filepicker.FormatJSON {
				t.Fatalf("Right again: exportFormat = %q, want json", got)
			}
			_ = formatBefore
		})
	}
}

// TestPickerLeftRightNoOpFormatInSaveFlow requires Left and Right to be
// consumed at picker scope in the save flow without toggling format (save is
// always SQL) and without reaching filename mutation.
func TestPickerLeftRightNoOpFormatInSaveFlow(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	_, p := savePickerModel(t, f)
	// Seed filename text from filename focus, then return to directory focus.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab})
	p, _ = pressKey(p, runeKey("draft"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // back to dir

	for _, focus := range []filepicker.Focus{filepicker.FocusDir, filepicker.FocusFilename} {
		t.Run(focusName(focus), func(t *testing.T) {
			cur := p
			if cur.picker.Focus() != focus {
				cur, _ = pressKey(cur, tea.KeyMsg{Type: tea.KeyTab})
			}
			if got := cur.picker.Focus(); got != focus {
				t.Fatalf("setup focus = %v, want %v", got, focus)
			}
			filenameBefore := cur.picker.Filename()
			cursorBefore := cur.picker.Cursor()
			formatBefore := cur.picker.Format()

			cur, _ = pressKey(cur, tea.KeyMsg{Type: tea.KeyLeft})
			if got := cur.picker.Format(); got != formatBefore {
				t.Fatalf("Left: save format drifted %q → %q", formatBefore, got)
			}
			if got := cur.picker.Filename(); got != filenameBefore {
				t.Errorf("Left: filename drifted %q → %q", filenameBefore, got)
			}
			if got := cur.picker.Cursor(); got != cursorBefore {
				t.Errorf("Left: cursor drifted %d → %d", cursorBefore, got)
			}

			cur, _ = pressKey(cur, tea.KeyMsg{Type: tea.KeyRight})
			if got := cur.picker.Format(); got != formatBefore {
				t.Fatalf("Right: save format drifted %q → %q", formatBefore, got)
			}
			if got := cur.picker.Filename(); got != filenameBefore {
				t.Errorf("Right: filename drifted %q → %q", filenameBefore, got)
			}
			if got := cur.picker.Cursor(); got != cursorBefore {
				t.Errorf("Right: cursor drifted %d → %d", cursorBefore, got)
			}
		})
	}
}

// focusName returns a human-readable name for a filepicker.Focus.
func focusName(f filepicker.Focus) string {
	switch f {
	case filepicker.FocusDir:
		return "directory"
	case filepicker.FocusFilename:
		return "filename"
	default:
		return "unknown"
	}
}

// TestPickerFilenameReachableEditingKeys exercises every remaining reachable
// filename editing route through Model.Update: printable runes,
// Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, and Ctrl+U, requiring
// unchanged edit behavior with exact filename text and cursor assertions.
func TestPickerFilenameReachableEditingKeys(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	_, p := savePickerModel(t, f)
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus

	// Printable runes insert literally at the cursor.
	p, _ = pressKey(p, runeKey("abc"))
	if got := p.picker.Filename(); got != "abc" {
		t.Fatalf("printable: filename = %q, want \"abc\"", got)
	}
	if got := p.picker.Cursor(); got != 3 {
		t.Fatalf("printable: cursor = %d, want 3", got)
	}

	// Home/Ctrl+A move cursor to start.
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyHome},
		{Type: tea.KeyCtrlA},
	} {
		p, _ = pressKey(p, msg)
		if got := p.picker.Cursor(); got != 0 {
			t.Fatalf("%v: cursor = %d, want 0", msg.Type, got)
		}
	}

	// End/Ctrl+E move cursor to end.
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyEnd},
		{Type: tea.KeyCtrlE},
	} {
		p, _ = pressKey(p, msg)
		if got := p.picker.Cursor(); got != 3 {
			t.Fatalf("%v: cursor = %d, want 3", msg.Type, got)
		}
	}

	// Backspace deletes the rune before the cursor.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := p.picker.Filename(); got != "ab" {
		t.Fatalf("Backspace: filename = %q, want \"ab\"", got)
	}
	if got := p.picker.Cursor(); got != 2 {
		t.Fatalf("Backspace: cursor = %d, want 2", got)
	}

	// Ctrl+H also deletes the rune before the cursor.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlH})
	if got := p.picker.Filename(); got != "a" {
		t.Fatalf("Ctrl+H: filename = %q, want \"a\"", got)
	}
	if got := p.picker.Cursor(); got != 1 {
		t.Fatalf("Ctrl+H: cursor = %d, want 1", got)
	}

	// Delete removes the rune under the cursor.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyHome}) // cursor at 0
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDelete})
	if got := p.picker.Filename(); got != "" {
		t.Fatalf("Delete: filename = %q, want empty", got)
	}
	if got := p.picker.Cursor(); got != 0 {
		t.Fatalf("Delete: cursor = %d, want 0", got)
	}

	// Ctrl+U clears the whole buffer.
	p, _ = pressKey(p, runeKey("hello"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := p.picker.Filename(); got != "" {
		t.Fatalf("Ctrl+U: filename = %q, want empty", got)
	}
	if got := p.picker.Cursor(); got != 0 {
		t.Fatalf("Ctrl+U: cursor = %d, want 0", got)
	}
}

// TestPickerTabTogglesFocusAndPreservesState requires Tab and Shift+Tab to
// toggle between directory and filename focus while preserving filename text,
// cursor, directory, and highlight state.
func TestPickerTabTogglesFocusAndPreservesState(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}, {"sub", true}}
	_, p := savePickerModel(t, f)
	if got := p.picker.Focus(); got != filepicker.FocusDir {
		t.Fatalf("initial focus = %v, want directory", got)
	}
	// Seed filename text.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // → filename
	p, _ = pressKey(p, runeKey("name"))
	if got := p.picker.Filename(); got != "name" {
		t.Fatalf("seed: filename = %q, want \"name\"", got)
	}
	// Tab back to directory: filename text preserved.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab})
	if got := p.picker.Focus(); got != filepicker.FocusDir {
		t.Fatalf("Tab: focus = %v, want directory", got)
	}
	if got := p.picker.Filename(); got != "name" {
		t.Fatalf("Tab: filename drifted %q", got)
	}
	// Shift+Tab back to filename: focus and text preserved.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := p.picker.Focus(); got != filepicker.FocusFilename {
		t.Fatalf("Shift+Tab: focus = %v, want filename", got)
	}
	if got := p.picker.Filename(); got != "name" {
		t.Fatalf("Shift+Tab: filename drifted %q", got)
	}
	if got := p.picker.Cursor(); got != 4 {
		t.Fatalf("Shift+Tab: cursor = %d, want 4", got)
	}
}

// TestPickerEnterSubmitsFromFilenameFocus requires Enter from filename focus
// to submit the filename through validation, issuing a verify request for a
// valid basename, and producing an inline error for an invalid one.
func TestPickerEnterSubmitsFromFilenameFocus(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	_, p := savePickerModel(t, f)
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus

	// Empty submit: inline error, no verify request.
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("empty Enter issued a verify request")
	}
	if p.picker.Error() == nil {
		t.Fatal("empty Enter produced no inline error")
	}

	// Valid submit: verify request issued.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlU})
	p, _ = pressKey(p, runeKey("report"))
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("valid Enter issued no verify request")
	}
	p, msg := runList(t, p, cmd)
	if v, ok := msg.(PickerVerifyMsg); !ok || v.Path != "/work/report.sql" {
		t.Fatalf("verify = %+v ok=%v, want /work/report.sql", msg, ok)
	}
}

// TestPickerEnterNavigatesFromDirectoryFocus requires Enter from directory
// focus to enter the highlighted directory, issuing a navigation request.
func TestPickerEnterNavigatesFromDirectoryFocus(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	f.dirs["/work/out"] = []pickerFakeEntry{}
	_, p := savePickerModel(t, f)
	// Directory focus is the default; highlight starts at .. (index 0).
	// Move down to "out".
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if got := p.picker.Highlighted(); got != "out" {
		t.Fatalf("setup: highlighted = %q, want out", got)
	}
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on directory issued no navigation request")
	}
	p, _ = runList(t, p, cmd)
	if got := p.picker.CurrentDir(); got != "/work/out" {
		t.Fatalf("Enter: currentDir = %q, want /work/out", got)
	}
}

// TestPickerEscCancelsFromBothFocuses requires Esc to cancel the picker with
// exact opener restoration from both directory and filename focus.
func TestPickerEscCancelsFromBothFocuses(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	opener, p := savePickerModel(t, f)
	baseline := openerFingerprint(opener)

	// Esc from directory focus.
	escDir, _ := pressKey(p, tea.KeyMsg{Type: tea.KeyEscape})
	if escDir.pickerOpen {
		t.Fatal("directory-focus Esc left the picker open")
	}
	if fp := openerFingerprint(escDir); fp != baseline {
		t.Errorf("directory-focus Esc drifted:\n%s\nvs\n%s", fp, baseline)
	}

	// Esc from filename focus with half-typed text.
	_, p2 := savePickerModel(t, f)
	p2, _ = pressKey(p2, tea.KeyMsg{Type: tea.KeyTab})
	p2, _ = pressKey(p2, runeKey("draft"))
	escFile, _ := pressKey(p2, tea.KeyMsg{Type: tea.KeyEscape})
	if escFile.pickerOpen {
		t.Fatal("filename-focus Esc left the picker open")
	}
	if fp := openerFingerprint(escFile); fp != baseline {
		t.Errorf("filename-focus Esc drifted:\n%s\nvs\n%s", fp, baseline)
	}
}

// TestPickerDirectoryNavigationKeys requires Up/Down to move the highlight
// in directory focus without touching the filename buffer.
func TestPickerDirectoryNavigationKeys(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"a", true}, {"b", true}, {"c", true}}
	_, p := savePickerModel(t, f)
	// Seed filename text to prove navigation never touches it.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab})
	p, _ = pressKey(p, runeKey("keep"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // back to dir

	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if got := p.picker.Highlighted(); got != "a" {
		t.Fatalf("Down: highlighted = %q, want a", got)
	}
	if got := p.picker.Filename(); got != "keep" {
		t.Fatalf("Down: filename drifted %q", got)
	}

	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if got := p.picker.Highlighted(); got != "b" {
		t.Fatalf("Down: highlighted = %q, want b", got)
	}

	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	if got := p.picker.Highlighted(); got != "a" {
		t.Fatalf("Up: highlighted = %q, want a", got)
	}

	// Up again moves to ".." (index 0).
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	if got := p.picker.Highlighted(); got != ".." {
		t.Fatalf("Up: highlighted = %q, want ..", got)
	}

	// Up at top is a boundary no-op.
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatal("Up at top issued a request")
	}
	if got := p.picker.Highlight(); got != 0 {
		t.Fatalf("Up at top: highlight = %d, want 0", got)
	}
}

// TestPickerListingUnchangedByFilenameEditing requires filename editing to
// never touch the directory listing.
func TestPickerListingUnchangedByFilenameEditing(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}, {"sub", true}}
	_, p := savePickerModel(t, f)
	wantListing := []string{"..", "out", "sub"}
	if got := p.picker.Listing(); !reflect.DeepEqual(got, wantListing) {
		t.Fatalf("setup: listing = %q, want %q", got, wantListing)
	}
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
	p, _ = pressKey(p, runeKey("file"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyHome})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyEnd})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyBackspace})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDelete})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := p.picker.Listing(); !reflect.DeepEqual(got, wantListing) {
		t.Fatalf("listing changed by filename editing: %q, want %q", got, wantListing)
	}
}
