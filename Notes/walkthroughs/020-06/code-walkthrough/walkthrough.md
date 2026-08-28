# Issue #20: Minimal query-history append

*2026-08-27T16:50:04Z by Showboat 0.6.1*
<!-- showboat-id: e743e552-aa75-4851-a9b4-ff360e9ec363 -->

Issue #20 delivers the minimal query-history append per the History and Execution and Result Lifecycle decisions of Notes/PRD-sqloid.md. Every artifact lives under this approved directory: _demo20/main.go is the runnable demonstration program. Ownership is split: internal/querybuilder owns the canonical normalized snapshot and equality, internal/history owns stable IDs, chronological storage, the exact 20-entry cap, and consecutive suppression, and internal/ui owns only the append timing.

```bash
go test ./internal/history -count=1 -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestConsecutiveIdenticalExecutionIsSuppressed (DUR)
--- PASS: TestSuppressedAppendConsumesNoStableIDOrEviction (DUR)
--- PASS: TestAlternatingExecutionsRetainBothEntries (DUR)
--- PASS: TestNormalizedDifferencesAppend (DUR)
--- PASS: TestFailedExecutionStillRetainsStartAppend (DUR)
--- PASS: TestEmptyStoreHasDeterministicEmptyBehavior (DUR)
--- PASS: TestAppendAssignsStableNonPositionalIDs (DUR)
--- PASS: TestEntriesPreserveChronologicalOrder (DUR)
--- PASS: TestLookupAddressesEntriesByStableID (DUR)
--- PASS: TestCapacityIsExactlyTwentyWithOldestFirstEviction (DUR)
--- PASS: TestRepeatedPayloadsRetainDistinctEntries (DUR)
--- PASS: TestAppendStoresImmutableCompleteState (DUR)
--- PASS: TestRetrievalReturnsDefensiveCopies (DUR)
ok  	github.com/chris/sqloid/internal/history	DUR
```

The storage suite proves stable nonzero identities that are not slice positions and survive bulk eviction unchanged, immutable deep-copied complete states (source and retrieved-value mutation cannot alter a retained entry), chronological order, lookup by stable ID, repeated payloads keeping distinct identities, and exact capacity 20 with oldest-first eviction preserving all surviving IDs.

Section 1 and 2: live demonstration of stable non-positional IDs surviving eviction and immutable stored copies after source and retrieved-state mutation attempts, with repeated oldest-first eviction and surviving IDs unchanged.

```bash
go run ./Notes/walkthroughs/020-06/code-walkthrough/_demo20 2>&1 | sed -n '/== 1/,/== 3/p'
```

```output
== 1. Stable non-positional IDs, capacity 20, oldest-first eviction ==
Capacity=20  Len after 25 appends=20
first five IDs all evicted: [false false false false false]
oldest surviving entry: index 5 -> ID 6 want 6 table t5

== 2. Immutable stored copies ==
source mutated, retained unchanged: true
retrieval mutated, store unchanged: true

== 3. Normalized comparison differences ==
```

Section 3: normalized comparison differences from the QueryBuilder HistoryState. Every significant field — command, table identity, projection order, WHERE operator, entered representation (7 vs 07), parsed bound type (INTEGER 7 vs TEXT "7"), GROUP BY order, ORDER BY direction, Limit empty-vs-number and the number, UPDATE ordered choice/value, INSERT ordered choices — is equality-significant even where rendered SQL or bound database values could match, while two states differing only in entered Limit bytes (5 vs 05) with the same accepted number compare equal because entered Limit bytes are transient once accepted. Output order is sorted for determinism.

```bash
go run ./Notes/walkthroughs/020-06/code-walkthrough/_demo20 2>&1 | sed -n '/== 3/,/== 4/p'
```

```output
== 3. Normalized comparison differences ==
command                          equal=false
group order                      equal=false
insert ordered choices           equal=false
limit empty vs number            equal=false
limit number                     equal=false
order direction                  equal=false
projection order                 equal=false
table                            equal=false
update choice/value              equal=false
where bound type                 equal=false
where entered representation     equal=false
where operator                   equal=false
limit 5 vs 05 accepted 5         equal=true (entered bytes are transient)

== 4. Append policy A→A→B→A ==
```

Section 4: the append policy through A→A→B→A. The second A suppresses only the consecutive duplicate — no entry, no ID (0 returned) — while the later A retains its own stable ID: suppression compares only the immediately preceding retained execution. A full store also survives a suppressed append without eviction.

```bash
go run ./Notes/walkthroughs/020-06/code-walkthrough/_demo20 2>&1 | sed -n '/== 4/,/== 5/p'
```

```output
== 4. Append policy A→A→B→A ==
A appended=true id=1; A again appended=false id=0 (suppressed)
B appended=true id=2; A again appended=true id=3
Len=3 order: 1:a 2:b 3:a

== 5. UI execution-start seam timing ==
```

Section 5: the execution-start timing seam in internal/ui. Runnable evaluation on the base Command field emits only PreExecutionRequestedMsg and appends nothing; handling that seam appends nothing; the actual ExecutionStartedMsg appends once and suppresses an identical consecutive start; a later lifecycle event standing in for a failed execution cannot undo the start append; and an unconfirmed UPDATE execution-start message never appends because confirmation (Issues #37/#38) begins the sole actual write.

```bash
go run ./Notes/walkthroughs/020-06/code-walkthrough/_demo20 2>&1 | awk '/== 5/{f=1} f'
```

```output
== 5. UI execution-start seam timing ==
focused field: Command report={Runnable:true Field:Command Reason:}
runnable Enter -> PreExecutionRequestedMsg=true, history Len=0 (nothing appended)
pre-execution seam handled, history Len=0 (still nothing)
actual SELECT execution started, history Len=1
identical execution started again, history Len=1 (suppressed)
after failure only later, Len=1 retained entry id=1 command=SELECT
unconfirmed UPDATE execution-start message, history Len=1 (never appended)
```

Section 6: the exhaustive normalized-equality suite in internal/querybuilder/history_state_test.go and the scripted UI seam tests in internal/ui/query_history_append_test.go, including runnable evaluation never appending, A→B retaining distinct IDs through later lifecycle events, and UPDATE never appending without confirmation.

```bash
go test ./internal/querybuilder ./internal/ui -count=1 -run 'History|ExecutionStart|RunnableEvaluationNever|WriteCommandsNever' -v 2>&1 | grep -E '^(--- (PASS|FAIL)|ok)' | sed -E 's/[0-9]+\.[0-9]+s/DUR/'
```

```output
--- PASS: TestHistoryStatePreservesSignificantFields (DUR)
--- PASS: TestHistoryStateExcludesTransientState (DUR)
--- PASS: TestHistoryStateDeepCopiesSlices (DUR)
ok  	github.com/chris/sqloid/internal/querybuilder	DUR
--- PASS: TestRunnableEvaluationNeverAppendsHistory (DUR)
--- PASS: TestExecutionStartAppendsThenSuppressesConsecutive (DUR)
--- PASS: TestDistinctExecutionStartRetainsEntries (DUR)
--- PASS: TestWriteCommandsNeverAppendWithoutConfirmation (DUR)
ok  	github.com/chris/sqloid/internal/ui	DUR
```

Closing: Ctrl+P/N navigation, restoration into the builder, history cursors, result history, and selected-entry eviction fallback are explicitly deferred to Issue #35; Issue #22 owns emitting ExecutionStartedMsg after successful pre-execution schema validation so that failed executions retain the entry appended at start. See Notes/PRD-sqloid.md (History implementation decision, Execution and Result Lifecycle) and Notes/wiki/query-history-append.md.
