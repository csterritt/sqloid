# INSERT builder, omission, and prompt restoration (Issue #39)

Issue #39 completes the fourth write command: guided INSERT construction with one {Value, NULL, Default/Omit} prompt per insertable column, exact SQL/parameter generation including the all-omit `DEFAULT VALUES` form, exact prompt restoration on revision, and the exact zero-insertable-column block. It builds on the schema catalog (Issue #9), SQL atoms (Issue #14), the write state and runnable seams (Issue #19), and the UPDATE assignment pattern (Issue #37).

## Insertability from `PRAGMA table_xinfo`

Insertability is authoritative metadata, never inferred:

- `internal/connection/schema.go` decodes `name, type, hidden, pk` from `main.pragma_table_xinfo(?)` — the bound-parameter table-valued form keeps the object name out of SQL text.
- `internal/schema/catalog.go` marks a column `Insertable` exactly when `hidden == 0` and the object is write eligible. Hidden and generated columns (any nonzero `hidden` value: fts5-style module columns, `GENERATED ALWAYS AS` stored/virtual columns) are excluded regardless of declared type or declared defaults. Views are never write eligible, so no view column is insertable.
- `InsertableCount` counts insertable columns; zero marks the non-runnable INSERT state.
- **There is no AUTOINCREMENT-based, nullable-based, or default-based skip.** An `INTEGER PRIMARY KEY AUTOINCREMENT` column prompts like any other; a `NOT NULL` column prompts; a column with a `DEFAULT` prompts. `internal/schema/insertability_test.go` proves this with fixtures mixing AUTOINCREMENT keys, defaulted TEXT, untyped columns, generated columns, and hidden module inputs.
- `PrimaryKey` carries the raw `pk` slot and `Object.PrimaryKeyCount` the number of key columns, decoded verbatim.

## The INTEGER PRIMARY KEY omission hint

`Object.InsertHint(col)` returns the exact `schema.InsertOmissionHint` — `(auto-assigned if omitted)` — only when **all** hold: the object is rowid-addressable (`RowidHas`, so never a WITHOUT ROWID table or virtual table), the column is insertable, the table has exactly one primary-key column (`PrimaryKeyCount == 1`), that column is this column (`PrimaryKey == 1`), and its declared type is exactly `INTEGER` (case-insensitive, trimmed). Consequently:

- `id INTEGER PRIMARY KEY` receives the hint (omission lets SQLite auto-assign the rowid).
- `INT`, `BIGINT`, or any other similar declared type never receives it; a non-primary INTEGER column never receives it; multi-column keys never receive it (each slot alone would not auto-assign); `INTEGER PRIMARY KEY` on a WITHOUT ROWID table never receives it.
- The hint annotates the prompt only: it never changes the offered choices, never pre-selects omission, and never alters behavior (proven by `TestInsertOmissionHintDoesNotForceOmission`).
- The UI renders it from `QueryBuilder.InsertPromptHint(column)` — QueryBuilder metadata, never UI type inference. `InsertPromptHint` requires an existing prompt state so a vanished column cannot render a hint.

## Prompt plan and immutable choice state

`internal/querybuilder/write_state.go` (pre-seeded by Issue #19, completed here):

- `InsertableColumns()` lists the refreshed visible insertable columns in declared order; `BeginInsertPrompts()` seeds one incomplete `InsertColumn` per column in that order (repeated calls and zero-column tables are no-ops); `InsertColumns()` returns a fresh slice.
- Choices are the closed typed set `InsertChoiceNone / Value / NULL / Default/Omit` with `ChooseInsertColumn(column, choice)` accepting only the three real choices, discarding any prior choice or submission.
- `SubmitInsertValue(column, text)` records one universal submission for the unsubmitted Value choice: empty text completes as empty TEXT; a typed `NULL` stays bound TEXT, structurally distinct from the SQL-NULL choice. `ClearInsertValue(column)` (Issue #19 whole-value seam) drops the entire submission — text, parsed bound type/value, and submitted flag — atomically while keeping the Value choice.
- State is UI-independent; there is no declared-type filtering and no AUTOINCREMENT special case anywhere.

## Runnable gating

`RunnableReport` (Issue #19) evaluates INSERT in visual order: a zero-insertable-column table blocks at `RunFieldInsertColumns` with the exact reason `table has no insertable columns`; then every prompted column in schema order must have exactly one choice (`complete the choice for column X`) and a submitted Value (`submit a value for column X`). All-omit is valid. A missing prompt state is an incomplete choice, so prompts must be begun for runnable data.

## Statement generation

`internal/querybuilder/insert_sql.go` — pure over complete state, gated by the runnable report:

- Traversal is in authoritative schema prompt order. **Value** columns contribute a quoted identifier and a `?` placeholder plus exactly one bound parameter via `Value.ParamValue()` (int64 INTEGER, float64 REAL, verbatim string TEXT — including empty and typed-`NULL` strings). **NULL** columns stay in both lists as the SQL keyword `NULL` with no parameter. **Default/Omit** columns are absent from both lists entirely.
- When every prompted column is omitted the exact `INSERT INTO "table" DEFAULT VALUES` form is emitted — no empty parentheses, no parameters.
- Identifier quoting reuses Issue #14's `quoteIdentifierAtom` (atom-by-atom, embedded double quotes doubled): table `tr "icky` and column `co "l` render `INSERT INTO "tr ""icky" ("co ""l") VALUES (?)`.
- Parameter order is exactly the included Value choices' order; NULL and omitted columns shift nothing.
- Incomplete, unrunnable, or zero-insertable state renders empty SQL and nil params — never partial SQL. `TestInsertSQLRejectsIncompleteState` proves it.
- No hidden virtual-module arguments are synthesized and no defaults inferred: `internal/connection/insert.go`'s `ExecuteInsert` runs the rendered statement through the normal `RunRequest` boundary, so a module requiring hidden inputs (or a `NOT NULL`/constraint violation) surfaces as an ordinary failed request with the cause preserved — proven by `internal/connection/insert_integration_test.go`, including real fts5 inserts, quoted-identifier round trips, DEFAULT VALUES applying declared defaults, and INTEGER PRIMARY KEY omission auto-assigning rowid 1.

## UI integration and restoration

`internal/ui/insert_popup.go` mirrors the Issue #37 SET flow:

- `applyBuilder` calls `BeginInsertPrompts` so selecting a table under INSERT immediately presents the full prompt plan in the Insert field (`id: incomplete, email: incomplete, ...`); command/table changes clear only dependent INSERT state through the QueryBuilder transitions.
- Enter on the Insert field opens one shared **scroll-only** choice popup for the first incomplete column with exactly the rows `Value`, `NULL`, `Default/Omit`; the highlight restores onto the column's current choice. The popup status line reads `Column: id (auto-assigned if omitted)` only when the schema metadata grants the hint.
- **Value** opens universal text entry seeded byte-for-byte with the prior entered representation (empty and whitespace preserved); Enter submits, Esc cancels with the committed state untouched. **NULL** and **Default/Omit** complete immediately with no text entry and advance to the next column.
- Tab/Shift+Tab inside the choice popup move between column prompts with state intact; completing the last prompt returns focus to the Insert field.
- Backspace/Delete on the focused Insert field clear the whole submitted value of the cursor column (`ClearInsertValue`), preserving the Value choice and column identity, keeping NULL and omission structurally distinct.
- The zero-column case: Enter shows exactly `table has no insertable columns` in the field content, opens no popup, issues no validation or connection command, and appends no history (`TestInsertZeroInsertableColumnsBlockExactUI`).
- Execution stays behind the established handoff: runnable Enter emits only `PreExecutionRequestedMsg` into the Issue #21 validation workflow; prompt handling never executes SQL or appends history.
- History-ready state: `HistoryState()` exposes every column with its choice and, for submitted Value choices, the parsed value and exact entered representation; all-omit and mixed states round-trip through Issue #35's `RestoreBuilder` unchanged.

## Tests

- `internal/schema/insertability_test.go` — schema-order/exclusion fixtures and the exact hint scope matrix.
- `internal/querybuilder/insert_prompt_test.go` — prompt plan, closed choice set, hint scope, zero-column blocking.
- `internal/querybuilder/insert_sql_test.go` — exact SQL/params: single Value, mixed Value/NULL/omit, empty TEXT, typed NULL, explicit NULL, DEFAULT VALUES, quoting, virtual tables, rejection of incomplete state, hint neutrality.
- `internal/connection/insert_integration_test.go` — real modernc.org/sqlite effects through `ExecuteInsert`.
- `internal/ui/insert_flow_test.go` — scripted prompt flow, hint rendering, mixed completion, exact revision restoration (choice, buffer bytes, cursor, highlight, focus), whole-value clearing, zero-column blocking, history-ready state, Tab navigation, UI prompt seeding.

Cross-references: [schema-catalog.md](schema-catalog.md), [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [runnable-state-feedback.md](runnable-state-feedback.md), [update-assignment-builder.md](update-assignment-builder.md), [delete-predicate-builder.md](delete-predicate-builder.md), [query-history-navigation.md](query-history-navigation.md), [schema-validation-workflow.md](schema-validation-workflow.md), and the INSERT Query Grammar, Runnable-State Contract, INSERT handling, Builder interaction, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md` (Issues #9, #14, #19, #39).
