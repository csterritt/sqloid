// SELECT execution and request identity (Issue #24), per the Identities and
// state decision in Notes/PRD-sqloid.md. One actual SELECT execution receives
// exactly one fresh nonzero execution ID, and the two concurrent requests it
// launches — first page and complete-result count — each receive a fresh
// nonzero, role-specific request ID. Identities are monotonic and never
// reused, and the two roles have no interchangeable identity, so a delayed,
// duplicated, or superseded response can never mutate current state.

package result

import (
	"strconv"
	"sync/atomic"
)

// selectExecutionID and selectRequestID issue process-unique, monotonically
// increasing identities starting at 1, so zero never identifies anything.
var (
	selectExecutionID atomic.Uint64
	selectRequestID   atomic.Uint64
	writeExecutionID  atomic.Uint64
)

// NextSelectExecutionID returns the next fresh, nonzero SELECT execution ID.
func NextSelectExecutionID() uint64 { return selectExecutionID.Add(1) }

// NextWriteExecutionID returns the next fresh, nonzero actual-write execution
// ID (Issue #41). Write executions have their own monotonic identity space,
// allocated only at deliberate confirmation, so a write execution ID never
// collides with a SELECT execution, request, or preparation identity.
func NextWriteExecutionID() uint64 { return writeExecutionID.Add(1) }

// NextSelectRequestID returns the next fresh, nonzero role-specific request
// ID for one of an execution's two concurrent requests.
func NextSelectRequestID() uint64 { return selectRequestID.Add(1) }

// SelectRole identifies which of an execution's two concurrent requests a
// completion belongs to. Roles are distinct: a request ID issued for one role
// is never accepted for the other.
type SelectRole int

const (
	// RoleFirstPage is the first-page SELECT request of an execution.
	RoleFirstPage SelectRole = iota + 1
	// RoleCount is the complete-SELECT count request of an execution.
	RoleCount
)

// String renders the human-facing role name used in tests and diagnostics.
func (r SelectRole) String() string {
	switch r {
	case RoleFirstPage:
		return "first-page"
	case RoleCount:
		return "count"
	default:
		return "SelectRole(" + strconv.Itoa(int(r)) + ")"
	}
}

// SelectRequest identifies one settled completion arriving from either
// concurrent request of a SELECT execution: which execution produced it,
// which role it ran, and which request ID the role launched under.
type SelectRequest struct {
	ExecutionID uint64
	Role        SelectRole
	RequestID   uint64
}

// SelectTracker guards one SELECT execution's two concurrent completions with
// the two-level identity rule: a completion mutates active state only when
// both its execution ID and its role-specific request ID match the current
// identities, and each role is consumed at most once. Delayed responses from
// superseded executions, wrong-role IDs, and duplicate responses are all
// rejected without touching any other role. The zero value rejects every
// request because execution IDs are never zero.
type SelectTracker struct {
	executionID         uint64
	pageID, countID     uint64
	pageDone, countDone bool
}

// NewSelectTracker records the identities assigned to one fresh SELECT
// execution: one execution ID and the two distinct role request IDs.
func NewSelectTracker(executionID, pageID, countID uint64) SelectTracker {
	return SelectTracker{executionID: executionID, pageID: pageID, countID: countID}
}

// ExecutionID returns the execution identity this tracker guards. The zero
// value means no execution is tracked.
func (t *SelectTracker) ExecutionID() uint64 { return t.executionID }

// Accept reports whether req exactly matches a still-unconsumed role of this
// execution; on true it consumes that role so a duplicate is rejected later.
func (t *SelectTracker) Accept(req SelectRequest) bool {
	if req.ExecutionID != t.executionID {
		return false
	}
	switch req.Role {
	case RoleFirstPage:
		if t.pageDone || req.RequestID != t.pageID {
			return false
		}
		t.pageDone = true
		return true
	case RoleCount:
		if t.countDone || req.RequestID != t.countID {
			return false
		}
		t.countDone = true
		return true
	default:
		return false
	}
}
