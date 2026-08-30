# Issue #39: INSERT choices, omission, and prompt restoration walkthrough

*2026-08-29T12:20:59Z by Showboat 0.6.1*
<!-- showboat-id: edf51c72-1459-4ce0-b812-21e969279b3e -->

Issue #39 (Notes/PRD-sqloid.md) adds end-to-end INSERT construction: table_xinfo-derived visible insertability with hidden/generated exclusion, the closed Value/NULL/Default/Omit choice set in schema order, the exact (auto-assigned if omitted) hint only for the single-column INTEGER PRIMARY KEY rowid alias, quoted SQL with exact parameter order, the all-omit DEFAULT VALUES form, zero-column blocking, and exact prompt restoration in the TUI. Each step runs the project's own tests as proof.

```bash
go test ./internal/schema -run 'TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce|TestInsertOmissionHintExactness|TestInsertHintCaseInsensitiveExactInteger' -v -count=1
```

```output
=== RUN   TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce
=== RUN   TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/ordinary_mixed_table
=== RUN   TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/virtual_table_hidden_input
=== RUN   TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/all-hidden_table
--- PASS: TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce (0.00s)
    --- PASS: TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/ordinary_mixed_table (0.00s)
    --- PASS: TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/virtual_table_hidden_input (0.00s)
    --- PASS: TestInsertableColumnsCoverEveryVisibleColumnExactlyOnce/all-hidden_table (0.00s)
=== RUN   TestInsertOmissionHintExactness
--- PASS: TestInsertOmissionHintExactness (0.00s)
=== RUN   TestInsertHintCaseInsensitiveExactInteger
--- PASS: TestInsertHintCaseInsensitiveExactInteger (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.002s
```

Schema fixtures prove every visible insertable column appears exactly once in schema order while hidden, generated, and virtual-table hidden inputs are excluded — including an all-hidden table. The omission hint is granted only to the single-column INTEGER PRIMARY KEY rowid alias, case-insensitively on exactly INTEGER, never for INT/BIGINT, multi-column keys, WITHOUT ROWID, or non-primary columns.

```bash
go test ./internal/querybuilder -run 'TestInsertPromptPlanFollowsSchemaOrderAndExclusion|TestInsertPromptChoicesAreExactlyTheClosedSet|TestInsertPromptHintExactScope|TestZeroInsertableColumnsBlockExactly' -v -count=1
```

```output
=== RUN   TestInsertPromptPlanFollowsSchemaOrderAndExclusion
--- PASS: TestInsertPromptPlanFollowsSchemaOrderAndExclusion (0.00s)
=== RUN   TestInsertPromptChoicesAreExactlyTheClosedSet
--- PASS: TestInsertPromptChoicesAreExactlyTheClosedSet (0.00s)
=== RUN   TestInsertPromptHintExactScope
--- PASS: TestInsertPromptHintExactScope (0.00s)
=== RUN   TestZeroInsertableColumnsBlockExactly
--- PASS: TestZeroInsertableColumnsBlockExactly (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.004s
```

The QueryBuilder prompt plan exposes every insertable column once in schema order with exactly the closed choice set {Value, NULL, Default/Omit}; the hint is metadata-only and never forces omission; zero insertable columns block with the exact reason 'table has no insertable columns' and no runnable report.

```bash
go test ./internal/querybuilder -run 'TestInsertSQLOrdersValuesNullsAndOmissions|TestInsertSQLQuotesUnusualNames|TestInsertSQLVirtualTableBestEffort|TestInsertSQLRejectsIncompleteState|TestInsertOmissionHintDoesNotForceOmission' -v -count=1
```

```output
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/single_Value
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/mixed_Value_NULL_omit_keeps_prompt_order
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/empty_Value_is_empty_TEXT_parameter
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/typed_NULL_stays_bound_TEXT
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/explicit_NULL_binds_nothing
=== RUN   TestInsertSQLOrdersValuesNullsAndOmissions/all_omitted_emits_DEFAULT_VALUES
--- PASS: TestInsertSQLOrdersValuesNullsAndOmissions (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/single_Value (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/mixed_Value_NULL_omit_keeps_prompt_order (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/empty_Value_is_empty_TEXT_parameter (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/typed_NULL_stays_bound_TEXT (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/explicit_NULL_binds_nothing (0.00s)
    --- PASS: TestInsertSQLOrdersValuesNullsAndOmissions/all_omitted_emits_DEFAULT_VALUES (0.00s)
=== RUN   TestInsertSQLQuotesUnusualNames
--- PASS: TestInsertSQLQuotesUnusualNames (0.00s)
=== RUN   TestInsertSQLVirtualTableBestEffort
--- PASS: TestInsertSQLVirtualTableBestEffort (0.00s)
=== RUN   TestInsertSQLRejectsIncompleteState
--- PASS: TestInsertSQLRejectsIncompleteState (0.00s)
=== RUN   TestInsertOmissionHintDoesNotForceOmission
--- PASS: TestInsertOmissionHintDoesNotForceOmission (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.005s
```

SQL generation includes Value columns with ? placeholders in prompt order, NULL columns as the SQL keyword with no parameter, and omissions absent from both lists; empty TEXT is bound as complete TEXT distinct from typed-NULL TEXT; unusual quoted names are safely doubled; all-omit renders INSERT INTO "t" DEFAULT VALUES; incomplete state produces no SQL.

```bash
go test ./internal/connection -run 'TestInsertIntegrationMixedChoicesAndEffects|TestInsertIntegrationDefaultValuesProvesAllOmitRuns|TestInsertIntegrationQuotedNamesAndVirtualTable|TestInsertIntegrationOrdinaryDatabaseErrors' -v -count=1 | sed -E 's/ \([0-9.]+s\)/(Xs)/g; s/\t[0-9.]+s$/\tXs/'
```

```output
=== RUN   TestInsertIntegrationMixedChoicesAndEffects
--- PASS: TestInsertIntegrationMixedChoicesAndEffects(Xs)
=== RUN   TestInsertIntegrationDefaultValuesProvesAllOmitRuns
--- PASS: TestInsertIntegrationDefaultValuesProvesAllOmitRuns(Xs)
=== RUN   TestInsertIntegrationQuotedNamesAndVirtualTable
--- PASS: TestInsertIntegrationQuotedNamesAndVirtualTable(Xs)
=== RUN   TestInsertIntegrationOrdinaryDatabaseErrors
--- PASS: TestInsertIntegrationOrdinaryDatabaseErrors(Xs)
PASS
ok  	github.com/chris/sqloid/internal/connection	Xs
```

Real SQLite integration: mixed choices insert and read back exactly (empty TEXT vs NULL vs typed-NULL TEXT), DEFAULT VALUES applies declared defaults, INTEGER PRIMARY KEY auto-assigns when omitted, quoted names round-trip, the fts5 virtual table accepts visible-column inserts, and ordinary database errors (NOT NULL violations) surface without builder fabrication. (Durations normalized to Xs for stable verification.)

```bash
go test ./internal/ui -run 'TestInsert' -v -count=1
```

```output
=== RUN   TestInsertPromptFlowChoicesHintAndCompletion
--- PASS: TestInsertPromptFlowChoicesHintAndCompletion (0.00s)
=== RUN   TestInsertPromptRevisionRestoresEverythingExact
--- PASS: TestInsertPromptRevisionRestoresEverythingExact (0.00s)
=== RUN   TestInsertWholeValueClearingKeepsChoice
--- PASS: TestInsertWholeValueClearingKeepsChoice (0.00s)
=== RUN   TestInsertZeroInsertableColumnsBlockExactUI
--- PASS: TestInsertZeroInsertableColumnsBlockExactUI (0.00s)
=== RUN   TestInsertHistoryReadyStateExact
--- PASS: TestInsertHistoryReadyStateExact (0.00s)
=== RUN   TestInsertTabNavigationBetweenPrompts
--- PASS: TestInsertTabNavigationBetweenPrompts (0.00s)
=== RUN   TestInsertUISeedsPromptsOnTableSelection
--- PASS: TestInsertUISeedsPromptsOnTableSelection (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

Scripted TUI coverage: one prompt per insertable column in schema order with the hint rendered only where schema metadata grants it, mixed choices completing with an empty-TEXT parameter, exact revision restoration of choice/buffer/cursor/highlight/focus, whole-value clearing preserving the Value choice, Tab/Shift+Tab navigation, zero-column blocking with the exact message and no popup/command/history, history-ready mixed and all-omit states, and prompt seeding on table selection.
