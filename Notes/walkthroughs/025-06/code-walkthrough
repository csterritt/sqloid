# Issue #25 — Serialized vertical result paging

*2026-08-28T01:07:21Z by Showboat 0.6.1*
<!-- showboat-id: 6ae8ecfa-ebeb-4e87-9aa2-040102a087ec -->

Issue #25 makes adjacent Page Up/Down navigation real: each key requests exactly one adjacent absolute logical range through QueryBuilder's page API, at most one page request is ever pending, and the page size equals all complete visible data rows. Per Notes/tasks/025-serialized-vertical-result-paging.md and Notes/PRD-sqloid.md (Paging consistency, Grid rendering/cache, Resize/layout, Testing Decisions); cross-references Issues #22 and #24. Every step re-runs the real repository tests as evidence.

**1. Page SQL construction over a large fixture: exact ranges and the eligible rowid fallback.** The table-driven QueryBuilder suite pins the exact LIMIT/OFFSET ranges with the page limit clamped to the remaining user Limit, ordered bound parameters preserved, and the implicit `ORDER BY rowid` fallback applied only to the one eligible case — an ordinary rowid table with no declared rowid, _rowid_, or oid shadow, no user ORDER BY, and no aggregate/group shape.

```bash
go test ./internal/querybuilder -count=1 -run 'TestPageSQL' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestPageSQLEligibleFallbackRanges
=== RUN   TestPageSQLEligibleFallbackRanges/first_page_appends_rowid_fallback
=== RUN   TestPageSQLEligibleFallbackRanges/adjacent_page_exact_range
=== RUN   TestPageSQLEligibleFallbackRanges/where_params_preserved_in_order
=== RUN   TestPageSQLEligibleFallbackRanges/page_limit_clamped_to_remaining_user_limit
=== RUN   TestPageSQLEligibleFallbackRanges/offset_beyond_user_limit_yields_no_statement
=== RUN   TestPageSQLEligibleFallbackRanges/unbounded_logical_result_keeps_page_limit
=== RUN   TestPageSQLEligibleFallbackRanges/page_limit_below_remaining_user_limit
--- PASS: TestPageSQLEligibleFallbackRanges ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/first_page_appends_rowid_fallback ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/adjacent_page_exact_range ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/where_params_preserved_in_order ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/page_limit_clamped_to_remaining_user_limit ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/offset_beyond_user_limit_yields_no_statement ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/unbounded_logical_result_keeps_page_limit ((t))
    --- PASS: TestPageSQLEligibleFallbackRanges/page_limit_below_remaining_user_limit ((t))
=== RUN   TestPageSQLDefaultRangeValues
--- PASS: TestPageSQLDefaultRangeValues ((t))
=== RUN   TestPageSQLInvalidRangesRefused
=== RUN   TestPageSQLInvalidRangesRefused/zero_page_limit
=== RUN   TestPageSQLInvalidRangesRefused/negative_page_limit
=== RUN   TestPageSQLInvalidRangesRefused/negative_offset
--- PASS: TestPageSQLInvalidRangesRefused ((t))
    --- PASS: TestPageSQLInvalidRangesRefused/zero_page_limit ((t))
    --- PASS: TestPageSQLInvalidRangesRefused/negative_page_limit ((t))
    --- PASS: TestPageSQLInvalidRangesRefused/negative_offset ((t))
=== RUN   TestPageSQLUserOrderByPreservedWithoutRowid
=== RUN   TestPageSQLUserOrderByPreservedWithoutRowid/ascending_keeps_exact_expression
=== RUN   TestPageSQLUserOrderByPreservedWithoutRowid/descending_keeps_exact_direction
--- PASS: TestPageSQLUserOrderByPreservedWithoutRowid ((t))
    --- PASS: TestPageSQLUserOrderByPreservedWithoutRowid/ascending_keeps_exact_expression ((t))
    --- PASS: TestPageSQLUserOrderByPreservedWithoutRowid/descending_keeps_exact_direction ((t))
=== RUN   TestPageSQLAggregateGroupedOrderByPreserved
=== RUN   TestPageSQLAggregateGroupedOrderByPreserved/grouped_aggregate_ascending
=== RUN   TestPageSQLAggregateGroupedOrderByPreserved/grouped_aggregate_descending
--- PASS: TestPageSQLAggregateGroupedOrderByPreserved ((t))
    --- PASS: TestPageSQLAggregateGroupedOrderByPreserved/grouped_aggregate_ascending ((t))
    --- PASS: TestPageSQLAggregateGroupedOrderByPreserved/grouped_aggregate_descending ((t))
=== RUN   TestPageSQLAggregateGroupedNoFallback
--- PASS: TestPageSQLAggregateGroupedNoFallback ((t))
=== RUN   TestPageSQLExcludedObjectsNoFallback
=== RUN   TestPageSQLExcludedObjectsNoFallback/view_is_unordered
=== RUN   TestPageSQLExcludedObjectsNoFallback/virtual_table_is_unordered
=== RUN   TestPageSQLExcludedObjectsNoFallback/WITHOUT_ROWID_table_is_unordered
=== RUN   TestPageSQLExcludedObjectsNoFallback/rowid_alias_shadow_is_unordered
=== RUN   TestPageSQLExcludedObjectsNoFallback/_rowid__alias_shadow_is_unordered
=== RUN   TestPageSQLExcludedObjectsNoFallback/oid_alias_shadow_is_unordered
--- PASS: TestPageSQLExcludedObjectsNoFallback ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/view_is_unordered ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/virtual_table_is_unordered ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/WITHOUT_ROWID_table_is_unordered ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/rowid_alias_shadow_is_unordered ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/_rowid__alias_shadow_is_unordered ((t))
    --- PASS: TestPageSQLExcludedObjectsNoFallback/oid_alias_shadow_is_unordered ((t))
=== RUN   TestPageSQLIneligibleShapesRefused
=== RUN   TestPageSQLIneligibleShapesRefused/unselected_command
=== RUN   TestPageSQLIneligibleShapesRefused/update_command_over_eligible_table
=== RUN   TestPageSQLIneligibleShapesRefused/select_with_no_table_selected
--- PASS: TestPageSQLIneligibleShapesRefused ((t))
    --- PASS: TestPageSQLIneligibleShapesRefused/unselected_command ((t))
    --- PASS: TestPageSQLIneligibleShapesRefused/update_command_over_eligible_table ((t))
    --- PASS: TestPageSQLIneligibleShapesRefused/select_with_no_table_selected ((t))
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
```

**2. Adjacent paging over a 100-row fixture, boundaries, and the user's Limit.** The scripted UI model tests press Page Down and Page Up on an idle active SELECT and prove the request carries exactly the adjacent absolute range (`LIMIT 11 OFFSET 3` for a 3-row displayed page at 80×24), stops at the known low/high boundaries, and never reads past the user's Limit.

```bash
go test ./internal/ui -count=1 -run 'TestPageDownRequestsAdjacentRange|TestPageUpSuppressedAtLowBoundary|TestPageUpRequestsAdjacentBackwardRange|TestPageDownStopsAtHighBoundary|TestPageDownRespectsUserLimitBoundary' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestPageDownRequestsAdjacentRange
--- PASS: TestPageDownRequestsAdjacentRange ((t))
=== RUN   TestPageUpSuppressedAtLowBoundary
--- PASS: TestPageUpSuppressedAtLowBoundary ((t))
=== RUN   TestPageUpRequestsAdjacentBackwardRange
--- PASS: TestPageUpRequestsAdjacentBackwardRange ((t))
=== RUN   TestPageDownStopsAtHighBoundary
--- PASS: TestPageDownStopsAtHighBoundary ((t))
=== RUN   TestPageDownRespectsUserLimitBoundary
--- PASS: TestPageDownRespectsUserLimitBoundary ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**3. Serialization: held-pending responses, repeated/opposite keys, count overlap, local horizontal movement.** With the one page request held pending behind a barrier, repeated and opposite Page keys are consumed with no additional connection request while `loading next page…` stays visible; the independent count still settles with its exact wording; and left/right movement stays local.

```bash
go test ./internal/ui -count=1 -run 'TestRepeatedAndOppositeKeysSuppressedWhilePending|TestHorizontalMovementStaysLocalWhilePending|TestCountCoexistsWithPendingPage' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestRepeatedAndOppositeKeysSuppressedWhilePending
--- PASS: TestRepeatedAndOppositeKeysSuppressedWhilePending ((t))
=== RUN   TestHorizontalMovementStaysLocalWhilePending
--- PASS: TestHorizontalMovementStaysLocalWhilePending ((t))
=== RUN   TestCountCoexistsWithPendingPage
--- PASS: TestCountCoexistsWithPendingPage ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**4. Exact page sizes at multiple supported terminal heights, and after a resize.** The requested LIMIT is always exactly the complete visible data rows after the results border, status/count line, and frozen header — 11 at 80×24, 15 at 100×30, 34 at 160×50 — with no partially visible row counted, and the next request after a resize uses the new exact value.

```bash
go test ./internal/ui -count=1 -run 'TestPageSizeEqualsCompleteVisibleRows|TestPageSizeAfterResizeUsesNewValue' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestPageSizeEqualsCompleteVisibleRows
=== RUN   TestPageSizeEqualsCompleteVisibleRows/80x24
=== RUN   TestPageSizeEqualsCompleteVisibleRows/100x30
=== RUN   TestPageSizeEqualsCompleteVisibleRows/160x50
--- PASS: TestPageSizeEqualsCompleteVisibleRows ((t))
    --- PASS: TestPageSizeEqualsCompleteVisibleRows/80x24 ((t))
    --- PASS: TestPageSizeEqualsCompleteVisibleRows/100x30 ((t))
    --- PASS: TestPageSizeEqualsCompleteVisibleRows/160x50 ((t))
=== RUN   TestPageSizeAfterResizeUsesNewValue
--- PASS: TestPageSizeAfterResizeUsesNewValue ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**5. Execution boundary.** `connection.ExecutePage` runs exactly one bound page statement — already carrying its exact LIMIT/OFFSET range — on its own dedicated lease, returning adjacent disjoint typed ranges, a typed empty page past the end, and ordinary failure classification.

```bash
go test ./internal/connection -count=1 -run 'TestExecutePage' -v 2>&1 | sed -E 's/[0-9]+(\.[0-9]+)?s|\(cached\)/(t)/g'
```

```output
=== RUN   TestExecutePageAdjacentRanges
--- PASS: TestExecutePageAdjacentRanges ((t))
=== RUN   TestExecutePageFailureIsOrdinaryRequest
--- PASS: TestExecutePageFailureIsOrdinaryRequest ((t))
PASS
ok  	github.com/chris/sqloid/internal/connection	(t)
```

Every step re-ran green. The full module also passes `gofmt`, `go vet ./...`, and `go test ./...`. The ordering policy stays explicit: implicit `ORDER BY rowid` only for the eligible ordinary-rowid case; views, virtual tables, WITHOUT ROWID or shadowed tables, aggregate/grouped queries, ties, and concurrent writes carry no implied stability; explicit user ASC/DESC ordering — including grouped-aggregate expressions — is preserved byte-for-byte with no appended tie-breaker. Cancellation, stale generations, and viewport generations remain owned by Issues #26/#28.
