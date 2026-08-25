# Issues Critique — Sqloid v1 (vs Notes/PRD-sqloid.md)

Reviewed: all 55 files in `Notes/issues/` against the PRD and `Notes/Ideas.md`.

Overall: this is a strong, unusually traceable issue set. Nearly every issue cites parent user stories, carries testable acceptance criteria, and encodes exact PRD strings. The issues below are ordered roughly by severity.

## A. Coverage gaps (PRD requirements no issue fully owns)

1. **Contextual help *content* is unowned.** Issue 48 builds the help mechanism (`?` routing, Esc restore), and Issues 40/40b mention "reduced help", but no issue owns the actual help text required by the PRD: count-semantics explanation ("complete SELECT including Limit… may drift… never clamps pages", story 59) and the WHERE SQL-NULL guidance in contextual help (story 28, "contextual help says to use IS NULL/IS NOT NULL…"). Issue 15's ACs cover the inline hint but only gesture at help. Add a small issue (or extend 48) owning help content per context.

2. **Exact count-wording variants are under-tested in ACs.** PRD high-risk item 13 demands exact `Result count: N` and `Result count: N (after Limit M)` tests. Issue 20 says "exact limited-result header wording" but its acceptance criteria never name the `(after Limit M)` variant. Make the second form an explicit AC.

3. **Too-small screen: Ctrl+W availability is dropped.** The PRD's context/action matrix says on the too-small screen "Active request cancellation remains available". Issue 7 owns the too-small state but none of its ACs mention Ctrl+W. Add it.

4. **Story 27 is only half covered.** "Backspace/Delete removes the most recent projection entry **or clears the focused whole-value field**." Issue 14's ACs cover projection removal only; no issue owns whole-value-field clearing (WHERE value, Limit, filename input aside). Extend Issue 14 or Issue 17's scope explicitly.

5. **"Starting an actual execution exits history first" has no AC home.** The PRD History section states it; neither Issue 31 nor Issue 32 asserts it. It's easy to lose and it guards the eviction-interaction invariant. Add an AC to 31 (query) and/or 32 (result).

6. **Startup idle view is undefined everywhere.** What does the results area show before any command is chosen? Neither PRD nor any issue defines it (blank? hint text like "Press S/U/D/I…"?). This will cause an implementer decision that renders differently across issues 7/9. Resolve in Issue 7 or 9.

7. **Directory listing order in the file picker is unspecified.** Issue 46's tests assert "ordering/navigation", but the PRD never defines directory-entry ordering (alphabetical? dirs-first? case sensitivity?). The test cannot be written deterministically until the PRD or the issue pins it.

## B. Dependency-graph problems

1. **Issue 3 blocked-by omits Issue 2.** Its first AC ("that database is opened") requires the Issue 2 validation/opening path. Either drop "is opened" from Issue 3 (discovery only, hand the path to Issue 2's opener) or add the dependency. Currently the graph invites implementing a parallel open path.

2. **Issue 23 blocked-by omits Issue 20.** Its ACs require `Counting rows…` feedback, which Issue 20 introduces. As written, 23 could be scheduled before count exists.

3. **Issue 25 blocked-by omits Issues 27/28.** Resize recovery "fetches the containing page", which lands in the capped contiguous cache; clamping/fetching without the eviction/byte-cap rules risks building a viewport path that violates cache invariants and later rework.

4. **Issue 49 blocked-by omits Issue 18 (and arguably 5b/22/23).** Accepted quit must "during pre-execution validation … request cancellation, wait for settlement, and exit without appending either history". Without Issue 18 as a prerequisite, the quit-during-validation cleanup path has no supplier at integration time.

5. **Minor:** Issue 39b covers estimate/commit/rollback feedback but not the `beginning`/`executing` write phases' running label; confirm whether Issue 23's scope statement ("SELECT/page/count") plus 39b leaves `executing` feedback owned by anyone. One line in 39b fixes it.

## C. Duplication / drift risks

1. **Literal-SQL serialization is implemented twice.** Issue 36 renders destructive write SQL "with safely serialized Value literals" for the estimate modal; Issue 42 separately specifies standalone SQL serialization (quote doubling, `X'hex'`, NULL keyword). Nothing says these share one module. Since 36 precedes 42, the modal will likely grow a private serializer that 42 then duplicates or refactors. Either add an early shared "SQL literal rendering" atom to Issue 12 (it already owns "safe SQL atoms") or make 42's blocker relationship explicit and require 36 to consume it.

2. **Exact warning/error strings are repeated across issues** (`Result truncated: 64 MiB cache limit` appears verbatim in 28, 29, 43; `selected result has no tabular data to export` in 40, 43; terminal messages in 6, 40b). Consider a shared constants note in one issue each, or at least name one issue as the definition site so golden-string tests can't diverge.

3. **Issues 19/41 split is clean but fragile:** 19 implements the rendering logic, 41 extracts it. The 41 ACs are good; consider having 19 land the logic in a package-shaped seam from day one to make 41 nearly mechanical.

## D. Smaller correctness / precision nits

1. **Issue 12 / PRD REAL rule edge:** `strconv.ParseFloat` accepts hexadecimal floating-point (`0x1p2`) and forms like `1e400`→ErrRange (correctly falls through). Hex floats will bind as REAL under the stated rule. Probably intended, but worth one table-driven test row so it's a decision, not an accident.

2. **Issue 14's sentinel AC** correctly extends the dedup rule to `COUNT(*)`, but the PRD says selecting `COUNT(*)` "adds the sentinel directly"; ensure "cannot be added twice" means the popup hides/disables it after selection, not just rejects — the AC is ambiguous about which mechanism.

3. **Issue 16 LIMIT negative:** PRD says "Zero, malformed input, and overflow are invalid"; negatives are malformed under `-?[0-9]+` INTEGER parsing, but the issue adds "negative" as its own invalid class — fine, just keep the reason string consistent with Issue 17's focus-reason contract.

4. **Issue 21 `ORDER BY rowid` append** is correct per PRD, but its AC set doesn't include the interaction where the user's ORDER BY references an aggregate (no rowid append) — covered implicitly by "aggregate/grouped query" clause; fine, just ensure a test row exists.

5. **Issue 40 vs 40b overlap** is well-managed (explicit ownership notes). Same pattern could be imitated by 19/41 and 23/39b, which are slightly less crisp.

6. **HITL/AFK labels:** Issues 7, 20, 24, 50 are HITL largely due to manual review; that's appropriate, but the manual rendering matrix (80×24/100×30/160×50) is referenced in several issues with no single owner of the full manual pass. Issue 50 owns capability suites; consider adding the completed manual matrix checklist there or in a dedicated release issue.

## E. What's done well (keep)

- Stable-ID, finalization, and commit-boundary semantics are decomposed without contradiction across 20/22/29/30/38/39.
- The 5b extraction (cancellation infrastructure ahead of consumers) is the right de-risking move and its consumers (18, 24, 37, 38) correctly reference it.
- Terminal-state trio (40, 40b) handles empty-history fallback that most plans forget.
- Release gate (50) closes the real gap that per-issue capability tests were previously unowned as a suite, with traceability-table requirement.

## Summary

Blocking before task generation: fix the four dependency omissions (B1–B4), assign help-content ownership (A1), and resolve the literal-SQL duplication (C1). The rest are cheap AC amendments.
