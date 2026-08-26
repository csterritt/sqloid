# Tasks for #29: Whole-column horizontal scrolling

Parent issue: #29
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify visible-column layout calculations

**Type**: RED
**Output**: Failing pure tests cover first-visible index, width recomputation, oversized-column caps/ellipsis, no intra-cell scrolling, boundaries, and resize clamping.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused table-driven pure layout tests in `internal/ui` for the result grid contract in Issue #29 and the UI Behavior, Module Design, Testing Decisions, and exact-horizontal-units sections of `Notes/PRD-sqloid.md`. Drive layout from deduplicated output columns, rendered cell values, available grid width, and a first-visible output-column index; cover narrow and wide terminals, Unicode display widths, multiple columns fitting, exact-fit boundaries, no room for another complete column, and indexes at both ends. Require every layout pass to recompute visible columns and widths starting at the selected index, cap and ellipsize a single oversized column to the available cell area, and expose no character or byte offset that could permit intra-cell horizontal scrolling. Add resize cases that preserve a valid first-visible index and clamp an invalid one after column or width changes. Keep this task test-only and separate horizontal arithmetic from Bubble Tea key handling.

---

### 2. Implement column-indexed result layout

**Type**: GREEN
**Output**: Pure horizontal layout tests pass for narrow, wide, Unicode, and oversized columns.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the pure result-grid layout seam in `internal/ui` needed by Task 1. Make the first-visible output-column index the only horizontal position, derive each visible column and its width afresh from the current viewport and rendered display widths, and cap an oversized first column so its header and cells ellipsize within the grid rather than gaining an intra-cell offset. Preserve the frozen deduplicated header and existing vertical row allocation, return stable boundary information for navigation, and normalize resize state by preserving then clamping the index to the available output columns. Keep terminal rendering concerns at the UI boundary and implement only enough production behavior to pass the pure layout tests without adding key dispatch assigned to Task 4.

---

### 3. Specify horizontal key behavior

**Type**: RED
**Output**: Failing model tests require Shift+Page and `,`/`.` to move exactly one whole column, no-op at boundaries, and remain local while requests run.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted `(model, msg) → (model, cmd)` tests in `internal/ui` for all four Issue #29 bindings under the authoritative Global Key Precedence and Context/Action Matrix in `Notes/PRD-sqloid.md`. Require Shift+Page Down and `.` to increment the first-visible output-column index by exactly one, and Shift+Page Up and `,` to decrement it by exactly one, regardless of how many columns fit. Assert no-op behavior at the first and last columns, no database command or request-count change for any accepted move, and equivalent behavior while SELECT first-page, later-page, or count work is in flight through the controllable `internal/connection` fake. Cover higher-precedence terminal, quit-confirmation, overlay, focused input/search, and too-small contexts so keys are consumed according to their owning context and do not leak into result scrolling. Keep this task test-only and reuse Task 1's pure layout assertions for the resulting recomputation.

---

### 4. Integrate whole-column scrolling

**Type**: GREEN
**Output**: All four bindings and preserved/clamped resize-state tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire the four horizontal bindings into the base result context in `internal/ui`, changing only the first-visible output-column index and invoking the pure recomputation from Task 2. Move one index per accepted key, consume boundary presses without dispatching work, and keep horizontal movement local while any SELECT page or count request is pending. Preserve the established terminal, quit, overlay, focused-input, request-pending, and too-small precedence rules. On resize, retain the prior index when valid and clamp it to the nearest valid output-column boundary otherwise, including empty and single-column results, while leaving vertical request orchestration to Issue #32. Implement only enough to make Tasks 1 and 3 pass without introducing intra-cell scroll state or changing `internal/connection` behavior.

---

### 5. Document horizontal scrolling

**Type**: DOCUMENT
**Output**: Wiki documentation records units, bindings, width behavior, oversized cells, boundaries, and resize.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #29 implementation and tests from `internal/ui` into the appropriate pages under `Notes/wiki`. Document that horizontal position is a first-visible output-column index; list Shift+Page Down and `.`, Shift+Page Up and `,`; state that each accepted press moves exactly one whole column, boundaries are no-ops, and movement remains local during database requests. Record width recomputation from the new first column, Unicode display-width handling, oversized-column capping and ellipsis without intra-cell scrolling, and resize preservation followed by clamping. Cross-reference Issue #29 and the UI Behavior, SELECT lifecycle, Global Key Precedence and Context/Action Matrix, Keybinding portability, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the horizontal-scroll walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/029-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/029-06/code-walkthrough`. Demonstrate all four bindings against narrow, wide, Unicode, and oversized result columns; capture one-index movement, width recomputation, both boundary no-ops, and capped ellipsis with no intra-cell scroll state. Show that horizontal movement dispatches no database request while page or count work is pending, then resize at the first, middle, and last visible-column indexes to demonstrate preservation and clamping. Reference Issue #29 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review horizontal overflow

**Type**: REVIEW
**Output**: Human verifies all bindings, boundaries, oversized columns, and resize preservation.
**Depends on**: 6

Review the `internal/ui` layout and key handling, wiki updates, and `Notes/walkthroughs/029-06/code-walkthrough` against Issue #29. At the supported terminal sizes, use narrow, wide, Unicode, exact-fit, and oversized-column fixtures; exercise Shift+Page Up, Shift+Page Down, `,`, and `.` from the first, middle, and last columns. Confirm one whole-column movement per press, no-op boundaries, recomputed widths, capped ellipsis with no intra-cell scrolling, local behavior during pending page/count work, and preserved or correctly clamped first-visible indexes after resize before approving the issue.

---
