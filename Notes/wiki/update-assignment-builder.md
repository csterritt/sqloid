# Issue #37: UPDATE assignment builder and prompt restoration

Issue #37 completes UPDATE construction from refreshed schema identities through ordered SET assignment prompts, optional shared WHERE, safe SQL generation, runnable feedback, and history-ready restoration. It builds on the reusable popup contract from [Issue #12](searchable-popups.md), universal values and safe SQL atoms from [Issue #14](sql-atoms-and-literals.md), shared predicates from [Issue #17](where-guided-predicates.md), and runnable/whole-value seams from [Issue #19](runnable-state-feedback.md).

## Ordered SET state

`internal/querybuilder/write_state.go` owns immutable UPDATE assignment state. `SetCandidates()` derives only visible columns from the selected refreshed write-eligible table, in schema order. `AcceptSetColumn` accepts one of those identities, preserves first-acceptance order, and rejects an already-selected, hidden, empty, unknown, or otherwise ineligible identity without changing the builder.

Each `SetAssignment` has exactly one closed `SetChoice`: `Value` or `NULL`. There is no Default/Omit transition for UPDATE. A Value choice is incomplete until `SubmitSetValue` records both the exact entered representation and its universal parsed `Value`; empty input is complete TEXT and typed `NULL` is literal TEXT. The structural NULL choice is complete immediately, stores no entered value, renders SQL `NULL`, and contributes no parameter.

## Runnable and SQL contract

`QueryBuilder.RunnableReport()` accepts UPDATE only when an eligible refreshed table is selected, at least one SET assignment exists, every SET identity remains visible and unique, every choice is Value or NULL, every Value is submitted, and the optional shared WHERE is absent or complete. Failures target SET before WHERE in visual order. Guided duplicate selection is prevented, while the report still rejects duplicate or malformed states installed through defensive test seams.

`internal/querybuilder/update_sql.go` exposes `UpdateSQL()` and `UpdateParams()`. SQL has the exact shape:

```sql
UPDATE "table" SET "first" = ?, "second" = NULL [WHERE "filter" = ?]
```

Every table and column name is quoted as one schema-derived identifier atom, including embedded quote doubling. User values never enter SQL text. Parameters contain submitted Value assignments in SET selection order, skip NULL assignments, and append the optional value-taking WHERE parameter last. An absent WHERE is a valid unqualified write; `IS NULL` and `IS NOT NULL` WHERE operators add no parameter. Incomplete, stale, malformed, or non-UPDATE state produces no executable UPDATE SQL or parameter request.

## Prompt flow and restoration

`internal/ui/set_popup.go` integrates the state into the `Set` builder field. Enter opens the reusable searchable multi-select over `SetCandidates`; each unique accepted column commits immediately and the popup remains available for further choices. Esc preserves accepted columns and moves into one scroll-only choice popup per assignment. Those popups contain exactly `Value` then `NULL`.

Value opens the universal text prompt for every declared type, seeded byte-for-byte from a previously submitted assignment. The committed assignment is left untouched while revision is open, so moving among prompts or cancelling cannot reparse or partially commit it. Enter submits through QueryBuilder's universal parser; NULL commits immediately without opening text entry. Completion advances through selected assignments and then focuses the optional shared Where field.

Tab and Shift+Tab move among open assignment-choice prompts while restoring each choice highlight. At the base Set field, Up and Down select the assignment to revise. Popup or value-entry Esc restores Set focus and keeps the prior assignment snapshot exact. Changing Value to NULL drops its entered/bound value; changing back to Value opens fresh entry. Backspace/Delete on the base Set field calls `ClearSetValue` for the selected assignment, preserving the Value choice and column identity while atomically removing entered text, parsed type/value, and submission completion.

## History-ready and pre-execution boundary

`HistoryState` already includes ordered SET identities, structural choices, exact entered representations, parsed values, and bound kinds. `RestoreBuilder` rebuilds them through canonical transitions against the current catalog; unresolved identities reject the restoration rather than installing a partial builder. UI prompt revisiting reads the restored assignment directly, preserving choice, text, and concrete type.

Invalid base-context Enter focuses the authoritative first-invalid Set or Where field and emits no validation, preparation, execution, or history command. Runnable UPDATE emits only the existing `PreExecutionRequestedMsg` path. Issue #37 does not estimate targets, append UPDATE history, prepare a destructive confirmation, or execute a write; those remain later destructive-write issues.

## Tests

- `internal/querybuilder/update_test.go` covers schema-derived uniqueness/order, hidden and unknown rejection, immutable submission, Value/NULL completion, empty and typed-NULL TEXT, atom quoting, all-Value/all-NULL/mixed assignments, qualified and unqualified SQL, null-operator WHERE, SET-then-WHERE parameter order, and executable-request rejection for incomplete state.
- `internal/ui/update_flow_test.go` scripts multi-selection and duplicate attempts, exact Value/NULL choice membership/order, universal value entry, sequential continuation to Where, restored highlight/text/cursor/bound kind, Tab/Shift+Tab and arrow assignment navigation, cancellation, Value-to-NULL-to-Value revision, empty TEXT, selected-assignment whole-value clearing, and resulting runnable feedback.

## Cross-references

Issues #12, #14, #17, #19, #35, and #37; [Query Grammar](../PRD-sqloid.md#query-grammar-v1), [Runnable-State Contract](../PRD-sqloid.md#runnable-state-contract), [Builder and Display Interaction](../PRD-sqloid.md#builder-and-display-interaction), SQL safety, Builder lifecycle, QueryBuilder/UI Module Design, UPDATE Testing Decisions, [query-history-append.md](query-history-append.md), and [query-history-navigation.md](query-history-navigation.md).
