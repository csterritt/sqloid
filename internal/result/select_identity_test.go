package result

import "testing"

// TestSelectExecutionIDsAreDistinctAndMonotonic covers the Issue #24 identity
// rule: every SELECT execution receives one fresh nonzero execution ID and
// every role-specific request a fresh nonzero request ID; identities are
// never reused or interchangeable across executions or roles.
func TestSelectExecutionIDsAreDistinctAndMonotonic(t *testing.T) {
	firstExec := NextSelectExecutionID()
	if firstExec == 0 {
		t.Fatal("NextSelectExecutionID() = 0, want a nonzero identity")
	}
	secondExec := NextSelectExecutionID()
	if secondExec == 0 || secondExec == firstExec {
		t.Fatalf("execution IDs %d and %d are not distinct nonzero identities", firstExec, secondExec)
	}
	if secondExec <= firstExec {
		t.Errorf("execution IDs are not monotonic: %d then %d", firstExec, secondExec)
	}

	firstPage := NextSelectRequestID()
	firstCount := NextSelectRequestID()
	if firstPage == 0 || firstCount == 0 {
		t.Fatal("request IDs must be nonzero")
	}
	if firstPage == firstCount {
		t.Fatalf("page and count request IDs are not distinct: both %d", firstPage)
	}
	if NextSelectRequestID() == firstPage || NextSelectRequestID() == firstCount {
		t.Error("request IDs were reused")
	}
}

// TestSelectTrackerAcceptsExactIdentities covers the two-level identity
// guard: a completion mutates active state only when both its SELECT
// execution ID and its role-specific request ID match, and each role is
// consumed at most once.
func TestSelectTrackerAcceptsExactIdentities(t *testing.T) {
	exec := NextSelectExecutionID()
	pageID := NextSelectRequestID()
	countID := NextSelectRequestID()
	tracker := NewSelectTracker(exec, pageID, countID)

	if !tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleFirstPage, RequestID: pageID}) {
		t.Fatal("exact page identity rejected")
	}
	if !tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleCount, RequestID: countID}) {
		t.Fatal("exact count identity rejected")
	}
}

// TestSelectTrackerRejectsWrongIdentity covers wrong-role IDs, duplicated
// responses, and delayed responses from superseded executions.
func TestSelectTrackerRejectsWrongIdentity(t *testing.T) {
	exec := NextSelectExecutionID()
	pageID := NextSelectRequestID()
	countID := NextSelectRequestID()
	newerExec := NextSelectExecutionID()

	tests := []struct {
		name     string
		tracker  SelectTracker
		request  SelectRequest
		settled  bool // a prior same-role completion already consumed
		accepted bool
	}{
		{
			name:     "page completion wearing count's request ID",
			tracker:  NewSelectTracker(exec, pageID, countID),
			request:  SelectRequest{ExecutionID: exec, Role: RoleFirstPage, RequestID: countID},
			accepted: false,
		},
		{
			name:     "count completion wearing page's request ID",
			tracker:  NewSelectTracker(exec, pageID, countID),
			request:  SelectRequest{ExecutionID: exec, Role: RoleCount, RequestID: pageID},
			accepted: false,
		},
		{
			name:     "correct request ID under a stale execution ID",
			tracker:  NewSelectTracker(exec, pageID, countID),
			request:  SelectRequest{ExecutionID: newerExec, Role: RoleCount, RequestID: countID},
			accepted: false,
		},
		{
			name:     "duplicate page response after acceptance",
			tracker:  NewSelectTracker(exec, pageID, countID),
			request:  SelectRequest{ExecutionID: exec, Role: RoleFirstPage, RequestID: pageID},
			settled:  true,
			accepted: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.settled {
				if !tc.tracker.Accept(tc.request) {
					t.Fatalf("setup: first acceptance rejected")
				}
			}
			if got := tc.tracker.Accept(tc.request); got != tc.accepted {
				t.Errorf("Accept(%+v) = %v, want %v", tc.request, got, tc.accepted)
			}
		})
	}
}

// TestSelectTrackerRejectsSwappedRoles covers that the two roles have no
// interchangeable identity: consuming the page role must not consume count.
func TestSelectTrackerRejectsSwappedRoles(t *testing.T) {
	exec := NextSelectExecutionID()
	pageID := NextSelectRequestID()
	countID := NextSelectRequestID()
	tracker := NewSelectTracker(exec, pageID, countID)

	if tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleCount, RequestID: pageID}) {
		t.Fatal("page request ID accepted for the count role")
	}
	if tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleFirstPage, RequestID: countID}) {
		t.Fatal("count request ID accepted for the page role")
	}
	if !tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleFirstPage, RequestID: pageID}) {
		t.Fatal("exact page identity rejected after rejected swaps")
	}
	if !tracker.Accept(SelectRequest{ExecutionID: exec, Role: RoleCount, RequestID: countID}) {
		t.Fatal("exact count identity rejected after rejected swaps")
	}
}
