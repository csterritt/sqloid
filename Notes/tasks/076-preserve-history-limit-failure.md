# Tasks for #76: Preserve typed over-limit failures in result history

Parent issue: #76
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify typed limit-failure history round trips

**Type**: RED  
**Output**: Failing lifecycle tests cover page/value failures, one-based positions, exact historical messages, retained leading rows, immutable export metadata, and the unset control.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issues #71 and #72 are complete. Add table-driven lifecycle tests in `internal/ui` beside `bytecap_test.go`, `snapshot_finalize_test.go`, and `result_history_browse_test.go`, with immutable entry assertions in `internal/history` where the typed metadata is stored. For both `result.KindPage` and `result.KindValue`, settle/finalize an active SELECT carrying a `result.LimitFailure` at representative one-based logical positions after complete leading rows have entered the contiguous cache. Require the finalized snapshot to preserve kind and position as typed facts, historical projection at smaller and larger terminal heights to restore `ResultView.LimitFailure`, and rendering to reuse the exact `result.LimitFailure.Error` line for page versus value failures. Assert retained leading rows, absolute positions, typed cells, and BLOB bytes remain unchanged and export capture keeps their immutable snapshot metadata without a partial row. Include source/projection mutation checks and a no-failure control that remains unset through finalization and projection. Keep this task test-only and do not duplicate failure message literals in UI production code.

---

### 2. Persist and restore typed limit failures

**Type**: GREEN  
**Output**: Page/value limit failures round-trip through immutable history and render exactly with retained leading rows and no synthesized failures.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the smallest typed metadata extension across `internal/history/snapshot.go`, `internal/history/result_entry.go`, and the finalization/projection seams in `internal/ui/active_select.go`, `internal/ui/snapshot_metadata.go`, and `internal/ui/result_history.go`. Preserve a copied `result.LimitFailure` kind and one-based position from the accepted active `ResultView` through finalization, immutable store append/lookup, export capture metadata, and local historical projection; use value semantics or defensive copying so later mutation cannot alter the snapshot. Restore the typed failure onto `ResultView.LimitFailure` and let `internal/ui/results_grid.go` continue rendering solely through `result.LimitFailure.Error`. Keep all complete leading cache rows and their absolute retained range, never manufacture a partial row, and leave metadata absent when no failure occurred. Do not conflate typed page/value failure with byte-cap eviction disclosure or terminal outcome. Implement only enough to make Task 1 pass, then run focused `internal/result`, `internal/history`, `internal/ui`, and export tests plus established Go verification.

---

### 3. Document historical over-limit failures

**Type**: DOCUMENT  
**Output**: Wiki documentation records typed failure kind/position, exact shared rendering, retained-row behavior, immutability, and no-failure semantics.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #76 implementation and tests into the existing byte-cap/oversized-value, snapshot metadata, result-history browsing, and immutable export pages under `Notes/wiki`. Document the distinct typed page and value failures, their one-based logical position, immutable finalization/store/projection path, and exclusive ownership of exact messages by `result.LimitFailure.Error`. Explain that complete leading rows and absolute positions remain available to history/export without a partial row, that this failure fact is separate from byte-cap truncation and terminal outcome, and that a snapshot with no failure remains unset. Cross-reference Issues #31, #33, #36, #49, #71, #72, and #76 plus user stories 55, 64, and 89 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only when page membership changes and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the limit-failure history walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/076-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/076-04/code-walkthrough`, with the main file named `walkthrough.md`. Produce separate page-limit and value-limit settlements with representative row N values, finalize each, inspect the typed immutable metadata, and browse at different terminal heights to show the exact shared failure lines and complete retained leading rows. Demonstrate absolute positions and BLOB bytes remain stable through history and export capture, mutation cannot alter the stored failure, and a no-failure snapshot projects without synthesizing one. Include focused passing tests, reference Issue #76 and `Notes/PRD-sqloid.md`, and keep every generated artifact in the approved directory.

---
