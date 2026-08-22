### Sqloid

This is a project to create a SQL editor that can connect to multiple types of database and execute queries. Note: v1 narrows the original 'SQL editor' vision to a structured query builder / data browser (guided fields instead of typed SQL); free-text SQL entry is planned as a post-v1 advanced mode.

### Features
- Connect to multiple types of database
- Execute queries
- View results
- Save queries
- Save results

### Implementation ideas

- Uses golang for code
- Uses mow.cli (https://github.com/jawher/mow.cli) for the command line interface using command-word/arguments style
    - sqlite <file> - run the app with the given sqlite database file
    - d1 - run the app with a local d1 database
- Uses bubbletea (https://github.com/charmbracelet/bubbletea) for the UI and Lipgloss (https://github.com/charmbracelet/lipgloss) for styling, with (where appropriate) components from bubbles (https://github.com/charmbracelet/bubbles)
- For the 'sqlite <file>' command, the user gives the path to a sqlite database on the command line, with an error if it's not given, does not exist, or is not a sqlite database
- For the 'd1' command, the program should look in the '.wrangler/state/v3/d1/miniflare-D1DatabaseObject' directory for a single sqlite database file (that is not have 'metadata' in the name) and use that. If there is no such file, or if there are multiple such files, it should error.
- The main idea is that the user can type in SQL commands.

### UI

- There are fields at the bottom of the screen. There are results at the top. The results should take up most of the area, but the fields can grow to multiple lines as bits get added.
- The fields are (list is not positional; flow order is Command → Table → Column(s) → Where → Group By → Order By → Limit):
    - Command
    - Column(s)
    - Table
    - Where
    - Order By
    - Limit
    - Group By
- The UI starts in the 'Command' field, and the user can hit:
    - S - which expands to 'Select'
    - U - which expands to 'Update'
    - D - which expands to 'Delete'
    - I - which expands to 'Insert'
- After the command is selected, the UI moves to the 'Table' field
- Then the UI pops up a list of all the tables in the given database, and the user can select a table from the list, either by scrolling or fuzzy search
- After the table is selected, the UI moves to the 'Column(s)' field, again with a list of columns in that table, with '*' at the top, selected by default. selecting one with 'enter' should add it to the list of columns
- After a column is selected, the user is given a pop-up list of 'Value/Count/Min/Max/Avg/Sum' to choose from. 'Value' is selected by default
- After the user selects an option, the UI shows the pop-up column list again so they can select more than one column
- After one or more columns are selected, the user can hit Enter to run the query, or tab/arrow keys to move to the next field
- arrows and tab/shift-tab move between fields
- Page up/down page through results 
- Shift page up/down scroll results left/right 
- Header shows result total row count, range currently displayed
- Ctrl p/n to scroll through previous queries 
- Ctrl shift p/n to scroll through previous results 
- If the user modifies a query, it should be added to the history at the bottom

### Decisions (from PRD interview and critiques; see Notes/PRD-sqloid.md — authoritative)

- v1 supports only sqlite files and local d1; other databases are out of scope
- Database connections are read-write; pure-Go sqlite driver (modernc.org/sqlite); startup opens with mode=rw (no create), runs a `PRAGMA schema_version` probe, and treats any failure (including read-only files and busy-timeout expiry at open) as exit-status-1 startup failure
- Startup validation: existence → readable → 16-byte 'SQLite format 3\0' header check → read-write open → schema probe
- d1 discovery: case-sensitive matching for `.sqlite` extension and lowercase 'metadata' substring exclusion; -shm/-wal ignored by extension; error if zero or multiple candidates; path relative to working dir
- 5s busy timeout at open (the schema probe) and on every statement; mid-session lock errors are ordinary query errors
- Query grammar: SELECT with projection entries — wildcard `*` (sole entry, no aggregate prompt, clears prior entries) OR (column, aggregate?) pairs from {Value, Count, Min, Max, Avg, Sum}; identity is the (column, aggregate) pair so Value(age)+AVG(age) coexist; special COUNT(*) offered only when no columns selected yet, may coexist with other aggregates; MIN/MAX/AVG/SUM never against '*'; single-predicate WHERE (SELECT/UPDATE/DELETE only); multi-select GROUP BY; single-column ORDER BY restricted in grouped queries to grouped columns + selected aggregates; LIMIT 1..int64max (zero/overflow rejected inline); UPDATE SET / DELETE / INSERT flows; INSERT with all Default/Omit emits DEFAULT VALUES; joins/subqueries/IN/HAVING/etc. out of scope
- LIKE values bound verbatim: % and _ are wildcards, no escaping in v1 (documented limitation); SQLite default case behaviour
- Numeric parse-and-bind: verbatim input, no trimming; `-?[0-9]+` within int64 → INTEGER; else strconv.ParseFloat within float64 range (`1.`/`.5`/exponents ok) → REAL; anything else TEXT (leading '+', whitespace, NaN/Inf/hex spellings); non-finite REALs from db data export as JSON quoted strings / CSV text
- Schema metadata: objects carry kind {ordinary-table, virtual-table, view}, rowid capability {has-rowid, without-rowid, n/a}, and rowid-shadowing flag; columns carry name/type/insertability via table_xinfo; table/column lists refreshed on each popup open; refresh failure retains stale listing with notice + retry/cancel
- Views selectable for SELECT only; command switch revalidates retained table (view + write command → clear Table, focus moves there); ordinary/virtual tables retained across switches; virtual tables listed as ordinary tables everywhere including INSERT (best-effort)
- Execution lifecycle: one logical execution per Enter owning up to two concurrent requests (first page + count) with operation IDs; Enter ignored while in flight (hint shown); builder editable but changes don't affect the running execution; Ctrl+W cancels the whole execution at any phase; late responses discarded by ID; Ctrl+C during execution = quit confirmation modal
- Exactly one history entry per ended execution: success snapshot; count-failure snapshot; mid-scan failure snapshot marked 'failed at row N'; cancelled-before-rows marker (not exportable); cancelled-after-rows snapshot marked Cancelled (exportable); first-page/write failure error entry (not exportable)
- Paging: LIMIT/OFFSET pages sized to terminal height; implicit ORDER BY rowid only for ordinary unshadowed rowid tables — none for views/virtual/WITHOUT ROWID/shadowed/aggregated queries; stability claimed only where implicit unique rowid applies; tie instability and concurrent-write drift documented (separate autocommit reads, no held transaction)
- Sliding-window result cache capped at 10k rows: Page Down past window evicts oldest pages; Page Up re-fetches evicted pages fresh; snapshot freezes when the execution ends (navigate away/error/cancel) containing ≤10k retained rows, marked truncated beyond; export = exactly the snapshot
- Write safety: every user-visible write runs inside an application-controlled transaction (BEGIN; statement; COMMIT; ROLLBACK on error or cancellation) so failed/cancelled writes always leave the db untouched — autocommit alone is insufficient under FAIL conflict resolution / trigger RAISE(FAIL); multi-statement transactions remain out of scope
- Destructive-write confirmation modal opens immediately ('Estimating affected rows…', confirm disabled until estimate completes); estimate via pre-flight COUNT(*) wrapper, labelled non-binding; estimate failure still allows confirm (SQL + warnings shown); Ctrl+W cancels estimate and closes modal; TOCTOU race between estimate and confirm accepted; post-write summary always shows driver RowsAffected()
- Ctrl+S priority: current runnable builder state → last executed query; viewing a historical result saves its query; nothing available → inline message, no picker; terminal error state defaults to last executed query, Ctrl+P/N selectable
- Saving is one-way export; .sql files serialize literals safely (quote-doubled strings, NULL keyword, X'hex' blobs, double-quoted identifiers, trailing semicolon); saves are atomic via temp-file-in-destination + rename (pre-existing file untouched on failure; edge-case permission failures an accepted v1 limitation); invalid filename = empty basename or containing '/' or NUL
- Export formats: CSV RFC 4180 (deduped header names, minimal quoting, UTF-8, CRLF; NULL and empty string both empty — lossy); JSON array of objects with deduplicated keys, numbers as JSON numbers, NULL null, blobs base64, non-finite REALs quoted strings; duplicate output names deduplicated left-to-right with lowest non-colliding suffix (name_2, name_3, …), applied uniformly to grid headers, CSV headers, and JSON keys; SQL never altered
- Popups: table/column/GROUP BY/ORDER BY searchable (case-insensitive subsequence fuzzy search); aggregate/operator deliberately scroll-only; Esc preserves selections in multi-select popups, cancels single-select without change
- Key model: Left/Right + Tab/Shift+Tab navigate builder fields; Up/Down used only inside popups and to toggle Order By direction on the Order By field; Backspace/Delete removes last-added entry on Column(s), clears whole assisted fields elsewhere; Shift+Tab/arrows revisit UPDATE/INSERT prompts with prior choices pre-filled; full context/key matrix lives in the PRD itself (no external design doc)
- Horizontal scroll: Shift+PageUp/Down plus `,`/`.` fallbacks (terminals often intercept Shift+Page keys)
- Quitting: q quits from Command field only; Ctrl+C elsewhere (idle/executing/modal) shows quit confirmation except terminal error state where it exits immediately status 1
- Terminal error state (db file deleted before execution): full-screen message; Ctrl+S/Ctrl+X export from memory (default last executed query/result; Ctrl+P/N/E/Y select another); ? help works; only q (normal) or Ctrl+C (immediate, status 1) quit; file replacement at same path not detected (accepted limitation)
- History: in-memory per session; snapshots immutable once appended, ≤10k rows marked truncated; each list capped at 20 oldest-evicted; new query entries append only at execution when normalized state differs (typed value comparison significant)
- Errors replace results view, Esc back to builder; count failure never blocks rows; export/path errors inline in save flow; only startup failures and detected file deletion end sessions; no timeouts, no logging (TUI corruption + sensitive-data rationale)
- INSERT prompts cover insertable columns only ({Value, NULL, Default/Omit}); INTEGER PRIMARY KEY noted auto-assigned if omitted; generated/hidden never prompted; zero-insertable-column tables blocked inline; virtual-table inserts best-effort via visible columns
- Responsiveness (~100ms first indexed page) is guidance under target envelope, excluded from definition of done; slow operations handled by cancellation
- Testing: external-behaviour tests incl. required high-risk boundary coverage (async overlap/cancel phases, count-failure-with-rows, 10k paging boundary, write rollback incl. trigger FAIL, pre-flight estimate flow, view→write switch, COUNT(*) vs '*', numeric boundaries/non-finite export, rowid shadowing/grouped paging, schema refresh during built query); UI behavioural tests via bubbletea scripted keys; rendering manual-only

### Future ideas, won't be implemented in version 1

- Full screen for nested query or flat text entry 
- Freeze column(s) for left/right scroll
