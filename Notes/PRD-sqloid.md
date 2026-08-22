# PRD: Sqloid v1

## Problem Statement

Inspecting and querying a SQLite database currently means dropping down to the `sqlite3` shell or an external GUI. The shell demands that every query be typed out by hand, column names memorised, and results are hard to read, page through, or export. There is no lightweight, keyboard-driven tool that lets a developer explore a database's schema, build common queries interactively, view results in a scrollable grid, and export them — whether the database is a plain SQLite file or a local Cloudflare D1 database created by wrangler/miniflare.

## Solution

Sqloid v1 is a terminal UI application (single user, single developer — meaning no authentication, collaboration, or server behaviour) that opens a SQLite database — either given explicitly on the command line, or discovered in a local wrangler D1 state directory — and presents a full-screen interface: results occupy most of the screen, and a query-builder bar sits at the bottom.

The original project vision was a general SQL editor in which the user types SQL commands. For v1 this is deliberately narrowed to a structured query builder / data browser: the guided, field-by-field builder is what makes the tool valuable (no memorised schemas, type-aware entry, safe parameter binding), and free-text SQL entry is deferred to a post-v1 advanced mode.

The user builds SELECT, UPDATE, DELETE, and INSERT queries by stepping through fields: command, table (chosen from a popup list of actual tables), column(s) (chosen from a popup list, with optional aggregates), where, group by, order by, and limit. Values are entered through a universal text entry with parse-and-bind coercion (see Value Entry below). Queries run with Enter; results are shown in a grid with a frozen header row, page up/down vertical paging and horizontal scrolling, a total row count in the header, and full history of past queries and results navigable by keybinding. Queries and results can be exported to files (`.sql` for queries; CSV or JSON for results). Errors replace the results view and are themselves navigable via result history.

## Query Grammar (v1)

Sqloid v1 is explicitly a **builder for a subset of SQL**, not a full query engine. The supported grammar is:

- **SELECT**: `SELECT <projection> FROM <table> [WHERE <predicate>] [GROUP BY <column, ...>] [ORDER BY <column-or-aggregate> [ASC|DESC]] [LIMIT <n>]`
  - The projection consists of **entries**, each being one of:
    - a **wildcard projection** `*` — which must be the only projection entry (choosing it clears any prior entries); no aggregate popup is offered for it; or
    - a **column entry**: `(column, aggregate?)` where aggregate is empty (`Value`) or from the fixed enum {Count, Min, Max, Avg, Sum}. Different aggregates on the same column may coexist (`Value(age)` + `AVG(age)`); re-selecting an identical pair adds nothing.
  - When the projection contains no column entries, the aggregate popup additionally offers a special `COUNT(*)` entry (the only way to produce bare `COUNT(*)`). `COUNT(*)` is an ordinary column entry whose column is the sentinel `*` and may coexist with other aggregate entries. `MIN(*)`, `MAX(*)`, `AVG(*)`, and `SUM(*)` are never offered.
  - Removing an entry removes the most recently added one. Reordering is not supported in v1.
- **WHERE (SELECT/UPDATE/DELETE)**: exactly one predicate — `<column> <operator> <value>` — with operators from a fixed popup: `=`, `!=`, `<`, `<=`, `>`, `>=`, `IS NULL`, `IS NOT NULL`, `LIKE`. All operators are offered for all columns. `IS NULL` / `IS NOT NULL` take no value. No AND/OR, parentheses, or IN in v1.
- **GROUP BY**: multi-select of the table's columns; duplicates prevented; empty means no GROUP BY.
- **ORDER BY**: a single column with a toggleable direction (default ascending). In aggregate/grouped queries the popup offers only Group By columns and selected aggregate entries (emitted as their aggregate expression).
- **LIMIT**: integers 1 to 9,223,372,036,854,775,807 only; zero and overflow rejected with an inline message; empty means unbounded (see Paging).
- **UPDATE**: `UPDATE <table> SET <column> = <value>, ... [WHERE <predicate>]` — SET columns chosen by multi-select popup, one new value entered per column.
- **DELETE**: `DELETE FROM <table> [WHERE <predicate>]`.
- **INSERT**: `INSERT INTO <table> (<column>, ...) VALUES (<value>, ...)` — one value entry per non-skipped column; if every prompted column is Default/Omit, Sqloid emits `INSERT INTO "table" DEFAULT VALUES` instead. No WHERE/GROUP BY/ORDER BY/LIMIT fields are shown for INSERT.

Joins, subqueries, expressions, IN, HAVING, and multiple WHERE predicates are all outside the v1 grammar (see Out of Scope).

LIKE values are bound verbatim: `%` and `_` act as SQLite wildcards and there is no escaping mechanism in v1 (searching for a literal `%` is not possible — documented limitation). Case behaviour is SQLite's default; no custom collation is implied. This is unrelated to the case-insensitive *popup search* behaviour.

## Execution Lifecycle

All executions (queries, counts, and writes) follow one observable lifecycle:

1. **Start**: Enter triggers at most one logical execution, which owns up to two concurrent requests (first page + count) for SELECT, or one request for writes/counts. Each execution carries an operation ID.
2. **While in flight**: Enter is ignored (the results header hints "running — Ctrl+W to cancel"). Builder fields remain editable but changes do not affect the running execution.
3. **Cancellation**: Ctrl+W cancels the whole logical execution — both page and count requests together, at any phase (including rows-arrived-count-pending). Ctrl+W while the destructive-write pre-flight estimate runs cancels the estimate and closes the modal.
4. **Late responses**: responses arriving after completion, cancellation, or supersession are discarded by operation ID.
5. **End — exactly one history entry per ended execution**:

| Outcome | Entry contents | Exportable? |
|---|---|---|
| Full success | rows snapshot | yes |
| Count failed, rows fine | rows snapshot (header showed count error) | yes |
| Page failure mid-scan | snapshot of rows fetched so far, marked "failed at row N" | yes |
| Cancelled before any rows | "Cancelled" marker entry | no |
| Cancelled after rows arrived | rows snapshot marked "Cancelled" | yes |
| First-page failure / write failure | error entry (message + query) | no |

All entries are reachable via Ctrl+E/Ctrl+Y; snapshots are immutable once appended.

6. **Active result cache vs snapshot**: while an execution's result is active, paging fetches new pages into a sliding-window cache capped at 10,000 rows; Page Down past the retained window evicts the oldest cached pages. Page Up re-fetches evicted pages as fresh autocommit reads (consistent with the no-held-transaction model; concurrent-write drift applies to re-fetches as documented below). The snapshot freezes when the execution ends — when the user navigates away to build or execute a new query, or on error/cancellation — containing whatever ≤10,000 distinct rows were retained, marked truncated if the logical total exceeded that. Export contains exactly the retained snapshot rows; to export a bounded subset deliberately, set a Limit first.

## Context / Key Matrix

Every key's behaviour per focus context:

### Builder fields (nothing modal/popup/text-entry open)

| Context | Keys |
|---|---|
| Command field | `S/U/D/I` (case-insensitive) choose/replace command; other printable keys ignored; `q` quits immediately |
| Column(s) field | Backspace/Delete removes the most recently added projection entry (one per press); nothing when empty |
| Where / Order By / Limit / Group By / SET columns fields | Backspace/Delete clears the whole field |
| Order By field | Up/Down toggles direction ASC↔DESC |
| All fields | Left/Right and Tab/Shift+Tab move between fields |
| Enter (state runnable) | run query |

Up/Down are used only inside popups and on the Order By field; elsewhere in the builder they do nothing.

### Popups

- **Single-select** (table, column, aggregate, operator): Up/Down move; printable characters fuzzy-search in searchable popups (table, column, GROUP BY, ORDER BY — case-insensitive subsequence match; empty query shows all; no matches shows "no matches" and keeps the popup open; selection resets on query change); aggregate/operator popups are deliberately scroll-only. Enter selects; Esc cancels without changing the field.
- **Multi-select** (Column(s), GROUP BY, SET columns): Enter adds the selection and re-shows the popup; Esc closes while preserving selections made so far. Selections are visible in the field behind the popup.

### Text entry (Where value, UPDATE values, INSERT Value choice, Limit, save filename)

Printable keys insert; Backspace/Delete delete; Enter submits; Esc cancels unchanged (restoring the prior value).

### Modals (quit confirm, destructive-write confirm, overwrite confirm)

Enter/y confirm; Esc/n cancel; Ctrl+C confirms quit in the quit modal and cancels any other modal back to the builder. Modals never stack.

### Write-flow revision

Shift+Tab/arrows return freely to earlier UPDATE SET prompts and INSERT column prompts; previously entered choices/values are pre-filled for revision.

### Terminal error state (database file deleted)

Ctrl+S/Ctrl+X export from memory (initial selection: last executed query / last result snapshot; Ctrl+P/N and Ctrl+E/Y navigate history to select another entry); `?` lists the reduced key set; only `q` quits normally, or Ctrl+C which quits immediately with exit status 1 (no confirmation).

## User Stories

1. As a user, I want to run `sqloid sqlite <file>`, so that I can open a SQLite database file directly.
2. As a user, I want an error when no file argument is given to the `sqlite` command, so that I know immediately what went wrong.
3. As a user, I want an error when the given file does not exist, so that I don't end up in an empty session against a typo.
4. As a user, I want an error when the given file is not a working SQLite database, so that I don't get cryptic failures later.
5. As a user, I want to run `sqloid d1`, so that I can open the local D1 database created by wrangler without hunting for its path.
6. As a user, I want the program to look in the `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` directory for a single SQLite database file (ignoring files with `metadata` in the name, and `-shm`/`-wal` files), so that the right database is picked automatically.
7. As a user, I want a clear error ("There is more than one SQLite database in .wrangler") when the d1 directory contains more than one candidate database, so that I can clean it up.
8. As a user, I want an error when no candidate database exists in the d1 directory, so that I know there is nothing to open.
9. As a user, I want the database opened read-write, so that UPDATE, DELETE, and INSERT statements actually persist.
10. As a user, I want the connection monitored so that the program errors out if the database file is deleted mid-session, so that I don't silently work against a dead handle.
11. As a user, I want to quit the program at any time, so that I'm never trapped in the UI.
12. As a user, I want to hit `q` while on the Command field to quit immediately without confirmation, so that quitting is fast.
13. As a user, I want Ctrl+C to show a "Quit?" confirmation modal (when idle or during execution), so that I don't lose in-progress work accidentally.
14. As a user, I want Ctrl+W while a query or count is running to cancel the whole logical execution (page and count together), showing "Cancelled" in the results view, so that a slow query never traps me. Ctrl+C during execution instead shows the "Quit?" confirmation modal, matching its idle behaviour.
15. As a user, I want the UI to stay interactive while a query runs, with "Running..."/"Counting rows..." shown, so that execution doesn't freeze the interface.
16. As a user, I want Enter to be ignored while an execution is in flight (with a hint to cancel via Ctrl+W), so that heavy queries can't accidentally stack against the same file.
17. As a user, I want the results to occupy the top majority of the screen and the field bar the bottom, so that I can see data while building queries.
18. As a user, I want the field bar to grow to multiple lines as fields gain content, so that long WHERE clauses remain visible.
19. As a user, I want to start in the Command field, so that building a query has an obvious entry point.
20. As a user, I want a single keypress of `S`, `U`, `D`, or `I` in the Command field to expand immediately to Select, Update, Delete, or Insert, so that command choice is instant.
21. As a user, I want to return to the Command field with arrows/tab and press a different letter to replace my command choice, so that changing command type is easy.
22. As a user, I want switching to a write command to clear the Table field when the retained table is a view (focus moving to Table), while retaining eligible ordinary/virtual tables across switches, so that the builder never holds a table my command can't target.
23. As a user, I want the UI to move to the Table field after the command is chosen, so that the flow proceeds naturally.
24. As a user, I want a popup list of the database's tables and views (main schema only; `sqlite_%` and `_cf_METADATA` excluded), refreshed each time it opens, with scrolling and case-insensitive fuzzy search (subsequence match; empty query shows all; no matches shows "no matches" and keeps the popup open; selection resets on query change), so that I can pick a table quickly and see external changes.
25. As a user, I want a popup list of the chosen table's columns after the table is picked, with `*` at the top selected by default, so that plain `SELECT *` is the fastest path.
26. As a user, I want choosing `*` to mean plain wildcard projection — sole entry, no aggregate prompt — so that `SELECT *` stays unambiguous.
27. As a user, I want Enter on a column to show the aggregate popup ({Value, Count, Min, Max, Avg, Sum}, Value default), then add the result to the column list and re-show the column popup, so that I can build multi-column and aggregate projections quickly.
28. As a user, I want a special `COUNT(*)` choice offered when no columns are selected yet, so that bare total-count queries are reachable, coexisting with other aggregates like `MAX(age)`.
29. As a user, I want Enter on the Column(s) field (after one or more entries are chosen) to run the query, so that running is fast.
30. As a user, I want tab/arrow keys to move to the next field instead of running, so that I can refine Where/Order By/Limit first.
31. As a user, I want the Where field to be assisted — column popup first, then an operator popup, then typed value entry — so that valid predicates are easy to write.
32. As a user, I want the Group By field to be an assisted multi-select of columns, empty meaning no GROUP BY, so that aggregate queries are straightforward.
33. As a user, I want the query to refuse to run when the column selection mixes aggregates and non-aggregated columns and not every non-aggregated column appears in Group By, so that I never run an ambiguous aggregate query.
34. As a user, I want Order By to be a single column with a toggleable sort direction (ascending/descending), and Limit to accept positive integers only, so that sorting and capping results are simple and unambiguous.
35. As a user, I want ORDER BY choices in aggregate/grouped queries restricted to grouped columns and selected aggregates, so that ordering never reintroduces ambiguous bare columns.
36. As a user, I want the UPDATE flow to be: pick table, pick SET column(s), enter a new value per column, optional assisted WHERE, then confirmation before running, so that destructive writes are deliberate.
37. As a user, I want the DELETE flow to be: pick table, assisted WHERE, then confirmation before running, so that I can't delete by accident.
38. As a user, I want the INSERT flow to be: pick table, then one value prompt per non-skipped column, so that adding rows is guided.
39. As a user, I want every typed value bound by a single rule — valid integer → INTEGER, valid decimal/exponent number → REAL, otherwise TEXT — with SQLite's column affinity coercing as needed, so no legitimate value is rejected.
40. As a user, I want a confirmation modal before any UPDATE or DELETE runs, showing the operation type, table name, the rendered SQL with literal values, and a pre-flight estimated row count (non-binding) obtained by a `SELECT COUNT(*)` wrapper, so that destructive operations are always informed as well as deliberate.
41. As a user, I want the estimate to appear as "Estimating affected rows…" with confirm disabled until it completes, and if the estimate fails to still allow confirmation (with SQL and warnings shown), so that a contended database never blocks a deliberate write outright.
42. As a user, I want unqualified UPDATE/DELETE (no WHERE) to be allowed but loudly flagged in the confirmation modal with a warning that all rows in the table will be affected, so that legitimate bulk changes are possible but never accidental.
43. As a user, I want confirmation modals confirmed with Enter/y and dismissed with Esc, so that modal interaction is consistent everywhere.
44. As a user, I want each INSERT column prompt to offer a choice of {Value, NULL, Default/Omit} — Value opening text entry, NULL binding an explicit NULL, Default/Omit excluding the column — so that I can distinguish omitted/defaulted columns from empty strings and NULLs.
45. As a user, I want Default/Omit on every column to produce `INSERT INTO ... DEFAULT VALUES`, so that fully-defaulted inserts work.
46. As a user, I want to navigate back to earlier UPDATE SET or INSERT prompts with prior choices pre-filled, so that revising write values before running is easy.
47. As a user, I want Enter to run the query, so that execution has one obvious trigger.
48. As a user, I want Page Up/Page Down to page through results vertically, so that I can navigate large result sets.
49. As a user, I want Shift+Page Up/Down (and `,`/`.`) to scroll results left/right, so that I can see wide tables even when the terminal intercepts Shift+Page keys.
50. As a user, I want the column header row frozen when scrolling vertically, so that I always know what the columns are.
51. As a user, I want the header to show the logical result row count (i.e., after any user Limit) and the range currently displayed, so that I know where I am in the data.
52. As a user, I want the page size computed from the terminal height, so that paging always fills the screen.
53. As a user, I want layout and paging recomputed on terminal resize, preserving the first visible row index where possible, so that resizing doesn't lose my place.
54. As a user, I want to browse beyond 10,000 fetched rows via a sliding window (older cached pages evicted, Page Up re-fetching them fresh), so that unlimited browsing doesn't exhaust memory.
55. As a user, I want Ctrl+P/Ctrl+N to scroll through previous queries, so that I can revisit earlier work.
56. As a user, I want selecting a previous query to repopulate all builder fields exactly as if I had just entered it, so that I can tweak and re-run.
57. As a user, I want only actual changes (different columns, changed WHERE, etc.) to add a new entry to the query history, so that history isn't polluted by identical re-runs.
58. As a user, I want Ctrl+E/Ctrl+Y to scroll through previous results, so that I can refer back to earlier output.
59. As a user, I want query errors to replace the current results view, so that I always see what happened.
60. As a user, I want to navigate back to previous (successful) results via result history after an error, so that an error doesn't destroy my context.
61. As a user, I want UPDATE/DELETE results to show the rows affected along with the query that produced the change, so that writes are auditable in the UI.
62. As a user, I want INSERT results to show the row add count, so that I know the insert succeeded.
63. As a user, I want write-statement results included in the result history, so that I can page back through them like query results.
64. As a user, I want an empty SELECT result to show a message rather than an empty grid, so that "no rows" is unambiguous.
65. As a user, I want Ctrl+S to save the current runnable query (or the last executed one, or a viewed historical result's query) as a plain `.sql` text file (with safely serialized literals — see Saving), so that I can keep useful queries.
66. As a user, I want `?` to open a help modal listing all keybindings for the current context, so that the key set is discoverable.
67. As a user, I want key behaviour documented per context (field focus, popup open, modal open, text entry), so that Enter/arrows/Escape behave predictably wherever I am.
68. As a user, I want Ctrl+X to save the current result, prompting for a file name and a type (CSV or JSON), so that I can export data.
69. As a user, I want saving to use a file picker for the directory and text entry for the file name, so that choosing a destination is easy.
70. As a user, I want a confirmation prompt when saving over an existing file, so that I don't clobber files by accident.
71. As a user, I want failed saves to leave any pre-existing destination file untouched, so that a mid-write failure never corrupts my files.
72. As a user, I want all user-entered values passed as bound parameters, and all table/column identifiers taken only from the inspected schema and emitted double-quoted, so that injection is impossible.
73. As a user, I want NULL values rendered as an empty cell in the grid and CSV, and as `null` in JSON, so that missing data is represented sensibly in each format.
74. As a user, I want results fetched from the database in pages, so that large tables don't exhaust memory.
75. As a user, I want the first page and the total row count fetched concurrently, with rows displayed as soon as they arrive, so that a slow count doesn't delay seeing data.
76. As a user, I want an empty Limit to mean an unlimited logical result paged one terminal-page at a time, so that I can browse whole tables.
77. As a user, I want the header to show "Counting rows..." until the total count arrives and an error message if counting fails, so that the header is never misleading.
78. As a user, I want every failed or cancelled write to leave the database untouched, so that partial effects never persist from an aborted operation.

## Implementation Decisions

- **Language and stack**: Go, with `modernc.org/sqlite` (pure Go, no cgo) as the SQLite driver, `bubbletea` for the TUI, `lipgloss` for styling, and `bubbles` components where appropriate. CLI parsing with `mow.cli` using command-word/arguments style (`sqlite <file>`, `d1`). CLI basics: `--help`/`-h` prints usage; `--version`/`-v` prints version; unexpected arguments print a usage error to stderr and exit 2; a directory passed to `sqlite` fails the SQLite header check ("not a SQLite database"), exit 1.
- **Grid rendering**: cells truncate at the column's computed width with an ellipsis; embedded tabs/newlines render as visible symbols; Unicode cell widths measured with a runewidth approach; duplicate result column names are deduplicated deterministically over the full output-name set (see Export formats) in the header; terminals narrower than minimum column widths fall back to horizontal scrolling.
- **Resize**: page size recomputed around the first currently visible row index, preserving position where possible.
- **File picker**: starts in the working directory; shows directories (including hidden) with parent navigation via `..`; no directory creation in v1; Esc cancels; invalid filename means an empty basename or a basename containing `/` or NUL (directory selection is a separate picker step, so paths in the name field are rejected) and shows inline errors; missing extensions appended automatically (`.sql`/`.csv`/`.json`); write-permission failures surface as inline save-flow errors.
- **Atomic saves**: exports and saved queries write to a temp file in the destination directory and rename over the target; any serialization/I/O failure before the rename leaves the pre-existing file untouched and the temp file cleaned up. Temp-file-and-rename can fail in edge cases (e.g., restrictive destination-directory permissions) but usually works — accepted v1 limitation; failures surface inline.
- **Scope of databases**: v1 supports SQLite files and local D1 (miniflare) databases only. Other database types are explicitly out of scope.
- **D1 discovery**: candidate files are `.sqlite` files (case-sensitive extension) in the miniflare D1 directory whose names do not contain a lowercase `metadata` substring; `-shm` and `-wal` files are ignored by extension. Matching is deliberately case-sensitive so candidate sets are identical on Linux and macOS. The path is resolved relative to the process working directory. Exactly one candidate must exist; multiple candidates produce the single message "There is more than one SQLite database in .wrangler"; an absent, unreadable, or candidate-free directory produces "no candidate database found in .wrangler".
- **Startup validation and errors**: startup sequence: existence check → readable check → 16-byte `SQLite format 3\0` header check → open read-write without creating (`mode=rw`) → harmless schema probe (`PRAGMA schema_version`). Any failure — including inability to establish read-write mode (e.g., a read-only file) or busy-timeout expiry during the probe — is a startup failure printing one line to stderr (naming the file and OS reason) and exiting with status 1. Successful startup writes nothing to stdout/stderr. v1 assumes writable sessions; there is no degraded read-only mode.
- **Busy handling**: a 5-second busy timeout applies at open (to the schema probe above) and to all statements. At open, expiry errors out and exits; mid-session it surfaces as a normal query error ("database is locked") in the results view.
- **Session health**: before each statement execution (query, count, or write), the database file is checked for existence at its original path. If it is gone (deleted or renamed away — replacement at the same path is not detected, an accepted limitation), the session enters a terminal error state: a full-screen message "Database file no longer exists — session ended", where Ctrl+S and Ctrl+X still work to export queries/result snapshots from memory (defaulting to the last executed query/result, with Ctrl+P/N/E/Y available to select another entry), and `q` quits normally while Ctrl+C quits immediately with exit status 1 (no confirmation — the session has already ended). Other connection errors during execution are ordinary query errors and do not end the session.
- **Query construction**: all queries are built from structured field state. User values are bound as parameters (no concatenation of values into SQL). Identifiers (table and column names) originate only from schema inspection — never free-typed — and are always emitted double-quoted with internal double-quotes doubled. Aggregate functions come from the fixed enum {COUNT, MIN, MAX, AVG, SUM} plus the sentinel `COUNT(*)`, and sort directions from {ASC, DESC}.
- **Aggregate rule**: all-aggregate selections (e.g., `COUNT(*), MAX(age)`) run without a Group By. If the selection mixes aggregate and non-aggregate columns, every non-aggregate selected column must also be in Group By or execution is blocked with a clear message. Wildcard projection `*` must be the sole entry and cannot coexist with any aggregate. HAVING is explicitly unsupported in v1. Projection entry identity is the (column, aggregate) pair: different aggregates on the same column may coexist; identical pairs cannot be added twice.
- **Numeric value parsing**: input is taken verbatim with no trimming — whitespace anywhere makes it TEXT. INTEGER if it matches `-?[0-9]+` and fits signed 64-bit (leading zeros fine; leading `+` falls through). Otherwise REAL if accepted by Go's `strconv.ParseFloat` within float64 range (`1.`, `.5`, exponent forms allowed; leading `+` or range overflow → TEXT). Everything else binds as TEXT verbatim (`NaN`, `Inf`, `0x1A`, padded strings). Non-finite REALs can arise only from pre-existing db data; JSON exports them as quoted strings (`"Inf"`, `"-Inf"`, `"NaN"`), CSV as their textual form.
- **Paging**: results are fetched with LIMIT/OFFSET paging; page size derives from terminal height and is recomputed on resize. When a SELECT has no user ORDER BY, Sqloid appends `ORDER BY rowid` only if the queried object is an ordinary rowid table with no declared column shadowing `rowid`; for views, virtual tables, WITHOUT ROWID tables, rowid-shadowed tables, and aggregate/grouped queries no implicit order is added. Default paging is therefore stable only where the implicit unique rowid applies; otherwise page composition is not guaranteed stable — documented alongside tie instability from non-unique user ORDER BY columns and concurrent-write drift between pages (pages are separate autocommit reads — no read transaction is held, since one would block wrangler sharing the file). The first page and the total row count run concurrently (one logical execution; see Execution Lifecycle); the header shows "Counting rows..." until the count arrives, or an error message if counting fails. An empty Limit means the logical result is unbounded and navigated via the sliding-window cache (see Execution Lifecycle); a user-entered Limit caps the logical result, and the reported total count reflects that cap.
- **History**: in-memory only, per session. Query history stores full field state (so it can repopulate the builder); result history follows the one-entry-per-execution outcome table in Execution Lifecycle. Revisiting a historical result shows exactly what was captured — no re-fetching or re-execution; re-running the query gets fresh data. Each result snapshot holds at most the 10,000 retained rows (marked truncated beyond); each history list is capped at 20 entries with oldest-first eviction. Snapshots are re-sliced to the current terminal height when viewed. New query-history entries append only at execution time, and only when the executed state differs from the last executed state under full normalized comparison: command, table, ordered projection (column + aggregate per entry, wildcard flag), where (column + operator + value + bound type), group by list, order by (column + direction), limit (empty vs number), and SET/INSERT choices for writes. Value type is significant (`'1'` text ≠ `1` integer) as is column order; typed values are compared as entered. Historical entries are never mutated.
- **Saving**: one-way export only in v1 — no loading of saved queries or results. Ctrl+S saves the current builder state if it forms a runnable query; otherwise the last executed query from history; when viewing a historical result, that snapshot's associated query; if neither exists (incomplete builder, nothing ever run), an inline "no runnable query to save" message appears and no picker opens. In the terminal error state the last executed query is the default, selectable via Ctrl+P/N. Queries export as standalone executable `.sql`: literal values safely serialized into the text (strings escaped by quote-doubling, `NULL` keyword, `X'hex'` blobs) with identifiers double-quoted (internal double-quotes doubled) and a trailing semicolon; one statement per file.
- **Export scope**: exporting a result exports its in-memory snapshot exactly as held (≤10,000 retained rows; see Execution Lifecycle for finalization and truncation marking). To export a bounded subset deliberately, set a Limit before running. Historical results export their snapshots identically. Error results, "Cancelled"-before-rows entries, and write summaries have nothing tabular to export — Ctrl+X is a no-op with a message for them.
- **Export formats**: CSV is RFC 4180 — header row of deduplicated column names, minimal quoting, embedded newlines/tabs preserved inside quoted fields, UTF-8, CRLF line endings. NULL and empty string both render empty in CSV — an accepted, lossy limitation. JSON is an array of objects keyed by deduplicated column name; INTEGER/REAL export as JSON numbers (non-finite REALs as quoted strings), NULL as `null`, BLOBs as base64 strings. Computed/aggregate columns use the label SQLite assigns (e.g., `COUNT(*)`). Output-name deduplication walks columns left to right: the first occurrence keeps its name; each subsequent duplicate gets the lowest suffix (`name_2`, `name_3`, …) that does not collide with any name already in the final set (including originals). The same deduplicated set applies uniformly to grid headers, CSV headers, and JSON keys; generated SQL and driver metadata are never altered.
- **Value entry and types**: one universal text entry for all columns; on submit the input is parsed and bound per Numeric value parsing above, letting SQLite's column affinity coerce. NULL is entered only via explicit popup/operator choices; empty string is just empty text. BLOBs cannot be entered via the v1 UI (explicit limitation); BLOB or binary result values display as `[BLOB n bytes]`, export to CSV as lowercase hex and to JSON as base64.
- **INSERT column handling**: the INSERT flow prompts once per insertable column (see Schema). There is no AUTOINCREMENT-based skipping: any column can be excluded via its per-column {Value, NULL, Default/Omit} choice. INTEGER PRIMARY KEY columns are prompted like any other, with a "(auto-assigned if omitted)" note; choosing Default/Omit there yields auto-assignment. Choosing Default/Omit for every column emits `INSERT INTO "table" DEFAULT VALUES`, executed through the normal path with a rows-added summary. A table with zero insertable columns shows "table has no insertable columns" inline and never enters the value flow. Virtual tables are best-effort: only visible insertable columns are prompted, and module errors (e.g., modules requiring hidden-column inputs) surface as ordinary query errors — documented limitation.
- **Schema scope**: the table list shows ordinary tables, virtual tables, and views from the `main` schema only, via `sqlite_master`, excluding internal `sqlite_%` objects and the D1 `_cf_METADATA` table. Views are selectable for SELECT; write commands (UPDATE/DELETE/INSERT) list only tables, not views. The table/column listing is refreshed from the database every time the table popup is opened (so tables created or dropped by another process are picked up on the next query); column metadata for a chosen table is likewise fetched fresh. Refresh failures (lock, corruption, external change) show an inline error ("could not refresh: …") and retain the stale listing with a notice, offering retry/cancel; a detected file deletion takes precedence as the terminal state.
- **Destructive-write safety**: unqualified UPDATE/DELETE (no WHERE) are allowed but the confirmation modal shows a prominent warning that all rows in the table will be affected. The modal opens immediately showing operation type, table name, rendered SQL with literal values, and "Estimating affected rows…"; the pre-flight estimated row count (a `SELECT COUNT(*)` wrapper) is **non-binding** — confirm (Enter/y) stays disabled until the estimate completes. If the estimate fails (lock timeout, error), the modal says so and confirmation becomes allowed anyway (SQL and warnings still shown). Ctrl+W while estimating cancels the estimate and closes the modal. Esc cancels. Another process changing matching rows between estimate and confirmation is an accepted concurrency limitation (time-of-check/time-of-use); the post-write summary always shows the driver's actual `RowsAffected()` (triggers may make it differ from rows conceptually changed).
- **Write atomicity**: every user-visible write (UPDATE/DELETE/INSERT) executes inside an application-controlled transaction: BEGIN; statement; COMMIT — with ROLLBACK on any statement error or cancellation. Failed or cancelled writes therefore always leave the database untouched, including under SQLite's FAIL conflict resolution and trigger `RAISE(FAIL)` (single-statement autocommit alone would not guarantee this). Transactions spanning multiple user operations remain out of scope.
- **INSERT value entry**: each column prompt offers {Value, NULL, Default/Omit} — Value opens text entry (empty string = empty string), NULL binds an explicit NULL, Default/Omit excludes the column from the INSERT statement.
- **Builder lifecycle**: changing the command (S/U/D/I) keeps the chosen table if it remains eligible for the new command (ordinary/virtual tables are eligible everywhere; views only for SELECT) and otherwise clears the Table field with focus moving to Table; downstream fields (columns, where, group by, order by, limit, SET/INSERT values) always clear. Shift+Tab/arrows allow returning to earlier UPDATE/INSERT prompts with prior choices pre-filled. Popups and modals are input-modal per the Context/Key Matrix. Enter runs whenever no popup/modal/text-entry is open and the current state is runnable per the grammar, aggregate rules, and ORDER BY restrictions. Restoring from history copies the state into the builder — historical entries are never mutated; a new entry is appended on run only if the state differs from the last executed state.
- **Keybindings**: all bindings are single, terminal-distinguishable Ctrl chords or plain keys — query history `Ctrl+P`/`Ctrl+N`; result history `Ctrl+E`/`Ctrl+Y`; save query `Ctrl+S`; export result `Ctrl+X`; cancel running operation `Ctrl+W`; help modal `?`; horizontal scroll `,`/`.` as reachable fallbacks alongside Shift+PageUp/Down. No Alt-based or Ctrl+Shift-based bindings (indistinguishable in common terminals / bound by multiplexers like zellij). The complete per-context behaviour is the Context/Key Matrix section of this document — there is no external key-matrix design doc.
- **Quitting**: `q` quits immediately from the Command field only; Ctrl+C anywhere else (idle, executing, or in another modal) shows the quit confirmation modal — except the terminal error state, where Ctrl+C quits immediately with status 1.
- **Errors**: any query error replaces the results view and produces exactly one history entry per the Execution Lifecycle outcome table; previous results remain reachable. Esc dismisses an error back to the builder. A count failure does not prevent rows from displaying — the header shows the count error and paging continues without the total; a page-fetch failure mid-scan ends the execution with a "failed at row N" snapshot of what was fetched. Export/path errors appear inline in the save flow for retry or cancel. Only startup failures and detected file deletion end the session. There are no query timeouts in v1: operations run until done or cancelled.
- **Logging**: none in v1 — no debug flag, no log file. All diagnostics surface in the UI; this avoids TUI display corruption and keeps potentially sensitive query values out of logs.
- **Execution model**: queries and counts execute off the UI thread (bubbletea commands) as independent cancellable operations owned by one logical execution (see Execution Lifecycle). Enter is ignored while an execution is in flight.

## Module Design

- **Connection**
  - **Responsibility**: owns the database handle and everything about how a database is opened and queried.
  - **Interface**: open-by-path (with the full startup validation sequence) and open-d1 (discovery rules); independent cancellable operations `ExecutePage(query, offset, limit) → (rows, error)` and `ExecuteCount(query) → (count, error)` — completing independently so the UI can show rows despite count failure — plus `ExecuteWrite(statement) → (rowsAffected, error)` wrapping the internal BEGIN/COMMIT/ROLLBACK transaction; liveness checking for mid-session file deletion. Operation identity and cancellation semantics live here so async behaviour never leaks into ad-hoc UI conventions. Hides the driver choice and all SQL plumbing from callers.
  - **Tested**: yes (especially d1 discovery/validation logic).

- **Schema**
  - **Responsibility**: describes the database's structure — objects and their columns.
  - **Interface**: list objects, each carrying kind ∈ {ordinary-table, virtual-table, view} and rowid capability ∈ {has-rowid, without-rowid, not-applicable} plus a flag for declared columns shadowing `rowid`; list columns (name, declared type, insertability) for a table. Insertability is computed from `PRAGMA table_xinfo`: hidden or generated columns are not insertable; all others are. These metadata let callers implement command eligibility (M1) and implicit-order decisions without raw catalog SQL. Backs all popups and the INSERT flow. Refresh failures are surfaced to the caller as errors (stale-listing retention is a UI decision).
  - **Tested**: yes.

- **QueryBuilder**
  - **Responsibility**: holds the command-specific field state (SELECT: wildcard flag or ordered (column, aggregate) entries, where, group by, order by, limit; UPDATE: set assignments, where; DELETE: where; INSERT: ordered column/value pairs or DEFAULT VALUES) and turns it into SQL plus bound parameters, per the Query Grammar section.
  - **Interface**: get/set each field; render SQL + params; report whether the current state is runnable (aggregate-without-group-by blocks; invalid ORDER BY in grouped queries blocks); detect whether state differs from a previous state (for history). Pure logic — no UI dependencies.
  - **Tested**: yes (thoroughly).

- **UI**
  - **Responsibility**: the bubbletea model — field bar, results grid with frozen header, popups (tables, columns, aggregates, quit/save modals), the Context/Key Matrix, execution-lifecycle handling (operation IDs, cancellation, sliding-window cache), and wiring user actions to QueryBuilder/Connection.
  - **Interface**: bubbletea Init/Update/View. Composes all other modules.
  - **Tested**: behaviourally — key-sequence tests cover critical flows (transitions, popups, modals, quit, history, errors, cancellation phases); visual rendering/layout is verified manually.

- **History**
  - **Responsibility**: in-memory lists of past query states and past result snapshots, with position tracking, entry caps, eviction, and the one-entry-per-execution outcome rules.
  - **Interface**: append query state / result snapshot (with outcome classification); step back/forward through each list; current item access; per-list cap of 20 entries (oldest evicted); snapshots capped at 10,000 rows and marked truncated/failed-at-N/cancelled as appropriate.
  - **Tested**: yes.

- **Export**
  - **Responsibility**: writes a result set to CSV or JSON, and a query to a `.sql` file — atomically (temp file + rename).
  - **Interface**: write result snapshot (rows + deduplicated column metadata) to a path in a chosen format (RFC 4180 CSV or array-of-objects JSON); write query (SQL text with safely serialized literals and quoted identifiers, trailing semicolon) to a path. NULL/non-finite/BLOB handling per format. Pre-existing files untouched on any failure.
  - **Tested**: yes.

## Testing Decisions

- Tests target external behaviour: given field state, QueryBuilder produces this SQL and these params; given rows, Export produces this CSV/JSON; given a directory layout, Connection's d1 discovery picks (or rejects) the right file. Implementation details stay untested.
- **UI behavioural tests are required** for critical flows, using bubbletea's `(model, msg) → (model, cmd)` testability with scripted key sequences: builder flow transitions (command → table → columns → run) including popup cancel; history navigation and repopulation; confirmation modal accept/cancel including the unqualified-write warning and estimate-failure flow; quit paths (`q`, Ctrl+C incl. during execution, Ctrl+W cancel vs Ctrl+C quit); error display and dismissal. Pure visual rendering (View output/layout) stays untested and is verified manually — identified here as a deliberate manual-only area.
- **High-risk boundary coverage is required** (external behaviour, not implementation):
  1. Async overlap: superseded/late responses discarded; cancellation at each execution phase (before rows, after rows, count-pending)
  2. First-page success with count failure — rows display, header shows the error
  3. Paging/history at and beyond the 10,000-row cap: sliding-window eviction, Page Up re-fetch, snapshot truncation and finalization
  4. Write rollback on constraint failure and on cancellation mid-write
  5. Pre-flight estimate failure/cancellation in the destructive-write modal; unqualified-write confirmation flow
  6. Command switch from a view to a write command clears the table
  7. `COUNT(*)` versus wildcard projection builder behaviour
  8. Numeric parser boundaries (whitespace, `+`, `1.`, `.5`, overflow) and non-finite export values
  9. Rowid-shadowing and grouped-query paging order
  10. Schema refresh failure/drop/change during a built query
- **CLI + database integration tests**: startup validation cases (missing/missing-file/non-sqlite/read-only file), d1 discovery against temp-directory fixtures, and end-to-end query execution through Connection against fixture databases.
- The project is greenfield, so there is no prior art in this codebase; standard Go table-driven tests with `testing` are the convention.

## Out of Scope

- Any database engine other than SQLite/local D1 (Postgres, MySQL, remote D1, etc.).
- Loading or importing saved queries or results — saving is one-way in v1.
- Frozen columns during horizontal scrolling (deferred to post-v1).
- Full-screen text editing mode (for nested queries or free-text SQL entry), and free-text SQL entry itself (deferred to a post-v1 advanced mode). The main UI itself is a full-screen TUI; it is the editing mode that is excluded.
- HAVING clauses.
- Editing result cells directly in the grid (removed from scope by decision).
- Multiple simultaneous database connections or switching databases within a session.
- Query history or saved-file persistence across sessions.
- Transactions spanning multiple user operations (the internal single-write transaction wrapper is in scope; see Write atomicity).
- LIKE wildcard escaping (literal `%`/`_` search unsupported in v1).
- Virtual-table INSERT flows requiring hidden-column inputs (best-effort via visible columns only).
- Degraded read-only sessions on unwritable database files (startup fails instead).
- Windows-specific terminal concerns.

## Acceptance Criteria and Supported Environment

- **Supported environments**: Linux and macOS. Terminal requirements: standard xterm-compatible key/arrow sequences and at least 16 colors; no mouse support in v1. Note: some terminals intercept Shift+PageUp/Down for scrollback — the `,`/`.` fallbacks provide equivalent functionality everywhere.
- **Minimum terminal size**: 80×24. Below that, display a "terminal too small" message instead of a broken layout.
- **SQLite assumptions**: any database file openable by `modernc.org/sqlite` (SQLite 3 format); WAL-mode databases supported read-write.
- **Target envelope** (guidance, not guarantees): tables up to ~100k rows navigated comfortably via paging; result snapshots capped at 10,000 rows (see History); the first page of a paged query ideally renders within ~100ms for indexed queries within this envelope. Responsiveness targets are explicitly excluded from the definition of done — slow operations are handled by cancellation.
- **Definition of done**: all user stories implemented with their stated acceptance behaviour; test suite green per Testing Decisions (including the high-risk boundary coverage); a manual verification pass on rendering in an 80×24 terminal.

## Open Questions

None remaining. All questions from the original interview were resolved previously; this revision resolved the second critique in full — 6 blockers (wildcard/COUNT(\*) model, execution lifecycle and cancellation, sliding-window paging vs snapshots, write rollback transactions, in-document key matrix, pre-flight estimate flow), 15 majors (command-switch revalidation, stable-order honesty, projection identity and grouped ORDER BY, DEFAULT VALUES/virtual-table INSERT, numeric grammar, startup read-write probe, schema metadata, history outcome lifecycle, Ctrl+S target rules, horizontal-scroll fallbacks and terminal-state Ctrl+C, output-name dedup, atomic saves, Connection interface split, high-risk test coverage, responsiveness reclassified), and 7 minors (WHERE scoping, popup search scope, LIKE wildcards, Limit bounds, D1 case rule, terminal-state export selection, story renumbering) — each either incorporated as a requirement above or documented as an accepted limitation.

## Further Notes

- The project vision ("connect to multiple types of database") is broader than v1; the Connection module is deliberately designed to hide driver details so additional engines can be added later without changing callers. The original "type in SQL commands" vision is likewise broader than v1; free-text SQL is deferred to a post-v1 advanced mode (see Solution).
- The `d1` discovery rules mirror the real miniflare layout: one long-random-string `.sqlite` file plus a `metadata.sqlite` file to be ignored.
- The aggregate + GROUP BY rule exists because SQLite silently permits bare non-aggregated columns in aggregate queries (returning an arbitrary row's value); blocking at the UI level prevents ambiguous rather than failing queries. The same rationale restricts ORDER BY candidates in grouped queries.
- The total count is computed by wrapping the query in `SELECT COUNT(*) FROM (<inner query>)`; it counts the logical result after the user's Limit, and runs concurrently with the first page fetch so slow counts never block the first rows.
- The internal transaction around writes exists because SQLite's autocommit is not sufficient for the untouched guarantee: the FAIL conflict algorithm (including trigger `RAISE(FAIL)`) aborts a statement without backing out changes already applied to earlier rows by that same statement.
