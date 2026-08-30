package filepicker

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty basename", "", ErrEmptyFilename},
		{"slash", "a/b", ErrFilenameSlash},
		{"trailing slash", "name/", ErrFilenameSlash},
		{"absolute destination", "/etc/passwd", ErrFilenameSlash},
		{"NUL byte", "na\x00me", ErrFilenameNUL},
		{"valid simple", "report", nil},
		{"dots and spaces", "my report.v2 final", nil},
		{"leading dot", ".env.sql", nil},
		{"multiple extensions", "backup.sql.bak", nil},
		{"mixed case", "ReSuLt.SQL", nil},
		{"unicode", "résumé数据库", nil},
		{"question mark and q", "q?query", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateFilename(tc.in)
			if !errors.Is(got, tc.want) && !(got == nil && tc.want == nil) {
				t.Fatalf("ValidateFilename(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompleteNameAppendsRequiredExtensionExactlyOnce(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		in     string
		want   string
	}{
		{"missing sql", FormatSQL, "report", "report.sql"},
		{"missing csv", FormatCSV, "rows", "rows.csv"},
		{"missing json", FormatJSON, "data", "data.json"},
		{"complete sql preserved", FormatSQL, "report.sql", "report.sql"},
		{"complete csv preserved", FormatCSV, "rows.csv", "rows.csv"},
		{"complete json preserved", FormatJSON, "data.json", "data.json"},
		{"other extension not rewritten", FormatSQL, "report.bak", "report.bak.sql"},
		{"uppercase ext is not the required suffix", FormatSQL, "report.SQL", "report.SQL.sql"},
		{"casing kept verbatim", FormatSQL, "My Report", "My Report.sql"},
		{"dots and leading dot", FormatSQL, ".hidden", ".hidden.sql"},
		{"multiple extensions", FormatJSON, "x.sql.csv", "x.sql.csv.json"},
		{"unicode", FormatCSV, "résultats 中", "résultats 中.csv"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CompleteName(tc.in, tc.format)
			if got != tc.want {
				t.Fatalf("CompleteName(%q, %q) = %q, want %q", tc.in, tc.format, got, tc.want)
			}
			if strings.HasSuffix(tc.in, tc.format.Extension()) && got != tc.in {
				t.Fatalf("complete name %q was rewritten to %q", tc.in, got)
			}
		})
	}
}

// submitFixture builds a picker listed at /work with one child directory.
func submitFixture(t *testing.T, f *fakeFS, format Format) Model {
	t.Helper()
	f.dirs["/work"] = []fakeEntry{{"out", true}}
	m, _ := startAt(t, f, "/work", format)
	return m
}

func TestSubmitRejectsInvalidNamesInlineWithoutFilesystemWork(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrEmptyFilename},
		{"slash", "a/b", ErrFilenameSlash},
		{"NUL", "a\x00b", ErrFilenameNUL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeFS()
			m := submitFixture(t, f, FormatSQL)
			for _, r := range tc.in {
				m.InsertRunes([]rune{r})
			}
			req, ok := m.Submit()
			if ok {
				t.Fatalf("invalid name %q issued verify request %+v", tc.in, req)
			}
			err := m.Error()
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			var pe *Error
			if !errors.As(err, &pe) || pe.Kind != KindValidate {
				t.Fatalf("error = %v, want typed validation error", err)
			}
			// Nothing moved: no completion, no extra reads, no file created.
			if _, done := m.Completed(); done {
				t.Fatal("invalid submit completed")
			}
			if len(f.reads) != 1 || len(f.created) != 0 {
				t.Fatalf("reads %v created %v, want only the start listing and zero writes", f.reads, f.created)
			}
			// Editing clears the inline error and submission may proceed.
			if tc.in == "" {
				m.InsertRunes([]rune("x"))
			} else {
				m.Backspace()
			}
			if m.Error() != nil {
				t.Fatalf("edit did not clear error: %v", m.Error())
			}
		})
	}
}

func TestSubmitVerificationLifecycle(t *testing.T) {
	f := newFakeFS()
	m := submitFixture(t, f, FormatSQL)
	m.InsertRunes([]rune("report"))

	req, ok := m.Submit()
	if !ok || !req.Verify || req.Path != "/work/report.sql" {
		t.Fatalf("submit request = %+v ok=%v, want verify of /work/report.sql", req, ok)
	}
	if !m.Pending() {
		t.Fatal("submit did not mark the picker pending")
	}
	if _, ok := m.Submit(); ok {
		t.Fatal("duplicate submit issued while pending")
	}

	// A stale verification with an old attempt identity is inert.
	m.ApplyVerified(VerifiedMsg{Path: req.Path, Attempt: req.Attempt - 1})
	if _, done := m.Completed(); done {
		t.Fatal("stale verification completed the picker")
	}
	// A failed verification stays inline and retains selection/input/format.
	m.ApplyVerified(VerifiedMsg{Path: req.Path, Attempt: req.Attempt, Err: fs.ErrPermission})
	err := m.Error()
	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != KindVerify || !IsPermission(pe) {
		t.Fatalf("error = %v, want typed verify permission error", err)
	}
	if m.Filename() != "report" || m.Cursor() != 6 || m.Format() != FormatSQL {
		t.Fatalf("retained filename %q cursor %d format %q", m.Filename(), m.Cursor(), m.Format())
	}
	if m.Highlight() != 0 {
		t.Fatalf("highlight = %d, want retained 0", m.Highlight())
	}
	// Corrected retry completes with the exact joined path.
	retry, ok := m.Retry()
	if !ok || !retry.Verify || retry.Path != "/work/report.sql" || retry.Attempt == req.Attempt {
		t.Fatalf("retry = %+v ok=%v", retry, ok)
	}
	m.ApplyVerified(VerifiedMsg{Path: retry.Path, Attempt: retry.Attempt})
	path, done := m.Completed()
	if !done || path != "/work/report.sql" {
		t.Fatalf("completed = %q done=%v, want /work/report.sql", path, done)
	}
}

func TestFilenameEditingKeysAndSeparation(t *testing.T) {
	m := Model{format: FormatCSV}
	m.InsertRunes([]rune("ré"))
	if m.Filename() != "ré" || m.Cursor() != 2 {
		t.Fatalf("after insert filename %q cursor %d", m.Filename(), m.Cursor())
	}
	m.Left()
	m.InsertRunes([]rune("!"))
	if got := m.Filename(); got != "r!é" {
		t.Fatalf("mid-buffer insert = %q, want r!é", got)
	}
	if m.Focus() != FocusDir {
		t.Fatalf("filename editing implicitly changed focus to %v", m.Focus())
	}
	// Editing keys affect only the input.
	m.SetFocus(FocusFilename)
	m.End()
	m.Backspace()
	if got := m.Filename(); got != "r!" {
		t.Fatalf("backspace at end = %q, want r!", got)
	}
	m.Delete() // no-op at end
	if m.Cursor() != len([]rune(m.Filename())) {
		t.Fatalf("cursor %d moved past end by no-op delete", m.Cursor())
	}
	m.Home()
	m.Delete()
	if got := m.Filename(); got != "!" {
		t.Fatalf("delete at home = %q, want !", got)
	}
	// Editing clears a prior inline error but never the file list.
	m.err = &Error{Kind: KindRead, Path: "/x", Err: fs.ErrNotExist}
	m.InsertRunes([]rune("a"))
	if m.Error() != nil {
		t.Fatalf("edit did not clear inline error: %v", m.Error())
	}
}

func TestEditingNeverChangesDirectoryList(t *testing.T) {
	f := newFakeFS()
	m := submitFixture(t, f, FormatJSON)
	before := m.Listing()
	m.SetFocus(FocusFilename)
	m.InsertRunes([]rune("out/..x"))
	m.Home()
	m.End()
	m.SetFocus(FocusDir)
	if got := m.Listing(); !equalStrings(got, before) {
		t.Fatalf("listing changed by editing: %q vs %q", got, before)
	}
}

func equalStrings(a, b []string) bool {
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
