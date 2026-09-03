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
// finalization preserves without clamping anything. Issue #76: the typed
// limit-failure kind and one-based position are preserved from the accepted
// active ResultView through finalization as immutable facts independent of
// the terminal outcome and byte-cap eviction disclosure.
type Finalization struct {
	Outcome                history.TerminalOutcome
	Reason                 string
	HasFailurePosition     bool
	FailurePosition        int64
	LimitFailureKind       result.LimitKind
	LimitFailurePosition   int64
	InvalidUTF             bool
	ReachedLow             bool
	ReachedHigh            bool
	CountWorkFinished      bool
	PageWorkFinished       bool
	ObservedShortFinalPage bool
	CountCacheInconsistent bool
}

// authoritativeFacts holds the endpoint observations and traversal facts
// derived from one shared helper for both active export and finalization
// (Issue #80). Identical active model state produces equivalent facts
// through this single seam: the retained-cache low evidence, the successful
// limited-result count or observed short/empty page high evidence, the
// count/page work state, and the count/cache contradiction without clamping.
// The terminal outcome is not derived here: active capture carries none and
// finalization supplies it separately through Finalization.
type authoritativeFacts struct {
	ReachedLow  bool
	ReachedHigh bool
	Traversal   history.TraversalFacts
}

// deriveAuthoritativeFacts derives the endpoint observations and traversal
// facts from the authoritative cache facts, count state, page pending state,
// and the accepted pageExhausted observation. Both active export and
// finalization call this single helper so identical active state produces
// equivalent endpoint/traversal facts and matching completeness labels.
//
// ReachedLow derives from the retained low boundary (position 1 retained) or
// truthful row-cap eviction evidence; an empty observed page (pageExhausted
// with no retained range) establishes both endpoints at position 0.
// ReachedHigh derives from a successful limited-result count relative to the
// retained range (total at or below the retained end) or the accepted
// pageExhausted observation. CountCacheInconsistent derives exactly as
// Issue #78 specifies: a successful count whose total falls below the
// retained cache end contradicts the cache, and the contradiction is
// preserved without clamping either the total or the retained range.
func deriveAuthoritativeFacts(facts history.CacheFacts, count result.CountState, pagePending, pageExhausted bool) authoritativeFacts {
	af := authoritativeFacts{
		Traversal: history.TraversalFacts{
			CountWorkFinished:      count.Status != result.CountPending,
			PageWorkFinished:       !pagePending,
			ObservedShortFinalPage: pageExhausted,
		},
	}
	if facts.HasRetainedRange {
		af.ReachedLow = facts.Start == 1 || facts.RowCapEvictions > 0
		if count.Status == result.CountSuccess && count.Total <= int64(facts.End) {
			af.ReachedHigh = true
		}
	}
	if pageExhausted {
		af.ReachedHigh = true
		if !facts.HasRetainedRange {
			af.ReachedLow = true
		}
	}
	// Issue #78: a successful limited-result count whose total falls below
	// the retained cache end contradicts the cache. The contradiction is
	// recorded without rewriting the total, retained range, endpoint
	// observations, or count state; the corrected history.Classify from
	// Issue #77 then rejects complete.
	if count.Status == result.CountSuccess && facts.HasRetainedRange && int64(facts.End) > count.Total {
		af.Traversal.CountCacheInconsistent = true
	}
	return af
}

// SnapshotFacts finalizes this model's authoritative facts into an immutable
// history.SnapshotMetadata value together with the traversal facts captured
// at the same instant for history.Classify. The known total comes from the
// settled count state (the count of the complete SELECT including the user's
// Limit, so rows beyond that limited logical result are irrelevant and
// classification needs no raw builder Limit), the retained range and
// eviction facts come from the viewport cache, and every other fact comes
// from the Finalization inputs. The returned values are independent of the
// model: later navigation, eviction, or count settlement cannot change them.
func (m *Model) SnapshotFacts(f Finalization) (history.SnapshotMetadata, history.TraversalFacts, error) {
	countWorkFinished := f.CountWorkFinished || m.countState.Status != result.CountPending
	facts := history.FactsFromCache(m.viewportCacheOf())
	meta, err := history.NewSnapshotMetadata(facts, history.Lifecycle{
		Outcome:              f.Outcome,
		Reason:               f.Reason,
		HasFailurePosition:   f.HasFailurePosition,
		FailurePosition:      f.FailurePosition,
		LimitFailureKind:     f.LimitFailureKind,
		LimitFailurePosition: f.LimitFailurePosition,
		InvalidUTF:           f.InvalidUTF,
		ReachedLow:           f.ReachedLow,
		ReachedHigh:          f.ReachedHigh,
		HasKnownTotal:        m.countState.Status == result.CountSuccess,
		KnownTotal:           m.countState.Total,
	})
	if err != nil {
		return history.SnapshotMetadata{}, history.TraversalFacts{}, fmt.Errorf("ui: finalize snapshot facts: %w", err)
	}
	traversal := history.TraversalFacts{
		CountWorkFinished:      countWorkFinished,
		PageWorkFinished:       f.PageWorkFinished,
		ObservedShortFinalPage: f.ObservedShortFinalPage,
		CountCacheInconsistent: f.CountCacheInconsistent,
	}
	return meta, traversal, nil
}
