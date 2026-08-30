// UI-independent save destination boundary and atomic file output
// (Issue #53), per the Atomic saves decision in Notes/PRD-sqloid.md.
// Destination inspection is a typed filesystem-boundary check: it reports
// whether the resolved destination already exists through the injected
// SaveFS boundary without truncating, removing, renaming, writing, or
// opening the destination. The write stage is destination-local atomic
// replacement: a uniquely named temporary file is created in the resolved
// destination's own directory (never a global temp location), receives the
// already-captured immutable bytes exactly once, is synced and closed, and
// a final rename is the sole replacement boundary. Every pre-rename failure
// closes whatever was opened, removes every temporary artifact, and returns
// a typed stage error without touching an existing destination; a rename
// failure is reported as an inline-retry error, cleans up best-effort, and
// never claims that replacement occurred. No picker, model, or database
// concept enters this package's save boundary.

package export

import (
	"fmt"
	"os"
	"path/filepath"
)

// SaveStage names the one save stage a StageError failed at: serialization,
// temporary-file creation, write, sync, close, or the final rename.
type SaveStage int

const (
	// StageSerialize is the serialization of the immutable payload.
	StageSerialize SaveStage = iota + 1
	// StageCreate is destination-local temporary-file creation.
	StageCreate
	// StageWrite is the single write of the captured bytes.
	StageWrite
	// StageSync is the temporary file's sync.
	StageSync
	// StageClose is the temporary file's pre-rename close.
	StageClose
	// StageRename is the final replacement rename.
	StageRename
)

// String renders the stage name for tests and inline diagnostics.
func (s SaveStage) String() string {
	switch s {
	case StageSerialize:
		return "serialization"
	case StageCreate:
		return "temporary-file creation"
	case StageWrite:
		return "write"
	case StageSync:
		return "sync"
	case StageClose:
		return "close"
	case StageRename:
		return "rename"
	default:
		return "SaveStage(out-of-range)"
	}
}

// StageError is the typed inline-retry save failure: it names the exact
// failed stage and wraps the underlying cause. No success is ever claimed
// alongside it.
type StageError struct {
	Stage SaveStage
	Err   error
}

// Error renders the stage-qualified failure message.
func (e *StageError) Error() string {
	return fmt.Sprintf("save failed at %s: %v", e.Stage, e.Err)
}

// Unwrap exposes the cause for errors.Is/As inspection.
func (e *StageError) Unwrap() error { return e.Err }

// SaveFile is one open temporary save artifact the boundary owns until the
// rename or cleanup completes.
type SaveFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// SaveFS is the injected save filesystem boundary: destination-existence
// detection and the complete temporary-file lifecycle. Every call is
// observable by tests; the real implementation is OSSaveFS.
type SaveFS interface {
	// Exists reports whether path already exists. It never truncates,
	// removes, renames, writes, or opens the destination.
	Exists(path string) (bool, error)
	// TempFile creates a uniquely named temporary file inside dir. Pattern
	// follows os.CreateTemp semantics; implementations must resolve the
	// name inside dir, never a global temp location.
	TempFile(dir, pattern string) (SaveFile, error)
	// Name returns the filesystem path of one open temporary artifact so
	// cleanup can remove it.
	Name(f SaveFile) string
	// Rename atomically moves oldPath over newPath: the sole replacement
	// boundary.
	Rename(oldPath, newPath string) error
	// Remove deletes one path (cleanup of temporary artifacts only).
	Remove(path string) error
}

// OSSaveFS is the real SaveFS over the operating system's filesystem.
type OSSaveFS struct{}

// Exists implements SaveFS via os.Stat without any destructive call.
func (OSSaveFS) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// TempFile implements SaveFS via os.CreateTemp inside the given directory.
func (OSSaveFS) TempFile(dir, pattern string) (SaveFile, error) {
	return os.CreateTemp(dir, pattern)
}

// Name implements SaveFS for os.File-backed temporary artifacts.
func (OSSaveFS) Name(f SaveFile) string {
	return f.(*os.File).Name()
}

// Rename implements SaveFS via os.Rename.
func (OSSaveFS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// Remove implements SaveFS via os.Remove.
func (OSSaveFS) Remove(path string) error { return os.Remove(path) }

// DestinationStatus classifies one resolved save destination.
type DestinationStatus int

const (
	// DestinationNew means nothing exists at the resolved destination.
	DestinationNew DestinationStatus = iota + 1
	// DestinationExisting means a file already occupies the destination.
	DestinationExisting
)

// InspectDestination classifies the resolved destination through the
// injected boundary. Detection performs no truncation, removal, rename, or
// write, and never opens or serializes anything: callers use the result to
// route a new path straight to the write stage and an existing path into
// exactly one overwrite confirmation.
func InspectDestination(fs SaveFS, path string) (DestinationStatus, error) {
	exists, err := fs.Exists(path)
	if err != nil {
		return DestinationNew, err
	}
	if exists {
		return DestinationExisting, nil
	}
	return DestinationNew, nil
}

// TempPattern returns the destination-local, non-colliding temporary-file
// pattern for base: a hidden dotfile of the target name inside the
// destination directory, so it can never collide with or expose a partial
// target through its final name.
func TempPattern(base string) string {
	return "." + base + ".sqloid-*"
}

// WriteAtomic performs the atomic output of exactly the supplied immutable
// bytes at path: a uniquely named temporary file created in the resolved
// destination directory (never a global temp location), one single write of
// the captured bytes, sync, complete close, and a final rename as the sole
// replacement boundary. Every pre-rename failure closes what was opened,
// removes the temporary artifact, and returns a typed StageError without
// touching an existing destination. A rename failure removes the temporary
// artifact best-effort and returns a StageRename error: replacement is
// never claimed. Success is reported only after the rename succeeded.
func WriteAtomic(fs SaveFS, path string, data []byte) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	f, err := fs.TempFile(dir, TempPattern(base))
	if err != nil {
		return &StageError{Stage: StageCreate, Err: err}
	}
	tmpName := fs.Name(f)
	cleanup := func() error {
		// Best-effort close for stages that leave the file open; the close
		// stage itself never closes twice.
		_ = f.Close()
		return fs.Remove(tmpName)
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		_ = cleanup()
		return &StageError{Stage: StageWrite, Err: err}
	}
	if err := f.Sync(); err != nil {
		_ = cleanup()
		return &StageError{Stage: StageSync, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageClose, Err: err}
	}
	if err := fs.Rename(tmpName, path); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageRename, Err: err}
	}
	return nil
}
