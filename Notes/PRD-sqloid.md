# PRD: Sqloid v1

## Problem Statement

Inspecting and querying a SQLite database currently means dropping down to the `sqlite3` shell or an external GUI. The shell demands that every query be typed out by hand, column names memorised, and results are hard to read, page through, or export. There is no lightweight, keyboard-driven tool that lets a developer explore a database's schema, build common queries interactively, view results in a scrollable grid, and export them — whether the database is a plain SQLite file or a local Cloudflare D1 database created by wrangler/miniflare.

## Solution

Sqloid is a terminal UI application (single user, single developer) that opens a SQLite database — either given explicitly on the command line, or discovered in a local wrangler D1 state directory — and presents a full-screen interface: results occupy most of the screen, and a query-builder bar sits at the bottom.

The user builds SELECT, UPDATE, DELETE, and INSERT queries by stepping through fields: command, table (chosen from a popup list of actual tables), column(s) (chosen from a popup list, with optional aggregates), where, group by, order by, and limit. Values are entered with type-aware validation. Queries run with Enter; results are shown in a grid with a frozen header row, page up/down vertical paging and horizontal scrolling, a total row count in the header, and full history of past queries and results navigable by keybinding. Queries and results can be exported to files (`.sql` for queries; CSV or JSON for results). Errors replace the results view and are themselves navigable via result history.

## User Stories

1. As a user, I want to run `sqloid sqlite <file>`, so that I can open a SQLite database file directly.
2. As a user, I want an error when no file argument is given to the `sqlite` command, so that I know immediately what went wrong.
3. As a user, I want an error when the given file does not exist, so that I don't end up in an empty session against a typo.
4. As a user, I want an error when the given file is not a SQLite database, so that I don't get cryptic failures later.
5. As a user, I want to run `sqloid local-d1`, so that I can open the local D1 database created by wrangler without hunting for its path.
6. As a user, I want the program to look in the `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` directory for a single SQLite database file (ignoring files with `metadata` in the name, and `-shm`/`-wal` files), so that the right database is picked automatically.
7. As a user, I want a clear error ("There is more than one Sqlite database in .wrangler") when the d1 directory contains more than one candidate database, so that I can clean it up.
8. As a user, I want an error when no candidate database exists in the d1 directory, so that I know there is nothing to open.
9. As a user, I want the database opened read-write, so that UPDATE, DELETE, and INSERT statements actually persist.
10. As a user, I want the connection monitored so that the program errors out if the database file is deleted mid-session, so that I don't silently work against a dead handle.
11. As a user, I want to quit the program at any time, so that I'm never trapped in the UI.
12. As a user, I want to hit `q` while on the Command field to quit immediately without confirmation, so that quitting is fast.
13. As a user, I want Ctrl+C at any other point to show a quit confirmation modal, so that I don't lose in-progress work accidentally.
14. As a user, I want the results to occupy the top majority of the screen and the field bar the bottom, so that I can see data while building queries.
15. As a user, I want the field bar to grow to multiple lines as fields gain content, so that long WHERE clauses remain visible.
16. As a user, I want to start in the Command field, so that building a query has an obvious entry point.
17. As a user, I want a single keypress of `S`, `U`, `D`, or `I` in the Command field to expand immediately to Select, Update, Delete, or Insert, so that command choice is instant.
18. As a user, I want to return to the Command field with arrows/tab and press a different letter to replace my command choice, so that changing command type is easy.
19. As a user, I want the UI to move to the Table field after the command is chosen, so that the flow proceeds naturally.
20. As a user, I want a popup list of all tables in the database, with scrolling and fuzzy search, so that I can pick a table quickly.
21. As a user, I want a popup list of the chosen table's columns after the table is picked, with `*` at the top selected by default, so that SELECT `*` is the fastest path.
22. As a user, I want Enter on a column to add it to the column list and re-show the popup, so that I can select multiple columns.
23. As a user, I want a popup of `Value/Count/Min/Max/Avg/Sum` after each column choice, with Value selected by default, so that aggregates are one keypress away.
24. As a user, I want Enter on the Column(s) field (after one or more columns are chosen) to run the query, so that running is fast.
25. As a user, I want tab/arrow keys to move to the next field instead of running, so that I can refine Where/Order By/Limit first.
26. As a user, I want the Where field to be assisted — column popup first, then typed value entry with numbers-only validation for numeric columns and text otherwise — so that valid predicates are easy to write.
27. As a user, I want the Group By field to be an assisted multi-select of columns, empty meaning no GROUP BY, so that aggregate queries are straightforward.
28. As a user, I want the query to refuse to run while at least one aggregate is selected and no Group By is chosen, so that I never run an invalid aggregate query.
29. As a user, I want the Order By and Limit fields editable with the same type-aware validation, so that I can sort and cap results.
30. As a user, I want the UPDATE flow to be: pick table, pick SET column(s), enter a new value per column (numbers-only for numeric columns, text otherwise), optional assisted WHERE, then confirmation before running, so that destructive writes are deliberate.
31. As a user, I want the DELETE flow to be: pick table, assisted WHERE, then confirmation before running, so that I can't delete by accident.
32. As a user, I want the INSERT flow to be: pick table, then one value entry per column (skipping AUTO-INCREMENT columns), so that adding rows is guided.
33. As a user, I want a confirmation modal before any UPDATE or DELETE runs, so that destructive operations are always deliberate.
34. As a user, I want Enter to run the query, so that execution has one obvious trigger.
35. As a user, I want Page Up/Page Down to page through results vertically, so that I can navigate large result sets.
36. As a user, I want Shift+Page Up/Down to scroll results left/right, so that I can see wide tables.
37. As a user, I want the column header row frozen when scrolling vertically, so that I always know what the columns are.
38. As a user, I want the header to show the total result row count and the range currently displayed, so that I know where I am in the data.
39. As a user, I want the page size computed from the terminal height, so that paging always fills the screen.
40. As a user, I want layout and paging recomputed on terminal resize, so that resizing doesn't break the view.
41. As a user, I want Ctrl+P/Ctrl+N to scroll through previous queries, so that I can revisit earlier work.
42. As a user, I want selecting a previous query to repopulate all builder fields exactly as if I had just entered it, so that I can tweak and re-run.
43. As a user, I want only actual changes (different columns, changed WHERE, etc.) to add a new entry to the query history, so that history isn't polluted by identical re-runs.
44. As a user, I want Ctrl+Shift+P/Ctrl+N to scroll through previous results, so that I can refer back to earlier output.
45. As a user, I want query errors to replace the current results view, so that I always see what happened.
46. As a user, I want to navigate back to previous (successful) results via result history after an error, so that an error doesn't destroy my context.
47. As a user, I want UPDATE/DELETE results to show the rows affected along with the query that produced the change, so that writes are auditable in the UI.
48. As a user, I want INSERT results to show the row add count, so that I know the insert succeeded.
49. As a user, I want write-statement results included in the result history, so that I can page back through them like query results.
50. As a user, I want an empty SELECT result to show a message rather than an empty grid, so that "no rows" is unambiguous.
51. As a user, I want Ctrl+S to save the current query as a plain `.sql` text file, so that I can keep useful queries.
52. As a user, I want Ctrl+Shift+S to save the current result, prompting for a file name and a type (CSV or JSON), so that I can export data.
53. As a user, I want saving to use a file picker for the directory and text entry for the file name, so that choosing a destination is easy.
54. As a user, I want a confirmation prompt when saving over an existing file, so that I don't clobber files by accident.
55. As a user, I want all user-entered values passed as bound parameters, so that values are safely quoted and injection is impossible.
56. As a user, I want NULL values rendered as an empty cell in the grid and CSV, and as `null` in JSON, so that missing data is represented sensibly in each format.
57. As a user, I want results fetched from the database in pages, so that large tables don't exhaust memory.
58. As a user, I want the total row count obtained by wrapping my query in `SELECT COUNT(*) FROM (<inner query>)`, so that the header shows the true unbounded count even though display is capped.
59. As a user, I want a default cap of one terminal-page of rows when no Limit is entered, so that queries always return promptly.

## Implementation Decisions

- **Language and stack**: Go, with `modernc.org/sqlite` (pure Go, no cgo) as the SQLite driver, `bubbletea` for the TUI, `lipgloss` for styling, and `bubbles` components where appropriate. CLI parsing with `mow.cli` using command-word/arguments style (`sqlite <file>`, `local-d1`).
- **Scope of databases**: v1 supports SQLite files and local D1 (miniflare) databases only. Other database types are explicitly out of scope.
- **D1 discovery**: candidate files are `.sqlite` files in the miniflare D1 directory whose names do not contain `metadata`; `-shm` and `-wal` files are ignored by extension. Exactly one candidate must exist, else the program errors (single generic message for the multiple-candidates case) and exits.
- **Session health**: the database file is monitored during the session; deletion of the file mid-session produces an error and ends the session.
- **Query construction**: all queries are built from structured field state and executed with parameter binding. No string concatenation of user values into SQL.
- **Aggregate rule**: if any selected column uses an aggregate (Count/Min/Max/Avg/Sum), the query requires a non-empty Group By before it will run.
- **Paging**: results are fetched with LIMIT/OFFSET paging; page size derives from terminal height and is recomputed on resize. Total count comes from a wrapped `SELECT COUNT(*)` query. When the user leaves Limit empty, a default cap of one page applies; when the user sets Limit, it is honoured within the paging scheme.
- **History**: in-memory only, per session. Query history stores full field state (so it can repopulate the builder); result history stores rendered result sets including error results and write-statement summaries. Only a genuine change to field state creates a new query-history entry.
- **Saving**: one-way export only in v1 — no loading of saved queries or results. Queries export as plain `.sql`; results export as CSV (NULL → empty cell) or JSON (NULL → `null`). Overwriting an existing file requires confirmation.
- **Quitting**: `q` quits immediately from the Command field only; Ctrl+C anywhere else shows a confirmation modal.
- **Errors**: any query error replaces the results view; previous results remain reachable via result history.

## Module Design

- **Connection**
  - **Responsibility**: owns the database handle and everything about how a database is opened and queried.
  - **Interface**: open-by-path (with validation: exists, is a SQLite database) and open-local-d1 (discovery rules); paged query execution returning rows plus total count; statement execution returning rows affected; liveness checking for mid-session file deletion. Hides the driver choice and all SQL plumbing from callers.
  - **Tested**: yes (especially d1 discovery/validation logic).

- **Schema**
  - **Responsibility**: describes the database's structure — tables and their columns with types.
  - **Interface**: list tables; list columns (name, type, whether auto-increment) for a table. Backs all popups and type-aware validation.
  - **Tested**: yes.

- **QueryBuilder**
  - **Responsibility**: holds the complete field state (command, table, columns with aggregates, where, group by, order by, limit) and turns it into SQL plus bound parameters.
  - **Interface**: get/set each field; render SQL + params; report whether the current state is runnable (e.g., aggregate-without-group-by blocks execution); detect whether state differs from a previous state (for history). Pure logic — no UI dependencies.
  - **Tested**: yes (thoroughly).

- **UI**
  - **Responsibility**: the bubbletea model — field bar, results grid with frozen header, popups (tables, columns, aggregates, quit/save modals), keybindings, and wiring user actions to QueryBuilder/Connection.
  - **Interface**: bubbletea Init/Update/View. Composes all other modules.
  - **Tested**: no (kept thin; behaviour covered indirectly via the modules it composes).

- **History**
  - **Responsibility**: in-memory lists of past query states and past results, with position tracking.
  - **Interface**: append query state / result; step back/forward through each list; current item access.
  - **Tested**: yes.

- **Export**
  - **Responsibility**: writes a result set to CSV or JSON, and a query to a `.sql` file.
  - **Interface**: write result (rows + column metadata) to a path in a chosen format; write query text to a path. NULL handling per format.
  - **Tested**: yes.

## Testing Decisions

- Tests target external behaviour of the deep modules: given field state, QueryBuilder produces this SQL and these params; given rows, Export produces this CSV/JSON; given a directory layout, Connection's d1 discovery picks (or rejects) the right file. Implementation details stay untested.
- Modules with tests: Connection (discovery/validation/query paging), Schema, QueryBuilder, History, Export. The bubbletea UI layer is untested by design.
- The project is greenfield, so there is no prior art in this codebase; standard Go table-driven tests with `testing` are the convention.

## Out of Scope

- Any database engine other than SQLite/local D1 (Postgres, MySQL, remote D1, etc.).
- Loading or importing saved queries or results — saving is one-way in v1.
- Frozen columns during horizontal scrolling (deferred to post-v1).
- Full-screen mode, nested query editing, or free-text SQL entry (deferred to post-v1).
- Editing result cells directly in the grid (removed from scope by decision).
- Multiple simultaneous database connections or switching databases within a session.
- Query history or saved-file persistence across sessions.
- Transactions spanning multiple statements.
- Windows-specific terminal concerns.

## Open Questions

- None remaining — all questions raised during the interview were resolved.

## Further Notes

- The project vision ("connect to multiple types of database") is broader than v1; the Connection module is deliberately designed to hide driver details so additional engines can be added later without changing callers.
- The `local-d1` discovery rules mirror the real miniflare layout: one long-random-string `.sqlite` file plus a `metadata.sqlite` file to be ignored.
- The aggregate + GROUP BY rule exists because SQLite rejects mixed aggregate/non-aggregate selects without grouping; blocking at the UI level prevents guaranteed-failure queries.
- The unbounded `SELECT COUNT(*)` wrapper means the header's total reflects the full result set even though only one page is fetched at a time.
