# PRD: Sqloid v1

## Problem Statement

Inspecting and querying a SQLite database currently means dropping down to the `sqlite3` shell or an external GUI. The shell demands that every query be typed out by hand, column names memorised, and results are hard to read, page through, or export. There is no lightweight, keyboard-driven tool that lets a developer explore a database's schema, build common queries interactively, view results in a scrollable grid, and export them — whether the database is a plain SQLite file or a local Cloudflare D1 database created by wrangler/miniflare.

## Solution

Sqloid v1 is a terminal UI application (single user, single developer — meaning no authentication, collaboration, or server behaviour) that opens a SQLite database — either given explicitly on the command line, or discovered in a local wrangler D1 state directory — and presents a full-screen interface: results occupy most of the screen, and a query-builder bar sits at the bottom.

The original project vision was a general SQL editor in which the user types SQL commands. For v1 this is deliberately narrowed to a structured query builder / data browser: guided selection, deterministic value parsing, and safe parameter binding remove the need to memorise schemas while keeping free-text SQL entry deferred to a post-v1 advanced mode. No column-type filtering or type-specific input hints are required in v1.

The user builds SELECT, UPDATE, DELETE, and INSERT queries by stepping through fields: command, table, column(s), where, group by, order by, and limit. Values use universal text entry with deterministic parse-and-bind coercion. Queries run with Enter. SELECT results appear in a frozen-header grid with serialized vertical paging, horizontal scrolling, and an independently read count. In-session query and result histories each retain the 20 most recent entries. Queries and results can be exported to `.sql`, CSV, or JSON files.

## Query Grammar (v1)

Sqloid v1 is a **builder for a subset of SQL**, not a full query engine:

- **SELECT**: `SELECT <projection> FROM <table> [WHERE <predicate>] [GROUP BY <column, ...>] [ORDER BY <column-or-aggregate> [ASC|DESC]] [LIMIT <n>]`.
  - A projection is either the sole wildcard `*`, or an ordered list of entries `(column, aggregate?)`, where aggregate is empty (`Value`) or one of {Count, Min, Max, Avg, Sum}. Different aggregates on one column may coexist; an identical pair is not added twice.
  - In an empty Column(s) popup, `*` is the default-selected first item and the synthetic `COUNT(*)` item is immediately below it. `COUNT(*)` appears only while the projection is empty. Selecting it adds the sentinel directly, without opening the named-column aggregate popup, and reopens Column(s). It can coexist with subsequently selected aggregate entries. Removing entries back to empty makes it reappear. Named columns always continue to the aggregate popup. `MIN(*)`, `MAX(*)`, `AVG(*)`, and `SUM(*)` are never offered.
  - Selecting wildcard clears prior entries and makes it the sole entry. Removing an entry removes the most recently added one; reordering is unsupported.
- **WHERE (SELECT/UPDATE/DELETE)**: zero or one predicate `<column> <operator> <value>`, with operator from {`=`, `!=`, `<`, `<=`, `>`, `>=`, `IS NULL`, `IS NOT NULL`, `LIKE`}. All operators are offered for all columns. Null operators take no value. AND/OR, parentheses, and IN are unsupported.
- **GROUP BY**: assisted multi-select of table columns; duplicates are prevented and empty means no GROUP BY. In every query that has GROUP BY, every nonaggregate projected column must be grouped. Wildcard with GROUP BY is prohibited. Mixed aggregate/nonaggregate projection without GROUP BY is invalid. All-aggregate projection without GROUP BY is valid.
- **ORDER BY**: one column/expression with direction defaulting to ASC. Aggregate/grouped queries offer only grouped columns and selected aggregate entries.
- **LIMIT**: integer 1 through 9,223,372,036,854,775,807. Zero, malformed input, and overflow are invalid; empty means an unbounded logical result.
- **UPDATE**: `UPDATE <table> SET <column> = <value>, ... [WHERE <predicate>]`; one or more unique SET columns and a completed value for each are required.
- **DELETE**: `DELETE FROM <table> [WHERE <predicate>]`.
- **INSERT**: `INSERT INTO <table> (<column>, ...) VALUES (<value>, ...)`; each insertable column is Value, NULL, or Default/Omit. If all are omitted, emit `INSERT INTO "table" DEFAULT VALUES`. INSERT has no WHERE/GROUP BY/ORDER BY/LIMIT.

LIKE values are bound verbatim: `%` and `_` retain SQLite wildcard meaning, with no v1 escape mechanism. Case behaviour is SQLite's default and unrelated to case-insensitive popup search. Joins, subqueries, expressions, IN, HAVING, and multiple predicates are out of scope.

## Runnable-State Contract

QueryBuilder data validity and UI context gates are separate. A state is data-runnable only when the following authoritative prerequisites hold; Enter executes only when it is also in a base context with no overlay or focused text/search input, no request is in flight, and the screen is large enough.

| Command | Required data | Command-specific validity | Common gates |
|---|---|---|---|
| SELECT | Eligible table; nonempty projection | Valid WHERE and Limit; every grouped nonaggregate is grouped; wildcard not grouped; mixed aggregate/nonaggregate without GROUP BY invalid; all-aggregate without GROUP BY valid; valid grouped ORDER BY | Selected command; all identifiers still exist in refreshed schema; no incomplete value prompt |
| UPDATE | Eligible table; at least one SET assignment; every SET value complete | Optional WHERE complete; SET columns unique | Same common gates; destructive pre-execution workflow required |
| DELETE | Eligible table | Optional WHERE complete | Same common gates; destructive pre-execution workflow required |
| INSERT | Eligible table with at least one insertable column; every prompted column complete | Each column has exactly one Value/NULL/Default choice; all-omit is valid and emits `DEFAULT VALUES`; a zero-insertable-column table is never runnable | Same common gates |

When Enter is attempted on invalid data, execution does not start: focus moves to the first invalid field in visual order and a specific inline reason is shown. An invalid UI context consumes Enter according to that context instead.

## Builder and Display Interaction

- Choosing S/U/D/I on the Command field immediately selects/replaces the command and advances to Table. A command change retains the table only if it remains eligible, clears all downstream command-specific state, and focuses the first newly required field; switching a selected view to a write command clears Table.
- The refreshed Table popup and searchable column/GROUP BY/ORDER BY popups use case-insensitive subsequence matching: empty search shows all, no match keeps the popup open with `no matches`, and changing search resets its highlighted selection. Aggregate and operator popups are scroll-only. Single-select Enter accepts; multi-select Enter adds and reopens; Esc preserves only already completed multi-selections.
- WHERE guides column → operator → value. UPDATE guides SET multi-selection → one value per selected column → optional WHERE. INSERT guides one {Value, NULL, Default/Omit} choice per insertable column. Shift+Tab/arrows can revisit UPDATE/INSERT prompts with prior values pre-filled.
- Left/Right and Tab/Shift+Tab move builder focus. Up/Down only moves popup selection or toggles ORDER BY direction in its base field. Backspace/Delete removes the most recent projection entry or clears the focused whole-value field as appropriate.
- The grid freezes its deduplicated header, shows the displayed absolute range and independent count status, and supports Shift+Page Up/Down plus `,`/`.` for horizontal movement. Empty SELECTs show an explicit `No rows` result rather than a blank grid. Query/write errors replace the result view but older results remain available after finalization.
- Ctrl+P/N walks query history and restores complete builder state without mutating history. Ctrl+E/Y enters result history and views immutable snapshots without re-fetching. UPDATE/DELETE summaries show actual `RowsAffected()` and INSERT shows rows added, each with its executed query.

## Execution and Result Lifecycle

### Identities and state

An **active SELECT** is distinct from a **request in flight**. It can remain active between requests and own future page requests. Every actual SELECT or write execution has an execution ID. Each database request also has a request ID; page requests include a viewport generation. Resizing or deactivating/finalizing a SELECT advances the relevant generation. A response may mutate UI/cache only if its execution, request, and generation are current.

Destructive estimation is a **pre-execution workflow**, not an execution. It has its own preparation identity and phases `estimating` and `awaiting-confirmation`. Opening it appends neither query nor result history. Esc, Ctrl+W while estimating, or any dismissal cancels preparation and creates no history. Confirming starts the sole actual write execution, appends query history subject to normalized deduplication, and ultimately creates exactly one result entry.

An **accepted quit** is either `q` from a context where it is enabled or confirmation in the quit modal. During preparation it cancels any estimate, waits for that request to settle, and exits without appending either history. During an active SELECT it cancels and settles pending requests, finalizes the SELECT once under its cancellation/success state, and exits; an idle active SELECT finalizes directly. During a write it follows the commit-boundary rules below. “Without confirmation” never means abandoning pending database or transaction cleanup.

### SELECT

1. Enter on a runnable SELECT starts one execution and appends query history subject to deduplication. Its first page and count are concurrent, independent cancellable autocommit reads. They may observe different committed states.
2. At most the count request and one page request may coexist. All page requests, including the first and later navigation, are serialized. Page Up/Down is ignored while a page request is pending, with visible loading feedback. Horizontal scrolling remains local.
3. Ctrl+W cancels all active requests for the active SELECT, including a later page-only request. Late responses from a cancelled old execution are discarded even after a newer execution begins.
4. Resize invalidates the pending page request's viewport generation. Its stale response is rejected; after cancellation/settlement, the required page for the new exact page size is fetched. Entering result history or otherwise finalizing/deactivating also rejects late page responses.
5. Count failure does not end successful paging. The header changes from `Counting rows...` to `Count unavailable`; it never represents the count as an exact shared snapshot or clamps displayed/cached rows to an inconsistent count.
6. A SELECT is finalized only by: starting an actual new execution (opening an estimate is not enough), entering result history, cancellation or failure that ends the SELECT, or an accepted quit. Builder editing/focus, popups, help, save flow, write estimation, query-history browsing/restoration, resize, result export/copy, and idle periods do not finalize it.
7. Finalization creates exactly one immutable result-history entry for that execution. Success, count failure with rows, partial page failure, and cancellation after rows retain the captured rows and metadata. Cancellation before rows creates a non-tabular Cancelled entry; first-page failure creates an error entry. There is no additional entry for each page or count request.

### Cache and snapshot invariant

The active cache is one contiguous inclusive range of **absolute logical row positions**, capped at 10,000 positions. Duplicate-valued rows remain separate positions.

- A forward adjacent page appends/replaces overlap and evicts the low end when over cap. A backward adjacent page prepends/replaces overlap and evicts the high end when over cap.
- Overlap replaces the same logical positions and never duplicates them. A stale response whose requested range is nonadjacent to the current retained range after accounting for overlap is rejected rather than creating a gap.
- Snapshot/export rows are ordered by ascending logical position, regardless of traversal order.
- Metadata records retained start/end, optional known total, reached-low/reached-high endpoints, whether eviction occurred, a **completeness status**, and a separate **terminal outcome**. Completeness uses `complete`, `partial`, and `truncated` labels: `complete` is exclusive and requires both logical endpoints known plus the entire logical result retained; `truncated` means known or observed rows were evicted or exceed the retained range; `partial` means unseen rows may remain or count/page work did not finish. Incomplete snapshots retain both labels when truthful (for example, partial + truncated when rows were both unseen and evicted). Terminal outcome is independently `success`, `cancelled`, or `failed`, with cancellation/failure reason and last failure position where applicable.
- If count failed or is unavailable, an observed short or empty final page establishes the high endpoint. Otherwise the total remains unknown and the snapshot is partial unless both endpoints are established by observation. Count/cache inconsistencies are not clamped.

### Writes and commit boundary

INSERT starts its actual execution directly. UPDATE/DELETE follow `builder → estimating → awaiting-confirmation → beginning → executing → rollback-cleanup or committing → committed/failed/cancelled/outcome-unknown`.

- `beginning` and `executing` are cancellable. An atomic cancellation check occurs after statement completion and immediately before beginning COMMIT. Cancellation or statement failure initiates rollback cleanup.
- Rollback cleanup and `committing` are noncancellable. Ctrl+W after the commit boundary is ignored with `Commit in progress; cancellation is no longer available` feedback.
- A cancellation or statement failure guarantees the database is untouched only after rollback is confirmed successful. A successful commit yields one write summary entry. Each actual write produces exactly one result entry.
- If rollback or commit cannot be resolved, the result is **outcome unknown**. The application waits until no transaction or driver work remains pending, then enters the outcome-unknown terminal state. That state forbids further database work but preserves in-memory query/result save/export and immediate error-status quit controls.
- An accepted quit while a write is cancellable requests cancellation and waits for rollback resolution. An accepted quit while committing waits for commit resolution. It never tears down the process while transaction/driver work remains pending; only after resolution, or after pending work has ended with an unknown outcome, does quit complete.

## Global Key Precedence and Context/Action Matrix

Precedence is: **terminal state → quit confirmation → top overlay → focused text/search input → request-in-flight restrictions → base context**. A higher level consumes the key before lower levels.

- In every nonterminal context, Ctrl+C opens the quit confirmation, suspending the exact current context (focus, overlay, popup search, picker state, active SELECT, viewport). Ctrl+C in quit confirmation confirms quit. Esc/n cancels quit and restores that exact suspended context and focus.
- In terminal states, terminal rules override all others. In ordinary UI, Esc cancels only the top overlay and restores its opener's exact focus/state. Modals do not stack, except that quit temporarily suspends one overlay.
- `?` inserts a literal question mark in focused text or popup/file-picker search input. It opens contextual help only in base contexts. Help Esc returns exact prior focus.
- Base actions are available only with no overlay and no focused input/search. Query/result history and Ctrl+S/Ctrl+X are disallowed while any database request is in flight, with explanatory feedback.
- Ctrl+W applies only to cancellable active database requests/phases. Elsewhere it is ignored. It cancels active SELECT count/page requests and cancellable write phases; during commit/rollback cleanup it is ignored with boundary feedback.

| Context | Enter / printable | Esc | Navigation | Ctrl+P/N, Ctrl+E/Y | Ctrl+S / Ctrl+X | Ctrl+W | `?` | `q` / Ctrl+C |
|---|---|---|---|---|---|---|---|---|
| Base builder/result | Enter runs valid state; Command accepts S/U/D/I | Clears displayed error or no-op | Tab/arrows fields; Page Up/Down serialized paging; Shift+Page or `,`/`.` horizontal | Query/result history when idle | Save/export when idle and target exists | Cancels active request only | Help | `q` requests quit without confirmation only on Command; Ctrl+C opens quit confirmation |
| Popup | Select/add | Close unchanged or preserve completed multi-selections | Popup Up/Down; printable search where searchable | Disallowed | Disallowed | Cancels request, not popup, only if one exists | Inserts in searchable popup; otherwise no-op | `q` treated as input/search if applicable; Ctrl+C opens quit confirmation |
| Text/search input | Submit value | Restore prior value and focus | Input editing | Disallowed | Disallowed | Cancels request only if one exists | Inserts `?` | `q` inserts; Ctrl+C opens quit confirmation |
| Non-quit modal/overlay, including estimate/confirm/save/overwrite/help | Modal-specific | Cancel top overlay and restore opener | Modal-specific | Disallowed | Disallowed | Estimate/request cancellation only where stated | No-op unless focused search | Ctrl+C opens quit confirmation |
| Quit confirmation | Enter/y/Ctrl+C confirms | Esc/n restores exact suspended context | No other action | Disallowed | Disallowed | Disallowed | No-op | `q` no-op |
| Request pending in otherwise base context | Enter ignored with running hint | Base Esc behavior | Page Up/Down ignored while page pending; local horizontal allowed | Disallowed | Disallowed | Cancel if phase cancellable | Base help only if no input/overlay | `q` requests quit without confirmation only if Command has focus; Ctrl+C opens quit confirmation |
| Terminal deletion/outcome-unknown | No DB action | No-op | History selection if available | Allowed for in-memory selection | Allowed from immutable memory; by definition no transaction/driver work remains pending | No-op | Reduced help | `q` and Ctrl+C exit immediately with status 1 |
| Too-small screen (<80×24) | Other keys ignored | No-op | No-op | Disallowed | Disallowed | Active request cancellation remains available | No-op | `q` requests quit without confirmation; Ctrl+C opens quit confirmation |

The too-small screen preserves all application state behind the message. Resize to at least 80×24 restores the exact prior context and focus. In both terminal deletion and outcome-unknown states, no transaction/driver work remains pending and `q` or Ctrl+C exits immediately with status 1. In nonterminal states, `q` on Command or the too-small screen skips only the confirmation modal: it remains an accepted quit and performs the preparation, SELECT, or write cleanup specified above.

## User Stories

1. As a user, I want `sqloid sqlite <file>` to open a SQLite database directly.
2. As a user, I want a usage error when the `sqlite` command has no file argument.
3. As a user, I want missing, unreadable, directory, non-SQLite, read-only, and failed-probe inputs rejected without creating a file.
4. As a user, I want `sqloid d1` to discover my local wrangler database relative to the working directory.
5. As a user, I want D1 discovery to inspect `.wrangler/state/v3/d1/miniflare-D1DatabaseObject`, use its exact case-sensitive candidate rules, and ignore metadata/sidecar files.
6. As a user, I want exact, actionable errors for zero and multiple D1 candidates.
7. As a user, I want successful startup silent and startup failures to name the path/reason and use the documented exit status.
8. As a user, I want the database opened read-write so UPDATE, DELETE, and INSERT persist, with no silent read-only fallback.
9. As a user, I want the original path checked before each database request so deletion or rename-away is detected on the next operation, without a false promise of continuous monitoring.
10. As a user, I want to quit at any time: `q` without confirmation on Command and the too-small screen, Ctrl+C confirmation in all nonterminal contexts, and immediate status-1 quit in terminal states; any required request/transaction cleanup completes before a nonterminal exit.
11. As a user, I want cancelling a quit confirmation to restore the exact suspended context, overlay, search, viewport, and focus.
12. As a user, I want Ctrl+W to cancel only active SELECT requests, estimate work, or cancellable write phases, rather than conflict with quit.
13. As a user, I want the UI to remain interactive with `Running…`, `Counting rows…`, page-loading, estimation, commit, and rollback feedback.
14. As a user, I want Enter ignored while a request is in flight, with a Ctrl+W hint, so requests cannot stack.
15. As a user, I want results including their header to occupy at least half the screen and the builder at most one-third, without overlap.
16. As a user, I want a growing builder to scroll so the complete focused field remains visible.
17. As a user, I want initial focus on Command and one S/U/D/I keypress to choose a command and advance to Table.
18. As a user, I want returning to Command and choosing another command to replace it, retaining only an eligible table and clearing downstream fields.
19. As a user, I want switching a selected view to a write command to clear Table and focus it, while eligible ordinary/virtual tables remain selected.
20. As a user, I want the main-schema table popup refreshed whenever it opens and to list eligible tables/views while excluding `sqlite_%` and `_cf_METADATA`.
21. As a user, I want searchable popups to scroll and use case-insensitive subsequence matching, with defined empty/no-match/reset behavior.
22. As a user, I want refresh failures to retain a visibly stale list and offer retry/cancel, while deletion takes terminal precedence.
23. As a SELECT user, I want Column(s) to open with `*` first and default-selected so plain `SELECT *` is fastest.
24. As a SELECT user, I want `COUNT(*)` immediately below `*` only when Column(s) is empty; selecting it adds the sentinel directly and reopens Column(s).
25. As a SELECT user, I want removing the final entry to make `COUNT(*)` reappear, and selecting a named column to continue through the Value/Count/Min/Max/Avg/Sum popup.
26. As a SELECT user, I want wildcard to be the sole projection and named `(column, aggregate)` entries to preserve order, permit different aggregates, and reject exact duplicates.
27. As a user, I want Backspace/Delete to remove the most recent projection or clear the appropriate whole field, with no effect when already empty.
28. As a user, I want WHERE guided through column, operator, and value, including no-value NULL operators and verbatim LIKE wildcard semantics.
29. As a user, I want GROUP BY as assisted multi-select, ORDER BY as one valid grouped/aggregate candidate with direction toggle, and Limit as bounded positive integer entry.
30. As a user, I want every nonaggregate projected column grouped in every GROUP BY query, wildcard GROUP BY prohibited, and mixed aggregate/nonaggregate without GROUP BY rejected.
31. As a user, I want all-aggregate projection without GROUP BY to remain valid.
32. As a user, I want invalid Enter to focus the first invalid field and explain the exact command-specific prerequisite.
33. As an UPDATE user, I want to choose one or more SET columns, enter one value for each, optionally add WHERE, inspect an estimate modal, and then confirm.
34. As a DELETE user, I want an optional assisted WHERE followed by the same estimate/confirmation safety workflow.
35. As an INSERT user, I want one {Value, NULL, Default/Omit} prompt for each insertable column, with empty Value distinct from NULL and omission.
36. As an INSERT user, I want all prompted columns omitted to emit `DEFAULT VALUES`, while a table with zero insertable columns reports `table has no insertable columns` and cannot run.
37. As a write user, I want to revisit prior UPDATE/INSERT prompts with choices and values pre-filled.
38. As a user, I want deterministic verbatim integer/real/text parsing and SQLite affinity, without unsupported column-type filtering or hints.
39. As a destructive-write user, I want the modal to show operation, table, rendered SQL with literal values, and a prominent no-WHERE all-rows warning.
40. As a destructive-write user, I want `Estimating matching target rows…` with confirmation disabled until estimation succeeds or fails.
41. As a destructive-write user, I want estimate failure displayed without hiding SQL/warnings and then to be allowed to confirm deliberately.
42. As a destructive-write user, I want Enter/y to confirm and Esc/n to dismiss; dismissal or estimate cancellation creates no query/result history.
43. As a destructive-write user, I want the estimate to count matching target rows from the identical WHERE predicate only, not UPDATE SET values, trigger effects, or guaranteed changed rows.
44. As a user, I want confirmation to start the sole actual write execution and produce exactly one result-history entry.
45. As a user, I want writes transactional, with cancellation before commit followed by rollback cleanup and no untouched claim until rollback is confirmed.
46. As a user, I want Ctrl+W ignored with feedback after the commit boundary, and any accepted quit to wait for commit/rollback resolution.
47. As a user, I want unresolved commit/rollback to end pending work before entering an outcome-unknown terminal state with in-memory save/export and status-1 quit.
48. As a user, I want Page Up/Down to fetch vertical pages and Shift+Page Up/Down plus `,`/`.` to scroll horizontally.
49. As a user, I want only one page request pending, repeated/opposite Page keys ignored with feedback, and count plus at most one page request concurrently.
50. As a user, I want Ctrl+W to cancel first/later page and count requests together and stale old/generation responses discarded.
51. As a user, I want a frozen deduplicated header, current absolute range, count status, and an explicit `No rows` message.
52. As a user, I want the exact page size computed from complete visible data rows at the current terminal height.
53. As a user, I want resize to preserve the exact first logical row when retained/valid, otherwise clamp to a known endpoint or fetch it, while stale old-size responses are rejected.
54. As a user, I want to browse beyond 10,000 logical positions with one contiguous cache: forward evicts low, backward evicts high, overlap replaces, and duplicate-valued rows remain separate positions.
55. As a user, I want snapshots/export in ascending logical-position order with truthful retained range, endpoints, known-total, eviction, completeness, and terminal-outcome metadata.
56. As a user, I want a short/empty final page to establish the high endpoint after count failure; otherwise an unseen remainder stays partial/unknown.
57. As a user, I want first-page rows and total count read concurrently and independently so slow counting does not delay rows.
58. As a user, I want `Counting rows…` until count completion and a visible count error without losing successful rows or paging.
59. As a user, I want help/header wording to disclose that `Count: N` is an independent autocommit read that can drift from pages and is never used to clamp them.
60. As a user, I want active SELECT lifetime independent from request lifetime, so editing/focus/popups/help/save/estimate/query history/resize/export do not finalize it.
61. As a user, I want only an actual new execution, entering result history, ending cancellation/failure, or an accepted quit to finalize an active SELECT into one immutable entry.
62. As a user, I want Ctrl+P/N to navigate the 20 most recent in-session query states and restore every builder field without mutating history.
63. As a user, I want normalized identical reruns deduplicated and actual state changes appended only when actual execution starts, not when estimation opens.
64. As a user, I want Ctrl+E/Y to navigate the 20 most recent immutable result snapshots without re-fetching; rerun gets fresh data.
65. As a user, I want query errors to replace the result view, Esc to dismiss them, and previous results to remain reachable.
66. As a user, I want UPDATE/DELETE summaries to show actual `RowsAffected()` and INSERT summaries to show rows added, with the executed query.
67. As a user, I want Ctrl+S to save a viewed result's query, otherwise the current runnable query, otherwise the last executed query, with a clear message if none exists.
68. As a user, I want saved SQL to be one standalone statement with quoted identifiers, safely serialized literal values, and trailing semicolon.
69. As a user, I want Ctrl+X unavailable during requests; when idle it takes an immutable instant copy without finalizing or changing the active SELECT.
70. As a user, I want export picker cancel/complete to return to the unchanged active result and show complete/partial/truncated and UTF warnings outside CSV/JSON data.
71. As a user, I want directory picking from the working directory, hidden directories and `..`, separate filename entry, extension appending, and inline validation/permission errors.
72. As a user, I want overwrite confirmation and temp-file-plus-rename saves that preserve an existing destination and clean temporary files on pre-rename failure.
73. As a user, I want RFC 4180 UTF-8 CSV and array-of-objects JSON using deterministic deduplicated output names.
74. As a user, I want NULL, empty strings, BLOBs, embedded tabs/newlines, and non-finite pre-existing REALs represented by the documented format-specific rules.
75. As a user, I want every maximal invalid UTF-8 TEXT sequence replaced by one U+FFFD consistently in grid/CSV/JSON, with metadata/UI warning but no extra export records; BLOBs remain unchanged.
76. As a user, I want all typed values bound and all identifiers selected from schema and double-quoted, preventing injection.
77. As a user, I want `?` to open context help only in a base context and to insert literally in focused text/search.
78. As a user, I want Esc to cancel only the top overlay and every cancel/complete path to restore its exact opener focus.
79. As a user, I want below-80×24 display to preserve hidden application state, allow `q`/Ctrl+C behavior, and restore exact context on resize.
80. As a user, I want terminal deletion and outcome-unknown states to forbid database work but permit selection and save/export from immutable memory before immediate status-1 quit.

## Implementation Decisions

- **Language and stack**: Go with pure-Go/no-cgo `modernc.org/sqlite`; Bubble Tea for the TUI, Lip Gloss for styling, Bubbles components where appropriate, and `mow.cli` command-word/argument parsing for `sqlite <file>` and `d1`.
- **CLI behavior**: `--help`/`-h` prints usage; `--version`/`-v` prints version; unexpected arguments print a usage error to stderr and exit 2. A directory passed to `sqlite` fails the header validation as `not a SQLite database` and exits 1. A missing sqlite argument is a usage error; validation never creates the target.
- **D1 discovery**: resolve `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` relative to process working directory. Candidates have case-sensitive `.sqlite` extension and no lowercase `metadata` substring; `-shm`/`-wal` sidecars are ignored. Case sensitivity is deliberate for identical Linux/macOS sets. Exactly one candidate is required. Multiple candidates produce exactly `There is more than one SQLite database in .wrangler`; an absent, unreadable, or candidate-free directory produces `no candidate database found in .wrangler`.
- **Startup validation and errors**: existence check → readable check → exact 16-byte `SQLite format 3\0` header check → open read-write without creating (`mode=rw`) → harmless `PRAGMA schema_version` probe. Any failure, including read-only open or probe busy-timeout, prints one line to stderr naming the file and OS/driver reason and exits 1. Successful startup writes nothing to stdout/stderr. There is no degraded read-only mode.
- **Busy handling**: a five-second busy timeout applies to the startup probe and every request. Startup expiry exits as above; mid-session expiry appears as ordinary `database is locked` request error unless the path recheck classifies deletion.
- **Session health**: no watcher or continuous monitoring. Check the original path immediately before every database request: schema refresh, estimate, count, page, or an entire phased write transaction. The write transaction is one request, so there is one check before BEGIN and no path check between its statement and COMMIT. If the path is absent, including rename-away, enter the full-screen terminal deletion state `Database file no longer exists — session ended`. On any request error, recheck the path: absent means terminal deletion; present means ordinary request/transaction error or outcome-unknown handling as applicable. Idle deletion is discovered only on the next operation. Replacement at the same path is not detected.
- **SQL safety**: user values are bound; identifiers come only from refreshed schema and are double-quoted with embedded quotes doubled. Aggregate and direction values are fixed choices.
- **Numeric value parsing**: input is verbatim with no trimming; whitespace anywhere makes TEXT. INTEGER matches `-?[0-9]+` and must fit signed 64-bit (leading zeros allowed; leading `+` falls through). Otherwise REAL is any finite value accepted by `strconv.ParseFloat` within float64 (`1.`, `.5`, exponent forms allowed; leading `+` and overflow fall through). Everything else is TEXT verbatim, including `NaN`, `Inf`, `0x1A`, and padded strings. SQLite column affinity may coerce bound values. NULL is available only through explicit popup/operator choices; empty input is empty TEXT. BLOB input is unsupported.
- **Estimate SQL and modal**: the estimate is exactly `SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]`. It binds only WHERE parameters in predicate order, never UPDATE SET parameters. It is an independent read labeled `estimated matching target rows`; it excludes trigger side effects and may differ from changed/no-op rows and `RowsAffected()`. The modal opens immediately and continuously displays operation type, table name, rendered write SQL with safely serialized literal values, and the prominent no-WHERE all-rows warning when applicable. It initially shows `Estimating matching target rows…`, with Enter/y confirmation disabled. When the estimate succeeds it shows the estimate and enables confirmation; when it fails it shows the failure while retaining SQL/warnings and also enables confirmation. Enter/y then confirms the sole actual write; Esc/n dismisses preparation without history. Ctrl+W while estimating cancels the estimate and dismisses preparation without history.
- **Write transaction**: Connection explicitly implements beginning/executing, atomic pre-commit cancellation check, noncancellable rollback cleanup/committing, and outcome resolution. It must not report cancelled/failed-and-untouched before rollback is confirmed.
- **Paging consistency**: LIMIT/OFFSET requests. The logical-result count is generated as `SELECT COUNT(*) FROM (<SELECT including the user's LIMIT>)` and therefore attempts to count the limited logical result. Without user ORDER BY, append `ORDER BY rowid` only for ordinary rowid tables without a declared rowid shadow; no implied stability for views, virtual/WITHOUT ROWID/shadowed, aggregate/grouped, ties, or concurrent writes. Count and page are separate concurrent autocommit reads with no shared snapshot and may drift. Header `Count: N` means the independent count response, not an exact total for displayed pages; never clamp inconsistencies. Help discloses this.
- **Grid rendering/cache**: exact page size equals the number of complete grid data rows available after borders/header/status, excluding the frozen header. Cache and metadata follow the lifecycle invariant. Cells truncate at computed width with an ellipsis; tabs/newlines render as visible symbols; Unicode widths use a runewidth approach. Duplicate headers use the full-set deduplication rule. If the terminal is narrower than minimum column widths, horizontal scrolling is used rather than corrupting layout.
- **Resize/layout**: results region including its header is at least half of terminal rows. Builder uses only needed lines up to one-third of terminal rows; overflow scrolls so the full focused field/prompt is visible. Regions and borders never overlap or render outside bounds. Resize recomputes exact page size and preserves exact first logical position if retained/valid, otherwise clamps to a known endpoint or requests the containing page.
- **Invalid UTF-8 TEXT**: decode each maximal invalid byte sequence as exactly one U+FFFD consistently in grid, CSV, and JSON. BLOB handling is unchanged. Result metadata records that replacement occurred; the results header and export flow show a warning. Export succeeds, emits no extra warning records/fields, and retains row count/order.
- **Export formats and values**: CSV is RFC 4180 UTF-8 with CRLF, a header row, minimal quoting, and embedded tabs/newlines preserved in quoted fields. NULL and empty string both become an empty CSV field, an accepted lossy limitation. JSON is an array of objects: INTEGER/REAL are numbers, NULL is `null`, and non-finite pre-existing REALs are quoted `"Inf"`, `"-Inf"`, or `"NaN"`; CSV uses their textual form. BLOB displays `[BLOB n bytes]`, exports as lowercase hex CSV and base64 JSON. Computed/aggregate columns retain SQLite labels such as `COUNT(*)` before deduplication.
- **Output names**: walk left-to-right across the full original output-name set. First occurrence keeps its name; each duplicate gets the lowest `_2`, `_3`, etc. suffix not colliding with any already-final name, including original names. The same names appear in grid, CSV headers, and JSON keys; generated SQL and driver metadata are never altered.
- **History**: in-memory and session-only. Each list retains 20 recent entries with oldest-first eviction. Query history stores complete field state; restore copies it into the builder and never mutates the entry. Append occurs only when an actual execution starts and only if normalized state differs from the last executed state: command, table, ordered projection (column/aggregate and wildcard), WHERE column/operator/value/bound type, GROUP BY order, ORDER BY/direction, Limit empty-vs-number, and UPDATE/INSERT values/choices. Entered representation, value type (`'1'` TEXT versus `1` INTEGER), and column order are significant. Result history stores immutable snapshots with retained positions and all cache completeness/terminal/UTF metadata. Historical results never re-fetch; rerun gets fresh data. Snapshots re-slice to current terminal height when viewed.
- **Result export scope**: Ctrl+X is disabled during any request. At idle it captures an immutable instant copy of active rows and metadata before opening the picker, without finalizing the SELECT. Picker cancel or successful completion returns to the unchanged active result. Historical snapshots export identically. In a terminal state, the last result snapshot is the initial export target and Ctrl+E/Y selects another. Error results, cancelled-before-rows markers, and write summaries have no tabular target, so Ctrl+X reports that and opens no picker. To deliberately export a bounded subset, set Limit before execution.
- **Export warnings**: before destination selection/confirmation, the UI identifies complete, partial, and/or truncated state plus cancelled/failed and invalid-UTF information where applicable. These metadata/warnings never become CSV rows, CSV columns, JSON objects, or JSON properties.
- **Query save targeting**: one-way export only; loading is unsupported. Ctrl+S targets a viewed historical result's associated query; otherwise the current builder if runnable; otherwise the last actually executed query. If none exists, show inline `no runnable query to save` and do not open the picker. Terminal-state default is the last executed query, with Ctrl+P/N selection. Saved SQL is one standalone executable statement with identifiers double-quoted, strings quote-doubled, `NULL` keyword, BLOBs as `X'hex'`, and trailing semicolon.
- **File picker**: starts in working directory; displays directories including hidden ones and supports parent `..`; directory creation is unsupported. Directory selection and filename text entry are separate. Empty basename or basename containing `/` or NUL is invalid and shows inline error. Missing `.sql`/`.csv`/`.json` extension is appended. Esc cancels to the exact opener; permission/path errors stay inline for retry/cancel. Existing files require overwrite confirmation.
- **Atomic saves**: write a temporary file in the destination directory and rename it over the target. Serialization/I/O failure before rename preserves any existing destination and cleans the temp file. Temp-and-rename can itself fail under restrictive destination-directory permissions; this accepted limitation surfaces inline without claiming replacement occurred.
- **Schema scope and refresh**: query `main.sqlite_master` only for ordinary tables, virtual tables, and views, excluding `sqlite_%` and `_cf_METADATA`. Views are SELECT-only; write command lists contain tables. Refresh object listing every time Table opens and refresh chosen-table columns fresh, so external create/drop/change appears on the next operation. Lock/corruption/change failures show inline `could not refresh: …`, retain stale data with a notice, and offer retry/cancel; path deletion overrides with terminal state.
- **Schema metadata**: every object carries kind {ordinary-table, virtual-table, view}, rowid capability {has-rowid, without-rowid, not-applicable}, and declared-rowid-shadow flag. Columns carry name, declared type, and insertability from `PRAGMA table_xinfo`; hidden/generated columns are noninsertable. Declared type is metadata only and does not add type-specific v1 entry behavior.
- **INSERT handling**: prompt every insertable column; there is no AUTOINCREMENT-based skip. Each prompt offers Value/NULL/Default/Omit; Value opens text entry and empty is empty TEXT, NULL binds explicit NULL, and Default/Omit excludes the column. INTEGER PRIMARY KEY is prompted with `(auto-assigned if omitted)`. All prompted columns omitted emits `INSERT INTO "table" DEFAULT VALUES` through the normal execution path. A table with zero insertable columns shows `table has no insertable columns`, never enters prompts, and is non-runnable. Virtual tables are best effort using visible insertable columns; module failures requiring hidden inputs are ordinary query errors.
- **Builder lifecycle**: changing command retains only an eligible table, clears all downstream state, and focuses the first requirement. Restoring history copies state. Popups/modals/text entry remain input-modal under the precedence table. Enter starts execution only when the authoritative data table and UI gates both pass.
- **Keybinding portability**: bindings use terminal-distinguishable plain keys or Ctrl chords. No Alt or Ctrl+Shift binding is required because common terminals/multiplexers cannot distinguish or reserve them. `,`/`.` are reachable horizontal-scroll fallbacks when Shift+Page is intercepted. This PRD's context/action table is authoritative; there is no external key matrix.
- **Errors and timeouts**: query errors replace results and create the lifecycle-defined single entry; Esc dismisses to the builder and older results remain. Count failure leaves rows/paging active with count error. Mid-scan page failure finalizes a snapshot with failure position. Save/path errors remain in save flow. Only startup failure, detected deletion, and resolved outcome-unknown end database work. v1 has no query timeout: requests run until completion or accepted cancellation.
- **Logging**: none in v1—no debug flag or log file. Diagnostics stay in the TUI to avoid display corruption and sensitive query-value logs.

## Module Design

- **Connection**
  - **Responsibility**: owns database discovery/opening, handle, health checks, async request cancellation/identity, SQL plumbing, and transaction outcome resolution while hiding the driver from callers.
  - **Interface**: open-by-path with the full validation sequence; open-D1 with exact discovery errors; cancellable independent `ExecutePage(query, offset, limit) → rows/error`, `ExecuteCount(query) → count/error`, and `ExecuteEstimate(query, whereParams) → count/error`; phased transactional `ExecuteWrite(statement) → rowsAffected/definite-or-unknown outcome`. It checks the path once before each request (once before the entire write transaction), rechecks after errors, and exposes enough request identity for UI generation guards.
  - **Tested**: unit tests with a controllable fake for ordering/cancellation/outcomes plus SQLite integration tests, especially discovery, validation, drift behavior, rollback, and commit boundaries.
- **Schema**
  - **Responsibility**: describes main-schema objects and columns independently of UI.
  - **Interface**: lists object kind, rowid capability, rowid shadowing, and columns with name/declared type/insertability; refresh failures are typed so UI can retain stale data and distinguish deletion.
  - **Tested**: table-driven catalog fixtures and SQLite integration tests for views, virtual/WITHOUT ROWID tables, shadowing, hidden/generated columns, refresh/drop, and zero-insertable tables.
- **QueryBuilder**
  - **Responsibility**: stores command-specific structured state (SELECT projection/WHERE/GROUP/ORDER/Limit, UPDATE assignments/WHERE, DELETE WHERE, INSERT ordered choices) and produces SQL plus bound params.
  - **Interface**: immutable get/set transitions; execution and exact destructive-estimate rendering; authoritative runnable report with first invalid field/reason; grouping/order rules; normalized history comparison. No UI dependency.
  - **Tested**: thorough pure tests, including identifier quoting, parsing/binding, exact parameter order, grouping matrix, DEFAULT VALUES, and every runnable prerequisite.
- **UI**
  - **Responsibility**: Bubble Tea model for field bar, grid, popups, text entry, modals, file flow, global precedence/context table, focus restoration, preparation/execution state machines, request/generation guards, serialized paging, contiguous cache, metadata warnings, terminal states, and layout.
  - **Interface**: Bubble Tea `Init`/`Update`/`View`, composing all other modules without embedding database behavior.
  - **Tested**: scripted `(model, msg) → (model, cmd)` behavior with fake Connection for transitions, popups, modal/quit precedence, history, errors, export, cancellation and late responses. Pixel/terminal rendering remains the specified manual matrix.
- **History**
  - **Responsibility**: two in-memory 20-entry lists, navigation positions, oldest-first eviction, normalized query deduplication, and one immutable result snapshot per actual execution.
  - **Interface**: append/query/step/current operations; no preparation append; snapshots capped at 10,000 contiguous positions with separate completeness and terminal outcome, endpoint/range/count/eviction/failure/cancellation/UTF metadata.
  - **Tested**: yes, including caps, dedup, finalization events, non-finalization events, metadata combinations, and terminal entries.
- **Export**
  - **Responsibility**: serializes an already immutable result copy to CSV/JSON and a query to SQL, then saves atomically.
  - **Interface**: result rows plus deduplicated columns and value policy; standalone query serialization; temp-and-rename path write. Completeness/outcome/UTF metadata drives UI warnings only and never alters data formats.
  - **Tested**: exact bytes for RFC 4180/JSON/SQL, value/name/invalid-UTF cases, and injected serialization/I/O/rename failures proving destination preservation.

## Testing Decisions

Tests target external contracts rather than private structure: given builder state, assert SQL/params/runnable reason; given rows, assert exact CSV/JSON; given directory/catalog state, assert discovery/schema behavior. Use standard Go table-driven unit tests, Bubble Tea `(model, msg) → (model, cmd)` scripted behavior, a controllable fake Connection for deterministic request order/cancellation/transaction outcomes, and `modernc.org/sqlite` integration tests for actual SQL, lock, schema, and transaction behavior. The project is greenfield with no legacy test convention to preserve.

Required baseline UI flows include command → table → columns → run; every popup accept/cancel/search path; history navigation/repopulation; save/export picker, overwrite, retry, and cancel; estimate modal success/failure/cancel/confirm including unqualified warning; `q`, Ctrl+C, Ctrl+W, and Esc precedence; error display/dismissal; and exact focus restoration. View layout is deliberately manual-only under the matrix below.

Required high-risk coverage includes:

1. Preparation identity and all estimate/confirmation/dismissal/query-history/result-history transitions; exactly one entry only after confirmed actual write.
2. Cancelled old execution followed by a new execution, with every late count/page response rejected.
3. Serialized paging: repeated/opposite Page keys while pending, count plus at most one page, loading feedback, resize generation invalidation, deactivation, and Ctrl+W on later page-only work.
4. Bidirectional traversal beyond 10,000 positions: forward low eviction, backward high eviction, alternating directions, overlap replacement, nonadjacent stale rejection, duplicate-valued rows as separate positions, and ascending export.
5. Metadata classification for known/unknown totals and observed short/empty endpoints after count failure; separate complete/partial/truncated (including truthful combinations) from success/cancelled/failed; never clamp inconsistencies.
6. Every GROUP BY boundary: grouped nonaggregate-only projections, mixed projections with/without GROUP BY, all-aggregate without GROUP BY, wildcard rejection, and ORDER BY candidates.
7. Global key precedence in every row of the context table, Ctrl+C from every modal/overlay/input and exact restoration, Esc top-overlay behavior, `?` insertion, accepted `q`/confirmed-quit cleanup during preparation and active SELECT, too-small controls, and focus restoration.
8. Active SELECT finalization/non-finalization events; one history entry; Ctrl+X disabled during requests; idle immutable instant export; picker cancel/complete leaves active result unchanged; metadata warnings absent from data.
9. Exact estimate SQL and parameters for UPDATE/DELETE with absent WHERE, NULL operators, LIKE, quoted identifiers, and proof that UPDATE SET params are excluded.
10. Deletion before each request, including exactly one pre-check before the entire phased write transaction and no check between statement/COMMIT; rename-away; idle deletion discovered next operation; request-error post-check classification; and documented same-path replacement limitation.
11. Controlled concurrent count/page drift proving `Count: N` is not clamped or described as a shared snapshot, plus help disclosure.
12. Write cancellation while beginning/executing, atomic check immediately before commit, Ctrl+W after boundary, confirmed rollback guarantee, statement failure, unresolved rollback/commit terminal outcome, and quit waiting for resolution.
13. Exact bare `COUNT(*)` sequence: position below default `*`, direct sentinel add, popup reopen, absence after first selection, reappearance after removal to empty, and named-column aggregate flow.
14. Invalid UTF cases containing multiple maximal invalid byte sequences: one U+FFFD per sequence identically in grid/CSV/JSON, metadata/header/export warning, successful export with no extra records, and unchanged BLOB behavior.
15. Existing boundaries: startup/D1 fixtures, count failure with rows, schema refresh/drop, rowid ordering limitations, numeric parser, output-name collisions, NULL/BLOB/non-finite export, atomic save failure, constraints/trigger rollback, view-to-write command switch, DEFAULT VALUES, and unqualified-write warnings.
16. CLI/integration matrix: missing command argument, missing path, directory/non-SQLite/read-only file, header/probe/busy failure, silent successful startup, exact exit codes/messages, all D1 candidate cases, and end-to-end SELECT/write/export against fixture databases.
17. INSERT edge cases: zero insertable columns is non-runnable and never prompts; one-or-more insertable columns all omitted executes DEFAULT VALUES; INTEGER PRIMARY KEY omission and virtual-table hidden-input failure.

Pure rendering remains manual, but the required matrix is 80×24, 100×30, and 160×50. At each size verify: results including header are at least half-height; builder is at most one-third and scrolls the focused field into full view; exact complete-row page size; no border/region overlap; horizontal overflow; multiline long values; popups and modals at edges; first-row preservation and retained/clamp/fetch resize branches; active request resize; and transition below/above 80×24 with exact state/focus restoration.

## Out of Scope

- Any engine other than SQLite files/local D1 in v1, including PostgreSQL, MySQL, remote D1, and other remote services.
- Multiple simultaneous database connections or switching databases within one session.
- Loading/importing saved queries or results; save/export is one-way.
- Persistence of query/result history or saved-file indexes across sessions.
- Free-text SQL entry and the post-v1 full-screen SQL editing mode; the application itself remains a full-screen TUI.
- Joins, subqueries, arbitrary expressions, IN, HAVING, AND/OR, parentheses, or multiple WHERE predicates.
- Frozen columns during horizontal scrolling; only the header row is frozen.
- Direct result-cell editing.
- Transactions spanning multiple user operations; the internal single-write transaction remains in scope.
- LIKE wildcard escaping, so literal `%`/`_` search remains unsupported.
- Full virtual-table INSERT support where a module requires hidden-column input; visible-column best effort remains in scope.
- Degraded read-only sessions for unwritable databases.
- Mouse support and Windows-specific terminal concerns.
- Continuous filesystem monitoring and detection of replacement at the same original path.

## Acceptance Criteria and Supported Environment

- **Supported systems**: Linux and macOS; standard xterm-compatible key/arrow sequences; at least 16 colors; no mouse requirement. Some terminals intercept Shift+Page Up/Down, so `,`/`.` must provide equivalent horizontal scrolling.
- **SQLite support**: any SQLite 3-format database openable read-write by `modernc.org/sqlite`, including WAL mode and local miniflare D1 files. Pure-Go/no-cgo builds are required.
- **Minimum terminal**: 80×24. Below it, show `terminal too small` instead of malformed layout, preserve all application state, keep `q` available without confirmation and Ctrl+C confirmation functional, and restore exact context/focus when resized back.
- **Automated definition of done**: all user stories and authoritative tables/lifecycles are implemented; unit, fake-Connection behavior, CLI, and SQLite integration suites pass, including every high-risk case.
- **Manual definition of done**: complete the 80×24, 100×30, and 160×50 matrix and all listed resize/overlay/long-content scenarios, not only 80×24.
- **Visual invariants**: at every supported tested size, results including header occupy at least half the height; builder uses at most one-third and scrolls the complete focused field into view; page size exactly equals complete visible data rows; bounds/borders never overlap or escape; resize follows exact retained/clamp/fetch behavior.
- **Preparation/history invariant**: every preparation dismissal leaves both histories unchanged. Confirmation appends query history subject to dedup and each actual write produces exactly one result. Each finalized SELECT likewise produces exactly one result.
- **Transaction invariant**: beginning/executing accept cancellation; rollback cleanup/commit do not. Any cancelled/failed write described as untouched has confirmed rollback. Outcome unknown is terminal only after pending driver/transaction work has ended.
- **Cache/export invariant**: cache never exceeds 10,000 contiguous absolute logical positions, preserves duplicate-valued positions, handles overlap/eviction deterministically, and exports retained rows ascending with separate truthful completeness/outcome and UTF metadata reflected only in UI warnings.
- **Target envelope**: tables around 100,000 rows should be comfortable through paging, snapshots remain capped at 10,000 positions, and indexed first pages ideally render near 100 ms. These are guidance rather than definition-of-done performance guarantees; slow requests remain cancellable.

## Open Questions

None remaining. The original interviews and all findings in the first, second, and third critique are resolved by the requirements above. The third-critique decisions are incorporated explicitly in destructive preparation/history, serialized paging and cache invariants, grouping validity, key precedence, active-result finalization/export, exact estimate SQL, operation-time deletion detection, independent count/page reads, write commit outcomes, bare `COUNT(*)`, invalid UTF-8, runnable prerequisites, and measurable layout acceptance.

## Further Notes

- The Connection abstraction preserves the broader multi-engine vision while v1 remains SQLite/local D1. Free-text SQL likewise remains deferred.
- First-page and count requests deliberately use independent autocommit reads so wrangler sharing is not blocked. They can drift; `Count: N` is an informational independent result, not an exact snapshot total for the visible rows.
- The aggregate/GROUP BY rule prevents SQLite's arbitrary bare-value behavior in every grouped query, including nonaggregate-only projections; wildcard GROUP BY is therefore prohibited.
- The internal transaction provides an untouched guarantee only after rollback is confirmed. SQLite FAIL/trigger behavior motivates the wrapper, while the explicit commit boundary and outcome-unknown state avoid promises the driver cannot prove.
- Destructive matching-row estimation is independent and non-binding: it counts the quoted target under the identical WHERE predicate, not trigger effects, actual changed rows, or no-op differences.
