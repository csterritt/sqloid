//go:build !unix

// Fallback SaveFS for platforms without Unix syscall.Stat_t device/inode
// semantics. Sqloid's race-safe save contract targets Linux and macOS; on
// other platforms Stat returns a size-and-mtime-only identity (weaker
// race detection) and CreateExclusive uses os.OpenFile with O_CREATE|O_EXCL
// so the no-replace path still works. Sessions keep working without the
// Linux/macOS inode checks.

package export

import (
	"os"
)

// OSSaveFS is the real SaveFS over the operating system's filesystem.
type OSSaveFS struct{}

// Stat implements SaveFS via os.Stat, returning a DestinationIdentity with
// size and modification time only (no device/inode on non-Unix platforms).
func (OSSaveFS) Stat(path string) (DestinationIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DestinationIdentity{}, err
	}
	return DestinationIdentity{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

// CreateExclusive implements SaveFS via os.OpenFile with O_CREATE|O_EXCL.
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
