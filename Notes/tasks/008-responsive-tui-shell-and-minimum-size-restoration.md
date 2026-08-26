# Tasks for #8: Responsive TUI shell and minimum-size restoration

Parent issue: #8
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Add the TUI dependencies and shell boundary

**Type**: CONFIG
**Output**: Vetted Bubble Tea, Lip Gloss, and applicable Bubbles dependencies are pinned; an `internal/ui` shell boundary builds.
**Depends on**: none

Review Issue #8 and the Language and stack and UI Module Design decisions in `Notes/PRD-sqloid.md`, vet mutually compatible Bubble Tea, Lip Gloss, and only the Bubbles components applicable to the shell, and pin exact versions through Go tooling without hand-editing generated checksums. Establish a minimal buildable `internal/ui` Bubble Tea model boundary with `Init`, `Update`, and `View` responsibilities while keeping database behavior behind the future Connection composition seam and keeping `cmd/sqloid` a thin process boundary. Do not implement responsive region arithmetic, builder behavior, or undersized-state semantics in this configuration task.

---

### 2. Specify exact layout arithmetic

**Type**: RED
**Output**: Failing model tests cover the one-row footer, builder `floor(H/3)` cap, border ownership, results allocation, and complete-row page area.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add pure table-driven model and layout tests in `internal/ui` for Issue #8 and the Resize/layout requirements in `Notes/PRD-sqloid.md`. At supported heights, require exactly one bottom global footer row, a builder desired height that includes its own border and padding and is capped at floor of one third of total height, and assignment of every remaining row to a results region that remains greater than half-height. Make each region exclusively own its borders with no shared or overlapping row, account within results for its owned border, status/count line, and frozen header, and assert the exact number of complete data rows available for paging. Cover representative 80×24, 100×30, and 160×50 dimensions and varying builder content, keeping the tests independent of terminal pixel rendering and free of production implementation guidance.

---

### 3. Implement the responsive TUI regions

**Type**: GREEN
**Output**: Pure layout and shell rendering tests pass at supported sizes.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure responsive layout calculation and shell composition in `internal/ui` using the pinned Bubble Tea and Lip Gloss dependencies. Reserve the exact footer row, calculate the builder's border-and-padding-inclusive desired height and floor-of-one-third cap, assign all other rows to the independently bordered results region, and derive its complete-row page area after fixed owned rows. Keep dimensions and border ownership explicit and testable, make overlays draw over regions without causing reflow, and preserve a composition seam for later builder, results, paging, and Connection work without embedding database behavior or implementing undersized suspension yet.

---

### 4. Specify focused scrolling and minimum-size suspension

**Type**: RED
**Output**: Failing tests cover complete focused-field visibility, exact hidden-state restoration, `terminal too small`, and Ctrl+W routing while undersized.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model tests in `internal/ui` for builder focus scrolling and the below-80×24 contract from Issue #8 and `Notes/PRD-sqloid.md`. Grow the builder beyond its cap and move focus in both directions to require the complete focused field or prompt, including its full multiline extent, to remain visible through internal scrolling. Populate nontrivial model context, focus, content, overlay-related state, and cancellable-request ownership before resizing below minimum; require the view to be exactly `terminal too small`, preserve hidden application state without exposing or mutating it, and restore the exact prior context and focus when supported dimensions return. While undersized, assert ordinary hidden-state keys are ignored and Ctrl+W routes only when hidden state owns active cancellable work, entering the same cancellation flow without direct database behavior in UI tests.

---

### 5. Implement builder scrolling and undersized restoration

**Type**: GREEN
**Output**: Focus scrolling, state preservation/restoration, ignored keys, and hidden cancellation routing tests pass.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement builder viewport scrolling and minimum-size suspension in `internal/ui`. Recompute the capped viewport so the complete focused field or prompt remains visible, and when width is below 80 or height below 24 render only the exact required message while retaining the full underlying model unchanged for exact resize restoration. Gate undersized input so normal hidden-context actions cannot leak through or mutate state, while routing Ctrl+W through the model's generic cancellation command only when an active cancellable request is owned by the hidden state; preserve the existing global quit seam for later full key-precedence work. On return to supported dimensions, restore context and focus exactly and then apply normal layout calculation without reconstructing or resetting application state.

---

### 6. Document responsive layout behavior

**Type**: DOCUMENT
**Output**: Wiki documentation records region arithmetic, minimum size, state suspension, and resize restoration.
**Depends on**: 5

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #8 `internal/ui` implementation and tests into the appropriate pages under `Notes/wiki`. Document the one-row footer, border ownership, builder desired-height accounting and floor-of-one-third cap, results-row allocation and complete page-area calculation, focused-field internal scrolling, the exact 80×24 minimum, the exact undersized message, hidden-state suspension, ignored input, conditional Ctrl+W routing, and exact context/focus restoration after resize. Cross-reference Issue #8 and the relevant Resize/layout, UI Module Design, Testing Decisions, and supported-environment sections of `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md`, and append the required dated ingest record to `Notes/wiki/log.md` without rewriting previous log entries.

---

### 7. Create the responsive-shell walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/008-07/code-walkthrough`.
**Depends on**: 6

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/008-07/code-walkthrough`. Demonstrate the `internal/ui` shell at 80×24, 100×30, and 160×50 with evidence for exact footer, builder cap, independent borders, results allocation, and complete-row page area; include long builder content and focus movement that keeps the full focused field visible. Resize a nontrivial state below minimum and back to prove the exact `terminal too small` view and exact restoration, then show ignored hidden keys and Ctrl+W routing with and without active cancellable work. Reference Issue #8 and `Notes/PRD-sqloid.md`, and place every generated walkthrough artifact under the approved directory.

---

### 8. Review the rendering matrix

**Type**: REVIEW
**Output**: Human verifies 80×24, 100×30, 160×50, below-minimum restoration, and cancellation while undersized.
**Depends on**: 7

Review the completed `internal/ui` shell, model tests, wiki updates, and walkthrough against Issue #8 and the manual rendering matrix in `Notes/PRD-sqloid.md`. At 80×24, 100×30, and 160×50, verify exact region arithmetic, one-row footer, builder cap and full focused-field scrolling, results-owned fixed rows and complete-row page area, greater-than-half results height, exclusive borders, and overlay non-reflow. With nontrivial hidden context and active cancellable work, resize below and above minimum, confirm only `terminal too small` is displayed, state and focus restore exactly, hidden input is ignored, and Ctrl+W enters normal cancellation without exposing or otherwise mutating hidden UI state before approving the issue.

---
