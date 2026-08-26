# Tasks for #49: Immutable result-export capture and warnings

Parent issue: #49
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify export targeting and instant capture

**Type**: RED
**Output**: Failing model tests cover idle active/historical tabular selection, immutable copy timing, unchanged active SELECT, terminal in-memory selection, and zero database work.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven capture tests in `internal/export` and scripted Ctrl+X model tests in `internal/ui`, using typed rows, deduplicated output names, and snapshot metadata from `internal/result`, for Issue #49 and the Result export scope, Export warnings, Global Key Precedence and Context/Action Matrix, Export/UI Module Design, and export Testing Decisions in `Notes/PRD-sqloid.md`. Cover an idle active tabular SELECT, each selected historical tabular result, and selected tabular snapshots in deletion, replacement, and outcome-unknown terminal workflows. Require Ctrl+X to deep-copy columns, logical positions, every typed value including BLOB bytes, and metadata synchronously before any picker command or later model mutation can run; mutate the cache, history source, selected entry, and original byte slices afterward and prove the captured payload is unchanged and remains in ascending logical-position order. Assert capture neither finalizes nor deactivates an active SELECT, changes its execution/request/generation/cache/viewport/history state, nor starts page, count, health-check, or other database work. In terminal cases, require targeting to follow the current in-memory result selection, including Ctrl+E/Y changes, without consulting the database. Keep this task test-only; eligibility rejection and warning presentation belong to later tasks.

---

### 2. Implement immutable export capture

**Type**: GREEN
**Output**: Ctrl+X captures the correct rows/metadata without finalizing or mutating active/history state.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal capture boundary across `internal/ui`, `internal/result`, and `internal/export`. Resolve Ctrl+X against the currently viewed active or stable selected historical/terminal tabular result, then synchronously construct an export-owned immutable value that deep-copies deduplicated columns, ascending logical row positions, typed cells and BLOB bytes, and all snapshot metadata before returning any picker-opening effect. Keep export input independent of live cache/history backing storage and carry metadata separately from serializable rows. Treat capture as an in-memory action: preserve the active SELECT lifetime and all execution, request, generation, cache, viewport, builder, focus, and history-selection state, and create no database command. Implement only enough to make Task 1 pass; do not add eligibility messages, warnings, CSV/JSON serialization, or picker behavior.

---

### 3. Specify request gating and non-tabular rejection

**Type**: RED
**Output**: Failing tests cover all pending requests, terminal/error/write/cancelled-before-rows/empty selections, exact shared rejection text, and no picker.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend scripted `internal/ui` model tests and pure `internal/export` eligibility tests to cover Ctrl+X during every request-bearing state: schema validation/refresh, estimate, SELECT first/later page, count-only work, cancellation settlement, write beginning/executing/rollback/commit, and any save/export request already pending. Require the existing generic pending gate to consume export with explanatory request feedback, unchanged selection/state, no capture, no picker, and no additional database work. At idle, cover ordinary and terminal selections that are errors, write summaries, outcome-unknown entries, cancelled-before-rows markers, other non-tabular entries, or empty/missing-backed selections. Require every non-tabular case to report exactly `selected result has no tabular data to export`, sourced from one shared Issue #49 definition in `internal/export`, and prove no picker or serializer command is created. Include cancelled/failed snapshots that retained rows as eligible tabular controls, and include zero-row SELECT snapshots with tabular columns as tabular controls so terminal outcome is not confused with data shape. Keep this task test-only and leave warning composition to Task 5.

---

### 4. Implement export eligibility checks

**Type**: GREEN
**Output**: Ordinary and terminal gating tests pass with the message defined only here.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one typed export-eligibility contract in `internal/export` and route Ctrl+X through it from the authoritative precedence/gating point in `internal/ui`. Reuse the generic any-request-pending restriction before capture, then accept only a currently backed tabular selection from ordinary active/history or terminal in-memory state. Define `selected result has no tabular data to export` once at this Issue #49 boundary and have all UI consumers reuse it for empty selections, errors, write/outcome summaries, cancelled-before-rows entries, and every other non-tabular result. Do not reject retained tabular rows merely because terminal outcome is cancelled/failed or the SELECT is empty but has tabular schema. On rejection preserve exact model, focus, active SELECT, and history/terminal selection, create no picker/serializer/database command, and do not duplicate terminal-specific branches or message literals. Implement only enough to make Task 3 pass.

---

### 5. Specify metadata warning presentation

**Type**: RED
**Output**: Failing tests cover complete/partial/truncated, cancelled/failed, invalid UTF, Issue 31 byte warning reuse, warning order, no serializer records, and exact cancel/complete restoration.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add a table-driven metadata-to-warning matrix in `internal/ui` with captured fixtures from `internal/result` and serializer-spy inputs in `internal/export`. Cover exclusive complete state, partial, truncated, partial-plus-truncated, row-cap and byte-cap truncation, success/cancelled/failed outcomes with reasons and failure positions, invalid-UTF replacement, and all truthful combinations. Require one deterministic presentation order: completeness state first, truncation details including Issue #31's shared exact `Result truncated: 64 MiB cache limit` definition, terminal-outcome information next, and invalid-UTF disclosure last; assert absent facts add no warning and shared definitions are referenced rather than copied. Require warnings to be visible before destination selection or confirmation and prove metadata never appears as a serializer row, column, object, property, or synthetic value. From active, historical, and terminal openers, cancel with Esc and complete successfully, then assert exact restoration of opener mode, focus, selection, viewport, builder, active SELECT identity/lifetime/cache, and terminal state; captured data remains stable throughout and no database work occurs. Keep this task test-only and do not specify CSV/JSON bytes owned by Issues #50 and #51.

---

### 6. Implement export warning flow

**Type**: GREEN
**Output**: Warnings appear before destination selection while serializer input and opener state remain correct.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement warning derivation and the pre-destination export flow in `internal/ui` from immutable typed metadata carried separately by `internal/result` and `internal/export`. Render complete/partial/truncated, row/byte truncation, cancelled/failed, and invalid-UTF information in the tested canonical order, reusing Issue #31's byte-cap warning definition exactly. Keep the export-owned row/name/value payload free of warning records and metadata fields when handing it to later CSV/JSON serializers. Suspend the exact opener state while warning and destination steps are active; on Esc or successful completion restore the same focus, active/history/terminal selection, viewport, builder, and active SELECT lifecycle without finalization, refetch, or mutation. Ensure later changes to live result state cannot alter the captured copy, and implement only enough to make Task 5 pass without taking ownership of format bytes or filesystem persistence.

---

### 7. Document immutable export capture

**Type**: DOCUMENT
**Output**: Wiki documentation records targeting, capture timing, gating, terminal behavior, warning definitions, and data/metadata separation.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #49 implementation and tests from `internal/export`, `internal/result`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document Ctrl+X targeting for idle active, historical, and terminal in-memory selections; synchronous deep-copy timing before picker work; ascending positions and exact typed/BLOB identity; and the invariant that capture does not finalize or mutate an active SELECT or start database work. Record all-request gating, tabular eligibility, the single shared exact rejection `selected result has no tabular data to export`, and no-picker behavior for empty/error/write/outcome-unknown/cancelled-before-rows entries while retained-row cancelled/failed snapshots remain exportable. Define warning combinations and order for complete/partial/truncated, cap details, cancelled/failed, and invalid UTF; identify Issue #31 as owner of `Result truncated: 64 MiB cache limit`; and explain that metadata drives UI only and never serializer records or properties. Record exact cancel/complete opener restoration, cross-reference Issues #31, #33, #34, #36, #45, #46, and #49 plus the SELECT lifecycle, Cache and snapshot invariant, Result export scope, Export warnings, context/action matrix, Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the export-capture walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/049-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/049-08/code-walkthrough`. Demonstrate Ctrl+X from an idle active SELECT, selected historical snapshots, and deletion/replacement/outcome-unknown terminal selections; mutate live rows, metadata, BLOB sources, and selections after capture to prove the export copy is immutable and ascending while the active SELECT remains unchanged and unfinalized. Exercise every pending-request gate and non-tabular/empty/error/write/cancelled-before-rows case, showing the exact shared rejection and no picker or database work, then contrast retained-row cancelled/failed tabular exports. Present complete, partial, truncated, partial-plus-truncated, byte-cap, cancelled/failed, and invalid-UTF warning combinations in canonical order before destination selection, prove serializer spies receive no warning records/properties, and capture exact active/history/terminal restoration after cancel and completion. Reference Issue #49 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 9. Review export capture

**Type**: REVIEW
**Output**: Human exercises active, historical, terminal, warned, non-tabular, cancel, and complete paths.
**Depends on**: 8

Review immutable capture and eligibility in `internal/export`, typed rows/metadata in `internal/result`, UI gating/warnings/restoration in `internal/ui`, wiki updates, and `Notes/walkthroughs/049-08/code-walkthrough` against Issue #49. Manually export an idle active result, several historical selections, and tabular terminal selections; alter live state immediately after Ctrl+X and verify the captured rows, names, BLOB bytes, order, and metadata do not change, no database request starts, and the active SELECT remains live. Attempt export during representative pending requests and from empty, error, write, outcome-unknown, and cancelled-before-rows selections, confirming exact shared feedback and no picker. Exercise every warning axis and combination, verify Issue #31 wording and order outside serializer data, then cancel and complete from active, historical, and terminal openers and confirm exact state/focus restoration before approving the issue.

---
