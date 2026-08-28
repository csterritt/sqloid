# Schema Catalog and Table Eligibility

How Sqloid describes the main schema: one refresh returns the schema version plus every eligible object with its kind, write eligibility, rowid capability, declared-rowid shadowing, and typed columns — independent of UI and free of type-specific input behavior. Implementation lives in `internal/schema` (types and rules) and `internal/connection/schema.go` (request plumbing), per Issue #9 and the Schema scope and Schema metadata decisions in [PRD-sqloid.md](../../PRD-sqloid.md) (Schema scope, Schema metadata, Module Design, and Testing Decisions sections).

## Catalog queries (main schema only)

A catalog read is one `RunRequest` on `DB.ReadCatalog(ctx)` (`internal/connection/schema.go`), so request-boundary identity checks ([session-health.md](session-health.md)), dedicated leasing ([connection-pool.md](connection-pool.md)), and cancellation ([cancellation-infrastructure.md](cancellation-infrastructure.md)) apply exactly as to any other database request:

1. `PRAGMA schema_version` — captured in the same request as the objects and stored as `Catalog.Version`; pre-execution validation (Issue #21) later compares against it.
2. `SELECT name, type, COALESCE(sql, '') FROM main.sqlite_master WHERE type IN ('table','view') ORDER BY name` — ordinary and virtual tables (`type='table'`) and views only; indexes and triggers are never cataloged.
3. Per object: `SELECT name, type, hidden FROM main.pragma_table_xinfo(?)` — the bound-parameter pragma table-valued function scopes the read to the main schema and keeps object names out of SQL text entirely. `hidden` is carried verbatim: any non-zero value marks the column non-insertable (virtual-table hidden columns use 1; generated columns use 2 for VIRTUAL and 3 for STORED).

Rows are handed to the pure `schema.BuildCatalog(Input)` in `internal/schema/catalog.go`, which owns every rule; the connection layer only gathers. Failures wrap their driver cause with `%w` and return the failed `RequestResult` (with post-error identity reclassification) so the UI can retain stale data and distinguish deletion/replacement (Issue #13).

## Object kinds and write eligibility

| Kind | Detection | Write eligible | Rowid |
|---|---|---|---|
| `ordinary-table` | `type='table'`, SQL not `CREATE VIRTUAL TABLE` | yes | `has-rowid`, or `without-rowid` when the stored SQL ends with `WITHOUT ROWID` |
| `virtual-table` | SQL begins `CREATE VIRTUAL TABLE` (case-insensitive) | yes (visible columns, best effort) | `not-applicable` (module-specific; fts5 exposes docid, others vary) |
| `view` | `type='view'` | never (SELECT-only) | `not-applicable` |

- `WITHOUT ROWID` detection is suffix-based because SQL grammar places the clause last: trailing whitespace, one final semicolon, and a trailing single-line comment are tolerated, and the last two whitespace-separated tokens must be `WITHOUT ROWID` case-insensitively.
- fts5's shadow tables (`*_config`, `*_content`, `*_data`, `*_docsize`, `*_idx`) arrive in `sqlite_master` as ordinary tables and are cataloged as such — `_config` and `_idx` are genuinely `WITHOUT ROWID`, the rest carry rowids. They are not the exclusion's business.
- `RowidShadowed` is true only for has-rowid ordinary tables where a declared column occupies the rowid alias slot `rowid` / `_rowid_` / `oid` (case-insensitive); views and virtual tables never report shadowing.

## Exclusions

`BuildCatalog` never emits:

- any object whose name starts with `sqlite_` (case-insensitive) — SQLite's reserved namespace, including auto-created `sqlite_sequence` and `sqlite_stat%`;
- any object named `_cf_METADATA` (case-insensitive) — Cloudflare D1's internal bookkeeping table.

Both exclusions are verified against fixtures that genuinely contain such objects, so absence in the catalog is meaningful. Everything else eligible is sorted ascending by name for determinism regardless of `sqlite_master` enumeration order.

## Columns: declared type, visibility, insertability

Each `Column` carries:

- `Name` and `DeclaredType` — passed through verbatim from `table_xinfo` (empty when untyped). Declared type is metadata only and deliberately influences nothing: v1 universal value entry has no type-specific behavior, and this package adds no coercion, affinity-based controls, or input hints.
- `Hidden` — true whenever `table_xinfo`'s raw `hidden` value is non-zero, which covers both virtual-table hidden columns and generated columns (VIRTUAL/STORED) without this layer guessing a driver-side split.
- `Insertable` — true only when the column is not hidden **and** the object is write eligible. Views therefore report zero insertable columns and SELECT-only columns even though `table_xinfo` itself lists them. `Object.InsertableCount` is the count; a zero value on an eligible table is the "table has no insertable columns" non-runnable case (Issue #39 consumes it). There is no AUTOINCREMENT-based skip: INTEGER PRIMARY KEY columns are insertable like any other.

## Contracts and boundaries

- **UI independence**: `internal/schema` imports no driver, Bubble Tea, `internal/ui`, or `internal/querybuilder`; it exports plain types (`Catalog`, `Object`, `Column`, `ObjectKind`, `RowidCapability`) and the pure `BuildCatalog`.
- **Driver hiding**: only `internal/connection` touches SQLite; catalog reads ride the shared request boundary.
- **Failure shape**: `DB.ReadCatalog` returns `(catalog, RequestResult)`; non-health failures are wrapped in `catalogError` (`could not refresh: <step>: <cause>`) with the cause preserved for `errors.Is`/`errors.As`, and health classification wins per [session-health.md](session-health.md).
- Later consumers: Table popup refresh (Issue #11), pre-execution schema-version validation (Issue #21), insertability/rowid use in the builder and query generation.

Cross-references: [connection-pool.md](connection-pool.md), [cancellation-infrastructure.md](cancellation-infrastructure.md), [session-health.md](session-health.md), [unit-tests.md](unit-tests.md), [source-code.md](source-code.md).
