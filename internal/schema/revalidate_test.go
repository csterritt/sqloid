// Pre-execution schema-version revalidation contract (Issue #21, Task 1):
// table-driven, UI-independent proof that an unchanged PRAGMA schema_version
// reuses the exact cached Catalog without issuing a catalog refresh, a
// changed version refreshes through the established Attempt seam, ordinary
// refresh failures carry only their cause so callers retain the prior cache,
// and terminal health classifications map to typed outcomes. Error-string
// inference is never used: every outcome is a typed RevalidateStatus.

package schema

import (
	"errors"
	"testing"
)

func TestRevalidateUnchangedVersionReusesExactCacheWithoutRefresh(t *testing.T) {
	prior := BuildCatalog(Input{
		Version: 7,
		Master:  []MasterRow{{Name: "t", Type: "table", SQL: "CREATE TABLE t (a INTEGER)"}},
	})
	priorObject := prior.Objects[0]

	refreshCalls := 0
	got := Revalidate(prior, 7, func() Attempt {
		refreshCalls++
		return NewSuccess(BuildCatalog(Input{Version: 7}))
	})

	if got.Status != RevalidateUnchanged {
		t.Errorf("Revalidate status = %v, want RevalidateUnchanged", got.Status)
	}
	if refreshCalls != 0 {
		t.Errorf("refresh issued %d times on unchanged version, want 0", refreshCalls)
	}
	if got.Catalog != prior {
		t.Error("unchanged revalidation did not return the exact cached catalog pointer")
	}
	if got.Catalog.Objects[0] != priorObject {
		t.Error("unchanged revalidation did not reuse the exact cached object metadata")
	}
	if got.Cause != nil {
		t.Errorf("unchanged revalidation cause = %v, want nil", got.Cause)
	}
}

func TestRevalidateChangedVersionRefreshesThroughSeam(t *testing.T) {
	prior := BuildCatalog(Input{Version: 3})
	refreshed := BuildCatalog(Input{
		Version: 4,
		Master:  []MasterRow{{Name: "t", Type: "table", SQL: "CREATE TABLE t (a INTEGER)"}},
	})

	refreshCalls := 0
	got := Revalidate(prior, 4, func() Attempt {
		refreshCalls++
		return NewSuccess(refreshed)
	})

	if got.Status != RevalidateRefreshed {
		t.Errorf("Revalidate status = %v, want RevalidateRefreshed", got.Status)
	}
	if refreshCalls != 1 {
		t.Errorf("refresh issued %d times on changed version, want exactly 1", refreshCalls)
	}
	if got.Catalog != refreshed {
		t.Error("refreshed revalidation did not return the refreshed catalog")
	}
	if got.Cause != nil {
		t.Errorf("refreshed revalidation cause = %v, want nil", got.Cause)
	}
}

func TestRevalidateOrdinaryRefreshFailureCarriesOnlyCause(t *testing.T) {
	prior := BuildCatalog(Input{Version: 3})
	cause := errors.New("database is locked")

	got := Revalidate(prior, 4, func() Attempt { return NewFailure(cause) })

	if got.Status != RevalidateRefreshFailed {
		t.Errorf("Revalidate status = %v, want RevalidateRefreshFailed", got.Status)
	}
	if got.Catalog != nil {
		t.Errorf("failed revalidation catalog = %v, want nil so the caller retains the prior cache", got.Catalog)
	}
	if got.Cause != cause {
		t.Errorf("failed revalidation cause = %v, want the attempt cause verbatim", got.Cause)
	}
}

func TestRevalidateTerminalHealthClassifications(t *testing.T) {
	prior := BuildCatalog(Input{Version: 3})
	cases := []struct {
		name   string
		status RefreshStatus
		want   RevalidateStatus
	}{
		{"deleted", RefreshDeleted, RevalidateDeleted},
		{"replaced", RefreshReplaced, RevalidateReplaced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Revalidate(prior, 4, func() Attempt { return NewTerminal(tc.status) })
			if got.Status != tc.want {
				t.Errorf("Revalidate status = %v, want %v", got.Status, tc.want)
			}
			if got.Catalog != nil || got.Cause != nil {
				t.Errorf("terminal revalidation carried catalog=%v cause=%v, want neither", got.Catalog, got.Cause)
			}
		})
	}
}

func TestRevalidateMalformedAttemptSettlesAsRefreshFailed(t *testing.T) {
	prior := BuildCatalog(Input{Version: 3})

	cases := []struct {
		name    string
		attempt Attempt
	}{
		{
			name:    "zero status empty payload",
			attempt: Attempt{},
		},
		{
			name: "zero status with contradictory catalog",
			attempt: Attempt{
				Status:  0,
				Catalog: BuildCatalog(Input{Version: 4}),
			},
		},
		{
			name: "zero status with contradictory cause",
			attempt: Attempt{
				Status: 0,
				Cause:  errors.New("should be ignored"),
			},
		},
		{
			name:    "unknown status empty payload",
			attempt: Attempt{Status: RefreshStatus(99)},
		},
		{
			name: "unknown status with contradictory catalog and cause",
			attempt: Attempt{
				Status:  RefreshStatus(99),
				Catalog: BuildCatalog(Input{Version: 4}),
				Cause:   errors.New("should be ignored"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refreshCalls := 0
			var got Revalidation
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Revalidate panicked on malformed attempt: %v", r)
				}
				if got.Status != RevalidateRefreshFailed {
					t.Errorf("Revalidate status = %v, want RevalidateRefreshFailed for malformed attempt", got.Status)
				}
				if got.Catalog != nil {
					t.Errorf("malformed revalidation catalog = %v, want nil so the caller retains the prior cache", got.Catalog)
				}
				if got.Cause == nil {
					t.Fatal("malformed revalidation cause = nil, want a non-nil diagnostic identifying the malformed attempt status")
				}
				if refreshCalls != 1 {
					t.Errorf("refresh issued %d times on changed version, want exactly 1", refreshCalls)
				}
			}()
			got = Revalidate(prior, 4, func() Attempt {
				refreshCalls++
				return tc.attempt
			})
		})
	}
}

// TestRevalidationValidity pins the settled payload contract for every
// Revalidation outcome, mirroring the Attempt.Valid() truth table in
// refresh_test.go: each accepted status carries exactly its required fields,
// every missing-required-field and forbidden-extra-field combination is
// rejected, and zero or unknown statuses are never valid. The matrix is the
// authoritative reference for the Revalidation.Valid() guard added in Issue
// #83 Task 2.
func TestRevalidationValidity(t *testing.T) {
	catalog := catalogWith("users")
	cause := errors.New("database is locked")
	for _, tc := range []struct {
		name         string
		revalidation Revalidation
		want         bool
	}{
		// Accepted status/payload combinations: exactly the required fields.
		{"unchanged carries the prior catalog", Revalidation{Status: RevalidateUnchanged, Catalog: catalog}, true},
		{"refreshed carries the refreshed catalog", Revalidation{Status: RevalidateRefreshed, Catalog: catalog}, true},
		{"refresh failed carries only a cause", Revalidation{Status: RevalidateRefreshFailed, Cause: cause}, true},
		{"deleted terminal carries neither", Revalidation{Status: RevalidateDeleted}, true},
		{"replaced terminal carries neither", Revalidation{Status: RevalidateReplaced}, true},

		// Missing required fields on accepted statuses.
		{"unchanged without a catalog is inconsistent", Revalidation{Status: RevalidateUnchanged}, false},
		{"refreshed without a catalog is inconsistent", Revalidation{Status: RevalidateRefreshed}, false},
		{"refresh failed without a cause is inconsistent", Revalidation{Status: RevalidateRefreshFailed}, false},

		// Forbidden extra fields on accepted statuses.
		{"unchanged carrying a cause is inconsistent", Revalidation{Status: RevalidateUnchanged, Catalog: catalog, Cause: cause}, false},
		{"refreshed carrying a cause is inconsistent", Revalidation{Status: RevalidateRefreshed, Catalog: catalog, Cause: cause}, false},
		{"refresh failed carrying a catalog would leak partial replacement", Revalidation{Status: RevalidateRefreshFailed, Catalog: catalog, Cause: cause}, false},
		{"deleted carrying a catalog is inconsistent", Revalidation{Status: RevalidateDeleted, Catalog: catalog}, false},
		{"deleted carrying a cause is inconsistent", Revalidation{Status: RevalidateDeleted, Cause: cause}, false},
		{"replaced carrying a catalog is inconsistent", Revalidation{Status: RevalidateReplaced, Catalog: catalog}, false},
		{"replaced carrying a cause is inconsistent", Revalidation{Status: RevalidateReplaced, Cause: cause}, false},

		// Zero and unknown statuses are never settled outcomes.
		{"unsettled zero status is not a valid outcome", Revalidation{}, false},
		{"unknown status is not a valid outcome", Revalidation{Status: RevalidateStatus(99)}, false},
		{"unknown status with a catalog is not a valid outcome", Revalidation{Status: RevalidateStatus(99), Catalog: catalog}, false},
		{"unknown status with a cause is not a valid outcome", Revalidation{Status: RevalidateStatus(99), Cause: cause}, false},
	} {
		if got := tc.revalidation.Valid(); got != tc.want {
			t.Errorf("%s: Valid()=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRevalidateProductionOutputsAreValid requires every Revalidate path —
// unchanged, changed-success, ordinary failure, terminal deletion/replacement,
// and the Issue #82 malformed-attempt defensive fallback — to settle into a
// Revalidation accepted by Valid(). This is the production-side half of the
// Issue #83 invariant: no Revalidate output may escape the seam in an
// unsettled or contradictory payload state.
func TestRevalidateProductionOutputsAreValid(t *testing.T) {
	prior := BuildCatalog(Input{Version: 3})
	refreshed := BuildCatalog(Input{
		Version: 4,
		Master:  []MasterRow{{Name: "t", Type: "table", SQL: "CREATE TABLE t (a INTEGER)"}},
	})
	cause := errors.New("database is locked")

	for _, tc := range []struct {
		name    string
		refresh func() Attempt
		version int64
	}{
		{"unchanged reuses prior cache", func() Attempt {
			t.Fatal("refresh must not be invoked on unchanged version")
			return Attempt{}
		}, 3},
		{"changed success installs refreshed catalog", func() Attempt { return NewSuccess(refreshed) }, 4},
		{"ordinary failure carries only cause", func() Attempt { return NewFailure(cause) }, 4},
		{"terminal deletion carries neither", func() Attempt { return NewTerminal(RefreshDeleted) }, 4},
		{"terminal replacement carries neither", func() Attempt { return NewTerminal(RefreshReplaced) }, 4},
		// Issue #82 malformed-attempt defensive fallbacks all settle as a
		// valid refresh-failed value with a non-nil diagnostic cause.
		{"malformed zero-status attempt settles valid", func() Attempt { return Attempt{} }, 4},
		{"malformed zero-status attempt with stray catalog settles valid", func() Attempt {
			return Attempt{Status: 0, Catalog: refreshed}
		}, 4},
		{"malformed zero-status attempt with stray cause settles valid", func() Attempt {
			return Attempt{Status: 0, Cause: errors.New("should be ignored")}
		}, 4},
		{"malformed unknown-status attempt settles valid", func() Attempt {
			return Attempt{Status: RefreshStatus(99)}
		}, 4},
		{"malformed unknown-status attempt with stray payload settles valid", func() Attempt {
			return Attempt{Status: RefreshStatus(99), Catalog: refreshed, Cause: errors.New("ignored")}
		}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Revalidate(prior, tc.version, tc.refresh)
			if !got.Valid() {
				t.Errorf("Revalidate produced invalid Revalidation %+v for %s", got, tc.name)
			}
		})
	}
}

func TestRevalidateStatusString(t *testing.T) {
	cases := []struct {
		status RevalidateStatus
		want   string
	}{
		{RevalidateUnchanged, "unchanged"},
		{RevalidateRefreshed, "refreshed"},
		{RevalidateRefreshFailed, "refresh-failed"},
		{RevalidateDeleted, "deleted"},
		{RevalidateReplaced, "replaced"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("RevalidateStatus(%d).String() = %q, want %q", int(tc.status), got, tc.want)
		}
	}
}
