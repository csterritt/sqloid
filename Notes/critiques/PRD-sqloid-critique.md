# Critique: PRD: Sqloid v1

## Review Scope

This is a requirements-only review of:

- `Notes/PRD-sqloid.md`
- `Notes/Ideas.md`

It evaluates completeness, internal consistency, fidelity to the source ideas, SQL/SQLite correctness, edge cases, error handling, logging, testability, and readiness for issue generation. It does not generate implementation issues or tasks.

## Overall Verdict

**Not ready for issue generation.**

The PRD is unusually detailed for an early version and has a clear module decomposition, explicit v1 boundaries, and broad user-story coverage. However, several central behaviors are contradictory or not yet defined precisely enough to implement consistently. At least one stated SQL rule is factually wrong, several keybindings are not distinguishable in common terminals, and the paging, history, and export models conflict with one another.

The `Open Questions` claim that no questions remain is not supportable in the document's current state.

## What Is Strong

- The v1 database scope is explicit: SQLite files and local D1 only.
- CLI validation cases are identified rather than left implicit.
- The PRD distinguishes query history from result history.
- Parameter binding, paged fetching, resize handling, overwrite confirmation, and write confirmation are considered early.
- The one-way export boundary and session-only history boundary are clear.
- Responsibilities are divided into plausible modules rather than concentrated in the UI.
- Most decisions added to `Ideas.md` during the PRD interview are represented in the PRD.
- The PRD explicitly excludes several tempting sources of scope creep.

## Severity Definitions

- **Blocker**: The requirement is contradictory, technically invalid, or too incomplete to implement without inventing product behavior.
- **Major**: Implementation could proceed, but different reasonable interpretations would produce materially different behavior.
- **Minor**: The intended behavior is mostly clear but needs a precise edge-case or acceptance rule.

## Blockers

### B1. The aggregate rule is incorrect and blocks basic valid queries

**References:** PRD user story 28, Implementation Decision `Aggregate rule`, Further Notes; Ideas decision on aggregate and GROUP BY.

The PRD requires a non-empty `GROUP BY` whenever any aggregate is selected and says this is necessary because SQLite rejects mixed aggregate/non-aggregate queries without grouping. Both parts are wrong:

- `SELECT COUNT(*) FROM table` is valid and should not require `GROUP BY`.
- SQLite permits bare non-aggregate columns in aggregate queries, although their values can be ambiguous. It does not reject all such queries.
- Requiring merely a non-empty `GROUP BY` is insufficient for deterministic, standard-style semantics. A selected non-aggregate column could still be absent from the grouping list.

The requirements should instead choose and document one policy. A defensible policy is:

- all-aggregate selections may run without `GROUP BY`;
- when aggregate and non-aggregate selections are mixed, every non-aggregate selected expression must be grouped; and
- `HAVING` is explicitly unsupported in v1 if it is not being added.

### B2. Default limiting, paging, and total-count requirements contradict one another

**References:** PRD user stories 35, 38, 39, 57, 58, and 59; Implementation Decision `Paging`; Further Notes.

The PRD simultaneously says:

- results can be paged vertically;
- results are fetched in pages;
- no entered Limit imposes a total cap of one terminal page; and
- the header shows the true unbounded count.

If an omitted Limit caps the entire result to one page, Page Down has no next page. If it means only the initial fetch size, then it is a page size, not a result cap. The requirements must distinguish:

1. **page size** — rows fetched/displayed at once;
2. **user limit** — maximum rows in the logical result; and
3. **total count** — either before or after the user limit.

The count wrapper is also ambiguous. Wrapping a query that already contains the user Limit counts the limited result, while removing the Limit counts the unbounded result. The PRD currently claims both that a user Limit is honored and that the count is unbounded.

Required decision: define the exact count and navigable range for both an empty Limit and an explicit Limit. Also define what the header shows if counting fails or is still running.

### B3. The structured query language is not defined sufficiently to construct SQL

**References:** PRD user stories 26–32; Module Design `QueryBuilder`.

The field names and popup sequence do not define a query grammar:

- `WHERE` has a column and value but no specified operator.
- It is unclear whether multiple predicates, `AND`/`OR`, parentheses, `IS NULL`, `LIKE`, `IN`, or comparison operators are supported.
- `ORDER BY` should choose a column/expression and direction; “the same type-aware validation” as value entry is not meaningful.
- `LIMIT` needs its own integer/range rules, not column-type validation.
- `GROUP BY` behavior is undefined for duplicate columns and columns not selected for display.
- The SELECT column list has no rules for removing, reordering, or preventing duplicate selections.
- It is unclear whether `*` may coexist with named columns or aggregates.

The command-specific state is also absent from the proposed `QueryBuilder` interface. UPDATE needs SET assignments, INSERT needs an ordered set of provided/omitted values, and DELETE does not use a column selection. A generic state of “command, table, columns with aggregates, where, group by, order by, limit” cannot represent all four promised commands.

The PRD must define the v1 grammar and the fields shown for each command before issues can be derived from it.

### B4. Destructive-query safety is incomplete

**References:** PRD user stories 30, 31, 33, and 47.

Confirmation alone does not define safe behavior:

- An optional/empty WHERE appears to permit unqualified UPDATE and DELETE, but this is not called out.
- The confirmation content is unspecified. It should define whether the operation, table, predicate, and potentially affected-row count are shown.
- Confirm/cancel keys and modal cancellation behavior are not defined.
- Database-lock, constraint, trigger, busy-timeout, and disk-write errors are not covered.
- “Rows affected” should state whether it uses SQLite's returned affected-row count and what users should expect when triggers are involved.

A single SQLite write statement is already atomic in autocommit mode, so a new multi-statement transaction requirement is not inherently necessary. What is necessary is an explicit product decision about unqualified writes, the confirmation information, and failure behavior.

INSERT has a separate safety/completeness problem: prompting for every non-AUTOINCREMENT column prevents omission of nullable or defaulted columns and does not address generated columns. The user must be able to distinguish “omit/use default,” `NULL`, and an empty string.

### B5. Result history conflicts with paged fetching and bounded memory

**References:** PRD user stories 44–49 and 57; Implementation Decision `History`; Module Design `History`.

The PRD says results are fetched in pages but result history stores rendered result sets. It does not say what an old result actually contains:

- all rows as an immutable snapshot;
- only pages fetched so far;
- only the current page;
- a query descriptor that is re-executed when revisited; or
- data spooled outside memory.

Each choice has different semantics. Re-execution is not historical if the database changed. Storing all rows contradicts the large-result memory goal. Storing cached pages makes historical paging and export incomplete.

The requirements must define snapshot semantics, paging behavior for historical results, and entry/byte limits with an eviction policy. Query-history limits should be defined as well.

### B6. “Save current query” is not defined for bound parameters

**References:** PRD user stories 51 and 55; Implementation Decisions `Query construction` and `Saving`; Module Design `Export`.

Execution correctly requires bound literal values, but a plain `.sql` export containing placeholders is not self-contained unless its parameter values are also represented. Conversely, interpolating values into exported SQL requires a separate, correctly escaped SQL-literal renderer.

The PRD must specify whether a saved query is:

- executable standalone SQL with safely serialized literals;
- SQL with placeholders plus a documented parameter representation; or
- a non-executable template.

This matters especially for UPDATE/DELETE audit output and the stated goal of keeping useful queries.

### B7. Several required keybindings are not reliably distinguishable in terminals

**References:** PRD user stories 41, 44, 51, and 52; Ideas history and saving decisions.

In traditional terminal input, Ctrl+Shift+P is commonly identical to Ctrl+P, Ctrl+Shift+N to Ctrl+N, and Ctrl+Shift+S to Ctrl+S. Therefore, the proposed query-history and result-history bindings cannot reliably be distinguished. Ctrl+S can also interact with terminal software flow control. Shift+Page Up/Down is intercepted for scrollback by some terminals.

This is not merely a missing implementation detail; the required controls may be impossible in supported environments. The PRD needs a terminal-feasible primary binding set, fallback bindings, and a supported-terminal policy.

The broader input model also needs a context/key matrix. Enter currently means popup selection, add another column, submit a value, run from the Column(s) field, and run generally. Arrow keys are described as field navigation but are also needed for popup movement and text-cursor movement.

### B8. The product definition is inconsistent with the source idea

**References:** Ideas “The main idea is that the user can type in SQL commands”; PRD Solution and Out of Scope.

The source describes a SQL editor in which users type SQL commands. The PRD defines a constrained single-table query builder and explicitly excludes free-text SQL. This may be an intentional decision from the PRD interview, but the source and resulting product definition still disagree.

The resulting v1 cannot express joins, DDL, PRAGMA statements, arbitrary expressions, subqueries, HAVING, or other normal SQL-editor use cases. Either:

- rename/reframe v1 as a SQLite data browser and structured query builder;
- bring free-text SQL into v1; or
- explicitly document that the original “SQL editor” goal has been deferred and why.

There is also a wording contradiction: the Solution calls the application a full-screen interface, while Out of Scope excludes “Full-screen mode.” If the latter means a full-screen text editor, it must say so.

## Major Findings

### M1. SQLite type handling is oversimplified

SQLite uses dynamic typing and column affinity, not a strict numeric-versus-text split. Requirements are missing for:

- INTEGER versus REAL input, including sign, decimals, and exponent notation;
- NUMERIC affinity;
- `NULL` entry and predicates;
- empty string versus omitted/default value;
- BLOB entry, display, and export;
- invalid UTF-8 or binary result values; and
- columns with no declared type.

Over-validating input may reject values SQLite legitimately accepts. The PRD should define a small, explicit affinity policy and state which storage classes are unsupported for v1.

### M2. Schema metadata requirements are insufficient for promised INSERT behavior

The Schema module only exposes name, type, and “whether auto-increment.” Guided inserts also need, at minimum, nullability, default presence/value, primary-key status, and generated/hidden status.

SQLite's `AUTOINCREMENT` keyword is not exposed as a simple column flag by `PRAGMA table_info`; generated columns may require `table_xinfo`; and an `INTEGER PRIMARY KEY` can be automatically assigned even without the explicit `AUTOINCREMENT` keyword. The product behavior should be based on whether a value may be omitted, not merely on whether the DDL contains `AUTOINCREMENT`.

### M3. Identifier handling is missing from the security requirement

Only literal values can be bound as SQL parameters. Table names, column names, sort directions, and aggregate names cannot be bound. The PRD should require:

- table/column identifiers to come from the inspected schema;
- identifiers to be quoted correctly, including names containing spaces, quotes, or SQL keywords; and
- aggregate functions and sort directions to come from fixed enums.

“All user-entered values are bound” is good but does not by itself make all generated SQL injection-safe or correct.

### M4. Paging has no stability or consistency model

LIMIT/OFFSET paging without deterministic ORDER BY has undefined ordering. Concurrent writes between the count and page queries can also make the total and displayed ranges disagree, even with ORDER BY.

The PRD should choose whether to:

- require or automatically provide deterministic ordering where possible;
- permit unstable paging with an explicit warning; and
- accept count/page drift or use a read snapshot.

A universal rowid fallback is not valid because views and `WITHOUT ROWID` tables exist.

### M5. Unbounded counting can defeat the responsiveness goal

Fetching one page does not guarantee a prompt response if Sqloid first executes an expensive full-result count. There are no requirements for asynchronous execution, progress, cancellation, timeout, or behavior when only the count fails.

The PRD should define whether rows may appear before the total, whether “count unavailable” is acceptable, and how the user cancels a long query. Ctrl+C is currently reserved for quitting rather than cancellation.

### M6. Export scope and serialization are incomplete

“Save the current result” does not specify whether the export contains:

- the visible page;
- all fetched pages;
- the entire logical result up to the user Limit; or
- the unbounded query result.

It also does not define behavior for a historical result, an error result, or a write summary. Full-result export must remain streaming/bounded if large-result memory safety is a goal.

Format decisions are missing for:

- CSV dialect, headers, quoting, embedded newlines, and UTF-8 encoding;
- the intentional ambiguity between NULL and empty string in CSV;
- JSON shape (array of objects versus arrays);
- duplicate result-column names in JSON objects;
- numeric precision and BLOB encoding; and
- aggregate/result column labels.

Rendering NULL and empty string identically in the grid and CSV is lossy. If retained, the PRD should state that this is an accepted limitation rather than calling the representations unambiguously sensible.

### M7. Command changes and builder lifecycle are undefined

When the user changes SELECT to UPDATE, DELETE, or INSERT, the PRD does not say which prior fields are retained, reset, or transformed. It also omits:

- how a selected column is removed or reordered;
- how a popup is completed or cancelled;
- whether command keys are case-sensitive;
- how the user clears an optional field;
- whether Enter can run from every field or only specific contexts; and
- whether returning from query history creates a branch or mutates the selected entry.

These are observable product behaviors, not merely UI implementation choices.

### M8. Database and schema object scope is incomplete

“All tables” is ambiguous. Requirements should address:

- views;
- virtual tables;
- SQLite internal objects such as `sqlite_sequence`;
- temporary and attached schemas;
- tables created or dropped by another process during the session; and
- schema caching/refresh behavior.

If v1 only supports ordinary tables from the main schema and loads schema once at startup, that should be explicit.

### M9. Connection and local-D1 error behavior is incomplete

The PRD should define:

- whether the D1 path is resolved relative to the process working directory;
- behavior when the directory is absent or unreadable;
- filename matching and case sensitivity;
- read/write permission failures;
- what qualifies as a valid SQLite file and whether an empty file is accepted;
- behavior for locked/busy databases and whether a busy timeout is used;
- behavior when wrangler is concurrently using the database; and
- exit status and stderr expectations for startup failures.

The exact multiple-candidate message is specified, but other errors lack equivalent acceptance criteria. “Sqlite” should also be normalized to “SQLite.”

### M10. Mid-session file monitoring lacks observable semantics

The PRD requires the application to end if the file is deleted but does not specify:

- detection timing;
- whether rename or replacement counts as deletion;
- whether a final error screen is shown before exit;
- whether the user can save the current query/result after detection; or
- how normal connection errors differ from monitored deletion.

A platform-specific monitoring mechanism need not be mandated by a PRD, but the externally observable behavior and latency do need to be defined.

### M11. Error handling and diagnostic logging are not designed

Only query-error placement is specified. Missing cases include connection loss, locked database, constraint failure, schema changes, count failure, partial page failure, and export/path errors.

The PRD should define:

- whether a failed new operation preserves the last successful result in history;
- whether count failure can coexist with successful rows;
- how errors are dismissed or retried;
- which errors end the session;
- what startup failures print to stderr; and
- whether a debug mode or diagnostic log exists.

Logging during a full-screen TUI must not corrupt the display, and query values may be sensitive. If diagnostics include SQL or parameters, redaction behavior should be explicit.

### M12. History equality and mutation semantics are vague

“Only genuine changes” needs a comparison rule and an append event. The PRD should say whether history is appended when a query runs or whenever fields are edited, and whether equality is based on exact normalized field state.

Column order, aggregate choice, typed value type, sort direction, and Limit should all have explicit significance. Merely requiring state to differ from “a previous state” is ambiguous: the immediately previous executed state, the currently selected history item, or any historical state could be intended.

### M13. Core UI behavior is declared untested despite dominating the requirements

Most user stories concern focus, key dispatch, popup transitions, modals, resize behavior, and history navigation in the Bubble Tea model. Marking the entire UI “untested by design” leaves the majority of acceptance behavior uncovered.

The PRD need not prescribe implementation-detail tests, but it should require behavioral tests over model updates/key sequences for critical flows, plus CLI/database integration tests. Manual-only verification should be identified explicitly if retained.

### M14. Product acceptance criteria and operating envelope are absent

There are no measurable success criteria or supported-environment requirements. At minimum, define:

- supported operating systems and terminal capabilities;
- minimum terminal dimensions or small-terminal behavior;
- supported SQLite feature/version assumptions;
- representative database/result sizes;
- responsiveness or cancellation expectations; and
- a definition of done tied to user-story acceptance scenarios.

“Windows-specific terminal concerns” being out of scope does not establish whether Linux only or Linux and macOS are supported.

## Minor Findings

### m1. Field order differs between the source and PRD

`Ideas.md` lists Column(s) before Table, while its own interaction flow and the PRD correctly require Table before Column(s). Normalize the source or explicitly treat its list as non-positional.

### m2. Table and column fuzzy search lacks acceptance behavior

Case sensitivity, matching behavior, empty-query behavior, no-match display, and selection preservation are unspecified. The exact algorithm can remain an implementation choice, but observable behavior should be stable.

### m3. Result-grid rendering needs edge-case rules

Define cell width/truncation, embedded tabs/newlines, very long values, Unicode width, duplicate column names, BLOB display, empty columns, and behavior when the terminal is too narrow or short.

### m4. Resize semantics do not preserve a logical location

Recomputing the page size can change page boundaries. State whether the first currently visible row, selected row, or page number is preserved when possible.

### m5. File-picker behavior is underspecified

The requirements do not define starting directory, hidden directories, parent navigation, directory creation, cancellation, invalid filenames, automatic extensions, or write-permission errors.

### m6. Quit behavior and modal conventions need a common rule

Define confirm, cancel, and Escape behavior consistently across quit, overwrite, and destructive-operation modals. Also define Ctrl+C behavior while a modal is already open or while a query is executing.

### m7. CLI basics are omitted

Help, version output, unexpected arguments, directory paths passed to `sqlite`, and consistent nonzero exit codes are not covered.

### m8. The “single user, single developer” phrase is unclear

If this means no authentication, collaboration, or multi-user server behavior, state that directly. It should not imply that only one human may use the distributed tool.

## Traceability to `Ideas.md`

The PRD preserves most interview decisions, including read-write access, local-D1 discovery, save formats, confirmation for UPDATE/DELETE, paged results, NULL representation, history repopulation, and quit behavior.

The material traceability gaps are:

1. The original “type in SQL commands”/SQL editor vision was narrowed to a structured builder without an explicit rationale.
2. The source and PRD field orders differ.
3. “Full screen” is used both as the main UI form and as an excluded feature.
4. Some interview decisions were copied into the PRD without technical correction, notably the aggregate/GROUP BY rule and terminal keybindings.

## Required Decisions Before Issue Generation

The PRD should be revised to resolve, at minimum, the following questions:

1. Is v1 a SQL editor or a constrained SQLite query builder/data browser?
2. What exact SELECT, WHERE, GROUP BY, ORDER BY, UPDATE, DELETE, and INSERT grammar is supported?
3. Are unqualified UPDATE and DELETE allowed, and what must their confirmation show?
4. How are omitted/default, NULL, and empty-string values entered and distinguished?
5. Does an empty Limit mean one page total or an unlimited logical result fetched one page at a time?
6. Does total count apply before or after a user Limit, and may it be unavailable or delayed?
7. What ordering/consistency guarantee applies across pages?
8. What constitutes a historical result, and how is history bounded?
9. What rows are exported from current and historical results?
10. How are bound parameters represented in saved `.sql` files?
11. What terminal-feasible keybindings distinguish query history, result history, query save, and result export?
12. Which schema objects and SQLite affinities/storage classes are supported?
13. What are the startup, query, count, lock, connection-loss, and export error behaviors?
14. What execution cancellation or responsiveness behavior is required?
15. Which UI interactions receive automated behavioral coverage?
16. What operating systems, terminal capabilities, and data sizes define the supported v1 envelope?

## Recommendation

Revise the PRD before generating issues. Resolve all blockers first, then convert the major findings into explicit requirements, deliberate exclusions, or acceptance rules. After revision, repeat the requirements critique; only then should the PRD be treated as a reliable source for issue generation.
