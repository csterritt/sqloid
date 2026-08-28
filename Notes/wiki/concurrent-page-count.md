# Issue #24 — Concurrent first page and independent result count

Issue #24 makes one actual SELECT launch two concurrent requests — the first page and the complete-limited-result count — each on its own dedicated lease from the exact-two pool, running as independent autocommit reads with no shared snapshot. See `Notes/PRD-sqloid.md` (Paging consistency, Connection pool, Execution and Result Lifecycle, Module Design, Testing Decisions), and cross-references to [first-select-result-grid.md](first-select-result-grid.md), [connection-pool.md](connection-pool.md), [cancellation-infrastructure.md](cancellation-infrastructure.md), and [session-health.md](session-health.md).

## First-page and count request construction

`QueryBuilder` remains the sole source of safely rendered SQL (see [group-order-limit.md](group-order-limit.md)):

- **First page** — `SelectSQL()`/`SelectParams()` unchanged: quoted projection, WHERE, GROUP BY, ORDER BY, LIMIT in grammar order, with the WHERE predicate's ordered bound parameters.
- **Count** — `CountSQL()` renders exactly `SELECT COUNT(*) FROM (<SelectSQL()>)`: the complete SELECT is counted as a subquery, **with the user's LIMIT inside the subquery**, so rows beyond the Limit are irrelevant to completeness. The wording never implies a table count or a pre-Limit count. `CountParams()` returns exactly `SelectParams()` — identical values in identical order — so one execution seam serves both requests. A snapshot that cannot render a SELECT (non-SELECT command, missing pieces) yields an empty count string, never a partially valid statement.

## Execution identities

One actual SELECT execution receives exactly one fresh nonzero **SELECT execution ID** (`result.NextSelectExecutionID()`), and its two concurrent requests each receive a fresh nonzero **role-specific request ID** (`result.NextSelectRequestID()`): one for the first page (`RoleFirstPage`) and one for the count (`RoleCount`). IDs are monotonically increasing and never reused, and the two roles have no interchangeable identity — a request ID issued for one role is never accepted for the other. The zero value never identifies anything, so an unassigned `SelectTracker` rejects every completion.

The `result.SelectTracker` guards settlement with the **two-level identity rule**: a page or count completion mutates active state only when both its execution ID and its role-specific request ID match the current identities, and each role is consumed at most once. Delayed responses from superseded executions, wrong-role IDs, duplicate responses, and cancelled messages are discarded without touching current rows, count text, history, or errors.

## Concurrent execution on two dedicated leases

`connection.DB.RunFirstPage` and the new `connection.DB.RunCount` each route through the shared `RunRequest` boundary (Issue #6 lifecycle, Issue #7 health classification) and lease one physical connection from the exact-two pool (Issue #5, see [connection-pool.md](connection-pool.md)). Both calls are launched together from the UI (`tea.Batch`) in `startSelectPage()` without waiting for either result; there is no shared mutex, transaction, single worker queue, or request sequencing tying them together.

`RunCount` scans exactly one row — the wrapped complete-SELECT `COUNT(*)` — as an `int64`. Count failures wrap their cause in the package's own typed `countError` (`could not run count — <step>: <cause>`), preserving driver causes via `%w`; outcome classification and health precedence stay owned by `RunRequest`, and a count failure is never reinterpreted as a page failure (or vice versa).

### Journal modes, snapshots, and drift

In both WAL and rollback-journal modes the two reads run concurrently as independent autocommit reads:

- **No shared snapshot** — the page and the count may observe the database at different instants. The PRD-permitted drift (count greater than, less than, or otherwise inconsistent with fetched positions) is an accepted outcome, never reconciled, and rows are **strictly never clamped** to an inconsistent count.
- **External writers** — an interleaved external writer may commit between the two reads (WAL: unblocked; rollback journal: delayed until the read's lease is released or failing with the ordinary `database is locked` error). Both outcomes are accepted; no retry behavior hides them.
- **Journal mode untouched** — Sqloid issues no journal-changing pragmas; `Open` records and the capability suite verifies the mode before and after concurrent page/count runs. Busy handling remains the established five-second per-connection busy timeout.
- **Lease hygiene** — each request retains its lease through true settlement and releases it on every success, error, and cancellation path; a third lease cannot be admitted while both are held (pool stays exactly two). The barrier hooks used to prove overlap (`DB.beforeFirstPage`, `DB.beforeCount`) are documented test-only seams: production control flow leaves them nil.

## UI state and presentation

The model stores the current execution's `SelectTracker` plus an explicit `countState result.CountState`; rendering composes the status/count line from explicit state and never infers it from row length:

- **Pending** — exactly `Counting rows…` while the count request is in flight.
- **Success without a user Limit** — exactly `Result count: N`.
- **Success with a user Limit** — exactly `Result count: N (after Limit M)`, with Limit metadata captured at launch from the executed builder (`QB.LimitValue()`), so later builder edits cannot change the wording of an already-settled count.
- **Failure** — exactly `Count unavailable`. Successful rows, their grid, paging capability, and history are retained unchanged; the count failure never converts the active SELECT into a page failure, and a first-page failure follows its ordinary result-error boundary independently of count completion.

The status/count line composes the exact count wording with the independently displayed range (`Result count: N — rows 1-N`); no inconsistent total clamps, extends, or reorders the displayed rows. Help (`CountHelpLines()`) records that the count covers the complete limited SELECT — the user's Limit stays inside the counted subquery — not the table size or a pre-Limit size, that page and count are independent autocommit reads with no shared snapshot and may drift, and that rows are never clamped.

## Tests

- `internal/querybuilder/count_sql_test.go` — exact count subquery with Limit inside, bound-parameter preservation, aggregate/grouped SELECTs, non-SELECT emptiness.
- `internal/result/count_test.go` — exact wording variants, zero-value renders nothing.
- `internal/result/select_identity_test.go` — distinct monotonic IDs, two-level acceptance, wrong-role/duplicate/stale rejection.
- `internal/ui/concurrent_count_test.go` — scripted launch identities, count-before-page and after-page settlement, after-Limit wording, `Count unavailable` isolation, page-failure independence, stale/superseded response rejection, delayed count after a newer execution, help content.
- `internal/connection/count_overlap_test.go` — mandatory capability suite: WAL and rollback-journal fixtures with recorded pre-open mode, simultaneous barrier-held overlap proving genuine concurrency, distinct physical connections, third-lease block, unchanged journal mode, independent-snapshot drift across an external writer, rollback-journal delay or ordinary lock error, count-failure isolation, and clean lease release.

See [unit-tests.md](unit-tests.md) and [source-code.md](source-code.md) for the complete catalogs. Later-page identities, viewport generations, cache behavior, and paging navigation belong to Issue #26 and later issues and are deliberately absent here.
