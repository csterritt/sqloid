# Issue #38: DELETE predicate builder and destructive handoff

Issue #38 completes DELETE construction from refreshed schema identities through the optional shared WHERE, safe SQL generation, runnable feedback, and the pre-execution-only destructive handoff. It adds no new builder state: DELETE composes Issue #9's write-table eligibility ([schema-catalog.md](schema-catalog.md)), Issue #14's quoting/binding atoms ([sql-atoms-and-literals.md](sql-atoms-and-literals.md)), Issue #17's shared optional predicate ([where-guided-predicates.md](where-guided-predicates.md)), and Issue #19's authoritative runnable evaluator ([runnable-state-feedback.md](runnable-state-feedback.md)).

## Write-table eligibility

DELETE accepts only refreshed write-eligible identities supplied by `internal/schema`: ordinary and virtual tables. Views, prohibited system objects, and unknown names are never offered by `EligibleTables()` and `SelectTable` ignores them; a selection whose identity vanishes from the refreshed catalog is cleared together with every downstream state, including the WHERE predicate and any open draft. Command or table replacement clears only dependent DELETE state — the WHERE predicate plus its draft — through the wholesale `discardSelectors` rule.

## Optional WHERE: absent is valid, unqualified

An eligible table with an absent WHERE is a valid **unqualified** delete targeting every row; no synthetic value is required. A complete predicate **qualifies** the delete. The shared predicate semantics are unchanged: the same nine operators, the same column → operator → conditional value flow, typed `NULL` and empty input as verbatim TEXT, LIKE `%`/`_` bound byte-for-byte, and `IS NULL`/`IS NOT NULL` completing with no value prompt and no parameter. Revisiting and cancelling prior stages restore exact column, operator, entered text, bound type, and opener focus.

## SQL and parameters

`internal/querybuilder/delete_sql.go` exposes `DeleteSQL()` and `DeleteParams()`:

```sql
DELETE FROM "table" [WHERE "filter" = ?]
```

- The table name is one schema-derived identifier atom with embedded quote doubling; column atoms quote identically. No user-entered value ever appears in SQL text.
- No WHERE and every `IS NULL`/`IS NOT NULL` predicate produce zero parameters; value-taking comparisons and LIKE bind exactly one universally parsed parameter at its concrete driver type (INTEGER, REAL, empty TEXT, typed TEXT `NULL`, verbatim `%`/`_`).
- A complete predicate is appended exactly once. Incomplete, stale, malformed, or non-DELETE state produces no executable SQL or parameter request.

## Runnable feedback and first-invalid targeting

`RunnableReport()` accepts DELETE only when a refreshed write-eligible table is selected and the shared WHERE is absent or complete. An open draft or incomplete predicate blocks at `RunFieldWhere` with the shared no-incomplete-value-prompt gate; a committed predicate naming a vanished column reports the stale-where-column reason. Invalid base-context Enter focuses the Where field, renders the exact reason verbatim, and issues no validation, preparation, execution, or history command — no preparation identity or history entry exists for invalid attempts.

## Global context precedence and preparation handoff

Open popups, focused universal value entry, overlays (quit confirmation), in-flight/request-pending states, and the too-small terminal all consume Enter before runnable evaluation, exactly per the PRD context matrix. On runnable data — the no-WHERE form and every complete predicate form — DELETE emits only the established `PreExecutionRequestedMsg` pre-execution validation seam under Issue #21's preparation identity. It never invokes `internal/connection` write execution directly and never appends query or result history: DELETE appends only at its later confirmation-driven write start. This page's implementation performs no database work and imports neither `internal/ui` nor `internal/connection`.

## Tests

- `internal/querybuilder/delete_sql_test.go` covers ordinary and virtual eligible tables, rejected view/system/stale identities, embedded-quote table atoms, the exact no-WHERE `DELETE FROM "table"` shape with no parameters, complete SQL plus exact parameters for every fixed operator (INTEGER, REAL, empty TEXT, typed TEXT `NULL`, verbatim `%`/`_`, and the two null operators with none), runnable-report assertions, and executable-request rejection for partially selected column/operator/value states with first-invalid targeting.
- `internal/ui/delete_flow_test.go` scripts command/table selection over only write-eligible objects, the no-WHERE runnable seam into the validation workflow, value equality, LIKE with verbatim wildcards, null-operator value-prompt bypass, whole-value clearing, revisiting/cancelling with exact state/focus restoration, incomplete-stage Enter focusing Where with the exact reason and no preparation identity or history, and popup/value-input/overlay/request-pending/too-small contexts consuming Enter before runnable evaluation.

## Cross-references

Issues #9, #14, #17, #19, #21, and #38; [Query Grammar](../PRD-sqloid.md#query-grammar-v1), [Runnable-State Contract](../PRD-sqloid.md#runnable-state-contract), [Builder and Display Interaction](../PRD-sqloid.md#builder-and-display-interaction), SQL safety, Schema scope, QueryBuilder/UI Module Design, pre-execution identities, and Testing Decisions in [Notes/PRD-sqloid.md](../PRD-sqloid.md); [schema-catalog.md](schema-catalog.md), [where-guided-predicates.md](where-guided-predicates.md), [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [runnable-state-feedback.md](runnable-state-feedback.md), [schema-validation-workflow.md](schema-validation-workflow.md), [update-assignment-builder.md](update-assignment-builder.md), and [query-history-append.md](query-history-append.md).
