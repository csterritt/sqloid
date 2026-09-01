# Issue #66: Authoritative SELECT renderer gating

Every public SELECT-family SQL and parameter method emits nothing unless the selected SELECT passes the authoritative `RunnableReport` (Issue #19, extended by Issue #65's stale-projection gate). The gate consolidates renderability onto the single runnable authority so no partially rendered clauses or values escape a non-runnable state, while accepted builders retain exact quoting, parameter order, Limit/OFFSET clamping, the rowid fallback, and complete-limited-result count semantics. See `Notes/PRD-sqloid.md` (Runnable-State Contract, Query Grammar, safe rendering, schema revalidation, QueryBuilder Module Design, Testing Decisions) and cross-references to [runnable-state-feedback.md](runnable-state-feedback.md), [first-select-result-grid.md](first-select-result-grid.md), [concurrent-page-count.md](concurrent-page-count.md), [serialized-vertical-paging.md](serialized-vertical-paging.md), and [group-order-limit.md](group-order-limit.md).

## The single runnable authority

`QueryBuilder.RunnableReport()` (Issue #19, `internal/querybuilder/runnable.go`) is the sole validity source for the SELECT renderer family. Issue #66 retires the renderer's own command/table checks — `SelectSQL`, `PageSQL`, `CountSQL`, `SelectParams`, `PageParams`, and `CountParams` now require the selected command to be `CommandSelect` **and** the same authoritative `RunnableReport` to be runnable before any clause or parameter is produced. No second validator, recursive report/render dependency, duplicate stale-identifier check, or renderer-specific interpretation of validity exists: the gate is one `if q.command != CommandSelect || !q.RunnableReport().Runnable` test at the shared `renderSelectCore` seam and at `SelectParams`.

The shared `renderSelectCore` assembly seam (`internal/querybuilder/select_sql.go`) is preserved for accepted state: it assembles the quoted projection, table, WHERE, GROUP BY, and ORDER BY (plus the page-only `ORDER BY rowid` fallback) exactly as before, but only after the runnable gate passes. Rejected methods return only empty SQL and nil parameters — never partially rendered clauses or values — including cases whose component state is locally formattable (a formattable projection beside an invalid Limit, an open WHERE draft, a stale grouped column, or a mixed aggregate/nonaggregate projection without GROUP BY).

## Rejected output contract

Every non-runnable SELECT class yields empty `SelectSQL`/`PageSQL`/`CountSQL` and nil `SelectParams`/`PageParams`/`CountParams`:

- **Missing command, table, or projection** — unselected command, no table, or empty projection.
- **Stale table** — the selected table vanished from the refreshed catalog.
- **Stale named Value and aggregate projections** (Issue #65) — a committed `ProjectionColumn` entry (Value or any of Count/Min/Max/Avg/Sum) whose column no longer exists among the selected object's visible columns; the synthetic wildcard and `COUNT(*)` sentinel are exempt by `ProjectionKind`, never by display text.
- **Incomplete or stale WHERE** — an open WHERE draft, a structurally incomplete predicate, or a committed WHERE naming a vanished column.
- **Every invalid grouping class** — mixed aggregate/nonaggregate projection without GROUP BY, wildcard beside GROUP BY, a stale grouped column.
- **Invalid ORDER BY** — a committed ORDER BY expression no longer offered among the current context candidates.
- **Malformed, zero, negative, or overflow Limit** — any nonempty entered Limit representation that did not parse into `[1, 9223372036854775807]`.
- **Any open value state** — an open WHERE draft or pending value submission, alone or combined with later invalid state.

Non-SELECT commands (UPDATE/DELETE/INSERT) are rejected by the `CommandSelect` test even when the write command itself is runnable, so a write builder's fields can never render as SELECT fragments.

## Unchanged valid SELECT/page/count semantics

Accepted builders retain exact quoting and binding order:

- **Wildcard** — `SELECT * FROM "items"`.
- **`COUNT(*)` sentinel** — `SELECT COUNT(*) FROM "items"`.
- **Named Value and aggregate projections** — quoted identifiers and aggregate tokens, e.g. `SELECT "score" FROM "items"`, `SELECT COUNT("score") FROM "items"`.
- **Quoted identifiers** — atom-by-atom quoting with embedded-quote doubling, e.g. `SELECT "col""x" FROM "we""ird"`.
- **Completed WHERE values** — one bound parameter in placeholder order, e.g. `SELECT * FROM "items" WHERE "name" = ?` with `SelectParams() = [int64(42)]`.
- **Grouped/aggregate ordering** — exact expression and direction, e.g. `SELECT COUNT("name") FROM "items" GROUP BY "id" ORDER BY COUNT("name") DESC`.
- **User Limit** — canonical integer, e.g. `LIMIT 7`.
- **Page-size clamping** — `PageSQL(pageLimit, offset)` renders `LIMIT min(pageLimit, userLimit - offset) OFFSET offset`; offsets at or beyond the user's Limit yield empty. Paging literals add no bindings: `PageParams()` matches `SelectParams()`.
- **Nonzero OFFSET** — exact integer OFFSET rendered canonically.
- **Count wrapping** — `CountSQL()` renders exactly `SELECT COUNT(*) FROM (<SelectSQL()>)` with any user LIMIT inside the subquery; `CountParams()` matches `SelectParams()` in order and typed values.
- **Eligible page-only `ORDER BY rowid` fallback** — `PageSQL` appends `ORDER BY rowid` only for an ordinary rowid table with no declared rowid shadow, no user ORDER BY, and no aggregate/group shape; `SelectSQL` and `CountSQL` never carry the fallback.

Range validation for invalid page size/offset (`pageLimit < 1` or `offset < 0`) is preserved independently of builder validity: `PageSQL` checks the range before the runnable gate, so an invalid range yields empty regardless of runnability.

## Dependency: Issue #65 before Issue #66

Issue #66 builds on Issue #65's projection validation: `reportStaleProjection` (added in `runnable.go` by Issue #65) validates every committed named `ProjectionColumn` entry against the selected object's refreshed visible columns before later SELECT fields. Without Issue #65, a stale projected column would leave `RunnableReport` runnable, so the Issue #66 gate would render SQL referencing a vanished column. Issue #66's gate trusts `RunnableReport` as the single authority precisely because Issue #65 made the report authoritative for projection staleness.

## Tests

- `internal/querybuilder/select_renderer_gate_test.go` (Issue #66 Task 1) — the full rejection matrix: missing command/table/projection, stale table, stale named Value and each aggregate projection, incomplete and stale WHERE, every invalid grouping and ORDER BY class, malformed/zero/negative/overflow Limit, and open value states combined with later invalid state. Each case asserts `RunnableReport().Runnable` is false, then requires `SelectSQL`, `PageSQL`, `CountSQL`, `SelectParams`, `PageParams`, and `CountParams` to emit nothing. Non-SELECT runnable UPDATE/DELETE/INSERT builders are required to keep the family empty.
- `internal/querybuilder/select_renderer_valid_test.go` (Issue #66 Task 3) — exact regression locks for valid SELECT, paging, count, Limit/OFFSET, quoting, rowid fallback, and parameter order after gating. Every positive case asserts the `RunnableReport` runnable prerequisite, then locks exact SQL across `SelectSQL`/`PageSQL`/`CountSQL` and parameter order across `SelectParams`/`PageParams`/`CountParams`, with paging literals adding no bindings and count wrapping the unchanged base SELECT. Non-SELECT runnable builders stay empty.
- Existing `internal/querybuilder/firstpage_sql_test.go`, `page_sql_test.go`, `count_sql_test.go`, `group_by_test.go`, `order_by_test.go`, and `limit_test.go` updated to commit projections through the guided aggregate-completion seam so their fixtures rest on accepted runnable state (the renderer no longer expands an empty projection to all visible columns, since `RunnableReport` requires a nonempty projection).
- `internal/connection/select_cancellation_capability_test.go` `probeQB` updated to complete the named-column projection aggregate so the probe builder is runnable under the gate.

See [unit-tests.md](unit-tests.md) and [source-code.md](source-code.md) for the complete catalogs.

## Cross-references

Issues #19, #65, #66; the Runnable-State Contract, Query Grammar, safe rendering, schema revalidation, QueryBuilder Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; [runnable-state-feedback.md](runnable-state-feedback.md), [first-select-result-grid.md](first-select-result-grid.md), [concurrent-page-count.md](concurrent-page-count.md), [serialized-vertical-paging.md](serialized-vertical-paging.md), [group-order-limit.md](group-order-limit.md), [sql-atoms-and-literals.md](sql-atoms-and-literals.md), and [schema-validation-workflow.md](schema-validation-workflow.md).
