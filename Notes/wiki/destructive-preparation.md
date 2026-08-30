# Issue #40: Destructive-write estimate presentation and count

Issue #40 opens destructive preparation for runnable validated UPDATE and DELETE statements: a modal that continuously shows the operation, table, and canonical rendered write SQL, dispatches exactly one independent matching-target estimate, and keeps confirmation disabled through estimate settlement. It composes Issue #37's UPDATE assignments and Issue #38's DELETE predicate with Issue #14's shared identifier/literal atoms ([sql-atoms-and-literals.md](sql-atoms-and-literals.md)), Issues #17/#19's shared predicate and runnable report ([where-guided-predicates.md](where-guided-predicates.md), [runnable-state-feedback.md](runnable-state-feedback.md)), and Issue #21's pre-execution validation handoff ([schema-validation-workflow.md](schema-validation-workflow.md)).

## Modal contents and ownership of serialization

After a settled successful validation (`RevalidateUnchanged`, or `RevalidateRefreshed` with still-runnable repaired data), `Model.executionRoute()` routes UPDATE and DELETE into `beginPreparation()` instead of the SELECT/INSERT execution-start seam. The modal opens immediately under a fresh monotonic preparation/request identity (`prepAttempt`) and renders, through every estimate state and under unrelated redraw/resize messages:

- the operation type (`DESTRUCTIVE UPDATE` / `DESTRUCTIVE DELETE`) and the selected table name;
- QueryBuilder's canonical rendered write SQL — `UpdateRenderedSQL()` / `DeleteRenderedSQL()` from `internal/querybuilder/estimate_sql.go`, derived from the same structured state as the executable statements. Every submitted SET or WHERE Value serializes through Issue #14's sole shared `RenderSQLLiteral` typed-literal renderer and `quoteIdentifierAtom` identifier atoms: INTEGER canonical decimal, REAL shortest round-trip, TEXT quote-doubled, typed TEXT `NULL` preserved as a quoted text literal, and the SQL-NULL assignment choice plus `IS NULL`/`IS NOT NULL` predicates as keywords;
- a **prominent all-rows warning** (`WARNING: no WHERE clause — this statement targets every row of <table>`) only when no WHERE predicate is committed; qualified writes show no false warning;
- the estimate status: exactly `Estimating matching target rows…` while pending, `cancelling…` after a Ctrl+W request until settlement, `Estimated matching target rows: N` on settled success, or `Estimate failed: <cause>` on settled failure — with SQL, table, and warning retained in every state.

The modal owns **no** serializer: the QueryBuilder literal renderer is the only SQL serialization path, and a modal-private serializer or predicate reconstruction would violate the ownership expectations asserted by the tests.

## Estimate SQL, params, and meaning

The estimate is exactly `SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]`, built by `EstimateSQL()` from the quoted selected table and the shared committed predicate verbatim — the same `?`-placeholder predicate SQL and binding semantics as the executable statements. `EstimateParams()` binds **only** the predicate's values in predicate order: zero parameters for no-WHERE statements and `IS NULL`/`IS NOT NULL` predicates, and never any UPDATE SET parameter, including when SET mixes Value and NULL assignments. `internal/connection/estimate.go` adds `ExecuteEstimate(ctx, statement, params)`, which runs the statement once as a cancellable independent autocommit read through the established `RunRequest` boundary (path identity, health classification) and returns the count/error without ever executing the write, appending history, or opening a transaction. The count is an **estimated matching target row count**: it excludes trigger side effects and may differ from changed/no-op rows and `RowsAffected()`.

## Identity, confirmation, dismissal, and history

Opening, pending, success, failure, cancellation, and dismissal append neither query nor result history, and nothing executes. Enter/y remain consumed no-ops while pending and after settlement — enabling confirmation is Issue #41's seam. `EstimateSettledMsg` carries the issuing preparation identity: stale identities mutate nothing; a current success retains the count and a current failure retains the cause while preserving SQL/warning and reaching the confirmation-ready seam. Esc/n dismisses, cancelling any in-flight estimate via the connection-scoped interrupt, restoring the exact builder focus; Ctrl+W while estimating requests cancellation once and shows exact `cancelling…`, and the cancelled settlement dismisses preparation without history.

## Tests

- `internal/querybuilder/estimate_sql_test.go` — exact rendered UPDATE/DELETE SQL across mixed/all-Value/all-NULL SET, typed TEXT `NULL`, empty TEXT, REAL/INTEGER literals, unusual quoted identifiers, null-operator predicates, and unqualified forms; command/runnable gating; exact estimate SQL and WHERE-only parameter semantics for both commands with SET values excluded.
- `internal/connection/estimate_integration_test.go` — SQLite-backed `ExecuteEstimate` counting matching targets, the unqualified zero-parameter form, read-only guarantee (no rows changed), surfaced query failures, and cancellation classification.
- `internal/ui/destructive_prep_test.go` — scripted opening from runnable validated UPDATE/DELETE with unique identity and exactly one estimate request, continuously visible operation/table/SQL/warning through redraw, resize, and pending states, exact pending text, disabled Enter/y, success/failure retention, stale-response rejection, Esc dismissal with observed cancellation, Ctrl+W-then-settlement dismissal, and zero history at every stage.

## Cross-references

Issues #14, #17, #21, #37, #38, and #40; the pre-execution identities, Writes lifecycle, [Estimate SQL and modal](../PRD-sqloid.md#estimate-sql-and-modal), SQL safety, History, Connection/QueryBuilder/UI Module Design, and Testing Decisions sections of [Notes/PRD-sqloid.md](../PRD-sqloid.md); [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [update-assignment-builder.md](update-assignment-builder.md), [delete-predicate-builder.md](delete-predicate-builder.md), [schema-validation-workflow.md](schema-validation-workflow.md), [query-history-append.md](query-history-append.md), and [session-health.md](session-health.md).
