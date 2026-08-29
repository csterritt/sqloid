# Issue #25 — Serialized vertical result paging

Issue #25 adds adjacent Page Up/Down navigation over one active SELECT's logical result: each key requests exactly one adjacent absolute logical range through QueryBuilder's page API, at most one page request is ever pending (while the Issue #24 count request may still settle alongside it), and the page size always equals all complete visible data rows. See `Notes/PRD-sqloid.md` (Paging consistency, Grid rendering/cache, Resize/layout, Module Design, Testing Decisions), and cross-references to [concurrent-page-count.md](concurrent-page-count.md), [first-select-result-grid.md](first-select-result-grid.md), [responsive-tui-shell.md](responsive-tui-shell.md), and [group-order-limit.md](group-order-limit.md).

## Page SQL construction and the ordering policy

`QueryBuilder` remains the sole source of safely rendered SQL (see [group-order-limit.md](group-order-limit.md)); paging extends the existing SELECT rendering path rather than creating a second builder:

- **`PageSQL(pageLimit, offset)`** renders the complete page statement: the base SELECT (quoted projection, WHERE, GROUP BY, and any explicit user ORDER BY kept byte-for-byte), the page LIMIT, and an exact integer OFFSET — `LIMIT <limit> OFFSET <offset>` with both rendered canonically from accepted integers, never interpolated user text. Bound parameters come from `PageParams()`, exactly `SelectParams()` in unchanged order: LIMIT/OFFSET contribute no parameters.
- **User Limit semantics are preserved by clamping, not rewriting**: the rendered LIMIT is the page limit clamped to the remaining user Limit (`min(pageLimit, userLimit - offset)`), so a page request can never read beyond the user's logical Limit. Offsets at or beyond the user's Limit, `pageLimit < 1`, `offset < 0`, and every unrenderable snapshot yield an empty string — never a partially valid query. The user's entered Limit text is never interpolated.
- **Ordering policy — one eligible case only.** The implicit `ORDER BY rowid` fallback is appended exactly when there is **no user ORDER BY** and the selected source is an **ordinary rowid table with no declared `rowid`, `_rowid_`, or `oid` shadow** (schema `KindOrdinaryTable`, `RowidHas`, `!RowidShadowed`), and the query is neither aggregate-only nor grouped. Every other shape stays explicitly **unordered with no stability claim**: views, virtual tables, WITHOUT ROWID tables, every declared-rowid shadow spelling, aggregate-only and grouped queries, ties, and concurrent writes have no implied stability.
- **Explicit user ORDER BY is preserved byte-for-byte** with its ASC/DESC direction and **no appended rowid tie-breaker** — including aggregate/grouped ORDER BY expressions such as `ORDER BY COUNT("id") DESC`, which survive exactly as the user selected them.

The rowid metadata consumed here (kind, capability, declared alias shadowing) is supplied entirely by `internal/schema`'s catalog ([schema-catalog.md](schema-catalog.md)); the QueryBuilder tests build schema objects as plain metadata fixtures and encode no schema classification or UI paging state.

## Page size: exact complete visible rows

The request limit derives from the existing results-area layout arithmetic: `CalculateLayout(height, fields).PageRows`, the results height minus its owned fixed rows (top/bottom border, status/count line, frozen header). This is the exact count of **complete** data rows visible; a partially visible row is never counted. At the supported heights with the standard builder bar this yields 11 rows at 80×24, 15 at 100×30, and 34 at 160×50, and a resize changes the next request's limit to the newly calculated exact value.

## Serialized paging orchestration

`internal/ui/paging.go` owns the orchestration:

- **Exactly the adjacent range.** Page Down requests `LIMIT pageSize OFFSET displayedEnd` where `displayedEnd = pageOffset + len(displayed rows)`; Page Up steps back by one page from the displayed start, clamped to offset zero with a correspondingly exact size (never reading before row 1). Each request carries a fresh nonzero role request ID from `result.NextSelectRequestID()`.
- **Boundaries.** The low boundary (already at offset zero), the high boundary (the last page returned fewer rows than requested), the user's Limit, and a nil Page executor each consume the key without issuing any command.
- **Serialization.** At most one page request is pending at any time, tracked independently from Issue #24's count request. While a page is pending, repeated and opposite page keys are consumed without stacking commands or issuing additional connection requests; the `loading next page…` feedback stays visible, and horizontal column movement remains local (left/right movement issues no page request). The count may settle while a page is pending: its exact `Result count: N` wording applies into its own presentation state without touching the pending page.
- **Settlement.** A page completion installs its rows only when its request ID matches the one pending request; stale or duplicated responses are discarded. Success replaces the displayed page with the new absolute logical range (`rows <offset+1>-<offset+N>` status). A page shorter than the requested size marks the known high boundary. Ordinary failures keep the previous page displayed — their error boundary, viewport-generation handling, and cancellation remain owned by Issues #26/#28.
- **Fresh executions.** Each new SELECT execution resets the paging state: the first page displays from offset zero with no pending request and no boundary knowledge, preserving Issue #22/#24 first-page/count behavior unchanged.

## Execution boundary

`connection.DB.ExecutePage(parent, statement, params)` runs exactly one bound page statement — already carrying its exact LIMIT/OFFSET range from the QueryBuilder seam — as one complete `RunRequest` on its own dedicated leased connection, with eager typed scanning and conversion into the shared `internal/result` page (Issue #6 lifecycle and Issue #7 health classification unchanged, causes preserved through the neutral `firstPageError` wrapper). No adjacent-offset arithmetic, serialization, or caching lives in the connection layer; the UI owns the page range.

## Testing

- `internal/querybuilder/page_sql_test.go` — UI-independent table-driven coverage: exact LIMIT/OFFSET ranges with clamping, parameter-order preservation, the single eligible rowid-fallback case, byte-for-byte user ORDER BY ASC/DESC (plain and aggregate/grouped) with no appended tie-breaker, every excluded object/query category as an explicit no-fallback case, ineligible shapes, and invalid ranges.
- `internal/ui/vertical_paging_test.go` — deterministic scripted model coverage: adjacent Page Down/Up ranges through the page API, low/high/user-Limit boundary suppression, held-pending requests proving repeated/opposite key suppression with no additional request and visible loading feedback, local horizontal movement, count-plus-one-page overlap, and page size equal to the complete visible rows at multiple supported heights plus after resize.
- `internal/connection/page_test.go` — SQLite-backed coverage: adjacent disjoint ranges, offset beyond data as a typed empty page, and ordinary failure classification with the driver cause preserved.
