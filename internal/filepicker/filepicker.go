// Package filepicker implements the UI-independent save/export destination
// picker model (Issue #52), per the File picker decision in
// Notes/PRD-sqloid.md. The model starts navigation at a caller-supplied
// process working directory, lists only navigable child directories — visible
// and hidden alike — with the navigable parent `..` always first, and orders
// child basenames by ascending case-sensitive bytewise Go string comparison:
// no locale collation and no natural numeric order. Directory creation is
// unsupported and regular files never enter the directory list.
//
// Filename text is state fully separate from directory selection: an empty
// basename or one containing `/` or NUL is invalid, and otherwise the format's
// exact `.sql`/`.csv`/`.json` suffix is appended only when missing. The model
// performs no serialization and owns no persistence: every filesystem access
// goes through the injected FS boundary, directory reads and destination
// verification run as caller-issued requests outside any render or state
// mutation, and write/overwrite behavior belongs to a later issue.
package filepicker

import (
	"errors"
	"io/fs"
	"os"
	"sort"
)

// DirEntry is the minimal directory-entry view the model needs. The real
// filesystem boundary is satisfied by os.DirEntry; fakes implement it too.
type DirEntry interface {
	// Name is the entry's base name, including a leading dot for hidden
	// entries.
	Name() string
	// IsDir reports whether the entry names a navigable directory.
	IsDir() bool
}

// FS is the injected filesystem boundary: the only way the model observes or
// touches the filesystem. ReadDir lists one directory; failures are typed by
// callers through Apply as inline picker errors. No other operation exists, so
// the model cannot create, rename, or delete anything.
type FS interface {
	// ReadDir returns the entries of the directory at path.
	ReadDir(path string) ([]DirEntry, error)
}

// OSFS is the real-filesystem FS implementation backed by os.ReadDir. Its
// entries are os.DirEntry values.
type OSFS struct{}

// ReadDir lists the directory at path through the operating system.
func (OSFS) ReadDir(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}

// Format is the closed save-format value an opener supplies for its flow.
type Format string

// The three supported save formats with their exact required extensions.
const (
	FormatSQL  Format = "sql"
	FormatCSV  Format = "csv"
	FormatJSON Format = "json"
)

// Extension returns the exact required extension for the format, lowercase
// with a leading dot. Unknown formats render their raw value.
func (f Format) Extension() string {
	return "." + string(f)
}

// ErrorKind classifies one typed inline picker error so openers can show
// exact feedback without matching error text.
type ErrorKind int

const (
	// KindRead is a directory listing failure during navigation, including
	// permission-denied starts.
	KindRead ErrorKind = iota + 1
	// KindVerify is a destination verification failure at submission: the
	// destination directory could not be confirmed navigable at save time.
	KindVerify
	// KindValidate is an inline filename validation failure that never
	// reaches the filesystem.
	KindValidate
)

// Error is one typed inline picker error. It wraps the underlying cause with
// %w-style inspection (errors.Is/As) while carrying the exact path and kind
// the opener needs to keep the picker inline for retry.
type Error struct {
	Kind ErrorKind
	Path string
	Err  error
}

// Error renders the lower-case inline feedback line shown in the picker.
func (e *Error) Error() string {
	switch e.Kind {
	case KindRead:
		return "could not open directory: " + e.Err.Error()
	case KindVerify:
		return "could not verify destination: " + e.Err.Error()
	case KindValidate:
		return e.Err.Error()
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying cause for errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Err }

// IsPermission reports whether the error's cause is a permission-denied
// filesystem failure, independent of any driver- or OS-specific text.
func IsPermission(err error) bool {
	return errors.Is(err, fs.ErrPermission)
}

// SortChildDirs orders child directory basenames by ascending case-sensitive
// bytewise Go string comparison. This is deliberately neither locale-aware
// collation nor natural numeric ordering: "B" sorts before "a", "d10" before
// "d2", and byte sequences of valid UTF-8 order by their bytes alone.
func SortChildDirs(dirs []string) {
	sort.Strings(dirs)
}
