// Immutable result-history entries (Issues #33 and #34), per the Cache and
// snapshot invariant and History Module Design decisions in
// Notes/PRD-sqloid.md. Every actual SELECT execution is finalized exactly
// once into one entry: a tabular snapshot whose retained rows, columns, and
// Issue #33 metadata are captured immutably, or — when cancellation or
// first-page failure occurs before any row is retained — a non-tabular
// Cancelled or error entry. AppendFinalized is the single entry point; it
// rejects a second entry for an already-finalized execution ID, so duplicate
// or late finalization messages can never append a second entry or mutate the
// first. The package has no database and no Bubble Tea dependency.

package history

import (
	"fmt"

	"github.com/chris/sqloid/internal/result"
)

// ResultKind is the entry kind of one finalized SELECT execution: a tabular
// snapshot (success, count failure with rows, or partial page failure with
// rows) or one of the defined non-tabular terminal entries.
type ResultKind int

const (
	// KindTabular marks a tabular snapshot carrying captured rows.
	KindTabular ResultKind = iota
	// KindCancelled marks the non-tabular Cancelled entry created when the
	// execution was cancelled before any row was retained.
	KindCancelled
	// KindError marks the non-tabular error entry created when the first page
	// failed before any row was retained.
	KindError
	// KindWrite marks the non-tabular write summary entry created when one
	// transactional write (Issue #42) resolved to a definite outcome. Summary
	// carries the exact operation-appropriate label and SQL the executed
	// standalone statement.
	KindWrite
	// KindOutcomeUnknown marks the non-tabular outcome-unknown entry (Issue
	// #45) created when a write's rollback or commit could not be resolved:
	// the entry carries Operation, Table, SQL, Phase, the driver error inside
	// Summary, and an optional RowsAffected explicitly labeled as not proving
	// persistence.
	KindOutcomeUnknown
)

// String renders the result-kind name for tests and diagnostics.
func (k ResultKind) String() string {
	switch k {
	case KindTabular:
		return "tabular"
	case KindCancelled:
		return "cancelled"
	case KindError:
		return "error"
	case KindWrite:
		return "write"
	case KindOutcomeUnknown:
		return "outcome-unknown"
	default:
		return fmt.Sprintf("ResultKind(%d)", int(k))
	}
}

// ResultCapacity is the exact number of finalized result entries the store
// retains at once (Issue #36). Each append beyond it evicts exactly the oldest
// retained entry first; surviving IDs are never changed.
const ResultCapacity = 20

// ResultEntry is one immutable finalized snapshot. For KindTabular,
// Columns and Rows carry the captured rows in ascending logical position
// order with their Issue #33 Metadata and Completeness classification; Rows
// is freshly copied at append, so later caller or source mutation can never
// alter the retained data. For KindCancelled and KindError, Reason carries
// the verbatim cancellation or failure reason and Rows is empty. For
// KindWrite, Summary carries the exact write summary label, SQL carries the
// executed standalone statement, and RowsAffected carries the actual
// statement count (never an estimate) for later Issue #45 re-labeling.
// ExecutionID records the one execution this entry belongs to.
type ResultEntry struct {
	ID           EntryID
	ExecutionID  uint64
	Kind         ResultKind
	Columns      []string
	Rows         [][]result.Value
	Metadata     SnapshotMetadata
	Completeness Completeness
	Reason       string
	SQL          string
	Summary      string
	RowsAffected int64
	Operation    string
	Table        string
	Phase        UnknownPhase
	// QueryEntryID is the stable ID of the query-history entry (Issue #20)
	// whose complete immutable state was this execution's input; zero when no
	// such entry exists (no wired history store, or an append suppressed with
	// no retained entry). It lets Ctrl+S save targeting resolve the viewed
	// result's query from immutable history identity rather than any rendered
	// text (Issue #48).
	QueryEntryID EntryID
}

// ResultStore is the in-memory result-history list of finalized SELECT
// entries. Append and retrieval always deep-copy mutable slices, so retained
// entries never alias caller storage. It is not safe for concurrent use; the
// single Bubble Tea update loop is its only expected caller.
type ResultStore struct {
	nextID    EntryID
	entries   []ResultEntry
	finalized map[uint64]struct{} // execution IDs already finalized
}

// NewResultStore returns an empty result-history store.
func NewResultStore() *ResultStore {
	return &ResultStore{finalized: make(map[uint64]struct{})}
}

// Len reports the number of retained result entries.
func (s *ResultStore) Len() int { return len(s.entries) }

// copyRows deep-copies a captured rows slice so the retained entry never
// aliases caller storage and later source mutation cannot reach the snapshot.
func copyRows(rows [][]result.Value) [][]result.Value {
	out := make([][]result.Value, len(rows))
	for i, row := range rows {
		copied := make([]result.Value, len(row))
		for j, v := range row {
			if v.Kind == result.KindBlob {
				v.Bytes = append([]byte(nil), v.Bytes...)
			}
			copied[j] = v
		}
		out[i] = copied
	}
	return out
}

// AppendFinalized retains entry as the newest result-history entry under a
// fresh stable ID and returns it. It rejects — deterministically, with the
// original entry untouched — a second entry for an execution ID that has
// already been finalized in this store, so replayed duplicate finalizer
// messages and repeated history-entry commands are no-ops. Columns and Rows
// are deep-copied on retention; later caller or cache mutation cannot change
// a finalized entry.
func (s *ResultStore) AppendFinalized(entry ResultEntry) (ResultEntry, bool) {
	if entry.ExecutionID == 0 {
		return ResultEntry{}, false
	}
	if _, done := s.finalized[entry.ExecutionID]; done {
		return ResultEntry{}, false
	}
	s.finalized[entry.ExecutionID] = struct{}{}
	s.nextID++
	retained := entry
	retained.ID = s.nextID
	retained.Columns = append([]string(nil), entry.Columns...)
	retained.Rows = copyRows(entry.Rows)
	s.entries = append(s.entries, retained)
	// Issue #36: exactly the newest ResultCapacity entries are retained;
	// eviction is oldest-first and never changes surviving IDs.
	if len(s.entries) > ResultCapacity {
		s.entries = s.entries[len(s.entries)-ResultCapacity:]
	}
	return retained, true
}

// Entries returns the retained entries oldest first as a fresh slice; the
// entries' rows are deep-copied too, so callers may mutate them freely.
func (s *ResultStore) Entries() []ResultEntry {
	out := make([]ResultEntry, len(s.entries))
	for i, e := range s.entries {
		copied := e
		copied.Columns = append([]string(nil), e.Columns...)
		copied.Rows = copyRows(e.Rows)
		out[i] = copied
	}
	return out
}
