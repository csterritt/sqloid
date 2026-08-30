// Package connection owns SQLite connection startup for Sqloid: ordered
// pre-open filesystem and header validation of explicit or discovered paths,
// followed by a read-write, non-creating open and a schema probe. It also
// classifies startup failures into the structured classes rendered as exact
// one-line diagnostics by internal/cli, per Issue #2 and the Startup
// validation and errors section of Notes/PRD-sqloid.md.
//
// The opened pool holds exactly two connections (its minimum and maximum) so
// concurrent page/count requests can each Lease a distinct dedicated physical
// connection, per Issue #5 and the Connection pool, limits, and busy handling
// decision in Notes/PRD-sqloid.md: every physical connection carries a
// five-second busy timeout set through the DSN, every lease is configured
// with an exact 64 MiB connection-local SQLITE_LIMIT_LENGTH, and journal mode
// is never set or changed on any connection.
package connection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"

	sqlite "modernc.org/sqlite" // pinned pure-Go "sqlite" driver
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	// poolSize pins the database/sql pool to exactly two connections, its
	// minimum and maximum alike, so that two concurrent page/count requests
	// can each lease a distinct physical connection without serializing or
	// growing beyond the PRD's exact-two contract.
	poolSize = 2

	// busyTimeoutMillis configures every physical connection's five-second
	// SQLite busy timeout through the supported modernc DSN parameter, so
	// lock waits settle no later than five seconds as the PRD requires.
	busyTimeoutMillis = 5000

	// sqlMaxLengthBytes is the exact 64 MiB connection-local SQLITE_LIMIT_LENGTH
	// applied to every leased connection before it performs any work. Length
	// limits cannot be set through SQL; sqlite.Limit on a *sql.Conn lease is
	// the documented mechanism for this configuration.
	sqlMaxLengthBytes = 64 << 20
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

	// path, startDev, and startIno are the recorded request-boundary health
	// reference (Issue #7): the validated target path plus the device and
	// inode it carried at successful startup. VerifyHealth compares stat of
	// path against them before every database request. On platforms without
	// device/inode support both identifiers remain zero.
	path     string
	startDev uint64
	startIno uint64

	// beforeFirstPage and beforeCount are test-only barrier seams for the
	// Issue #24 concurrency capability suite: when non-nil, each is invoked
	// inside the named request's operation after its lease is acquired and
	// before its statement runs, so tests can hold both concurrent requests
	// simultaneously and capture physical-connection identity. Production
	// control flow leaves both nil and never reads them elsewhere.
	beforeFirstPage, beforeCount func(ctx context.Context, conn *sql.Conn)

	// beforeWriteBegin, beforeWriteExec, beforeWriteCommit, and
	// beforeWriteRollback are test-only barrier seams for the Issue #42
	// transactional-write suite: when non-nil, each is invoked inside one
	// StartWrite request after its phase message is emitted and before the
	// next transaction step runs, so tests can hold every transition with
	// channel barriers and request cancellation at a deterministic point.
	// Production control flow leaves all four nil and never reads them.
	beforeWriteBegin, beforeWriteExec, beforeWriteCommit, beforeWriteRollback func(ctx context.Context, conn *sql.Conn)

	// identityChecks counts VerifyHealth invocations so tests can prove the
	// exactly-one-pre-BEGIN-check contract for phased writes without hooks;
	// reading it is test observability of the boundary, not production state.
	identityChecks atomic.Int64
}

// Lease acquires one dedicated physical connection from the exact-two pool,
// applies this package's required per-connection configuration to it before
// any query runs, and returns ownership of that connection to the caller.
// Concurrent callers receive distinct connections; a third concurrent Lease
// blocks until one of the two leases is released. Close of db releases the
// whole pool regardless of outstanding leases.
type Lease struct {
	conn *sql.Conn

	// interruptFn, when set, dispatches a connection-scoped interrupt on
	// this lease's physical connection without touching any other pooled
	// connection. Production leases leave it nil: cancelling the request
	// context is itself the connection-scoped interrupt on the pinned
	// driver. Tests install fake hooks to observe or replace the dispatch.
	interruptFn func()
}

// Conn returns the leased underlying connection for executing requests. It
// panics if called after Release: reusing a released lease would silently run
// against a different pooled connection and break caller-owned assumptions;
// callers must keep their work on the lease until settled and then release it.
func (l *Lease) Conn() *sql.Conn {
	if l.conn == nil {
		panic("connection: use of released lease")
	}
	return l.conn
}

// Release returns the leased connection to the pool. Release is safe to call
// more than once and after errors; subsequent Conn calls on the same lease
// panic rather than observing a connection owned elsewhere.
func (l *Lease) Release(ctx context.Context) error {
	if l.conn == nil {
		return nil
	}
	err := l.conn.Close()
	l.conn = nil
	return err
}

// Close closes the underlying pool.
func (db *DB) Close() error { return db.SQL.Close() }

// Lease checks out one dedicated connection from the exact-two pool and
// configures it with the connection-local length limit before handing it
// over. The busy timeout was already established by dsn when the physical
// connection was created; the limit has no DSN equivalent and must be applied
// per *sql.Conn via sqlite.Limit, the driver's documented mechanism.
func (db *DB) Lease(ctx context.Context) (*Lease, error) {
	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lease connection from pool: %w", err)
	}
	// Request-boundary health check (Issue #7): no physical connection — a
	// retained pooled member or one newly opened/reopened for this lease — is
	// admitted for use until the original path still carries its startup
	// device and inode. This check is the pre-request check itself: work can
	// only follow after Lease returns, so every request begins verified.
	if err := db.VerifyHealth(); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := sqlite.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, sqlMaxLengthBytes); err != nil {
		conn.Close()
		return nil, fmt.Errorf("configure leased connection length limit: %w", err)
	}
	return &Lease{conn: conn}, nil
}

// dsn builds the modernc.org/sqlite data source name for path: URI form so
// that mode=rw forbids creating a missing database, with the path percent-
// encoded so reserved characters such as '?' or '#' stay part of the filename.
func dsn(path string) string {
	u := mustFileURL(path)
	q := u.Query()
	q.Set("mode", "rw")
	// Applies to every physical connection the pool creates: busy handling is
	// per connection, so the five-second bound travels with the DSN itself.
	q.Set("_busy_timeout", strconv.Itoa(busyTimeoutMillis))
	u.RawQuery = q.Encode()
	return u.String()
}

// mustFileURL renders path as a file: URI whose query string can be extended;
// it only panics for inputs that cannot occur from normal callers.
//
// Relative paths render as opaque "file:<path>" URIs: url.URL.String() would
// otherwise promote the first path segment to a URI authority ("file://.
// wrangler/..."), which the SQLite URI parser rejects with "invalid uri
// authority" while diagnostics keep referring to the caller's relative path.
func mustFileURL(path string) url.URL {
	if filepath.IsAbs(path) {
		return url.URL{Scheme: "file", Path: path}
	}
	return url.URL{Scheme: "file", Opaque: path}
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
	closeOnError := func(err error) (*DB, error) {
		sqlDB.Close()
		return nil, err
	}

	// The exact-two pool: minimum and maximum are both enforced here, two
	// being enough for the two concurrent page/count leases Issue #5 needs.
	sqlDB.SetMaxOpenConns(poolSize)
	sqlDB.SetMaxIdleConns(poolSize)
	// The first real contact happens here, so surface it immediately instead
	// of lazily on first use; failure is classified as above.
	if err := probe(sqlDB); err != nil {
		return closeOnError(classifyOpenError(path, err))
	}
	// Record the validated target's filesystem identity after the full
	// ordered validation succeeded; this reference backs every later
	// request-boundary verification per Issue #7.
	startDev, startIno, statErr := statIdentity(path)
	if statErr != nil {
		// The file vanished or became unstatable in the instant between
		// validation and recording; classify the same way as open failures.
		return closeOnError(classifyOpenError(path, statErr))
	}
	return &DB{SQL: sqlDB, path: path, startDev: startDev, startIno: startIno}, nil
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
