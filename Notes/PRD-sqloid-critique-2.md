# Second Critique: PRD: Sqloid v1

## Review Scope

This is a requirements-only review of:

- `Notes/PRD-sqloid.md`
- `Notes/Ideas.md`
- `Notes/PRD-sqloid-critique.md`, used only to verify whether the earlier findings were actually resolved

It evaluates completeness, internal consistency, fidelity to the source ideas, SQL/SQLite correctness, edge cases, error handling, logging, testability, and readiness for issue generation. It does not generate implementation issues or tasks.

Repository context was also searched for the context/key matrix referenced by the PRD. No such design document was found.

## Overall Verdict

**Not ready for issue generation.**

This revision is substantially stronger than the version assessed in the first critique. It resolves most of the original blockers and major findings: v1 is now clearly a structured SQLite query builder rather than a general SQL editor; the grammar, paging/count semantics, bounded history, export formats, value binding, schema scope, error presentation, cancellation, and supported environment are all much more explicit.

However, several core behaviors remain contradictory or undefined. In particular, the builder cannot consistently represent the stated `COUNT(*)` behavior; the asynchronous operation state machine is unspecified; unbounded navigation conflicts with the 10,000-row immutable-history model; the claim that failed writes leave the database untouched is not always true under SQLite semantics; and essential key behavior is deferred to a design document that does not exist. The `Open Questions` claim at lines 195–197 is therefore still not supportable.

## What Is Strong

- The intentional narrowing from a general SQL editor to a structured v1 query builder is explicit and traceable to `Ideas.md`.
- The supported SQL subset is much better bounded than before.
- The distinction between page size, user Limit, and logical result count is now clear.
- Count and first-page concurrency, count failure, cancellation, and the lack of query timeouts are acknowledged.
- Parameter binding and identifier allowlisting/quoting are correctly separated.
- Result-history snapshots, caps, eviction, and export formats are described in useful detail.
- Destructive writes receive confirmation, an all-rows warning, and an affected-row estimate.
- SQLite object scope, D1 discovery, startup errors, busy handling, and mid-session deletion behavior are largely explicit.
- Logging is deliberately excluded, with a sound TUI and sensitive-data rationale.
- Critical UI behavior is no longer entirely relegated to manual testing.
- No material requirement from `Ideas.md` has been silently dropped; the original multiple-database and free-text-SQL goals are clearly deferred beyond v1.

## Severity Definitions

- **Blocker**: The requirement is contradictory, technically invalid, or too incomplete to implement without inventing consequential product behavior.
- **Major**: Implementation could proceed, but different reasonable interpretations would produce materially different behavior or leave an important edge case unprotected.
- **Minor**: The intended behavior is mostly clear but needs a precise acceptance rule or consistency correction.

## Blockers

### B1. The column model cannot consistently represent `COUNT(*)`

**References:** lines 20, 56–58, and 116.

The grammar says each SELECT entry is either the lone `*` or a named column with an optional aggregate. That model does not permit an aggregate applied to `*`. The UI nevertheless opens the aggregate popup after every column choice, and the aggregate decision uses `COUNT(*)` as a supported example. It also says selecting `*` alongside any aggregate is blocked, which can be read as blocking `COUNT(*)` itself.

The requirements also do not say whether `MIN(*)`, `MAX(*)`, `AVG(*)`, and `SUM(*)` are offered even though those forms are invalid, or whether `COUNT(*)` may coexist with other aggregate entries as the example implies.

Define two distinct concepts:

1. wildcard projection, `SELECT *`, which must be the only projection entry; and
2. aggregate `COUNT(*)`, which is an aggregate entry and may coexist with other aggregate entries.

Specify which aggregate choices are offered for `*`, whether the popup is shown for wildcard projection, and how each form is represented in builder state and history.

### B2. The asynchronous execution state machine is undefined

**References:** lines 47–48, 99–101, 117, 130, and 132.

The first page and total count run concurrently, and the UI remains interactive, but the permitted actions and operation ownership are not defined. Consequential cases include:

- whether the builder can be edited while a query is running;
- whether Enter can start another query while page or count work is still in flight;
- whether a new execution cancels, supersedes, or runs alongside the old one;
- how late page/count responses are prevented from replacing a newer result;
- whether Ctrl+W cancels the page fetch and count as one logical execution or only whichever request remains active;
- what Ctrl+W does after rows have arrived but the count is still running; and
- whether `Cancelled` is added to result history, replaces the partial result, or leaves the newly fetched rows visible.

“Previous results intact” does not answer these cases because the concurrently arriving first page may already have become the current result. Define an explicit observable execution lifecycle, including allowed keys, supersession, cancellation scope, late-response handling, and result/history transitions.

### B3. Unbounded paging conflicts with immutable snapshots capped at 10,000 rows

**References:** lines 74–79, 98–100, 118, 120, and 191.

An unlimited logical result is said to be fully navigable, while a result history snapshot is immutable, never re-fetched, and capped at 10,000 rows. Export contains the rows held in that snapshot. The PRD does not define what happens when the user navigates beyond row 10,000:

- whether newly fetched rows replace older cached rows or are simply not retained;
- whether Page Up can revisit discarded pages by re-fetching them;
- which 10,000 rows become the historical snapshot when more than 10,000 have been viewed;
- how a historical result can show “exactly what was captured” if the visible page was outside the retained range; or
- whether export contains the first 10,000 distinct rows, the most recent 10,000, or another set.

The statement that users can export more by paging through the result is also misleading once the cap is reached, and setting a Limit does not itself fetch all limited pages.

Define active-result paging/cache behavior separately from finalized history-snapshot behavior. Specify the retained row ranges, re-fetch rules, Page Up behavior, snapshot finalization point, truncation indicator, and exact exported subset.

### B4. The guarantee that a failed write leaves the database untouched is false without stronger transaction requirements

**References:** lines 47, 125, 130, 132, and 183.

The PRD claims that a failed write leaves the database untouched because a single autocommit statement is atomic. SQLite's `FAIL` conflict algorithm, including `ON CONFLICT FAIL` and `RAISE(FAIL)` in triggers, can abort a statement without backing out changes made to earlier rows by that same statement. Cancellation/interruption behavior for an in-flight write is also not covered by the untouched guarantee.

This is a safety requirement, not merely an implementation detail. Either require Sqloid to execute each generated write inside an application-controlled transaction that is rolled back on every statement error or cancellation, or narrow the guarantee and explicitly document the SQLite cases in which partial effects can remain. “Transactions spanning multiple statements” can remain out of scope while still allowing an internal transaction around one user-visible write operation.

### B5. Essential input behavior is delegated to a key matrix that does not exist

**References:** lines 21, 57–73, 91–95, and 127–128.

The PRD says a context/key matrix in the design docs defines every key's behavior, but no such document exists in the repository. The prose does not independently define several required interactions:

- which key removes only the most recently added SELECT column;
- how a multi-select column, GROUP BY, or UPDATE SET popup is finished while preserving selections;
- whether Backspace/Delete on Column(s) removes one entry or clears the whole field;
- how ORDER BY direction is toggled;
- how users move back to revise an earlier UPDATE value or INSERT column choice;
- what Enter, arrows, Tab, Shift+Tab, Backspace/Delete, Esc, and printable input do in every popup and text-entry phase; and
- how focus is restored after each cancel/complete action.

The generic statement that Backspace/Delete clears an “assisted field” may directly conflict with remove-last-added behavior. Add the complete context/key matrix to the PRD or create and explicitly reference the actual design document before deriving issues.

### B6. Destructive-write confirmation is incomplete around its pre-flight count

**References:** lines 47, 65–71, 113, 125, and 132.

The confirmation is described as showing a “live affected-row count,” but the requirements do not say:

- whether the modal opens before or after counting completes;
- whether confirmation is disabled while the count is running;
- what happens when the count is locked, fails, or is cancelled;
- whether Ctrl+W cancels this count and where focus returns; or
- whether the user may proceed when the estimate is unavailable.

There is also an unavoidable time-of-check/time-of-use race if another process changes matching rows between the count and confirmed write. The PRD accepts count/page drift for reads but does not acknowledge the equivalent, more safety-sensitive write-confirmation race. Calling the count “live” and “affected-row count” overstates what a separate pre-flight query guarantees.

Specify the full count/modal state flow and either label the value as a non-binding pre-flight estimate with an accepted concurrency limitation, or define a consistency mechanism and its locking/timeout behavior. The post-write summary must continue to show the actual driver-reported affected-row count.

## Major Findings

### M1. Switching from SELECT can retain a table object invalid for write commands

**References:** lines 53, 55, 124, and 127.

Views are valid SELECT targets but are excluded from UPDATE/DELETE/INSERT popups. Changing the command nevertheless always keeps the selected table. A user can therefore select a view under SELECT and switch to a write command while retaining an object that the new command would never permit them to choose.

Require command switching to revalidate the retained object against the new command. Clear the Table field and all downstream state when it is not eligible; state whether focus moves to Table and whether changing between write commands retains an eligible ordinary/virtual table.

### M2. The implicit `ORDER BY rowid` rule does not reliably provide stable paging

**References:** lines 19–24, 116–117, and 203.

The rule is valid only for a simple row-level query where an unshadowed hidden rowid is available. It is incomplete because:

- a declared column named `rowid` can shadow the hidden rowid;
- aggregate/GROUP BY result rows do not map one-to-one to table rowids;
- ordering grouped results by a source rowid can be meaningless or ambiguous; and
- a user-selected ORDER BY column need not be unique, so ties can still move between LIMIT/OFFSET pages.

Define when an implicit unique row identifier may be used, how shadowed rowid aliases are handled, and what ordering policy applies to aggregate/grouped results. If stable order cannot be guaranteed, say so rather than claiming stability. If explicit non-unique ordering is accepted, document tie instability as well as concurrent-write drift.

### M3. Projection duplicate and aggregate identity rules are ambiguous

**References:** lines 20–23, 58, 63, 116, and 118.

“Duplicates are prevented” does not say whether identity is based on source column alone or on `(column, aggregate)`. These interpretations materially differ: one forbids selecting both `Value(age)` and `AVG(age)`, while the other permits it. The history model treats column plus aggregate as an entry, which suggests pair identity, but the grammar says re-selecting an existing column adds nothing.

The aggregate safety rule also considers selected non-aggregate columns but does not define which ORDER BY columns are offered or allowed in grouped queries. Ordering by an ungrouped, non-aggregate source column can reintroduce the ambiguity that the GROUP BY rule is intended to prevent.

Define projection-entry identity and the exact ORDER BY candidate/validation rules for aggregate and grouped queries.

### M4. INSERT does not cover the all-omitted or no-insertable-column cases

**References:** lines 28, 67, 72, 123–126, and 143.

The grammar always emits a parenthesized column list and VALUES list, but every insertable column can be omitted. SQLite then requires `INSERT INTO table DEFAULT VALUES`. The PRD does not specify this form or what happens if schema inspection yields no insertable columns.

Virtual tables are offered to write commands while all hidden columns are excluded from INSERT prompts; for some virtual-table modules, hidden columns are meaningful inputs. Decide whether such virtual-table inserts are unsupported, best-effort, or represented through an explicit exception. At minimum, define the all-omitted generated statement, runnable state, result summary, and error behavior.

### M5. Numeric parsing is not precise enough to produce consistent bindings and history

**References:** lines 68, 118, and 122.

“Valid integer” and “valid decimal/exponent number” leave observable cases undefined: leading/trailing whitespace, leading `+`, leading zeros, `1.`, `.5`, hexadecimal-looking text, integer overflow beyond signed 64-bit, exponent overflow, and `NaN`/`Inf` spellings. Different parsers would bind the same entered text as INTEGER, REAL, or TEXT, changing query behavior, exported SQL, and history equality.

Define the accepted lexical grammar and numeric ranges, whether whitespace is preserved, and whether overflow is an inline error or falls back to TEXT. Also define CSV/JSON behavior for non-finite REAL values; standard JSON cannot encode Infinity or NaN as numbers.

### M6. Header-only startup validation does not establish the stated database/read-write guarantees

**References:** lines 34–43, 105, 112–114, and 190.

A file with the 16-byte SQLite signature can still be corrupt or not openable as a database. Header checking also does not establish that the connection opened in read-write/no-create mode or that schema access succeeds. This is weaker than the user-facing promise that invalid databases fail at startup and that write commands persist.

Define whether startup performs a harmless database/schema operation after checking the signature, whether open mode must be read-write without creating a file, and whether inability to establish that mode is a startup failure. Clarify what “busy timeout at open” means by identifying the startup operation to which it applies.

### M7. Schema metadata is insufficient for the paging and command-eligibility decisions

**References:** lines 55, 117, 124, and 141–144.

The Schema interface returns tables and columns with type/insertability, but other requirements need to distinguish views, ordinary rowid tables, virtual tables, and `WITHOUT ROWID` tables. They also need command-specific eligibility and rowid capability. Without those properties, the stated module boundary cannot support the product behavior without leaking fresh schema SQL into other modules.

Require schema object metadata sufficient to make those decisions. Also define the user-visible behavior when a table/column refresh fails because of a lock, file deletion, corruption, or an external schema change, and whether stale popup data is retained or discarded.

### M8. Error, cancellation, and partial-page history semantics remain ambiguous

**References:** lines 47, 83–89, 118, and 130.

The PRD says every query error is appended to result history, while a page-fetch failure “appends what was fetched so far.” It is unclear whether a mid-scan failure creates an error entry plus a separate partial snapshot, converts the current result into an error while retaining partial rows, or appends rows to an existing snapshot after that snapshot is supposed to be immutable. Cancellation is displayed as `Cancelled` but is not classified as an error or explicitly included/excluded from result history.

Define one history entry lifecycle for first-page failure, count-only failure, later-page failure, cancellation before rows, and cancellation after rows. Specify what Ctrl+E/Ctrl+Y reaches in each case and what remains exportable.

### M9. “Save current query” has unresolved state and target semantics

**References:** lines 90, 114, 119–120, and 127–130.

The PRD does not say whether Ctrl+S saves:

- the current builder state even if it is incomplete or invalid;
- the last successfully executed query;
- a selected query-history entry after it repopulates the builder; or
- the query associated with a selected historical result.

This matters particularly in the terminal file-deletion state, where saving remains available but normal builder navigation does not. Define when Ctrl+S is enabled, which state it serializes, and the message shown when no standalone runnable query is available.

### M10. Some required terminal behavior is internally inconsistent or not portable enough

**References:** lines 45–47, 75, 109, 114, 128–129, 132, and 188.

Shift+Page Up/Down is commonly consumed by terminal scrollback rather than delivered to a TUI, yet no fallback binding is supplied. The claim that all required bindings are terminal-distinguishable does not address interception by the supported xterm-compatible environment.

Ctrl+C is also inconsistent: it always opens quit confirmation, Ctrl+C inside quit confirmation confirms quit, and the file-deletion terminal state says Ctrl+C quits with status 1. State whether that terminal state uses the normal two-step confirmation or exits immediately. Provide reachable fallback horizontal-scroll bindings or narrow the supported terminal requirements accordingly.

### M11. Duplicate output-name handling is incomplete

**References:** lines 106, 121, and 163.

Suffixing a duplicate as `name_2` can collide with an original result column already named `name_2`. The PRD also clearly applies suffixes to the grid header and JSON keys but does not state whether CSV headers use raw or deduplicated names.

Define a collision-free, deterministic naming rule over the complete output-name set and state its scope across the grid, CSV, and JSON. The transformation should not alter the generated SQL or driver metadata.

### M12. Save-overwrite failure behavior can still clobber an existing file

**References:** lines 95, 108, 119, and 161–164.

Overwrite confirmation prevents accidental consent, but a direct truncate-and-write can destroy the old file if serialization or I/O fails after confirmation. “Write-permission failures surface inline” does not define whether the previous file remains intact.

Specify whether saves must be atomic from the user's perspective and what happens to the pre-existing destination on failure. Also define “invalid filename” as at least a non-empty basename rather than a path, since directory selection is a separate UI step.

### M13. The Connection interface conflicts with independent page/count completion

**References:** lines 99–101, 117, 132, and 136–139.

“Paged query execution returning rows plus total count” suggests one combined completion, but the UI must receive the first page before the independently running count, retain rows if count fails, and cancel either the logical operation or its remaining component. The module contract does not represent these required outcomes.

Revise the requirements-level interface description so page data and count status can complete independently and carry operation identity/cancellation semantics. This is necessary to keep the asynchronous behavior from leaking into ad hoc UI conventions.

### M14. Testing decisions omit the highest-risk state and safety cases

**References:** lines 166–171 and 186–193.

The required tests cover basic flows but not the newly complex boundaries. The PRD should require external-behavior coverage for at least:

- overlapping/superseded asynchronous responses and cancellation at each execution phase;
- first-page success with count failure;
- paging and history behavior at and beyond 10,000 rows;
- write rollback on constraint/trigger failure and cancellation;
- pre-flight count failure/cancellation and unqualified-write confirmation;
- command switching from a view to a write command;
- `COUNT(*)` versus wildcard projection;
- numeric parser boundary cases and non-finite export behavior;
- rowid-shadowing and grouped-query paging; and
- schema refresh/drop/change during a built query.

These are observable product behaviors, not implementation details.

### M15. The responsiveness criterion is not reproducible

**References:** lines 191–193.

“First page within ~100ms for indexed queries” lacks a fixture size, query shape, cold/warm cache state, storage class, hardware envelope, and measurement point. It is labeled a target rather than a guarantee but is placed under acceptance criteria and definition of done, making pass/fail unclear.

Either define a reproducible benchmark scenario and tolerance or explicitly classify this as non-blocking product guidance excluded from the definition of done.

## Minor Findings

### m1. `WHERE (all commands)` contradicts the INSERT field set

**References:** lines 22 and 28.

INSERT explicitly has no WHERE field, so change “WHERE (all commands)” to “WHERE (SELECT/UPDATE/DELETE).”

### m2. Popup search scope is not explicit

**References:** lines 55–64 and 127.

Only the table popup has defined fuzzy-search behavior. State whether column, GROUP BY, ORDER BY, aggregate, and operator popups are searchable or scroll-only. Aggregate/operator lists can reasonably be scroll-only, but that should be deliberate.

### m3. LIKE wildcard behavior is unstated

**References:** lines 22 and 122.

State whether `%` and `_` are passed through as SQLite LIKE wildcards, how a literal wildcard is escaped if supported, and whether LIKE follows the connection's default SQLite collation/case behavior. No custom Unicode case-insensitivity should be implied by the unrelated case-insensitive popup search requirement.

### m4. Limit bounds need a complete parser rule

**References:** lines 25 and 64.

“Positive integers only” clearly excludes zero, but the maximum accepted value and overflow behavior are not stated. Define the accepted integer range and inline error behavior. If zero-row queries are useful, reconsider whether zero should be allowed; otherwise explicitly retain the positive-only choice.

### m5. D1 filename matching needs a case rule

**References:** lines 39–41 and 111.

Specify whether the `.sqlite` extension and `metadata` exclusion are case-sensitive. Linux and macOS filesystem behavior differs, and the candidate set should not depend on an unstated matching convention.

### m6. Terminal-error-state export selection is unclear

**References:** line 114.

When file deletion ends the session, state which in-memory query/result is initially selected for Ctrl+S/Ctrl+X and whether history navigation remains available. Otherwise users may be told export is available without a defined way to choose the snapshot they need.

### m7. User story numbering is awkward for downstream traceability

**References:** lines 44–48.

Identifiers `13a` and `13b` work for prose but are easy to mishandle in issue/test references. Renumber the user stories once the requirements stabilize or assign durable requirement IDs.

## Traceability to `Ideas.md`

Traceability is now strong. The PRD preserves or explicitly narrows the source decisions concerning:

- SQLite/local-D1 scope;
- read-write access and pure-Go SQLite;
- D1 discovery and validation;
- structured command/table/column flow;
- assisted WHERE/GROUP BY/ORDER BY/Limit behavior;
- UPDATE/DELETE confirmation and INSERT omission/NULL choices;
- paged results, concurrent count, cancellation, and history;
- query/result export and format details;
- identifier safety and value binding;
- schema scope and refresh behavior; and
- the deliberate deferral of free-text SQL and additional database engines.

The PRD contains additional rendering, file-picker, terminal-size, and test details that were not in the initial idea list. That is appropriate elaboration rather than a fidelity problem. The remaining defects are primarily internal completeness and correctness problems, not source-requirement omissions.

## Required Decisions Before Issue Generation

At minimum, revise the PRD to decide and document:

1. How wildcard projection and `COUNT(*)` differ in builder state and UI.
2. The complete asynchronous execution, supersession, cancellation, and late-response lifecycle.
3. Active paging/cache behavior and final history contents beyond 10,000 fetched rows.
4. The rollback guarantee for every failed or cancelled write.
5. The actual context/key matrix for every builder, popup, modal, and text-entry state.
6. The complete pre-flight count/confirmation flow and its concurrency limitation.
7. Command-switch revalidation when a SELECT view is retained for a write command.
8. Stable-order behavior for rowid shadowing, grouped results, and non-unique explicit ordering.
9. Projection identity, aggregate validation, and grouped ORDER BY rules.
10. INSERT behavior when every column is omitted and for problematic virtual-table schemas.
11. Exact numeric parsing/range and non-finite export rules.
12. Startup database/read-write validation beyond a signature check.
13. Schema metadata and refresh-error behavior needed by other requirements.
14. History behavior for partial failures and cancellations.
15. Which query Ctrl+S saves in every reachable state.
16. Reachable horizontal-scroll keys and consistent Ctrl+C behavior.
17. Collision-free output names and safe overwrite failure behavior.
18. Required tests for the high-risk state, paging, and write-safety boundaries.

## Recommendation

Revise the PRD again before generating issues. Resolve all blockers first, then either turn each major finding into an explicit requirement, document it as an accepted limitation, or remove the conflicting promise. After revision, perform one focused requirements pass over the execution state machine, write transaction safety, paging/history lifecycle, and complete key matrix; those four areas now carry most of the remaining implementation risk.
