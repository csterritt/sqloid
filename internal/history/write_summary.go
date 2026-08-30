// Operation-appropriate write summary labels (Issue #42), per the Builder and
// Display Interaction and History decisions in Notes/PRD-sqloid.md. A write
// summary is one immutable non-tabular result label built from the actual
// statement RowsAffected() — never the destructive estimate — with
// rows-affected wording for UPDATE/DELETE and rows-added wording for INSERT.
// A cancelled or failed write may claim the database was untouched only after
// rollback success is confirmed; without that confirmation the label makes no
// untouched claim at all.

package history

import (
	"fmt"
	"strings"
)

// WriteStatus is the definite terminal status of a resolved write, mirroring
// the connection boundary's committed/cancelled/failed classification. The
// unresolved outcome-unknown terminal workflow is Issue #45's.
type WriteStatus int

const (
	// WriteStatusCommitted means the write's transaction committed.
	WriteStatusCommitted WriteStatus = iota + 1
	// WriteStatusCancelled means the write was cancelled and rolled back;
	// the label may claim untouched only when rollback was confirmed.
	WriteStatusCancelled
	// WriteStatusFailed means the statement or commit failed and the
	// transaction rolled back; the label may claim untouched only when
	// rollback was confirmed.
	WriteStatusFailed
)

// String renders the status name for tests and diagnostics.
func (s WriteStatus) String() string {
	switch s {
	case WriteStatusCommitted:
		return "committed"
	case WriteStatusCancelled:
		return "cancelled"
	case WriteStatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("WriteStatus(%d)", int(s))
	}
}

// WriteSummary renders the exact one-line summary label for one resolved
// write of the named operation ("UPDATE", "DELETE", or "INSERT" as rendered
// by the builder). Committed UPDATE/DELETE report the actual statement
// RowsAffected as rows affected; committed INSERT reports rows added.
// Cancelled and failed labels carry the verbatim failure cause where present
// and append the untouched claim only when rollbackConfirmed is true.
func WriteSummary(operation string, status WriteStatus, rowsAffected int64, rollbackConfirmed bool, cause string) string {
	if status == WriteStatusCommitted {
		if strings.EqualFold(operation, "INSERT") {
			return fmt.Sprintf("%s committed: %d rows added", operation, rowsAffected)
		}
		return fmt.Sprintf("%s committed: %d rows affected", operation, rowsAffected)
	}
	if status == WriteStatusCancelled {
		if rollbackConfirmed {
			return fmt.Sprintf("%s cancelled: rollback confirmed, database untouched", operation)
		}
		return fmt.Sprintf("%s cancelled", operation)
	}
	if rollbackConfirmed {
		return fmt.Sprintf("%s failed: %s (rollback confirmed, database untouched)", operation, cause)
	}
	return fmt.Sprintf("%s failed: %s", operation, cause)
}
