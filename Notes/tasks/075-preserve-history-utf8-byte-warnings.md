# Tasks for #75: Preserve invalid-UTF and byte-cap warnings in result history

Parent issue: #75
Parent PRD: PRD-sqloid.md
**Blocked by issues**: #72, #74
**Coordinates with**: #76; #75 lands first on the shared `Finalization`/`SnapshotMetadata`/`projectHistoryEntry` seam.
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify warning metadata through history round trips

**Type**: RED  
**Output**: Failing UI lifecycle tests cover invalid-UTF and byte-cap metadata from settlement through finalization, historical projection, warning display/export, immutable rows, and serializer exclusion.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issues #72 and #74 are complete. Land this issue before Issue #76; both extend the shared `Finalization`/`SnapshotMetadata`/`projectHistoryEntry` metadata seam. Add focused lifecycle tests in `internal/ui` beside `snapshot_finalize_test.go`, `result_history_browse_test.go`, `bytecap_test.go`, and `export_warnings_test.go`, using the existing `history.ResultEntry`, `history.SnapshotMetadata`, `ResultView`, and export-capture fixtures. Drive an accepted active page with malformed TEXT separately from a page/cache with persistent `TruncatedByByteCap`, then finalize via `appendFinalizedResultEntry`, enter result history, project at multiple terminal page sizes, render, and open export. Require active `Page.InvalidUTF` to become immutable snapshot `InvalidUTF`; require historical projection to restore both `Page.InvalidUTF` and `ResultView.ByteTruncated`; and require the existing shared UTF and 64 MiB warnings to appear in browsing and before export destination selection. Mutate live pages, projected views, source BLOBs, and cache state after finalization and prove stored rows/metadata remain unchanged. Add serializer-spy or existing CSV/JSON capture assertions proving neither warning becomes a row, column, object property, or data value. Keep this task test-only.

---

### 2. Preserve warnings in finalization and projection

**Type**: GREEN  
**Output**: Finalized history and reprojected views preserve invalid-UTF and byte-cap warnings without changing immutable data or serialized records.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimum changes across `internal/ui/active_select.go`, `internal/ui/snapshot_metadata.go`, and `internal/ui/result_history.go`, with narrow model changes only where required by the tests. At finalization, source invalid-UTF truth from the accepted active page and pass it through `Finalization` into immutable `history.SnapshotMetadata`; continue sourcing persistent byte-cap truth from the authoritative cache/result state without re-deriving it from current payload size. When `projectHistoryEntry` reconstructs a tabular historical `ResultView`, restore `Metadata.InvalidUTF` onto the new `result.Page` and `Metadata.TruncatedByByteCap` onto `ResultView.ByteTruncated`, preserving offset, rows, columns, BLOB copies, and current-height reslicing. Reuse `result.UTFWarning` and `result.ByteCapWarning` at existing presentation/export boundaries and do not inject metadata into `internal/export` serializer records. Implement only enough to make Task 1 pass, then run focused history, UI, CSV, and JSON tests plus the established Go verification command.

---

### 3. Document historical warning preservation

**Type**: DOCUMENT  
**Output**: Wiki documentation records warning ownership, finalization/projection round trips, immutable behavior, and exclusion from CSV/JSON data.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #75 implementation and tests into the existing snapshot metadata, result-history browsing, byte-cap, shared typed-result, and immutable export pages under `Notes/wiki`. Document how accepted active-page invalid-UTF metadata and persistent cache byte-cap metadata enter one immutable finalized snapshot, how local historical projection restores `Page.InvalidUTF` and `ResultView.ByteTruncated` at any terminal size, and how browsing/export presents the same shared warning definitions that were true during execution. State that warning facts do not alter rows or logical positions and never become CSV records, JSON properties, or synthetic values. Cross-reference Issues #31, #33, #36, #49, #72, #74, and #75 plus user stories 55, 64, and 70 in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if necessary and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting history.

---

### 4. Create the historical-warning walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/075-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/075-04/code-walkthrough`, with the main file named `walkthrough.md`. Separately execute/finalize a malformed-TEXT result and a byte-cap-truncated result, inspect their immutable snapshot metadata, browse each at changed terminal sizes, and show the restored shared warning in the historical result and pre-destination export flow. Mutate live and projected sources to prove rows, BLOB bytes, positions, and warning facts remain immutable, and inspect CSV/JSON serializer input/output to prove no warning record or property is emitted. Include focused passing test output, reference Issue #75 and `Notes/PRD-sqloid.md`, and place all showboat artifacts under the approved directory.

---
