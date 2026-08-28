// Consecutive-identical append suppression (Issue #20): the single
// execution-start append entry point for query history. Per the PRD History
// decision, an actual execution appends its normalized state unless it is
// equal to the immediately preceding retained execution; A→B→A therefore
// retains both A entries. A suppressed append consumes no stable ID and
// causes no eviction.

package history

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// AppendExecution appends state as one actual execution start unless it is
// normalized-equal to the immediately preceding retained execution, in which
// case it is suppressed. It returns the new entry's stable ID with true when
// appended, and (0, false) when suppressed: no ID is allocated and storage is
// untouched. Callers append only when an actual execution starts — after
// successful pre-execution validation and, for UPDATE/DELETE, destructive
// confirmation — never during runnable evaluation, validation, estimation,
// cancellation, or dismissal.
func (s *Store) AppendExecution(state qb.HistoryState) (EntryID, bool) {
	if n := len(s.entries); n > 0 && s.entries[n-1].State.Equal(state) {
		return 0, false
	}
	return s.Append(state), true
}
