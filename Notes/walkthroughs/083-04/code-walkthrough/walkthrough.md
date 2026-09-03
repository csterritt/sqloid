# Issue #083 Code Walkthrough: Revalidation Payload Invariant

*2026-09-03T22:15:42Z by Showboat 0.6.1*
<!-- showboat-id: fe1d99e0-2a02-470d-bef6-f7519743729c -->

Issue #83 (Notes/tasks/083-add-revalidation-valid-invariant.md, Notes/PRD-sqloid.md Schema scope, cache, and validation and Testing Decisions) adds the authoritative `Revalidation.Valid()` invariant guard to `internal/schema/revalidate.go`, mirroring `Attempt.Valid()` in `internal/schema/refresh.go`. The invariant encodes the exact settled payload contract that every `Revalidation` already obeys: each accepted status carries exactly its required fields, so consumers can rely on a nil-Catalog refresh-failed outcome alone meaning "retain the prior cache". Issue #82 finalized the malformed-attempt defensive fallback (zero/unknown `Attempt.Status` settles as `RevalidateRefreshFailed` with a non-nil diagnostic cause and nil catalog); Issue #83 now requires every `Revalidate` output — unchanged, changed-success, ordinary failure, terminal deletion/replacement, and the Issue #82 malformed-attempt fallback — to satisfy the invariant. The invariant adds no constructor, status value, `Revalidate` mapping, catalog identity, cause preservation, or UI behavior: it only codifies the contract. Reference: Issues #82–#83 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/083-04/code-walkthrough/.

## The `Revalidation.Valid()` guard

The new method sits beside the `Revalidation` type and mirrors the explicit status switch and exact field checks of `Attempt.Valid()`. Each accepted status carries exactly its required fields; zero or unknown statuses return false; missing required fields and forbidden extra fields are rejected.

```bash
sed -n '/\/\/ Valid reports whether the settled revalidation/,/^}/p' internal/schema/revalidate.go
```

```output
// Valid reports whether the settled revalidation obeys the payload rules
// above: each status carries exactly its required fields, which lets consumers
// rely on a nil-Catalog refresh-failed outcome alone meaning "retain the prior
// cache". The matrix mirrors Attempt.Valid() and rejects zero or unknown
// statuses, missing required fields, and any contradictory extra payload.
func (r Revalidation) Valid() bool {
	switch r.Status {
	case RevalidateUnchanged, RevalidateRefreshed:
		return r.Catalog != nil && r.Cause == nil
	case RevalidateRefreshFailed:
		return r.Cause != nil && r.Catalog == nil
	case RevalidateDeleted, RevalidateReplaced:
		return r.Catalog == nil && r.Cause == nil
	default:
		return false
	}
}
```

The matrix is the authoritative reference: `RevalidateUnchanged` and `RevalidateRefreshed` accept exactly a non-nil `Catalog` with nil `Cause`; `RevalidateRefreshFailed` accepts exactly a non-nil `Cause` with nil `Catalog`; `RevalidateDeleted` and `RevalidateReplaced` accept only nil `Catalog` and nil `Cause`; the zero `RevalidateStatus` and any unknown out-of-range value fall through to the default branch and return false.

## The `Attempt.Valid()` mirror

The structure mirrors `Attempt.Valid()` in `internal/schema/refresh.go` — same explicit status switch, same exact field checks — so the two typed payloads share one payload-discipline shape across the refresh and revalidation seams.

```bash
sed -n '/\/\/ Valid reports whether the settled attempt/,/^}/p' internal/schema/refresh.go
```

```output
// Valid reports whether the settled attempt obeys the payload rules above:
// each status carries exactly its required fields, which lets consumers rely
// on a nil-Catalog failure alone meaning "retain the prior catalog".
func (a Attempt) Valid() bool {
	switch a.Status {
	case RefreshOK:
		return a.Catalog != nil && a.Cause == nil
	case RefreshFailed:
		return a.Cause != nil && a.Catalog == nil
	case RefreshDeleted, RefreshReplaced:
		return a.Catalog == nil && a.Cause == nil
	default:
		return false
	}
}
```

## The truth table — `TestRevalidationValidity`

The truth table in `internal/schema/revalidate_test.go` pins every valid and invalid `Revalidation` status/payload combination. The valid cases are the five accepted statuses carrying exactly their required fields; the invalid cases cover every missing-required-field and forbidden-extra-field combination plus the zero and unknown statuses.

```bash
sed -n '/func TestRevalidationValidity/,/^}/p' internal/schema/revalidate_test.go
```

```output
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
```

The valid rows are the five accepted statuses with exactly their required payload: unchanged and refreshed each carry a non-nil catalog with nil cause; refresh-failed carries a non-nil cause with nil catalog; deleted and replaced carry neither. The invalid rows cover every missing-required case (unchanged/refreshed without a catalog, refresh-failed without a cause), every forbidden-extra case (a cause on unchanged/refreshed, a catalog on refresh-failed, either field on the terminal statuses), and the zero and unknown statuses — including unknown statuses carrying a stray catalog or cause — which all return false regardless of payload.

## Production-output assertions — `TestRevalidateProductionOutputsAreValid`

The production-side half of the invariant requires every `Revalidate` path to settle into a `Revalidation` accepted by `Valid()`. This is the proof that no `Revalidate` output can escape the seam in an unsettled or contradictory payload state — including the Issue #82 malformed-attempt defensive fallback.

```bash
sed -n '/func TestRevalidateProductionOutputsAreValid/,/^}/p' internal/schema/revalidate_test.go
```

```output
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
```

The cases cover every `Revalidate` path: unchanged (refresh never invoked, prior cache reused), changed-success (refreshed catalog installed), ordinary failure (cause carried verbatim, no catalog), terminal deletion and replacement (neither field set), and the five Issue #82 malformed-attempt variants — zero status with empty payload, zero status with a stray catalog, zero status with a stray cause, unknown status with empty payload, and unknown status with both stray fields. Each malformed variant defensively settles as `RevalidateRefreshFailed` with a non-nil diagnostic cause and nil catalog, which `Valid()` accepts.

## Running the invariant suite

Run the focused schema tests to prove the truth table and production-output assertions both pass green.

```bash
go test ./internal/schema/ -run '^TestRevalidationValidity$|^TestRevalidateProductionOutputsAreValid$' -v -count=1
```

```output
=== RUN   TestRevalidationValidity
--- PASS: TestRevalidationValidity (0.00s)
=== RUN   TestRevalidateProductionOutputsAreValid
=== RUN   TestRevalidateProductionOutputsAreValid/unchanged_reuses_prior_cache
=== RUN   TestRevalidateProductionOutputsAreValid/changed_success_installs_refreshed_catalog
=== RUN   TestRevalidateProductionOutputsAreValid/ordinary_failure_carries_only_cause
=== RUN   TestRevalidateProductionOutputsAreValid/terminal_deletion_carries_neither
=== RUN   TestRevalidateProductionOutputsAreValid/terminal_replacement_carries_neither
=== RUN   TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_settles_valid
=== RUN   TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_with_stray_catalog_settles_valid
=== RUN   TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_with_stray_cause_settles_valid
=== RUN   TestRevalidateProductionOutputsAreValid/malformed_unknown-status_attempt_settles_valid
=== RUN   TestRevalidateProductionOutputsAreValid/malformed_unknown-status_attempt_with_stray_payload_settles_valid
--- PASS: TestRevalidateProductionOutputsAreValid (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/unchanged_reuses_prior_cache (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/changed_success_installs_refreshed_catalog (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/ordinary_failure_carries_only_cause (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/terminal_deletion_carries_neither (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/terminal_replacement_carries_neither (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_settles_valid (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_with_stray_catalog_settles_valid (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/malformed_zero-status_attempt_with_stray_cause_settles_valid (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/malformed_unknown-status_attempt_settles_valid (0.00s)
    --- PASS: TestRevalidateProductionOutputsAreValid/malformed_unknown-status_attempt_with_stray_payload_settles_valid (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.002s
```

Both tests pass: the truth table accepts every valid combination and rejects every invalid one, and every `Revalidate` production path — including all five Issue #82 malformed-attempt fallbacks — settles into a `Revalidation` accepted by `Valid()`.

## Repository-wide verification

Run the full Go verification suite required by the project's coding standards: `gofmt`, `go vet`, `go test ./...`, and `go build ./...`.

```bash
gofmt -d internal/schema/revalidate.go internal/schema/revalidate_test.go && go vet ./... && go test ./... && go build ./... && echo ALL GREEN
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/070-06/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
ALL GREEN
```

All four verification steps pass: `gofmt -d` reports no formatting differences, `go vet ./...` is clean, `go test ./...` is green across every package, and `go build ./...` succeeds. The Issue #83 invariant is complete and verified.
