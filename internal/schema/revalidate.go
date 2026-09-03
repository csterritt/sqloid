// Typed pre-execution schema-version revalidation outcomes (Issue #21), per
// the Schema scope and cache decisions in Notes/PRD-sqloid.md. Before any
// SELECT or write execution, the UI asks Schema to revalidate the cached
// catalog against the database's current PRAGMA schema_version. An unchanged
// version reuses the exact cached object and column metadata without issuing
// any catalog refresh; a changed version refreshes through the established
// Attempt seam, so ordinary refresh failures carry only their cause (the
// prior cache stands, per Issue #13) and deletion/replacement classifications
// take their terminal precedence (per Issue #7). Outcomes are typed values:
// no consumer may infer any of this from error strings.

package schema

import "fmt"

// RevalidateStatus classifies one settled schema-version revalidation. The
// zero value is not a settled outcome.
type RevalidateStatus int

const (
	// RevalidateUnchanged means the current PRAGMA schema_version equals the
	// cached version: the exact cached Catalog was reused and no catalog
	// refresh was issued.
	RevalidateUnchanged RevalidateStatus = iota + 1
	// RevalidateRefreshed means the version changed and the refreshed
	// snapshot in Catalog was installed as the authoritative metadata.
	RevalidateRefreshed
	// RevalidateRefreshFailed means an ordinary refresh failure (lock,
	// corruption, change race): only Cause is set and every consumer must
	// retain the prior cache unchanged behind `could not refresh: <cause>`
	// reporting.
	RevalidateRefreshFailed
	// RevalidateDeleted is terminal: the database file no longer exists at
	// the request boundary.
	RevalidateDeleted
	// RevalidateReplaced is terminal: a different file now owns the startup
	// path (device/inode mismatch).
	RevalidateReplaced
)

// String renders the human-facing classification used in tests, diagnostics,
// and composition wiring from the Connection boundary.
func (s RevalidateStatus) String() string {
	switch s {
	case RevalidateUnchanged:
		return "unchanged"
	case RevalidateRefreshed:
		return "refreshed"
	case RevalidateRefreshFailed:
		return "refresh-failed"
	case RevalidateDeleted:
		return "deleted"
	case RevalidateReplaced:
		return "replaced"
	default:
		return fmt.Sprintf("RevalidateStatus(%d)", int(s))
	}
}

// Revalidation is one settled schema-version revalidation. Exactly one
// Status is meaningful, with the same payload discipline as Attempt: an
// unchanged outcome carries the exact prior Catalog pointer, a refreshed
// outcome carries the refreshed Catalog, an ordinary failure carries only
// Cause, and terminal outcomes carry neither. Revalidation values are
// immutable after settlement: later DDL races never retroactively mutate a
// settled outcome.
type Revalidation struct {
	Status  RevalidateStatus // settled classification of this revalidation
	Catalog *Catalog         // exact prior cache on unchanged, refreshed snapshot on refreshed; nil otherwise
	Cause   error            // underlying failure, non-nil exactly when Status is RevalidateRefreshFailed
}

// Valid reports whether the settled revalidation obeys the payload rules
// above: each status carries exactly its required fields, which lets consumers
// rely on a nil-Catalog refresh-failed outcome alone meaning "retain the prior
// cache". The matrix mirrors Attempt.Valid() and rejects zero or unknown
// statuses, missing required fields, and any contradictory extra payload.
func (r Revalidation) Valid() bool {
	switch r.Status {
	case RevalidateUnchanged, RevalidateRefreshed:
		return r.Catalog != nil && r.Cause == nil
	case RevalidateRefreshFailed:
		return r.Cause != nil && r.Catalog == nil
	case RevalidateDeleted, RevalidateReplaced:
		return r.Catalog == nil && r.Cause == nil
	default:
		return false
	}
}

// VersionAttempt is one settled PRAGMA schema_version read (Issue #21): the
// transport step that precedes Revalidate. RefreshOK carries Version;
// RefreshFailed carries Cause; RefreshDeleted and RefreshReplaced carry
// neither and classify terminally exactly like catalog-refresh attempts.
// Composition roots map connection health classifications onto these typed
// outcomes so no consumer infers anything from error strings.
type VersionAttempt struct {
	Status  RefreshStatus // RefreshOK, RefreshFailed, RefreshDeleted, or RefreshReplaced
	Version int64         // current schema version, meaningful only on RefreshOK
	Cause   error         // underlying failure, non-nil exactly on RefreshFailed
}

// NewVersionOK records one successful version read carrying v.
func NewVersionOK(v int64) VersionAttempt { return VersionAttempt{Status: RefreshOK, Version: v} }

// NewVersionFailure records one ordinary failed version read with cause err.
func NewVersionFailure(err error) VersionAttempt {
	return VersionAttempt{Status: RefreshFailed, Cause: err}
}

// NewVersionDeleted records one version read whose request boundary found the
// database file absent.
func NewVersionDeleted() VersionAttempt { return VersionAttempt{Status: RefreshDeleted} }

// NewVersionReplaced records one version read whose request boundary found a
// different file on the startup path.
func NewVersionReplaced() VersionAttempt { return VersionAttempt{Status: RefreshReplaced} }

// Revalidate compares the database's current schema version against the
// cached catalog. An equal version reuses prior — the exact cached pointer —
// without ever invoking refresh. A changed version refreshes through the
// established Attempt seam and maps its settled classification onto the
// typed revalidation outcome. Prior must be the caller's retained cache;
// refresh is invoked at most once and only on a changed version.
func Revalidate(prior *Catalog, currentVersion int64, refresh func() Attempt) Revalidation {
	if prior != nil && currentVersion == prior.Version {
		return Revalidation{Status: RevalidateUnchanged, Catalog: prior}
	}
	att := refresh()
	switch att.Status {
	case RefreshOK:
		return Revalidation{Status: RevalidateRefreshed, Catalog: att.Catalog}
	case RefreshFailed:
		return Revalidation{Status: RevalidateRefreshFailed, Cause: att.Cause}
	case RefreshDeleted:
		return Revalidation{Status: RevalidateDeleted}
	case RefreshReplaced:
		return Revalidation{Status: RevalidateReplaced}
	default:
		// Defensive against an unsettled or out-of-range Attempt payload;
		// never produced by the constructors in refresh.go. Settle as an
		// ordinary refresh failure with a concrete diagnostic cause and no
		// catalog so the stale-refresh workflow retains the prior cache
		// rather than leaking an unset status to consumers (Issue #82).
		return Revalidation{
			Status: RevalidateRefreshFailed,
			Cause:  fmt.Errorf("schema revalidate: unsettled refresh attempt status %s", att.Status),
		}
	}
}
