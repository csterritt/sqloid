# Tasks for #19: Authoritative runnable-state feedback

Parent issue: #19
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify command-independent runnable reports

**Type**: RED
**Output**: Failing table tests cover every SELECT/UPDATE/DELETE/INSERT prerequisite, common gate, first-invalid field, and exact/specific reason.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create exhaustive UI-independent table tests in `internal/querybuilder/runnable_test.go` for one authoritative runnable report across SELECT, UPDATE, DELETE, and INSERT. Model the report as runnable or as the first invalid typed field plus a specific reason, and order failures by each command's visual builder order rather than validation implementation order. Cover missing command, ineligible or stale table/identifiers, incomplete value prompts, and every common data gate; for SELECT cover empty projection, incomplete/invalid WHERE, every grouping and ORDER BY rule, and empty/valid/invalid Limit including the exact Limit reason. Define forward-compatible UPDATE cases for no assignments, duplicate SET columns, incomplete Value/NULL choices, unsubmitted Value entries, and optional incomplete WHERE; DELETE with eligible table, absent/complete/incomplete WHERE; and INSERT with zero insertable columns, incomplete per-column Value/NULL/Default choices, unsubmitted Value entries, and valid all-omit state. Require entered empty TEXT to count as submitted where universal values allow it, SQL NULL choices to differ from typed TEXT `NULL`, and every multi-failure case to return only the earliest invalid field/reason. Keep this task test-only, use typed placeholder write state in `internal/querybuilder` where end-to-end write flows are not yet integrated, and do not import UI focus or request state.

---

### 2. Implement the authoritative runnable evaluator

**Type**: GREEN
**Output**: UI-independent runnable reports pass for all four commands without starting work.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement a pure authoritative runnable evaluator in `internal/querybuilder/runnable.go`, integrating it with command-specific state in `internal/querybuilder/builder.go` and reusing WHERE, grouping, ORDER BY, LIMIT, projection, and Schema identity contracts rather than duplicating validators. Return a typed field target and stable specific reason for the first failure in visual order, with a runnable result carrying no UI action and never starting validation, estimation, execution, or history append. Define the UPDATE/INSERT assignment and choice completion seams needed by Issues #37 and #39 so all four command contracts are representable now: unique complete UPDATE SET choices with submitted Values, optional complete WHERE for UPDATE/DELETE, complete INSERT choices including all-omit validity, and explicit zero-insertable-column blocking. Keep selected-command and refreshed-identity common gates authoritative, preserve empty entered Value as complete after submission, and distinguish absent, pending, invalid, and complete state structurally. Avoid imports from `internal/ui` or `internal/history`, and implement only enough to make Task 1 pass without beginning write-flow UI or database work.

---

### 3. Specify whole-value clearing behavior

**Type**: RED
**Output**: Failing tests cover WHERE, Limit, future UPDATE/INSERT Value fields, Backspace/Delete, empty no-ops, and resulting invalidity.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add immutable transition tests in `internal/querybuilder/whole_value_test.go` and scripted key tests in `internal/ui/whole_value_clearing_test.go` for the general completed whole-value clearing contract. Cover completed WHERE values for SELECT/UPDATE/DELETE, valid and invalid nonempty Limit text, and the forward-compatible UPDATE/INSERT submitted Value fields exposed by Task 2; require both Backspace and Delete in the focused base field to clear the entire entered representation, parsed/bound value, and submission marker atomically. Require already absent or empty whole fields to be unchanged no-ops with no focus, popup, command, execution, or history side effect; distinguish submitted empty TEXT from an already empty/unsubmitted field and follow the tested field-specific clear result. Assert clearing a value-taking WHERE leaves its selected column/operator intact but makes the predicate incomplete, clearing Limit restores unbounded validity, and clearing UPDATE/INSERT Value leaves the Value choice selected but incomplete for later flows. Require keys inside focused text/search input or an open popup to retain that context's editing behavior rather than triggering base-field clearing. Keep this task test-only and do not broaden Issue #16's remove-latest projection behavior.

---

### 4. Implement reusable whole-value clearing

**Type**: GREEN
**Output**: Builder transition tests pass and expose the shared behavior to later write flows.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement reusable immutable whole-value clearing transitions in `internal/querybuilder/whole_value.go` or the narrow existing value-owning files, and route base-field Backspace/Delete through `internal/ui/model.go` and the shared focused-field key seam. Clear exact entered text, parsed/bound type, and submission completion while preserving surrounding structural choices: WHERE column/operator, LIMIT field identity, and future UPDATE/INSERT Value choice and column identity. Make absent/already-clear state return unchanged state, ensure Limit clearing becomes the valid unbounded state while required predicate or assignment Values become incomplete, and let the authoritative runnable report derive resulting validity. Respect global key precedence, text-input editing, popup modality, and Issue #16's separate projection removal behavior; produce no execution command or history mutation. Expose the same QueryBuilder transition for Issues #37 and #39 rather than requiring later UI-specific clearing implementations, and make Task 3 pass.

---

### 5. Specify Enter gating and focus feedback

**Type**: RED
**Output**: Failing model tests cover valid/invalid data, base versus invalid UI contexts, visual-order focus, exact Limit feedback, and absence of execution commands.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create scripted Bubble Tea model tests in `internal/ui/runnable_feedback_test.go`, using QueryBuilder states from Task 1 and `(model, msg) → (model, cmd)` patterns from the PRD Testing Decisions. In a base, supported-size, idle context, require Enter on invalid data to consume the key, move focus to the report's typed first-invalid field in visual order, show its specific inline reason, and return no validation, estimation, execution, or history command; include multi-failure SELECT and representative UPDATE/DELETE/INSERT states. Require invalid nonempty Limit to focus Limit and display exactly `Limit must be an integer from 1 to 9223372036854775807`. For valid data, require Enter to emit only the next pre-execution action appropriate to the existing lifecycle seam, never direct execution in this issue. Cover valid data in popup, focused text/search input, help/modal or other overlay, request-in-flight, and too-small contexts, requiring the higher-precedence context to consume Enter with its own behavior and no runnable action. Assert correcting/clearing fields removes or updates stale reasons appropriately, focus targets can represent future write prompts, and runnable evaluation stays UI-independent. Keep this task test-only.

---

### 6. Connect runnable reports to the TUI

**Type**: GREEN
**Output**: Enter consumption, focus movement, inline reasons, and no-execution tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Connect `internal/querybuilder`'s authoritative runnable report to base-context Enter handling in `internal/ui/model.go`, the builder focus model, and inline feedback rendering. Apply the global precedence table before consulting data validity: overlays and focused inputs consume Enter locally, pending requests and too-small screens block it as specified, and only an idle supported base context requests a report. On invalid data, map the typed report field to the exact visual focus target, including nested future UPDATE/INSERT prompt targets, render the QueryBuilder-supplied reason verbatim, clear superseded feedback when the field is corrected, and return no command that can validate, estimate, execute, or append history. On runnable data, emit only the pre-execution lifecycle command/seam expected by later schema validation and destructive preparation issues; do not execute SQL in this issue. Keep field-order knowledge localized to the builder/report mapping rather than maintaining a second validity engine in UI, preserve whole-value clearing and popup restoration behavior, and make Task 5 pass for all four commands.

---

### 7. Document runnable-state contracts

**Type**: DOCUMENT
**Output**: Wiki documentation records per-command prerequisites, common gates, field order, clearing, and UI/data separation.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #19 implementation and tests from `internal/querybuilder` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the authoritative UI-independent runnable report, typed first-invalid field and specific reason, visual-order precedence, selected-command/refreshed-identifier/incomplete-prompt common gates, and every SELECT, UPDATE, DELETE, and INSERT prerequisite, including future write-state seams. Explain the separation between data-runnable state and UI context gates, Enter behavior in base versus popup/input/overlay/pending/too-small contexts, no direct execution, and exact Limit feedback. Record Backspace/Delete whole-value clearing for WHERE, Limit, and later UPDATE/INSERT Value fields; submitted empty TEXT, already-empty no-ops, preserved surrounding choices, resulting validity, and its distinction from projection removal. Cross-reference Issues #16-19 and later Issues #21, #37, #38, and #39 plus the Runnable-State Contract, Builder and Display Interaction, Global Key Precedence, QueryBuilder, UI, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the runnable-state walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/019-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/019-08/code-walkthrough`. Demonstrate table-driven authoritative reports for every SELECT/UPDATE/DELETE/INSERT prerequisite and common gate, including multiple simultaneous failures returning the first visual field and one specific reason. Clear completed WHERE and Limit values with both Backspace and Delete, show empty no-ops and resulting incomplete/unbounded validity, and exercise the shared future UPDATE/INSERT Value transition without claiming those end-to-end flows are complete. In scripted model evidence, press Enter on representative invalid SELECT states, including exact invalid Limit feedback, and show focus movement plus absence of validation/execution/history commands. Contrast runnable data in base context with popup, text/search, overlay, pending-request, and too-small contexts, showing UI precedence and only the next pre-execution seam. Reference Issue #19 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
