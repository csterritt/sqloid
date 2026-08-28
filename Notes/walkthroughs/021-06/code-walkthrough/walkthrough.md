# Issue #21: Pre-execution schema-version validation

*2026-08-27T18:40:56Z by Showboat 0.6.1*
<!-- showboat-id: 8341899e-6f7f-4f22-9933-9da8e4246e70 -->

Issue #21 delivers cancellable pre-execution schema-version validation per the Execution and Result Lifecycle and Schema scope/cache/validation decisions of Notes/PRD-sqloid.md. Every artifact lives under this approved directory: _demo21/main.go is the runnable demonstration program. Ownership: internal/schema owns the typed unchanged/refreshed/refresh-failed/deleted/replaced outcomes, internal/querybuilder owns dependent-only repair with the authoritative first-reason report, internal/connection hides PRAGMA schema_version reads behind the cancellable request boundary, and internal/ui owns the validation workflow — runnable Enter, preparation identities, retry/cancel, exact cancelling…, terminal precedence, and the no-history guarantee.

```bash
go test ./internal/schema ./internal/querybuilder -run 'Revalidate' -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestRevalidateUnchangedVersionReusesExactCacheWithoutRefresh (DUR)
--- PASS: TestRevalidateChangedVersionRefreshesThroughSeam (DUR)
--- PASS: TestRevalidateOrdinaryRefreshFailureCarriesOnlyCause (DUR)
--- PASS: TestRevalidateTerminalHealthClassifications (DUR)
--- PASS: TestRevalidateStatusString (DUR)
ok  	github.com/chris/sqloid/internal/schema	DUR
--- PASS: TestRevalidateIdenticalCatalogPreservesEverything (DUR)
--- PASS: TestRevalidateDroppedTableClearsOnlyDependentState (DUR)
--- PASS: TestRevalidateEligibilityChangeClearsWriteCommandTable (DUR)
--- PASS: TestRevalidateIdentifierInvalidationCases (DUR)
--- PASS: TestRevalidateInsertabilityInvalidation (DUR)
--- PASS: TestRevalidateRowidPropertyInvalidation (DUR)
--- PASS: TestRevalidateNilCatalogIsNoOp (DUR)
ok  	github.com/chris/sqloid/internal/querybuilder	DUR
```

Sections 1 and 2: the typed outcome tests. schema.Revalidate reuses the exact prior pointer on an unchanged version without ever invoking refresh, refreshes once through the seam on a changed version, and an ordinary failure carries only its cause while the prior cache stands (terminals mapped from Issue #7 health kinds). QueryBuilder.Revalidate drops only state transitively dependent on an invalidated table/identifier/insertability fact/rowid property and reports the post-repair first reason.

```bash
go run ./Notes/walkthroughs/021-06/code-walkthrough/_demo21
```

```output
== 1. schema.Revalidate outcomes ==
unchanged: status=unchanged samePointer=true
changed:   status=refreshed refreshCalls=1 catalog==refreshed=true
failure:   status=refresh-failed cause="database is locked" cacheStands=true
== 2. QueryBuilder.Revalidate repair fixtures ==
identifier: cleared=true entries=1 limit=(5,true) runnable=true firstReason=""
eligibility: cleared=true tableSelected=false runnable=false
insertability: cleared=true prompts=2 (note prompt dropped; id+email remain)
rowid: committed=true cleared=true orderRemaining=false projection=1
== 3. Live sqlite: version read, changed refresh, post-validation DDL ==
opened: version=2 catalog=2 objects=[extras0 users]
unchanged: status=unchanged samePointer=true (no refresh request issued)
changed:   version=3 status=refreshed objects=[extras0 logs users]
post-validation race: settled=unchanged stillUnchanged=true nextReadSees=4 (ordinary execution territory)
```

The live section opens a real modernc.org/sqlite file through connection.Open, reuses the cached catalog without issuing any refresh on an unchanged version, refreshes to the new snapshot after external DDL changes PRAGMA schema_version, and shows that a settled unchanged outcome remains immutable after a post-validation DDL race — the next request boundary discovers the change, making it an ordinary execution error rather than a retroactive validation failure.

```bash
go test ./internal/ui -run 'TestRunnableEnterOpensValidationWithNoHistory|TestChangedVersionRefreshesOnceAndPreservesUnrelatedState|TestRepairFocusesFirstInvalidReasonAndBlocksExecution' -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestRunnableEnterOpensValidationWithNoHistory (DUR)
--- PASS: TestChangedVersionRefreshesOnceAndPreservesUnrelatedState (DUR)
--- PASS: TestRepairFocusesFirstInvalidReasonAndBlocksExecution (DUR)
ok  	github.com/chris/sqloid/internal/ui	DUR
```

Runnable Enter opens the distinct validation workflow and issues the schema-version request before any execution command, appending nothing; an unchanged version reuses the cache with zero catalog refreshes; a changed version refreshes exactly once, removing only the vanished column's projection entry while preserving Limit and the surviving entry; and a refresh that invalidates the selected object blocks execution and focuses the first specific reason.

```bash
go test ./internal/ui -run 'TestStaleValidationRetryUsesFreshIdentityAndCancelRestores|TestCancelRestoresContextWithoutExecution' -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestStaleValidationRetryUsesFreshIdentityAndCancelRestores (DUR)
--- PASS: TestCancelRestoresContextWithoutExecution (DUR)
ok  	github.com/chris/sqloid/internal/ui	DUR
```

An ordinary locked refresh failure retains the exact prior cache behind the quoted stale status plus the inline could-not-refresh cause; retry issues a fresh version read under preparation identity 2, a response from the superseded identity 1 is discarded on arrival, and cancel (Esc) restores the exact pre-validation builder context without execution — with no history appended by the failed or cancelled flow. The retained-stale-data test uses a lock failure fixture exactly like Issue #13's.

Ctrl+W during an in-flight validation dispatches the connection-scoped cancellation exactly once (a second press is refused, and Enter cannot start a replacement request), renders the exact cancelling status until settlement, and a late success arriving after cancellation is classified as cancelled and discarded wholesale — the workflow closes with no execution and no history.

```bash
go test ./internal/ui -run 'TestTerminalOverridesValidation|TestReplacedVersionReadIsTerminal|TestPostValidationRaceIsOrdinaryExecutionError' -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestTerminalOverridesValidation (DUR)
--- PASS: TestReplacedVersionReadIsTerminal (DUR)
--- PASS: TestPostValidationRaceIsOrdinaryExecutionError (DUR)
ok  	github.com/chris/sqloid/internal/ui	DUR
```

Deletion and replacement injected before work suppress the workflow entirely (no request is issued); a version read classifying as deleted or replaced transitions to the exact session-ended terminal presentations and rejects late completions; and after a settled successful validation the execution route stands with exactly one execution-start history append — DDL after validation surfaces only through the later ordinary execution-error path.

```bash
go test ./internal/connection -run 'Revalidate|SchemaVersion' -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestReadSchemaVersionReturnsCurrentVersion (DUR)
--- PASS: TestReadSchemaVersionCancelledContextFailsWithCancellation (DUR)
--- PASS: TestRevalidateUnchangedVersionSkipsCatalogRefresh (DUR)
--- PASS: TestRevalidateChangedVersionRefreshesSelectedObjectAndColumns (DUR)
--- PASS: TestRevalidateOrdinaryCorruptionRefreshFailureRetainsPriorCache (DUR)
ok  	github.com/chris/sqloid/internal/connection	DUR
```

Finally, the Connection boundary itself: ReadSchemaVersion runs as one cancellable request (a cancelled context fails with cancellation, no partial reads), and the fake-Connection and modernc.org/sqlite integration coverage proves an unchanged version skips the catalog refresh entirely while a changed version refreshes the selected object and columns, with ordinary corruption failures retaining the prior cache. Reference: Issue #21 and Notes/PRD-sqloid.md (Execution and Result Lifecycle; Schema scope, cache, and validation; Builder lifecycle; Module Design; Testing Decisions).
