// Scripted destination-picker flow tests (Issue #52), driving the picker
// composition through Update exactly as the Context/Action Matrix requires:
// working-directory start through the fake boundary, `..`-first bytewise
// directory navigation, separate literal filename input (including `?` and
// `q`), inline validation and path/permission errors with retained state,
// exact opener restoration on Esc and completion, and stale-response
// inertness. No test here depends on the real filesystem or any database.

package ui

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
)

// pickerFakeEntry is one fake-filesystem entry for the picker tests.
type pickerFakeEntry struct {
	name  string
	isDir bool
}

func (e pickerFakeEntry) Name() string { return e.name }
func (e pickerFakeEntry) IsDir() bool  { return e.isDir }

// pickerFakeFS is the fake filesystem boundary for the picker tests. Every
// read is recorded; failing paths return their recorded error.
type pickerFakeFS struct {
	dirs  map[string][]pickerFakeEntry
	fail  map[string]error
	reads []string
}

func (f *pickerFakeFS) ReadDir(path string) ([]filepicker.DirEntry, error) {
	f.reads = append(f.reads, path)
	if err, ok := f.fail[path]; ok {
		return nil, err
	}
	entries, ok := f.dirs[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	out := make([]filepicker.DirEntry, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}

// pickerNewFakeFS returns an empty fake filesystem with initialized maps.
func pickerNewFakeFS() *pickerFakeFS {
	return &pickerFakeFS{dirs: map[string][]pickerFakeEntry{}, fail: map[string]error{}}
}

// runList executes the returned listing/verify command and feeds its
// response back through Update, as the Bubble Tea runtime would.
func runList(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Msg) {
	t.Helper()
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	next, _ := m.Update(msg)
	return next.(Model), msg
}

// savePickerModel builds a model with a serializable prepared save target,
// a picker seeded against the fake boundary at /work, the start listing
// settled, and an empty injected save boundary.
func savePickerModel(t *testing.T, f *pickerFakeFS) (Model, Model) {
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
	m.savePrepared = &export.SQLSaveTarget{State: validSelectState()}
	// Open directly through the seam: the Ctrl+S entry path (resolution,
	// then picker open) is covered by the save-targeting tests.
	pm := m
	cmd := pm.openPicker(pickerFlowSave, filepicker.FormatSQL)
	opened := pm
	if !opened.pickerOpen || cmd == nil {
		t.Fatal("picker did not open with a listing command")
	}
	if opened.picker.StartDir() != "/work" {
		t.Fatalf("picker start = %q, want the injected working directory /work", opened.picker.StartDir())
	}
	settled, _ := runList(t, opened, cmd)
	return m, settled
}

func TestPickerOpensAtWorkingDirectoryAcrossOpeners(t *testing.T) {
	// Query save opener: bytewise, `..`-first, file-free listing.
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{
		{"out", true}, {".hidden", true}, {"B", true}, {"a", true},
		{"d10", true}, {"d2", true}, {".conf", true}, {"é中", true}, {"f.txt", false},
	}
	_, p2 := savePickerModel(t, f)
	if p2.picker.CurrentDir() != "/work" || p2.picker.Format() != filepicker.FormatSQL {
		t.Fatalf("save picker dir %q format %q", p2.picker.CurrentDir(), p2.picker.Format())
	}
	got := p2.picker.Listing()
	// Go string order: "B" < "a", "d10" < "d2" (non-natural), ".conf" <
	// ".hidden" (byte order); "f.txt" is a file and never appears.
	wantList := []string{"..", ".conf", ".hidden", "B", "a", "d10", "d2", "out", "é中"}
	if !reflect.DeepEqual(got, wantList) {
		t.Fatalf("listing = %q\nwant           %q (.. first, bytewise, non-natural, no files)", got, wantList)
	}
	if p2.picker.Highlight() != 0 {
		t.Fatalf("initial highlight = %d, want the `..` row", p2.picker.Highlight())
	}

	// CSV and JSON export openers begin the same way through the warning
	// flow's Enter, carrying the immutable capture into the picker.
	for _, format := range []filepicker.Format{filepicker.FormatCSV, filepicker.FormatJSON} {
		f := pickerNewFakeFS()
		f.dirs["/work"] = []pickerFakeEntry{{"sub", true}}
		m := New()
		m.PickerFS = f
		m.PickerStart = "/work"
		m.exportPrepared = &export.Capture{Payload: export.Payload{Names: []string{"c"}}}
		m.exportFormat = format
		m.exportWarnings = []string{"Result is complete"}
		m.exportWarningsOpen = true
		opened, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
		if !opened.pickerOpen || opened.picker.Format() != format {
			t.Fatalf("%s: picker not opened with format %q", format, opened.picker.Format())
		}
		if opened.exportPrepared == nil {
			t.Fatalf("%s: immutable capture lost", format)
		}
		if opened.exportWarningsOpen {
			t.Fatalf("%s: warning flow left open under the picker", format)
		}
		settled, _ := runList(t, opened, cmd)
		if got := settled.picker.Listing(); !reflect.DeepEqual(got, []string{"..", "sub"}) {
			t.Fatalf("%s: listing = %q", format, got)
		}
	}
}

func TestPickerNavigationKeysAndSeparateFilename(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{
		{"out", true}, {".conf", true}, {"sub", true}, {"f.txt", false},
	}
	f.dirs["/work/out"] = []pickerFakeEntry{{"deep", true}}
	f.dirs["/work/out/deep"] = []pickerFakeEntry{}
	f.dirs["/work/.conf"] = []pickerFakeEntry{}
	_, p := savePickerModel(t, f)

	// Type a filename first: navigating directories must not touch it.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus
	p, _ = pressKey(p, runeKey("my file"))
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // directory focus
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if got := p.picker.Highlighted(); got != "out" {
		t.Fatalf("highlighted = %q, want out (bytewise order)", got)
	}
	if p.picker.Filename() != "my file" || p.picker.Cursor() != 7 {
		t.Fatalf("filename drifted during navigation: %q cursor %d", p.picker.Filename(), p.picker.Cursor())
	}

	// Enter the visible child, then its nested child, then `..` twice.
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ = runList(t, p, cmd)
	if p.picker.CurrentDir() != "/work/out" {
		t.Fatalf("child navigation landed at %q", p.picker.CurrentDir())
	}
	if got := p.picker.Listing(); !reflect.DeepEqual(got, []string{"..", "deep"}) {
		t.Fatalf("out listing = %q, want [.. deep]", got)
	}
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown}) // highlight deep
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ = runList(t, p, cmd)
	if p.picker.CurrentDir() != "/work/out/deep" {
		t.Fatalf("nested navigation landed at %q", p.picker.CurrentDir())
	}
	if got := p.picker.Listing(); !reflect.DeepEqual(got, []string{".."}) {
		t.Fatalf("deep listing = %q, want only [..]", got)
	}
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter}) // `..` highlighted
	p, _ = runList(t, p, cmd)
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ = runList(t, p, cmd)
	if p.picker.CurrentDir() != "/work" {
		t.Fatalf("repeated parent navigation landed at %q", p.picker.CurrentDir())
	}
	if p.picker.Filename() != "my file" {
		t.Fatalf("filename lost across navigation: %q", p.picker.Filename())
	}

	// Hidden child navigation: highlight walks down to the last directory.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyDown})
	if got := p.picker.Highlighted(); got != "sub" {
		t.Fatalf("highlighted = %q, want sub (last directory; file never listed)", got)
	}
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	if got := p.picker.Highlighted(); got != ".conf" {
		t.Fatalf("highlighted = %q, want .conf", got)
	}
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ = runList(t, p, cmd)
	if p.picker.CurrentDir() != "/work/.conf" {
		t.Fatalf("hidden child navigation landed at %q", p.picker.CurrentDir())
	}

	// Boundary presses are no-ops and issue no requests; `..` still works.
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ = runList(t, p, cmd)
	if p.picker.CurrentDir() != "/work" {
		t.Fatalf("parent from hidden child landed at %q", p.picker.CurrentDir())
	}
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		t.Fatal("up at top issued a request")
	}
	if p.picker.Highlight() != 0 {
		t.Fatalf("up at top moved highlight to %d", p.picker.Highlight())
	}
}

func TestPickerFilenameLiteralKeysAndValidation(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	_, p := savePickerModel(t, f)
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab}) // filename focus

	// Printable keys — including `?` and `q` — insert literally: no help,
	// no quit confirmation, no leakage.
	p, _ = pressKey(p, runeKey("q"))
	p, _ = pressKey(p, runeKey("?"))
	p, _ = pressKey(p, runeKey("q"))
	if p.quitConfirm {
		t.Fatal("q opened the quit confirmation while filename input is focused")
	}
	if p.terminalHelpOpen {
		t.Fatal("? opened help while filename input is focused")
	}
	if got := p.picker.Filename(); got != "q?q" {
		t.Fatalf("filename = %q, want q?q", got)
	}

	// Empty submit: inline validation error, picker stays, no verify read.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlU})
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("empty basename issued a verify request")
	}
	if p.picker.Error() == nil || !p.pickerOpen {
		t.Fatalf("empty submit: err %v open %v", p.picker.Error(), p.pickerOpen)
	}
	if got := p.picker.Error().Error(); got != "filename is empty" {
		t.Fatalf("error text = %q", got)
	}

	// Slash-containing basename: same inline rejection.
	p, _ = pressKey(p, runeKey("a/b"))
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("slash basename issued a verify request")
	}
	if got := p.picker.Error(); got == nil || !strings.Contains(got.Error(), "'/'") {
		t.Fatalf("slash error = %v", p.picker.Error())
	}
	// The listing is untouched by editing.
	if got := p.picker.Listing(); !reflect.DeepEqual(got, []string{"..", "out"}) {
		t.Fatalf("listing changed by editing: %q", got)
	}

	// A valid name appends .sql exactly once via the verify request path.
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyCtrlU})
	p, _ = pressKey(p, runeKey("report"))
	p, cmd = pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	p, msg := runList(t, p, cmd)
	if v, ok := msg.(PickerVerifyMsg); !ok || v.Path != "/work/report.sql" {
		t.Fatalf("verify = %+v ok=%v, want /work/report.sql", msg, ok)
	}
	// Issue #53: the verify response also minted the save-flow capture and
	// issued the destination inspection; settle it before asserting.
	p, _ = runSaveCmds(t, p, nil)
	if _, done := p.picker.Completed(); !done {
		t.Fatal("verification did not complete the picker")
	}
}

func TestPickerCompletionRestoresOpenerExactly(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	opener, p := savePickerModel(t, f)
	baseline := openerFingerprint(opener)

	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab})
	p, _ = pressKey(p, runeKey("report"))
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	// The verify response is the completion boundary: Update mints the
	// Issue #53 capture, inspects the destination, writes, and restores the
	// exact opener atomically with the completed destination recorded.
	settled, msgs := runSaveCmds(t, p, cmd)
	if len(msgs) == 0 {
		t.Fatal("completion ran no save-flow messages")
	}
	if _, ok := msgs[0].(PickerVerifyMsg); !ok {
		t.Fatalf("completion message = %T, want verify", msgs[0])
	}
	if settled.pickerOpen {
		t.Fatal("picker still open after completion")
	}
	if got := settled.saveCompletedPath; got != "/work/report.sql" {
		t.Fatalf("saveCompletedPath = %q, want /work/report.sql", got)
	}
	if fp := openerFingerprint(settled); fp != baseline {
		t.Errorf("completion opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
	}
}

func TestPickerEscRestoresOpenerFromEveryFocusAndErrorState(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	opener, p := savePickerModel(t, f)
	baseline := openerFingerprint(opener)

	// Esc from directory focus.
	escDir, _ := pressKey(p, tea.KeyMsg{Type: tea.KeyEscape})
	if escDir.pickerOpen || escDir.pickerSuspended != nil {
		t.Fatal("directory-focus Esc left picker state behind")
	}
	if fp := openerFingerprint(escDir); fp != baseline {
		t.Errorf("directory-focus Esc drifted:\n%s\nvs\n%s", fp, baseline)
	}

	// Esc from filename focus with a half-typed name: the opener is still
	// restored exactly and the draft is discarded with it.
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

	// Esc from an inline-error state: a permission-denied navigation keeps
	// its typed error until Esc cancels with exact restoration, retaining
	// the opener's captured save target.
	f.fail["/work/out"] = fs.ErrPermission
	_, p3 := savePickerModel(t, f)
	p3, _ = pressKey(p3, tea.KeyMsg{Type: tea.KeyDown}) // highlight out
	p3, cmd := pressKey(p3, tea.KeyMsg{Type: tea.KeyEnter})
	p3, _ = runList(t, p3, cmd)
	if p3.picker.Error() == nil || !filepicker.IsPermission(p3.picker.Error()) {
		t.Fatalf("permission failure not inline: %v", p3.picker.Error())
	}
	if p3.picker.Filename() != "" || p3.picker.CurrentDir() != "/work" {
		t.Fatalf("failure state not retained: dir %q name %q", p3.picker.CurrentDir(), p3.picker.Filename())
	}
	escErr, _ := pressKey(p3, tea.KeyMsg{Type: tea.KeyEscape})
	if escErr.pickerOpen {
		t.Fatal("error-state Esc left the picker open")
	}
	if fp := openerFingerprint(escErr); fp != baseline {
		t.Errorf("error-state Esc drifted:\n%s\nvs\n%s", fp, baseline)
	}
	if escErr.savePrepared == nil {
		t.Fatal("Esc lost the opener's captured save target")
	}
}

func TestPickerRetryRetainsStateAndRejectsStaleResponses(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"locked", true}, {"sub", true}}
	f.dirs["/work/locked"] = []pickerFakeEntry{}
	f.dirs["/work/sub"] = []pickerFakeEntry{}
	_, settled := savePickerModel(t, f)

	// Fail the navigation twice, then succeed on the corrected retry with
	// highlight, filename, and format retained throughout.
	f.fail["/work/locked"] = fs.ErrPermission
	settled, _ = pressKey(settled, tea.KeyMsg{Type: tea.KeyDown}) // locked
	var cmd tea.Cmd
	settled, cmd = pressKey(settled, tea.KeyMsg{Type: tea.KeyEnter})
	failed, _ := runList(t, settled, cmd)
	if failed.picker.Error() == nil || failed.picker.CurrentDir() != "/work" {
		t.Fatalf("failure not inline: %v dir %q", failed.picker.Error(), failed.picker.CurrentDir())
	}
	if failed.picker.Format() != filepicker.FormatSQL {
		t.Fatalf("format not retained: %q", failed.picker.Format())
	}
	// A late listing carrying the already-settled attempt identity is inert.
	failed, msg := runList(t, failed, func() tea.Msg {
		return PickerListMsg{Path: "/work/locked", Attempt: 2, Dirs: []string{"ghost"}}
	})
	if got := failed.picker.Listing(); reflect.DeepEqual(got, []string{"..", "ghost"}) {
		t.Fatalf("stale response applied to the picker: listing = %q", got)
	}
	_ = msg
	// Repeated failure, then corrected retry.
	failed, cmd = pressKey(failed, tea.KeyMsg{Type: tea.KeyEnter})
	failed2, _ := runList(t, failed, cmd)
	if failed2.picker.Error() == nil {
		t.Fatal("repeated failure left no inline error")
	}
	delete(f.fail, "/work/locked")
	retried, cmd := pressKey(failed2, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("corrected retry issued nothing")
	}
	ok, _ := runList(t, retried, cmd)
	if ok.picker.Error() != nil || ok.picker.CurrentDir() != "/work/locked" {
		t.Fatalf("corrected retry: err %v dir %q", ok.picker.Error(), ok.picker.CurrentDir())
	}
	if ok.picker.Highlight() != 0 {
		t.Fatalf("highlight after retry = %d, want 0", ok.picker.Highlight())
	}
}

func TestPickerVerifyFailureRetainsEverythingForRetry(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"out", true}}
	opener, p := savePickerModel(t, f)
	baseline := openerFingerprint(opener)
	p, _ = pressKey(p, tea.KeyMsg{Type: tea.KeyTab})
	p, _ = pressKey(p, runeKey("ré port"))
	f.fail["/work"] = fs.ErrPermission
	p, cmd := pressKey(p, tea.KeyMsg{Type: tea.KeyEnter})
	settled, msg := runList(t, p, cmd)
	verifyMsg, ok := msg.(PickerVerifyMsg)
	if !ok || verifyMsg.Path != "/work/ré port.sql" {
		t.Fatalf("verify = %+v, want the unicode-joined destination", msg)
	}
	err := settled.picker.Error()
	if err == nil || !filepicker.IsPermission(err) {
		t.Fatalf("verify failure not inline: %v", err)
	}
	if got := settled.picker.Filename(); got != "ré port" || settled.picker.Cursor() != 7 {
		t.Fatalf("filename/cursor not retained: %q %d", got, settled.picker.Cursor())
	}
	if fp := openerFingerprint(settled); fp != baseline {
		t.Errorf("verify failure opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
	}
	// Nothing was serialized or written: the fake recorded only reads.
	if len(f.reads) != 2 {
		t.Fatalf("reads = %v, want start list + submit verify only", f.reads)
	}
	// Retry then completes and restores the opener through the Issue #53
	// inspection and write stages.
	delete(f.fail, "/work")
	retried, cmd := pressKey(settled, tea.KeyMsg{Type: tea.KeyEnter})
	final, msgs2 := runSaveCmds(t, retried, cmd)
	if len(msgs2) == 0 {
		t.Fatal("retry ran no save-flow messages")
	}
	if _, ok := msgs2[0].(PickerVerifyMsg); !ok {
		t.Fatalf("retry message = %T, want verify", msgs2[0])
	}
	if final.pickerOpen {
		t.Fatal("picker still open after completed retry")
	}
	if got := final.saveCompletedPath; got != "/work/ré port.sql" {
		t.Fatalf("saveCompletedPath = %q", got)
	}
	if fp := openerFingerprint(final); fp != baseline {
		t.Errorf("completed retry opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
	}
}

func TestPickerPerformsNoDatabaseWork(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{{"sub", true}}
	sel := &fakeSelectExecutor{}
	count := &fakeCountExecutor{total: 1}
	page := &fakePageExecutor{rowsShown: 3}
	refresh := &fakeRefresher{}
	m := New()
	m.PickerFS = f
	m.PickerStart = "/work"
	m.Select = sel.selectPage
	m.Count = count.count
	m.Page = page.page
	m.Refresher = refresh
	m.savePrepared = &export.SQLSaveTarget{}

	pm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlS))
	pm, _ = runList(t, pm, cmd)
	// Navigate, edit, submit, cancel — every key through Update.
	for _, k := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyTab},
		runeKey("x"),
		tea.KeyMsg{Type: tea.KeyEnter},
	} {
		next, c := pressKey(pm, k)
		pm, _ = runList(t, next, c)
	}
	esc, _ := pressKey(pm, tea.KeyMsg{Type: tea.KeyEscape})
	if esc.pickerOpen {
		t.Fatal("final Esc did not cancel")
	}
	if sel.calls+count.calls+page.issued+refresh.calls != 0 {
		t.Fatalf("picker flow issued database work: sel %d count %d page %d refresh %d",
			sel.calls, count.calls, page.issued, refresh.calls)
	}
}
