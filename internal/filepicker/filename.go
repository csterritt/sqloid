// Filename validation and format-aware completion (Issue #52), per the File
// picker decision in Notes/PRD-sqloid.md. Validation is a pure basename
// check: empty basenames and basenames containing `/` or NUL are rejected
// before any destination is constructed or any filesystem effect issued.
// Completion appends the opener's required extension exactly once — only
// when that exact suffix is missing — preserving every otherwise-valid
// literal byte of the entered text, including dots, spaces, leading dots,
// multiple extensions, and mixed case. The picker owns no serializer and no
// persistence; joining the completed basename to the selected directory is a
// pure path operation performed only at submission.
package filepicker

import (
	"errors"
	"strings"
)

// Filename validation failures. Callers show these inline in the same picker;
// none of them ever reaches the filesystem or a serializer.
var (
	// ErrEmptyFilename rejects the empty basename.
	ErrEmptyFilename = errors.New("filename is empty")
	// ErrFilenameSlash rejects any basename containing a path separator.
	ErrFilenameSlash = errors.New("filename may not contain '/'")
	// ErrFilenameNUL rejects any basename containing a NUL byte.
	ErrFilenameNUL = errors.New("filename may not contain NUL")
)

// ValidateFilename checks one entered basename without touching the
// filesystem. It returns the matching sentinel for an empty basename, any
// basename containing `/`, or any basename containing NUL; nil otherwise.
func ValidateFilename(name string) error {
	if name == "" {
		return ErrEmptyFilename
	}
	if strings.Contains(name, "/") {
		return ErrFilenameSlash
	}
	if strings.ContainsRune(name, 0) {
		return ErrFilenameNUL
	}
	return nil
}

// CompleteName returns the basename with the format's required extension
// appended exactly once: an already-complete name is returned verbatim, and
// all otherwise-valid text — including dots, spaces, leading dots, multiple
// extensions, mixed case, and non-ASCII bytes — is never rewritten.
func CompleteName(name string, format Format) string {
	ext := format.Extension()
	if strings.HasSuffix(name, ext) {
		return name
	}
	return name + ext
}

// JoinDestination joins the selected directory with the validated, completed
// basename into the exact destination path. It is pure: no filesystem access
// occurs here; verification belongs to the caller's issued request.
func JoinDestination(dir, name string, format Format) string {
	return dir + "/" + CompleteName(name, format)
}
