# Tasks for #79: Remove unused traversal limit fields

Parent issue: #79
Parent PRD: PRD-sqloid.md
**Blocked by issues**: #78
**Acceptance criteria**: AC1–AC3 → Task 1
**Manual verification**: Task 3 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Remove dead traversal Limit inputs

**Type**: REFACTOR  
**Output**: `TraversalFacts` and every producer/fixture omit HasLimit and Limit while the complete classification matrix and UI metadata tests remain unchanged.  
**Verification obligation**: The existing classifier truth table and behavioral tests pass unchanged; no classification semantics change.
**Supplemental checks**: Repository-wide build and test pass; any truth-table label change is a regression, not an expectation update.
**Depends on**: none

Begin only after Issue #78 is complete. This is the third change in the Issue #77 → #78 → #79 → #80 classifier sequence. Remove `HasLimit` and `Limit` from `history.TraversalFacts` in `internal/history/snapshot_classify.go`, from `SnapshotFacts` in `internal/ui/snapshot_metadata.go`, from `activeExportFacts` in `internal/ui/export.go`, and from every production initializer and test fixture found across `internal/history` and `internal/ui`. Update nearby comments so they accurately say that a successful known total already counts the complete SELECT including the user's Limit and therefore classification needs no raw builder Limit. Do not change any remaining traversal fact, endpoint derivation, no-clamping behavior, or complete/partial/truncated outcomes. Update the existing classifier truth table to compare limited known-total cases against equivalent unbounded fact sets and run focused history, finalization, and export metadata tests plus the repository's established Go verification command; treat any label change as a regression rather than adjusting expectations.

---

### 2. Document the limited-result classification boundary

**Type**: DOCUMENT  
**Output**: Wiki documentation states that known total already includes Limit, lists only consumed traversal facts, and records unchanged classification semantics.  
**Depends on**: 1

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #79 refactor and unchanged test evidence into the existing snapshot metadata, concurrent count, and export pages under `Notes/wiki`. Remove any documentation suggesting that `history.Classify` consumes a separate raw Limit, and state explicitly that the successful known total is for the complete SELECT including the user's Limit, so rows beyond that limited logical result are irrelevant. List the retained traversal inputs—count/page work completion, observed short final page, and count/cache inconsistency—and explain that complete, partial, truncated, endpoint, and contradiction outcomes are unchanged. Cross-reference Issues #33, #59's owning behavior, #77, #78, and #79 plus user stories 55, 56, and 59 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if necessary and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 3. Create the traversal-facts refactor walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/079-03/code-walkthrough`.  
**Depends on**: 2

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/079-03/code-walkthrough`, with the main file named `walkthrough.md`. Show the reduced `TraversalFacts` definition and each finalization/active-export producer with no `HasLimit` or `Limit`, then exercise paired limited known-total and equivalent unbounded classifier cases to demonstrate identical complete, partial, truncated, endpoint, and count/cache-inconsistency results. Include repository search evidence that no removed field remains, focused passing history/UI/export tests, and established verification output. Reference Issue #79 and `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---
