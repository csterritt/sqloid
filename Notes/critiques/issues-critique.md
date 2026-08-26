# Issues Critique — Sqloid v1

Reviewer: skeptical senior engineer, issues-only review.
Inputs: `Notes/PRD-sqloid.md` (authoritative), `Notes/Ideas.md`, all 49 files in `Notes/issues/` (implemented in filename order).
Scope: the 49 issues themselves — their completeness, ordering, dependencies, and quality — not the PRD and not the code.

---

## Summary

The issue set is strong as a骨架: all 90 PRD user stories are mapped to an issue, the dependency graph is acyclic with every blocker earlier-numbered than its dependents (so filename order is a valid topological order), the template is consistent, and the high-risk areas (cancellation, commit boundary, dual cache caps, identity checks) are represented. Tracer bullet (Issue 22) exists.

The main weaknesses are **ordering/sequencing problems that will force rework or stall AFK progress**, a few **missing or under-owned PRD requirements**, and **thin acceptance criteria on several complex issues**. The most consequential finding is that shared value-rendering/name-dedup (Issue 47) is positioned *after* the first result grid (Issue 22), which guarantees rework. The second is that three HITL issues (8, 24, 28) sit on the critical path and gate large bodies of AFK work.

Findings are prioritized. "P0" means fix before implementation starts; "P1" means fix soon; "P2" means nice-to-have.

---

## Strengths

- All 90 user stories are covered by at least one issue (coverage map verified).
- Dependency graph is acyclic and consistent with the "implemented in order" rule: no issue is blocked by a later-numbered issue.
- Uniform template (Type, Blocked by, Parent PRD, What to build, How to verify, Acceptance criteria, User stories addressed).
- Acceptance criteria are mostly Given/When/Then, which is good for verification.
- High-risk PRD areas are represented: scoped cancellation + bounded settlement (28), commit boundary (43), outcome-unknown (45), dual cache caps (30/31), identity checks (7), concurrent count/page (24).
- A tracer bullet (Issue 22) is present and correctly positioned after the runnable/validation chain.
- AFK/HITL labeling distinguishes human-review needs from autonomous work.

---

## P0 — Fix before implementation starts

### P0-1. Issue 47 (shared rendering/dedup) is after Issue 22 (first grid) → guaranteed rework

Issue 47 centralizes "result output names and value representation shared by grid and exporters": full-set collision-safe deduplication, exact finite REAL tokens, visible grid control characters, maximal invalid-UTF-8 replacement, and `[BLOB n bytes]` rendering. It is blocked only by Issue 22.

Issue 22 ("First end-to-end SELECT and result grid") must render a bordered grid with deduplicated frozen headers and result values — i.e. it needs exactly the rendering that Issue 47 promises to centralize. Because 47 comes after 22, Issue 22 must either (a) duplicate the rendering/dedup logic and throw it away when 47 lands, or (b) 47 is effectively done inside 22 and the later issue is a no-op rename.

Either way the current ordering is wrong. The shared name-dedup + value-rendering module is a dependency of the grid, not a follow-on.

**Recommendation:** make Issue 47 (or a split of it: name dedup + value rendering) a dependency of Issue 22, or merge the relevant acceptance criteria into Issue 22 and repurpose 47 to "extract/centralize rendering for exporters." The same rework risk applies to BLOB display, invalid-UTF-8 handling, and REAL token formatting, all of which the grid needs on first render.

### P0-2. Query history (Issue 35) is late; executions in 22–34 append no history

The PRD is explicit: "Append occurs only when an actual execution starts" (History) and "After successful pre-execution schema validation, Enter starts one SELECT execution and appends query history unless it is identical to the immediately preceding execution" (SELECT §1). Issue 22's first SELECT execution is therefore required to append query history.

But query history is owned by Issue 35, which is blocked by 34, which is blocked by 26/33, etc. Issues 22 through 34 all perform or simulate executions with no query-history append, no consecutive-identical suppression, and no stable IDs. The first place this contract is exercised is Issue 35, ~12 issues after the first execution.

**Recommendation:** introduce a minimal query-history append (stable ID + append-on-execution + consecutive-identical suppression) no later than Issue 22, and let Issue 35 own the *navigation/restore/eviction* layer rather than the append contract. Otherwise the append-on-execution invariant is untested for a long stretch and likely to be retrofitted incorrectly.

### P0-3. Three HITL issues (8, 24, 28) are on the critical path and gate AFK work

- Issue 8 (HITL) → blocks 11, 29, 54, 55 (and transitively almost everything UI).
- Issue 24 (HITL) → blocks 25 (paging), 33, 34, 35, 36, 45, 48, 49… i.e. the entire result/history/export chain.
- Issue 28 (HITL) → blocks 42 (writes), 43, 45, 55.

Each of these contains a large AFK-testable core (layout arithmetic; barrier-based concurrent count/page overlap; barrier-based cancellation/isolation/late-success rejection) plus a genuinely HITL manual review (the 80×24/100×30/160×50 matrix; cross-platform journal inspection; cross-platform cancellation latency observation). Labeling the whole issue HITL means AFK work cannot proceed past these gates without a human, even though most of the logic and its automated tests could land autonomously.

**Recommendation:** split each into an **AFK** issue (logic + automated barrier/table tests) and a **HITL** issue (manual matrix/cross-platform review that depends on the AFK half). This unblocks downstream AFK work behind the AFK half while preserving the human review.

### P0-4. Issue 55 (quit) is missing cleanup dependencies

Issue 55 acceptance requires "required request/transaction cleanup finishes before exit" and "accepted quit while a write … waits for commit/rollback resolution." That cleanup is owned by Issue 34 (active SELECT finalization on quit) and Issues 42/43 (write commit-boundary + quit settlement). Issue 55 is blocked by 28, 41, 53, 54 — but **not** by 34, 42, or 43. Issue 34 is not even a transitive dependency (55→28→26; 34→26,33 — 33 is not under 55's chain).

**Recommendation:** add Issue 34, Issue 42, and Issue 43 as blockers of 55 (or restructure so quit cleanup is composed from those modules). As written, 55 could be "done" before the SELECT/write cleanup it depends on exists.

### P0-5. No issue owns the deletion/replacement terminal-state UI workflow

Issue 7 *detects* deletion/replacement and enters the terminal state. Issue 45 owns the *outcome-unknown* terminal UI workflow (Ctrl+E/Y navigation, Ctrl+S save, Ctrl+X rejection, immediate status-1 quit, in-memory history selection). There is no symmetric issue for the **deletion/replacement** terminal UI workflow, which the PRD specifies in the same breath: "Terminal deletion/replacement/outcome-unknown states … forbid database work but permit applicable in-memory history selection/query saving before immediate status-1 quit" (User story 80) and the Context/Action Matrix "Terminal deletion/replacement/outcome-unknown" row.

Issue 13 covers terminal *precedence* during refresh, and Issue 55 covers terminal immediate exit, but neither owns the full deletion/replacement terminal UI behavior (initial result-history selection, Ctrl+S targeting last executed query, Ctrl+X rejection, Ctrl+P/N). Issue 45's scope is explicitly outcome-unknown only.

**Recommendation:** add an issue (or extend Issue 7's scope with explicit acceptance criteria) for the deletion/replacement terminal UI workflow, mirroring Issue 45.

---

## P1 — Fix soon

### P1-1. Issue 48 (SQL save) missing dependency on Issue 36 (result history)

Issue 48's targeting priority is "viewed historical result's associated query → current runnable builder → last executed query." The first option requires result-history browsing (Issue 36). Issue 48 is blocked by 35 (query history) and 42 (last executed) but **not** by 36. The "viewed historical result's associated query" path cannot be implemented or tested until 36 exists.

**Recommendation:** add Issue 36 as a blocker of 48.

### P1-2. Issue 21 (validation cancellation) precedes Issue 28 (general Ctrl+W)

Issue 21's automated tests assert "Ctrl+W" cancellation of validation, and its acceptance implies cancellable validation. But the general Ctrl+W + connection-scoped interrupt infrastructure is Issue 28, which is blocked by 26 and 27 (both after 21). So 21 must either stub cancellation or pull in infrastructure that 28 owns.

**Recommendation:** either make Issue 21 depend on a split-AFK Issue 28 (per P0-3) for the cancellation path, or narrow 21's acceptance to "validation issues cancel/abort" and let 28 wire the real interrupt. Today the cancellation contract for validation is tested before the cancellation module exists.

### P1-3. Issue 27 (in-flight feedback) claims write-phase feedback before writes exist

Issue 27 is blocked by 22 only, yet its scope and acceptance include "estimate/commit/rollback feedback." Estimate (40/41) and commit/rollback (42/43) do not exist until long after 27. The SELECT/page/count feedback is testable at 27; the write-phase feedback is not.

**Recommendation:** split 27 into "SELECT/page/count in-flight feedback" (blocked by 22) and "write-phase in-flight feedback" (blocked by 42/43), or add 42/43 as blockers for the write-phase portions and narrow 27's acceptance accordingly.

### P1-4. No issue owns the integrated release-capability suite or modernc pinning

The PRD makes two things release- and upgrade-blocking: (a) "Mandatory Linux/macOS release and dependency-upgrade blocker" capability suites (journal overlap, pool size, cancellation latency/isolation/late results, limits, rollback, commit boundaries); and (b) "Pin an exact vetted modernc version" with "a version that fails must be changed, never silently accepted as best-effort."

Pieces of (a) are scattered across Issues 5, 7, 24, 28, 42, 43, each of which mentions "release-blocking" or "Mandatory Linux/macOS." But no issue owns the **assembled** capability suite that must pass on every release and every modernc upgrade, nor the **pinning policy + upgrade-gate** (go.mod pin, CI gate that fails on a bad modernc). This is exactly the kind of cross-cutting requirement that falls through the cracks when distributed.

**Recommendation:** add an issue that owns the integrated Linux/macOS capability suite and the modernc pin/upgrade-gate, depending on 5/7/24/28/42/43. Without it, the "release-blocking" language in the per-issue verify sections is unowned.

### P1-5. Implicit `ORDER BY rowid` fallback for stable paging is unowned

PRD "Paging consistency": "Without user ORDER BY, append `ORDER BY rowid` only for ordinary rowid tables without a declared rowid shadow; no implied stability for views, virtual/WITHOUT ROWID/shadowed, aggregate/grouped, ties, or concurrent writes."

This is a paging-construction concern (it changes the emitted SELECT used by Page Up/Down). Issue 25 (paging) does not mention it in scope or acceptance. Issue 18 (ORDER BY rules) covers user-supplied ORDER BY, not the implicit fallback. Issue 22 (first SELECT) doesn't mention it. The rowid-shadow metadata exists in Issue 9, but nothing wires "no user ORDER BY + ordinary rowid table without shadow → append `ORDER BY rowid`" into the page SQL.

**Recommendation:** assign the implicit rowid-order fallback explicitly to Issue 25 (paging) with an acceptance criterion, since it is part of producing correct page SQL.

### P1-6. Issue 5 (pool) is over-coupled to Issue 4 (D1 diagnostics)

Issue 5 ("Two-connection SQLite pool and limits") is blocked by Issue 2 **and** Issue 4. Issue 4 is D1 *diagnostics* (error messages for zero/multiple candidates). The pool/Connection foundation does not depend on D1 diagnostics text; it depends on a validated open path. Issue 2 (sqlite open) is the real dependency; D1 discovery (3) produces a path that is then validated by the same code as Issue 2.

Coupling the pool foundation to D1 diagnostics means the entire Connection/Schema foundation (7, 9, and everything downstream) waits for D1 error-message wording. If D1 is lower priority, this serializes the foundation unnecessarily.

**Recommendation:** drop Issue 4 as a blocker of 5; keep Issue 2 (and optionally Issue 3 if the Connection module's `open-D1` entry point is needed at pool-build time). Wire D1 diagnostics (4) into the Connection module separately.

### P1-7. Issue 54 (help/overlay precedence) is over-coupled to Issue 52 (picker)

Issue 54 is blocked by 8, 12, and 52. The core precedence logic (terminal → top overlay → input → request → base; literal `?` in inputs; Esc cancels only top overlay) does not depend on the file picker. The picker is one overlay among many; the precedence rules are generic. Coupling 54 to 52 makes the foundational key-precedence behavior land late (after the entire save/export chain), which in turn makes Issue 55 late.

**Recommendation:** drop 52 as a blocker of 54; if the "every matrix row" verification needs the picker, add a separate HITL verification that depends on both 54 and 52.

### P1-8. Long AFK chain before the first end-to-end SELECT

Issue 22 (first visible SELECT) is blocked by 21 ← 19 ← {17,18} ← {16,17} ← 15 ← {11,12} ← 11 ← {8,9} ← 9 ← 5 ← {2,4} ← … That is ~14 issues of pure foundation (CLI, validation, pool, schema, command/table, popup, projection, WHERE, GROUP/ORDER/LIMIT, runnable, stale-refresh, validation) before a single end-to-end SELECT renders. The PRD's testing philosophy explicitly favors external-contract tracer bullets.

**Recommendation:** insert an early thin tracer (e.g. "hardcoded `SELECT *` from a chosen table, no validation, no popups, raw grid") right after Issue 5/9 to de-risk the Bubble Tea ↔ Connection ↔ Schema integration before the full builder is built on top. This is not a new feature; it is a risk-reduction milestone that the current plan lacks.

---

## P2 — Quality / completeness nits

### P2-1. Several complex issues have thin acceptance criteria

- **Issue 7** (request-boundary identity) has 3 criteria but the PRD high-risk #11 is extensive: raced replacement + request error classified terminal immediately; raced replacement + request success detected at the *next* boundary; one precheck for an entire write with none between statement/COMMIT. These specific cases are not in the acceptance criteria.
- **Issue 26** (request identities/stale rejection) has 3 criteria; the execution-ID vs request-ID vs generation distinction and the "no replacement starts before settlement" rule deserve explicit criteria.
- **Issue 19** (runnable state) has 3 criteria, all SELECT-flavored, but its scope is the *general* runnable framework including UPDATE/DELETE/INSERT prerequisites (SET uniqueness, Value/NULL completion, INSERT per-column completion, zero-insertable-column). The write prerequisites are tested in 37/38/39, but 19's acceptance should state it delivers the general framework, not just SELECT.

**Recommendation:** expand acceptance criteria on 7, 26, and 19 to name the specific raced/edge cases from the PRD.

### P2-2. Non-finite REAL grid rendering is not clearly owned

Issue 47 owns finite REAL tokens. Issues 50/51 own non-finite REAL in CSV/JSON (`"Inf"`/`"-Inf"`/`"NaN"` quoted or textual). The PRD says "Existing non-finite policy remains separate" but does not specify grid display of non-finite REALs. Issue 47's acceptance only mentions finite REAL. It is unclear what the grid shows for an existing `Inf`/`NaN` REAL.

**Recommendation:** decide and document grid rendering for non-finite REALs (likely the textual token), and assign it to Issue 47.

### P2-3. COUNT(*) sentinel vs named-aggregate dedup boundary is fuzzy

Issue 15 owns the empty-projection `COUNT(*)` path; Issue 16 owns ordered projection editing and exact-duplicate rejection. The PRD says bare `COUNT(*)` "can coexist with subsequently selected aggregate entries" and "an identical pair is not added twice." The interaction — selecting `COUNT(*)` then adding `COUNT(col)` (distinct, allowed) vs adding a second `COUNT(*)` (impossible by UI, but the dedup rule still applies) — sits on the 15/16 boundary. Issue 16 is blocked by 15, so it is probably fine, but the dedup behavior for the sentinel specifically should be called out in 16's acceptance.

**Recommendation:** add an acceptance criterion to 16 covering dedup interaction with the `COUNT(*)` sentinel.

### P2-4. Issue 1 version flag output is unspecified

Issue 1 acceptance: "Given help or version flags, when invoked, then the documented information is printed successfully." The PRD says `--version`/`-v` prints version but does not define the string. "Documented information" is circular. Decide the version string format (e.g. `sqloid <semver>` or `sqloid version <semver>`) and put it in the acceptance criterion.

### P2-5. Issue 11 acceptance doesn't fully capture user story 19

User story 19: "switching a selected view to a write command to clear Table and focus it, while eligible ordinary/virtual tables remain selected." Issue 11 acceptance: "Given a selected view and a switch to a write command, then Table is cleared and focused." The "eligible ordinary/virtual tables remain selected" clause (i.e. the table *list* still shows eligible tables, the *selection* is cleared) is ambiguous in the PRD and dropped in the issue. Clarify whether "remain selected" means the list is still populated or some selection persists, and reflect it in the acceptance criterion.

### P2-6. "database is locked" mid-session ordinary error handling is unowned

PRD: "Mid-session expiry appears as ordinary `database is locked` request error unless health classification overrides it." Issue 5 owns the busy timeout; Issue 36 owns query-error recovery generally. But the specific "database is locked" ordinary-error path (as opposed to health-terminal classification) is not called out in any acceptance criterion. Ensure Issue 36 (or 27) explicitly covers a busy-timeout-exceeded request error rendered as an ordinary query error (not terminal).

### P2-7. Issue 37 is not explicitly blocked by Issue 17 (WHERE reuse)

Issue 37 (UPDATE) is blocked by 14 and 19. It reuses the shared WHERE flow from Issue 17. 17 is transitively present via 19, but UPDATE's WHERE is a direct reuse and should be a direct dependency for clarity (Issue 38 correctly lists 17). Minor consistency fix.

### P2-8. No issue mentions go.mod / module init / build setup

Issue 1 is the entry point but does not mention `go.mod`, module path, or build/CI setup. This is usually implicit, but for a greenfield repo it is worth one acceptance criterion on Issue 1 ("the module builds with `go build ./...` and `go vet` is clean") so the foundation is testable from day one.

---

## Cross-cutting recommendations

1. **Re-sequence around rendering and history (P0-1, P0-2).** Move shared name-dedup + value rendering ahead of the first grid, and move query-history append ahead of the first execution. Both are PRD invariants that currently first appear long after the code that must satisfy them.
2. **Split the three critical-path HITL issues (P0-3).** AFK logic + automated tests now; HITL manual review after. This is the single biggest unblocker for autonomous progress.
3. **Add the two missing ownership issues (P0-5, P1-4):** deletion/replacement terminal UI workflow, and the integrated release-capability suite + modernc pin/upgrade gate.
4. **Tighten dependency edges (P0-4, P1-1, P1-2, P1-3, P1-6, P1-7, P2-7).** Quit cleanup deps; save→result-history; validation→cancellation; in-flight write feedback→writes; pool→drop D1; precedence→drop picker; UPDATE→WHERE.
5. **Insert an early integration tracer (P1-8)** to de-risk the TUI/Connection/Schema stack before the full builder is built on it.
6. **Expand thin acceptance criteria (P2-1)** on Issues 7, 26, 19 so the raced/edge cases from the PRD high-risk list are explicitly verified, not just implied by "How to verify."

---

## Coverage note

All 90 user stories are mapped. No user story is uncovered. The gaps above are about *how* the PRD requirements are distributed and ordered, not about missing user stories. The PRD "Module Design" per-module Tested requirements are distributed across issues but the integrated Connection release-capability suite (P1-4) is the only one with no clear owner.

---

## Resolutions (applied 2026-08-25)

All critique items were reviewed with the user. The following changes were applied to the issue files.

### New issues created (7)

| File | Source | Blocked by | Blocks |
|---|---|---|---|
| `006-cancellation-infrastructure.md` | P1-2 (split of 28 for dependency) | 5 | 21, 28 |
| `010-early-integration-tracer.md` | P1-8 (early tracer) | 5, 9 | — (optional before 22) |
| `020-minimal-query-history-append.md` | P0-2 (history append before first execution) | 19 | 22 |
| `023-non-finite-real-grid-rendering.md` | P2-2 (non-finite REAL grid) | 22 | — |
| `044-write-phase-in-flight-feedback.md` | P1-3 (split of 27) | 42, 43 | — |
| `046-deletion-replacement-terminal-workflow.md` | P0-5 (deletion/replacement terminal UI) | 7, 36 | 55 |
| `056-release-capability-suite-and-modernc-pin.md` | P1-4 (integrated release suite + pin) | 5, 7, 24, 28, 42, 43 | — |

### Issue split (1)

- **Issue 28** split into `006` (cancellation infrastructure: context + connection-scoped interrupt, blocked by 5) and `028` (application to SELECT/write + bounded settlement tests, blocked by 6, 26, 27). This was necessitated by P1-2: making 21 depend on 28 would create a circular dependency (21→28→26→25→22→21). The infrastructure half (006) can come before 21; the application half (28) stays in place. This is a scope split for dependency reasons, distinct from the P0-3 AFK/HITL split decision (declined).

- **Issue 27** narrowed to SELECT/page/count feedback only. Write-phase feedback (estimate/commit/rollback) moved to `044` (blocked by 42, 43).

### Issue repurposed (1)

- **Issue 47** repurposed from "centralize name-dedup + value rendering" to "extract and centralize rendering for exporters." The initial name-dedup + value rendering implementation was merged into Issue 22 (per P0-1 decision). Issue 47 now factors that logic into a reusable module for CSV/JSON exporters, blocked by 22 and 49.

### Dependency changes (8)

| Issue | Change | Reason |
|---|---|---|
| 5 | Dropped 4 from blockers | Pool doesn't depend on D1 diagnostics wording (P1-6) |
| 21 | Added 6 as blocker | Validation cancellation needs cancellation infrastructure (P1-2) |
| 22 | Added 20 as blocker | First execution must append query history (P0-2) |
| 28 | Changed 5→6 in blockers | Infrastructure moved to 006 (P1-2 split) |
| 37 | Added 17 as blocker | UPDATE reuses shared WHERE flow directly (P2-7) |
| 48 | Added 36 as blocker | "Viewed historical result's query" targeting needs result history (P1-1) |
| 54 | Dropped 52 from blockers | Precedence rules are generic, not picker-dependent (P1-7) |
| 55 | Added 34, 42, 43, 46 as blockers | Quit cleanup invokes SELECT finalization + write settlement + deletion/replacement terminal UI (P0-4, P0-5) |

### Acceptance criteria changes (9)

| Issue | Change |
|---|---|
| 1 | Specified version string format (`sqloid <version>`); added `go build ./...` + `go vet` clean criterion (P2-4, P2-8) |
| 7 | Added raced replacement + error (terminal immediately), raced replacement + success (next boundary), one precheck for entire write (P2-1) |
| 11 | Clarified: table *selection* cleared, eligible table *list* remains populated on view-to-write switch (P2-5) |
| 16 | Added COUNT(*) sentinel dedup interaction criterion (P2-3) |
| 19 | Expanded scope to general framework (all 4 commands); added UPDATE/DELETE/INSERT prerequisite criteria (P2-1) |
| 22 | Merged name-dedup + value rendering (REAL tokens, BLOB display, invalid-UTF-8, control chars, dedup) (P0-1) |
| 25 | Added implicit `ORDER BY rowid` fallback criteria (P1-5) |
| 26 | Added execution-ID vs request-ID vs generation distinction; newer-execution late discard (P2-1) |
| 36 | Added `database is locked` ordinary-error criterion (P2-6) |

### No change (1)

- **P0-3**: Issues 8, 24, 28 remain HITL. The AFK/HITL split was declined; the large AFK-testable cores remain gated by human review.
