// UI-independent save destination boundary and atomic file output
// (Issue #53), per the Atomic saves decision in Notes/PRD-sqloid.md.
// Destination inspection is a typed filesystem-boundary check: it reports
// whether the resolved destination already exists and captures a durable
// state token through the injected SaveFS boundary without truncating,
// removing, renaming, writing, or opening the destination. Issue #64
// closes the inspection-to-rename race by carrying explicit overwrite
// intent plus the inspected state token into WriteAtomic. An unconfirmed
// new-file save publishes only through an atomic exclusive creation
// (O_CREATE|O_EXCL on Linux/macOS) and never renames over a path that
// appeared after inspection; a confirmed replacement re-verifies the
// destination state at the last safe point (after staging, before the
// sole rename) and refuses with a typed destination-changed error when
// the state no longer matches. Every pre-rename failure closes whatever
// was opened, removes every temporary artifact, and returns a typed
// stage error without touching an existing destination; a rename failure
// is reported as an inline-retry error, cleans up best-effort, and never
// claims that replacement occurred. No picker, model, or database
// concept enters this package's save boundary.

package export

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SaveStage names the one save stage a StageError failed at: serialization,
// temporary-file creation, write, sync, close, or the final rename.
type SaveStage int

const (
	// StageSerialize is the serialization of the immutable payload.
	StageSerialize SaveStage = iota + 1
	// StageCreate is destination-local temporary-file creation (replace
	// path) or exclusive destination creation (no-replace path).
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

// DestinationExistsError reports that an unconfirmed no-replace save found
// the destination already present at the persistence boundary: another
// process created it after inspection. The raced destination is preserved
// byte-for-byte; no replacement occurred. Callers distinguish this from an
// ordinary stage failure via errors.As and route the user through fresh
// inspection and overwrite confirmation.
type DestinationExistsError struct {
	Path string
}

// Error renders the destination-exists race message.
func (e *DestinationExistsError) Error() string {
	return fmt.Sprintf("destination %s already exists", e.Path)
}

// DestinationChangedError reports that a confirmed replacement found the
// destination state changed at the persistence boundary: another process
// replaced, removed, or materially modified it after inspection. The
// changed destination is preserved byte-for-byte; no replacement occurred.
// Callers distinguish this from an ordinary stage failure via errors.As
// and route the user through fresh inspection and a new overwrite
// confirmation for the latest state.
type DestinationChangedError struct {
	Path    string
	Missing bool
}

// Error renders the destination-changed race message.
func (e *DestinationChangedError) Error() string {
	if e.Missing {
		return fmt.Sprintf("destination %s no longer exists", e.Path)
	}
	return fmt.Sprintf("destination %s changed", e.Path)
}

// SaveFile is one open save artifact the boundary owns until the publish
// or cleanup completes. For the no-replace path the artifact is the
// exclusively created destination itself; for the replace path it is the
// destination-local temporary file.
type SaveFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// SaveFS is the injected save filesystem boundary: destination-state
// inspection, exclusive creation, and the complete temporary-file
// lifecycle. Every call is observable by tests; the real implementation is
// OSSaveFS.
type SaveFS interface {
	// Stat returns the durable identity of path, or an error satisfying
	// os.IsNotExist when path is missing. It never truncates, removes,
	// renames, writes, or opens the destination.
	Stat(path string) (DestinationIdentity, error)
	// CreateExclusive opens path for writing with exclusive creation: the
	// call fails if path already exists. The returned SaveFile receives
	// the captured bytes; the destination is published only after Sync
	// and Close complete. Used by the no-replace path.
	CreateExclusive(path string) (SaveFile, error)
	// TempFile creates a uniquely named temporary file inside dir. Pattern
	// follows os.CreateTemp semantics; implementations must resolve the
	// name inside dir, never a global temp location. Used by the replace
	// path.
	TempFile(dir, pattern string) (SaveFile, error)
	// Name returns the filesystem path of one open save artifact so
	// cleanup can remove it.
	Name(f SaveFile) string
	// Rename atomically moves oldPath over newPath: the sole replacement
	// boundary.
	Rename(oldPath, newPath string) error
	// Remove deletes one path (cleanup of temporary artifacts or a failed
	// exclusive creation).
	Remove(path string) error
}

// DestinationStatus classifies one resolved save destination.
type DestinationStatus int

const (
	// DestinationNew means nothing exists at the resolved destination.
	DestinationNew DestinationStatus = iota + 1
	// DestinationExisting means a file already occupies the destination.
	DestinationExisting
)

// DestinationIdentity is the durable, comparable identity of one destination
// at inspection time, captured by InspectDestination and re-verified by
// WriteAtomic before any replacement. The zero value represents a missing
// destination. On Linux/macOS the identity includes device and inode so a
// same-named replacement cannot be mistaken for the inspected file; the
// fake filesystem in tests uses a monotonic counter.
type DestinationIdentity struct {
	Size    int64
	ModTime time.Time
	Ino     uint64
	Dev     uint64
}

// Equal reports whether two identities refer to the same destination state.
func (id DestinationIdentity) Equal(other DestinationIdentity) bool {
	return id.Size == other.Size &&
		id.ModTime.Equal(other.ModTime) &&
		id.Ino == other.Ino &&
		id.Dev == other.Dev
}

// DestinationState is the durable state token returned by InspectDestination:
// the destination status plus the identity captured at inspection time.
// WriteAtomic requires this token alongside an explicit SaveIntent so the
// persistence boundary can refuse a raced destination without a
// check-then-act fallback.
type DestinationState struct {
	Status   DestinationStatus
	Identity DestinationIdentity
}

// SaveIntent declares the caller's overwrite authorization tied to one
// inspected DestinationState.
type SaveIntent int

const (
	// IntentNoReplace means the destination was inspected as missing; the
	// save publishes only through an atomic exclusive creation and never
	// replaces a path that appeared later.
	IntentNoReplace SaveIntent = iota + 1
	// IntentReplace means the destination was inspected as existing and the
	// user confirmed replacement; replacement is authorized only when the
	// destination still matches the inspected state at the last safe
	// point.
	IntentReplace
)

// InspectDestination classifies the resolved destination through the
// injected boundary and captures a durable state token. Detection performs
// no truncation, removal, rename, or write, and never opens or serializes
// anything: callers use the result to route a new path to an unconfirmed
// no-replace write and an existing path into exactly one overwrite
// confirmation whose confirmation records IntentReplace tied to the
// returned state.
func InspectDestination(fs SaveFS, path string) (DestinationState, error) {
	id, err := fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DestinationState{Status: DestinationNew}, nil
		}
		return DestinationState{}, err
	}
	return DestinationState{Status: DestinationExisting, Identity: id}, nil
}

// TempPattern returns the destination-local, non-colliding temporary-file
// pattern for base: a hidden dotfile of the target name inside the
// destination directory, so it can never collide with or expose a partial
// target through its final name.
func TempPattern(base string) string {
	return "." + base + ".sqloid-*"
}

// WriteAtomic performs the atomic output of exactly the supplied immutable
// bytes at path under the explicit overwrite intent tied to the inspected
// state. For IntentNoReplace the destination is published through an
// atomic exclusive creation (O_CREATE|O_EXCL on Linux/macOS); if the
// destination appeared after inspection the raced file is preserved
// byte-for-byte and a DestinationExistsError is returned. For
// IntentReplace a uniquely named temporary file is created in the
// destination directory, receives the captured bytes exactly once, is
// synced and closed, and the destination state is re-verified before the
// sole rename; if the state changed a DestinationChangedError is returned
// and the changed file is preserved. Every pre-publish failure closes what
// was opened, removes every staging artifact, and returns a typed
// StageError without touching an existing destination. Issue #63: a
// nil-error short write is converted to io.ErrShortWrite before
// constructing the StageError. Success is reported only after the
// exclusive creation closes (no-replace) or the rename succeeds
// (replace).
func WriteAtomic(fs SaveFS, path string, data []byte, state DestinationState, intent SaveIntent) error {
	switch intent {
	case IntentNoReplace:
		return writeAtomicNoReplace(fs, path, data)
	case IntentReplace:
		return writeAtomicReplace(fs, path, data, state)
	default:
		return fmt.Errorf("save intent %d is invalid", intent)
	}
}

// writeAtomicNoReplace publishes the captured bytes through an atomic
// exclusive creation. The destination was inspected as missing; if another
// process created it before persistence, CreateExclusive fails and the
// raced file is preserved byte-for-byte. On success the destination is
// created at path directly (no temp file, no rename): the exclusive
// creation is the atomic publish boundary. A pre-close failure removes the
// partially written destination (which the boundary owns exclusively) and
// returns a typed StageError.
func writeAtomicNoReplace(fs SaveFS, path string, data []byte) error {
	f, err := fs.CreateExclusive(path)
	if err != nil {
		if os.IsExist(err) {
			return &DestinationExistsError{Path: path}
		}
		return &StageError{Stage: StageCreate, Err: err}
	}
	cleanup := func() error {
		// Best-effort close for stages that leave the file open; the close
		// stage itself never closes twice.
		_ = f.Close()
		return fs.Remove(path)
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		// Issue #63: a nil error with a short byte count is a failed write,
		// not a silent partial save. Convert it to io.ErrShortWrite so the
		// cause is actionable through errors.Is and the stage-qualified text
		// names the short write rather than <nil>. A non-nil writer error
		// stays the cause unchanged.
		cause := err
		if cause == nil {
			cause = io.ErrShortWrite
		}
		_ = cleanup()
		return &StageError{Stage: StageWrite, Err: cause}
	}
	if err := f.Sync(); err != nil {
		_ = cleanup()
		return &StageError{Stage: StageSync, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = fs.Remove(path)
		return &StageError{Stage: StageClose, Err: err}
	}
	return nil
}

// writeAtomicReplace publishes the captured bytes through a
// destination-local temporary file plus atomic rename, authorized only
// when the destination still matches the inspected state at the last safe
// point (after staging, before rename). If the destination changed,
// disappeared, or was replaced after inspection, the staged temp is
// removed and a DestinationChangedError is returned without replacement.
func writeAtomicReplace(fs SaveFS, path string, data []byte, state DestinationState) error {
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
		// Issue #63: a nil error with a short byte count is a failed write,
		// not a silent partial save. Convert it to io.ErrShortWrite so the
		// cause is actionable through errors.Is and the stage-qualified text
		// names the short write rather than <nil>. A non-nil writer error
		// stays the cause unchanged.
		cause := err
		if cause == nil {
			cause = io.ErrShortWrite
		}
		_ = cleanup()
		return &StageError{Stage: StageWrite, Err: cause}
	}
	if err := f.Sync(); err != nil {
		_ = cleanup()
		return &StageError{Stage: StageSync, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageClose, Err: err}
	}
	// Re-verify the destination still matches the inspected state at the
	// last safe point before the sole replacement boundary. A changed,
	// removed, or replaced destination refuses replacement and preserves
	// the external bytes.
	current, err := fs.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = fs.Remove(tmpName)
			return &DestinationChangedError{Path: path, Missing: true}
		}
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageRename, Err: err}
	}
	if !current.Equal(state.Identity) {
		_ = fs.Remove(tmpName)
		return &DestinationChangedError{Path: path}
	}
	if err := fs.Rename(tmpName, path); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageRename, Err: err}
	}
	return nil
}
