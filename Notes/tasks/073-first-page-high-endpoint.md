# Tasks for #73: Establish the high endpoint from a short first page

Parent issue: #73
Parent PRD: PRD-sqloid.md
**Blocked by issues**: #72
**Acceptance criteria**: AC1–AC4 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify first-page high-endpoint tracking

**Type**: RED  
**Output**: Failing UI lifecycle tests cover retained request size, accepted empty/short/full settlements, stale identities, paging suppression, and active/finalized completeness facts.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issue #72 is complete. Preserve its settlement metadata at the shared `applySelectSettled` seam. Add focused table-driven and scripted tests in `internal/ui/first_select_test.go` and the closest existing paging, snapshot-finalization, and export test files, following the request-identity fixtures in `internal/ui/first_select.go`, `internal/ui/paging.go`, and `internal/ui/snapshot_finalize_test.go`. Retain the exact requested first-page size with the dispatched request identity, then cover accepted zero-row, short, and exactly-full responses; require only zero/short responses to set `pageExhausted` and feed `ObservedShortFinalPage` into active export and `appendFinalizedResultEntry`. Prove an accepted empty or short response makes forward Page Down a no-op instead of requesting the same or any later offset, and that count-unavailable empty/short fully retained results classify complete in both active and finalized paths. Replay stale execution, request, and viewport-generation settlements with short data and require no endpoint, cache, result, or paging mutation. Keep this task test-only and do not change production behavior.

---

### 2. Implement accepted first-page endpoint observation

**Type**: GREEN  
**Output**: Current empty/short first pages establish the high endpoint, prevent duplicate paging, and produce truthful active/finalized completeness while full or stale pages do not.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimum production changes in `internal/ui/first_select.go` and the existing request-identity/model seams that accept `SelectSettledMsg`, with only the smallest necessary adjustments in `internal/ui/paging.go`, `internal/ui/export.go`, and `internal/ui/active_select.go`. Capture the layout-derived requested first-page size at dispatch and bind it to the same execution/request/generation identity as the response; compare row count only after all current-response guards accept the settlement. Set `pageExhausted` for fewer rows than that retained size, including zero, leave it false for exactly-full pages, and ensure stale or cancelled settlements remain inert. Reuse `pageExhausted` as `ObservedShortFinalPage` for active export and finalization, and reuse the existing paging high-boundary guard so Page Down after any exhausted first page, short or empty, issues no duplicate request. Preserve cache rows, independent count facts, no-clamping semantics, and active SELECT lifetime. Implement only enough to make Task 1 pass and run the focused `internal/ui` tests plus the repository's established Go verification command.

---

### 3. Document first-page endpoint evidence

**Type**: DOCUMENT  
**Output**: Wiki documentation records first-page request-size identity, short/empty endpoint rules, stale-response behavior, paging suppression, and export/finalization effects.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #73 implementation and tests into the appropriate existing pages under `Notes/wiki`, especially the first SELECT, serialized paging, snapshot metadata, and immutable export documentation. Explain that the requested first-page size is retained with execution/request/generation identity; only an accepted response with fewer rows, including zero, establishes the observed high endpoint; exactly-full and stale responses do not; and an accepted short or empty response prevents Page Down from issuing another forward request, including the short-but-nonempty case. Record how `pageExhausted` supplies `ObservedShortFinalPage` consistently to active export and finalization so a fully retained count-unavailable short/empty result can be complete. Cross-reference Issue #73 and the SELECT lifecycle, cache and snapshot invariant, and user stories 51, 55, and 56 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if page membership changes and append the required dated entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the first-page endpoint walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/073-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/073-04/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate the retained requested first-page size and accepted empty, short, and exactly-full settlements with count unavailable; show that empty and short-nonempty pages set `pageExhausted` and both suppress any forward Page Down work, and produce matching complete active-export and finalized labels, while a full page leaves the remainder unknown. Replay stale execution, request, and generation responses to prove they cannot alter endpoints or cache state. Include focused passing test output, reference Issue #73 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
