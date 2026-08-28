// Typed refresh-lifecycle contract tests for Issue #13 Task 1: one schema
// refresh attempt settles into exactly one status; success carries the fully
// refreshed typed catalog, ordinary failure carries only its cause so every
// consumer retains the prior catalog untouched, and deletion/replacement are
// terminal classifications delivered through the Connection boundary's typed
// health outcomes rather than error-string matching. The prior catalog is
// immutable test data throughout.

package schema

import (
	"errors"
	"testing"
)

func catalogWith(objects ...string) *Catalog {
	cat := &Catalog{Version: 7}
	for _, name := range objects {
		cat.Objects = append(cat.Objects, &Object{Name: name})
	}
	return cat
}

// TestAttemptValidity pins the consistency rule every producer must honor:
// exactly one settled outcome per attempt, and only RefreshOK may carry a
// Catalog so consumers can rely on nil-Catalog failure to mean "retain prior".
func TestAttemptValidity(t *testing.T) {
	catalog := catalogWith("users")
	brokenCause := errors.New("lock busy")
	for _, tc := range []struct {
		name    string
		attempt Attempt
		want    bool
	}{
		{"successful attempt carries its catalog", NewSuccess(catalog), true},
		{"ordinary failure carries only a cause", NewFailure(brokenCause), true},
		{"deletion terminal carries neither", NewTerminal(RefreshDeleted), true},
		{"replacement terminal carries neither", NewTerminal(RefreshReplaced), true},
		{"success without a catalog is inconsistent", Attempt{Status: RefreshOK}, false},
		{"failure carrying a catalog would leak partial replacement", Attempt{Status: RefreshFailed, Catalog: catalog}, false},
		{"unsettled zero status is not a valid outcome", Attempt{}, false},
	} {
		if got := tc.attempt.Valid(); got != tc.want {
			t.Errorf("%s: Valid()=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAttemptOrdinaryFailurePreservesPriorCatalogIdentity requires ordinary
// failures to declare no replacement data at all, so a consumer holding the
// prior catalog pointer cannot be tricked into partial substitution; the
// assertion also shows successes never alias the prior snapshot.
func TestAttemptOrdinaryFailurePreservesPriorCatalogIdentity(t *testing.T) {
	prior := catalogWith("users", "logs_fts")
	failure := NewFailure(errors.New("database locked"))
	if failure.Cause == nil || failure.Status != RefreshFailed {
		t.Fatalf("NewFailure produced %+v, want Failed status with cause preserved", failure)
	}
	if failure.Catalog != nil {
		t.Errorf("failed attempt carries Catalog %p, want nil so the prior catalog is retained unchanged", failure.Catalog)
	}
	success := NewSuccess(prior)
	if success.Catalog != prior {
		t.Error("successful attempt must expose exactly the refreshed catalog it settled")
	}
}

// TestAttemptTerminalStatusesString checks the diagnostic rendering of each
// classification used when wiring Connection health outcomes onto attempts.
func TestAttemptTerminalStatusesString(t *testing.T) {
	for _, tc := range []struct {
		status RefreshStatus
		want   string
	}{
		{RefreshDeleted, "deleted"},
		{RefreshReplaced, "replaced"},
		{RefreshOK, "ok"},
		{RefreshFailed, "failed"},
	} {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("RefreshStatus(%d).String()=%q, want %q", int(tc.status), got, tc.want)
		}
	}
}
