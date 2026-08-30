# Issue #46: Deletion and replacement terminal workflow

*2026-08-29T17:13:08Z by Showboat 0.6.1*
<!-- showboat-id: 1cf1c8c2-01bd-4bcd-8903-53c69dfbf328 -->


Issue #46 (Notes/PRD-sqloid.md 'Session health' and 'Global Key Precedence and Context/Action Matrix') closes the session-health lifecycle: when any request boundary classifies Issue #7's typed deletion (`HealthDeleted`) or typed same-path replacement (`HealthReplaced`) outcome, the UI enters the corresponding terminal state whose exact primary message is `Database file no longer exists — session ended` or `Database file was replaced — session ended`, owned only by `internal/ui`. Entry happens only after every pending transaction/driver work has ended, all database-capable actions are suppressed, in-memory history navigation stays available, a reduced help lists only what works, and `q`/Ctrl+C quit immediately with status 1. First: the typed classification tables at every request boundary.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestHealthTerminal' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_populated
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_populated
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_empty
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_empty
--- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_populated (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_populated (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_empty (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_empty (Ts)
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/deleted_populated
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/replaced_populated
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/deleted_empty
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/replaced_empty
--- PASS: TestHealthTerminalImmediateQuitFromEveryContext (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/deleted_populated (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/replaced_populated (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/deleted_empty (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/replaced_empty (Ts)
=== RUN   TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing
--- PASS: TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing (Ts)
=== RUN   TestHealthTerminalHelpKeepsDatabaseWorkBlocked
--- PASS: TestHealthTerminalHelpKeepsDatabaseWorkBlocked (Ts)
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry/deleted
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry/replaced
--- PASS: TestHealthTerminalSelectsNewestResultOnEntry (Ts)
    --- PASS: TestHealthTerminalSelectsNewestResultOnEntry/deleted (Ts)
    --- PASS: TestHealthTerminalSelectsNewestResultOnEntry/replaced (Ts)
=== RUN   TestHealthTerminalResultNavigationBoundaries
=== RUN   TestHealthTerminalResultNavigationBoundaries/deleted
=== RUN   TestHealthTerminalResultNavigationBoundaries/replaced
--- PASS: TestHealthTerminalResultNavigationBoundaries (Ts)
    --- PASS: TestHealthTerminalResultNavigationBoundaries/deleted (Ts)
    --- PASS: TestHealthTerminalResultNavigationBoundaries/replaced (Ts)
=== RUN   TestHealthTerminalQueryHistoryNavigation
=== RUN   TestHealthTerminalQueryHistoryNavigation/deleted
=== RUN   TestHealthTerminalQueryHistoryNavigation/replaced
--- PASS: TestHealthTerminalQueryHistoryNavigation (Ts)
    --- PASS: TestHealthTerminalQueryHistoryNavigation/deleted (Ts)
    --- PASS: TestHealthTerminalQueryHistoryNavigation/replaced (Ts)
=== RUN   TestHealthTerminalEmptyResultHistoryFallback
=== RUN   TestHealthTerminalEmptyResultHistoryFallback/deleted
=== RUN   TestHealthTerminalEmptyResultHistoryFallback/replaced
--- PASS: TestHealthTerminalEmptyResultHistoryFallback (Ts)
    --- PASS: TestHealthTerminalEmptyResultHistoryFallback/deleted (Ts)
    --- PASS: TestHealthTerminalEmptyResultHistoryFallback/replaced (Ts)
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback/deleted
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback/replaced
--- PASS: TestHealthTerminalEmptyQueryHistoryFallback (Ts)
    --- PASS: TestHealthTerminalEmptyQueryHistoryFallback/deleted (Ts)
    --- PASS: TestHealthTerminalEmptyQueryHistoryFallback/replaced (Ts)
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage/deleted
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage/replaced
--- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage (Ts)
    --- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage/deleted (Ts)
    --- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage/replaced (Ts)
=== RUN   TestHealthTerminalResizeIssuesNoRequests
=== RUN   TestHealthTerminalResizeIssuesNoRequests/deleted
=== RUN   TestHealthTerminalResizeIssuesNoRequests/replaced
--- PASS: TestHealthTerminalResizeIssuesNoRequests (Ts)
    --- PASS: TestHealthTerminalResizeIssuesNoRequests/deleted (Ts)
    --- PASS: TestHealthTerminalResizeIssuesNoRequests/replaced (Ts)
=== RUN   TestHealthTerminalIsTypedNotErrorText
--- PASS: TestHealthTerminalIsTypedNotErrorText (Ts)
=== RUN   TestHealthTerminalStringsOwnedByUIOnly
--- PASS: TestHealthTerminalStringsOwnedByUIOnly (Ts)
=== RUN   TestHealthTerminalForbidsDatabaseWork
=== RUN   TestHealthTerminalForbidsDatabaseWork/deleted
=== RUN   TestHealthTerminalForbidsDatabaseWork/replaced
--- PASS: TestHealthTerminalForbidsDatabaseWork (Ts)
    --- PASS: TestHealthTerminalForbidsDatabaseWork/deleted (Ts)
    --- PASS: TestHealthTerminalForbidsDatabaseWork/replaced (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```


The full reduced-set run above proves: the reduced help lists only the in-memory actions (history selection, help dismissal, immediate quit — never execution, refresh, paging, rerun, cancellation, or any database suggestion) in both terminals with populated and empty histories; `q` and Ctrl+C exit immediately with status 1 from every subview (primary message, query history, result history, help) bypassing the confirmation and scheduling nothing; database work stays blocked while help is open; and resizing issues no requests. Next: typed classification entering both terminals from every request boundary during ordinary activity — first SELECT, independent count, later page, destructive estimate, phased write, validation, and catalog refresh.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -v -run 'TestSelectBoundary|TestCountBoundary|TestPageBoundary|TestEstimateBoundary|TestWriteBoundary|TestValidationBoundary|TestRefreshBoundary' 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestWriteBoundaryResistsRegressionAndStaleIdentities
--- PASS: TestWriteBoundaryResistsRegressionAndStaleIdentities (Ts)
=== RUN   TestSelectBoundaryHealthErrorEntersDeletionTerminal
--- PASS: TestSelectBoundaryHealthErrorEntersDeletionTerminal (Ts)
=== RUN   TestSelectBoundaryHealthErrorEntersReplacementTerminal
--- PASS: TestSelectBoundaryHealthErrorEntersReplacementTerminal (Ts)
=== RUN   TestCountBoundaryHealthErrorDuringOrdinaryActivity
=== RUN   TestCountBoundaryHealthErrorDuringOrdinaryActivity/deleted
=== RUN   TestCountBoundaryHealthErrorDuringOrdinaryActivity/replaced
--- PASS: TestCountBoundaryHealthErrorDuringOrdinaryActivity (Ts)
    --- PASS: TestCountBoundaryHealthErrorDuringOrdinaryActivity/deleted (Ts)
    --- PASS: TestCountBoundaryHealthErrorDuringOrdinaryActivity/replaced (Ts)
=== RUN   TestPageBoundaryHealthErrorDuringOrdinaryActivity
=== RUN   TestPageBoundaryHealthErrorDuringOrdinaryActivity/deleted
=== RUN   TestPageBoundaryHealthErrorDuringOrdinaryActivity/replaced
--- PASS: TestPageBoundaryHealthErrorDuringOrdinaryActivity (Ts)
    --- PASS: TestPageBoundaryHealthErrorDuringOrdinaryActivity/deleted (Ts)
    --- PASS: TestPageBoundaryHealthErrorDuringOrdinaryActivity/replaced (Ts)
=== RUN   TestEstimateBoundaryHealthError
--- PASS: TestEstimateBoundaryHealthError (Ts)
=== RUN   TestWriteBoundaryHealthError
--- PASS: TestWriteBoundaryHealthError (Ts)
=== RUN   TestValidationBoundaryHealthClassification
=== RUN   TestValidationBoundaryHealthClassification/deleted
=== RUN   TestValidationBoundaryHealthClassification/replaced
--- PASS: TestValidationBoundaryHealthClassification (Ts)
    --- PASS: TestValidationBoundaryHealthClassification/deleted (Ts)
    --- PASS: TestValidationBoundaryHealthClassification/replaced (Ts)
=== RUN   TestRefreshBoundaryHealthClassification
=== RUN   TestRefreshBoundaryHealthClassification/deleted
=== RUN   TestRefreshBoundaryHealthClassification/replaced
--- PASS: TestRefreshBoundaryHealthClassification (Ts)
    --- PASS: TestRefreshBoundaryHealthClassification/deleted (Ts)
    --- PASS: TestRefreshBoundaryHealthClassification/replaced (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```


Every boundary classification lands in the exact terminal state with no pending work — the SELECT/count/page settlements, the estimate settlement (preparation dismissed, not an estimate failure), the phased write's authoritative typed `WriteResult.Health` field after the transaction and driver work have fully ended, and the pre-existing validation/refresh typed mappings routing into the same `enterTerminal` transition. Next: the classification is typed, never error text, the exact strings live only in `internal/ui`, and every database-capable key from either terminal produces no command and zero executor requests.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -v -run 'TestHealthTerminalIsTypedNotErrorText|TestHealthTerminalStringsOwnedByUIOnly|TestHealthTerminalForbidsDatabaseWork' 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestHealthTerminalIsTypedNotErrorText
--- PASS: TestHealthTerminalIsTypedNotErrorText (Ts)
=== RUN   TestHealthTerminalStringsOwnedByUIOnly
--- PASS: TestHealthTerminalStringsOwnedByUIOnly (Ts)
=== RUN   TestHealthTerminalForbidsDatabaseWork
=== RUN   TestHealthTerminalForbidsDatabaseWork/deleted
=== RUN   TestHealthTerminalForbidsDatabaseWork/replaced
--- PASS: TestHealthTerminalForbidsDatabaseWork (Ts)
    --- PASS: TestHealthTerminalForbidsDatabaseWork/deleted (Ts)
    --- PASS: TestHealthTerminalForbidsDatabaseWork/replaced (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```


The decoy error whose text is exactly the UI message is classified as an ordinary failure, the connection layer carries no terminal copy, and every database-capable key from either terminal is consumed before any command can be built. Next: immutable history selection — newest result on entry, Ctrl+E/Y and Ctrl+P/N traversal with deterministic boundaries, and the empty-history fallbacks keeping the exact terminal message as the whole primary view with no synthetic or missing-backed entry.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -v -run 'TestHealthTerminalSelectsNewestResultOnEntry|TestHealthTerminalResultNavigationBoundaries|TestHealthTerminalQueryHistoryNavigation|TestHealthTerminalEmptyResultHistoryFallback|TestHealthTerminalEmptyQueryHistoryFallback|TestHealthTerminalBothHistoriesEmptyPrimaryMessage|TestHealthTerminalResizeIssuesNoRequests' 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry/deleted
=== RUN   TestHealthTerminalSelectsNewestResultOnEntry/replaced
--- PASS: TestHealthTerminalSelectsNewestResultOnEntry (Ts)
    --- PASS: TestHealthTerminalSelectsNewestResultOnEntry/deleted (Ts)
    --- PASS: TestHealthTerminalSelectsNewestResultOnEntry/replaced (Ts)
=== RUN   TestHealthTerminalResultNavigationBoundaries
=== RUN   TestHealthTerminalResultNavigationBoundaries/deleted
=== RUN   TestHealthTerminalResultNavigationBoundaries/replaced
--- PASS: TestHealthTerminalResultNavigationBoundaries (Ts)
    --- PASS: TestHealthTerminalResultNavigationBoundaries/deleted (Ts)
    --- PASS: TestHealthTerminalResultNavigationBoundaries/replaced (Ts)
=== RUN   TestHealthTerminalQueryHistoryNavigation
=== RUN   TestHealthTerminalQueryHistoryNavigation/deleted
=== RUN   TestHealthTerminalQueryHistoryNavigation/replaced
--- PASS: TestHealthTerminalQueryHistoryNavigation (Ts)
    --- PASS: TestHealthTerminalQueryHistoryNavigation/deleted (Ts)
    --- PASS: TestHealthTerminalQueryHistoryNavigation/replaced (Ts)
=== RUN   TestHealthTerminalEmptyResultHistoryFallback
=== RUN   TestHealthTerminalEmptyResultHistoryFallback/deleted
=== RUN   TestHealthTerminalEmptyResultHistoryFallback/replaced
--- PASS: TestHealthTerminalEmptyResultHistoryFallback (Ts)
    --- PASS: TestHealthTerminalEmptyResultHistoryFallback/deleted (Ts)
    --- PASS: TestHealthTerminalEmptyResultHistoryFallback/replaced (Ts)
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback/deleted
=== RUN   TestHealthTerminalEmptyQueryHistoryFallback/replaced
--- PASS: TestHealthTerminalEmptyQueryHistoryFallback (Ts)
    --- PASS: TestHealthTerminalEmptyQueryHistoryFallback/deleted (Ts)
    --- PASS: TestHealthTerminalEmptyQueryHistoryFallback/replaced (Ts)
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage/deleted
=== RUN   TestHealthTerminalBothHistoriesEmptyPrimaryMessage/replaced
--- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage (Ts)
    --- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage/deleted (Ts)
    --- PASS: TestHealthTerminalBothHistoriesEmptyPrimaryMessage/replaced (Ts)
=== RUN   TestHealthTerminalResizeIssuesNoRequests
=== RUN   TestHealthTerminalResizeIssuesNoRequests/deleted
=== RUN   TestHealthTerminalResizeIssuesNoRequests/replaced
--- PASS: TestHealthTerminalResizeIssuesNoRequests (Ts)
    --- PASS: TestHealthTerminalResizeIssuesNoRequests/deleted (Ts)
    --- PASS: TestHealthTerminalResizeIssuesNoRequests/replaced (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```


Both terminal variants behave identically across populated and empty histories: entry selects the newest stable-backed immutable result (or nothing, keeping the exact terminal message as the whole view), Ctrl+P/N traverse complete query-history states and Ctrl+E/Y the immutable results with deterministic no-op boundaries, empty histories are no-ops with no synthetic entry, and resize reprojects locally with zero requests. Finally: reduced help contents and the immediate status-1 quit.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -v -run 'TestHealthTerminalReducedHelpListsOnlyInMemoryActions|TestHealthTerminalImmediateQuitFromEveryContext|TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing|TestHealthTerminalHelpKeepsDatabaseWorkBlocked' 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_populated
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_populated
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_empty
=== RUN   TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_empty
--- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_populated (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_populated (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/deleted_empty (Ts)
    --- PASS: TestHealthTerminalReducedHelpListsOnlyInMemoryActions/replaced_empty (Ts)
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/deleted_populated
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/replaced_populated
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/deleted_empty
=== RUN   TestHealthTerminalImmediateQuitFromEveryContext/replaced_empty
--- PASS: TestHealthTerminalImmediateQuitFromEveryContext (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/deleted_populated (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/replaced_populated (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/deleted_empty (Ts)
    --- PASS: TestHealthTerminalImmediateQuitFromEveryContext/replaced_empty (Ts)
=== RUN   TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing
--- PASS: TestHealthTerminalQuitBypassesConfirmationAndSchedulesNothing (Ts)
=== RUN   TestHealthTerminalHelpKeepsDatabaseWorkBlocked
--- PASS: TestHealthTerminalHelpKeepsDatabaseWorkBlocked (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```


That closes Issue #46: typed deletion and replacement at any request boundary end the session atomically with no pending transaction or driver work, exact UI-owned terminal messages, immutable in-memory history navigation with empty fallbacks, a reduced help with no database suggestions, and immediate status-1 `q`/Ctrl+C without confirmation. Ctrl+S (save) and Ctrl+X (export) inside these terminals remain owned by Issues #48 and #49. Full detail: Notes/wiki/health-terminal.md and Notes/PRD-sqloid.md.

