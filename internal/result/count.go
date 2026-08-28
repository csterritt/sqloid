// Independent result-count presentation state (Issue #24), per the Paging
// consistency decision in Notes/PRD-sqloid.md. The count of one SELECT
// execution is its own lifecycle, observed independently of the first page:
// pending, successful, or unavailable, each rendering from explicit state —
// never inferred from row length. HasLimit and Limit are executed metadata
// captured at launch from the executed builder, so later builder edits cannot
// change the wording of an already-settled count.

package result

import "strconv"

// CountStatus identifies the presentation state of one SELECT execution's
// independent result count. The zero value means no count request was issued.
type CountStatus int

const (
	// CountPending means the count request is in flight; the established
	// presentation is the exact pending wording from Header.
	CountPending CountStatus = iota + 1
	// CountSuccess means the count settled successfully with a total.
	CountSuccess
	// CountUnavailable means the count request failed; rows and paging stay
	// fully usable and the failure never becomes a page failure.
	CountUnavailable
)

// String renders the human-facing name of the count status used in tests and
// diagnostics.
func (s CountStatus) String() string {
	switch s {
	case CountPending:
		return "pending"
	case CountSuccess:
		return "success"
	case CountUnavailable:
		return "unavailable"
	default:
		return "CountStatus(" + strconv.Itoa(int(s)) + ")"
	}
}

// CountState is one SELECT execution's independent result-count state. Total
// is meaningful only on CountSuccess; HasLimit and Limit carry the executed
// builder's user Limit for the exact after-Limit wording and never clamp or
// reinterpret Total.
type CountState struct {
	Status   CountStatus
	Total    int64
	HasLimit bool
	Limit    int64
}

// Header returns the exact PRD status/count wording for this state:
// `Counting rows…` while pending, exactly `Result count: N` on success
// without a user Limit, exactly `Result count: N (after Limit M)` on success
// with one, and exactly `Count unavailable` on failure. The wording never
// implies a table size or a pre-Limit count. A zero-value state (no count
// request issued) returns the empty string so callers render only from
// explicit state.
func (c CountState) Header() string {
	switch c.Status {
	case CountPending:
		return "Counting rows…"
	case CountSuccess:
		total := strconv.FormatInt(c.Total, 10)
		if c.HasLimit {
			return "Result count: " + total + " (after Limit " + strconv.FormatInt(c.Limit, 10) + ")"
		}
		return "Result count: " + total
	case CountUnavailable:
		return "Count unavailable"
	default:
		return ""
	}
}
