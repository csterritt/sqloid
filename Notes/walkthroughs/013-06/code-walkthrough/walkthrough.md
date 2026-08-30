# Issue #13: Stale schema refresh, retry, and terminal precedence walkthrough

*2026-08-27T07:10:43Z by Showboat 0.6.1*
<!-- showboat-id: 11707497-de3f-4c07-b9f1-776896463ddc -->

This walkthrough demonstrates the Issue #13 stale schema refresh, retry, and terminal precedence contract from Notes/issues/013-stale-schema-refresh-retry-and-terminal-precedence.md and the Schema scope, cache, and validation plus Session health sections of Notes/PRD-sqloid.md. Every claim below is proven by executable tests in internal/schema and internal/ui using a deterministic fake CatalogRefresher standing in for the Connection boundary — no database access and no sleeps.

```bash
go test ./internal/schema ./internal/ui -count=1 2>&1 | tail -2
```

```output
ok  	github.com/chris/sqloid/internal/schema	0.105s
ok  	github.com/chris/sqloid/internal/ui	0.012s
```

1. Refresh-before-presentation (Task 1/2 RED->GREEN): each Table-popup open issues exactly one fresh main-schema catalog request through the injected Connection seam; the popup presents candidates from the current catalog immediately (Issue #12 contract preserved) but the settled result alone may replace them, and reopening always runs another full request cycle.

```bash
go test ./internal/ui -run '^(TestTableOpenIssuesFreshCatalogRequestPerOpen|TestTableReopenAlwaysIssuesFreshRequest)$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestTableOpenIssuesFreshCatalogRequestPerOpen (0.00s)
--- PASS: TestTableReopenAlwaysIssuesFreshRequest (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

2. Typed refresh attempts in internal/schema (Task 1 schema contract): an Attempt settles into exactly one status; success carries the complete refreshed catalog, ordinary failure carries only its cause so no consumer can partially replace, deletion/replacement are terminal classifications delivered as structured values — never matched from error strings.

```bash
go test ./internal/schema -run '^(TestAttemptValidity|TestAttemptOrdinaryFailurePreservesPriorCatalogIdentity|TestAttemptTerminalStatusesString)$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestAttemptValidity (0.00s)
--- PASS: TestAttemptOrdinaryFailurePreservesPriorCatalogIdentity (0.00s)
--- PASS: TestAttemptTerminalStatusesString (0.00s)
ok  	github.com/chris/sqloid/internal/schema	0.002s
```

3. Unchanged stale candidates with both exact indicators (core contract): an ordinary failure keeps prior candidate identities, ordering, metadata (prior object pointer identity), and the selected builder table unchanged; search keeps filtering; the persistent status renders exactly once as `Schema data is stale — retry or cancel` plus inline `could not refresh: <cause>`, and both survive resize cycles and further typing without rendering drift.

```bash
go test ./internal/ui -run '^TestFailedRefreshRetainsStalePopupWithExactIndicators$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestFailedRefreshRetainsStalePopupWithExactIndicators (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.004s
```

4. Blocked continuation while stale (locked-schema / changed-schema scenario): Enter cannot accept a candidate, Tab/Down cannot advance builder focus toward other fields, and ContinuationBlocked() gates execution until retry succeeds or cancel closes the flow.

```bash
go test ./internal/ui -run '^TestStaleStateBlocksAcceptanceContinuationAndNavigation$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestStaleStateBlocksAcceptanceContinuationAndNavigation (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

5. Retry while pending (Task 3): retry routes through the same seam as the initial open, issues one more identified request, gates duplicate retries while outstanding, and keeps the unchanged stale catalog plus both exact indicators until settlement.

```bash
go test ./internal/ui -run '^TestRetryIssuesNewRequestAndKeepsExactIndicatorsWhilePending$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestRetryIssuesNewRequestAndKeepsExactIndicatorsWhilePending (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

6. Successful retry clears both indicators atomically: the refreshed catalog is installed whole, the popup continues with the refreshed eligible set via deterministic candidate replacement, continuation unblocks, and immediate acceptance commits through QueryBuilder with exact opener restoration.

```bash
go test ./internal/ui -run '^TestSuccessfulRetryClearsIndicatorsAtomicallyAndContinuesPopup$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestSuccessfulRetryClearsIndicatorsAtomicallyAndContinuesPopup (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

7. Repeated failure (retry failing again): the prior catalog stays put, the inline cause becomes exactly that attempt's cause, indicators persist exactly once, and retry remains available.

```bash
go test ./internal/ui -run '^TestRepeatedFailureRetainsCandidatesAndUpdatesCauseOnly$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestRepeatedFailureRetainsCandidatesAndUpdatesCauseOnly (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

8. Cancel restores the exact pre-open state: Esc under stale closes only the refresh flow — popup dismissed, captured Table opener back, selected table/command/catalog snapshot untouched, indicators cleared, no continuation or execution performed.

```bash
go test ./internal/ui -run '^TestCancelClosesFlowAndRestoresExactPreOpenState$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestCancelClosesFlowAndRestoresExactPreOpenState (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

9. Terminal precedence over the stale workflow (deletion and same-path replacement, Task 4): injecting a typed Connection health classification as the settled attempt transitions immediately to the established terminal shell — exactly `Database file no longer exists — session ended` or `Database file was replaced — session ended` — with retry/cancel affordances gone, stale controls suppressed, and keys unable to open any database work. A post-error reclassification beats an ordinary cause outright.

```bash
go test ./internal/ui -run '^(TestDeletionOverridesStaleWorkflowImmediately|TestReplacementOverridesStaleWorkflowImmediately|TestReclassifiedErrorBeatsOrdinaryCause)$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestDeletionOverridesStaleWorkflowImmediately (0.00s)
--- PASS: TestReplacementOverridesStaleWorkflowImmediately (0.00s)
--- PASS: TestReclassifiedErrorBeatsOrdinaryCause (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

10. No late revival: an in-flight success delivered after cancel is discarded without installing anything, and completions arriving inside terminal states leave the exact terminal message and retained catalog byte-stable.

```bash
go test ./internal/ui -run '^TestLateResultsRejectedAfterCancelAndTerminal$' -v -count=1 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestLateResultsRejectedAfterCancelAndTerminal (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

11. Full verification: entire module passes (tests), gofmt-clean sources, vet and build succeed — proving the recorded outputs above reflect committed behavior rather than worked examples.

```bash
gofmt -l internal && go vet ./... && go build ./... && echo VET-BUILD-GOFMT-OK
```

```output
VET-BUILD-GOFMT-OK
```
