# Third Critique: PRD: Sqloid v1

## Review Scope

This is a requirements-only review of:

- `Notes/PRD-sqloid.md`
- `Notes/Ideas.md`
- `Notes/PRD-sqloid-critique-2.md`, used only to check whether the previous findings were fully resolved

It evaluates completeness, internal consistency, fidelity to the source ideas, SQL/SQLite correctness, state-machine behavior, edge cases, error handling, logging, testability, and readiness for issue generation. It does not generate implementation issues or tasks.

## Overall Verdict

**Not yet ready for issue generation.**

The PRD is now unusually detailed and substantially stronger than the revision covered by the second critique. It resolves most of that critique's explicit findings, including the wildcard/`COUNT(*)` data model, the high-level execution lifecycle, write transactions, startup validation, schema metadata, output-name deduplication, atomic saves, numeric parsing, and the inclusion of a context/key section.

The remaining problems are narrower, but several still affect core behavior. The destructive-write estimate is not integrated coherently into the execution/history lifecycle; later page requests have no defined concurrency or stale-response policy; reverse traversal of the 10,000-row cache is incomplete; grouped non-aggregate projections can still produce the ambiguity the PRD claims to prevent; and the key rules contain a direct Ctrl+C contradiction while omitting precedence for several global actions. These require product decisions rather than implementation guesses.

## What Is Strong

- The v1 scope is explicit: SQLite/local D1 and a structured builder, with additional engines and free-text SQL deliberately deferred.
- The supported SQL grammar is tightly bounded, including operators, aggregates, value coercion, identifier quoting, and unsupported constructs.
- Startup validation, D1 discovery, busy handling, schema refresh, and terminal file-deletion behavior are described in substantially testable terms.
- The PRD correctly distinguishes wildcard projection from aggregate `COUNT(*)` in its state model.
- Write safety correctly requires an application-controlled transaction rather than assuming a failed autocommit statement is always atomic.
- Count failure, first-page failure, partial-page failure, and cancellation have explicit intended result-history outcomes.
- Export formats, duplicate-name handling, BLOB/NULL/non-finite values, atomic replacement, and snapshot size limits are much clearer than in earlier revisions.
- The module boundaries are requirements-oriented and mostly support the stated behavior.
- Logging is deliberately excluded with an appropriate TUI and sensitive-data rationale, rather than simply forgotten.
- High-risk behavioral tests are required instead of leaving all TUI behavior to manual verification.
- `Ideas.md` now explicitly says the PRD is authoritative for decisions made during interviews and critiques. The deliberate result-history key change from Ctrl+Shift+P/N to Ctrl+E/Y is therefore a traceable refinement, not a fidelity defect.

## Severity Definitions

- **Blocker**: The requirement is contradictory, technically invalid, or too incomplete to implement without inventing consequential product behavior.
- **Major**: Implementation could proceed, but different reasonable interpretations would produce materially different behavior or leave an important edge case unprotected.
- **Minor**: The intended behavior is mostly clear but needs a precise acceptance rule or consistency correction.

## Blockers

### B1. The destructive-write estimate does not fit the execution and history lifecycle

**References:** lines 39–58, 88, 139–142, 202–203, 210, 216, and 234–236.

Enter on a runnable UPDATE or DELETE opens a modal and starts an estimate. Confirmation later starts the write. The general lifecycle, however, says Enter starts one logical execution, that a count or write owns one request, and that every ended execution creates exactly one result-history entry.

The PRD does not establish whether estimate and write are:

1. two logical executions;
2. two phases of one logical execution; or
3. a pre-execution UI workflow followed by one write execution.

Each interpretation currently contradicts another requirement. If the estimate is an execution, cancelling it should apparently create a `Cancelled` history entry. If estimate and write are one execution, the lifecycle must include a waiting-for-confirmation phase with no request in flight, and cancellation of the modal must be distinguished from cancellation of an executed write. If the estimate is pre-execution, the blanket statement that all counts follow the lifecycle is false.

This also leaves query-history timing undefined: opening the estimate modal should not necessarily record a write that the user never confirms, but “append at execution time” does not identify whether estimation counts as execution.

Define one write workflow with explicit phases, operation identity, allowed keys, and history effects, for example:

`builder → estimating → awaiting confirmation → writing → committed/failed/cancelled`.

State separately what happens when the user cancels during estimation, dismisses after estimation, cancels during the statement, or quits at each phase. Specify exactly when query history and result history receive entries. A dismissed pre-flight modal should not accidentally look like an attempted database write.

### B2. Later page requests have no concurrency or stale-response policy

**References:** lines 41–44, 58, 147–153, 194, 210, 216, and 248–251.

The operation ID protects against responses arriving after an execution is completed, cancelled, or superseded. It does not resolve conflicting responses within the same active SELECT execution.

Once the first page is visible, the requirements do not say what happens if the user:

- presses Page Down repeatedly while a page fetch is pending;
- presses Page Up before a Page Down response arrives;
- resizes while a page request using the old page size is pending;
- navigates to history or starts editing while a later page is pending; or
- presses Ctrl+W while only a later page request, rather than the first page or count, is running.

All of these responses can carry the same logical operation ID while targeting different offsets. Applying them in arrival order can display the wrong page or corrupt cache order. The phrase “up to two concurrent requests (first page + count)” also describes only startup and does not state whether count plus a later page, or multiple later pages, may overlap.

Choose and specify a policy: serialize page fetches and ignore/coalesce navigation while one is pending, or attach page-request identity and a requested viewport generation in addition to the execution ID. Define loading feedback, key behavior, cancellation, resize behavior, and which response is allowed to update the current viewport and cache.

### B3. Reverse traversal of the 10,000-row cache remains incomplete

**References:** lines 58, 153, 173–177, 194–197, 235–236, and 251.

The PRD defines forward eviction: Page Down beyond the retained window evicts the oldest pages. It says Page Up re-fetches evicted pages, but does not say what is evicted when those older pages are reinserted into a full cache.

Consequential questions remain:

- Is the retained window always one contiguous logical range?
- Does reverse re-fetch evict the newest pages, the least recently used pages, or something else?
- Can alternating Page Up/Page Down produce non-contiguous retained ranges?
- When a re-fetched page overlaps retained rows, are entries replaced or duplicated?
- In what logical order are non-contiguous retained rows exported?
- Which rows become the immutable snapshot after the user has traversed forward, backward, and forward again?
- If count failed, how is a snapshot marked incomplete when additional unseen rows may exist?

“Exactly the retained snapshot rows” is not enough until retention and ordering are deterministic. Define the active cache as a precise data structure with invariant(s), reverse-eviction behavior, overlap handling, snapshot ordering, and completeness/truncation metadata. Require tests that traverse both directions across the cap, not only forward past it.

### B4. GROUP BY validation still permits ambiguous bare values

**References:** lines 19–33, 131–134, 192, 225–226, and 294.

The PRD blocks a query only when its projection mixes aggregate and non-aggregate entries and a non-aggregate entry is absent from GROUP BY. That does not protect grouped queries containing no aggregate.

For example, these appear runnable under the stated rule:

```sql
SELECT "age" FROM "people" GROUP BY "city";
SELECT * FROM "people" GROUP BY "city";
```

SQLite accepts such queries but chooses arbitrary bare values within each group. That is the exact behavior the Further Notes say the builder prevents.

Apply the rule to every query with GROUP BY, not only mixed aggregate queries: every non-aggregate projected column must be grouped. Decide explicitly whether wildcard projection with GROUP BY is prohibited or is allowed only when every expanded table column is grouped. Update the runnable-state contract and boundary tests accordingly.

### B5. The context/key rules are still internally contradictory and incomplete

**References:** lines 60–96, 111–116, 147–168, 202, 205–208, and 280–281.

The modal rules say Ctrl+C confirms quit in the quit modal and **cancels any other modal back to the builder**. The Quitting rule says Ctrl+C anywhere else, explicitly including “in another modal,” **shows the quit confirmation modal**. Both cannot be implemented.

The section also claims to define every key per focus context, but it does not assign precedence or availability for Page Up/Down, horizontal scroll, Ctrl+P/N, Ctrl+E/Y, Ctrl+S, Ctrl+X, Ctrl+W, `?`, or Ctrl+C across builder fields, popups, text entry, result/error history, save flow, active execution, and the terminal-too-small screen. Examples left open include whether `?` inserts into text entry or opens help, whether history keys work while a popup is open, and whether result paging works during text entry.

Replace the partial matrix with either:

- a true context-by-action matrix, including global-action precedence; or
- an explicit precedence hierarchy followed by context-specific overrides.

Resolve Ctrl+C consistently for every modal. Include focus restoration after cancel/complete and define the minimal key set on the terminal-too-small screen so the “quit at any time” story remains true.

## Major Findings

### M1. Snapshot finalization uses an undefined “navigate away to build” event

**References:** lines 42, 56–58, 64–75, 154–159, 195, and 205.

The builder remains visible and editable while results are active, so there is no clearly defined transition “away to build.” It is unclear whether a snapshot freezes when focus enters a builder field, when a field is modified, when query history restores state, when result history is opened, when a popup opens, or only when a new query is executed.

The phrase “when the execution ends” is also circular: after the first page and count finish, the SELECT may still own future paging, but no request is in flight. Define the active-result state separately from in-flight requests and identify every finalization event. Focus-only movement should not accidentally freeze a result unless that is intentional.

### M2. Result export is undefined while the active result can still change

**References:** lines 58, 167–177, 194–198, and 239–241.

Ctrl+X may be invoked while the count or a page request is pending, or before the active cache has been finalized into a history snapshot. Opening the file picker does not say whether execution continues. If a late page mutates the cache while serialization is underway, “export exactly the snapshot” has no stable meaning.

Specify whether Ctrl+X is disabled while a request is in flight, finalizes the active result, cancels pending work, or takes an immutable copy at the instant export begins. State what metadata/warning accompanies a partial active export and whether opening or cancelling the picker affects the active execution.

### M3. The pre-flight “COUNT(*) wrapper” is not a complete generated-query rule

**References:** lines 139–141 and 202.

A SELECT can be wrapped as `SELECT COUNT(*) FROM (<select>)`; an UPDATE or DELETE cannot be placed in that subquery position. The PRD calls the destructive-write estimate a “SELECT COUNT(*) wrapper” without defining the actual SQL transformation.

Require an estimate generated from the write target and predicate, for example `SELECT COUNT(*) FROM <quoted-table> [WHERE <same-predicate>]`, using the same bound predicate parameters. Label it an estimate of matching target rows, not a guaranteed count of all trigger side effects or of the driver's eventual `RowsAffected()`. Add tests proving that UPDATE SET parameters are not mistakenly included in the estimate and that NULL/LIKE predicates are reproduced correctly.

### M4. The file-deletion promise is stronger than the specified detection mechanism

**References:** lines 109, 190, and 216.

The user story asks for the connection to be monitored so the program errors if the file is deleted mid-session. The implementation decision checks only immediately before each statement. Deletion while idle is therefore not detected until another operation, and deletion between the check and statement execution may not produce the terminal state at all because an already-open SQLite handle can remain usable.

Either require periodic/filesystem monitoring with its race and portability behavior, or narrow the user story to detection before attempted database operations. State how a deletion discovered by a driver error after the pre-check is classified.

### M5. Concurrent first-page and count reads can describe different database states

**References:** lines 41, 150–151, 174–177, 194, and 295.

The first page and total count are separate concurrent autocommit reads. Under concurrent writes, they can observe different committed states. The PRD acknowledges drift between pages but still describes the header as the logical result row count without acknowledging that it may not correspond to the rows currently displayed.

Either accept and document that the count is a non-snapshot total from an independent read, or define a consistency mechanism. The former is consistent with the decision not to hold a read transaction, but the header and help text must not imply an exact shared snapshot.

### M6. The untouched-on-cancellation guarantee needs a commit boundary

**References:** lines 43–54, 177, 203, and 216.

An internal transaction allows rollback before commit, but “every cancelled write leaves the database untouched” needs a defined point after which cancellation is no longer accepted. Ctrl+W racing with COMMIT cannot both guarantee cancellation and guarantee rollback if the commit has already taken effect. Some commit/I/O failures can also have outcomes that cannot safely be summarized by an unconditional promise without checking connection/transaction state.

Define phases such as cancellable statement execution, committing, and completed. Once commit begins, either ignore Ctrl+W and report the eventual commit result, or document an indeterminate/fatal outcome for the exceptional cases the driver cannot resolve. Tests should exercise cancellation immediately before and after the commit boundary.

### M7. The UI path for selecting bare `COUNT(*)` is still unclear

**References:** lines 20–24, 79–80, and 125–128.

The state representation is now clear, but the interaction is not. The aggregate popup is said to offer `COUNT(*)` when no entries exist, while that popup is otherwise opened only after the user selects a column. The PRD does not say whether `COUNT(*)` appears in the column popup, whether selecting any named column merely opens an aggregate popup containing an unrelated `COUNT(*)` choice, or what happens to that pending column if `COUNT(*)` is chosen.

Define the exact key sequence and popup location for producing `SELECT COUNT(*)`, then cover that sequence in the required UI behavioral tests.

### M8. The high-risk test list contains an undefined supersession case and omits the remaining boundaries

**References:** lines 115–116 and 244–259.

The test list requires “superseded/late responses discarded,” but Enter is ignored while an execution is in flight and no other supersession event is defined. The intended case may be navigation away, cancellation followed by a new execution, or same-execution page navigation; each needs different identity rules.

After resolving the findings above, add explicit coverage for:

- destructive-write estimate/confirmation/history phases;
- stale same-execution page responses and resize;
- reverse traversal across the 10,000-row boundary;
- grouped non-aggregate and wildcard validation;
- global-key precedence and Ctrl+C in every modal;
- active-result export during count/page work;
- first-page/count snapshot drift; and
- cancellation at the write commit boundary.

### M9. UTF-8 promises do not cover invalid SQLite TEXT

**References:** lines 182 and 198–199.

SQLite TEXT can contain byte sequences that are not valid UTF-8. The PRD promises UTF-8 CSV and defines Unicode-width rendering, but does not say whether invalid text is replaced, escaped, exported as bytes, or treated as an error. JSON also requires valid Unicode strings.

Define one deterministic policy across the grid, CSV, and JSON. The simplest requirement is usually replacement with U+FFFD plus a visible/export warning, but rejecting export or using an explicit encoding representation are also valid product choices.

## Minor Findings

### m1. “Full history” contradicts the 20-entry cap

**References:** lines 13 and 195.

The Solution promises “full history,” while each history list evicts its oldest entry after 20. Replace “full history” with “in-session query and result history” or state the cap at the first promise.

### m2. “Distinct rows” is misleading for duplicate-valued results

**Reference:** line 58.

The cache must preserve duplicate result rows. If “distinct” means distinct logical row positions rather than SQL-distinct values, say “up to 10,000 retained row positions” or simply “rows.”

### m3. The “type-aware entry” value proposition is not implemented by the requirements

**References:** lines 11, 25, 39, 193, and 199.

All operators are offered for all columns and every value uses the same lexical parse-and-bind rule. Column declared type is inspected but does not affect entry. Either remove “type-aware entry” from the Solution or define the actual type-aware behavior, such as hints or operator filtering. Parse-and-bind alone is type inference from text, not column-aware entry.

### m4. Runnable-state prerequisites are not enumerated

**References:** lines 73, 128, and 224–227.

The QueryBuilder explicitly mentions aggregate and ORDER BY validity, but does not list all mandatory conditions: selected command and eligible table, non-empty SELECT projection, at least one UPDATE SET assignment, completed value prompts, valid Limit, and so on. These are inferable, but a single command-by-command runnable-state table would make Enter behavior and tests unambiguous.

### m5. Some visual acceptance language remains subjective

**References:** lines 17–18, 151–152, 247, 281, and 284.

“Top majority,” “grow to multiple lines,” “preserving position where possible,” and “broken layout” lack pass/fail rules. Rendering may remain manual-only, but the manual checklist should still define expected proportions, truncation/wrapping behavior, and resize outcomes at minimum and representative sizes.

## Traceability to `Ideas.md`

Traceability is strong. The PRD preserves or deliberately narrows the source goals for:

- command-line SQLite and local-D1 opening;
- the Go/Bubble Tea/Lip Gloss/mow.cli stack;
- structured Command → Table → Column(s) query construction;
- assisted WHERE, GROUP BY, ORDER BY, and Limit fields;
- UPDATE, DELETE, and INSERT flows;
- paged and horizontally scrollable results;
- query and result history;
- query and result saving; and
- post-v1 free-text SQL and additional database engines.

The result-history binding differs from the early idea (`Ctrl+Shift+P/N` versus `Ctrl+E/Y`), but `Ideas.md` identifies the PRD decisions as authoritative and the PRD gives a sound terminal-compatibility rationale. This should be treated as an explicit refinement, not an omitted requirement.

The PRD adds many requirements not present in the initial idea list—transactions, cache finalization, atomic exports, schema refresh, exact numeric parsing, and extensive error states. That elaboration is appropriate, but it creates most of the remaining state-machine risk identified above.

## Required Decisions Before Issue Generation

At minimum, revise the PRD to decide and document:

1. Whether destructive-write estimation is part of the write execution, a separate execution, or a pre-execution phase, including all history effects.
2. How later page requests are serialized or identified, and how navigation, resize, cancellation, and stale same-execution responses interact.
3. The cache invariant and eviction/export behavior when re-fetching older pages into a full 10,000-row cache.
4. GROUP BY validation for non-aggregate-only projections and wildcard projection.
5. A complete global-key precedence model, including the contradictory Ctrl+C behavior in non-quit modals.
6. Exact active-result finalization events.
7. The behavior of Ctrl+X while count/page work or active caching is still underway.
8. The generated estimate query and its non-binding “matching target rows” meaning.
9. Whether file deletion is monitored continuously or only detected before attempted operations.
10. The accepted consistency limitation between concurrent page and count reads.
11. The cancellation/commit boundary for writes.
12. The exact UI sequence for adding bare `COUNT(*)`.
13. Invalid-UTF-8 handling and tests for the newly clarified high-risk boundaries.

## Recommendation

Revise the PRD once more before generating issues. The revision can be focused: no broad redesign is needed. Resolve the five blockers first, then incorporate the major findings into the execution lifecycle, paging/cache rules, key matrix, export behavior, and testing section. After that, perform a short consistency pass to ensure the same write and SELECT state names are used in the lifecycle, module interfaces, user stories, and tests.
