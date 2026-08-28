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
