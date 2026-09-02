# Issue #22 — First end-to-end SELECT and result grid

Issue #22 replaces the disposable Issue #10 tracer with the first production query path: a runnable QueryBuilder passes Issue #21 schema validation, exactly one first-page SELECT executes through the Connection boundary, and the results region renders a frozen-header grid from the shared `internal/result` seam. See `Notes/PRD-sqloid.md` (Builder and Display Interaction, Execution and Result Lifecycle, Grid rendering/cache, Invalid UTF-8 TEXT, Output names, Module Design, Testing Decisions), and cross-references to [schema-validation-workflow.md](schema-validation-workflow.md), [query-history-append.md](query-history-append.md), [cancellation-infrastructure.md](cancellation-infrastructure.md), [session-health.md](session-health.md), and [early-integration-tracer.md](early-integration-tracer.md) (superseded).

## Production flow

1. **Builder → SQL** — `QueryBuilder.SelectSQL()`/`SelectParams()` are the sole source of safely quoted SQL and ordered bound parameters (see [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [group-order-limit.md](group-order-limit.md)). Issue #66 gates both on the authoritative `RunnableReport` (see [select-renderer-gating.md](select-renderer-gating.md)): a non-runnable SELECT yields empty SQL and nil parameters, never a partially valid statement.
2. **Validation handoff** — runnable Enter completes Issue #21 schema validation first; only the successful settled handoff emits `ExecutionStartedMsg`. Failed validation creates no execution and no history.
3. **Actual-execution boundary** — `ExecutionStartedMsg` appends query history (Issue #20 rules, including consecutive-identical suppression) and then issues exactly one first-page request via `startSelectPage()`, capturing the builder's exact SQL/parameters.
4. **Connection first page** — `connection.DB.RunFirstPage` runs the one bound SELECT as a complete `RunRequest` on a dedicated lease (Issue #6 lifecycle, Issue #7 health classification). Rows are scanned eagerly with copied values and converted once via `result.FromDriver`.
5. **Settlement** — `SelectSettledMsg` stores the typed `result.Page` (success) or ordinary error (failure, cause preserved) in `Model.Result *ResultView`. Issue #72 extends settlement to copy `ByteTruncated` and `LimitFailure` from `FirstPageResult` into `ResultView` and OR byte truncation with `viewportCache.TruncatedByByteCap()` after merging the first page (see [byte-cap-oversized-values.md](byte-cap-oversized-values.md)). No database or driver type enters Bubble Tea state.
6. **Grid** — the results region renders the settled page through `internal/result` only.

## Shared result representation (`internal/result`)

The single UI-independent seam consumed by the grid and by future CSV/JSON exporters, never duplicated:

- **Typed values** — `Value`/`Kind` preserve NULL, INTEGER, REAL, TEXT, and BLOB distinctions; render helpers never coerce numeric-looking TEXT, null, or BLOB-like values into another type.
- **Output names** — `DeduplicateNames` applies the full-set collision rule: first occurrence unchanged; each duplicate receives the lowest `_2`, `_3`, … suffix colliding with neither an already-final name nor any original name. Original driver labels remain separately in `Page.Columns`; `HeaderNames()` returns the deduplicated display/export names.
- **Finite REAL tokens** — `RealToken` renders the shortest round-tripping 'g' token, appending `.0` exactly when the token contains none of `.`, `e`, `E` (so `1` → `1.0`, `-0.0` stays `-0.0`); locale-independent. Non-finite policy is Issue #23's, implemented in [non-finite-real-grid.md](non-finite-real-grid.md).
- **Invalid UTF-8** — `DecodeText` replaces each maximal invalid byte sequence with exactly one U+FFFD and sets `Page.InvalidUTF` warning metadata without changing row or column counts. The grid shows the persistent `invalid UTF-8 replaced with U+FFFD` warning in the status line.
- **Control characters** — `GridText` renders tabs as `⇥` and newlines as `⏎` in grid-facing TEXT.
- **BLOBs** — payloads are copied and retained byte-for-byte (including invalid UTF-8 and empty bytes) while display is exactly `[BLOB n bytes]`.
- **`FromDriver`** — converts the plain driver value set (`nil`, `int64`, `float64`, `string`, `[]byte`) once at the Connection boundary; any other type panics rather than coercing.

## Grid behavior

For a successful first page the results region shows a bordered grid: one frozen header row of full-set deduplicated names (stable while data rows are viewed), a status line with the inclusive absolute displayed range (`rows 1-N`, never page-relative), and one line per row. Column widths derive from the widest rendered cell (header included) capped at 32 terminal cells with `…` ellipsis; Unicode width uses go-runewidth; rows stay complete. An executed empty SELECT renders exactly `No rows` with no data range, distinct from the pre-execution `Select a command (S/U/D/I) to begin` prompt. Ordinary execution errors settle into the same result-error boundary and replace idle content.

## Query-history append boundary

History appends exactly when `ExecutionStartedMsg` is handled — at the actual-execution boundary after successful validation, before any database work is issued. Failed executions keep their start append; validation, cancellation, and estimation never append; consecutive-identical suppression follows Issue #20.

## Tracer removal (Task 7)

The hardcoded Issue #10 runtime path is fully removed: `internal/ui/tracer.go`, `internal/connection/tracer.go`, `internal/schema/tracer.go`, and their tests are deleted, along with `Model.Trace`, `StartTraceMsg`/`traceSettledMsg`, `TraceResult`/`TraceGrid`, `schema.ChooseTracerTarget`/`SelectAllSQL`, and `connection.RunTraceSelectAll`. A user-visible SELECT reaches Connection only through the current QueryBuilder → validation orchestration. Retained reusable test support includes the fake-refresher/fake-executor fixtures, SQLite integration harnesses, and shell/layout assertions. `internal/result/architecture_test.go` adds parsed-source architecture assertions: the result package imports neither Bubble Tea nor any driver, `internal/ui` owns no private result formatting, and no tracer route survives.

## Deferred contracts

Independent concurrent count (Issue #24) was completed — see [concurrent-page-count.md](concurrent-page-count.md). Later paging/cache caps and viewport generations (Issue #26), CSV/JSON export escaping, result history navigation, and horizontal scrolling are deliberately absent; `internal/result` is shaped so exporters extend rather than copy it. Non-finite REAL grid rendering was completed by Issue #23 (see [non-finite-real-grid.md](non-finite-real-grid.md)); Issue #47 finalized these shared names, tokens, normalization, and BLOB/NULL contracts for the exporter-facing boundary (see [shared-typed-result-rendering.md](shared-typed-result-rendering.md)).