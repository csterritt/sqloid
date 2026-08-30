// Scripted Issue #53 save-flow tests (overwrite confirmation and atomic
// output) driving every step through Update exactly as the Bubble Tea
// runtime would: destination inspection through the injected boundary,
// exactly one non-stacking overwrite confirmation, immutable payload/
// selection capture against live-state mutation, cancel restoration, and
// the staged temp-file-plus-rename failures with preserved destinations,
// cleanup, inline retry, and no false success. No test depends on the real
// filesystem or any database.

package ui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/result"
)

// saveFlowFakeFS is the fake export.SaveFS boundary for the save-flow tests:
// every call is recorded, staged failures inject deterministically, and
// existing destinations keep byte-for-byte contents.
type saveFlowFakeFS struct {
	existing       map[string]bool
	contents       map[string][]byte
	calls          []string
	tempSeq        int
	failCreate     error
	failWriteAfter int // -1 never; n fails any write larger than n bytes
	failSync       bool
	failClose      bool
	failRename     error
}

func newSaveFlowFakeFS() *saveFlowFakeFS {
	return &saveFlowFakeFS{
		existing:       map[string]bool{},
		contents:       map[string][]byte{},
		failWriteAfter: -1,
	}
}

func (f *saveFlowFakeFS) Exists(path string) (bool, error) {
	f.calls = append(f.calls, "exists:"+path)
	return f.existing[path], nil
}

func (f *saveFlowFakeFS) TempFile(dir, pattern string) (export.SaveFile, error) {
	f.calls = append(f.calls, "create:"+dir)
	if f.failCreate != nil {
		return nil, f.failCreate
	}
	f.tempSeq++
	name := dir + "/" + pattern[:1] + fmt.Sprintf("temp-%d", f.tempSeq)
	f.calls = append(f.calls, "tempname:"+name)
	return &saveFlowFakeFile{fs: f, name: name}, nil
}

func (f *saveFlowFakeFS) Name(fl export.SaveFile) string {
	return fl.(*saveFlowFakeFile).name
}

func (f *saveFlowFakeFS) Rename(oldPath, newPath string) error {
	f.calls = append(f.calls, "rename:"+oldPath+"->"+newPath)
	if f.failRename != nil {
		return f.failRename
	}
	f.contents[newPath] = f.contents[oldPath]
	delete(f.contents, oldPath)
	return nil
}

func (f *saveFlowFakeFS) Remove(path string) error {
	f.calls = append(f.calls, "remove:"+path)
	delete(f.contents, path)
	return nil
}

type saveFlowFakeFile struct {
	fs   *saveFlowFakeFS
	name string
}

func (fl *saveFlowFakeFile) Write(p []byte) (int, error) {
	fl.fs.calls = append(fl.fs.calls, fmt.Sprintf("write:%s:%d", fl.name, len(p)))
	if fl.fs.failWriteAfter >= 0 && len(p) > fl.fs.failWriteAfter {
		fl.fs.contents[fl.name] = append([]byte(nil), p[:fl.fs.failWriteAfter]...)
		return fl.fs.failWriteAfter, errors.New("injected write failure")
	}
	fl.fs.contents[fl.name] = append([]byte(nil), p...)
	return len(p), nil
}

func (fl *saveFlowFakeFile) Sync() error {
	fl.fs.calls = append(fl.fs.calls, "sync:"+fl.name)
	if fl.fs.failSync {
		return errors.New("injected sync failure")
	}
	return nil
}

func (fl *saveFlowFakeFile) Close() error {
	fl.fs.calls = append(fl.fs.calls, "close:"+fl.name)
	if fl.fs.failClose {
		return errors.New("injected close failure")
	}
	return nil
}

// createdTempName returns the single temp artifact name recorded, or "".
func (f *saveFlowFakeFS) createdTempName() string {
	for _, c := range f.calls {
		if strings.HasPrefix(c, "tempname:") {
			return strings.TrimPrefix(c, "tempname:")
		}
	}
	return ""
}

// destructiveCalls lists every boundary call that would touch a destination.
func (f *saveFlowFakeFS) destructiveCalls() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "create:") || strings.HasPrefix(c, "write:") ||
			strings.HasPrefix(c, "rename:") || strings.HasPrefix(c, "remove:") ||
			strings.HasPrefix(c, "sync:") || strings.HasPrefix(c, "tempname:") {
			out = append(out, c)
		}
	}
	return out
}

// validSelectState is one serializable SELECT save target.
func validSelectState() qb.HistoryState {
	return qb.HistoryState{
		Command:    qb.CommandSelect,
		Table:      "items",
		TableSet:   true,
		Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}},
	}
}

// runSaveCmds runs a save-flow command chain (verify, inspection, write)
// through Update until no further command is issued, as the runtime would.
func runSaveCmds(t *testing.T, m Model, cmd tea.Cmd) (Model, []tea.Msg) {
	t.Helper()
	var msgs []tea.Msg
	for i := 0; cmd != nil && i < 8; i++ {
		msg := cmd()
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
		msgs = append(msgs, msg)
	}
	return m, msgs
}

// savePickerFlowModel builds a model with a serializable prepared save
// target, the picker settled at /work on the fake listing boundary, and the
// injected save boundary installed.
func savePickerFlowModel(t *testing.T, f *pickerFakeFS, s *saveFlowFakeFS) (Model, Model) {
	t.Helper()
	if f.fail == nil {
		f.fail = map[string]error{}
	}
	if f.dirs == nil {
		f.dirs = map[string][]pickerFakeEntry{}
	}
	m := New()
	m.Width, m.Height = 80, 24
	m.PickerFS = f
	m.PickerStart = "/work"
	m.SaveFS = s
	m.savePrepared = &export.SQLSaveTarget{State: validSelectState(), Source: export.SaveFromRunnableBuilder}
	pm := m
	cmd := pm.openPicker(pickerFlowSave, filepicker.FormatSQL)
	settled, _ := runList(t, pm, cmd)
	return m, settled
}

// submitDestination types name into the focused filename pane, submits, and
// runs the full verify/inspection chain, stopping at an open overwrite
// confirmation when the destination already exists.
func submitDestination(t *testing.T, m Model, name string) Model {
	t.Helper()
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
	m, _ = pressKey(m, runeKey(name))
	m, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	final, _ := runSaveCmds(t, m, cmd)
	return final
}

// confirmSave answers the open overwrite confirmation with Enter and runs
// the resulting write chain.
func confirmSave(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	final, _ := runSaveCmds(t, next, cmd)
	return final
}

func TestExistingDestinationOpensExactlyOneConfirmationWithoutReplacement(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	s := newSaveFlowFakeFS()
	s.existing["/work/report.sql"] = true
	s.contents["/work/report.sql"] = []byte("SELECT 1;")
	_, p := savePickerFlowModel(t, f, s)

	p = submitDestination(t, p, "report")
	if !p.overwriteOpen {
		t.Fatal("existing destination did not open the overwrite confirmation")
	}
	if view := p.View(); strings.Count(view, "Overwrite existing file?") != 1 {
		t.Fatalf("confirmation not shown exactly once: %q", view)
	}
	if !p.pickerOpen {
		t.Fatal("picker below the confirmation was discarded")
	}
	// Existing-file detection performed no destructive filesystem call and
	// left the destination untouched.
	if got := s.destructiveCalls(); len(got) != 0 {
		t.Fatalf("detection made destructive calls: %q", got)
	}
	if got := s.contents["/work/report.sql"]; string(got) != "SELECT 1;" {
		t.Fatalf("destination bytes changed before confirmation: %q", got)
	}
}

func TestConfirmationCapturesImmutablePayloadAndSelection(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	s.existing["/work/new.sql"] = true
	_, p := savePickerFlowModel(t, f, s)

	// Reach the confirmation, then mutate live builder and prepared-target
	// state behind it.
	p = submitDestination(t, p, "new")
	if !p.overwriteOpen {
		t.Fatal("confirmation not open")
	}
	captured := *p.saveCapture
	p.savePrepared.State.Table = "mutated"
	p.savePrepared.Source = 99

	// Confirm: the captured destination, format, payload, and selection are
	// authoritative; nothing re-resolves or re-serializes mutable state.
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	final, _ := runSaveCmds(t, p, cmd)
	if got := final.saveCompletedPath; got != captured.path {
		t.Fatalf("completed path = %q, want the captured %q", got, captured.path)
	}
	if final.saveCapture != nil && final.saveCapture != &captured {
		t.Fatal("confirmation re-resolved the capture")
	}
	if string(captured.payload) != "SELECT * FROM \"items\";" {
		t.Fatalf("captured payload mutated: %q", captured.payload)
	}
	if captured.selection != "runnable-builder" {
		t.Fatalf("captured selection = %q", captured.selection)
	}
	if got := s.contents[captured.path]; !bytes.Equal(got, captured.payload) {
		t.Fatalf("written bytes = %q, want the immutable captured copy %q", got, captured.payload)
	}
}

func TestOverwriteCancelRestoresIntactPickerAndCapturedCopy(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	s.existing["/work/r.sql"] = true
	_, p := savePickerFlowModel(t, f, s)
	p = submitDestination(t, p, "r")
	if !p.overwriteOpen {
		t.Fatal("confirmation not open")
	}

	// Unrelated keys are consumed with no effect and no stacking.
	p, _ = pressKey(p, runeKey("q?x"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if !p.overwriteOpen || s.destructiveCalls() != nil {
		t.Fatal("unrelated keys leaked or the confirmation stacked")
	}

	// Esc cancels only the overwrite question: the picker returns intact
	// with its filename, directory, format, warnings, and captured copy.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyEscape})
	if p.overwriteOpen {
		t.Fatal("Esc did not close the confirmation")
	}
	if !p.pickerOpen {
		t.Fatal("Esc discarded the intact picker")
	}
	if got := p.picker.Filename(); got != "r" {
		t.Fatalf("picker filename = %q, want r", got)
	}
	if got := p.picker.CurrentDir(); got != "/work" {
		t.Fatalf("picker directory = %q", got)
	}
	if got := p.picker.Format(); got != filepicker.FormatSQL {
		t.Fatalf("picker format = %q", got)
	}
	if p.saveCapture == nil || p.saveCapture.path != "/work/r.sql" {
		t.Fatal("Esc discarded the captured immutable copy")
	}
	if len(s.destructiveCalls()) != 0 {
		t.Fatalf("cancel made destructive calls: %q", s.destructiveCalls())
	}

	// n cancels identically from a fresh confirmation reached through the
	// retained filename and focus.
	p2, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p2, _ = runSaveCmds(t, p2, cmd)
	if !p2.overwriteOpen {
		t.Fatal("re-submission did not reopen the confirmation")
	}
	p2, _ = pressKey(p2, runeKey("n"))
	if p2.overwriteOpen || !p2.pickerOpen || p2.saveCapture == nil {
		t.Fatal("n did not restore the intact picker with the captured copy")
	}
}

func TestExportConfirmationCapturesWarningsAndPayload(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
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
	s.existing["/work/out.csv"] = true
	opened, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	opened, _ = runList(t, opened, cmd)
	p := submitDestination(t, opened, "out")
	if !p.overwriteOpen {
		t.Fatal("export confirmation not open")
	}
	if len(p.saveCapture.warnings) != 1 || p.saveCapture.warnings[0] != "Result is complete" {
		t.Fatalf("captured warnings = %q", p.saveCapture.warnings)
	}
	if p.saveCapture.selection != "result capture" || p.saveCapture.format != filepicker.FormatCSV {
		t.Fatalf("captured selection/format = %q/%q", p.saveCapture.selection, p.saveCapture.format)
	}
	if string(p.saveCapture.payload) != "id\r\n3\r\n" {
		t.Fatalf("captured CSV payload = %q", p.saveCapture.payload)
	}
	// Confirming proceeds with the captured payload exactly.
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	_, _ = runSaveCmds(t, p, cmd)
	if got := s.contents["/work/out.csv"]; string(got) != "id\r\n3\r\n" {
		t.Fatalf("written CSV = %q", got)
	}
}

func TestSaveAtomicSuccessNewAndExistingDestinations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
		before   string
	}{
		{"new-destination", false, ""},
		{"replacement", true, "SELECT 1;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := pickerNewFakeFS()
			f.dirs["/work"] = []pickerFakeEntry{}
			s := newSaveFlowFakeFS()
			if tc.existing {
				s.existing["/work/r.sql"] = true
				s.contents["/work/r.sql"] = []byte(tc.before)
			}
			_, p := savePickerFlowModel(t, f, s)
			baseline := openerFingerprint(p)
			p = submitDestination(t, p, "r")
			var final Model
			if tc.existing {
				final = confirmSave(t, p)
			} else {
				final = p
			}
			want := "SELECT * FROM \"items\";"
			if tc.existing && string(s.contents["/work/r.sql"]) != want {
				t.Fatalf("replacement bytes = %q, want %q", s.contents["/work/r.sql"], want)
			}
			if !tc.existing && string(s.contents["/work/r.sql"]) != want {
				t.Fatalf("new-destination bytes = %q, want %q", s.contents["/work/r.sql"], want)
			}
			if name := s.createdTempName(); name != "" {
				if _, ok := s.contents[name]; ok {
					t.Fatalf("temporary artifact %q leaked", name)
				}
			}
			if final.pickerOpen {
				t.Fatal("picker left open after one completion transition")
			}
			if final.saveCompletedPath != "/work/r.sql" {
				t.Fatalf("saveCompletedPath = %q", final.saveCompletedPath)
			}
			if fp := openerFingerprint(final); fp != baseline {
				t.Errorf("completion opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
			}
		})
	}
}

func TestSaveStageFailuresPreserveCleanupAndRetryInline(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*saveFlowFakeFS)
	}{
		{"temp-create", func(s *saveFlowFakeFS) { s.failCreate = errors.New("no dir") }},
		{"partial-write", func(s *saveFlowFakeFS) { s.failWriteAfter = 3 }},
		{"full-write", func(s *saveFlowFakeFS) { s.failWriteAfter = 0 }},
		{"sync", func(s *saveFlowFakeFS) { s.failSync = true }},
		{"close", func(s *saveFlowFakeFS) { s.failClose = true }},
		{"rename", func(s *saveFlowFakeFS) { s.failRename = errors.New("busy") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := pickerNewFakeFS()
			f.dirs["/work"] = []pickerFakeEntry{}
			s := newSaveFlowFakeFS()
			s.existing["/work/r.sql"] = true
			s.contents["/work/r.sql"] = []byte("SELECT 1;")
			tc.setup(s)
			_, p := savePickerFlowModel(t, f, s)
			baseline := openerFingerprint(p)
			reached := submitDestination(t, p, "r")
			failed := confirmSave(t, reached)
			if failed.overwriteOpen {
				t.Fatal("failure left the confirmation open")
			}
			if failed.saveFailure == "" {
				t.Fatal("failure not shown inline")
			}
			if view := failed.View(); !strings.Contains(view, "Save failed") {
				t.Fatalf("view lacks the inline failure: %q", view)
			}
			// Byte-for-byte preservation of the existing destination, no
			// success claim, and no leaked temporary artifact.
			if got := s.contents["/work/r.sql"]; string(got) != "SELECT 1;" {
				t.Fatalf("existing destination changed: %q", got)
			}
			if failed.saveCompletedPath != "" {
				t.Fatal("failure claimed a completed destination")
			}
			if name := s.createdTempName(); name != "" {
				if _, ok := s.contents[name]; ok {
					t.Fatalf("temporary artifact %q not cleaned up", name)
				}
			}
			// Retry reuses the same captured copy: the boundary receives the
			// identical bytes again.
			s.failCreate, s.failWriteAfter, s.failSync, s.failClose, s.failRename = nil, -1, false, false, nil
			retried, cmd := pressKey(failed, tea.KeyMsg{Type: tea.KeyEnter})
			final, _ := runSaveCmds(t, retried, cmd)
			if got := s.contents["/work/r.sql"]; string(got) != "SELECT * FROM \"items\";" {
				t.Fatalf("retry bytes = %q, want the same captured copy written", got)
			}
			if final.saveCompletedPath != "/work/r.sql" {
				t.Fatal("successful retry did not complete")
			}
			if fp := openerFingerprint(final); fp != baseline {
				t.Errorf("retry completion fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
			}
		})
	}
}

func TestSaveFailureCancelRestoresOpenerExactly(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	s.failRename = errors.New("busy")
	s.existing["/work/r.sql"] = true
	opener, p := savePickerFlowModel(t, f, s)
	baseline := openerFingerprint(opener)
	reached := submitDestination(t, p, "r")
	failed := confirmSave(t, reached)
	if failed.saveFailure == "" {
		t.Fatal("failure not inline")
	}
	cancelled, _ := pressKey(failed, tea.KeyMsg{Type: tea.KeyEscape})
	if cancelled.pickerOpen || cancelled.saveFailure != "" {
		t.Fatal("cancel left save-flow state behind")
	}
	if cancelled.saveCompletedPath != "" {
		t.Fatal("cancel recorded a completed path")
	}
	if fp := openerFingerprint(cancelled); fp != baseline {
		t.Errorf("cancel opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
	}
}

func TestStaleSaveCompletionsAndDuplicateConfirmsAreInert(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	_, p := savePickerFlowModel(t, f, s)
	p = submitDestination(t, p, "r")
	if p.pickerOpen || p.saveCompletedPath != "/work/r.sql" {
		t.Fatal("setup did not complete the save")
	}
	// A duplicate completion for an already-finished attempt is inert.
	stale, _ := p.Update(SaveCompletedMsg{Attempt: p.saveAttempt})
	if sm := stale.(Model); sm.saveCompletedPath != "/work/r.sql" || sm.pickerOpen {
		t.Fatal("duplicate completion mutated restored state")
	}
	// A stale inspection response carrying an old attempt identity is inert.
	stale2, _ := p.Update(SaveInspectMsg{Path: "/work/r.sql", Attempt: 1, Status: export.DestinationExisting})
	if sm := stale2.(Model); sm.overwriteOpen || sm.pickerOpen || sm.saveFailure != "" {
		t.Fatal("stale inspection response mutated settled state")
	}
	// An unrelated key after settlement leaks nowhere.
	final, _ := pressKey(stale2.(Model), runeKey("x"))
	if final.overwriteOpen || final.saveFailure != "" || final.pickerOpen {
		t.Fatal("post-settlement key leaked into save-flow state")
	}
}
