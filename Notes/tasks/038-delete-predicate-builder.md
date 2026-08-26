# Tasks for #38: DELETE predicate builder

Parent issue: #38
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify DELETE state and generated SQL

**Type**: RED
**Output**: Failing QueryBuilder tests cover eligible tables, no WHERE, every complete shared predicate form, incomplete WHERE, safe SQL/params, and data-runnable reports.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent, table-driven DELETE tests in `internal/querybuilder`, reusing table eligibility from Issue #9, the canonical quoting/binding atoms from Issue #14, the shared optional predicate from Issue #17, and Issue #19's authoritative runnable report. Cover ordinary and virtual eligible write tables, rejected views/system/excluded/stale identities, safely quoted names including embedded quotes, and qualified versus unqualified DELETE state. Require an eligible table with absent WHERE to be data-runnable and generate exact `DELETE FROM <quoted table>` SQL with no parameters. For each fixed predicate operator, assert complete SQL and exact parameters: one bound universal value for value-taking comparisons and LIKE, none for `IS NULL`/`IS NOT NULL`; include INTEGER, REAL, empty TEXT, typed TEXT `NULL`, and verbatim `%`/`_`. Represent partially selected column/operator/value states as incomplete, return the first invalid predicate component and a specific reason, and produce no executable request for them. Keep this task test-only, preserve shared predicate semantics unchanged, and do not start preparation or execution.

---

### 2. Implement DELETE construction

**Type**: GREEN
**Output**: Qualified/unqualified DELETE SQL and runnable-state tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure DELETE command state and SQL generation in `internal/querybuilder`, composing the existing selected-table identity, shared optional predicate, canonical identifier quoting, parameter binding, and authoritative runnable evaluator rather than duplicating any of them. Allow only refreshed write-eligible table identities supplied by `internal/schema`, preserve a structurally absent WHERE as valid, append a complete predicate exactly once, and reject incomplete or stale state with the report's first typed invalid field/reason. Generate deterministic qualified and unqualified executable statements with no values interpolated, with zero parameters for no WHERE and null operators and the shared exact parameter for value-taking predicates. Expose a data-runnable result and statement request suitable for later destructive preparation, but import neither `internal/ui` nor `internal/connection` and perform no database work. Implement only enough to make Task 1 pass.

---

### 3. Specify DELETE builder transitions

**Type**: RED
**Output**: Failing model tests cover optional WHERE navigation, first-invalid focus, no preparation when incomplete, and preparation—not execution—when runnable.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model tests in `internal/ui` for DELETE command/table selection and its optional shared WHERE flow, following Issue #17 popup/value behavior, Issue #19 Enter gating, and the global context precedence in `Notes/PRD-sqloid.md`. Cover advancing from an eligible table directly with no WHERE, opening and completing column → operator → conditional value navigation, every null operator bypassing value input, value comparisons, LIKE with verbatim wildcards, and revisiting/cancelling prior predicate stages with exact state/focus restoration. Construct each incomplete predicate stage and require base Enter to focus its first invalid component in visual order, show the QueryBuilder reason, and return no schema-validation, destructive-preparation, write-execution, or history command. For no-WHERE and every complete predicate form, require the runnable path to emit only the established pre-execution validation/preparation handoff and never invoke `internal/connection` write execution directly. Cover popup, focused input, overlay, request-pending, and too-small contexts consuming Enter before runnable evaluation. Keep this task test-only and assert no preparation identity/history exists for invalid attempts.

---

### 4. Integrate DELETE into the builder

**Type**: GREEN
**Output**: End-to-end no-WHERE, value, LIKE, and null-operator flows pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire DELETE state from `internal/querybuilder` into command/table focus, the shared WHERE popup/value flow, runnable feedback, and pre-execution handoff in `internal/ui`. Populate Table from refreshed write-eligible ordinary and virtual tables while excluding views and prohibited objects, represent optional WHERE without synthetic values, and delegate all predicate transitions, universal parsing, quoting, SQL construction, and first-invalid validity to QueryBuilder. Preserve exact column/operator/value text/bound type and opener focus across navigation and Esc, honor whole-value clearing and global key precedence, and clear only dependent DELETE state when command/table changes. On invalid Enter, focus the report target and issue no preparation; on qualified or unqualified runnable state, create only the preparation-ready request/state expected by the destructive workflow, never execute the write or append query/result history in this issue. Make Tasks 1 and 3 pass for no-WHERE, value, LIKE, and SQL-null operator paths.

---

### 5. Document DELETE construction

**Type**: DOCUMENT
**Output**: Wiki documentation records optional predicates, runnable states, generated SQL, and preparation handoff.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #38 implementation and tests from `internal/querybuilder` and `internal/ui`, together with the consumed schema and shared predicate contracts, into the appropriate pages under `Notes/wiki`. Document write-table eligibility, absent WHERE as valid and unqualified, every supported predicate and conditional value rule, typed TEXT `NULL`, LIKE wildcard binding, null-operator no-parameter behavior, safe identifier quoting, exact generated DELETE SQL/params, incomplete-state first-invalid feedback, and qualified versus unqualified data-runnable reports. Explain the separation between data validity, UI context gates, schema validation, destructive preparation, and actual execution; record that runnable Enter hands off to preparation rather than executing or appending history. Cross-reference Issues #9, #14, #17, #19, and #38 and the Query Grammar, Runnable-State Contract, Builder and Display Interaction, pre-execution identities, SQL safety, Schema scope, QueryBuilder/UI Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the DELETE-builder walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/038-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/038-06/code-walkthrough`. Demonstrate DELETE table eligibility and exact safely quoted SQL for an unqualified no-WHERE state, then walk through column → operator → value for `=`, `LIKE`, `IS NULL`, and `IS NOT NULL`, including empty TEXT, typed `NULL`, and `%`/`_` binding evidence. Show qualified runnable SQL/parameters, incomplete column/operator/value states, first-invalid focus and reasons, exact restoration/cancellation, and whole-value clearing. Capture that invalid attempts create no preparation or history and that every runnable path hands off to destructive preparation rather than direct execution, including higher-precedence popup/input/overlay/pending controls. Reference Issue #38 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
