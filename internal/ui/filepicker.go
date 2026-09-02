// Save/export destination picker composition inside the TUI (Issue #52),
// per the File picker decision and the Global Key Precedence and
// Context/Action Matrix in Notes/PRD-sqloid.md. The model-independent
// directory/filename state lives in internal/filepicker; this file only
// opens the picker for the Ctrl+S query-save and Ctrl+X CSV/JSON export
// openers, routes focused keys ahead of global handling, dispatches the
// filepicker requests as tea.Cmd functions, and restores the exact opener
// snapshot atomically on Esc and on successful completion. The picker flow
// starts at the process working directory supplied at open, starts no
// database work, serializes nothing, and touches no file: persistence,
// overwrite confirmation, and atomic temp-and-rename saves belong to their
// owning later issues.

package ui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chris/sqloid/internal/filepicker"
)

// pickerFlow identifies which opener owns the open picker.
type pickerFlow int

const (
	// pickerFlowSave is the Ctrl+S query-save opener.
	pickerFlowSave pickerFlow = iota + 1
	// pickerFlowExport is the Ctrl+X result-export opener.
	pickerFlowExport
)

// PickerListMsg answers one issued navigation listing request. The picker
// ignores responses whose attempt identity no longer matches.
type PickerListMsg struct {
	Path    string
	Attempt uint64
	Dirs    []string
	Err     error
}

// PickerVerifyMsg answers one issued destination-verification request.
type PickerVerifyMsg struct {
	Path    string
	Attempt uint64
	Err     error
}

// openPicker suspends the exact opener model, opens the picker at the
// process working directory (captured here through the injected boundary),
// and returns the command that issues the start listing. PickerFS nil means
// the real filesystem; PickerStart empty means os.Getwd. Both exist only as
// test seams for the fake boundary.
func (m *Model) openPicker(flow pickerFlow, format filepicker.Format) tea.Cmd {
	snap := *m
	snap.pickerOpen = false
	snap.picker = filepicker.Model{}
	snap.pickerSuspended = nil
	snap.saveCompletedPath = ""
	snap.exportCompletedPath = ""
	m.pickerSuspended = &snap
	m.pickerFlowKind = flow

	fs := m.PickerFS
	if fs == nil {
		fs = filepicker.OSFS{}
	}
	start := m.PickerStart
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			// Without a start directory the picker cannot begin; the
			// opening is abandoned and the exact opener is restored.
			*m = snap
			return nil
		}
		start = wd
	}
	pm, req := filepicker.Start(fs, start, format)
	m.picker = pm
	m.pickerOpen = true
	return m.pickerReadCmd(req)
}

// pickerReadCmd wraps one filepicker.NavRequest into a tea.Cmd that runs the
// blocking filesystem read outside Update and returns the matching message.
func (m Model) pickerReadCmd(req filepicker.NavRequest) tea.Cmd {
	fs := m.PickerFS
	if fs == nil {
		fs = filepicker.OSFS{}
	}
	readPath := req.Path
	if req.Verify {
		// Destination verification checks that the destination's directory
		// is still navigable at save time; the entries themselves are unused.
		readPath = filepath.Dir(req.Path)
	}
	return func() tea.Msg {
		entries, err := fs.ReadDir(readPath)
		if err != nil {
			if req.Verify {
				return PickerVerifyMsg{Path: req.Path, Attempt: req.Attempt, Err: err}
			}
			return PickerListMsg{Path: req.Path, Attempt: req.Attempt, Err: err}
		}
		if req.Verify {
			return PickerVerifyMsg{Path: req.Path, Attempt: req.Attempt}
		}
		dirs := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		return PickerListMsg{Path: req.Path, Attempt: req.Attempt, Dirs: dirs}
	}
}

// pickerRestore closes the picker and atomically restores the exact opener
// snapshot, recording the completed destination (empty on cancellation).
func (m Model) pickerRestore(completedPath string) Model {
	r := *m.pickerSuspended
	if m.pickerFlowKind == pickerFlowSave {
		r.saveCompletedPath = completedPath
	} else {
		r.exportCompletedPath = completedPath
	}
	return r
}

// pickerToggleFormat switches the export opener's closed save format between
// CSV and JSON; the query-save format is always SQL and cannot change.
func (m *Model) pickerToggleFormat() {
	if m.pickerFlowKind != pickerFlowExport {
		return
	}
	if m.exportFormat == filepicker.FormatCSV {
		m.exportFormat = filepicker.FormatJSON
	} else {
		m.exportFormat = filepicker.FormatCSV
	}
}

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

// drawPickerOverlay composites the destination-picker box over the composed
// shell inside the results region. Rendering is deterministic: the title with
// the current save format, the current directory, the `..`-first directory
// listing with its highlighted row and focused-pane marker, the filename
// buffer with its cursor, any inline error, and the fixed footer.
func (m Model) drawPickerOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	title := "Save query as (.sql)"
	if m.pickerFlowKind == pickerFlowExport {
		title = "Export result as (" + string(m.exportFormat) + ") — left/right format"
	}
	lines := []string{title, m.picker.CurrentDir() + "/"}
	for i, entry := range m.picker.Listing() {
		row := entry + "/"
		if i == m.picker.Highlight() {
			if m.picker.Focus() == filepicker.FocusDir {
				row = valueCursorStyle.Render(row)
			}
			row = "> " + row
		} else {
			row = "  " + row
		}
		lines = append(lines, truncateCell(row, maxWidth))
	}
	buf := m.picker.Filename()
	cursor := m.picker.Cursor()
	runes := []rune(buf)
	nameLine := "File: "
	var cur string
	if cursor < len(runes) {
		cur = valueCursorStyle.Render(string(runes[cursor]))
		nameLine += string(runes[:cursor]) + cur + string(runes[cursor+1:])
	} else {
		nameLine += buf + valueCursorStyle.Render(" ")
	}
	if m.picker.Focus() == filepicker.FocusFilename {
		nameLine = "> " + nameLine
	} else {
		nameLine = "  " + nameLine
	}
	lines = append(lines, truncateCell(nameLine, maxWidth))
	if err := m.picker.Error(); err != nil {
		lines = append(lines, truncateCell(err.Error(), maxWidth))
	}
	lines = append(lines, truncateCell("Enter select · Tab directory/file · Esc cancel", maxWidth))
	longest := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > longest {
			longest = w
		}
	}
	w := longest + 2
	if w < 4 {
		w = 4
	}
	if w > maxWidth {
		w = maxWidth
	}
	box := valuePromptStyle.Width(w).Height(len(lines)).Render(strings.Join(lines, "\n"))
	return composeOverlay(base, box, 1, 1)
}
