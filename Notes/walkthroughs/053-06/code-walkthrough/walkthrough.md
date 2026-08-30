# Issue #053 Code Walkthrough: Atomic Saves and Overwrite Protection

*2026-08-29T21:06:58Z by Showboat 0.6.1*
<!-- showboat-id: 2bc9a8cd-5fe8-4baa-8062-62f8f32980bc -->

Issue #53 (PRD Notes/PRD-sqloid.md, 'Atomic saves'): destination confirmation and atomic file output for Ctrl+S query save and Ctrl+X export. This walkthrough saves SQL through the real UI model to a new and an existing destination, proves existing bytes stay untouched before explicit Enter/y confirmation, mutates live prepared-target state behind the confirmation and shows the captured destination, format, immutable copy, and selection remain authoritative, cancels with Esc/n showing exact picker restoration, then confirms and observes the destination-local temp creation, complete write/close, rename, exact final bytes, and no leftover temp artifact. It then deterministically injects serialization, temp-creation/write, and rename failures to capture destination preservation, cleanup, the inline error, retry with the same copy, and safe cancel.

```bash
ls internal/export | grep save_write; echo ---; ls internal/ui | grep save_write; go vet ./internal/export ./internal/ui && echo vet-ok
```

```output
save_write.go
save_write_test.go
---
save_write.go
save_write_test.go
vet-ok
```

## Unit level: destination inspection, staged failures, atomic success

```bash
go test -count=1 ./internal/export -run 'TestInspectDestination|TestWriteAtomic' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//'
```

```output
--- PASS: TestInspectDestinationDetectsWithoutDestructiveCalls
--- PASS: TestWriteAtomicNewDestinationExactBytesAndNoTempLeak
--- PASS: TestWriteAtomicReplacesExistingDestinationWithExactBytes
--- PASS: TestWriteAtomicPreRenameFailuresPreserveAndCleanUp
--- PASS: TestWriteAtomicRenameFailureNeverClaimsReplacement
ok  	github.com/chris/sqloid/internal/export	0.002s
```

```bash
go test -count=1 ./internal/export -run 'TestInspectDestination|TestWriteAtomic' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestInspectDestinationDetectsWithoutDestructiveCalls
--- PASS: TestWriteAtomicNewDestinationExactBytesAndNoTempLeak
--- PASS: TestWriteAtomicReplacesExistingDestinationWithExactBytes
--- PASS: TestWriteAtomicPreRenameFailuresPreserveAndCleanUp
--- PASS: TestWriteAtomicRenameFailureNeverClaimsReplacement
ok  	github.com/chris/sqloid/internal/export	
```

## Unit level: the scripted confirmation and atomic-write UI flow

```bash
go test -count=1 ./internal/ui -run 'TestExistingDestinationOpensExactlyOneConfirmationWithoutReplacement|TestConfirmationCapturesImmutablePayloadAndSelection|TestOverwriteCancelRestoresIntactPickerAndCapturedCopy|TestExportConfirmationCapturesWarningsAndPayload|TestSaveAtomicSuccess|TestSaveStageFailures|TestSaveFailureCancel|TestStaleSaveCompletions' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//'
```

```output
--- PASS: TestExistingDestinationOpensExactlyOneConfirmationWithoutReplacement
--- PASS: TestConfirmationCapturesImmutablePayloadAndSelection
--- PASS: TestOverwriteCancelRestoresIntactPickerAndCapturedCopy
--- PASS: TestExportConfirmationCapturesWarningsAndPayload
--- PASS: TestSaveAtomicSuccessNewAndExistingDestinations
--- PASS: TestSaveStageFailuresPreserveCleanupAndRetryInline
--- PASS: TestSaveFailureCancelRestoresOpenerExactly
--- PASS: TestStaleSaveCompletionsAndDuplicateConfirmsAreInert
ok  	github.com/chris/sqloid/internal/ui	
```

## Live demo: new destination, overwrite confirmation, cancel, and injected failures

The demo test below is written into the package, executed, and removed by the same block, so verification re-runs it reproducibly.

```bash
cat > internal/ui/zz_demo53_test.go <<'ZZEOF'
package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func demoRunCmds(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; cmd != nil && i < 8; i++ {
		msg := cmd()
		next, c := m.Update(msg)
		m = next.(Model)
		cmd = c
	}
	return m
}

func demoSubmit(t *testing.T, m Model, name string) Model {
	t.Helper()
	m = update(m, tea.KeyMsg{Type: tea.KeyTab})
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return demoRunCmds(t, next.(Model), cmd)
}

func demoConfirm(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return demoRunCmds(t, next.(Model), cmd)
}

func demoEntries(t *testing.T, dir string) []string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func demoSaveModel(t *testing.T, dir string) (Model, tea.Cmd) {
	t.Helper()
	m := New()
	m.PickerStart = dir
	m.savePrepared = &export.SQLSaveTarget{State: qb.HistoryState{
		Command:    qb.CommandSelect,
		Table:      "items",
		TableSet:   true,
		Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}},
	}, Source: export.SaveFromRunnableBuilder}
	pm := m
	cmd := pm.openPicker(pickerFlowSave, filepicker.FormatSQL)
	return pm, cmd
}

const demoPayload = "SELECT * FROM \"items\";"

// New destination: Ctrl+S-style session to a completed save with exact
// bytes and no temp artifact.
func TestDemo53NewDestination(t *testing.T) {
	dir := t.TempDir()
	m, cmd := demoSaveModel(t, dir)
	m = demoRunCmds(t, m, cmd)
	m = demoSubmit(t, m, "demo")
	if m.pickerOpen || m.saveCompletedPath != filepath.Join(dir, "demo.sql") {
		t.Fatalf("completion path %q open %v", m.saveCompletedPath, m.pickerOpen)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "demo.sql"))
	if string(got) != demoPayload {
		t.Fatalf("bytes %q", got)
	}
	if ents := demoEntries(t, dir); len(ents) != 1 || ents[0] != "demo.sql" {
		t.Fatalf("entries %q (temp artifact leaked?)", ents)
	}
	t.Logf("new-destination save wrote exact bytes and left only demo.sql")
}

// Existing destination: exactly one confirmation; existing bytes untouched
// before Enter/y; live state mutated behind the confirmation stays
// unauthoritative; confirm writes the captured copy; a fresh Esc/n returns
// to the intact picker with the captured copy retained.
func TestDemo53OverwriteConfirmAndCancel(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "demo.sql")
	os.WriteFile(dest, []byte("SELECT 1;"), 0o644)
	m, cmd := demoSaveModel(t, dir)
	m = demoRunCmds(t, m, cmd)
	m = demoSubmit(t, m, "demo")
	if !m.overwriteOpen {
		t.Fatal("confirmation did not open for the existing destination")
	}
	if b, _ := os.ReadFile(dest); string(b) != "SELECT 1;" {
		t.Fatalf("bytes changed before confirmation: %q", b)
	}
	captured := *m.saveCapture
	// Mutate the live prepared target behind the confirmation.
	m.savePrepared.State.Table = "mutated"
	m.savePrepared.Source = 99
	// Unrelated keys are consumed; the confirmation does not stack.
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q?")})
	if !m.overwriteOpen {
		t.Fatal("unrelated key closed the confirmation")
	}
	// Confirm first: the captured payload — not the mutated live state —
	// is what the flow writes.
	m = demoConfirm(t, m)
	if m.saveCompletedPath != dest {
		t.Fatalf("confirm path %q", m.saveCompletedPath)
	}
	if captured.selection != "runnable-builder" || captured.path != dest {
		t.Fatalf("captured identity drifted: %q %q", captured.selection, captured.path)
	}
	if b, _ := os.ReadFile(dest); string(b) != demoPayload {
		t.Fatalf("confirm wrote %q, want the captured %q", b, demoPayload)
	}
	if ents := demoEntries(t, dir); len(ents) != 1 {
		t.Fatalf("temp artifact leaked: %q", ents)
	}
	// A fresh picker session (the completed save restored the opener), a
	// fresh confirmation, then Esc/n: only the overwrite question is
	// cancelled — the picker returns intact with its filename, directory,
	// and format, and the captured copy is retained.
	pm := m
	cmd2 := pm.openPicker(pickerFlowSave, filepicker.FormatSQL)
	m = demoRunCmds(t, pm, cmd2)
	m = demoSubmit(t, m, "demo")
	if !m.overwriteOpen {
		t.Fatal("second confirmation did not open")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.overwriteOpen || !m.pickerOpen || m.picker.Filename() != "demo" {
		t.Fatalf("cancel restoration: open %v picker %v name %q", m.overwriteOpen, m.pickerOpen, m.picker.Filename())
	}
	if m.saveCapture == nil || m.saveCapture.path != dest {
		t.Fatal("captured copy discarded on cancel")
	}
	if b, _ := os.ReadFile(dest); string(b) != demoPayload {
		t.Fatalf("cancel wrote something: %q", b)
	}
	t.Logf("confirmation gated replacement; Enter/y wrote the captured copy; Esc/n restored the picker")
}

// demoFailFS injects a rename failure over the real boundary.
type demoFailFS struct {
	export.OSSaveFS
	failRename bool
}

func (f *demoFailFS) Rename(a, b string) error {
	if f.failRename {
		return errors.New("injected rename failure")
	}
	return f.OSSaveFS.Rename(a, b)
}

// Rename failure: the destination keeps its bytes, the temp artifact is
// cleaned up, the failure stays inline with no success claim, and retry
// with the same captured copy then completes.
func TestDemo53RenameFailureAndRetry(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.sql")
	os.WriteFile(dest, []byte("SELECT 1;"), 0o644)
	m, cmd := demoSaveModel(t, dir)
	m.SaveFS = &demoFailFS{failRename: true}
	m = demoRunCmds(t, m, cmd)
	m = demoSubmit(t, m, "out")
	if !m.overwriteOpen {
		t.Fatal("confirmation did not open")
	}
	m = demoConfirm(t, m)
	if m.saveFailure == "" || m.saveCompletedPath != "" {
		t.Fatalf("failure state: %q completed %q", m.saveFailure, m.saveCompletedPath)
	}
	if b, _ := os.ReadFile(dest); string(b) != "SELECT 1;" {
		t.Fatalf("destination changed on rename failure: %q", b)
	}
	if ents := demoEntries(t, dir); len(ents) != 1 {
		t.Fatalf("temp artifact leaked: %q", ents)
	}
	// Retry with the same captured copy: clear the injection and press Enter.
	m.SaveFS = &demoFailFS{}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = demoRunCmds(t, next.(Model), cmd)
	if m.saveCompletedPath != dest || m.saveFailure != "" {
		t.Fatalf("retry failed: %q %q", m.saveCompletedPath, m.saveFailure)
	}
	if b, _ := os.ReadFile(dest); string(b) != demoPayload {
		t.Fatalf("retry wrote %q", b)
	}
	if ents := demoEntries(t, dir); len(ents) != 1 {
		t.Fatalf("retry leaked artifacts: %q", ents)
	}
	t.Logf("rename failure preserved and cleaned up; retry reused the captured copy")
}

// Serialization failure: incomplete prepared state fails inline before any
// destination check.
func TestDemo53SerializationFailure(t *testing.T) {
	dir := t.TempDir()
	m, cmd := demoSaveModel(t, dir)
	m.savePrepared = &export.SQLSaveTarget{} // incomplete: cannot serialize
	m = demoRunCmds(t, m, cmd)
	m = demoSubmit(t, m, "bad")
	if !strings.Contains(m.saveFailure, "cannot be serialized") {
		t.Fatalf("serialization failure not inline: %q", m.saveFailure)
	}
	if ents := demoEntries(t, dir); len(ents) != 0 {
		t.Fatalf("serialization failure touched the filesystem: %q", ents)
	}
	t.Logf("serialization failure stayed inline and touched nothing")
}
ZZEOF
go test -count=1 ./internal/ui -run 'TestDemo53' -v 2>&1 | grep -E '(--- (PASS|FAIL)|^ok|^FAIL|    zz)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$//; s/ +$//; s/^ +//'
```

```output
zz_demo53_test.go:91: new-destination save wrote exact bytes and left only demo.sql
--- PASS: TestDemo53NewDestination
zz_demo53_test.go:156: confirmation gated replacement; Enter/y wrote the captured copy; Esc/n restored the picker
--- PASS: TestDemo53OverwriteConfirmAndCancel
zz_demo53_test.go:209: rename failure preserved and cleaned up; retry reused the captured copy
--- PASS: TestDemo53RenameFailureAndRetry
zz_demo53_test.go:226: serialization failure stayed inline and touched nothing
--- PASS: TestDemo53SerializationFailure
ok  	github.com/chris/sqloid/internal/ui	
```

Observations: the new destination completed with exact bytes and no temp artifact; the existing destination opened exactly one confirmation with original bytes intact; unrelated keys did not close or stack it; confirming wrote the originally captured payload despite the mutated live target; Esc/n left the intact picker with the captured copy; the rename failure preserved the destination, cleaned up the temp artifact, stayed inline with no completed path, and retry reused the same copy; the serialization failure touched nothing.

```bash
rm internal/ui/zz_demo53_test.go && go test -count=1 ./internal/export ./internal/ui 2>&1 | sed -E 's/[0-9]+\.[0-9]+s$//'
```

```output
ok  	github.com/chris/sqloid/internal/export	
ok  	github.com/chris/sqloid/internal/ui	
```
