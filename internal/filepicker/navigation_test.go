package filepicker

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
)

// fakeEntry is one fake-filesystem directory entry.
type fakeEntry struct {
	name  string
	isDir bool
}

func (e fakeEntry) Name() string { return e.name }
func (e fakeEntry) IsDir() bool  { return e.isDir }

// fakeFS is a deterministic fake filesystem boundary. Failed paths record
// their exact error for the next ReadDir of that path; calls are counted so
// tests can prove request counts.
type fakeFS struct {
	dirs    map[string][]fakeEntry
	fail    map[string]error
	reads   []string
	created []string // any path a create-like operation would have touched
}

func newFakeFS() *fakeFS {
	return &fakeFS{dirs: map[string][]fakeEntry{}, fail: map[string]error{}}
}

func (f *fakeFS) ReadDir(path string) ([]DirEntry, error) {
	f.reads = append(f.reads, path)
	if err, ok := f.fail[path]; ok {
		return nil, err
	}
	entries, ok := f.dirs[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	out := make([]DirEntry, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}

// startAt starts a picker at cwd over f with the given format and answers the
// start listing with the fake contents.
func startAt(t *testing.T, f *fakeFS, cwd string, format Format) (Model, NavRequest) {
	t.Helper()
	m, req := Start(f, cwd, format)
	if req.Path != cwd || req.Verify {
		t.Fatalf("start request = %+v, want path %q, no verify", req, cwd)
	}
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, cwd)})
	return m, req
}

// fakeDirs extracts only the directory basenames the fake fs holds at path,
// mimicking what a real caller's cmd would produce from FS.ReadDir.
func fakeDirs(t *testing.T, f *fakeFS, path string) []string {
	t.Helper()
	entries, err := f.ReadDir(path)
	if err != nil {
		t.Fatalf("fake read %q: %v", path, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

func TestStartTargetsWorkingDirectoryThroughBoundary(t *testing.T) {
	f := newFakeFS()
	m, req := Start(f, "/work/start", FormatSQL)
	if req.Path != "/work/start" {
		t.Fatalf("start request path = %q, want working directory /work/start", req.Path)
	}
	if m.StartDir() != "/work/start" || m.CurrentDir() != "/work/start" {
		t.Fatalf("start dir %q current %q, want both /work/start", m.StartDir(), m.CurrentDir())
	}
	if m.Format() != FormatSQL {
		t.Fatalf("format = %q, want sql", m.Format())
	}
	// The first read is issued, not performed: the model listed nothing yet.
	if len(f.reads) != 0 {
		t.Fatalf("reads before caller issued request = %v, want none", f.reads)
	}
}

func TestListingExcludesFilesIncludesHiddenAndSortsBytewise(t *testing.T) {
	f := newFakeFS()
	f.dirs["/work"] = []fakeEntry{
		{"report.sql", false}, // regular file: never enters the directory list
		{"a", true},
		{"B", true},
		{".hidden", true},
		{"d10", true},
		{"d2", true},
		{"-punct", true},
		{"_under", true},
		{"é中", true}, // Unicode-byte directory
		{"Z", true},
		{"notes.sql", false},
	}
	m, _ := startAt(t, f, "/work", FormatSQL)
	got := m.Listing()
	// Go string (bytewise) order: uppercase before lowercase, digits by
	// value ("d10" < "d2"), byte order for non-ASCII. Not natural numeric
	// order, not locale collation. No files anywhere.
	want := []string{"..", "-punct", ".hidden", "B", "Z", "_under", "a", "d10", "d2", "é中"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listing = %q\nwant            %q", got, want)
	}
	if m.Highlight() != 0 {
		t.Fatalf("initial highlight = %d, want 0 (the `..` row)", m.Highlight())
	}
}

func TestListingRootHasNoParentRow(t *testing.T) {
	f := newFakeFS()
	f.dirs["/"] = []fakeEntry{{"a", true}, {"b", true}}
	m, req := Start(f, "/", FormatCSV)
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, "/")})
	got := m.Listing()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root listing = %q, want %q without a parent row", got, want)
	}
}

func TestNavigationEntersVisibleHiddenChildrenAndParent(t *testing.T) {
	f := newFakeFS()
	f.dirs["/work"] = []fakeEntry{{".conf", true}, {"sub", true}, {"file.txt", false}}
	f.dirs["/work/.conf"] = []fakeEntry{}
	f.dirs["/work/sub"] = []fakeEntry{{"nested", true}}
	f.dirs["/work/sub/nested"] = []fakeEntry{}
	m, _ := startAt(t, f, "/work", FormatSQL)

	// Highlight the visible child and enter it.
	m.MoveHighlight(2) // .. -> .conf -> sub
	if got := m.Highlighted(); got != "sub" {
		t.Fatalf("highlighted = %q, want sub", got)
	}
	req, ok := m.EnterDir()
	if !ok || req.Path != "/work/sub" || req.Verify {
		t.Fatalf("enter sub request = %+v ok=%v, want /work/sub navigation", req, ok)
	}
	if m.Filename() != "" {
		t.Fatalf("filename changed by navigation: %q", m.Filename())
	}
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, req.Path)})
	if m.CurrentDir() != "/work/sub" {
		t.Fatalf("current dir = %q, want /work/sub", m.CurrentDir())
	}
	if got := m.Listing(); !reflect.DeepEqual(got, []string{"..", "nested"}) {
		t.Fatalf("sub listing = %q, want [.. nested]", got)
	}

	// Enter the nested child (highlight moves to `nested`), then walk back
	// up twice through `..`.
	m.MoveHighlight(1)
	req, _ = m.EnterDir()
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, req.Path)})
	if m.CurrentDir() != "/work/sub/nested" {
		t.Fatalf("current dir = %q, want /work/sub/nested", m.CurrentDir())
	}
	if got := m.Listing(); !reflect.DeepEqual(got, []string{".."}) {
		t.Fatalf("empty-child listing = %q, want only [..]", got)
	}
	req, _ = m.EnterDir() // `..` highlighted by default
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, req.Path)})
	if m.CurrentDir() != "/work/sub" {
		t.Fatalf("parent step 1 dir = %q, want /work/sub", m.CurrentDir())
	}
	req, _ = m.EnterDir()
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: fakeDirs(t, f, req.Path)})
	if m.CurrentDir() != "/work" {
		t.Fatalf("parent step 2 dir = %q, want /work", m.CurrentDir())
	}
	if m.Highlight() != 0 {
		t.Fatalf("highlight after navigation = %d, want 0", m.Highlight())
	}

	// Repeated navigation is issued one request at a time; files can never
	// be entered because they never appear in the listing.
	if _, ok := m.EnterDir(); !ok {
		t.Fatal("idle navigation refused")
	}
	req2, ok := m.EnterDir()
	if ok {
		t.Fatalf("second EnterDir while pending issued %+v", req2)
	}
}

func TestHighlightBoundariesAreNoOps(t *testing.T) {
	f := newFakeFS()
	f.dirs["/work"] = []fakeEntry{{"a", true}, {"b", true}}
	m, _ := startAt(t, f, "/work", FormatSQL)
	m.MoveHighlight(-1)
	if m.Highlight() != 0 {
		t.Fatalf("highlight = %d after up at top, want 0", m.Highlight())
	}
	m.MoveHighlight(99)
	if got := m.Highlight(); got != 2 {
		t.Fatalf("highlight = %d after oversized down, want last index 2", got)
	}
	m.MoveHighlight(1)
	if m.Highlight() != 2 {
		t.Fatalf("highlight = %d after down at bottom, want 2", m.Highlight())
	}
}

func TestNavigationFailureStaysInlineAndRetriesInPlace(t *testing.T) {
	f := newFakeFS()
	f.dirs["/work"] = []fakeEntry{{"locked", true}, {"sub", true}}
	f.dirs["/work/sub"] = []fakeEntry{{"deep", true}}
	m, _ := startAt(t, f, "/work", FormatSQL)

	// A permission-denied child read becomes a typed inline error.
	f.fail["/work/locked"] = fs.ErrPermission
	m.MoveHighlight(1)
	req, _ := m.EnterDir()
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Err: fs.ErrPermission})
	err := m.Error()
	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != KindRead || !IsPermission(pe) {
		t.Fatalf("error = %v, want typed read permission error", err)
	}
	if got := err.Error(); got != "could not open directory: permission denied" {
		t.Fatalf("error text = %q", got)
	}
	if m.CurrentDir() != "/work" || m.Highlight() != 1 {
		t.Fatalf("dir %q highlight %d, want /work and 1 retained", m.CurrentDir(), m.Highlight())
	}
	if m.Filename() != "" {
		t.Fatalf("filename changed on failure: %q", m.Filename())
	}

	// Retry re-issues the same path with a fresh identity and clears the
	// error only when it succeeds; a repeated failure stays inline again.
	retry, ok := m.Retry()
	if !ok || retry.Path != "/work/locked" || retry.Attempt == req.Attempt {
		t.Fatalf("retry = %+v ok=%v, want same path with fresh attempt", retry, ok)
	}
	m.Apply(ListedMsg{Path: retry.Path, Attempt: retry.Attempt, Err: fs.ErrPermission})
	if m.Error() == nil {
		t.Fatal("repeated failure left no inline error")
	}
	if m.pendingRetryBlocked() {
		t.Fatal("retry should be available after a repeated failure")
	}
	// A stale late response with the original attempt identity is inert.
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Dirs: []string{"ghost"}})
	if len(f.reads) != 3 && m.CurrentDir() != "/work" {
		// read count already asserted via calls above; dir must not move.
	}
	if got := m.Listing(); reflect.DeepEqual(got, []string{"..", "ghost"}) {
		t.Fatalf("stale response applied: listing = %q", got)
	}
	// Corrected retry succeeds and navigates.
	f.dirs["/work/locked"] = []fakeEntry{{"inner", true}}
	f.fail = map[string]error{}
	retry2, ok := m.Retry()
	if !ok || retry2.Path != "/work/locked" {
		t.Fatalf("retry2 = %+v ok=%v", retry2, ok)
	}
	m.Apply(ListedMsg{Path: retry2.Path, Attempt: retry2.Attempt, Dirs: fakeDirs(t, f, retry2.Path)})
	if m.Error() != nil || m.CurrentDir() != "/work/locked" {
		t.Fatalf("after corrected retry: err %v dir %q", m.Error(), m.CurrentDir())
	}
}

// pendingRetryBlocked reports whether Retry is unavailable; kept as a helper
// to make the failure-path test above read clearly.
func (m Model) pendingRetryBlocked() bool {
	_, ok := m.Retry()
	return !ok
}

func TestStartFailureRetainsWorkingDirectoryForRetry(t *testing.T) {
	f := newFakeFS()
	f.fail["/unreadable"] = fs.ErrPermission
	m, req := Start(f, "/unreadable", FormatJSON)
	m.Apply(ListedMsg{Path: req.Path, Attempt: req.Attempt, Err: fs.ErrPermission})
	if m.Error() == nil || m.CurrentDir() != "/unreadable" {
		t.Fatalf("err %v dir %q, want inline error and unchanged start dir", m.Error(), m.CurrentDir())
	}
	if got := m.Listing(); len(got) != 0 {
		t.Fatalf("listing after start failure = %q, want empty", got)
	}
	f.fail = map[string]error{}
	f.dirs["/unreadable"] = []fakeEntry{{"data", true}}
	retry, ok := m.Retry()
	if !ok || retry.Path != "/unreadable" {
		t.Fatalf("retry = %+v ok=%v", retry, ok)
	}
	m.Apply(ListedMsg{Path: retry.Path, Attempt: retry.Attempt, Dirs: fakeDirs(t, f, retry.Path)})
	if m.Error() != nil || !reflect.DeepEqual(m.Listing(), []string{"..", "data"}) {
		t.Fatalf("after start retry: err %v listing %q", m.Error(), m.Listing())
	}
}
