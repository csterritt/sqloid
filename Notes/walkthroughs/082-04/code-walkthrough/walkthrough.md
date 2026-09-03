# Issue #082 Code Walkthrough: Settle Malformed Schema Revalidation Attempts

*2026-09-03T19:32:02Z by Showboat 0.6.1*
<!-- showboat-id: 0e4b8ef4-3177-4652-99a9-cc11d361ded8 -->

Issue #82 (Notes/tasks/082-settle-malformed-schema-revalidation.md, Notes/PRD-sqloid.md Schema scope, cache, and validation decisions) closes the defensive gap in `internal/schema/revalidate.go`: the default branch of `Revalidate` previously returned an empty `Revalidation{}`, leaking an unset `RevalidateStatus(0)` to consumers when an `Attempt` carried a zero or out-of-range `RefreshStatus`. Such attempts are never produced by the constructors in `internal/schema/refresh.go`, but the defensive branch must still settle to an actionable typed outcome. Issue #82 finalizes the complete Attempt-to-Revalidation status map so every revalidation path returns a settled status, and zero/unknown attempt statuses defensively settle as `RevalidateRefreshFailed` with a concrete diagnostic cause and no catalog rather than leaking an unset status to consumers. This keeps the result actionable under the existing stale-refresh workflow (the prior cache stands behind the diagnostic cause) and pushes no malformed-state handling into UI consumers. The forthcoming Issue #83 will encode this finalized mapping as an invariant. Reference: Issue #82 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/082-04/code-walkthrough/.

```bash
sed -n '/\/\/ Revalidate compares the database/,/^}/p' internal/schema/revalidate.go
```

```output
// Revalidate compares the database's current schema version against the
// cached catalog. An equal version reuses prior — the exact cached pointer —
// without ever invoking refresh. A changed version refreshes through the
// established Attempt seam and maps its settled classification onto the
// typed revalidation outcome. Prior must be the caller's retained cache;
// refresh is invoked at most once and only on a changed version.
func Revalidate(prior *Catalog, currentVersion int64, refresh func() Attempt) Revalidation {
	if prior != nil && currentVersion == prior.Version {
		return Revalidation{Status: RevalidateUnchanged, Catalog: prior}
	}
	att := refresh()
	switch att.Status {
	case RefreshOK:
		return Revalidation{Status: RevalidateRefreshed, Catalog: att.Catalog}
	case RefreshFailed:
		return Revalidation{Status: RevalidateRefreshFailed, Cause: att.Cause}
	case RefreshDeleted:
		return Revalidation{Status: RevalidateDeleted}
	case RefreshReplaced:
		return Revalidation{Status: RevalidateReplaced}
	default:
		// Defensive against an unsettled or out-of-range Attempt payload;
		// never produced by the constructors in refresh.go. Settle as an
		// ordinary refresh failure with a concrete diagnostic cause and no
		// catalog so the stale-refresh workflow retains the prior cache
		// rather than leaking an unset status to consumers (Issue #82).
		return Revalidation{
			Status: RevalidateRefreshFailed,
			Cause:  fmt.Errorf("schema revalidate: unsettled refresh attempt status %s", att.Status),
		}
	}
}
```

The complete Attempt-to-Revalidation status map is now visible in the switch:

- `RefreshOK` → `RevalidateRefreshed` carrying `att.Catalog` (the refreshed snapshot installs as authoritative).
- `RefreshFailed` → `RevalidateRefreshFailed` carrying `att.Cause` verbatim (the prior cache stands; the cause is never reinterpreted).
- `RefreshDeleted` → `RevalidateDeleted` (terminal; neither catalog nor cause).
- `RefreshReplaced` → `RevalidateReplaced` (terminal; neither catalog nor cause).
- zero or unknown `Attempt.Status` (the `default` branch) → `RevalidateRefreshFailed` with a non-nil diagnostic `Cause` of the form `schema revalidate: unsettled refresh attempt status <status>` and a nil `Catalog`.

The diagnostic cause uses the existing `RefreshStatus.String()` convention from `internal/schema/refresh.go`, which renders zero as `RefreshStatus(0)` and any out-of-range value as `RefreshStatus(N)`. The malformed attempt's contradictory payload fields (a stray `Catalog` or `Cause`) are ignored — the default status mapping is authoritative — so the result is always actionable under the existing stale-refresh workflow and no malformed-state handling is pushed into UI consumers.

```bash
go test ./internal/schema/ -run '^TestRevalidateMalformedAttemptSettlesAsRefreshFailed$' -v -count=1 | sed 's/ok  	github.com\/chris\/sqloid\/internal\/schema	[0-9.]*s/ok  	github.com\/chris\/sqloid\/internal\/schema	0.00s/'
```

```output
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_empty_payload
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_catalog
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_cause
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_empty_payload
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_with_contradictory_catalog_and_cause
--- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_empty_payload (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_catalog (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_cause (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_empty_payload (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_with_contradictory_catalog_and_cause (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.00s
```

The five malformed-status regression cases all pass. Each sends a changed-version `Revalidate` call an `Attempt` whose status is zero or an out-of-range `RefreshStatus(99)`, including contradictory payload fields (a stray `Catalog`, a stray `Cause`, or both). Every case requires `RevalidateRefreshFailed`, a non-nil diagnostic cause that identifies the malformed or unknown attempt status, a nil catalog, exactly one refresh invocation, and no panic — proving the default status mapping is authoritative and the contradictory fields are ignored.

```bash
sed -n '/func TestRevalidateMalformedAttemptSettlesAsRefreshFailed/,/^}/p' internal/schema/revalidate_test.go
```

```output
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
```

```bash
go test ./internal/schema/ -run '^TestRevalidate' -v -count=1 | sed 's/	[0-9.]*s$/	0.00s/; s/ok  	github.com\/chris\/sqloid\/internal\/schema	[0-9.]*s/ok  	github.com\/chris\/sqloid\/internal\/schema	0.00s/'
```

```output
=== RUN   TestRevalidateUnchangedVersionReusesExactCacheWithoutRefresh
--- PASS: TestRevalidateUnchangedVersionReusesExactCacheWithoutRefresh (0.00s)
=== RUN   TestRevalidateChangedVersionRefreshesThroughSeam
--- PASS: TestRevalidateChangedVersionRefreshesThroughSeam (0.00s)
=== RUN   TestRevalidateOrdinaryRefreshFailureCarriesOnlyCause
--- PASS: TestRevalidateOrdinaryRefreshFailureCarriesOnlyCause (0.00s)
=== RUN   TestRevalidateTerminalHealthClassifications
=== RUN   TestRevalidateTerminalHealthClassifications/deleted
=== RUN   TestRevalidateTerminalHealthClassifications/replaced
--- PASS: TestRevalidateTerminalHealthClassifications (0.00s)
    --- PASS: TestRevalidateTerminalHealthClassifications/deleted (0.00s)
    --- PASS: TestRevalidateTerminalHealthClassifications/replaced (0.00s)
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_empty_payload
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_catalog
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_cause
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_empty_payload
=== RUN   TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_with_contradictory_catalog_and_cause
--- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_empty_payload (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_catalog (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/zero_status_with_contradictory_cause (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_empty_payload (0.00s)
    --- PASS: TestRevalidateMalformedAttemptSettlesAsRefreshFailed/unknown_status_with_contradictory_catalog_and_cause (0.00s)
=== RUN   TestRevalidateStatusString
--- PASS: TestRevalidateStatusString (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.00s
```

The constructor-produced mappings stay unchanged: unchanged-version reuse of the exact cached pointer without invoking refresh, changed-version refresh installing the new catalog, ordinary `RefreshFailed` carrying only the verbatim cause with the prior cache standing, terminal deleted/replaced kinds carrying neither catalog nor cause, and the human-readable `RevalidateStatus.String()` tokens all pass exactly as before Issue #82. The new malformed-status cases pass alongside them, so every revalidation path now returns an actionable settled status and consumers never receive an empty or unknown revalidation status.

```bash
go test ./internal/schema/... -count=1 | sed 's/ok  	github.com\/chris\/sqloid\/internal\/schema	[0-9.]*s/ok  	github.com\/chris\/sqloid\/internal\/schema	0.00s/'
```

```output
ok  	github.com/chris/sqloid/internal/schema	0.00s
```

```bash
go vet ./... && go build ./...
```

```output
```

Focused schema verification passes (`go test ./internal/schema/...`), and the module-wide `go vet ./...` and `go build ./...` are clean. The defensive settlement is the only behavioral change: ordinary `RefreshFailed` causes are preserved verbatim, successful catalog pointers and terminal deletion/replacement payloads are unchanged, and no malformed-state handling is pushed into UI consumers. Consumers never receive an empty or unknown revalidation status because the `default` branch now settles to `RevalidateRefreshFailed` with a concrete diagnostic cause and a nil catalog — the same actionable shape an ordinary refresh failure produces, so the existing stale-refresh workflow (prior cache retained behind `could not refresh: <cause>` reporting) applies without modification. Reference: Issue #82 and Notes/PRD-sqloid.md.

