# Tasks for #39: INSERT choices, omission, and prompt restoration

Parent issue: #39
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify insertable-column prompting

**Type**: RED
**Output**: Failing Schema/QueryBuilder tests cover visible insertable columns, generated/hidden exclusion, Value/NULL/Default choices, INTEGER PRIMARY KEY hinting, and zero-column blocking.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven schema fixtures and UI-independent prompt-plan tests in `internal/schema` and `internal/querybuilder`, following Issue #9's `PRAGMA table_xinfo` metadata and Issue #19's forward-compatible INSERT runnable seams. For ordinary and virtual tables, require every visible insertable column to appear exactly once in schema order, while hidden and generated columns are excluded regardless of declared type or defaults; prove there is no AUTOINCREMENT-based or nullable/default-based skip. For every included column, require exactly the closed choices Value, NULL, and Default/Omit in deterministic order, without declared-type filtering. Mark an INTEGER PRIMARY KEY prompt with exactly the semantic hint `(auto-assigned if omitted)` while retaining all three choices; ensure similar declared types and non-primary INTEGER columns do not receive it. Cover unusual quoted names, mixed visible/hidden/generated layouts, all-hidden/generated tables, and virtual-table hidden inputs. Require zero insertable columns to produce the exact blocking reason `table has no insertable columns`, no prompt plan, and a non-runnable report. Keep this task test-only and do not generate SQL or open UI prompts yet.

---

### 2. Implement INSERT choice state

**Type**: GREEN
**Output**: Column prompt plans, choices, completion, and blocking tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the INSERT prompt plan and immutable per-column choice state in `internal/querybuilder`, consuming authoritative insertability, schema order, declared type, and primary-key metadata from `internal/schema`. Include each visible insertable column once, exclude every hidden/generated column, expose only typed Value, NULL, and Default/Omit transitions, and annotate only INTEGER PRIMARY KEY columns with the omission hint without changing behavior or auto-selecting omission. Model unchosen, Value-with-unsubmitted/submitted-entry, NULL, and Default/Omit states structurally; submitted empty Value must be complete TEXT, while NULL and omission complete without text. Integrate Issue #19's authoritative runnable report so every planned column must be complete and a zero-column table returns exactly `table has no insertable columns`, cannot open prompts, and cannot run. Keep state UI-independent, avoid AUTOINCREMENT heuristics and declared-type filtering, and implement only enough to make Task 1 pass.

---

### 3. Specify INSERT SQL and parameters

**Type**: RED
**Output**: Failing tests cover empty TEXT, NULL, omission, mixed values, parameter order, all-omit `DEFAULT VALUES`, quoting, and virtual-table best-effort behavior.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exact, table-driven SQL/parameter tests in `internal/querybuilder` and focused `modernc.org/sqlite` integration tests through `internal/connection` for complete INSERT states. Require Value columns to appear in schema prompt order with `?` placeholders and exact universal bound types, including empty TEXT, INTEGER, REAL, and typed TEXT `NULL`; require NULL columns to remain included with the SQL keyword `NULL` and no parameter; require Default/Omit columns to be absent from both column and value lists. Cover single and mixed Value/NULL/omit choices, unusual table/column names with embedded quotes, constraints/default expressions, and exact parameter order over included Value choices while skipping NULL and omitted columns. When all prompts are omitted, require exactly `INSERT INTO <quoted table> DEFAULT VALUES` with no parameters and prove it runs through the normal path. Exercise ordinary tables, INTEGER PRIMARY KEY omission/auto-assignment, and virtual tables using only visible insertable columns; successful modules insert, while modules requiring hidden inputs surface ordinary database errors without builder fabrication. Keep the RED changes test-only and do not add UI or execution gating.

---

### 4. Implement INSERT statement generation

**Type**: GREEN
**Output**: Pure SQL/parameter and SQLite integration tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure INSERT statement generation in `internal/querybuilder`, reusing Issue #14's atom-by-atom identifier quoting and universal parameter values. Traverse complete prompt state in authoritative schema order: include Value columns with placeholders and parameters, include NULL columns with keyword `NULL` and no parameters, and exclude Default/Omit columns entirely. Preserve exact Value parameter order across mixed choices and emit the all-omit `DEFAULT VALUES` form without empty parentheses. Reject incomplete or zero-insertable-column state through the existing runnable report rather than producing partial SQL. Connect only the narrow statement request needed by `internal/connection` integration fixtures so ordinary and virtual tables receive normal SQLite behavior and errors; do not synthesize hidden module arguments, infer defaults, special-case AUTOINCREMENT, or import `internal/ui`. Implement only enough to make Task 3 pass.

---

### 5. Specify INSERT prompt restoration and execution gating

**Type**: RED
**Output**: Failing model tests cover exact restored choices/text/types, whole-value clearing, history-ready state, zero-column message, and no prompt/execution when blocked.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model and view tests in `internal/ui` for one prompt per insertable column in schema order, using shared choice-popup, universal-value, whole-value-clearing, runnable-feedback, and history-state seams. Require each prompt to show exactly Value, NULL, and Default/Omit, with the INTEGER PRIMARY KEY omission hint only on the applicable field. Script mixed choices, submitted empty TEXT, typed `NULL`, Tab/Shift+Tab/arrows through every prompt, Esc cancellation, and repeated revision; require exact restoration of choice, original text including emptiness/whitespace, parsed bound type/value, popup highlight, cursor/input state, and opener focus without reparsing. Backspace/Delete on a completed Value field must preserve the Value choice while atomically clearing text, bound type/value, and completion; NULL and omission remain structurally distinct. Require a complete mixed or all-omit builder to expose exact history-ready state and only the established validation/execution handoff, with no direct execution from prompt handling. For zero insertable columns, assert exact visible `table has no insertable columns`, no choice/value popup, no `internal/connection` request, and no query/result history. Keep this task test-only.

---

### 6. Integrate INSERT prompts into the builder

**Type**: GREEN
**Output**: End-to-end prompt, revision, DEFAULT VALUES, and blocking tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire the `internal/querybuilder` INSERT prompt plan, immutable choices, statement generation, and runnable reports into builder focus, popup/value input, inline rendering, history-ready state, and pre-execution routing in `internal/ui`. Open one shared scroll-only choice popup for each insertable column in schema order; open universal text entry only for Value, complete NULL and Default/Omit immediately, and render the INTEGER PRIMARY KEY omission hint from QueryBuilder metadata rather than UI type inference. Preserve exact choice/text/bound type/value and input/focus state across navigation, cancellation, revision, whole-value clearing, and query-state copies. Ensure mixed choices generate the Task 4 statement, all omitted columns generate the normal `DEFAULT VALUES` path, and command/table changes clear only dependent INSERT state. If no insertable columns exist, show the exact blocking message, open no prompts, issue no validation or `internal/connection` execution command, and append no history. Make Tasks 1, 3, and 5 pass without implementing later write summary or transaction lifecycle work.

---

### 7. Document INSERT construction

**Type**: DOCUMENT
**Output**: Wiki documentation records insertability, choices, omission, parameter order, restoration, and special table cases.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #39 implementation and tests from `internal/schema`, `internal/querybuilder`, `internal/connection`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document `table_xinfo`-derived visible insertability, hidden/generated exclusion, schema-order prompting, the exact Value/NULL/Default/Omit meanings, empty TEXT versus NULL versus omission, SQL/parameter construction, omission from both lists, and all-omit `DEFAULT VALUES`. Record INTEGER PRIMARY KEY prompting and exact hint, the absence of AUTOINCREMENT skips and type-specific behavior, zero-column exact blocking/no-prompt/no-execution behavior, and virtual-table visible-column best effort with ordinary module errors. Explain exact revision/history restoration of choices, entered representation, bound types/values, whole-value clearing, runnable/history-ready state, and safe identifier/value handling. Cross-reference Issues #9, #14, #19, and #39 and the INSERT Query Grammar, Runnable-State Contract, Builder interaction, Schema metadata, INSERT handling, Builder lifecycle, module designs, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the INSERT-builder walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/039-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/039-08/code-walkthrough`. Show schema fixtures with visible insertable, hidden, and generated columns and prove prompt order/exclusion. Walk every prompt choice, including empty TEXT Value, INTEGER/REAL, typed `NULL` TEXT, explicit NULL, omitted/defaulted columns, mixed rows, all omitted `DEFAULT VALUES`, and INTEGER PRIMARY KEY omission with its exact hint. Capture safely quoted SQL, placeholders/NULL keywords, exact parameter order and bound types, plus successful SQLite effects. Revisit, cancel, clear, and revise prompts to demonstrate exact choice/text/type/focus restoration and history-ready state. Include zero-insertable-column behavior with the exact message and no popup/request/history, and virtual-table best-effort success or ordinary hidden-input error evidence. Reference Issue #39 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
