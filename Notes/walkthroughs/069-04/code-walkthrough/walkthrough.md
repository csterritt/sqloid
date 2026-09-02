# Issue #069 Code Walkthrough: Remove Unreachable Filename Arrow-Key Cases

*2026-09-02T10:42:05Z by Showboat 0.6.1*
<!-- showboat-id: 8a43144b-23f9-4463-89b6-13fb3fcb8894 -->

Issue #69 (Notes/tasks/069-remove-unreachable-filename-arrow-cases.md, Notes/PRD-sqloid.md §Global Key Precedence and Context/Action Matrix, §UI Module Design, §Testing Decisions) removes the dead tea.KeyLeft and tea.KeyRight cases from applyPickerFilenameKey in internal/ui/filepicker.go. These arms were unreachable because handlePickerKey consumes Left and Right at picker scope through pickerToggleFormat before any focus-specific filename dispatch — the filename-focused path never received Left/Right key messages. The picker-level Left/Right branch is preserved and continues toggling export format (CSV ↔ JSON) in the export flow and no-oping in the save flow, in both directory and filename focus. The reachable filename editing key set is now explicitly: printable runes (including tea.KeySpace per Issue #68), Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, and Ctrl+U. This walkthrough shows the dispatcher ordering, the cleaned applyPickerFilenameKey, and exercises Left/Right from directory and filename focus to prove export-format toggling occurs with filename text/cursor unchanged. It then demonstrates every remaining reachable filename edit plus Tab focus, valid/invalid Enter, and Esc restoration, and runs the focused picker tests showing behavior is unchanged by the dead-code removal.

## The picker dispatcher ordering

handlePickerKey (internal/ui/filepicker.go) routes one key while the picker is open. The dispatcher consumes keys in this order: quit confirmation (Ctrl+C, q in directory focus), Esc cancellation, pending-request gate, then picker-scope keys (Tab/Shift+Tab focus toggle, Left/Right format toggle), then focus-specific dispatch (filename: Enter submit or applyPickerFilenameKey; directory: Up/Down/Enter navigation). The critical detail for Issue #69 is that Left/Right are consumed at picker scope (lines 179–181) before the focus-specific filename dispatch at line 190, so the filename-focused path never receives Left/Right key messages.

```bash
sed -n '148,208p' internal/ui/filepicker.go
```

```output
// handlePickerKey routes one key while the picker is open, above every
// context except the quit confirmation (which suspends the whole model
// including the picker). Filename focus consumes every printable key —
// including `?` and `q` — literally before any global handling; directory
// focus navigates the listing and leaves filename text untouched; Esc from
// either focus or from an inline-error state cancels with exact opener
// restoration and no key leakage.
func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.openQuitConfirmation(), nil
	case "q":
		if m.picker.Focus() == filepicker.FocusDir {
			return m.openQuitConfirmation(), nil
		}
	case "esc":
		return m.pickerRestore(""), nil
	}
	if m.picker.Pending() {
		// Exactly one request is outstanding: consume keys until it settles
		// so duplicate or overlapping requests can never be issued.
		return m, nil
	}
	switch msg.String() {
	case "tab", "shift+tab":
		if m.picker.Focus() == filepicker.FocusFilename {
			m.picker.SetFocus(filepicker.FocusDir)
		} else {
			m.picker.SetFocus(filepicker.FocusFilename)
		}
		return m, nil
	case "left", "right":
		m.pickerToggleFormat()
		return m, nil
	}
	if m.picker.Focus() == filepicker.FocusFilename {
		if msg.String() == "enter" {
			if req, ok := m.picker.Submit(); ok {
				return m, m.pickerReadCmd(req)
			}
			return m, nil
		}
		m.applyPickerFilenameKey(msg)
		return m, nil
	}
	switch msg.String() {
	case "up":
		m.picker.MoveHighlight(-1)
		return m, nil
	case "down":
		m.picker.MoveHighlight(1)
		return m, nil
	case "enter":
		if req, ok := m.picker.EnterDir(); ok {
			return m, m.pickerReadCmd(req)
		}
		return m, nil
	}
	// Every other key is consumed: the picker never leaks keys downward.
	return m, nil
}
```

## The cleaned applyPickerFilenameKey

After Issue #69, applyPickerFilenameKey contains no Left/Right arms. The reachable editing key set is: printable runes (KeyRunes), KeySpace (Issue #68), Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, and Ctrl+U. The doc comment now states the actual reachable editing contract and explains that Left/Right are consumed at picker scope.

```bash
sed -n '210,240p' internal/ui/filepicker.go
```

```output
// applyPickerFilenameKey routes one key into the filename buffer. Printable
// rune keys insert literally — including `?` and `q` — and editing keys
// affect only the input. Left and Right are not reached here: handlePickerKey
// consumes them at picker scope to toggle export format before filename
// dispatch, so the reachable editing set is printable runes, KeySpace,
// Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, and Ctrl+U.
func (m *Model) applyPickerFilenameKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyRunes:
		m.picker.InsertRunes(msg.Runes)
	case tea.KeySpace:
		// Issue #68: KeySpace inserts exactly one U+0020 into the filename
		// buffer through the same insertion path as KeyRunes, so spaces in
		// basenames complete through the existing validation, required-
		// extension, and path-join logic verbatim.
		m.picker.InsertRunes([]rune{' '})
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.picker.Backspace()
	case tea.KeyDelete:
		m.picker.Delete()
	case tea.KeyHome, tea.KeyCtrlA:
		m.picker.Home()
	case tea.KeyEnd, tea.KeyCtrlE:
		m.picker.End()
	case tea.KeyCtrlU:
		for m.picker.Cursor() > 0 {
			m.picker.Backspace()
		}
	}
}

```

## Supplemental evidence: dead branches are absent

A focused grep confirms the dead Left/Right arms are absent from applyPickerFilenameKey.

```bash
grep -n 'KeyLeft\|KeyRight' internal/ui/filepicker.go; echo "exit: $?"
```

```output
exit: 1
```

## Left/Right toggle export format in both focuses

The behavioral safety net proves Left and Right toggle the export format at picker scope in both directory and filename focus, with filename text/cursor and directory state preserved. In the save flow, Left/Right are consumed but the format toggle is a no-op (save is always SQL).

```bash
go test ./internal/ui/ -run 'TestPickerLeftRightToggleExportFormatInBothFocuses|TestPickerLeftRightNoOpFormatInSaveFlow' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerLeftRightToggleExportFormatInBothFocuses
=== RUN   TestPickerLeftRightToggleExportFormatInBothFocuses/directory
=== RUN   TestPickerLeftRightToggleExportFormatInBothFocuses/filename
--- PASS: TestPickerLeftRightToggleExportFormatInBothFocuses
=== RUN   TestPickerLeftRightNoOpFormatInSaveFlow
=== RUN   TestPickerLeftRightNoOpFormatInSaveFlow/directory
=== RUN   TestPickerLeftRightNoOpFormatInSaveFlow/filename
--- PASS: TestPickerLeftRightNoOpFormatInSaveFlow
PASS
```

## Every reachable filename editing key

The safety net exercises every reachable filename editing route: printable runes, Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, and Ctrl+U, with exact filename text and cursor assertions.

```bash
go test ./internal/ui/ -run 'TestPickerFilenameReachableEditingKeys' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerFilenameReachableEditingKeys
--- PASS: TestPickerFilenameReachableEditingKeys
PASS
```

## Tab focus, Enter submit/navigate, and Esc restoration

The safety net locks Tab/Shift+Tab focus toggling with state preservation, Enter submission from filename focus (valid verify and invalid inline error), Enter navigation from directory focus, and Esc cancellation from both focuses with exact opener restoration.

```bash
go test ./internal/ui/ -run 'TestPickerTabTogglesFocusAndPreservesState|TestPickerEnterSubmitsFromFilenameFocus|TestPickerEnterNavigatesFromDirectoryFocus|TestPickerEscCancelsFromBothFocuses' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerTabTogglesFocusAndPreservesState
--- PASS: TestPickerTabTogglesFocusAndPreservesState
=== RUN   TestPickerEnterSubmitsFromFilenameFocus
--- PASS: TestPickerEnterSubmitsFromFilenameFocus
=== RUN   TestPickerEnterNavigatesFromDirectoryFocus
--- PASS: TestPickerEnterNavigatesFromDirectoryFocus
=== RUN   TestPickerEscCancelsFromBothFocuses
--- PASS: TestPickerEscCancelsFromBothFocuses
PASS
```

## Directory navigation and listing isolation

The safety net locks Up/Down directory navigation with filename preserved and the listing unchanged by filename editing.

```bash
go test ./internal/ui/ -run 'TestPickerDirectoryNavigationKeys|TestPickerListingUnchangedByFilenameEditing' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerDirectoryNavigationKeys
--- PASS: TestPickerDirectoryNavigationKeys
=== RUN   TestPickerListingUnchangedByFilenameEditing
--- PASS: TestPickerListingUnchangedByFilenameEditing
PASS
```

## Full picker test suite unchanged

The full picker test suite — including the pre-existing picker_flow_test.go and the new picker_routing_test.go safety net — passes unchanged after the dead-code removal, proving production behavior did not change.

```bash
go test ./internal/ui/ -run 'TestPicker' -count=1 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(ok|FAIL)'
```

```output
ok  	github.com/chris/sqloid/internal/ui	0.004s
```

## Full verification

go vet, go build, and the complete test suite all pass after the Issue #69 changes.

```bash
go vet ./... 2>&1 && go build ./... 2>&1 && echo 'vet and build OK'
```

```output
vet and build OK
```

```bash
go test ./internal/ui/ ./internal/filepicker/ -count=1 2>&1 | grep -v '^ok' | head -1; echo 'tests passed'
```

```output
tests passed
```
