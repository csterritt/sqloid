# Tasks for #16: Ordered projection editing and deduplication

Parent issue: #16
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify ordered projection state invariants

**Type**: RED
**Output**: Failing pure tests cover insertion order, multiple aggregates per column, exact duplicate rejection, sentinel defense, and wildcard exclusivity.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the pure table-driven tests in `internal/querybuilder/projection_test.go` around the typed projection state introduced by Issue #15, using column identities from `internal/schema`. Require insertion order to be preserved exactly across Value, Count, Min, Max, Avg, and Sum entries; permit the same column to appear with different aggregates and different columns to use the same aggregate; and reject only an exact repeated `(column identity, aggregate)` pair without reordering, replacing, or changing focus-transition data. Require the bare `COUNT(*)` sentinel to coexist with later named aggregate entries, while an identical sentinel transition invoked directly outside the conditional UI path is a no-op. Require wildcard selection at any point to atomically clear all prior named and sentinel entries and become the sole projection, reject direct attempts to append anything beside wildcard until a valid transition replaces or removes it, and prevent malformed sentinel/wildcard identities from bypassing invariants. Keep this task test-only and assert immutable before/after state, including duplicate and invalid no-ops.

---

### 2. Implement ordered projection transitions

**Type**: GREEN
**Output**: Projection state and defensive deduplication tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Complete the ordered projection state and transition methods in `internal/querybuilder/projection.go` and integrate them with `internal/querybuilder/builder.go`. Store typed entries in insertion order, distinguish wildcard, bare `COUNT(*)`, and named `(column, aggregate)` entries structurally, and compare stable Schema column identity plus aggregate choice for exact named duplicates. Append valid distinct entries without sorting; make duplicate sentinel and exact named-pair transitions no-ops; make wildcard selection replace the whole list atomically; and defensively prohibit any wildcard coexistence even when transitions are called outside `internal/ui`. Preserve Issue #15's candidate visibility and routing behavior, return new state rather than mutating prior slices or catalogs, avoid UI-specific labels in invariants, and implement only enough to make Task 1 pass.

---

### 3. Specify projection editing keys

**Type**: RED
**Output**: Failing model tests require Backspace/Delete to remove only the latest entry and no-op when empty while preserving popup behavior.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/querybuilder/projection_test.go` with a pure remove-latest transition contract and add scripted key/model coverage in `internal/ui/projection_editing_test.go`, following the keyboard-context matrix and exact focus ownership in `Notes/PRD-sqloid.md`. While the base Column(s) field is focused, require both Backspace and Delete to remove exactly the last committed projection entry, preserve every earlier entry and its order, and leave focus on Column(s); repeated presses must walk backward through named entries and the bare sentinel, and empty state must be an unchanged no-op. Require removing sole wildcard to produce empty projection, after which opening Column(s) again shows Issue #15's wildcard-first and `COUNT(*)`-second candidates. Assert that editing keys inside Column(s) search or aggregate popup remain governed by the reusable popup/input contract rather than deleting committed entries, and that removal does not close, reopen, reorder, or corrupt an already open popup. Keep this task test-only and cover duplicate-rejection history without treating a rejected duplicate as removable state.

---

### 4. Integrate projection removal into the builder

**Type**: GREEN
**Output**: Scripted editing tests pass without reordering or corrupting projection entries.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the immutable remove-latest transition in `internal/querybuilder/projection.go` and route Backspace/Delete from the base Column(s) field through it in `internal/ui/model.go` and `internal/ui/projection_popup.go` or the existing focused-field key seam. Remove one committed entry only, preserve earlier typed identities and insertion order, and make empty removal a true no-op. After removing sole wildcard or the final nonwildcard entry, rely on QueryBuilder candidate derivation so the next Column(s) open restores Issue #15's empty sequence; do not patch candidate lists in UI code. Respect text/search editing and open-popup modality, preserve popup search/highlight/viewport and exact opener focus when the key belongs to that context, and keep deduplication and wildcard exclusivity authoritative in QueryBuilder. Make Tasks 1 and 3 pass without adding reordering controls or broad whole-field clearing behavior assigned to later issues.

---

### 5. Document projection editing rules

**Type**: DOCUMENT
**Output**: Wiki documentation records ordering, duplicates, wildcard exclusivity, sentinel defense, and removal.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #16 implementation and tests from `internal/querybuilder`, `internal/ui`, and the consumed `internal/schema` identities into the appropriate pages under `Notes/wiki`. Document insertion-ordered typed projection entries, distinct aggregates on one column, exact-pair duplicate rejection, bare `COUNT(*)` coexistence and defensive duplicate rejection, wildcard atomic replacement and sole-entry invariant, immutable transition behavior, and Backspace/Delete removal of only the latest committed entry with empty no-op. Explain base Column(s) key scope versus focused search/aggregate popup behavior and how returning to empty restores Issue #15's candidates. Cross-reference Issues #15 and #16 and the Query Grammar, Builder and Display Interaction, keyboard context matrix, QueryBuilder, UI, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the projection-editing walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/016-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/016-06/code-walkthrough`. Demonstrate multiple named projection entries retaining insertion order, the same column with distinct aggregates, exact duplicate rejection without visible reordering, bare `COUNT(*)` coexisting with a named aggregate, and a direct duplicate-sentinel transition being safely ignored. Select wildcard after populated state and show atomic clearing and sole-entry exclusivity. Then use Backspace and Delete repeatedly from the base Column(s) field to remove one latest entry per press through wildcard, named, and sentinel states, including empty no-op and restoration of the wildcard/`COUNT(*)` empty popup sequence. Include evidence that the same keys in focused search or aggregate-popup contexts preserve the popup contract. Reference Issue #16 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
