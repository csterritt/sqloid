# Tasks for #80: Classify active exports from complete endpoint facts

Parent issue: #80
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify active-export/finalization fact parity

**Type**: RED  
**Output**: Failing parity tests cover known totals, observed short pages, missing endpoints, eviction, contradictions, and identical active/finalized completeness labels.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issues #73 and #79 are complete, as the final change in the Issue #77 → #78 → #79 → #80 classifier sequence. Add a table-driven parity suite in the closest `internal/ui` export and finalization test files, following `snapshot_finalize_test.go`, export warning fixtures, and the production `activeExportFacts`/`appendFinalizedResultEntry` seams. For one active model state, capture active export facts, then finalize a copy and compare retained range, known total, `ReachedLow`, `ReachedHigh`, `ObservedShortFinalPage`, count/cache inconsistency, eviction facts, and completeness labels. Cover a fully retained successful limited count, count unavailable with an accepted short/empty final page, missing low, missing high, unfinished work, row- and byte-cap eviction, known rows beyond retention, and successful count below retained end. Require active pre-picker warnings and finalized export labels to agree, preserve contradictory total/range without clamping, and prove export itself does not finalize or mutate the active SELECT. Keep this task test-only.

---

### 2. Share authoritative snapshot traversal facts

**Type**: GREEN  
**Output**: Active export and finalization derive equivalent endpoint/traversal facts through one helper and produce matching truthful completeness and warnings.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Refactor the minimum production code in `internal/ui/export.go`, `internal/ui/active_select.go`, and `internal/ui/snapshot_metadata.go` so active export and finalization call one shared helper for authoritative cache/count/page facts. Derive `ReachedLow` from the retained low boundary and truthful eviction evidence, derive `ReachedHigh` from a successful limited-result count relative to the retained range or the accepted `pageExhausted` observation from Issue #73, and pass `pageExhausted` as `TraversalFacts.ObservedShortFinalPage`. Derive successful count/cache inconsistency exactly as Issue #78 specifies, preserving independent known total and retained end without clamping. Carry row/byte eviction and invalid-UTF metadata unchanged, keep terminal outcome absent for active capture and supplied only at finalization, and ensure active export remains an in-memory nonfinalizing action. Consume the reduced `TraversalFacts` from Issue #79 with no raw Limit fields. Implement only enough to make Task 1 pass, then run focused history/UI/export parity and warning tests plus established Go verification.

---

### 3. Document authoritative active-export facts

**Type**: DOCUMENT  
**Output**: Wiki documentation records the shared helper, endpoint sources, short-page and contradiction handling, active/finalized parity, and nonfinalizing export behavior.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #80 implementation and parity tests into the existing immutable export, snapshot metadata, active SELECT lifetime, first SELECT, and concurrent count pages under `Notes/wiki`. Document the one shared active/finalized fact derivation seam: retained-cache low evidence, successful limited-result count or observed short/empty page high evidence, count/page work state, cap eviction, invalid UTF, and count/cache contradiction without clamping. Explain that identical active state produces equivalent endpoints and completeness before destination selection and after finalization, while terminal outcome exists only on the finalized snapshot and Ctrl+X does not finalize or mutate the active SELECT. Record missing-endpoint and eviction warning outcomes and the absence of raw Limit fields after Issue #79. Cross-reference Issues #49, #73, #77, #78, #79, and #80 plus user stories 55, 56, and 70 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if necessary and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the active-export parity walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/080-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/080-04/code-walkthrough`, with the main file named `walkthrough.md`. For each parity-table state, capture an active export and then finalize an equivalent model: fully retained known limited count, count unavailable with accepted short/empty page, missing low, missing high, row/byte eviction, unfinished work, and contradictory count below retained end. Show matching endpoint/traversal facts, completeness labels, and pre-picker/finalized warnings while preserving unclamped totals/ranges; also prove active export leaves execution identity, cache, viewport, and lifetime unchanged. Include focused passing tests and shared-helper evidence, reference Issue #80 and `Notes/PRD-sqloid.md`, and place all generated artifacts under the approved directory.

---
