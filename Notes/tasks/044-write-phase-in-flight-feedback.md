# Tasks for #44: Write-phase in-flight feedback

Parent issue: #44
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify every write-phase label

**Type**: RED
**Output**: Failing rendering tests require exact beginning/executing, estimate, commit, rollback, and cancellation feedback without changing read labels.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused phase-to-view tests in `internal/ui`, driven by typed lifecycle messages from the controllable `internal/connection` fake, for Issue #44 and the Writes and commit boundary, Global Key Precedence and Context/Action Matrix, UI/Connection Module Design, and baseline/high-risk Testing Decisions in `Notes/PRD-sqloid.md`. Hold beginning and executing separately and require exact `Running…`; hold estimate and require exact `Estimating matching target rows…`; hold committing and rollback cleanup and require exact `Committing…` and `Rolling back…`; request cancellation from every cancellable write phase and require exact `cancelling…` until settlement. Assert labels remain visible through permitted local updates, follow the current execution/request identity, cannot be overwritten by stale/duplicate phase messages, and map from typed phase state rather than inferred command or arbitrary strings. Add regression assertions that Issue #27's SELECT first-page `Running…`, count, page-loading, count-unavailable, and read cancellation labels are unchanged. Keep this task test-only and do not duplicate transaction orchestration from Issues #41–#43.

---

### 2. Implement write-phase presentation mapping

**Type**: GREEN
**Output**: Exact label tests pass from typed write phases.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one authoritative typed write-phase presentation mapping in `internal/ui` for estimate, beginning, executing, cancellation-requested, rollback cleanup, and committing state emitted by `internal/connection`. Render the exact required labels, preserve them through responsive local interaction, and apply execution/request identity guards so stale or duplicate messages cannot regress or replace current feedback. Keep presentation mapping separate from cancellability and action gating, and reuse the existing Issue #27 read-state renderer without renaming, branching through, or otherwise changing SELECT/page/count labels. Do not move transaction or cancellation decisions out of `internal/connection`; implement only enough to make Task 1 pass.

---

### 3. Specify write-request action gating

**Type**: RED
**Output**: Failing model tests cover Enter, phase-appropriate Ctrl+W hints/boundary feedback, histories, save/export, permitted local interaction, quit, and unchanged request count.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven `(model, msg) → (model, cmd)` tests in `internal/ui`, using `internal/connection` request counters and `internal/history` state, for the generic request-in-flight row of the PRD's Global Key Precedence and Context/Action Matrix across estimating, beginning, executing, cancellation-requested, rollback-cleanup, and committing write phases. In each phase require Enter to be consumed with phase-appropriate Ctrl+W guidance and no stacked execution; reject Ctrl+P/N, Ctrl+E/Y, Ctrl+S, and Ctrl+X with explanatory feedback and unchanged query/result selection; preserve allowed responsive local interaction and help according to context; and let q/Ctrl+C open the shared quit confirmation. For cancellable estimate/beginning/executing, require Ctrl+W routing and `cancelling…`; for rollback/commit, require no interrupt and exact `Commit in progress; cancellation is no longer available`. Assert database request count remains unchanged for every blocked/local action and cover terminal, quit-confirmation, top-overlay, focused-input, request-pending, and base precedence. Keep this task test-only and prove behavior uses Issue #27's generic pending gate rather than a parallel write-only gate or label inspection.

---

### 4. Integrate write phases with generic gating

**Type**: GREEN
**Output**: Write pending-state tests pass by reusing Issue 27 without duplicating read integration.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Feed typed pending/cancellable state for estimate and phased writes from `internal/connection` into the existing generic request-in-flight action gate in `internal/ui` established by Issue #27. Reuse its authoritative precedence point and blocked Enter, histories, save, and export paths; provide write-phase Ctrl+W hints, route cancellation only for estimating/beginning/executing, return exact post-boundary feedback for rollback/committing, preserve permitted local interaction and shared quit confirmation, and dispatch no stacked request. Keep gating independent from rendered labels and preserve current `internal/history` selection/content on rejected actions. Do not copy the read integration, add a second write-specific precedence ladder, or alter SELECT/page/count behavior; implement only enough to make Task 3 pass.

---

### 5. Document write-phase feedback

**Type**: DOCUMENT
**Output**: Wiki documentation records labels, action restrictions, cancellability differences, and ownership split.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #44 implementation and tests from `internal/connection`, `internal/history`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Record exact phase mapping: beginning/executing `Running…`, estimate `Estimating matching target rows…`, committing `Committing…`, rollback cleanup `Rolling back…`, and requested cancellation `cancelling…`. Document generic pending restrictions for Enter, query/result history, save, and export; phase-appropriate Ctrl+W hint/boundary feedback; permitted local responsiveness and help; shared quit confirmation; unchanged request counts and history state; and terminal/overlay/input/pending/base precedence. Explain that Issue #27 owns the generic gate and read integration, Issues #41–#43 own estimate/transaction/cancellation/commit lifecycle semantics in `internal/connection`, and Issue #44 owns write-phase presentation and feeding typed write state into the shared `internal/ui` gate without changing read labels. Cross-reference Issues #27 and #41–#44 and the Writes and commit boundary, Global Key Precedence and Context/Action Matrix, UI/Connection/History Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the write-feedback walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/044-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/044-06/code-walkthrough`. Hold deterministic estimating, beginning, executing, cancellation-requested, rollback-cleanup, and committing phases and capture the exact required label in each. During every phase exercise Enter, query and result history, save/export, permitted local interaction/help, q/Ctrl+C, and Ctrl+W; show explanatory rejections, unchanged histories and request counts, shared quit confirmation, cancellable-phase `cancelling…`, and exact post-boundary feedback. Include stale-message identity checks, evidence that generic gating does not inspect labels, and read-request regression evidence proving Issue #27's SELECT/page/count labels and behavior remain unchanged. Reference Issue #44 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review write feedback

**Type**: REVIEW
**Output**: Human holds each phase and verifies labels, Enter, history, save/export, quit, and cancellation behavior.
**Depends on**: 6

Review typed phases from `internal/connection`, unchanged history state in `internal/history`, phase rendering and generic gating in `internal/ui`, wiki updates, and `Notes/walkthroughs/044-06/code-walkthrough` against Issue #44. Manually hold estimate, beginning, executing, cancellation-requested, rollback-cleanup, and committing states; verify each exact label while exercising Enter, both histories, save/export, local interaction/help, quit, and Ctrl+W. Confirm blocked actions explain themselves and start no request, histories remain unchanged, quit uses the shared confirmation/cleanup path, cancellation is available only before the boundary, and rollback/commit gives exact boundary feedback. Re-run held SELECT/page/count phases to ensure read labels and gating did not change before approving the issue.

---
