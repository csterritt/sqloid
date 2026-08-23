# Sqloid

Sqloid is a keyboard-driven terminal application for exploring and modifying databases. The broader vision includes multiple database engines and free-text SQL, but v1 is deliberately limited to SQLite files and local Cloudflare D1 databases through a structured query builder and data browser.

`Notes/PRD-sqloid.md` is authoritative. This file is a concise product summary; where it omits detail or differs from the PRD, follow the PRD.

## V1 product

- Open an existing SQLite database with `sqloid sqlite <file>` or discover one local D1 database with `sqloid d1`.
- Validate files without creating them, open read-write, and fail clearly for missing, invalid, unreadable, read-only, ambiguous, or locked startup inputs.
- Build SELECT, UPDATE, DELETE, and INSERT statements from schema-backed fields rather than typing SQL.
- Display SELECT output in a frozen-header grid with vertical paging, horizontal scrolling, an independently read count, and clear empty/error states.
- Keep the 20 most recent query states and 20 most recent immutable result snapshots in memory for the current session.
- Save generated queries as executable `.sql` and export result snapshots as CSV or JSON.

## Builder and SQL scope

The builder starts at Command and guides the user through the applicable fields: Table, Column(s), Where, Group By, Order By, Limit, SET assignments, or INSERT values.

- Command uses S/U/D/I for SELECT, UPDATE, DELETE, or INSERT.
- Tables, views, and columns come from refreshed main-schema metadata; views are SELECT-only and internal objects are excluded.
- SELECT projection is either sole wildcard `*` or ordered `(column, aggregate)` entries.
- In an empty Column(s) popup, `*` is selected by default and synthetic `COUNT(*)` appears directly below it. Named columns continue to Value/Count/Min/Max/Avg/Sum selection.
- WHERE supports one assisted predicate with `=`, `!=`, `<`, `<=`, `>`, `>=`, `IS NULL`, `IS NOT NULL`, or `LIKE`.
- GROUP BY is multi-select. Every nonaggregate projected column in a grouped query must be grouped; wildcard with GROUP BY and mixed aggregate/nonaggregate projection without GROUP BY are invalid. All-aggregate projection without GROUP BY is valid.
- ORDER BY supports one valid column or selected aggregate and ASC/DESC. Limit is empty or a positive signed-64-bit integer.
- UPDATE requires one or more completed SET assignments; DELETE has an optional WHERE.
- INSERT offers Value, NULL, or Default/Omit for every insertable column. Omitting all prompted columns emits `DEFAULT VALUES`; a table with no insertable columns cannot run.
- Joins, subqueries, expressions, multiple predicates, AND/OR, IN, HAVING, and free-text SQL are outside v1.

All values use one deterministic parse-and-bind rule: verbatim signed int64 input becomes INTEGER, otherwise finite `strconv.ParseFloat` input becomes REAL, and everything else becomes TEXT. SQLite affinity may then coerce it. Identifiers always come from schema metadata and are double-quoted. V1 does not filter entry or operators by declared column type.

## UI and keys

- Results, including their header, occupy at least half the supported terminal height. The builder uses at most one-third and scrolls to keep the focused field visible.
- Minimum size is 80×24. A smaller terminal preserves application state and offers `q` without confirmation or Ctrl+C with confirmation; resizing back restores the exact context and focus.
- Tab/Shift+Tab and arrows navigate fields; popup navigation and searchable-popup behavior are context-specific.
- Page Up/Down navigates result pages. Shift+Page Up/Down and `,`/`.` scroll horizontally.
- Ctrl+P/N navigates query history; Ctrl+E/Y enters immutable result history.
- Ctrl+S saves the applicable query; Ctrl+X exports an idle result snapshot; Ctrl+W cancels cancellable database work.
- `?` opens contextual help at base level and inserts literally in focused text/search input.
- Ctrl+C opens quit confirmation from every nonterminal context and cancelling it restores the exact suspended state. `q` requests quit without confirmation only on Command or the too-small screen. Neither quit path abandons required request or transaction cleanup.
- Popups, text entry, modals, save flows, pending requests, terminal states, and the too-small screen follow the precedence and context matrix in the PRD.

## SELECT execution, paging, and snapshots

- An active SELECT is separate from an in-flight request and remains pageable while idle.
- The first page and count run concurrently as independent autocommit reads. They can observe different committed states; `Count: N` is informational and never clamps displayed rows.
- At most one page request is pending. Page Up/Down is ignored with loading feedback until it settles. Request IDs and viewport generations reject cancelled, resized, deactivated, or otherwise stale responses.
- Results use one contiguous cache of at most 10,000 absolute logical row positions. Forward traversal evicts the low end; backward traversal evicts the high end; overlap replaces by position; duplicate-valued rows remain separate.
- Snapshot/export rows are ordered by logical position and carry retained-range, endpoint, count, eviction, completeness, terminal-outcome, and warning metadata.
- An active SELECT finalizes exactly once when a new execution starts, result history is entered, cancellation/failure ends it, or quit is accepted. Builder edits, query-history browsing, overlays, resize, and export do not finalize it.

## Write safety

- UPDATE and DELETE first enter a pre-execution preparation workflow. It shows operation, table, rendered SQL, a prominent no-WHERE warning, and an independent estimate of matching target rows generated from the same target and WHERE predicate.
- Confirmation is disabled while estimation runs, then enabled whether the estimate succeeds or fails. Dismissal or estimate cancellation creates no query or result history.
- Confirmation starts the sole actual write execution. INSERT starts directly. Every actual write creates exactly one result entry.
- Writes use an application-controlled transaction. Beginning and statement execution are cancellable; rollback cleanup and COMMIT are not.
- Cancellation wins only before an atomic commit boundary. A write is described as untouched only after rollback is confirmed.
- An unresolved commit or rollback becomes an outcome-unknown terminal state after driver/transaction work ends. Further database work is forbidden, but in-memory save/export and status-1 quit remain available.

## History, saving, and data representation

- Query history appends only when an actual execution starts and normalized state differs from the prior executed state. Opening write preparation does not append history.
- Result history receives one immutable entry per finalized SELECT or actual write, with oldest-first eviction after 20 entries.
- Ctrl+X is unavailable while a request runs. At idle it takes an immutable instant copy without finalizing the active SELECT; cancelling or completing the picker returns to that unchanged result.
- Saving uses a directory picker, separate filename entry, overwrite confirmation, and temp-file-plus-rename replacement so pre-existing files survive pre-rename failures.
- CSV is RFC 4180 UTF-8 with CRLF. JSON is an array of objects. Duplicate output names are deterministically suffixed across grid, CSV, and JSON.
- NULL, BLOB, empty-string, embedded-control, and non-finite REAL behavior is format-specific as defined by the PRD.
- Invalid UTF-8 SQLite TEXT is replaced consistently with U+FFFD in grid, CSV, and JSON, with a UI warning but no extra export records or fields.

## Runtime and implementation

- Go with [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), Bubbles where useful, and [mow.cli](https://github.com/jawher/mow.cli).
- Linux and macOS, pure Go/no cgo, standard xterm-compatible keyboard behavior, and at least 16 colors.
- Five-second SQLite busy timeout. Mid-session lock failures are normal request errors.
- No continuous filesystem watcher: check the original database path before each database request and recheck after request errors. Idle deletion or rename-away is detected on the next operation; same-path replacement is not detected.
- No logging or query timeout in v1. Slow work remains cancellable where its phase permits.
- Automated tests cover builder SQL, schema/discovery, exports, histories, asynchronous response ordering, paging/cache boundaries, key precedence, write preparation and commit boundaries, deletion classification, count drift, invalid UTF-8, and CLI/database integration. Rendering uses the PRD's manual 80×24, 100×30, and 160×50 matrix.

## Post-v1 ideas

- Additional database engines and remote connections.
- Free-text SQL and a full-screen advanced editor for nested or arbitrary queries.
- Frozen columns during horizontal scrolling.
- Other grammar and editing capabilities explicitly deferred by the PRD.
- Log to log file
- 'd1-remote' queries against remote d1
- Read-only file (or locked by another process) => read-only session
