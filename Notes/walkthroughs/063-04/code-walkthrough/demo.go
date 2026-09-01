package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chris/sqloid/internal/export"
)

// miniFakeFS is a minimal SaveFS for the walkthrough demonstration, modeled
// on the Issue #53/#64 fake filesystem in internal/export/save_write_test.go.
type miniFakeFile struct {
	fs   *miniFakeFS
	name string
}

func (f *miniFakeFile) Write(p []byte) (int, error) {
	f.fs.calls = append(f.fs.calls, fmt.Sprintf("write:%s:%d", f.name, len(p)))
	if f.fs.failWriteAfter >= 0 && len(p) > f.fs.failWriteAfter {
		n := f.fs.failWriteAfter
		f.fs.contents[f.name] = append([]byte(nil), p[:n]...)
		return n, errors.New("injected write failure")
	}
	if f.fs.shortWriteNilAfter >= 0 && len(p) > f.fs.shortWriteNilAfter {
		n := f.fs.shortWriteNilAfter
		f.fs.contents[f.name] = append([]byte(nil), p[:n]...)
		return n, nil // nil-error short write
	}
	f.fs.contents[f.name] = append([]byte(nil), p...)
	return len(p), nil
}

func (f *miniFakeFile) Sync() error {
	f.fs.calls = append(f.fs.calls, "sync:"+f.name)
	return nil
}

func (f *miniFakeFile) Close() error {
	f.fs.calls = append(f.fs.calls, "close:"+f.name)
	return nil
}

type miniFakeFS struct {
	exists             map[string]bool
	contents           map[string][]byte
	identity           map[string]export.DestinationIdentity
	calls              []string
	tempSeq            int
	idSeq              int
	failWriteAfter     int
	shortWriteNilAfter int
}

func newMiniFakeFS() *miniFakeFS {
	return &miniFakeFS{
		exists:             map[string]bool{},
		contents:           map[string][]byte{},
		identity:           map[string]export.DestinationIdentity{},
		failWriteAfter:     -1,
		shortWriteNilAfter: -1,
	}
}

func (f *miniFakeFS) freshID() export.DestinationIdentity {
	f.idSeq++
	return export.DestinationIdentity{Ino: uint64(f.idSeq)}
}

func (f *miniFakeFS) Stat(path string) (export.DestinationIdentity, error) {
	f.calls = append(f.calls, "stat:"+path)
	if !f.exists[path] {
		return export.DestinationIdentity{}, os.ErrNotExist
	}
	return f.identity[path], nil
}

func (f *miniFakeFS) CreateExclusive(path string) (export.SaveFile, error) {
	f.calls = append(f.calls, "create-exclusive:"+path)
	if f.exists[path] {
		return nil, os.ErrExist
	}
	f.exists[path] = true
	f.identity[path] = f.freshID()
	f.tempSeq++
	return &miniFakeFile{fs: f, name: path}, nil
}

func (f *miniFakeFS) TempFile(dir, pattern string) (export.SaveFile, error) {
	f.calls = append(f.calls, "create:"+dir)
	f.tempSeq++
	name := dir + "/" + pattern[:1] + fmt.Sprintf("temp-%d", f.tempSeq)
	f.calls = append(f.calls, "tempname:"+name)
	return &miniFakeFile{fs: f, name: name}, nil
}

func (f *miniFakeFS) Name(fl export.SaveFile) string { return fl.(*miniFakeFile).name }

func (f *miniFakeFS) Rename(oldPath, newPath string) error {
	f.calls = append(f.calls, "rename:"+oldPath+"->"+newPath)
	f.contents[newPath] = f.contents[oldPath]
	f.identity[newPath] = f.freshID()
	delete(f.contents, oldPath)
	delete(f.identity, oldPath)
	return nil
}

func (f *miniFakeFS) Remove(path string) error {
	f.calls = append(f.calls, "remove:"+path)
	delete(f.contents, path)
	if f.exists[path] {
		delete(f.exists, path)
		delete(f.identity, path)
	}
	return nil
}

func (f *miniFakeFS) setExisting(path string, contents []byte) {
	f.exists[path] = true
	f.contents[path] = append([]byte(nil), contents...)
	f.identity[path] = f.freshID()
}

func (f *miniFakeFS) tempName() string {
	for _, c := range f.calls {
		if strings.HasPrefix(c, "tempname:") {
			return strings.TrimPrefix(c, "tempname:")
		}
	}
	return ""
}

func (f *miniFakeFS) hasCall(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func runCase(label string, existing bool, setup func(*miniFakeFS)) {
	fmt.Printf("=== %s ===\n", label)
	f := newMiniFakeFS()
	dst := "/work/out.csv"
	if existing {
		f.setExisting(dst, []byte("original bytes"))
	}
	setup(f)
	data := []byte("payload")
	state, _ := export.InspectDestination(f, dst)
	intent := export.IntentNoReplace
	if existing {
		intent = export.IntentReplace
	}
	err := export.WriteAtomic(f, dst, data, state, intent)
	fmt.Printf("  error:              %v\n", err)
	fmt.Printf("  errors.Is(ErrShortWrite): %v\n", errors.Is(err, io.ErrShortWrite))
	var se *export.StageError
	if errors.As(err, &se) {
		fmt.Printf("  StageError.Stage:   %s\n", se.Stage)
	}
	fmt.Printf("  boundary calls:     %q\n", f.calls)
	fmt.Printf("  sync ran:           %v\n", f.hasCall("sync:"))
	fmt.Printf("  rename ran:         %v\n", f.hasCall("rename:"))
	tn := f.tempName()
	if tn != "" {
		_, leaked := f.contents[tn]
		fmt.Printf("  temp created:       %s\n", tn)
		fmt.Printf("  temp removed:       %v\n", !leaked)
	}
	if existing {
		got := f.contents[dst]
		fmt.Printf("  destination bytes:  %q (preserved=%v)\n", got, bytes.Equal(got, []byte("original bytes")))
	} else {
		_, present := f.contents[dst]
		fmt.Printf("  destination absent: %v\n", !present)
	}
	fmt.Println()
}

func main() {
	runCase("nil-error short write (n=3, nil) — existing destination", true,
		func(f *miniFakeFS) { f.shortWriteNilAfter = 3 })
	runCase("nil-error short write (n=3, nil) — missing destination", false,
		func(f *miniFakeFS) { f.shortWriteNilAfter = 3 })
	runCase("non-nil write error (n=3, error) — existing destination", true,
		func(f *miniFakeFS) { f.failWriteAfter = 3 })
	runCase("complete write (n=len, nil) — existing destination", true,
		func(f *miniFakeFS) {})
}
