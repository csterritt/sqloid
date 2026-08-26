# Tasks for #11: Command and table selection lifecycle

Parent issue: #11
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify command and table state transitions

**Type**: RED
**Output**: Failing builder tests cover initial Command focus, S/U/D/I replacement, downstream clearing, eligible-table retention, and view-to-write clearing.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create pure table-driven tests in `internal/querybuilder/command_table_test.go`, using the typed object kinds and eligibility metadata from `internal/schema/schema.go` and `internal/schema/metadata.go`. Require an initial unselected command state, immutable S/U/D/I selection and replacement transitions, clearing of every downstream command-specific field, retention of a selected object only when eligible for the replacement command, and clearing of a selected view when moving from SELECT to UPDATE, DELETE, or INSERT while retaining the refreshed eligible write-table list. Assert the next required builder field/focus result as UI-independent transition data, cover ordinary and virtual tables as write candidates and views as SELECT-only, and keep this task test-only with no Bubble Tea dependency.

---

### 2. Implement command and table builder state

**Type**: GREEN
**Output**: UI-independent builder transition tests pass using Schema eligibility metadata.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Establish the UI-independent QueryBuilder state in `internal/querybuilder/builder.go` and implement command/table transitions in `internal/querybuilder/command_table.go`, consuming object kinds and eligibility from `internal/schema` rather than duplicating catalog rules. Follow the immutable transition and authoritative state responsibilities in the QueryBuilder Module Design section of `Notes/PRD-sqloid.md`. Represent Command, selected Table, refreshed eligible objects, downstream command-specific state, and the next required field; on replacement, clear all downstream state, retain only an eligible object, and clear a view for every write command while leaving eligible ordinary and virtual tables available. Do not import `internal/ui`, render copy, implement popup behavior, or build SQL; implement only enough to pass Task 1.

---

### 3. Specify idle rendering and key-driven focus

**Type**: RED
**Output**: Failing model/rendering tests require the exact startup prompt, no result-only headers, one-key selection, and focus advancement.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model tests in `internal/ui/command_table_test.go` and rendering assertions in `internal/ui/view_test.go`, following the Issue #8 shell's `(model, msg) → (model, cmd)` and region-ownership patterns. Require startup focus on Command, one plain S/U/D/I key to replace the command and advance focus to Table, and exact idle results content `Select a command (S/U/D/I) to begin` inside the existing bordered results region before any execution exists. Assert that the idle view has no frozen header, displayed range, or count and differs from an executed SELECT's `No rows` state without changing normal layout arithmetic. Cover revisiting Command and command replacement focus outcomes using injected Schema eligibility metadata; keep the task test-only and avoid direct database behavior.

---

### 4. Integrate Command and Table into the TUI

**Type**: GREEN
**Output**: Scripted UI tests pass for initial selection, revisiting Command, table retention/clearing, and idle-state distinction from `No rows`.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate `internal/querybuilder/builder.go` and `internal/querybuilder/command_table.go` with the Bubble Tea shell through `internal/ui/model.go`, `internal/ui/command_table.go`, and `internal/ui/view.go`. Render Command and Table fields from QueryBuilder state, route S/U/D/I only while Command is focused, advance to Table, and preserve the existing focus-navigation and responsive layout seams. On Command revisit and replacement, reflect QueryBuilder's downstream clearing and Schema-driven table retention rules exactly, including clearing a selected view and focusing Table for a write command while leaving eligible ordinary and virtual tables populated. Replace tracer-only startup presentation as needed without extending Issue #10's disposable query path; keep the exact idle prompt distinct from executed-empty `No rows`, and do not implement searchable popup behavior assigned to Issue #12. Make Tasks 1 and 3 pass without moving database logic into `internal/ui`.

---

### 5. Document builder startup lifecycle

**Type**: DOCUMENT
**Output**: Wiki documentation records idle state, command replacement, table eligibility, and focus transitions.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #11 implementation and tests from `internal/querybuilder` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document initial Command focus, S/U/D/I one-key replacement, advancement to Table, downstream state clearing, Schema-owned table eligibility, ordinary/virtual-table retention, view-to-write clearing, revisiting Command, and resulting focus transitions. Record the exact pre-execution idle prompt, absence of result-only headers/range/count, unchanged layout arithmetic, and distinction from executed-empty `No rows`. Cross-reference Issue #11 and the Builder and Display Interaction, Builder lifecycle, QueryBuilder, UI, and Testing Decisions sections of `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md`, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the command/table walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/011-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/011-06/code-walkthrough`. Demonstrate the exact startup idle results prompt with no result-only headers, initial Command focus, each S/U/D/I one-key selection, Table focus advancement, and Command revisiting. Use Schema fixtures to show eligible ordinary and virtual tables, a SELECT-eligible view, retention across compatible command changes, downstream clearing, and view clearing plus Table focus when switching to a write command; contrast the idle state with executed `No rows`. Reference Issue #11 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review command and table selection

**Type**: REVIEW
**Output**: Human confirms startup, S/U/D/I selection, table/view behavior, and command switching.
**Depends on**: 6

Review `internal/querybuilder/builder.go`, `internal/querybuilder/command_table.go`, `internal/ui/command_table.go`, related model/view changes, tests, wiki updates, and walkthrough against Issue #11. Start the TUI and confirm the exact idle prompt, absent result-only metadata, normal responsive layout, initial Command focus, each one-key selection, and advancement to Table. Select ordinary tables, virtual tables, and views, revisit Command, switch among read and write commands, and confirm eligible retention, complete downstream clearing, view-to-write clearing, populated write candidates, and exact focus results; also verify idle remains distinct from executed `No rows` before approving the issue.

---
