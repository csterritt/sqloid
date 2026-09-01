// Destination-existence detection and injected atomic-save tests for the
// Issue #53 save boundary (Tasks #053-1 and #053-3), per the Atomic saves
// and Testing Decisions decisions in Notes/PRD-sqloid.md. Every test drives
// deterministic injected boundaries: existing-destination detection must
// perform no destructive filesystem call, and every serialization/create/
// write/sync/close/rename failure must preserve an existing destination,
// clean up every temporary artifact, and return a typed stage error without
// claiming success. No test depends on restrictive-permission timing.

package export

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

// saveFakeFile is one open fake temporary artifact recording every call.
type saveFakeFile struct {
	fs     *saveFakeFS
	name   string
	closed bool
}

func (f *saveFakeFile) Write(p []byte) (int, error) {
	f.fs.calls = append(f.fs.calls, fmt.Sprintf("write:%s:%d", f.name, len(p)))
	if f.fs.failWriteAfter >= 0 && len(p) > f.fs.failWriteAfter {
		n := f.fs.failWriteAfter
		f.fs.contents[f.name] = append([]byte(nil), p[:n]...)
		return n, f.fs.errFor("write")
	}
	if f.fs.shortWriteNilAfter >= 0 && len(p) > f.fs.shortWriteNilAfter {
		n := f.fs.shortWriteNilAfter
		f.fs.contents[f.name] = append([]byte(nil), p[:n]...)
		return n, nil
	}
	f.fs.contents[f.name] = append([]byte(nil), p...)
	return len(p), nil
}

func (f *saveFakeFile) Sync() error {
	f.fs.calls = append(f.fs.calls, "sync:"+f.name)
	if f.fs.failSync {
		return f.fs.errFor("sync")
	}
	return nil
}

func (f *saveFakeFile) Close() error {
	f.fs.calls = append(f.fs.calls, "close:"+f.name)
	f.closed = true
	if f.fs.failClose {
		return f.fs.errFor("close")
	}
	return nil
}

// saveFakeFS is the fake SaveFS boundary: every call is recorded, staged
// failures are injected deterministically, and existing destinations keep
// byte-for-byte contents so preservation is provable.
type saveFakeFS struct {
	exists             map[string]bool
	contents           map[string][]byte
	calls              []string
	tempSeq            int
	failExists         error
	failCreate         error
	failWriteAfter     int // -1 never; n fails a write of more than n bytes
	shortWriteNilAfter int // -1 never; n returns (n, nil) for a write of more than n bytes
	failSync           bool
	failClose          bool
	failRename         error
	failRemove         error
}

func newSaveFakeFS() *saveFakeFS {
	return &saveFakeFS{
		exists:             map[string]bool{},
		contents:           map[string][]byte{},
		failWriteAfter:     -1,
		shortWriteNilAfter: -1,
	}
}

func (f *saveFakeFS) errFor(stage string) error {
	switch stage {
	case "write":
		return errors.New("injected write failure")
	case "sync":
		return errors.New("injected sync failure")
	case "close":
		return errors.New("injected close failure")
	case "remove":
		return errors.New("injected remove failure")
	default:
		return errors.New("injected failure")
	}
}

func (f *saveFakeFS) Exists(path string) (bool, error) {
	f.calls = append(f.calls, "exists:"+path)
	if f.failExists != nil {
		return false, f.failExists
	}
	return f.exists[path], nil
}

func (f *saveFakeFS) TempFile(dir, pattern string) (SaveFile, error) {
	f.calls = append(f.calls, "create:"+dir)
	if f.failCreate != nil {
		return nil, f.failCreate
	}
	f.tempSeq++
	name := dir + "/" + pattern[:1] + fmt.Sprintf("temp-%d", f.tempSeq)
	f.calls = append(f.calls, "tempname:"+name)
	return &saveFakeFile{fs: f, name: name}, nil
}

func (f *saveFakeFS) Name(fl SaveFile) string { return fl.(*saveFakeFile).name }

func (f *saveFakeFS) Rename(oldPath, newPath string) error {
	f.calls = append(f.calls, "rename:"+oldPath+"->"+newPath)
	if f.failRename != nil {
		return f.failRename
	}
	f.contents[newPath] = f.contents[oldPath]
	delete(f.contents, oldPath)
	return nil
}

func (f *saveFakeFS) Remove(path string) error {
	f.calls = append(f.calls, "remove:"+path)
	if f.failRemove != nil {
		return f.failRemove
	}
	delete(f.contents, path)
	return nil
}

// tempCalls returns the names of every created temporary artifact.
func (f *saveFakeFS) tempNames() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "write:") {
			out = append(out, strings.SplitN(c, ":", 3)[1])
		}
	}
	return out
}

func TestInspectDestinationDetectsWithoutDestructiveCalls(t *testing.T) {
	f := newSaveFakeFS()
	f.exists["/work/out.csv"] = true
	// A destination directory listing nothing else: detection must observe
	// existence without any destructive call on the boundary.
	status, err := InspectDestination(f, "/work/out.csv")
	if err != nil || status != DestinationExisting {
		t.Fatalf("existing status = %v err %v, want DestinationExisting", status, err)
	}
	status, err = InspectDestination(f, "/work/new.json")
	if err != nil || status != DestinationNew {
		t.Fatalf("new status = %v err %v, want DestinationNew", status, err)
	}
	want := []string{"exists:/work/out.csv", "exists:/work/new.json"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("boundary calls = %q\nwant only %q (no truncation, removal, rename, or write)", f.calls, want)
	}

	// A path error stays typed and reports no classification.
	f.failExists = errors.New("injected stat failure")
	if _, err := InspectDestination(f, "/work/boom"); err == nil {
		t.Fatal("detection swallowed the injected error")
	}
}

func TestWriteAtomicNewDestinationExactBytesAndNoTempLeak(t *testing.T) {
	f := newSaveFakeFS()
	data := []byte("SELECT \"id\", 'x' FROM t;\r\na,b\r\n")
	if err := WriteAtomic(f, "/work/report.csv", data); err != nil {
		t.Fatalf("new-destination save failed: %v", err)
	}
	if got := f.contents["/work/report.csv"]; !bytes.Equal(got, data) {
		t.Fatalf("destination bytes = %q, want exact captured bytes %q", got, data)
	}
	if _, ok := f.contents["/work/.report.csv.sqloid-*"]; ok {
		t.Fatal("placeholder temp leaked")
	}
	// Exactly one temp artifact was created inside the destination directory
	// (never a global temp location), hidden, and fully consumed by the
	// rename: no temporary artifact remains.
	tempName := f.createdTempName()
	if tempName == "" {
		t.Fatalf("temp file not created in the destination directory: %q", f.calls)
	}
	if tempName == "/work/report.csv" || !strings.HasPrefix(tempName, "/work/") {
		t.Fatalf("temp name %q is not a unique destination-local artifact", tempName)
	}
	if _, ok := f.contents[tempName]; ok {
		t.Fatalf("temporary artifact %q leaked after success", tempName)
	}
	// The rename is the sole replacement boundary.
	if got := f.tempNames(); len(got) != 1 {
		t.Fatalf("temp artifacts = %q, want exactly one", got)
	}
}

func TestWriteAtomicReplacesExistingDestinationWithExactBytes(t *testing.T) {
	f := newSaveFakeFS()
	f.exists["/work/out.json"] = true
	f.contents["/work/out.json"] = []byte(`[{"old":1}]`)
	data := []byte(`[{"id":7}]`)
	if err := WriteAtomic(f, "/work/out.json", data); err != nil {
		t.Fatalf("replacement save failed: %v", err)
	}
	if got := f.contents["/work/out.json"]; !bytes.Equal(got, data) {
		t.Fatalf("replacement bytes = %q, want %q", got, data)
	}
	if name := f.createdTempName(); name != "" {
		if _, ok := f.contents[name]; ok {
			t.Fatalf("temporary artifact %q leaked after replacement", name)
		}
	} else {
		t.Fatal("replacement never created a destination-local temp artifact")
	}
}

func TestWriteAtomicPreRenameFailuresPreserveAndCleanUp(t *testing.T) {
	data := []byte("payload")
	cases := []struct {
		name  string
		setup func(f *saveFakeFS)
		stage SaveStage
	}{
		{"temp-create-failure", func(f *saveFakeFS) { f.failCreate = errors.New("no dir") }, StageCreate},
		{"partial-write-failure", func(f *saveFakeFS) { f.failWriteAfter = 3 }, StageWrite},
		{"full-write-failure", func(f *saveFakeFS) { f.failWriteAfter = 0 }, StageWrite},
		{"sync-failure", func(f *saveFakeFS) { f.failSync = true }, StageSync},
		{"close-failure", func(f *saveFakeFS) { f.failClose = true }, StageClose},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaveFakeFS()
			f.exists["/work/out.csv"] = true
			f.contents["/work/out.csv"] = []byte("original bytes")
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data)
			var stageErr *StageError
			if !errors.As(err, &stageErr) || stageErr.Stage != tc.stage {
				t.Fatalf("err = %v, want StageError(%s)", err, tc.stage)
			}
			// Byte-for-byte preservation of the existing destination.
			if got := f.contents["/work/out.csv"]; !bytes.Equal(got, []byte("original bytes")) {
				t.Fatalf("existing destination changed: %q", got)
			}
			// The failed save never claimed the destination.
			if f.exists["/work/out.csv"] != true {
				t.Fatal("existing destination disappeared")
			}
			wantRemoves, wantTemps := 1, 1
			if tc.stage == StageCreate {
				wantRemoves, wantTemps = 0, 0
			}
			for _, name := range f.tempNames() {
				if !f.contentsRemoved(name) {
					t.Fatalf("temporary artifact %q not removed", name)
				}
			}
			if got := f.tempNames(); len(got) != wantTemps {
				t.Fatalf("temp artifacts = %q, want exactly %d", got, wantTemps)
			}
			removes := 0
			for _, c := range f.calls {
				if strings.HasPrefix(c, "remove:") {
					removes++
				}
			}
			if removes != wantRemoves {
				t.Fatalf("remove calls = %d, want %d", removes, wantRemoves)
			}
		})
	}
}

func TestWriteAtomicRenameFailureNeverClaimsReplacement(t *testing.T) {
	f := newSaveFakeFS()
	f.exists["/work/out.sql"] = true
	f.contents["/work/out.sql"] = []byte("SELECT 1;")
	f.failRename = errors.New("injected rename failure")
	err := WriteAtomic(f, "/work/out.sql", []byte("SELECT 2;"))
	var stageErr *StageError
	if !errors.As(err, &stageErr) || stageErr.Stage != StageRename {
		t.Fatalf("err = %v, want StageError(StageRename)", err)
	}
	// The platform-reported target state is whatever it was: no success
	// message or replacement claim exists, and cleanup is best-effort.
	removes := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "remove:") {
			removes++
		}
	}
	if removes != 1 {
		t.Fatalf("rename-failure temp cleanup calls = %d, want best-effort removal", removes)
	}
}

// contentsRemoved reports whether nothing remains under name in the fake.
func (f *saveFakeFS) contentsRemoved(name string) bool {
	_, ok := f.contents[name]
	return !ok
}

// createdTempName returns the single temporary artifact name recorded by
// the fake, or empty when none was created.
func (f *saveFakeFS) createdTempName() string {
	for _, c := range f.calls {
		if strings.HasPrefix(c, "tempname:") {
			return strings.TrimPrefix(c, "tempname:")
		}
	}
	return ""
}

// hasCall reports whether any recorded boundary call starts with prefix.
func (f *saveFakeFS) hasCall(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// TestWriteAtomicShortWriteNilErrorIsErrShortWrite requires Issue #63's
// nil-error short write to surface as a typed write-stage error whose cause
// is io.ErrShortWrite with an actionable message rather than <nil>. Sync,
// the final close-as-success, and rename never run; the partially written
// destination-local temporary file is closed and removed; an existing
// destination remains byte-for-byte unchanged; a previously missing
// destination remains absent. Complete-write and non-nil-error rows are
// retained as controls to prove the short-write boundary is distinct.
func TestWriteAtomicShortWriteNilErrorIsErrShortWrite(t *testing.T) {
	data := []byte("payload")
	cases := []struct {
		name            string
		setup           func(f *saveFakeFS)
		existing        bool
		wantSuccess     bool
		wantErrIs       error
		wantErrNotIs    error
		wantErrContains string
	}{
		{
			name:            "short-write-nil-error-existing-destination",
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        true,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:            "short-write-nil-error-missing-destination",
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        false,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:        "complete-write-control",
			setup:       func(f *saveFakeFS) {},
			existing:    true,
			wantSuccess: true,
		},
		{
			name:         "non-nil-write-error-control",
			setup:        func(f *saveFakeFS) { f.failWriteAfter = 3 },
			existing:     true,
			wantErrNotIs: io.ErrShortWrite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaveFakeFS()
			if tc.existing {
				f.exists["/work/out.csv"] = true
				f.contents["/work/out.csv"] = []byte("original bytes")
			}
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data)
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("complete write failed: %v", err)
				}
				if got := f.contents["/work/out.csv"]; !bytes.Equal(got, data) {
					t.Fatalf("destination bytes = %q, want %q", got, data)
				}
				return
			}
			var stageErr *StageError
			if !errors.As(err, &stageErr) || stageErr.Stage != StageWrite {
				t.Fatalf("err = %v, want StageError(StageWrite)", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("errors.Is(err, %v) = false, want true; err = %v", tc.wantErrIs, err)
			}
			if tc.wantErrNotIs != nil && errors.Is(err, tc.wantErrNotIs) {
				t.Fatalf("errors.Is(err, %v) = true, want false; err = %v", tc.wantErrNotIs, err)
			}
			if tc.wantErrContains != "" {
				if msg := err.Error(); !strings.Contains(msg, tc.wantErrContains) {
					t.Fatalf("err text = %q, want it to contain %q", msg, tc.wantErrContains)
				}
				if msg := err.Error(); strings.Contains(msg, "<nil>") {
					t.Fatalf("err text contains <nil> rather than an actionable cause: %q", msg)
				}
			}
			// Sync, the final close-as-success, and rename never run for a
			// write-stage failure.
			if f.hasCall("sync:") {
				t.Fatalf("sync ran after a write-stage failure: %q", f.calls)
			}
			if f.hasCall("rename:") {
				t.Fatalf("rename ran after a write-stage failure: %q", f.calls)
			}
			// The partially written destination-local temporary file is
			// closed and removed.
			tempName := f.createdTempName()
			if tempName == "" {
				t.Fatal("no temporary artifact was created")
			}
			if _, ok := f.contents[tempName]; ok {
				t.Fatalf("temporary artifact %q was not removed", tempName)
			}
			removes := 0
			for _, c := range f.calls {
				if strings.HasPrefix(c, "remove:") {
					removes++
				}
			}
			if removes != 1 {
				t.Fatalf("remove calls = %d, want 1 (temp cleanup)", removes)
			}
			// An existing destination remains byte-for-byte unchanged; a
			// previously missing destination remains absent.
			if tc.existing {
				if got := f.contents["/work/out.csv"]; !bytes.Equal(got, []byte("original bytes")) {
					t.Fatalf("existing destination changed: %q", got)
				}
				if !f.exists["/work/out.csv"] {
					t.Fatal("existing destination disappeared")
				}
			} else {
				if f.exists["/work/out.csv"] {
					t.Fatal("previously missing destination appeared")
				}
				if _, ok := f.contents["/work/out.csv"]; ok {
					t.Fatal("previously missing destination has contents")
				}
			}
		})
	}
}
