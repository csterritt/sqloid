// Ctrl+S/Ctrl+X destination confirmation and atomic write stage (Issue #53),
// per the Atomic saves, File picker, and Global Key Precedence decisions in
// Notes/PRD-sqloid.md. Path resolution (Issue #52) freezes one immutable
// save-flow capture: the resolved destination, output format, complete
// serialized payload (already-captured bytes from the Issue #48 SQL target
// or the Issue #49 export capture), the source selection's provenance, and
// the pre-destination warnings — all taken before any destination
// inspection. A new path advances that captured capture straight to the
// write stage; an existing path opens exactly one non-stacking overwrite
// confirmation that consumes every key until Enter/y (confirm) or Esc/n
// (cancel back to the intact picker). The write stage is Issue #53's
// destination-local temporary-file-plus-rename boundary, issued as a tea.Cmd
// outside Update; completion and failure arrive as typed messages guarded by
// the save-flow attempt identity so duplicate confirms and stale responses
// are inert. No branch after capture ever consults the live builder, the
// active result, or the current history selection: confirm, retry, and the
// write command all use the captured copy.

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
)

// saveCapture is the frozen Issue #53 save-flow identity: one resolved
// destination and format plus the complete immutable serialized payload,
// source-selection provenance, and warnings. Nothing inside it is ever
// re-resolved or re-serialized from mutable state.
type saveCapture struct {
	flow      pickerFlow
	path      string
	format    filepicker.Format
	payload   []byte
	selection string
	warnings  []string
}

// SaveInspectMsg answers one issued destination-inspection request. The
// Status classifies the resolved destination through the export boundary;
// Attempt guards stale responses against the capture identity.
type SaveInspectMsg struct {
	Path    string
	Attempt uint64
	Status  export.DestinationStatus
	Err     error
}

// SaveCompletedMsg reports one settled atomic write for its attempt.
type SaveCompletedMsg struct {
	Attempt uint64
}

// SaveFailedMsg reports one failed atomic write stage for its attempt.
type SaveFailedMsg struct {
	Attempt uint64
	Err     error
}

// saveFS resolves the injected save boundary; nil means the real filesystem.
func (m Model) saveFS() export.SaveFS {
	if m.SaveFS == nil {
		return export.OSSaveFS{}
	}
	return m.SaveFS
}

// captureSave freezes the complete immutable save-flow payload for the
// resolved destination. The SQL save flow serializes the Issue #48 target's
// immutable complete query state once; the export flow serializes the Issue
// #49 immutable capture's payload through the opener's closed format. Any
// serialization failure is a typed inline-retry error: nothing is written.
func (m Model) captureSave(path string) (saveCapture, error) {
	if m.pickerFlowKind == pickerFlowSave {
		if m.savePrepared == nil {
			return saveCapture{}, export.ErrNoRunnableQuery
		}
		stmt, err := export.SerializeSQLQuery(m.savePrepared.State)
		if err != nil {
			return saveCapture{}, err
		}
		return saveCapture{
			flow:      pickerFlowSave,
			path:      path,
			format:    filepicker.FormatSQL,
			payload:   []byte(stmt),
			selection: m.savePrepared.Source.String(),
		}, nil
	}
	if m.exportPrepared == nil {
		return saveCapture{}, export.ErrNoTabularData
	}
	var payload []byte
	if m.exportFormat == filepicker.FormatJSON {
		payload = export.JSON(m.exportPrepared.Payload)
	} else {
		payload = export.CSV(m.exportPrepared.Payload)
	}
	warnings := append([]string(nil), m.exportWarnings...)
	return saveCapture{
		flow:      pickerFlowExport,
		path:      path,
		format:    m.exportFormat,
		payload:   payload,
		selection: "result capture",
		warnings:  warnings,
	}, nil
}

// beginSaveFlow starts the Issue #53 save flow for the freshly resolved
// destination: it freezes the immutable capture (serialization included),
// then inspects the destination through the boundary as a command — never a
// destructive call. Exactly one capture identity is minted here; stale
// inspection responses are inert against it.
func (m Model) beginSaveFlow(path string) (tea.Model, tea.Cmd) {
	m.overwriteOpen = false
	m.saveRunning = false
	m.saveFailure = ""
	capture, err := m.captureSave(path)
	if err != nil {
		m.saveCapture = nil
		m.saveFailure = err.Error()
		m.saveFailurePath = path
		return m, nil
	}
	m.saveCapture = &capture
	m.saveFailurePath = ""
	m.saveAttempt++
	attempt := m.saveAttempt
	fs := m.saveFS()
	return m, func() tea.Msg {
		status, err := export.InspectDestination(fs, path)
		return SaveInspectMsg{Path: path, Attempt: attempt, Status: status, Err: err}
	}
}

// applySaveInspect consumes one inspection response. A stale attempt or an
// identity whose capture has since been replaced is inert. An inspection
// error stays inline with the same retry/cancel path as a write failure; a
// new destination advances the captured capture straight to the write
// stage, and an existing destination opens exactly one overwrite
// confirmation — the destination is never opened, truncated, or replaced
// here.
func (m Model) applySaveInspect(msg SaveInspectMsg) (tea.Model, tea.Cmd) {
	if m.saveCapture == nil || msg.Attempt != m.saveAttempt || msg.Path != m.saveCapture.path {
		return m, nil
	}
	if msg.Err != nil {
		m.saveFailure = msg.Err.Error()
		m.saveFailurePath = msg.Path
		return m, nil
	}
	if msg.Status == export.DestinationExisting {
		m.overwriteOpen = true
		return m, nil
	}
	return m.startSaveWrite()
}

// startSaveWrite advances the current captured capture to the write stage:
// the destination-local temp-file-plus-rename boundary runs outside Update
// and reports through typed messages guarded by this attempt identity.
func (m Model) startSaveWrite() (tea.Model, tea.Cmd) {
	if m.saveCapture == nil {
		return m, nil
	}
	m.overwriteOpen = false
	m.saveFailure = ""
	m.saveFailurePath = ""
	m.saveRunning = true
	m.saveAttempt++
	attempt := m.saveAttempt
	capture := *m.saveCapture
	fs := m.saveFS()
	cmd := func() tea.Msg {
		if err := export.WriteAtomic(fs, capture.path, capture.payload); err != nil {
			return SaveFailedMsg{Attempt: attempt, Err: err}
		}
		return SaveCompletedMsg{Attempt: attempt}
	}
	return m, cmd
}

// handleOverwriteConfirmKey consumes every key while the overwrite
// confirmation is open above the intact picker: Enter/y confirms exactly
// once (duplicates and other keys are inert), Esc/n cancels only the
// overwrite question and returns to the intact picker with the filename,
// directory, format, and captured copy preserved, and Ctrl+C still opens the
// shared quit confirmation. Nothing leaks into the picker below.
func (m Model) handleOverwriteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.openQuitConfirmation(), nil
	case "enter", "y":
		return m.startSaveWrite()
	case "esc", "n":
		// Cancel only the confirmation: the picker below is untouched, the
		// captured copy is retained, and no replacement ever started.
		m.overwriteOpen = false
		return m, nil
	}
	return m, nil
}

// handleSaveFailureKey consumes keys while an inline save failure is
// showing: Enter/y retries with the same captured destination, format,
// payload, and selection (re-running capture only when serialization itself
// failed), Esc/n cancels with exact opener restoration and no completed
// path, and every other key is consumed without leakage.
func (m Model) handleSaveFailureKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.openQuitConfirmation(), nil
	case "enter", "y":
		if m.saveCapture == nil {
			// A serialization-stage failure: re-run the capture from the
			// still-immutable prepared state against the resolved path.
			return m.beginSaveFlow(m.saveFailurePath)
		}
		return m.startSaveWrite()
	case "esc", "n":
		return m.pickerRestore(""), nil
	}
	return m, nil
}

// drawSaveFlowOverlay composites the Issue #53 save-flow overlays above the
// picker: the single overwrite confirmation, the in-flight save status, or
// the inline failure with its retry/cancel path. Rendering is deterministic
// and never reflows any region.
func (m Model) drawSaveFlowOverlay(base string) string {
	switch {
	case m.overwriteOpen:
		return m.drawOverwriteConfirmOverlay(base)
	case m.saveFailure != "":
		return m.drawSaveFailureOverlay(base)
	case m.saveRunning:
		return m.drawSaveStatusOverlay(base)
	default:
		return base
	}
}

// drawOverwriteConfirmOverlay renders exactly one non-stacking overwrite
// confirmation naming the resolved destination.
func (m Model) drawOverwriteConfirmOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	lines := []string{
		"Overwrite existing file?",
		truncateCell(m.saveCapture.path, maxWidth),
		"Enter/y overwrite · Esc/n cancel",
	}
	return composeSaveBox(base, lines, maxWidth)
}

// drawSaveStatusOverlay renders the in-flight write stage line.
func (m Model) drawSaveStatusOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	lines := []string{"Saving " + string(m.saveCapture.format) + "…"}
	return composeSaveBox(base, lines, maxWidth)
}

// drawSaveFailureOverlay renders one inline failure with the captured path
// and the retry/cancel path; success is never claimed.
func (m Model) drawSaveFailureOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	lines := []string{"Save failed", truncateCell(m.saveFailure, maxWidth)}
	if m.saveFailurePath != "" {
		lines = append(lines, truncateCell(m.saveFailurePath, maxWidth))
	}
	lines = append(lines, "Enter/y retry · Esc/n cancel")
	return composeSaveBox(base, lines, maxWidth)
}

// composeSaveBox frames the overlay lines and composites them like every
// other results-region overlay.
func composeSaveBox(base string, lines []string, maxWidth int) string {
	for i, l := range lines {
		lines[i] = truncateCell(l, maxWidth)
	}
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
