// Outcome-unknown write finalization support (Issue #45), per the Writes and
// commit boundary decisions in Notes/PRD-sqloid.md. When a write's rollback
// or commit cannot be resolved, the settled result is neither committed nor
// provably rolled back, so finalization appends one immutable non-tabular
// KindOutcomeUnknown entry carrying the operation, table, executed standalone
// SQL, commit-versus-rollback phase, driver error, and the optional actual
// statement RowsAffected with wording that explicitly says it does not prove
// persistence. No summary here claims the database was committed, rolled
// back, or untouched.

package history

import (
	"fmt"
)

// UnknownPhase identifies which noncancellable phase of an unresolved write
// did not resolve: the commit or the rollback cleanup. The UI maps the typed
// connection phases onto this value; the zero value is never stored.
type UnknownPhase int

const (
	// UnknownPhaseCommit means the commit phase started and its outcome
	// could not be resolved.
	UnknownPhaseCommit UnknownPhase = iota + 1
	// UnknownPhaseRollback means the rollback cleanup started and its
	// outcome could not be confirmed.
	UnknownPhaseRollback
)

// String renders the phase name for tests and diagnostics.
func (p UnknownPhase) String() string {
	switch p {
	case UnknownPhaseCommit:
		return "commit"
	case UnknownPhaseRollback:
		return "rollback"
	default:
		return fmt.Sprintf("UnknownPhase(%d)", int(p))
	}
}

// WriteUnknownSummary renders the exact one-line summary label for one
// unresolved write of the named operation. The label names the phase that
// did not resolve, preserves the driver error when present, and — when the
// statement itself reported a row count — reports it with wording that
// explicitly says it does not prove persistence. It never claims the
// database was committed, rolled back, or untouched.
func WriteUnknownSummary(operation string, phase UnknownPhase, cause string, rowsAffected int64, rowsKnown bool) string {
	what := "the rollback did not resolve"
	if phase == UnknownPhaseCommit {
		what = "the commit did not resolve"
	}
	label := fmt.Sprintf("%s outcome unknown: %s", operation, what)
	if cause != "" {
		label = fmt.Sprintf("%s (%s)", label, cause)
	}
	if rowsKnown {
		return fmt.Sprintf("%s; the statement reported %d rows affected, which does not prove persistence", label, rowsAffected)
	}
	return label + "; the final database state is not proven"
}
