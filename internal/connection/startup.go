// Package connection owns SQLite connection startup for Sqloid: ordered
// pre-open filesystem and header validation of explicit or discovered paths,
// followed by a read-write, non-creating open and a schema probe. It also
// classifies startup failures into the structured classes rendered as exact
// one-line diagnostics by internal/cli, per Issue #2 and the Startup
// validation and errors section of Notes/PRD-sqloid.md.
package connection

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"syscall"

	_ "modernc.org/sqlite" // registers the pinned pure-Go "sqlite" driver
)

// sqliteHeader is the exact 16-byte header required at offset 0 of every
// SQLite database file.
const sqliteHeader = "SQLite format 3\x00"

// FailureKind identifies one structured startup-failure class. The zero value
// is never used: every failure receives an explicit kind.
type FailureKind int

const (
	// FailureMissing means the target path does not exist; no file was ever
	// created for it.
	FailureMissing FailureKind = iota + 1
	// FailureUnreadable means the target exists but cannot be opened for
	// reading by this process (typically EACCES/EPERM).
	FailureUnreadable
	// FailureNotADatabase means the target is a directory or lacks the exact
	// 16-byte SQLite header at offset 0.
	FailureNotADatabase
	// FailureReadWrite means opening an existing, valid database without
	// create permission failed (EACCES/EPERM, EROFS, or another OS/driver
	// cause preserved in Cause).
	FailureReadWrite
)

// String renders the human-facing name of the failure class used in tests and
// diagnostics.
func (k FailureKind) String() string {
	switch k {
	case FailureMissing:
		return "missing"
	case FailureUnreadable:
		return "unreadable"
	case FailureNotADatabase:
		return "not-a-database"
	case FailureReadWrite:
		return "read-write-open"
	default:
		return fmt.Sprintf("FailureKind(%d)", int(k))
	}
}

// StartupError is the structured result of failed startup validation or
// opening. Error() already produces the exact one-line diagnostic mandated by
// the PRD, so internal/cli prints it verbatim to stderr with exit status 1.
type StartupError struct {
	Path  string      // absolute or relative target named in diagnostics
	Kind  FailureKind // structured class for programmatic handling
	Cause error       // underlying OS/driver error, if any; unwrappable
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *StartupError) Unwrap() error { return e.Cause }

// Error returns the exact one-line diagnostic for this failure, following the
// PRD message contract: read-write open failures use the documented prefix,
// invalid headers and directories say "not a SQLite database", and existence
// and readability failures are classified before any open occurs.
func (e *StartupError) Error() string {
	switch e.Kind {
	case FailureMissing:
		return fmt.Sprintf("%s: no such file or directory", e.Path)
	case FailureUnreadable:
		return fmt.Sprintf("%s: permission denied", e.Path)
	case FailureNotADatabase:
		return fmt.Sprintf("%s: not a SQLite database", e.Path)
	case FailureReadWrite:
		detail := readWriteDetail(e.Cause)
		if detail == "" {
			detail = "cannot open database read-write"
		}
		return fmt.Sprintf("cannot open database read-write: %s: %s", e.Path, detail)
	default:
		return fmt.Sprintf("%s: unknown startup failure (%d)", e.Path, int(e.Kind))
	}
}

// readWriteDetail maps a mode=rw open failure onto its documented message
// fragment. Permission causes are classified through wrapped errno values so
// EACCES and EPERM both render as "permission denied", EROFS as "read-only
// file system"; anything else keeps its raw OS/driver cause text verbatim.
func readWriteDetail(cause error) string {
	var errno syscall.Errno
	if errors.As(cause, &errno) {
		switch errno {
		case syscall.EACCES, syscall.EPERM:
			return "permission denied"
		case syscall.EROFS:
			return "read-only file system"
		}
		return errno.Error()
	}
	if cause != nil {
		return cause.Error()
	}
	return ""
}

// DB is a successfully started Sqloid session handle wrapping the pooled
// database/sql access for a validated database. Close releases the pool.
type DB struct {
	SQL *sql.DB
}

// Close closes the underlying pool.
func (db *DB) Close() error { return db.SQL.Close() }

// dsn builds the modernc.org/sqlite data source name for path: URI form so
// that mode=rw forbids creating a missing database, with the path percent-
// encoded so reserved characters such as '?' or '#' stay part of the filename.
func dsn(path string) string {
	u := mustFileURL(path)
	q := u.Query()
	q.Set("mode", "rw")
	u.RawQuery = q.Encode()
	return u.String()
}

// mustFileURL renders path as a file: URI whose query string can be extended;
// it only panics for inputs that cannot occur from normal callers.
func mustFileURL(path string) url.URL {
	return url.URL{Scheme: "file", Path: path}
}

// Open validates the database at path without creating or modifying it and,
// when valid, opens it read-write through the pinned driver and probes the
// schema with a harmless `PRAGMA schema_version`. Journal mode is never set
// or changed, and there is deliberately no read-only fallback.
//
// Validation runs in the mandated order — existence → readability → exact
// 16-byte SQLite header → read-write open → probe — and fails fast at the
// first failing step with a *StartupError whose Error() is the exact one-line
// diagnostic required by Issue #2.
func Open(path string) (*DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, &StartupError{Path: path, Kind: FailureMissing, Cause: err}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &StartupError{Path: path, Kind: FailureUnreadable, Cause: err}
	}
	defer f.Close()

	if info.IsDir() {
		return nil, &StartupError{Path: path, Kind: FailureNotADatabase}
	}
	header := make([]byte, len(sqliteHeader))
	n, err := f.Read(header)
	if err != nil || n != len(sqliteHeader) || string(header) != sqliteHeader {
		return nil, &StartupError{Path: path, Kind: FailureNotADatabase}
	}

	// Prove genuine writability before touching the driver: SQLite itself
	// would silently fall back to opening O_RDONLY when O_RDWR fails, which
	// would amount to exactly the silent read-only degradation the PRD
	// forbids. Classifying here keeps EACCES/EPERM and EROFS lossless.
	if err := openReadWrite(path); err != nil {
		return nil, &StartupError{Path: path, Kind: FailureReadWrite, Cause: err}
	}

	sqlDB, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, &StartupError{Path: path, Kind: FailureReadWrite, Cause: err}
	}
	// The first real contact happens here, so surface it immediately instead
	// of lazily on first use; failure is classified as above.
	if err := probe(sqlDB); err != nil {
		sqlDB.Close()
		return nil, classifyOpenError(path, err)
	}
	return &DB{SQL: sqlDB}, nil
}

// probe issues the harmless schema probe required after opening.
func probe(sqlDB *sql.DB) error {
	var version int64
	return sqlDB.QueryRow("PRAGMA schema_version").Scan(&version)
}

// classifyOpenError turns a driver-level mode=rw failure into the documented
// StartupError. Readability was already proven before opening, so any failure
// reaching this point belongs to the read-write class; its cause is kept
// unwrappable so permission-denied, read-only-filesystem, and other raw
// OS/driver details all render through readWriteDetail.
func classifyOpenError(path string, err error) error {
	return &StartupError{Path: path, Kind: FailureReadWrite, Cause: err}
}

// openReadWrite attempts an OS-level O_RDWR open of path without creation,
// returning its *PathError so errno classification stays lossless.
func openReadWrite(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// Session is the CLI-facing sqlite command handler: it starts a session on
// path and keeps it open until the caller's deferred Close point. For now it
// closes when the handler returns because no TUI consumes the handle yet;
// Issue #2 only requires successful startup to be silent.
func Session(path string) error {
	db, err := Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	return nil
}
