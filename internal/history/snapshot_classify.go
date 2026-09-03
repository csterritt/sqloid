// Truthful completeness and endpoint classification for Issues #33 and #77,
// per the Cache and snapshot invariant of Notes/PRD-sqloid.md: exclusive
// `complete` over the limited logical result, truthful `partial` and
// `truncated` that may coexist, count failure and short/empty observation
// endpoint rules, the unknown-remainder case, no clamping of inconsistent
// count/cache evidence, and the Issue #77 low-endpoint distinction: an
// unseen low endpoint on a nonempty result is partial (lower rows may be
// unseen), never truncated by itself, while an empty logical result
// completes without ReachedLow because there is no low row to observe.
// Terminal outcomes stay orthogonal: classification consumes completeness
// facts only and never inspects the terminal outcome.

package history

// Completeness is one snapshot's truthfully classified label combination.
// Complete is exclusive; Partial and Truncated may coexist and neither
// coexists with Complete. At least one label is always set.
type Completeness struct {
	Complete  bool
	Partial   bool
	Truncated bool
}

// String renders the label combination for tests and diagnostics.
func (c Completeness) String() string {
	switch {
	case c.Complete:
		return "complete"
	case c.Partial && c.Truncated:
		return "partial+truncated"
	case c.Partial:
		return "partial"
	case c.Truncated:
		return "truncated"
	default:
		return "none"
	}
}

// TraversalFacts carries the count and observed-page lifecycle facts
// supplied by internal/ui at finalization time. CountWorkFinished reports
// that the count request settled (success or failure) or was never issued;
// PageWorkFinished reports that no page work was outstanding or deferred.
// ObservedShortFinalPage records that a final page was actually observed to
// return fewer rows than requested (including an empty page), which alone —
// when the count is unavailable or failed — establishes the high endpoint.
// CountCacheInconsistent records contradictory count/cache evidence, which
// classification preserves without clamping any observed fact. The executed
// builder Limit is intentionally absent: a successful known total already
// counts the complete SELECT including the user's Limit, so rows beyond
// that limited logical result are irrelevant and classification needs no
// raw builder Limit.
type TraversalFacts struct {
	CountWorkFinished      bool
	PageWorkFinished       bool
	ObservedShortFinalPage bool
	CountCacheInconsistent bool
}

// Classify computes the exclusive-or-coexisting completeness labels from
// immutable snapshot metadata and traversal facts. It never mutates its
// inputs: rows, retained range, known total, and endpoint observations stand
// exactly as observed, even when contradictory. Complete is possible only
// when the high endpoint is established, every row of the limited logical
// result is retained, no eviction occurred, all count and page work finished,
// and the low-endpoint condition holds: an empty logical result (high == 0)
// is vacuously complete without a low row to observe, while a nonempty result
// requires ReachedLow. A successful known total already counts the complete
// SELECT including the user's Limit, so rows beyond that limited logical
// result are irrelevant and classification consumes no raw builder Limit.
func Classify(meta SnapshotMetadata, t TraversalFacts) Completeness {
	// The high endpoint is established either by a known total (the count of
	// the SELECT including the user's Limit) or by an actually observed
	// short or empty final page. It is never inferred from an unobserved
	// remainder.
	highKnown := meta.HasKnownTotal || t.ObservedShortFinalPage
	high := int64(0)
	if meta.HasKnownTotal {
		high = meta.KnownTotal
	} else if t.ObservedShortFinalPage {
		if meta.HasRetainedRange {
			high = int64(meta.RetainedEnd)
		}
		// No retained range with a short or empty observed final page means
		// the result is empty: the high endpoint is position 0.
	}

	// Known rows lie beyond the retained range when the retained end falls
	// short of the established high endpoint. Contradictory evidence is
	// preserved, never clamped away.
	rowsBeyondRange := highKnown && meta.HasRetainedRange && int64(meta.RetainedEnd) < high

	evicted := meta.RowCapEvicted || meta.TruncatedByByteCap
	workFinished := t.CountWorkFinished && t.PageWorkFinished
	inconsistent := t.CountCacheInconsistent

	// Empty logical results (high endpoint 0) are fully retained vacuously;
	// otherwise every limited row must sit inside the retained range with
	// its low end at position 1.
	fullRetention := high == 0 ||
		(meta.HasRetainedRange && meta.RetainedStart == 1 && int64(meta.RetainedEnd) >= high)

	// Issue #77: an empty logical result (high == 0) has no low row to
	// observe, so ReachedLow is not required; a nonempty result requires
	// ReachedLow so that unseen lower rows do not falsely complete.
	complete := !evicted && !inconsistent && highKnown && workFinished &&
		(high == 0 || meta.ReachedLow) && fullRetention

	// Partial: unseen limited-result rows may remain (unknown remainder, an
	// unobserved high endpoint with rows beyond the range, an unobserved low
	// endpoint on a nonempty result where lower rows may be unseen) or
	// count/page work did not finish, or count/cache evidence is
	// contradictory and cannot be trusted to be complete.
	partial := !complete && (!highKnown || !workFinished || inconsistent ||
		(!meta.ReachedHigh && rowsBeyondRange) || (high != 0 && !meta.ReachedLow))

	// Truncated: known or observed rows were evicted or lie beyond the
	// retained range. A missing low endpoint alone is never truncation.
	truncated := evicted || rowsBeyondRange

	switch {
	case complete:
		return Completeness{Complete: true}
	case partial && truncated:
		return Completeness{Partial: true, Truncated: true}
	case partial:
		return Completeness{Partial: true}
	default:
		return Completeness{Truncated: true}
	}
}
