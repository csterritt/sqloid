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
