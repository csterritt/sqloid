# Tasks for #10: Early integration tracer (hardcoded SELECT *)

Parent issue: #10
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the minimal query integration boundary

**Type**: RED
**Output**: Failing integration tests prove Connection and Schema can execute a hardcoded safe `SELECT *` and return typed rows/errors without UI database logic.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add SQLite-backed boundary tests in `internal/connection/tracer_test.go` and composition tests in `internal/schema/tracer_test.go`, using the catalog fixtures and typed metadata from Issue #9 and the request/lease patterns from `internal/connection`. Require one catalog-selected eligible table to become a safely identifier-quoted hardcoded `SELECT *`, execute through Connection, and return typed column names, rows, and errors suitable for composition without terminal copy. Cover a successful ordinary-table query, values needed to prove typed row transport, a safe unusual identifier, and a basic SQLite failure. Keep this task test-only; do not add `internal/ui` database calls or imply builder, validation, paging, count, history, cancellation, or write behavior.

---

### 2. Implement the disposable tracer execution path

**Type**: GREEN
**Output**: A clearly isolated tracer executes one chosen table through Connection and Schema.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the narrowly scoped execution request in `internal/connection/tracer.go` and the catalog-to-tracer composition seam in `internal/schema/tracer.go`, reusing `internal/schema/schema.go`, `internal/schema/catalog.go`, and the Connection pool, health-check, typed-error, and driver-hiding conventions established by earlier issues. Accept only a chosen object already validated by Schema, quote its identifier safely, execute exactly the disposable hardcoded `SELECT *`, and return typed headers, rows, or errors without presentation strings. Clearly isolate the tracer so Issue #22 can replace it, and do not introduce production query-builder, schema revalidation, paging, count, history, recovery, or write paths. Implement only enough to pass Task 1.

---

### 3. Specify minimal grid and error rendering

**Type**: RED
**Output**: Failing Bubble Tea model tests require bordered rows, column headers, and non-crashing basic error display.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea tests in `internal/ui/tracer_test.go` and focused rendering assertions in `internal/ui/results_test.go`, following the `(model, msg) → (model, cmd)` pattern and responsive region ownership from Issue #8. Feed typed tracer success and failure messages into the model without opening a database from the UI, and require a minimal bordered results grid with returned column headers and rows plus a basic non-crashing error state. Verify the existing builder/results/footer layout remains intact and that tracer output does not claim frozen-header paging, count, history, validation, or recovery behavior. Keep the task test-only and use test messages or fakes rather than embedding Connection logic in `internal/ui`.

---

### 4. Connect the tracer to the TUI

**Type**: GREEN
**Output**: End-to-end tracer tests render returned rows/errors while preserving Connection–Schema–UI boundaries.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add the disposable Bubble Tea composition path in `internal/ui/tracer.go` and integrate its messages and rendering through `internal/ui/model.go` and `internal/ui/view.go`, touching `cmd/sqloid/main.go` only if the existing thin executable composition root must wire dependencies. Invoke the Issue #10 tracer through injected Schema/Connection-facing commands, translate typed completion into model messages, and render the minimal bordered grid or basic error without direct SQL, database handles, driver types, or catalog queries in UI code. Preserve the Issue #8 responsive shell and keep all tracer-specific state visibly isolated for mandatory replacement by Issue #22; do not implement builder, validation, paging, count, history, cancellation, or error-recovery behavior. Make Tasks 1 and 3 pass with the smallest integration surface.

---

### 5. Document the disposable integration seam

**Type**: DOCUMENT
**Output**: Wiki documentation identifies the tracer’s purpose, boundaries, omissions, and mandatory replacement by Issue 22.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #10 files from `internal/connection`, `internal/schema`, and `internal/ui`, plus any thin composition-root change and all related tests, into the appropriate pages under `Notes/wiki`. Document the tracer's risk-reduction purpose, hardcoded safe `SELECT *` flow, typed row/error boundary, Connection–Schema–UI ownership, minimal bordered rendering, deliberate isolation, and every omitted builder, validation, paging, count, cancellation, history, and recovery behavior. State explicitly that Issue #22 must replace the tracer rather than extend it into a production query path. Cross-reference Issue #10 and the Module Design and Testing Decisions in `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md`, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the integration-tracer walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/010-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/010-06/code-walkthrough`. Demonstrate one Schema-selected fixture table flowing through `internal/schema/tracer.go`, `internal/connection/tracer.go`, and `internal/ui/tracer.go` into a minimal bordered grid with headers and rows, then demonstrate a basic query error rendering without a crash. Show that identifiers are handled safely and that UI files contain no database behavior. Call out the tracer's disposable scope, all unsupported production features, and mandatory replacement by Issue #22; reference Issue #10 and `Notes/PRD-sqloid.md`, and keep every generated artifact in the approved directory.

---

### 7. Review the tracer boundary

**Type**: REVIEW
**Output**: Human confirms rows/errors render and no production builder, validation, paging, or history behavior is implied.
**Depends on**: 6

Review `internal/connection/tracer.go`, `internal/schema/tracer.go`, `internal/ui/tracer.go`, their integration points, tests, wiki updates, and walkthrough against Issue #10. Run the tracer against a fixture table and a controlled basic failure, confirming typed rows and headers render in the bordered results area, errors do not crash, identifiers are safe, and Connection, Schema, and UI responsibilities remain separate. Confirm the implementation neither promises nor partially establishes production builder, schema validation, paging, count, cancellation, history, write, or recovery behavior, and that its mandatory replacement by Issue #22 is unambiguous before approving the issue.

---
