# Tasks for #37: UPDATE assignment builder and prompt restoration

Parent issue: #37
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify UPDATE assignment state and SQL

**Type**: RED
**Output**: Failing QueryBuilder tests cover unique ordered SET columns, Value/NULL completion, no Default/Omit, quoting, NULL keywords, optional WHERE, and SET-then-WHERE parameter order.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent, table-driven UPDATE tests in `internal/querybuilder`, following the immutable assignment seams from Issue #19, universal values and safe SQL atoms from Issue #14, and the shared predicate contract from Issue #17. Specify one or more unique schema-derived SET columns in selection order, with exactly one typed Value or NULL choice per column; prove duplicate selection cannot alter order or create a second assignment, Default/Omit is not representable, Value remains incomplete until submitted, submitted empty text is complete TEXT, and NULL is complete without entered text. Assert qualified and unqualified runnable reports, first-invalid assignment/WHERE targets, atom-by-atom quoting for table and column names including embedded quotes, exact `UPDATE … SET …` SQL, `?` only for Value assignments and value-taking WHERE operators, SQL `NULL` keywords with no parameter, and optional absent/complete/incomplete WHERE behavior. Require parameters to contain submitted Value assignments in SET-column order while skipping NULL, followed by the shared WHERE value when present; cover all-Value, all-NULL, mixed, typed TEXT `NULL`, and null-operator WHERE cases. Keep this task test-only and do not add prompt or preparation behavior.

---

### 2. Implement UPDATE construction

**Type**: GREEN
**Output**: Pure UPDATE state, runnable report, SQL, and parameter tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement immutable UPDATE assignment state and statement construction in `internal/querybuilder`, integrating the forward-compatible runnable-report and whole-value-clearing seams from Issue #19 rather than creating parallel validity rules. Accept SET identities only from refreshed eligible `internal/schema` columns, preserve first-selection order, prevent duplicates, and model Value versus NULL as a closed typed choice with submitted-value completion represented structurally; expose no Default/Omit transition. Reuse Issue #14 quoting, universal parsing/binding, and Issue #17 predicate rendering unchanged. Produce executable SQL and bound parameters deterministically, with every Value placeholder and parameter in SET order, every NULL rendered as the keyword with no parameter, and an optional complete WHERE appended afterward with its value last. Return a runnable report without UI or database side effects, reject incomplete/invalid state, support qualified and unqualified writes, and implement only enough to make Task 1 pass without destructive preparation or execution.

---

### 3. Specify UPDATE prompt flow and restoration

**Type**: RED
**Output**: Failing model tests cover SET multi-selection, per-column choices, universal Value entry, optional WHERE, whole-value clearing, and exact restored choice/text/bound type.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model and focused view tests in `internal/ui` for the complete UPDATE builder sequence, using Issue #12 popup behavior, Issue #14 universal value entry, Issue #17 optional WHERE navigation, and Issue #19 focus/clearing contracts. Require searchable SET multi-selection to preserve unique acceptance order, reopen after each accepted column, retain completed selections on Esc, and then visit one scroll-only choice prompt per selected column containing exactly Value and NULL in deterministic order. Require Value to open universal text entry for every declared type, including empty TEXT and typed `NULL`, while NULL completes immediately and opens no text input; after all assignments, navigate to the optional shared WHERE flow. Script Tab, Shift+Tab, and arrow revision through every assignment and WHERE stage, proving the exact selected choice, original entered text, parsed concrete bound type/value, popup highlight, and opener focus are restored without reparsing or partial commits. Cover changing Value to NULL and back, Esc cancellation, and Backspace/Delete whole-value clearing that preserves the Value choice but removes text, bound value/type, and submission completion atomically. Assert invalid Enter focuses the first incomplete assignment or predicate and emits no preparation, execution, or history command; runnable completion produces only the existing history-ready/pre-execution seam. Keep this task test-only.

---

### 4. Integrate UPDATE prompts into the builder

**Type**: GREEN
**Output**: End-to-end prompt navigation, completion, revision, and history-ready state tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire `internal/querybuilder` UPDATE transitions into the Bubble Tea builder model, popup, value-input, focus, and view seams in `internal/ui`. Populate SET choices from the selected table's refreshed eligible columns, use the shared searchable multi-select implementation with QueryBuilder-enforced uniqueness/order, and route each selected column through the closed Value/NULL choice state. Reuse universal value input for Value only and the existing shared WHERE flow after assignments; do not duplicate parsing, predicate completion, quoting, or runnable evaluation in UI. Preserve complete assignment snapshots so revisiting and cancellation restore exact choice, entered representation, bound type/value, cursor/input state, and opener focus, and route base-field Backspace/Delete through the Issue #19 whole-value transition. Make command/table changes clear only the documented downstream state, map first-invalid typed fields to the exact nested prompt, and retain complete ordered UPDATE state suitable for query-history comparison/restoration without appending history yet. Implement only enough to make Tasks 1 and 3 pass; destructive estimate presentation and actual write execution remain later issues.

---

### 5. Document UPDATE construction

**Type**: DOCUMENT
**Output**: Wiki documentation records SET choices, parameter order, optional WHERE, restoration, and runnable requirements.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #37 implementation and tests from `internal/querybuilder` and `internal/ui`, along with the consumed `internal/schema` identity metadata, into the appropriate pages under `Notes/wiki`. Document ordered unique SET selection, the exact Value/NULL choice set and exclusion of Default/Omit, submitted Value versus NULL completion, empty TEXT and typed TEXT `NULL`, safe identifier quoting, SQL `NULL` keyword behavior, optional shared WHERE, and the exact Value-in-SET-order then WHERE parameter contract. Record authoritative runnable requirements, first-invalid focus, whole-value clearing, command/table downstream clearing, and exact choice/text/bound-type restoration during revision and history-ready state. Cross-reference Issues #12, #14, #17, #19, and #37 and the Query Grammar, Runnable-State Contract, Builder and Display Interaction, SQL safety, Builder lifecycle, QueryBuilder/UI Module Design, and UPDATE Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the UPDATE-builder walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/037-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/037-06/code-walkthrough`. Build an UPDATE by selecting several SET columns in order, attempt a duplicate, and show per-column Value/NULL prompts with no Default/Omit. Enter INTEGER, REAL, empty TEXT, and typed `NULL` Values where applicable; mix them with NULL assignments, then add value-taking and null-operator optional WHERE examples. Capture exact quoted executable SQL, NULL keywords, placeholders, runnable reports, and parameter lists proving SET Value order, skipped NULLs, and the WHERE value last. Revisit every prompt with Tab, Shift+Tab, and arrows; demonstrate exact choice/text/bound-type restoration, cancellation, Value-to-NULL revisions, whole-value clearing, first-invalid focus, completion, and the history-ready state without claiming execution. Reference Issue #37 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
