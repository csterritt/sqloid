// Request-boundary database identity checks, per Issue #7 and the Session
// health section of Notes/PRD-sqloid.md: at startup Sqloid records the device
// and inode of the successfully validated database path; immediately before
// every database request — and before any newly opened or replacement pooled
// connection is admitted for use — it re-stats that original path and
// compares both identifiers. Absence (including rename-away) and same-path
// replacement produce typed outcomes carrying their underlying causes; the
// terminal wording rendered from these outcomes is owned by Issue #46 and is
// deliberately absent here. Health is strictly request-boundary based: there
// is no watcher, polling loop, or UI dependency anywhere in this package.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// HealthKind identifies one request-boundary database identity failure. The
// zero value means "no identity failure"; every detected failure names a kind.
type HealthKind int

const (
	// HealthDeleted means the original path no longer exists at all,
	// including rename-away: stat of the recorded path failed.
	HealthDeleted HealthKind = iota + 1
	// HealthReplaced means the original path exists but its device or inode
	// differs from the startup reference: a different file now owns the name.
	HealthReplaced
)

// String renders the human-facing classification used in tests and
// diagnostics. It contains no terminal-session copy, which Issue #46 owns.
func (k HealthKind) String() string {
	switch k {
	case HealthDeleted:
		return "deleted"
	case HealthReplaced:
		return "replaced"
	default:
		return fmt.Sprintf("HealthKind(%d)", int(k))
	}
}

// HealthError is the typed result of a failed request-boundary identity check.
// Kind classifies the outcome and Cause preserves the underlying stat error
// so callers can inspect it losslessly with errors.Is/errors.As.
type HealthError struct {
	Path  string     // the original startup path whose identity failed verification
	Kind  HealthKind // deleted or replaced
	Cause error      // underlying stat cause when present; nil for replacements
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *HealthError) Unwrap() error { return e.Cause }

// Error returns neutral diagnostic text naming the path and classification;
// terminal-facing copy is owned by Issue #46 and never lives in this string.
func (e *HealthError) Error() string {
	switch e.Kind {
	case HealthDeleted:
		return fmt.Sprintf("%s: database file absent at request boundary", e.Path)
	case HealthReplaced:
		return fmt.Sprintf("%s: database file device/inode differs from startup", e.Path)
	default:
		return fmt.Sprintf("%s: unknown health failure (%d)", e.Path, int(e.Kind))
	}
}

// VerifyHealth stats the recorded original path and compares both identifiers
// against the startup reference, exactly once per call. It returns nil when
// the file exists unchanged — including after ordinary in-place mutation that
// retains both identifiers, which stays inside normal SQLite behavior — or a
// *HealthError whose kind distinguishes absence from same-path replacement.
// Any stat failure classifies as absence because existence itself cannot be
// confirmed; its cause is preserved unwrappable. On platforms without device/
// inode support, verification trivially passes.
func (db *DB) VerifyHealth() error {
	db.identityChecks.Add(1)
	if !statIdentitySupported() {
		return nil
	}
	dev, ino, err := statIdentity(db.path)
	if err != nil {
		return &HealthError{Path: db.path, Kind: HealthDeleted, Cause: err}
	}
	if dev != db.startDev || ino != db.startIno {
		return &HealthError{Path: db.path, Kind: HealthReplaced}
	}
	return nil
}

// RequestResult is the settled result of RunRequest: the request outcome plus
// the typed identity classification required by the PRD's race rules.
type RequestResult struct {
	Outcome Outcome // terminal outcome of the request lifecycle

	// Err carries the underlying failure on a non-successful outcome: an
	// operation error (or an error acquiring/releasing the lease). It stays
	// nil on success unless release itself failed.
	Err error

	// Health is non-nil only when deletion or replacement was classified —
	// at the pre-request boundary, or after an error when post-error
	// reclassification takes precedence over ordinary handling.
	Health *HealthError
}

// RunRequest executes one complete database request as the reusable boundary
// every caller must route through: identity verification runs before any work
// (inside Lease, which likewise precedes use of any newly opened connection),
// then the operation runs cancellably on a dedicated leased connection, and
// after a failed outcome identity is re-verified before ordinary SQLite error
// handling so a raced deletion or replacement takes precedence. A successful
// result always stands even if the file was replaced after its precheck: the
// next RunRequest boundary detects that change before further work begins. A
// write transaction contained in op is one request, so it receives exactly one
// pre-BEGIN check with none between statements and COMMIT. Cancelling parent
// aborts lease acquisition and reaches op through the derived context.
func (db *DB) RunRequest(parent context.Context, op func(ctx context.Context, conn *sql.Conn) error) RequestResult {
	lease, err := db.Lease(parent)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return RequestResult{Outcome: OutcomeCancelled, Err: err}
		}
		var he *HealthError
		if errors.As(err, &he) {
			return RequestResult{Outcome: OutcomeFailed, Health: he}
		}
		return RequestResult{Outcome: OutcomeFailed, Err: err}
	}

	request := lease.BeginRequest(parent)
	var opErr error
	outcome := request.Run(func(ctx context.Context) error {
		opErr = op(ctx, lease.Conn())
		return opErr
	})
	closeErr := request.Close()

	switch outcome {
	case OutcomeSuccess:
		return RequestResult{Outcome: OutcomeSuccess, Err: closeErr}
	case OutcomeCancelled:
		return RequestResult{Outcome: OutcomeCancelled, Err: closeErr}
	default:
		cause := firstErr(opErr, closeErr)
		var he *HealthError
		if errors.As(db.VerifyHealth(), &he) {
			// A raced deletion or replacement wins the required race: the
			// typed classification takes precedence over ordinary handling,
			// while the request's own cause stays preserved alongside it.
			return RequestResult{Outcome: OutcomeFailed, Err: cause, Health: he}
		}
		return RequestResult{Outcome: OutcomeFailed, Err: cause}
	}
}

// firstErr returns the first non-nil error among candidates.
func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
