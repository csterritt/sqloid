# Issue #064 Code Walkthrough: Race-Safe Overwrite Intent at the Atomic Persistence Boundary

*2026-09-01T17:03:19Z by Showboat 0.6.1*
<!-- showboat-id: 907f5ac3-eecb-42fe-a2fa-09da365bd469 -->

Issue #64 (Notes/tasks/064-race-safe-save-overwrite-intent.md, Notes/PRD-sqloid.md §Atomic saves, §Export Module Design, §Testing Decisions; user stories 72 and 86) closes the inspection-to-rename race in the atomic save boundary. Before this issue, `WriteAtomic` in `internal/export/save_write.go` accepted only a path and payload: inspection found the destination missing or existing, the user confirmed overwrite, and a destination-local temp file was renamed over the target. Between inspection and rename, another process could create the missing destination (silently overwritten by the rename) or replace/remove the confirmed destination (silently clobbered). Issue #64 carries explicit overwrite intent (`SaveIntent`) plus the inspected destination state token (`DestinationState` with `DestinationIdentity`) into `WriteAtomic`. An unconfirmed new-file save publishes through atomic exclusive creation (`O_CREATE|O_EXCL` on Linux/macOS) — if the destination appeared after inspection, `WriteAtomic` returns a typed `DestinationExistsError` preserving the raced file. A confirmed replacement re-verifies the destination state at the last safe point (after staging, before the sole rename) — if the state changed, disappeared, or was replaced, `WriteAtomic` returns a typed `DestinationChangedError` preserving the changed file and cleaning the temp. Only unchanged confirmed state proceeds to the atomic rename. Issue #53 stage attribution and Issue #63 nil-error short-write conversion (`io.ErrShortWrite`) are retained on both paths. The UI save flow distinguishes race failures from ordinary stage failures: race errors preserve the immutable capture, clear running state, and issue a fresh inspection with a new attempt identity so the user sees the latest state and must renew confirmation before any replacement; ordinary stage failures stay on the existing inline retry/cancel path. This walkthrough deterministically injects both races through the Issue #53/#64 fake filesystem and displays the typed race errors, external-byte preservation, no-replace exclusive creation, pre-rename state re-verification, temp cleanup, fresh inspection, renewed confirmation, stale-message rejection, retry, cancellation, quit restoration, and Issue #63 short-write behavior. It contrasts an ordinary stage failure (same-capture retry) with a race failure (fresh inspection), then shows successful save after unchanged fresh confirmation. Reference Issues #53, #63, and #64 and `Notes/PRD-sqloid.md`.

```bash
sed -n '175,210p' internal/export/save_write.go
```

```output
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
```

```bash
sed -n '210,260p' internal/export/save_write.go
```

```output
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
```

The `WriteAtomic` signature now requires `state DestinationState` and `intent SaveIntent`. The no-replace path calls `CreateExclusive(path)` — `O_CREATE|O_EXCL` on Linux/macOS — which fails with `EEXIST` if the destination appeared after inspection, returning `DestinationExistsError`. The replace path stages a destination-local temp, writes/syncs/closes, then calls `Stat(path)` to re-verify the destination identity at the last safe point. If the identity changed or the destination disappeared, it returns `DestinationChangedError` (with `Missing=true`) and removes the temp — no rename, no replacement. Only when `current.Equal(state.Identity)` does the sole rename proceed. Issue #63 short-write conversion (`io.ErrShortWrite` for `(n < len, nil)`) is retained on both paths.

```bash
sed -n '95,130p' internal/export/save_write.go
```

```output
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
```

```bash
sed -n '130,170p' internal/export/save_write.go
```

```output
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
```

```bash
cat internal/export/save_write_unix.go
```

```output
//go:build unix

// Linux/macOS SaveFS implementation for Issue #53/#64. Stat returns a
// durable identity with device and inode so a same-named replacement
// cannot be mistaken for the inspected file. CreateExclusive uses
// O_CREATE|O_EXCL for the atomic no-replace creation path. TempFile,
// Rename, and Remove use the standard os operations.

package export

import (
	"os"
	"syscall"
)

// OSSaveFS is the real SaveFS over the operating system's filesystem.
type OSSaveFS struct{}

// Stat implements SaveFS via os.Stat, returning a DestinationIdentity with
// size, modification time, device, and inode. A missing path returns an
// error satisfying os.IsNotExist.
func (OSSaveFS) Stat(path string) (DestinationIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DestinationIdentity{}, err
	}
	id := DestinationIdentity{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st != nil {
		id.Ino = uint64(st.Ino)
		id.Dev = uint64(st.Dev)
	}
	return id, nil
}

// CreateExclusive implements SaveFS via os.OpenFile with O_CREATE|O_EXCL:
// the call fails with EEXIST if path already exists.
func (OSSaveFS) CreateExclusive(path string) (SaveFile, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
}

// TempFile implements SaveFS via os.CreateTemp inside the given directory.
func (OSSaveFS) TempFile(dir, pattern string) (SaveFile, error) {
	return os.CreateTemp(dir, pattern)
}

// Name implements SaveFS for os.File-backed save artifacts.
func (OSSaveFS) Name(f SaveFile) string {
	return f.(*os.File).Name()
}

// Rename implements SaveFS via os.Rename.
func (OSSaveFS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// Remove implements SaveFS via os.Remove.
func (OSSaveFS) Remove(path string) error { return os.Remove(path) }
```

The `SaveFS` interface replaces `Exists` with `Stat` (returning `DestinationIdentity` or `os.ErrNotExist`) and adds `CreateExclusive` for the no-replace path. The Unix `OSSaveFS` implements `Stat` via `os.Stat` with `syscall.Stat_t` for device/inode identity, and `CreateExclusive` via `os.OpenFile` with `O_CREATE|O_EXCL`. The non-Unix fallback (`save_write_other.go`) provides size/mtime-only identity and the same `O_CREATE|O_EXCL` exclusive creation.

```bash
sed -n '200,245p' internal/ui/save_write.go
```

```output
		return SaveCompletedMsg{Attempt: attempt}
	}
	return m, cmd
}

// applySaveFailure handles a failed atomic write. Ordinary stage failures
// (StageError) stay inline with the captured copy retained for retry.
// Issue #64 race failures — DestinationExistsError (unconfirmed save raced
// by external creation) and DestinationChangedError (confirmed replacement
// raced by external change) — preserve the immutable capture and issue a
// fresh destination inspection with a new attempt identity so the user
// sees the latest state and must renew confirmation before any replacement
// rather than blindly retrying the stale authorization.
func (m Model) applySaveFailure(msg SaveFailedMsg) (tea.Model, tea.Cmd) {
	if !m.saveRunning || msg.Attempt != m.saveAttempt {
		return m, nil
	}
	var existsErr *export.DestinationExistsError
	var changedErr *export.DestinationChangedError
	if errors.As(msg.Err, &existsErr) || errors.As(msg.Err, &changedErr) {
		// Race recovery: preserve the immutable capture and re-inspect the
		// destination with a fresh attempt identity. The capture's path,
		// format, payload, selection, and warnings are untouched; only the
		// state token will be updated when the fresh inspection arrives.
		m.saveRunning = true
		m.saveFailure = ""
		m.saveFailurePath = ""
		m.overwriteOpen = false
		m.saveAttempt++
		attempt := m.saveAttempt
		path := m.saveCapture.path
		fs := m.saveFS()
		return m, func() tea.Msg {
			state, err := export.InspectDestination(fs, path)
			return SaveInspectMsg{Path: path, Attempt: attempt, State: state, Err: err}
		}
	}
	// Ordinary stage failure: inline retry/cancel with the captured copy
	// retained.
	m.saveRunning = false
	m.saveFailure = msg.Err.Error()
	return m, nil
}

// handleOverwriteConfirmKey consumes every key while the overwrite
// confirmation is open above the intact picker: Enter/y confirms exactly
```

The UI `applySaveFailure` distinguishes race failures from ordinary stage failures via `errors.As`. Race errors (`DestinationExistsError` or `DestinationChangedError`) trigger race recovery: the immutable capture is preserved, running state is cleared, the overwrite confirmation is closed, and a fresh `InspectDestination` is issued with a new attempt identity. The fresh inspection opens a new overwrite confirmation for the latest state — the user must renew confirmation before any replacement. Ordinary `StageError` failures stay on the existing inline retry/cancel path with `saveFailure` set. Attempt identities remain monotonic so stale `SaveFailedMsg` or `SaveInspectMsg` from the pre-race attempt are inert.

```bash
go test ./internal/export/ -run 'TestWriteAtomicNoReplaceRacedByExternalCreation|TestWriteAtomicReplaceRacedByExternalChange|TestWriteAtomicReplaceRacedByExternalRemoval|TestWriteAtomicReplaceUnchangedStateSucceeds' -v
```

```output
=== RUN   TestWriteAtomicNoReplaceRacedByExternalCreation
--- PASS: TestWriteAtomicNoReplaceRacedByExternalCreation (0.00s)
=== RUN   TestWriteAtomicReplaceRacedByExternalChange
--- PASS: TestWriteAtomicReplaceRacedByExternalChange (0.00s)
=== RUN   TestWriteAtomicReplaceRacedByExternalRemoval
--- PASS: TestWriteAtomicReplaceRacedByExternalRemoval (0.00s)
=== RUN   TestWriteAtomicReplaceUnchangedStateSucceeds
--- PASS: TestWriteAtomicReplaceUnchangedStateSucceeds (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/export	(cached)
```

```bash
go test ./internal/export/ -run 'TestOSSaveFS' -v
```

```output
=== RUN   TestOSSaveFSNoReplaceRacedByExternalCreation
--- PASS: TestOSSaveFSNoReplaceRacedByExternalCreation (0.00s)
=== RUN   TestOSSaveFSNoReplaceSuccess
--- PASS: TestOSSaveFSNoReplaceSuccess (0.00s)
=== RUN   TestOSSaveFSReplaceRacedByExternalReplacement
--- PASS: TestOSSaveFSReplaceRacedByExternalReplacement (0.00s)
=== RUN   TestOSSaveFSReplaceRacedByExternalRemoval
--- PASS: TestOSSaveFSReplaceRacedByExternalRemoval (0.00s)
=== RUN   TestOSSaveFSReplaceUnchangedStateSucceeds
--- PASS: TestOSSaveFSReplaceUnchangedStateSucceeds (0.00s)
=== RUN   TestOSSaveFSNoReplaceAndReplaceStagingInDestinationDirectory
--- PASS: TestOSSaveFSNoReplaceAndReplaceStagingInDestinationDirectory (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/export	(cached)
```

```bash
go test ./internal/ui/ -run 'TestNewFileSaveRaced|TestConfirmedReplaceSaveRaced|TestRaceRecovery|TestOrdinaryStageFailureStays|TestRaceRecoverySuccessfulSave' -v
```

```output
=== RUN   TestNewFileSaveRacedByExternalCreationRoutesToRenewedConfirmation
--- PASS: TestNewFileSaveRacedByExternalCreationRoutesToRenewedConfirmation (0.00s)
=== RUN   TestConfirmedReplaceSaveRacedByExternalChangeRoutesToRenewedConfirmation
--- PASS: TestConfirmedReplaceSaveRacedByExternalChangeRoutesToRenewedConfirmation (0.00s)
=== RUN   TestRaceRecoveryStaleMessagesAreInert
--- PASS: TestRaceRecoveryStaleMessagesAreInert (0.00s)
=== RUN   TestRaceRecoveryEscCancelRestoresIntactPicker
--- PASS: TestRaceRecoveryEscCancelRestoresIntactPicker (0.00s)
=== RUN   TestRaceRecoveryQuitSuspendRestore
--- PASS: TestRaceRecoveryQuitSuspendRestore (0.00s)
=== RUN   TestOrdinaryStageFailureStaysOnSameCaptureRetryPath
--- PASS: TestOrdinaryStageFailureStaysOnSameCaptureRetryPath (0.00s)
=== RUN   TestRaceRecoverySuccessfulSaveAfterUnchangedFreshConfirmation
--- PASS: TestRaceRecoverySuccessfulSaveAfterUnchangedFreshConfirmation (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

```bash
go test ./internal/export/ -run 'TestWriteAtomicShortWriteNilErrorIsErrShortWrite' -v
```

```output
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-replace-existing
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-no-replace-missing
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/complete-write-replace-control
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/non-nil-write-error-replace-control
--- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-replace-existing (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-no-replace-missing (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/complete-write-replace-control (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/non-nil-write-error-replace-control (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/export	(cached)
```

```bash
go vet ./... && go test ./... && go build ./...
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

All export and UI tests pass, including the real-filesystem `OSSaveFS` race tests on Linux/macOS (`save_write_unix_test.go`) with deterministic barriers and no sleeps. Issue #63 short-write behavior (`io.ErrShortWrite` for `(n < len, nil)`) is retained on both the no-replace and replace paths. After Issue #57, representative cases should be driven through the shipped TUI/headless composition path (`internal/session`). The race-safe boundary preserves destination-local staging, atomicity where replacement is authorized, cleanup, retry, cancellation, quit restoration, and exact opener restoration — the only change is that raced destinations are now refused with typed errors and routed through fresh inspection and renewed confirmation rather than silently overwritten.

