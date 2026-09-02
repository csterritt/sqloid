# Issue #071 Code Walkthrough: Preserve Absolute Logical Positions for Later-Page Failures

*2026-09-02T15:38:22Z by Showboat 0.6.1*
<!-- showboat-id: 71d731cf-8106-4c51-989b-2cc4928a55e3 -->

Issue #71 (Notes/tasks/071-later-page-logical-failure-offset.md, Notes/PRD-sqloid.md §Paging consistency, §Cache and snapshot invariant, §Connection/UI Module Design, §Testing Decisions) preserves absolute logical positions for later-page failures. One logical zero-based offset is calculated with the page range, rendered into QueryBuilder.PageSQL's LIMIT/OFFSET range, passed explicitly through every executor seam, and used by the shared scanner to report one-based absolute offset + relative index + 1 positions for value-limit failures. This walkthrough traces one nonzero offset from pageRange through PageSQL, the UI executor/production adapter, StartPage, and runFirstPage. It triggers oversized value and page-cap failures at first and later relative rows on multiple pages and shows the exact one-based absolute row-N diagnostics with complete leading rows only. It compares an offset-zero first-page control and includes forward/backward and Limit-clamped contract evidence plus the updated cancellation capability test.

## The explicit offset contract in StartPage

StartPage (internal/connection/started_request.go) now accepts the logical offset explicitly and passes it unchanged to runFirstPage. StartFirstPage stays fixed at offset zero. Both retain the partial page of complete leading rows alongside a typed *result.LimitFailure.

```bash
sed -n '133,175p' internal/connection/started_request.go
```

```output
// StartFirstPage runs one first-page SELECT — the statement and parameters
// must come from QueryBuilder's rendering seam — as an externally
// cancellable request on a dedicated leased connection.
func (db *DB) StartFirstPage(parent context.Context, statement string, params []any) *StartedPageRequest {
	s := &StartedPageRequest{}
	s.started = db.startRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeFirstPage != nil {
			db.beforeFirstPage(ctx, conn) // test-only barrier seam (see DB doc)
		}
		p, err := runFirstPage(ctx, conn, statement, params, 0)
		// Issue #31: a typed value-limit failure still returns the complete
		// leading rows of this page alongside the failure, so the partial page
		// is kept even when the request failed.
		if p != nil {
			s.page = p
		}
		if err != nil {
			return err
		}
		return nil
	})
	return s
}

// StartPage runs one later-page SELECT exactly like StartFirstPage: one
// complete page statement from QueryBuilder's page API on a dedicated
// cancellable leased connection. offset is the count of absolute logical
// result rows before this page (the requested OFFSET), so Issue #31
// value-limit failures report the one-based absolute logical position. It is
// the same offset QueryBuilder rendered into the statement's LIMIT/OFFSET
// range, passed explicitly here rather than parsed from the SQL text.
// StartFirstPage stays fixed at offset zero.
func (db *DB) StartPage(parent context.Context, statement string, params []any, offset int64) *StartedPageRequest {
	s := &StartedPageRequest{}
	s.started = db.startRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		p, err := runFirstPage(ctx, conn, statement, params, offset)
		// Issue #31: a typed value-limit failure still returns the complete
		// leading rows of this page alongside the failure, so the partial page
		// is kept even when the request failed.
		if p != nil {
			s.page = p
		}
		if err != nil {
```

## The shared scanner computes absolute positions from offset

runFirstPage (internal/connection/firstpage.go) uses the offset parameter to compute one-based absolute logical positions for value-limit failures at every scan boundary: execute (offset+1), scan (offset+rowIdx+1), oversized value (offset+rowIdx+1), and iteration (offset+len(complete rows)+1).

```bash
sed -n '72,135p' internal/connection/firstpage.go
```

```output
func runFirstPage(ctx context.Context, conn *sql.Conn, statement string, params []any, offset int64) (*result.Page, error) {
	rows, err := conn.QueryContext(ctx, statement, params...)
	if err != nil {
		// Issue #31: the driver enforces SQLITE_LIMIT_LENGTH during statement
		// execution, so an oversized value can surface here before any row is
		// scanned. The failing row is the page's first: position offset+1.
		if failure := valueLimitFailure(err, offset+1); failure != nil {
			return nil, failure
		}
		return nil, wrapFirstPage("execute "+statement, err)
	}

	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return nil, wrapFirstPage("read columns", err)
	}

	scan := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range dest {
		dest[i] = &scan[i]
	}
	raw := make([][]any, 0, 64)
	for rowIdx := int64(0); rows.Next(); rowIdx++ {
		if err := rows.Scan(dest...); err != nil {
			if failure := valueLimitFailure(err, offset+rowIdx+1); failure != nil {
				// Typed Issue #31 failure: return the complete leading rows
				// plus the typed error; the oversized row is not exposed.
				rows.Close()
				partial := result.FromDriver(columns, raw)
				return &partial, failure
			}
			rows.Close()
			return nil, wrapFirstPage("scan row", err)
		}
		row := make([]any, len(scan))
		copy(row, scan)
		if pos := oversizedValue(row); pos {
			rows.Close()
			partial := result.FromDriver(columns, raw)
			var failure error = &result.LimitFailure{Kind: result.KindValue, Position: offset + rowIdx + 1}
			return &partial, failure
		}
		raw = append(raw, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		// Issue #31: the driver can also enforce the length limit while the
		// iteration is stepping. The row that failed is the one after every
		// completely scanned row, so its one-based absolute logical position
		// is offset+len(complete rows)+1, and only the complete leading rows
		// come back — never a partial row.
		if failure := valueLimitFailure(err, offset+int64(len(raw))+1); failure != nil {
			partial := result.FromDriver(columns, raw)
			return &partial, failure
		}
		return nil, wrapFirstPage("iterate rows", err)
	}
	rows.Close()

	converted := result.FromDriver(columns, raw)
	return &converted, nil
}
```

## The PageExecutor contract carries the offset explicitly

The PageExecutor type in internal/ui/paging.go now carries (sql, params, logicalOffset) from the single pageRange calculation. handlePageKey passes the exact pageRange offset to the executor.

```bash
sed -n '26,42p' internal/ui/paging.go && echo '---' && sed -n '96,108p' internal/ui/paging.go
```

```output
// PageExecutor performs one cancellable paged-page SELECT execution for the
// given safely rendered page statement (QueryBuilder's PageSQL, whose exact
// LIMIT/OFFSET range replaces the user's Limit clause), ordered bound
// parameters, and the logical offset (the count of absolute logical result
// rows before this page, identical to the OFFSET rendered into the
// statement). The offset is passed explicitly so Issue #31 value-limit
// failures report the one-based absolute logical position; it is never parsed
// from the statement text. It always runs inside a tea.Cmd — never in Update
// or View.
type PageExecutor func(ctx context.Context, sql string, params []any, offset int64) FirstPageResult

// PageSettledMsg carries one settled paged-page execution back through
// Update with the full identity (Issues #25 and #26) that guards it: the
// SELECT execution ID it ran under, its page request ID, and the viewport
// generation current at dispatch. Produced only by commands this package
// created; it mutates state only while every applicable identity is still
// current.
---
	params := m.QB.PageParams()
	exec := m.Page
	execution, generation := m.pageRequestExecution, m.pageRequestGeneration
	return func() tea.Msg {
		return PageSettledMsg{
			ExecutionID: execution,
			RequestID:   requestID,
			Generation:  generation,
			Result:      exec(pageCtx, statement, params, offset),
		}
	}
}

```

## The production adapter passes the offset to ExecutePage

pageAdapter in internal/session/session.go passes the supplied offset unchanged to connection.DB.ExecutePage — never parsing it from the SQL text or deriving it from displayed rows.

```bash
sed -n '136,151p' internal/session/session.go
```

```output
// pageAdapter returns the paged-page SELECT executor that runs one
// ExecutePage through db and maps the typed connection.RequestResult onto
// ui.FirstPageResult. offset is the count of absolute logical result rows
// before this page (the requested OFFSET), identical to the OFFSET encoded in
// the statement's LIMIT/OFFSET range by QueryBuilder's page API. It is passed
// explicitly to ExecutePage for Issue #31 value-limit position reporting so
// the scanner reports the one-based absolute logical position; it is never
// parsed from the statement text.
func pageAdapter(db *connection.DB) ui.PageExecutor {
	return func(ctx context.Context, sql string, params []any, offset int64) ui.FirstPageResult {
		page, res := db.ExecutePage(ctx, sql, params, offset)
		return mapFirstPage(page, res)
	}
}

// mapFirstPage converts one connection first-page or paged-page result into
```

## Contract evidence: StartPage value-limit failures at absolute positions

The connection tests prove StartPage receives nonzero logical offsets and reports absolute value-limit failure positions. Several nonzero OFFSET values exercise first and later page-relative rows. The oversized row is at absolute position 4 in a 3-small-row fixture.

```bash
go test ./internal/connection/ -run 'TestStartPageValueFailureAbsolutePositionWithNonZeroOffset' -count=1 -v 2>&1
```

```output
=== RUN   TestStartPageValueFailureAbsolutePositionWithNonZeroOffset
=== RUN   TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/first_relative_row_at_offset_3
=== RUN   TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/second_relative_row_at_offset_2
=== RUN   TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/third_relative_row_at_offset_1
--- PASS: TestStartPageValueFailureAbsolutePositionWithNonZeroOffset (0.81s)
    --- PASS: TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/first_relative_row_at_offset_3 (0.14s)
    --- PASS: TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/second_relative_row_at_offset_2 (0.52s)
    --- PASS: TestStartPageValueFailureAbsolutePositionWithNonZeroOffset/third_relative_row_at_offset_1 (0.14s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.813s
```

## Offset-zero first-page control

StartFirstPage and StartPage with offset zero behave identically: the oversized value at position 4 fails at position 4, with 3 complete leading rows retained.

```bash
go test ./internal/connection/ -run 'TestStartPageOffsetZeroControlMatchesFirstPage|TestStartFirstPageOffsetZeroControlUnchanged' -count=1 -v 2>&1
```

```output
=== RUN   TestStartPageOffsetZeroControlMatchesFirstPage
--- PASS: TestStartPageOffsetZeroControlMatchesFirstPage (0.33s)
=== RUN   TestStartFirstPageOffsetZeroControlUnchanged
--- PASS: TestStartFirstPageOffsetZeroControlUnchanged (0.14s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.477s
```

## UI executor receives the correct offset for forward, backward, and Limit-clamped paths

The page executor fake records the structured logical offset alongside SQL and parameters. The tests require it to equal the PageSQL OFFSET for forward, backward, nonzero-current-page, Limit-clamped, and resize/refetch paths.

```bash
go test ./internal/ui/ -run 'TestPageExecutorReceivesOffsetMatchingPageSQL' -count=1 -v 2>&1
```

```output
=== RUN   TestPageExecutorReceivesOffsetMatchingPageSQL
=== RUN   TestPageExecutorReceivesOffsetMatchingPageSQL/forward_from_first_page
=== RUN   TestPageExecutorReceivesOffsetMatchingPageSQL/backward_from_nonzero_page
=== RUN   TestPageExecutorReceivesOffsetMatchingPageSQL/forward_from_nonzero_page
=== RUN   TestPageExecutorReceivesOffsetMatchingPageSQL/resize_refetch_uses_new_offset
--- PASS: TestPageExecutorReceivesOffsetMatchingPageSQL (0.00s)
    --- PASS: TestPageExecutorReceivesOffsetMatchingPageSQL/forward_from_first_page (0.00s)
    --- PASS: TestPageExecutorReceivesOffsetMatchingPageSQL/backward_from_nonzero_page (0.00s)
    --- PASS: TestPageExecutorReceivesOffsetMatchingPageSQL/forward_from_nonzero_page (0.00s)
    --- PASS: TestPageExecutorReceivesOffsetMatchingPageSQL/resize_refetch_uses_new_offset (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

## UI-visible absolute row-N messages for value and page-cap failures

Injected value and page LimitFailure outcomes at known page-relative rows produce the UI-visible exact absolute row-N message. The value failure at offset 3 + relative 0 + 1 = row 4, and the page-cap failure at offset 14 + relative 2 + 1 = row 17.

```bash
go test ./internal/ui/ -run 'TestPageExecutorValueLimitFailureShowsAbsoluteRow|TestPageExecutorPageLimitFailureShowsAbsoluteRow' -count=1 -v 2>&1
```

```output
=== RUN   TestPageExecutorValueLimitFailureShowsAbsoluteRow
=== RUN   TestPageExecutorValueLimitFailureShowsAbsoluteRow/first_relative_row_at_offset_3
=== RUN   TestPageExecutorValueLimitFailureShowsAbsoluteRow/second_relative_row_at_offset_3
=== RUN   TestPageExecutorValueLimitFailureShowsAbsoluteRow/third_relative_row_at_offset_14
--- PASS: TestPageExecutorValueLimitFailureShowsAbsoluteRow (0.00s)
    --- PASS: TestPageExecutorValueLimitFailureShowsAbsoluteRow/first_relative_row_at_offset_3 (0.00s)
    --- PASS: TestPageExecutorValueLimitFailureShowsAbsoluteRow/second_relative_row_at_offset_3 (0.00s)
    --- PASS: TestPageExecutorValueLimitFailureShowsAbsoluteRow/third_relative_row_at_offset_14 (0.00s)
=== RUN   TestPageExecutorPageLimitFailureShowsAbsoluteRow
--- PASS: TestPageExecutorPageLimitFailureShowsAbsoluteRow (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.008s
```

## Production adapter passes the offset to the real connection

The wired Page seam in session.Compose passes the supplied logical offset to connection.DB.ExecutePage, so an oversized value on a later page fails at the one-based absolute logical position against the real database.

```bash
go test ./internal/session/... -run 'TestComposePageExecutorPassesOffsetToConnection' -count=1 -v 2>&1
```

```output
=== RUN   TestComposePageExecutorPassesOffsetToConnection
--- PASS: TestComposePageExecutorPassesOffsetToConnection (0.12s)
PASS
ok  	github.com/chris/sqloid/internal/session	0.125s
```

## Updated cancellation capability test

The later-page cancellation capability test (TestCapabilityLaterPageCancelInterruptsWithinOneSecond) now passes the logical offset to StartPage, matching the PageSQL OFFSET (probeTableRows/2). The cancellation contract is unchanged — the request still settles within the mandatory one-second bound.

```bash
go test ./internal/connection/ -run 'TestCapabilityLaterPageCancelInterruptsWithinOneSecond' -count=1 -v 2>&1
```

```output
=== RUN   TestCapabilityLaterPageCancelInterruptsWithinOneSecond
--- PASS: TestCapabilityLaterPageCancelInterruptsWithinOneSecond (10.27s)
PASS
ok  	github.com/chris/sqloid/internal/connection	10.268s
```

## Full verification

All tests pass across the connection, UI, and session packages.

```bash
go vet ./... && go build ./... && go test ./internal/connection/ ./internal/ui/ ./internal/session/... -count=1 2>&1 | tail -10
```

```output
ok  	github.com/chris/sqloid/internal/connection	42.083s
ok  	github.com/chris/sqloid/internal/ui	0.167s
ok  	github.com/chris/sqloid/internal/session	0.328s
```
