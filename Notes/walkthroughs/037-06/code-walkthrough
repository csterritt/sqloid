# Issue #37: UPDATE assignment builder and prompt restoration

*2026-08-29T01:07:53Z by Showboat 0.6.1*
<!-- showboat-id: f1e77ef3-0a88-48b9-bc53-763ddb3cf2f3 -->

This walkthrough demonstrates Issue #37 against Notes/PRD-sqloid.md: schema-derived unique ordered SET selection, exact Value/NULL prompts, universal values, safe UPDATE SQL, optional WHERE, prompt restoration, whole-value clearing, and the history-ready pre-execution boundary. Commands run the actual QueryBuilder and Bubble Tea tests; static summaries print only after those tests pass.

Part 1: the QueryBuilder accepts visible refreshed SET identities in first-selection order and rejects a duplicate without reordering or adding an assignment. Hidden and unknown columns are rejected, and Value submission leaves the prior immutable snapshot untouched.

```bash
go test ./internal/querybuilder -run '^(TestUpdateSetSelectionIsUniqueOrderedAndSchemaDerived|TestSubmitSetValueDoesNotMutatePriorSnapshot)$' -count=1 >/dev/null && printf 'ordered unique schema-derived SET selection: PASS\nimmutable Value submission: PASS\n'
```

```output
ordered unique schema-derived SET selection: PASS
immutable Value submission: PASS
```

Part 2: exact executable construction. The example mixes REAL 2.5, empty TEXT, typed TEXT NULL, and structural SQL NULL, then adds an INTEGER WHERE value. The duplicate attempt is false; identifiers with embedded quotes are doubled atom-by-atom; NULL adds no placeholder or parameter; the WHERE value follows all SET values.

```bash
go test ./internal/querybuilder -run '^ExampleQueryBuilder_UpdateSQL$' -count=1 >/dev/null && printf '%s\n' 'duplicate accepted: false' 'UPDATE "order""items" SET "score" = ?, "first""name" = ?, "note" = ?, "literal" = NULL WHERE "score" = ?' 'runnable: true' 'params: float64(2.5), string(""), string("NULL"), int64(7)'
```

```output
duplicate accepted: false
UPDATE "order""items" SET "score" = ?, "first""name" = ?, "note" = ?, "literal" = NULL WHERE "score" = ?
runnable: true
params: float64(2.5), string(""), string("NULL"), int64(7)
```

Part 3: the construction matrix also proves unqualified UPDATE, all-Value, all-NULL, mixed Value/NULL, typed NULL as TEXT, value-taking WHERE with its parameter last, and IS NULL WHERE with no parameter. An incomplete Value assignment yields neither SQL nor parameters.

```bash
go test ./internal/querybuilder -run '^(TestUpdateSQLAndParams|TestIncompleteUpdateProducesNoExecutableRequest)$' -count=1 >/dev/null && printf 'qualified and unqualified UPDATE matrix: PASS\nincomplete state emits no executable request: PASS\n'
```

```output
qualified and unqualified UPDATE matrix: PASS
incomplete state emits no executable request: PASS
```

Part 4: the Bubble Tea flow opens searchable SET multi-selection, keeps accepted selections unique and ordered, then visits one scroll-only choice popup per column containing exactly Value followed by NULL. Value opens universal entry; NULL completes immediately; finishing assignments focuses optional Where.

```bash
go test ./internal/ui -run '^TestUpdatePromptFlowSelectsUniqueColumnsAndCompletesAssignments$' -count=1 >/dev/null && printf 'SET multi-selection -> Value/NULL prompts -> optional Where: PASS\n'
```

```output
SET multi-selection -> Value/NULL prompts -> optional Where: PASS
```

Part 5: revision restores the exact selected choice, entered bytes, end cursor, and concrete parsed bound type without changing the committed assignment while a prompt is open. Tab and Shift+Tab move among assignment choice prompts with restored highlights; Up and Down select base-field assignments. Esc cancels without a partial commit. Value can change to NULL and back, with fresh empty TEXT entry after NULL.

```bash
go test ./internal/ui -run '^(TestUpdatePromptRevisionRestoresChoiceTextAndBoundType|TestUpdateAssignmentNavigationAndChoiceRevision|TestUpdateChoiceCanChangeValueToNullAndBack)$' -count=1 >/dev/null && printf 'choice/text/cursor/bound-type restoration: PASS\nTab/Shift+Tab and arrow revision: PASS\nEsc and Value/NULL revision: PASS\n'
```

```output
choice/text/cursor/bound-type restoration: PASS
Tab/Shift+Tab and arrow revision: PASS
Esc and Value/NULL revision: PASS
```

Part 6: Backspace/Delete clears the selected submitted assignment atomically while preserving its column and Value choice. The first incomplete SET assignment remains the authoritative runnable target, so invalid Enter produces no preparation, execution, or history command; runnable data reaches only the existing pre-execution seam.

```bash
go test ./internal/ui -run '^(TestUpdateWholeValueClearingUsesCurrentAssignment|TestEnterOnInvalidWriteStatesFocusesWriteTargets|TestEnterOnRunnableDataEmitsOnlyPreExecutionSeam)$' -count=1 >/dev/null && printf 'whole-value clearing and first-invalid SET focus: PASS\nrunnable completion emits pre-execution seam only: PASS\n'
```

```output
whole-value clearing and first-invalid SET focus: PASS
runnable completion emits pre-execution seam only: PASS
```

Part 7: history normalization and restoration retain ordered assignment identities, Value/NULL choices, exact entered text, and parsed concrete values; unresolved refreshed identities reject partial restoration. Issue #37 appends no UPDATE history and performs no destructive write.

```bash
go test ./internal/querybuilder -run '^(TestHistoryState|TestRestore)' -count=1 >/dev/null && printf 'history-ready UPDATE state and exact restoration: PASS\nno estimate, write execution, or UPDATE history append claimed\n'
```

```output
history-ready UPDATE state and exact restoration: PASS
no estimate, write execution, or UPDATE history append claimed
```

Closing: focused QueryBuilder and UI packages remain green.

```bash
go test ./internal/querybuilder ./internal/ui -count=1 >/dev/null && printf 'internal/querybuilder + internal/ui: PASS\n'
```

```output
internal/querybuilder + internal/ui: PASS
```
