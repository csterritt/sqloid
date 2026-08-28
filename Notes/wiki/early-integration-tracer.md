# Early Integration Tracer (Issue #10)

A disposable, deliberately minimal end-to-end integration milestone: one hardcoded, safely identifier-quoted `SELECT *` against a single catalog-chosen object flows through Schema → Connection → the Bubble Tea model into a minimal bordered results grid (or a basic non-crashing error display). Its purpose is risk reduction — proving the Connection ↔ Schema ↔ UI module boundaries and integration points before the real builder is built on top — not user-facing functionality.

**Status**: disposable. [Issue #22](project-overview.md) must **replace** this tracer entirely; it must never be extended into a production query path.

## Purpose and boundaries

- **Risk-reduction purpose**: validates early that the Issue #5/#6/#7 connection machinery (pooling, leases, request boundary, typed outcomes), the Issue #9 catalog contract, and the Issue #8 responsive Bubble Tea shell compose without embedding database behavior in the UI.
- **Ownership chain**: `internal/schema` selects and quotes; `internal/connection` executes through its established request boundary; `internal/ui` renders. No SQL, database handles, driver types, or catalog queries ever appear in `internal/ui`, and no terminal copy lives in `internal/schema`/`internal/connection`.
- The tracer accepts only an object already validated by Schema (`ChooseTracerTarget` rejects any name absent from a refreshed catalog snapshot with typed `*schema.TracerError`), so execution never runs against stale or unvalidated identifiers.

## Hardcoded safe SELECT * flow

1. A catalog snapshot comes from `DB.ReadCatalog` (see [schema-catalog.md](schema-catalog.md)).
2. `schema.ChooseTracerTarget(cat, name)` returns the cataloged object or `*TracerError` ("%q: not present in the refreshed schema catalog"). Any catalog-selected kind (ordinary table, virtual table, view) is acceptable because only a SELECT runs.
3. `schema.SelectAllSQL(obj)` renders the one hardcoded statement: `SELECT * FROM "<name>"` — no projection, predicate, ordering, limit, or parameters; the identifier is double-quoted with embedded quotes doubled, so even unusual names execute safely. Nothing user-entered can reach SQL text.
4. `DB.RunTraceSelectAll(parent, target)` executes it as exactly one complete `RunRequest` (see [session-health.md](session-health.md) for boundary semantics): identity verification inside the lease before work, cancellable read on a dedicated leased connection (five-second busy timeout and 64 MiB limit configured per [connection-pool.md](connection-pool.md)), settlement, and post-error identity reclassification where deletion/replacement takes precedence. No schema revalidation happens beyond the original composition choice.

## Typed row/error boundary

- `connection.TracerResult{Columns []string, Rows [][]any}` carries returned column names in result order and row values using driver/`database/sql` value types — `nil` (SQL NULL), `int64`, `float64`, `string`, `[]byte` — so downstream composition renders deterministically without re-parsing. Rows are copied slices because `database/sql` reuses scan arguments.
- Failures settle as failed `RequestResult`s with causes preserved via `%w` through neutral `tracerError` wrappers ("could not trace …"); no terminal wording lives in the wrapper, and failed executions return no result.
- Zero rows is success with headers intact; there is no distinct empty-state or count concept at this milestone.

## UI rendering

- `internal/ui/tracer.go` defines the composition seam: `StartTraceMsg{Execute func(ctx) TraceResult}` triggers one round trip; the executor always runs inside a returned `tea.Cmd` (never in `Update` or `View`); `traceSettledMsg` re-enters `Update`; completion stores fully owned isolated state in `Model.Trace *TraceView{Grid, Err, Settled}`. `TraceResult` translates to `TraceGrid{Headers, Rows}` strings plus an error string — no connection type crosses into the model.
- `view.go`'s `renderResults` shows the minimal bordered grid (bold pipe-joined header row, then pipe-joined data rows) inside the existing bordered results region of the Issue #8 shell when settled tracer state exists; failures show the plain error text instead. Neither copy claims paging, count, history, validation, recovery, or frozen-header behavior.
- `cmd/sqloid/main.go` was not touched: no TUI launch wiring exists yet (the session handler stays silent per Issues #2/#8), so dependency wiring for a live program remains with later issues.

## Deliberately omitted production features

No builder, popup, command/table selection, validation, pre-execution workflows, WHERE/GROUP BY/ORDER BY/LIMIT, parameter binding, paging, independent count request, result cache, frozen-header scrolling, query/result history, cancellation UX beyond parent-context propagation, retry/recovery, and no write paths of any kind exist in the tracer. Rendering owns no database logic; the executor is injected at composition time.

## Tests

See the tracer entries in [unit-tests.md](unit-tests.md). Boundary tests prove typed transport, safe unusual identifiers (`odd "name`), and basic SQLite failure; scripted `(model, msg) → (model, cmd)` UI tests prove the grid, the error state's non-crashing display, single-executor invocation, builder-bar isolation, and exact layout partitioning across 80×24/100×30/160×50.

Cross-references: Issue #10; Module Design and Testing Decisions in [`Notes/PRD-sqloid.md`](../PRD-sqloid.md); [responsive-tui-shell.md](responsive-tui-shell.md); [schema-catalog.md](schema-catalog.md); [session-health.md](session-health.md); [cancellation-infrastructure.md](cancellation-infrastructure.md).
