// Snapshot finalization inside the UI (Issue #33), per the Cache and
// snapshot invariant and History Module Design decisions in
// Notes/PRD-sqloid.md. At finalization the model converts its authoritative
// state — the dual-cap viewport cache, the independent count state, and the
// paging seam's endpoint observations — into an immutable typed
// internal/history SnapshotMetadata through the narrow CacheFacts/Lifecycle
// boundary. No presentation text is produced here: the shared Issue #31
// byte-cap warning and the completeness labels stay with their owning
// presentation boundaries.

package ui

import (
	"fmt"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// Finalization carries the terminal and observation inputs the paging and
// cancellation seams own at snapshot finalization. CountWorkFinished and
// PageWorkFinished report that count and page work settled (or were never
// issued); ObservedShortFinalPage records that a final page was actually
// observed to return fewer rows than requested, including an empty page;
// CountCacheInconsistent records contradictory count/cache evidence, which
// finalization preserves without clamping anything.
type Finalization struct {
	Outcome                history.TerminalOutcome
	Reason                 string
	HasFailurePosition     bool
	FailurePosition        int64
	InvalidUTF             bool
	ReachedLow             bool
	ReachedHigh            bool
	CountWorkFinished      bool
	PageWorkFinished       bool
	ObservedShortFinalPage bool
	CountCacheInconsistent bool
}

// SnapshotFacts finalizes this model's authoritative facts into an immutable
// history.SnapshotMetadata value together with the traversal facts captured
// at the same instant for history.Classify. The known total and the executed
// Limit come from the settled count state (the count of the complete limited
// SELECT result), the retained range and eviction facts come from the
// viewport cache, and every other fact comes from the Finalization inputs.
// The returned values are independent of the model: later navigation,
// eviction, or count settlement cannot change them.
func (m *Model) SnapshotFacts(f Finalization) (history.SnapshotMetadata, history.TraversalFacts, error) {
	countWorkFinished := f.CountWorkFinished || m.countState.Status != result.CountPending
	facts := history.FactsFromCache(m.viewportCacheOf())
	meta, err := history.NewSnapshotMetadata(facts, history.Lifecycle{
		Outcome:            f.Outcome,
		Reason:             f.Reason,
		HasFailurePosition: f.HasFailurePosition,
		FailurePosition:    f.FailurePosition,
		InvalidUTF:         f.InvalidUTF,
		ReachedLow:         f.ReachedLow,
		ReachedHigh:        f.ReachedHigh,
		HasKnownTotal:      m.countState.Status == result.CountSuccess,
		KnownTotal:         m.countState.Total,
	})
	if err != nil {
		return history.SnapshotMetadata{}, history.TraversalFacts{}, fmt.Errorf("ui: finalize snapshot facts: %w", err)
	}
	traversal := history.TraversalFacts{
		HasLimit:               m.countState.HasLimit,
		Limit:                  m.countState.Limit,
		CountWorkFinished:      countWorkFinished,
		PageWorkFinished:       f.PageWorkFinished,
		ObservedShortFinalPage: f.ObservedShortFinalPage,
		CountCacheInconsistent: f.CountCacheInconsistent,
	}
	return meta, traversal, nil
}
