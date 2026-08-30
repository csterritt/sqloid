# Issue #38: DELETE predicate builder walkthrough

*2026-08-29T10:17:36Z by Showboat 0.6.1*
<!-- showboat-id: 817e0e88-9200-4b3f-8cc5-67bd5875ea4b -->

Issue #38 (parent issue #38, Notes/PRD-sqloid.md) adds DELETE construction by composing existing contracts: Issue #9 write-table eligibility, Issue #14 quoting/binding atoms, Issue #17's shared optional WHERE predicate, and Issue #19's authoritative runnable report. This walkthrough proves each task's claims with the project's own tests.

## 1. Eligibility and the exact unqualified no-WHERE SQL

```bash
go test ./internal/querybuilder -run TestDeleteWithoutWhereIsRunnableUnqualifiedSQL -v -count=1
```

```output
=== RUN   TestDeleteWithoutWhereIsRunnableUnqualifiedSQL
--- PASS: TestDeleteWithoutWhereIsRunnableUnqualifiedSQL (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

The runnable report accepts DELETE on both the ordinary table `order"items` and the virtual table; each renders the exact safely quoted `DELETE FROM "order""items"` / `DELETE FROM "logs"` with zero parameters (assertions inside the test pin these strings verbatim).

## 2. Column → operator → value: every predicate operator, exact SQL and params

```bash
go test ./internal/querybuilder -run TestDeleteSQLAndParamsWithPredicate -v -count=1
```

```output
=== RUN   TestDeleteSQLAndParamsWithPredicate
=== RUN   TestDeleteSQLAndParamsWithPredicate/integer_value
=== RUN   TestDeleteSQLAndParamsWithPredicate/real_value
=== RUN   TestDeleteSQLAndParamsWithPredicate/empty_text_value
=== RUN   TestDeleteSQLAndParamsWithPredicate/typed_NULL_stays_TEXT
=== RUN   TestDeleteSQLAndParamsWithPredicate/LIKE_binds_verbatim_wildcards
=== RUN   TestDeleteSQLAndParamsWithPredicate/IS_NULL_binds_no_parameter
=== RUN   TestDeleteSQLAndParamsWithPredicate/IS_NOT_NULL_binds_no_parameter
--- PASS: TestDeleteSQLAndParamsWithPredicate (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/integer_value (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/real_value (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/empty_text_value (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/typed_NULL_stays_TEXT (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/LIKE_binds_verbatim_wildcards (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/IS_NULL_binds_no_parameter (0.00s)
    --- PASS: TestDeleteSQLAndParamsWithPredicate/IS_NOT_NULL_binds_no_parameter (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.001s
```

Each value-taking operator (=, !=, <, <=, >=, LIKE) binds exactly one universally parsed parameter — INTEGER 7, REAL 1.5, empty TEXT, typed `NULL` as literal TEXT, and `%a_b%` byte-for-byte; `IS NULL` and `IS NOT NULL` bind none. Parameter assertions use `reflect.DeepEqual` over the concrete driver types inside the test.

## 3. Rejected identities: views, unknown/system names, stale selections

```bash
go test ./internal/querybuilder -run TestDeleteRejectsNonTableIdentities -v -count=1
```

```output
=== RUN   TestDeleteRejectsNonTableIdentities
=== RUN   TestDeleteRejectsNonTableIdentities/view_selection_is_ignored
=== RUN   TestDeleteRejectsNonTableIdentities/unknown_system_name_is_ignored
=== RUN   TestDeleteRejectsNonTableIdentities/stale_selection_is_cleared_by_refresh
--- PASS: TestDeleteRejectsNonTableIdentities (0.00s)
    --- PASS: TestDeleteRejectsNonTableIdentities/view_selection_is_ignored (0.00s)
    --- PASS: TestDeleteRejectsNonTableIdentities/unknown_system_name_is_ignored (0.00s)
    --- PASS: TestDeleteRejectsNonTableIdentities/stale_selection_is_cleared_by_refresh (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.001s
```

The report targets RunFieldTable with `select a table` in every rejection and no executable request is produced.

## 4. Incomplete column/operator/value states: first-invalid focus and reasons

```bash
go test ./internal/querybuilder -run TestIncompleteDeletePredicateIsNotRunnable -v -count=1
```

```output
=== RUN   TestIncompleteDeletePredicateIsNotRunnable
=== RUN   TestIncompleteDeletePredicateIsNotRunnable/column_chosen_but_no_operator
=== RUN   TestIncompleteDeletePredicateIsNotRunnable/operator_chosen_but_no_value
--- PASS: TestIncompleteDeletePredicateIsNotRunnable (0.00s)
    --- PASS: TestIncompleteDeletePredicateIsNotRunnable/column_chosen_but_no_operator (0.00s)
    --- PASS: TestIncompleteDeletePredicateIsNotRunnable/operator_chosen_but_no_value (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.001s
```

Partially selected stages block at RunFieldWhere with the shared no-incomplete-value-prompt gate, and DeleteSQL/DeleteParams produce nothing executable.

## 5. Scripted TUI: selection, flows, restoration, gating, and the preparation handoff

```bash
go test ./internal/ui -run TestDelete -v -count=1
```

```output
=== RUN   TestDeleteTableSelectionOffersOnlyWriteEligibleObjects
--- PASS: TestDeleteTableSelectionOffersOnlyWriteEligibleObjects (0.00s)
=== RUN   TestDeleteNoWhereEnterHandsOffToPreparation
--- PASS: TestDeleteNoWhereEnterHandsOffToPreparation (0.00s)
=== RUN   TestDeletePredicateFlowsCompleteAndHandOff
=== RUN   TestDeletePredicateFlowsCompleteAndHandOff/value_equality
=== RUN   TestDeletePredicateFlowsCompleteAndHandOff/LIKE_verbatim_wildcards
=== RUN   TestDeletePredicateFlowsCompleteAndHandOff/IS_NULL_bypasses_value_input
=== RUN   TestDeletePredicateFlowsCompleteAndHandOff/IS_NOT_NULL_bypasses_value_input
--- PASS: TestDeletePredicateFlowsCompleteAndHandOff (0.00s)
    --- PASS: TestDeletePredicateFlowsCompleteAndHandOff/value_equality (0.00s)
    --- PASS: TestDeletePredicateFlowsCompleteAndHandOff/LIKE_verbatim_wildcards (0.00s)
    --- PASS: TestDeletePredicateFlowsCompleteAndHandOff/IS_NULL_bypasses_value_input (0.00s)
    --- PASS: TestDeletePredicateFlowsCompleteAndHandOff/IS_NOT_NULL_bypasses_value_input (0.00s)
=== RUN   TestDeletePredicateEscRestoresExactStateAndFocus
--- PASS: TestDeletePredicateEscRestoresExactStateAndFocus (0.00s)
=== RUN   TestDeleteWholeValueClearingPreservesChoice
--- PASS: TestDeleteWholeValueClearingPreservesChoice (0.00s)
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter/open_popup
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter/focused_value_input
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter/quit_confirmation_overlay
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter/request_pending
=== RUN   TestDeleteHigherPrecedenceContextsConsumeEnter/too_small_terminal
--- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter (0.00s)
    --- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter/open_popup (0.00s)
    --- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter/focused_value_input (0.00s)
    --- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter/quit_confirmation_overlay (0.00s)
    --- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter/request_pending (0.00s)
    --- PASS: TestDeleteHigherPrecedenceContextsConsumeEnter/too_small_terminal (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

These scripted tests prove: Table popup offers only write-eligible objects (the view is excluded); invalid attempts leave validationAttempt at 0 and zero history entries; every runnable path emits only PreExecutionRequestedMsg — never a direct internal/connection write; higher-precedence contexts (open popup, focused value input, quit-confirmation overlay, request pending, too-small terminal) consume Enter before runnable evaluation; and Esc/revision restores the exact committed predicate, highlighted operator, prompt buffer, and opener focus.

## 6. Qualified runnable SQL/params and non-runnable states produce no request

```bash
go test ./internal/querybuilder -run 'TestDelete' -count=1
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```
