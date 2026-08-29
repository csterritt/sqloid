// Immutable typed snapshot metadata (Issue #33), per the Cache and snapshot
// invariant and History Module Design decisions in Notes/PRD-sqloid.md. The
// metadata value is independent of retained row storage and of presentation
// strings: it records the optional inclusive retained start/end range,
// optional known total, reached-low/reached-high endpoint observations,
// persistent row-cap and byte-cap eviction facts, UTF status, and an
// independently typed terminal outcome with cancellation/failure reason and
// optional one-based last failure position. Facts are converted once, at a
// narrow boundary, from the authoritative internal/resultcache cache and the
// lifecycle inputs supplied by internal/ui; nothing here derives facts from
// labels or UI text, and the shared Issue #31 warning definition is reused
// only at later presentation boundaries, never duplicated in this model.

package history

import (
	"fmt"

	"github.com/chris/sqloid/internal/resultcache"
)

// Position is one absolute logical result row position, one-based, as
// defined authoritatively by internal/resultcache.
type Position = resultcache.Position

// TerminalOutcome is the independently typed terminal outcome of one SELECT
// execution's snapshot. It is orthogonal to completeness: success,
// cancellation, and failure each combine freely with any completeness label.
type TerminalOutcome int

const (
	// OutcomeNone is not a valid finalized outcome; the constructor rejects
	// it so a metadata value always records exactly one terminal outcome.
	OutcomeNone TerminalOutcome = iota
	// OutcomeSuccess means the execution settled successfully.
	OutcomeSuccess
	// OutcomeCancelled means the execution was cancelled; Reason carries the
	// cancellation reason.
	OutcomeCancelled
	// OutcomeFailed means the execution failed; Reason carries the failure
	// reason and HasFailurePosition/FailurePosition carry the optional
	// one-based last failure position.
	OutcomeFailed
)

// String renders the terminal outcome name for tests and diagnostics.
func (o TerminalOutcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeFailed:
		return "failed"
	default:
		return fmt.Sprintf("TerminalOutcome(%d)", int(o))
	}
}

// CacheFacts is the narrow set of authoritative facts converted from a
// resultcache.Cache at finalization time. It is a pure value type:
// FactsFromCache copies everything out of the cache, so later cache activity never
// changes an already-converted facts value.
type CacheFacts struct {
	// HasRetainedRange reports that the cache retained at least one row;
	// Start and End are the inclusive retained range and are meaningful only
	// when true.
	HasRetainedRange bool
	Start            Position
	End              Position
	// RowCapEvictions is the cumulative count of positions evicted by the
	// resultcache.MaxPositions cap across the cache's lifetime.
	RowCapEvictions int
	// TruncatedByByteCap is the persistent `truncated-by-byte-cap` typed
	// disclosure: set once any MaxPayloadBytes eviction occurred and never
	// cleared, independent of the current retained payload.
	TruncatedByByteCap bool
}

// FactsFromCache converts the authoritative cache facts to the narrow
// snapshot-metadata boundary type. The returned value is independent of the
// cache: later merges and evictions do not change it.
func FactsFromCache(c *resultcache.Cache) CacheFacts {
	facts := CacheFacts{
		RowCapEvictions:    c.RowCapEvictions(),
		TruncatedByByteCap: c.TruncatedByByteCap(),
	}
	if start, ok := c.Start(); ok {
		end, _ := c.End()
		facts.HasRetainedRange = true
		facts.Start, facts.End = start, end
	}
	return facts
}

// Lifecycle carries the non-cache inputs to snapshot finalization: the
// terminal outcome and its reason and optional one-based last failure
// position, the invalid-UTF-8 status, the reached-low/reached-high endpoint
// observations, and the optional known total of the limited logical result.
// Every field is a value; Reason is an immutable string, so the metadata
// value constructed from a Lifecycle can never alias mutable caller state.
type Lifecycle struct {
	Outcome            TerminalOutcome
	Reason             string
	HasFailurePosition bool
	FailurePosition    int64 // one-based; meaningful only with HasFailurePosition
	InvalidUTF         bool
	ReachedLow         bool
	ReachedHigh        bool
	HasKnownTotal      bool
	KnownTotal         int64
}

// SnapshotMetadata is one immutable, typed snapshot metadata value. Every
// field is a scalar value with copy semantics, so a finalized value can never
// be changed through any alias: constructing from CacheFacts and Lifecycle
// copies all inputs, and value copies of the metadata are fully independent.
// The value is independent of retained rows and of presentation strings —
// rendering, including the shared Issue #31 byte-cap warning, happens only
// later from these typed facts.
type SnapshotMetadata struct {
	// HasRetainedRange reports that at least one row was retained; Retained
	// Start and RetainedEnd are the inclusive retained range and are
	// meaningful only when true.
	HasRetainedRange bool
	RetainedStart    Position
	RetainedEnd      Position
	// HasKnownTotal reports that the total of the limited logical result is
	// known; KnownTotal is meaningful only when true.
	HasKnownTotal bool
	KnownTotal    int64
	// ReachedLow and ReachedHigh record the endpoint observations: whether
	// traversal established the first and last logical rows.
	ReachedLow  bool
	ReachedHigh bool
	// RowCapEvicted reports that the MaxPositions cap evicted positions at
	// some point; RowCapEvictions is the cumulative count. Both persist
	// after later traversal changes.
	RowCapEvicted   bool
	RowCapEvictions int
	// TruncatedByByteCap is the persistent typed `truncated-by-byte-cap`
	// fact. Once set it stays set even when later retained bytes fall below
	// the cap; its presentation is the shared internal/result warning, not
	// model text.
	TruncatedByByteCap bool
	// InvalidUTF records that at least one retained TEXT value required
	// maximal invalid-UTF-8 replacement.
	InvalidUTF bool
	// Outcome is the terminal outcome, independent of completeness.
	Outcome TerminalOutcome
	// Reason carries the cancellation or failure reason verbatim; empty is
	// meaningful only for OutcomeSuccess.
	Reason string
	// HasFailurePosition reports that a one-based last failure position was
	// recorded; FailurePosition is meaningful only when true and is
	// applicable only to cancellation and failure outcomes.
	HasFailurePosition bool
	FailurePosition    int64
}

// NewSnapshotMetadata constructs an immutable metadata value from cache
// facts and lifecycle inputs, validating shape without rewriting observed
// facts: a supplied retained range must have End >= Start, the outcome must
// be one of the three typed terminal outcomes, and a failure position must
// be one-based and only accompany a cancellation or failure. Absent facts
// are represented by their zero values, never by rewritten observations.
func NewSnapshotMetadata(facts CacheFacts, life Lifecycle) (SnapshotMetadata, error) {
	if facts.HasRetainedRange && facts.End < facts.Start {
		return SnapshotMetadata{}, fmt.Errorf("history: retained range end %d before start %d", facts.End, facts.Start)
	}
	switch life.Outcome {
	case OutcomeSuccess:
		if life.HasFailurePosition {
			return SnapshotMetadata{}, fmt.Errorf("history: failure position %d not applicable to a successful outcome", life.FailurePosition)
		}
	case OutcomeCancelled, OutcomeFailed:
		if life.HasFailurePosition && life.FailurePosition < 1 {
			return SnapshotMetadata{}, fmt.Errorf("history: failure position %d is not one-based", life.FailurePosition)
		}
	default:
		return SnapshotMetadata{}, fmt.Errorf("history: unknown terminal outcome %d", int(life.Outcome))
	}
	return SnapshotMetadata{
		HasRetainedRange:   facts.HasRetainedRange,
		RetainedStart:      facts.Start,
		RetainedEnd:        facts.End,
		HasKnownTotal:      life.HasKnownTotal,
		KnownTotal:         life.KnownTotal,
		ReachedLow:         life.ReachedLow,
		ReachedHigh:        life.ReachedHigh,
		RowCapEvicted:      facts.RowCapEvictions > 0,
		RowCapEvictions:    facts.RowCapEvictions,
		TruncatedByByteCap: facts.TruncatedByByteCap,
		InvalidUTF:         life.InvalidUTF,
		Outcome:            life.Outcome,
		Reason:             life.Reason,
		HasFailurePosition: life.HasFailurePosition,
		FailurePosition:    life.FailurePosition,
	}, nil
}
