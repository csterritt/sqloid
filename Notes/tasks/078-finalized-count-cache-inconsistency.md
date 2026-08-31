# Tasks for #78: Record count/cache inconsistency during finalization

Parent issue: #78
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify finalized count/cache contradiction propagation

**Type**: RED  
**Output**: Failing finalization tests prove successful count below retained end sets CountCacheInconsistent, preserves both facts, and never classifies complete, with boundary/status controls.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issue #77 is complete, as the second change in the Issue #77 → #78 → #79 → #80 classifier sequence. Add focused tests beside `internal/ui/snapshot_finalize_test.go` and the exactly-once finalization tests for `appendFinalizedResultEntry`. Build active caches whose retained end is greater than, equal to, and less than a successful `result.CountState.Total`, finalize through the production seam, and inspect the stored `history.ResultEntry`, `SnapshotMetadata`, `TraversalFacts`-driven completeness, and export selection. Require only the greater-than case to propagate `CountCacheInconsistent=true`; preserve the original known total and retained start/end without clamping either; and require completeness never to be complete. Add pending, unavailable, failed, and cancelled count controls with the same cache range and require no successful-count contradiction flag. Include empty-cache and exact-boundary controls and prove finalization remains immutable and exactly once. Keep this task test-only.

---

### 2. Record the contradiction before snapshot classification

**Type**: GREEN  
**Output**: Finalization derives CountCacheInconsistent from successful count versus retained end and preserves unclamped count/cache facts in history and export metadata.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Modify `appendFinalizedResultEntry` in `internal/ui/active_select.go` so it derives count/cache inconsistency from the same authoritative cache snapshot used for rows and range, before calling `SnapshotFacts`. When and only when `m.countState.Status == result.CountSuccess`, a retained end exists, and that end exceeds `m.countState.Total`, set `Finalization.CountCacheInconsistent`; do not rewrite the total, retained range, endpoint observations, rows, or count state. Let `internal/ui/snapshot_metadata.go` pass the flag through the existing `history.TraversalFacts` field and let the corrected `history.Classify` behavior from Issue #77 reject complete naturally. Preserve equal/lower boundaries, non-success count statuses, immutable entry copying, and active/finalized export metadata. Implement only enough to make Task 1 pass, then run focused `internal/ui` and `internal/history` tests plus established Go verification.

---

### 3. Document finalized count/cache contradictions

**Type**: DOCUMENT  
**Output**: Wiki documentation records the successful-count contradiction rule, no-clamping invariant, classification effect, controls, and finalization timing.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #78 implementation and tests into the existing concurrent count, active SELECT lifetime, snapshot metadata, result-history, and export pages under `Notes/wiki`. Define `CountCacheInconsistent` exactly as a successful limited-result count total below the authoritative retained cache end at finalization; state that equality/lower retained ends and pending/unavailable/failed/cancelled counts do not set it. Explain that count and cache are independent autocommit facts, both are preserved without clamping, and the contradiction prevents a complete label while immutable history/export retains the original values. Cross-reference Issues #24, #33, #34, #49, #77, and #78 plus user stories 55, 59, and 61 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if necessary and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the count/cache contradiction walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/078-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/078-04/code-walkthrough`, with the main file named `walkthrough.md`. Finalize an active result whose successful count total is below the retained cache end, inspect the propagated `CountCacheInconsistent`, original known total and range, stored history/export metadata, and non-complete classification. Contrast equal and lower retained-end boundaries plus pending, unavailable, failed, and cancelled count states to show the flag is not invented. Include exactly-once/immutability evidence and focused passing tests, reference Issue #78 and `Notes/PRD-sqloid.md`, and keep every generated artifact under the approved directory.

---
