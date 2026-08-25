# Critique 4 — Notes/PRD-sqloid.md

Reviewer stance: skeptical senior engineer doing a requirements-only review, checked against Notes/Ideas.md.

## Overall assessment

This is an unusually thorough PRD. The lifecycle contracts (request identity, viewport generations, commit boundary, cache invariant), the authoritative key-precedence matrix, and the explicit out-of-scope list are all stronger than what most v1 PRDs manage. The three prior critiques clearly landed. The remaining problems are mostly unresolved ambiguities, unstated assumptions about the chosen driver, and a few genuine requirement gaps rather than structural flaws.

## Major issues

### 1. Concurrent reads on one SQLite file are assumed, not specified
The PRD requires the first page and count to be **concurrent** autocommit reads, and later a page plus nothing else. With `modernc.org/sqlite` over a single `*sql.DB`, concurrency depends on connection-pool behavior and journal mode: in rollback-journal (non-WAL) databases, a reader blocks other readers at the file-lock level in ways that make "concurrent independent reads" behave differently than in WAL mode. WAL is named under supported environments, but nothing requires the database *be* in WAL mode, and sqloid must not modify the user's file pragmas. The PRD should state what is guaranteed when the target is a non-WAL database: are the two reads issued on separate pooled connections, is serialization acceptable, and does "concurrent" degrade to sequential without violating any stated contract?

### 2. Cancellation of in-flight SQL is an unverified driver capability
The entire lifecycle — Ctrl+W cancelling count/page requests, cancellable `beginning`/`executing` phases, quit waiting for settlement — presupposes that a running SQL statement can actually be interrupted. This depends on an interrupt mechanism in `modernc.org/sqlite`. If the driver lacks a usable interrupt (or it is best-effort), then "cancelled" requests still consume the busy timeout or run to completion, and several requirements (e.g., "waits for that request to settle", serialized paging responsiveness) silently change meaning. The PRD should either cite the specific driver mechanism as a dependency or define behavior when cancellation is slow/unresponsive (is there a bound on "settle"?).

### 3. UPDATE SET values cannot be NULL, and this is nowhere acknowledged
The numeric parsing section says NULL is available "only through explicit popup/operator choices." INSERT has such a choice ({Value, NULL, Default/Omit}), but UPDATE's SET value entry is described only as guided text entry with a "completed value." As written, v1 cannot set a column to NULL via UPDATE. That may be an intentional scoping decision, but it appears in neither the grammar section nor Out of Scope, and users will expect it (`SET x = NULL` is among the most common updates). Either add an explicit NULL choice to the SET value flow or list "UPDATE SET to NULL" in Out of Scope.

### 4. `= NULL` / `!= NULL` predicates are constructible and always vacuous
All operators are offered for all columns, and WHERE takes a value. Since WHERE values are entered as text (no NULL binding), the user can build `col = NULL`, which is never true in SQL. The PRD neither warns nor restricts this. Options: suppress `=`/`!=`… no — simplest is a UI hint when a null-inappropriate operator is paired with intent, or at least help-text disclosure. As specified, this is a silent wrong-result trap. Relatedly, `LIKE` on a column holding NULL behaves as non-matching; fine, but worth one line in help since the PRD already commits to disclosing count drift.

### 5. Grid/CSV rendering of finite REAL values is underspecified
Non-finite REALs get exact representations (`"Inf"`, `"NaN"`, …), but ordinary float formatting is undefined: How is `0.30000000000000004`, `1e20`, or `-0.0` displayed in the grid, written to CSV, and emitted as a JSON number? Different strategies (shortest-round-trip vs `%g` with fixed precision) will produce visibly different grids and exports, and JSON number formatting affects downstream parsers. Specify the algorithm (recommendation: Go's shortest representation that round-trips, consistently in grid, CSV, and JSON) and integer-valued REAL display (does REAL `1.0` render as `1` or `1.0`?). Same question for very large INTEGERs is answered by int64, but REAL is genuinely ambiguous.

## Moderate issues

### 6. Counting a LIMITed result is surprising and its purpose is unstated
The count is defined as `SELECT COUNT(*) FROM (<user SELECT including LIMIT>)`. For `SELECT * FROM t LIMIT 10` against a million-row table this always reports `Count: 10`, which most users will read as "the table has 10 rows," especially given the header wording discloses drift but not self-limiting. If this is deliberate (it makes `Count` reflect the logical result), say so explicitly in the help-disclosure requirement; if not, consider excluding LIMIT from the count subquery. Either way, the interaction between LIMIT and the completeness labels deserves a sentence: a LIMITed full scan to the end is `complete`, not `partial`.

### 7. History dedup compares only against the last executed state
Normalization dedup is defined as differing from "the last executed state." So A → B → A appends A twice, occupying 2 of 20 slots with identical entries. Probably acceptable for v1, but the PRD presents dedup as a correctness property without acknowledging this consequence. One clarifying sentence would do.

### 8. Path pre-check before every request is a per-keystroke-latency cost with no failure-mode detail
Checking the original path immediately before every page request adds a stat syscall per Page press — cheap, fine — but the interesting cases are underspecified: What if the stat succeeds but the subsequent open/statement fails because the file was replaced between check and request (TOCTOU)? The PRD's recheck-after-error rule classifies deletion, but replacement-at-same-path is declared undetected, so the session continues against whatever object now satisfies the path — potentially a different database with a different schema mid-execution. The "identifiers still exist in refreshed schema" gate helps, but a one-line acknowledgment that statement-level errors against a replaced file surface as ordinary errors would close the loop.

### 9. Schema refresh timing vs the runnable gate
Enter requires that "all identifiers still exist in refreshed schema," but refresh happens when Table/column popups open. If a table is dropped externally after the user finished building the query, does Enter trigger a fresh refresh (latency on every execution) or execute and let the query fail? Both defensible; pick one. Also specify whether the refresh failure path (stale-list retain/retry/cancel) blocks execution.

### 10. Outcome-unknown state and result history interplay
A write whose commit/rollback cannot be resolved enters outcome-unknown and produces "exactly one result entry" per the write lifecycle. But the terminal-state row permits history navigation and export "from immutable memory." Confirm explicitly that this write's result entry (a summary, presumably with unknown-outcome metadata) is the entry navigable/exportable there, and what exporting a write summary even means (story 70 implies summaries have no tabular target, so Ctrl+X reports no-target). The pieces are individually specified but never assembled for this state.

### 11. Estimate modal and quit confirmation suspension
Ctrl+C suspends "one overlay" via the quit modal, and modals otherwise don't stack. During `awaiting-confirmation`, Ctrl+C suspends the estimate modal; Esc/n restores it. But Esc inside the estimate modal means *dismiss preparation*. After restoring, is the modal in the identical awaiting state (estimate retained, confirmation enabled)? The "exact suspended context" language suggests yes, but given dismissal-vs-restore stakes here, call it out; same question for the overwrite-confirmation modal inside the save flow.

### 12. Window between eligibility checks and D1 discovery strings
Discovery pins `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` with exact case rules. Wrangler has changed its state layout before; the PRD treats these strings as permanent. Add a stated failure mode: if future wrangler versions relocate state, `sqloid d1` degrades to "no candidate database found" with no diagnostic hint — acceptable for v1, but document it so it isn't mistaken for a bug report later.

## Minor issues

13. **Layout arithmetic**: results ≥ ½ height + builder ≤ ⅓ leaves up to ~⅙ unassigned (status line/borders). Presumably intentional, but the visual-invariant section could state where the remainder goes to prevent implementer improvisation.
14. **`q` on Command**: Command accepts S/U/D/I printables; a user hunting for keys who types `q` instantly quits with no confirmation. Deliberate per the matrix, but it is the single most likely accidental-quit path in the app; consider requiring `q` twice or accepting the risk explicitly in Further Notes.
15. **Result-history eviction during viewing**: histories evict oldest beyond 20; if the user is viewing snapshot #18 and executes six more queries, what does their Ctrl+E view show when the viewed snapshot is evicted mid-viewing? Unspecified (likely "close/return"), but say something.
16. **Horizontal scrolling units**: `,`/`.` and Shift+Page scroll "horizontally" but by what amount — half screen, one column, page? Paging semantics are precise vertically and vague horizontally.
17. **BLOB grid truncation**: `[BLOB n bytes]` for a 50 MB blob is fine, but the count comes from reading the value; confirm blobs are length-read lazily/capped, else paging a blob-heavy table fetches megabytes per page. A performance-envelope sentence would cover it.
18. **Story 3 wording**: "read-only … rejected without creating a file" — validation order guarantees no creation, good; just ensure the read-only *open* failure (mode=rw) also names EACCES/EROFS distinctly enough for the user to act.
19. **Testing section**: excellent coverage list, but no test anywhere targets the non-WAL/concurrent-read behavior from issue 1 or driver-interrupt latency (issue 2); add them if those assumptions are confirmed.
20. **Ideas.md alignment**: consistent. Post-v1 items (read-only sessions, remote D1, logging, frozen columns) match Out of Scope. No contradictions found.

## Questions to resolve before issue generation

1. Is UPDATE-to-NULL in or out of scope for v1? (Issue 3)
2. What REAL formatting algorithm applies to grid/CSV/JSON? (Issue 5)
3. On non-WAL databases, do concurrent count+page reads remain concurrent, and is that a guarantee or best effort? (Issue 1)
4. Does Enter trigger a fresh schema validation, or does execution trust the last refresh? (Issue 9)
5. Is `Count` intentionally the count of the LIMITed logical result, and is that disclosed in help? (Issue 6)
6. What is the horizontal scroll step size? (Issue 16)

## Verdict

Requirements quality is high; the lifecycle, safety, and precedence specifications are near-implementable as written. Resolve issues 1–5 (driver concurrency/interrupt assumptions, UPDATE NULL gap, vacuous `= NULL`, REAL formatting) before generating issues — each will otherwise surface mid-implementation as either a blocked task or an undocumented behavioral decision. The moderate and minor items can be resolved inline during issue writing.
