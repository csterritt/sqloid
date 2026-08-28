// Typed main-schema catalog refresh lifecycle, per Issue #13 and the Schema
// scope decisions in Notes/PRD-sqloid.md. Every Table-popup open issues one
// refresh attempt through the Connection boundary; exactly one settled
// Attempt comes back. Ordinary failures carry only their cause so consumers
// retain the exact prior Catalog without partial replacement, while the
// deletion/replacement statuses represent the Connection boundary's typed
// health classifications (connection.HealthError kinds) and always take
// terminal precedence over the stale workflow.

package schema

import "fmt"

// RefreshStatus classifies one settled catalog-refresh outcome. The zero
// value is not a settled outcome.
type RefreshStatus int

const (
	// RefreshOK means the refresh succeeded and Attempt.Catalog holds the
	// complete refreshed snapshot to install.
	RefreshOK RefreshStatus = iota + 1
	// RefreshFailed is an ordinary failure (lock, corruption, change race):
	// only Cause is set, and every consumer must retain the prior catalog
	// unchanged behind `could not refresh: <cause>` reporting.
	RefreshFailed
	// RefreshDeleted is terminal: the database file no longer exists at the
	// request boundary (absent or renamed away).
	RefreshDeleted
	// RefreshReplaced is terminal: a different file now owns the startup
	// path (device/inode mismatch).
	RefreshReplaced
)

// String renders the human-facing classification used in tests, diagnostics,
// and composition wiring from Connection health outcomes.
func (s RefreshStatus) String() string {
	switch s {
	case RefreshOK:
		return "ok"
	case RefreshFailed:
		return "failed"
	case RefreshDeleted:
		return "deleted"
	case RefreshReplaced:
		return "replaced"
	default:
		return fmt.Sprintf("RefreshStatus(%d)", int(s))
	}
}

// Attempt is one settled result of a single catalog refresh request. Exactly
// one Status is meaningful, with its payload rule enforced by Valid: success
// carries Catalog, ordinary failure carries only Cause, terminal statuses
// carry neither. Attempts are immutable values after settlement.
type Attempt struct {
	Status  RefreshStatus // settled classification of this attempt
	Catalog *Catalog      // refreshed snapshot, non-nil exactly when Status is RefreshOK
	Cause   error         // underlying failure, non-nil exactly when Status is RefreshFailed
}

// NewSuccess returns an attempt that installs c as the refreshed catalog.
func NewSuccess(c *Catalog) Attempt {
	return Attempt{Status: RefreshOK, Catalog: c}
}

// NewFailure returns an ordinary failed attempt carrying cause verbatim;
// cause must be non-nil because the inline stale message renders it directly.
func NewFailure(cause error) Attempt {
	return Attempt{Status: RefreshFailed, Cause: cause}
}

// NewTerminal returns a deletion- or replacement-classified attempt whose
// result overrides any stale-schema workflow at every consumer.
func NewTerminal(status RefreshStatus) Attempt {
	return Attempt{Status: status}
}

// Valid reports whether the settled attempt obeys the payload rules above:
// each status carries exactly its required fields, which lets consumers rely
// on a nil-Catalog failure alone meaning "retain the prior catalog".
func (a Attempt) Valid() bool {
	switch a.Status {
	case RefreshOK:
		return a.Catalog != nil && a.Cause == nil
	case RefreshFailed:
		return a.Cause != nil && a.Catalog == nil
	case RefreshDeleted, RefreshReplaced:
		return a.Catalog == nil && a.Cause == nil
	default:
		return false
	}
}
