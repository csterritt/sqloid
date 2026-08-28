# Issue #24 — Concurrent first page and independent result count

*2026-08-28T00:01:35Z by Showboat 0.6.1*
<!-- showboat-id: 8d2d6b10-fb42-4f18-9567-bf5235823e83 -->

Issue #24 makes one actual SELECT launch two concurrent requests — the first page and the complete-limited-result count — each on its own dedicated lease from the exact-two pool. Per Notes/tasks/024-concurrent-first-page-and-independent-result-count.md and Notes/PRD-sqloid.md (Paging consistency, Connection pool, Testing Decisions). Every step re-runs the real repository tests as evidence; later-page identities (Issue #26) are deliberately absent.

**1. Count request construction.** QueryBuilder's count wraps the complete SELECT — with the user's Limit inside the subquery — and reuses the SELECT's ordered bound parameters unchanged. The test suite pins exact SQL for no-Limit, Limit, bound WHERE values, and grouped SELECTs.

```bash
go test ./internal/querybuilder -count=1 -run 'TestCountSQL' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCountSQLCountsTheCompleteSelect
=== RUN   TestCountSQLCountsTheCompleteSelect/no_limit
=== RUN   TestCountSQLCountsTheCompleteSelect/user_limit_stays_inside_the_subquery
=== RUN   TestCountSQLCountsTheCompleteSelect/bound_where_value
=== RUN   TestCountSQLCountsTheCompleteSelect/bound_where_value_with_limit_keeps_parameter_order
--- PASS: TestCountSQLCountsTheCompleteSelect ((t))
    --- PASS: TestCountSQLCountsTheCompleteSelect/no_limit ((t))
    --- PASS: TestCountSQLCountsTheCompleteSelect/user_limit_stays_inside_the_subquery ((t))
    --- PASS: TestCountSQLCountsTheCompleteSelect/bound_where_value ((t))
    --- PASS: TestCountSQLCountsTheCompleteSelect/bound_where_value_with_limit_keeps_parameter_order ((t))
=== RUN   TestCountSQLAggregateAndGroupedSelect
--- PASS: TestCountSQLAggregateAndGroupedSelect ((t))
=== RUN   TestCountSQLRequiresSelectCommand
--- PASS: TestCountSQLRequiresSelectCommand ((t))
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
```

**2. Identity rules and exact wording.** One SELECT execution ID plus distinct nonzero page/count request IDs, no interchangeable roles, at-most-once consumption; and the exact count presentations — Result count: N, Result count: N (after Limit M), Count unavailable, Counting rows… — from explicit state only.

```bash
go test ./internal/result -count=1 -run 'TestCountState|TestSelectExecution|TestSelectTracker|TestCountStateZeroValue' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCountStateHeaderPresentation
=== RUN   TestCountStateHeaderPresentation/pending_counts_rows
=== RUN   TestCountStateHeaderPresentation/success_without_limit
=== RUN   TestCountStateHeaderPresentation/success_with_limit
=== RUN   TestCountStateHeaderPresentation/success_with_limit_and_fewer_counted_rows
=== RUN   TestCountStateHeaderPresentation/count_unavailable
--- PASS: TestCountStateHeaderPresentation ((t))
    --- PASS: TestCountStateHeaderPresentation/pending_counts_rows ((t))
    --- PASS: TestCountStateHeaderPresentation/success_without_limit ((t))
    --- PASS: TestCountStateHeaderPresentation/success_with_limit ((t))
    --- PASS: TestCountStateHeaderPresentation/success_with_limit_and_fewer_counted_rows ((t))
    --- PASS: TestCountStateHeaderPresentation/count_unavailable ((t))
=== RUN   TestCountStateZeroValueIsNotPresented
--- PASS: TestCountStateZeroValueIsNotPresented ((t))
=== RUN   TestSelectExecutionIDsAreDistinctAndMonotonic
--- PASS: TestSelectExecutionIDsAreDistinctAndMonotonic ((t))
=== RUN   TestSelectTrackerAcceptsExactIdentities
--- PASS: TestSelectTrackerAcceptsExactIdentities ((t))
=== RUN   TestSelectTrackerRejectsWrongIdentity
=== RUN   TestSelectTrackerRejectsWrongIdentity/page_completion_wearing_count's_request_ID
=== RUN   TestSelectTrackerRejectsWrongIdentity/count_completion_wearing_page's_request_ID
=== RUN   TestSelectTrackerRejectsWrongIdentity/correct_request_ID_under_a_stale_execution_ID
=== RUN   TestSelectTrackerRejectsWrongIdentity/duplicate_page_response_after_acceptance
--- PASS: TestSelectTrackerRejectsWrongIdentity ((t))
    --- PASS: TestSelectTrackerRejectsWrongIdentity/page_completion_wearing_count's_request_ID ((t))
    --- PASS: TestSelectTrackerRejectsWrongIdentity/count_completion_wearing_page's_request_ID ((t))
    --- PASS: TestSelectTrackerRejectsWrongIdentity/correct_request_ID_under_a_stale_execution_ID ((t))
    --- PASS: TestSelectTrackerRejectsWrongIdentity/duplicate_page_response_after_acceptance ((t))
=== RUN   TestSelectTrackerRejectsSwappedRoles
--- PASS: TestSelectTrackerRejectsSwappedRoles ((t))
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
```

**3. Scripted UI orchestration.** The model launches both requests in one batch carrying one execution ID and two distinct role request IDs; completions settle in either arrival order; the after-Limit wording never clamps rows; count failure leaves rows and paging usable; stale and superseded responses are discarded.

```bash
go test ./internal/ui -count=1 -run 'TestConcurrentPage|TestCountArrives|TestCountWording|TestCountUnavailable|TestFirstPageFailure|TestStaleSelect|TestDelayedCount|TestCountHelp' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestCountArrivesBeforePage
--- PASS: TestCountArrivesBeforePage ((t))
=== RUN   TestCountWordingReflectsExecutedLimit
--- PASS: TestCountWordingReflectsExecutedLimit ((t))
=== RUN   TestCountUnavailableIsIsolated
--- PASS: TestCountUnavailableIsIsolated ((t))
=== RUN   TestFirstPageFailureIndependentOfCount
--- PASS: TestFirstPageFailureIndependentOfCount ((t))
=== RUN   TestStaleSelectCompletionsAreDiscarded
--- PASS: TestStaleSelectCompletionsAreDiscarded ((t))
=== RUN   TestDelayedCountAfterNewerSelectIsIgnored
--- PASS: TestDelayedCountAfterNewerSelectIsIgnored ((t))
=== RUN   TestCountHelpRecordsIndependentSnapshots
--- PASS: TestCountHelpRecordsIndependentSnapshots ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**4. WAL and rollback-journal overlap evidence.** The mandatory capability suite holds first page and count simultaneously behind test-only barrier hooks after both acquired leases, proves two distinct physical connections with a third blocked, verifies journal mode unchanged before and after, shows independent snapshots/drift across an interleaved external writer (delayed success or the ordinary database is locked error), and proves count failure leaves a successful page untouched.

```bash
go test ./internal/connection -count=1 -run 'TestConcurrentPageAndCountOverlapDistinctLeases|TestPageAndCountLeasesAreDistinctPhysicalConnections|TestIndependentSnapshotsPermitDrift|TestRollbackJournalExternalWriterDelayOrLockError|TestCountFailureDoesNotInvalidatePageRows' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestConcurrentPageAndCountOverlapDistinctLeases
=== RUN   TestConcurrentPageAndCountOverlapDistinctLeases/delete
=== RUN   TestConcurrentPageAndCountOverlapDistinctLeases/wal
--- PASS: TestConcurrentPageAndCountOverlapDistinctLeases ((t))
    --- PASS: TestConcurrentPageAndCountOverlapDistinctLeases/delete ((t))
    --- PASS: TestConcurrentPageAndCountOverlapDistinctLeases/wal ((t))
=== RUN   TestPageAndCountLeasesAreDistinctPhysicalConnections
--- PASS: TestPageAndCountLeasesAreDistinctPhysicalConnections ((t))
=== RUN   TestIndependentSnapshotsPermitDrift
--- PASS: TestIndependentSnapshotsPermitDrift ((t))
=== RUN   TestRollbackJournalExternalWriterDelayOrLockError
--- PASS: TestRollbackJournalExternalWriterDelayOrLockError ((t))
=== RUN   TestCountFailureDoesNotInvalidatePageRows
--- PASS: TestCountFailureDoesNotInvalidatePageRows ((t))
PASS
ok  	github.com/chris/sqloid/internal/connection	(t)
```

**5. Where the behavior lives.** Count SQL in internal/querybuilder/count_sql.go; identities and wording in internal/result/select_identity.go and count.go; independent execution in internal/connection/count.go behind the shared RunRequest boundary; launch, tracker gating, and status/count rendering in internal/ui (first_select.go, count.go, model.go, results_grid.go). Full narrative: Notes/wiki/concurrent-page-count.md.
