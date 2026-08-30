# Issue #36: Immutable result history and query-error recovery

*2026-08-28T21:47:59Z by Showboat 0.6.1*
<!-- showboat-id: eec42374-461f-4075-a9f0-018e93d50ef9 -->

This walkthrough demonstrates Issue #36 — immutable result-history browsing and query-error recovery — as specified in Notes/PRD-sqloid.md (Execution and Result Lifecycle, History Module Design, history Testing Decisions). Every claim below is proven by running the actual tests from internal/history and internal/ui.

Part 1: every actual execution finalizes exactly one immutable snapshot with a stable non-positional ID; the store retains exactly the 20 newest entries and evicts oldest first without changing surviving IDs.

```bash
go test ./internal/history/ -run 'TestResultStoreBoundedRetention|TestAppendFinalizedExactlyOnce' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)' | head -20
```

```output
=== RUN   TestAppendFinalizedExactlyOnce
--- PASS: TestAppendFinalizedExactlyOnce (0.00s)
=== RUN   TestResultStoreBoundedRetention
=== RUN   TestResultStoreBoundedRetention/under_capacity
=== RUN   TestResultStoreBoundedRetention/exactly_at_capacity
=== RUN   TestResultStoreBoundedRetention/one_past_capacity
=== RUN   TestResultStoreBoundedRetention/several_past_capacity
--- PASS: TestResultStoreBoundedRetention (0.00s)
ok  	github.com/chris/sqloid/internal/history	0.002s
```

The retention matrix holds under, at, and past capacity: exactly 20 entries survive chronologically and eviction never renumbers survivors.

Part 2: snapshots are deeply immutable — rows, columns, typed values including exact BLOB bytes, ascending absolute positions, and metadata survive mutation of the source, the live cache, and retrieved copies. Non-tabular error outcomes are retained like any other finalized execution.

```bash
go test ./internal/history/ -run 'TestResultStoreImmutableSnapshots|TestResultStoreRetainsNonTabularOutcomes' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestResultStoreImmutableSnapshots (0.00s)
--- PASS: TestResultStoreRetainsNonTabularOutcomes (0.00s)
ok  	github.com/chris/sqloid/internal/history	0.002s
```

Part 3: stable-ID selection primitives — oldest/newest/current, older/newer steps — behave deterministically on empty stores, at both boundaries, for evicted and never-allocated IDs, and never alias retained storage.

```bash
go test ./internal/history/ -run 'TestResultStoreSelection' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL)' | head -14
```

```output
=== RUN   TestResultStoreSelection
=== RUN   TestResultStoreSelection/empty_store
=== RUN   TestResultStoreSelection/oldest_and_newest
=== RUN   TestResultStoreSelection/older_and_newer_steps_are_independent_of_indices
=== RUN   TestResultStoreSelection/boundaries_are_deterministic_no-ops
=== RUN   TestResultStoreSelection/evicted_and_unknown_IDs_never_resolve
=== RUN   TestResultStoreSelection/returned_entries_are_deep_copies
--- PASS: TestResultStoreSelection (0.00s)
ok  	github.com/chris/sqloid/internal/history	0.002s
```

Part 4: browsing renders immutable snapshots — the pure projection reslices the selected entry locally for the current terminal height's complete-row capacity (absolute offset from the retained range), never rewrites the stored entry, never consults the live result cache, and never mutates retained data through the projected view.

```bash
go test ./internal/ui/ -run 'TestProjectHistoryEntry' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestProjectHistoryEntryReslicesAtTerminalHeight (0.00s)
--- PASS: TestProjectHistoryEntryNonTabular (0.00s)
--- PASS: TestProjectHistoryEntryImmutable (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Part 5: zero refetch while browsing — entry selection, repeated navigation down to the oldest boundary and back up through the newest-boundary exit, resize at multiple terminal heights, and rendering all issue zero database, page, or count requests. The only fresh-data path remains an actual rerun, and entering history finalized the active SELECT exactly once.

```bash
go test ./internal/ui/ -run 'TestResultHistoryBrowsingIssuesZeroRequests' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestResultHistoryBrowsingIssuesZeroRequests (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

Part 6: the Ctrl+E/Y key seam — Ctrl+E and Ctrl+Y enter at the newest entry, Ctrl+E steps older through tabular and non-tabular snapshots, boundary presses are no-ops, Ctrl+Y at the newest entry exits, and Esc leaves result history with all three entries still reachable. No key issues any request.

```bash
go test ./internal/ui/ -run 'TestResultHistoryKeysEnterAndTraverse' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestResultHistoryKeysEnterAndTraverse (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Part 7: starting an actual execution while result history is selected exits history mode first — the historical selection, cursor, and stale displayed rows clear before the execution and its Issue #34 finalization proceed.

```bash
go test ./internal/ui/ -run 'TestExecutionStartExitsResultHistory' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestExecutionStartExitsResultHistory (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Part 8: ordinary query-error recovery — a first-page failure finalizes exactly one lifecycle-defined error entry that becomes the newest result and replaces the visible result area; Esc dismisses only the displayed error without deleting history; older successful entries remain reachable through Ctrl+E/Y. A later-page failure after retained rows finalizes a tabular failed snapshot preserving the captured rows.

```bash
go test ./internal/ui/ -run 'TestQueryErrorReplacesResultAndDismisses|TestLaterPageErrorFinalizesFailedSnapshot' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestQueryErrorReplacesResultAndDismisses (0.00s)
--- PASS: TestLaterPageErrorFinalizesFailedSnapshot (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

Part 9: a request that exceeds the five-second busy timeout failing with 'database is locked' is an ordinary query error — one finalized error entry carrying the exact cause, the ordinary result-error boundary, and no terminal state. Where an authoritative health classification exists (path deletion/replacement), the terminal state overrides the lock error entirely.

```bash
go test ./internal/ui/ -run 'TestDatabaseIsLockedIsOrdinaryQueryError|TestTerminalHealthOverridesLockError' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestDatabaseIsLockedIsOrdinaryQueryError (0.00s)
--- PASS: TestTerminalHealthOverridesLockError (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

Part 10: exceeding 20 entries and the defensive eviction path. When an externally driven mutation evicts the selected entry (oldest, middle, or newest) while entries remain, selection moves to the new oldest retained entry — resliced at the current height — with exactly 'Previously viewed result was evicted from history'. The history-side matrix proves exactly the excess oldest entries disappear, every surviving ID stays intact, and the new-oldest target is deterministic.

```bash
go test ./internal/history/ -run 'TestResultStoreSelectedEvictionReconciliationMatrix' -count=1 -v 2>&1 | grep -cE '^    --- PASS'; go test ./internal/history/ -run 'TestResultStoreSelectedEvictionReconciliationMatrix' -count=1 2>&1 | tail -1
```

```output
6
ok  	github.com/chris/sqloid/internal/history	0.002s
```

```bash
go test ./internal/ui/ -run 'TestDefensiveEvictedSelectionNewOldest|TestDefensiveEvictedSelectionEmptyHistory|TestDefensiveEvictionNeverRendersEvictedRows' -count=1 -v 2>&1 | grep -E '^(    --- PASS|--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestDefensiveEvictedSelectionNewOldest (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/full_history,_oldest_selected (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/full_history,_middle_selected (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/full_history,_newest_selected (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/partially_filled,_oldest_selected (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/partially_filled,_newest_selected (0.00s)
    --- PASS: TestDefensiveEvictedSelectionNewOldest/partially_filled_then_filled,_middle_selected (0.00s)
--- PASS: TestDefensiveEvictedSelectionEmptyHistory (0.00s)
--- PASS: TestDefensiveEvictionNeverRendersEvictedRows (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.007s
```

Empty-history eviction returns to the base builder/result fallback with the same notice and no historical rows; the no-evicted-data walk proves no frame after resize, navigation, or dismissal can render rows from an evicted backing entry, with zero database requests throughout. Normal actual execution is the separate, non-defensive path: it exits history before its append, so selection is never evicted by Sqloid's own work.

Closing: the complete suite stays green.

```bash
go test ./... 2>&1 | tail -11
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	0.004s
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/ui	0.106s
```
