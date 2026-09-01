// Destination-existence detection and injected atomic-save tests for the
// Issue #53/#64 save boundary (Tasks #053-1, #053-3, #063-1, #064-1), per
// the Atomic saves and Testing Decisions decisions in Notes/PRD-sqloid.md.
// Every test drives deterministic injected boundaries: existing-destination
// detection must perform no destructive filesystem call, and every
// serialization/create/write/sync/close/rename failure must preserve an
// existing destination, clean up every temporary artifact, and return a
// typed stage error without claiming success. Issue #64 adds race-safe
// no-replace and confirmed-replacement tests: an unconfirmed save raced by
// external creation preserves the raced file and returns a typed
// DestinationExistsError; a confirmed replacement raced by external change
// preserves the changed file and returns a typed DestinationChangedError;
// unchanged confirmed state replaces atomically. No test depends on
// restrictive-permission timing.

package export

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

// saveFakeFile is one open fake save artifact recording every call.
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
// byte-for-byte contents so preservation is provable. Identity tokens use
// a monotonic counter so a replaced file gets a new identity even with
// identical contents.
type saveFakeFS struct {
	exists              map[string]bool
	contents            map[string][]byte
	identity            map[string]DestinationIdentity
	calls               []string
	tempSeq             int
	idSeq               int
	failStat            error
	failCreate          error
	failCreateExclusive error
	failWriteAfter      int // -1 never; n fails a write of more than n bytes
	shortWriteNilAfter  int // -1 never; n returns (n, nil) for a write of more than n bytes
	failSync            bool
	failClose           bool
	failRename          error
	failRemove          error
}

func newSaveFakeFS() *saveFakeFS {
	return &saveFakeFS{
		exists:             map[string]bool{},
		contents:           map[string][]byte{},
		identity:           map[string]DestinationIdentity{},
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

// freshID returns a new unique DestinationIdentity for a newly created or
// replaced destination.
func (f *saveFakeFS) freshID() DestinationIdentity {
	f.idSeq++
	return DestinationIdentity{Ino: uint64(f.idSeq)}
}

func (f *saveFakeFS) Stat(path string) (DestinationIdentity, error) {
	f.calls = append(f.calls, "stat:"+path)
	if f.failStat != nil {
		return DestinationIdentity{}, f.failStat
	}
	if !f.exists[path] {
		return DestinationIdentity{}, os.ErrNotExist
	}
	return f.identity[path], nil
}

func (f *saveFakeFS) CreateExclusive(path string) (SaveFile, error) {
	f.calls = append(f.calls, "create-exclusive:"+path)
	if f.failCreateExclusive != nil {
		return nil, f.failCreateExclusive
	}
	if f.exists[path] {
		return nil, os.ErrExist
	}
	f.exists[path] = true
	f.identity[path] = f.freshID()
	f.tempSeq++
	return &saveFakeFile{fs: f, name: path}, nil
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
	f.identity[newPath] = f.freshID()
	delete(f.contents, oldPath)
	delete(f.identity, oldPath)
	return nil
}

func (f *saveFakeFS) Remove(path string) error {
	f.calls = append(f.calls, "remove:"+path)
	if f.failRemove != nil {
		return f.failRemove
	}
	delete(f.contents, path)
	if f.exists[path] {
		// Only the no-replace path removes the destination itself on
		// pre-close failure; the replace path removes only temp artifacts.
		delete(f.exists, path)
		delete(f.identity, path)
	}
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

// setExisting marks path as an existing destination with the given contents
// and a fresh identity token.
func (f *saveFakeFS) setExisting(path string, contents []byte) {
	f.exists[path] = true
	f.contents[path] = append([]byte(nil), contents...)
	f.identity[path] = f.freshID()
}

// replaceExisting simulates an external process replacing the destination:
// new contents and a new identity token.
func (f *saveFakeFS) replaceExisting(path string, contents []byte) {
	f.exists[path] = true
	f.contents[path] = append([]byte(nil), contents...)
	f.identity[path] = f.freshID()
}

// removeExisting simulates an external process removing the destination.
func (f *saveFakeFS) removeExisting(path string) {
	delete(f.exists, path)
	delete(f.contents, path)
	delete(f.identity, path)
}

// createExternal simulates an external process creating a previously
// missing destination.
func (f *saveFakeFS) createExternal(path string, contents []byte) {
	f.exists[path] = true
	f.contents[path] = append([]byte(nil), contents...)
	f.identity[path] = f.freshID()
}

func TestInspectDestinationDetectsWithoutDestructiveCalls(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("old"))
	// A destination directory listing nothing else: detection must observe
	// existence without any destructive call on the boundary.
	state, err := InspectDestination(f, "/work/out.csv")
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("existing status = %v err %v, want DestinationExisting", state, err)
	}
	if state.Identity.Equal(DestinationIdentity{}) {
		t.Fatal("existing destination has no identity token")
	}
	state, err = InspectDestination(f, "/work/new.json")
	if err != nil || state.Status != DestinationNew {
		t.Fatalf("new status = %v err %v, want DestinationNew", state, err)
	}
	if !state.Identity.Equal(DestinationIdentity{}) {
		t.Fatal("missing destination has a non-zero identity token")
	}
	want := []string{"stat:/work/out.csv", "stat:/work/new.json"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("boundary calls = %q\nwant only %q (no truncation, removal, rename, or write)", f.calls, want)
	}

	// A path error stays typed and reports no classification.
	f.failStat = errors.New("injected stat failure")
	if _, err := InspectDestination(f, "/work/boom"); err == nil {
		t.Fatal("detection swallowed the injected error")
	}
}

func TestWriteAtomicNoReplaceNewDestinationExactBytesAndNoTempLeak(t *testing.T) {
	f := newSaveFakeFS()
	data := []byte("SELECT \"id\", 'x' FROM t;\r\na,b\r\n")
	state, err := InspectDestination(f, "/work/report.csv")
	if err != nil || state.Status != DestinationNew {
		t.Fatalf("inspect = %v err %v, want DestinationNew", state, err)
	}
	if err := WriteAtomic(f, "/work/report.csv", data, state, IntentNoReplace); err != nil {
		t.Fatalf("no-replace save failed: %v", err)
	}
	if got := f.contents["/work/report.csv"]; !bytes.Equal(got, data) {
		t.Fatalf("destination bytes = %q, want exact captured bytes %q", got, data)
	}
	// The no-replace path publishes directly at the destination through
	// exclusive creation — no temp file, no rename.
	if f.hasCall("rename:") {
		t.Fatalf("no-replace path renamed: %q", f.calls)
	}
	if f.hasCall("create:") {
		t.Fatalf("no-replace path created a temp file: %q", f.calls)
	}
}

func TestWriteAtomicReplaceExistingDestinationWithExactBytes(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.json", []byte(`[{"old":1}]`))
	state, err := InspectDestination(f, "/work/out.json")
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("inspect = %v err %v, want DestinationExisting", state, err)
	}
	data := []byte(`[{"id":7}]`)
	if err := WriteAtomic(f, "/work/out.json", data, state, IntentReplace); err != nil {
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

func TestWriteAtomicReplacePreRenameFailuresPreserveAndCleanUp(t *testing.T) {
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
			f.setExisting("/work/out.csv", []byte("original bytes"))
			state, _ := InspectDestination(f, "/work/out.csv")
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data, state, IntentReplace)
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

func TestWriteAtomicReplaceRenameFailureNeverClaimsReplacement(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.sql", []byte("SELECT 1;"))
	state, _ := InspectDestination(f, "/work/out.sql")
	f.failRename = errors.New("injected rename failure")
	err := WriteAtomic(f, "/work/out.sql", []byte("SELECT 2;"), state, IntentReplace)
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
		intent          SaveIntent
		setup           func(f *saveFakeFS)
		existing        bool
		wantSuccess     bool
		wantErrIs       error
		wantErrNotIs    error
		wantErrContains string
	}{
		{
			name:            "short-write-nil-error-replace-existing",
			intent:          IntentReplace,
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        true,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:            "short-write-nil-error-no-replace-missing",
			intent:          IntentNoReplace,
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        false,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:        "complete-write-replace-control",
			intent:      IntentReplace,
			setup:       func(f *saveFakeFS) {},
			existing:    true,
			wantSuccess: true,
		},
		{
			name:         "non-nil-write-error-replace-control",
			intent:       IntentReplace,
			setup:        func(f *saveFakeFS) { f.failWriteAfter = 3 },
			existing:     true,
			wantErrNotIs: io.ErrShortWrite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaveFakeFS()
			var state DestinationState
			if tc.existing {
				f.setExisting("/work/out.csv", []byte("original bytes"))
			}
			state, _ = InspectDestination(f, "/work/out.csv")
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data, state, tc.intent)
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
			// The partially written artifact is closed and removed. For the
			// replace path the artifact is a destination-local temp file;
			// for the no-replace path it is the exclusively created
			// destination itself (removed because the write failed).
			if tc.intent == IntentReplace {
				tempName := f.createdTempName()
				if tempName == "" {
					t.Fatal("no temp artifact was created")
				}
				if _, ok := f.contents[tempName]; ok {
					t.Fatalf("temp artifact %q was not removed", tempName)
				}
			} else {
				if !f.hasCall("create-exclusive:") {
					t.Fatal("no exclusive creation was attempted")
				}
			}
			removes := 0
			for _, c := range f.calls {
				if strings.HasPrefix(c, "remove:") {
					removes++
				}
			}
			if removes != 1 {
				t.Fatalf("remove calls = %d, want 1 (artifact cleanup)", removes)
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

// TestWriteAtomicNoReplaceRacedByExternalCreation requires an unconfirmed
// save inspected as missing to refuse persistence when another process
// creates the destination before the atomic exclusive creation. The raced
// file is preserved byte-for-byte, no temp file or rename occurs, and a
// typed DestinationExistsError is returned — never a StageError and never
// success.
func TestWriteAtomicNoReplaceRacedByExternalCreation(t *testing.T) {
	f := newSaveFakeFS()
	data := []byte("payload")
	state, err := InspectDestination(f, "/work/out.csv")
	if err != nil || state.Status != DestinationNew {
		t.Fatalf("inspect = %v err %v, want DestinationNew", state, err)
	}
	// Deterministic barrier: another process creates the destination
	// after inspection and before persistence.
	raced := []byte("external bytes")
	f.createExternal("/work/out.csv", raced)
	err = WriteAtomic(f, "/work/out.csv", data, state, IntentNoReplace)
	var existsErr *DestinationExistsError
	if !errors.As(err, &existsErr) {
		t.Fatalf("err = %v, want *DestinationExistsError", err)
	}
	if existsErr.Path != "/work/out.csv" {
		t.Fatalf("exists error path = %q, want /work/out.csv", existsErr.Path)
	}
	// The raced file is preserved byte-for-byte.
	if got := f.contents["/work/out.csv"]; !bytes.Equal(got, raced) {
		t.Fatalf("raced destination changed: %q, want %q", got, raced)
	}
	// No replacement occurred: no rename, no temp file.
	if f.hasCall("rename:") {
		t.Fatalf("no-replace race path renamed: %q", f.calls)
	}
	if f.hasCall("create:") {
		t.Fatalf("no-replace race path created a temp file: %q", f.calls)
	}
	// No leaked artifacts.
	if f.hasCall("remove:") {
		t.Fatalf("no-replace race path removed something: %q", f.calls)
	}
}

// TestWriteAtomicNoReplaceCreateExclusiveFailureIsStageError requires a
// non-existence CreateExclusive failure (e.g., permission denied) to be a
// StageCreate StageError, not a DestinationExistsError.
func TestWriteAtomicNoReplaceCreateExclusiveFailureIsStageError(t *testing.T) {
	f := newSaveFakeFS()
	state, _ := InspectDestination(f, "/work/out.csv")
	f.failCreateExclusive = errors.New("permission denied")
	err := WriteAtomic(f, "/work/out.csv", []byte("payload"), state, IntentNoReplace)
	var stageErr *StageError
	if !errors.As(err, &stageErr) || stageErr.Stage != StageCreate {
		t.Fatalf("err = %v, want StageError(StageCreate)", err)
	}
	var existsErr *DestinationExistsError
	if errors.As(err, &existsErr) {
		t.Fatal("non-existence create failure classified as DestinationExistsError")
	}
}

// TestWriteAtomicNoReplacePreCloseFailuresCleanUp requires the no-replace
// path to remove the exclusively created (partial) destination on write,
// sync, or close failure, leaving no artifact behind.
func TestWriteAtomicNoReplacePreCloseFailuresCleanUp(t *testing.T) {
	data := []byte("payload")
	cases := []struct {
		name  string
		setup func(f *saveFakeFS)
		stage SaveStage
	}{
		{"write-failure", func(f *saveFakeFS) { f.failWriteAfter = 3 }, StageWrite},
		{"sync-failure", func(f *saveFakeFS) { f.failSync = true }, StageSync},
		{"close-failure", func(f *saveFakeFS) { f.failClose = true }, StageClose},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaveFakeFS()
			state, _ := InspectDestination(f, "/work/out.csv")
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data, state, IntentNoReplace)
			var stageErr *StageError
			if !errors.As(err, &stageErr) || stageErr.Stage != tc.stage {
				t.Fatalf("err = %v, want StageError(%s)", err, tc.stage)
			}
			// The partially created destination is removed.
			if f.exists["/work/out.csv"] {
				t.Fatal("partial no-replace destination not removed")
			}
			if _, ok := f.contents["/work/out.csv"]; ok {
				t.Fatal("partial no-replace destination has contents")
			}
			// No rename ever ran.
			if f.hasCall("rename:") {
				t.Fatalf("no-replace failure path renamed: %q", f.calls)
			}
		})
	}
}

// TestWriteAtomicReplaceRacedByExternalChange requires a confirmed
// replacement to refuse persistence when another process replaces the
// destination between inspection and the last safe point. The changed file
// is preserved byte-for-byte, the staged temp is cleaned up, no rename
// occurs, and a typed DestinationChangedError is returned.
func TestWriteAtomicReplaceRacedByExternalChange(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("original"))
	state, err := InspectDestination(f, "/work/out.csv")
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("inspect = %v err %v, want DestinationExisting", state, err)
	}
	inspectedID := state.Identity
	// Deterministic barrier: another process replaces the destination
	// after inspection and before persistence.
	changed := []byte("external replacement")
	f.replaceExisting("/work/out.csv", changed)
	err = WriteAtomic(f, "/work/out.csv", []byte("payload"), state, IntentReplace)
	var changedErr *DestinationChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("err = %v, want *DestinationChangedError", err)
	}
	if changedErr.Path != "/work/out.csv" {
		t.Fatalf("changed error path = %q", changedErr.Path)
	}
	if changedErr.Missing {
		t.Fatal("changed error reports missing but destination was replaced")
	}
	// The changed file is preserved byte-for-byte.
	if got := f.contents["/work/out.csv"]; !bytes.Equal(got, changed) {
		t.Fatalf("changed destination overwritten: %q, want %q", got, changed)
	}
	// The identity changed from the inspected token.
	currentID := f.identity["/work/out.csv"]
	if currentID.Equal(inspectedID) {
		t.Fatal("replaced destination has the same identity as inspected")
	}
	// No rename occurred.
	if f.hasCall("rename:") {
		t.Fatalf("replace race path renamed: %q", f.calls)
	}
	// The staged temp was cleaned up.
	tempName := f.createdTempName()
	if tempName == "" {
		t.Fatal("no temp artifact was created")
	}
	if _, ok := f.contents[tempName]; ok {
		t.Fatalf("temp artifact %q not cleaned up after race", tempName)
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
}

// TestWriteAtomicReplaceRacedByExternalRemoval requires a confirmed
// replacement to refuse persistence when another process removes the
// destination between inspection and the last safe point. The staged temp
// is cleaned up and a typed DestinationChangedError with Missing=true is
// returned.
func TestWriteAtomicReplaceRacedByExternalRemoval(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("original"))
	state, _ := InspectDestination(f, "/work/out.csv")
	// Deterministic barrier: another process removes the destination.
	f.removeExisting("/work/out.csv")
	err := WriteAtomic(f, "/work/out.csv", []byte("payload"), state, IntentReplace)
	var changedErr *DestinationChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("err = %v, want *DestinationChangedError", err)
	}
	if !changedErr.Missing {
		t.Fatal("changed error does not report missing")
	}
	// No rename occurred.
	if f.hasCall("rename:") {
		t.Fatalf("replace race path renamed: %q", f.calls)
	}
	// The staged temp was cleaned up.
	tempName := f.createdTempName()
	if tempName == "" {
		t.Fatal("no temp artifact was created")
	}
	if _, ok := f.contents[tempName]; ok {
		t.Fatalf("temp artifact %q not cleaned up after race", tempName)
	}
}

// TestWriteAtomicReplaceUnchangedStateSucceeds requires a confirmed
// replacement to proceed atomically when the destination state remains
// unchanged from inspection to the last safe point.
func TestWriteAtomicReplaceUnchangedStateSucceeds(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("original"))
	state, _ := InspectDestination(f, "/work/out.csv")
	data := []byte("new payload")
	if err := WriteAtomic(f, "/work/out.csv", data, state, IntentReplace); err != nil {
		t.Fatalf("unchanged replacement failed: %v", err)
	}
	if got := f.contents["/work/out.csv"]; !bytes.Equal(got, data) {
		t.Fatalf("replacement bytes = %q, want %q", got, data)
	}
	// The rename is the sole replacement boundary.
	if !f.hasCall("rename:") {
		t.Fatalf("no rename occurred: %q", f.calls)
	}
	// No temp leaked.
	tempName := f.createdTempName()
	if tempName != "" {
		if _, ok := f.contents[tempName]; ok {
			t.Fatalf("temp artifact %q leaked", tempName)
		}
	}
}

// TestWriteAtomicReplaceStageAttributionRetained requires the replace path
// to retain Issue #53 stage attribution for every pre-rename failure and
// Issue #63 short-write conversion. The re-verification stat runs only
// after successful close and before rename.
func TestWriteAtomicReplaceStageAttributionRetained(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("original"))
	state, _ := InspectDestination(f, "/work/out.csv")
	// A successful replace calls stat after close and before rename.
	_ = WriteAtomic(f, "/work/out.csv", []byte("new"), state, IntentReplace)
	stats := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "stat:") {
			stats++
		}
	}
	// One stat from InspectDestination, one from the pre-rename
	// re-verification.
	if stats != 2 {
		t.Fatalf("stat calls = %d, want 2 (inspect + pre-rename verify)", stats)
	}
}

// TestWriteAtomicInvalidIntentReturnsError requires an unknown intent to
// be rejected without touching the destination.
func TestWriteAtomicInvalidIntentReturnsError(t *testing.T) {
	f := newSaveFakeFS()
	f.setExisting("/work/out.csv", []byte("original"))
	state, _ := InspectDestination(f, "/work/out.csv")
	err := WriteAtomic(f, "/work/out.csv", []byte("payload"), state, SaveIntent(99))
	if err == nil {
		t.Fatal("invalid intent did not return an error")
	}
	if got := f.contents["/work/out.csv"]; !bytes.Equal(got, []byte("original")) {
		t.Fatalf("invalid intent touched the destination: %q", got)
	}
}
