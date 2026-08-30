# Issue #052 Code Walkthrough: Directory and Filename Picker Flow

This walkthrough demonstrates the Issue #52 directory and filename picker flow (internal/filepicker, internal/ui) against Notes/PRD-sqloid.md: working-directory start, directory-only listing with .. first and bytewise non-natural order, separate literal filename entry, extension completion, inline validation and permission errors with retained state, exact opener restoration, and zero database work. Overwrite confirmation and atomic temp-and-rename persistence are owned by later issues.

```bash
ls internal/filepicker internal/ui | grep -E 'filepicker|picker_flow|navigation_test|filename' ; echo ---; ls internal/ui/filepicker.go internal/ui/picker_flow_test.go
```

```output
internal/filepicker:
filename.go
filename_test.go
filepicker.go
navigation_test.go
filepicker.go
outcome_unknown_navigation_test.go
picker_flow_test.go
query_history_navigation_test.go
---
internal/ui/filepicker.go
internal/ui/picker_flow_test.go
```

## Model tests: navigation, ordering, validation

```bash
go test -count=1 ./internal/filepicker -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | grep -v '/ ' | head -20 | sed -E 's/ ?\(0\.[0-9]+s\)//; s/\t[0-9]+\.[0-9]+s$/ [ok]/'
```

```output
=== RUN   TestValidateFilename
=== RUN   TestValidateFilename/empty_basename
=== RUN   TestValidateFilename/slash
=== RUN   TestValidateFilename/trailing_slash
=== RUN   TestValidateFilename/absolute_destination
=== RUN   TestValidateFilename/NUL_byte
=== RUN   TestValidateFilename/valid_simple
=== RUN   TestValidateFilename/dots_and_spaces
=== RUN   TestValidateFilename/leading_dot
=== RUN   TestValidateFilename/multiple_extensions
=== RUN   TestValidateFilename/mixed_case
=== RUN   TestValidateFilename/unicode
=== RUN   TestValidateFilename/question_mark_and_q
--- PASS: TestValidateFilename
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce/missing_sql
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce/missing_csv
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce/missing_json
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce/complete_sql_preserved
=== RUN   TestCompleteNameAppendsRequiredExtensionExactlyOnce/complete_csv_preserved
```

## UI scripted flow tests: openers, literal keys, restoration, zero database work

```bash
go test -count=1 ./internal/ui -run 'TestPicker' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/\t[0-9]+\.[0-9]+s$/ [ok]/'
```

```output
--- PASS: TestPickerOpensAtWorkingDirectoryAcrossOpeners
--- PASS: TestPickerNavigationKeysAndSeparateFilename
--- PASS: TestPickerFilenameLiteralKeysAndValidation
--- PASS: TestPickerCompletionRestoresOpenerExactly
--- PASS: TestPickerEscRestoresOpenerFromEveryFocusAndErrorState
--- PASS: TestPickerRetryRetainsStateAndRejectsStaleResponses
--- PASS: TestPickerVerifyFailureRetainsEverythingForRetry
--- PASS: TestPickerPerformsNoDatabaseWork
ok  	github.com/chris/sqloid/internal/ui [ok]
```

## Live model demo: navigation, filename entry, validation, permission retry

The demo test below is written into the package, executed, and removed by the same block, so verification re-runs it reproducibly.

```bash
cat > internal/filepicker/zz_demo_test.go <<'ZZEOF'
package filepicker

import (
	"io/fs"
	"testing"
)

type demoEntry struct{ name string; dir bool }
func (e demoEntry) Name() string { return e.name }
func (e demoEntry) IsDir() bool  { return e.dir }

type demoFS struct{ dirs map[string][]demoEntry; fail map[string]error }
func (f *demoFS) ReadDir(p string) ([]DirEntry, error) {
	if err, ok := f.fail[p]; ok { return nil, err }
	es, ok := f.dirs[p]
	if !ok { return nil, fs.ErrNotExist }
	out := make([]DirEntry, len(es))
	for i, e := range es { out[i] = e }
	return out, nil
}

func loadDirs(f *demoFS, p string) []string {
	dirs := []string{}
	for _, e := range f.dirs[p] { if e.IsDir() { dirs = append(dirs, e.Name()) } }
	return dirs
}

func TestZZPickerDemo(t *testing.T) {
	f := &demoFS{
		dirs: map[string][]demoEntry{
			"/work": {
				{"Zebra", true}, {"apple", true}, {"d10", true}, {"d2", true},
				{".conf", true}, {"-punct", true}, {"é中", true}, {"f.txt", false},
			},
			"/work/.conf": {},
			"/work/d2":    {{"nested", true}},
		},
		fail: map[string]error{},
	}
	m, req := Start(f, "/work", FormatSQL)
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: loadDirs(f, req.Path)})
	t.Logf("listing: %q (files never listed; d10 before d2: not natural)", m.Listing())

	for m.Highlighted() != "d2" { m.MoveHighlight(1) }
	req, _ = m.EnterDir()
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: loadDirs(f, req.Path)})
	t.Logf("entered /work/d2: dir=%q listing=%q", m.CurrentDir(), m.Listing())
	req, _ = m.EnterDir() // .. is highlighted first
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: loadDirs(f, req.Path)})
	t.Logf("after ..: dir=%q listing=%q", m.CurrentDir(), m.Listing())

	m.SetFocus(FocusFilename)
	m.InsertRunes([]rune("q?final"))
	t.Logf("filename with literal ?/q: %q", m.Filename())

	for m.Cursor() > 0 { m.Backspace() }
	if _, ok := m.Submit(); ok { t.Fatal("expected empty rejection") }
	t.Logf("empty submit error: %v", m.Error())
	m.InsertRunes([]rune("a/b"))
	if _, ok := m.Submit(); ok { t.Fatal("expected slash rejection") }
	t.Logf("slash submit error: %v", m.Error())
	for m.Cursor() > 0 { m.Backspace() }
	m.InsertRunes([]rune("report"))
	t.Logf("completed name: %q -> %q", m.Filename(), CompleteName(m.Filename(), FormatSQL))

	f.fail["/work/d2"] = fs.ErrPermission
	for m.Highlighted() != "d2" { m.MoveHighlight(1) }
	req, _ = m.EnterDir()
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Err: fs.ErrPermission})
	t.Logf("permission error: %v (dir=%q retained, filename=%q)", m.Error(), m.CurrentDir(), m.Filename())
	delete(f.fail, "/work/d2")
	req, ok := m.Retry()
	if !ok || req.Path != "/work/d2" { t.Fatalf("retry %+v", req) }
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: loadDirs(f, req.Path)})
	t.Logf("after corrected retry: dir=%q err=%v", m.CurrentDir(), m.Error())
}
ZZEOF
go test -count=1 ./internal/filepicker -run TestZZPickerDemo -v 2>&1 | grep -E "demo_test|--- (PASS|FAIL)" | sed -E 's/ ?\(0\.[0-9]+s\)//'; rm internal/filepicker/zz_demo_test.go
```

```output
    zz_demo_test.go:42: listing: [".." "-punct" ".conf" "Zebra" "apple" "d10" "d2" "é中"] (files never listed; d10 before d2: not natural)
    zz_demo_test.go:47: entered /work/d2: dir="/work/d2" listing=[".." "nested"]
    zz_demo_test.go:50: after ..: dir="/work" listing=[".." "-punct" ".conf" "Zebra" "apple" "d10" "d2" "é中"]
    zz_demo_test.go:54: filename with literal ?/q: "q?final"
    zz_demo_test.go:58: empty submit error: filename is empty
    zz_demo_test.go:61: slash submit error: filename may not contain '/'
    zz_demo_test.go:64: completed name: "report" -> "report.sql"
    zz_demo_test.go:70: permission error: could not open directory: permission denied (dir="/work" retained, filename="report")
    zz_demo_test.go:75: after corrected retry: dir="/work/d2" err=<nil>
--- PASS: TestZZPickerDemo
```

## Locale independence of ordering

The same suite passes under C, en_US.UTF-8, and de_DE.UTF-8 locales — the model never consults the process locale.

```bash
for loc in C en_US.UTF-8 de_DE.UTF-8; do LC_ALL=$loc LANG=$loc go test -count=1 ./internal/filepicker 2>&1 | tail -1 | sed -E 's/\t[0-9]+\.[0-9]+s$/ [ok]/'; done
```

```output
ok  	github.com/chris/sqloid/internal/filepicker [ok]
ok  	github.com/chris/sqloid/internal/filepicker [ok]
ok  	github.com/chris/sqloid/internal/filepicker [ok]
```

## Reference

Issue #52 and the File picker, Global Key Precedence and Context/Action Matrix, UI Module Design, and picker Testing Decisions sections of Notes/PRD-sqloid.md. Wiki: Notes/wiki/file-picker.md. Overwrite confirmation of an existing destination and atomic temp-and-rename persistence belong to their owning later issues; the picker records the verified destination only.
