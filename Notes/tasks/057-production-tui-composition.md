# Tasks for #57: Production TUI composition and binary smoke path

Parent issue: #57
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1 → Tasks 1–2; AC2 → Tasks 1–2 and 5–6; AC3 → Tasks 3–4; AC4 → Tasks 5–6; AC5 → Tasks 3–4
**Manual verification**: Task 8 owns both Phase A lifecycle checks and Phase B built-binary PTY interaction evidence.

## Delivery phases

- **Phase A — Tasks 1–4:** production composition root plus terminal lifecycle and shutdown. This is a separately landable prerequisite that unblocks Issues #58–#87's shipped-TUI manual and end-to-end verification.
- **Phase B — Tasks 5–8:** unattended PTY-driven built-binary capability gate, documentation, and evidence. Phase B begins only after Phase A lands and does not delay downstream package work.

## Tasks

### 1. Specify production model composition and real adapters

**Type**: RED  
**Output**: Failing composition tests require initial catalog loading and every database/filesystem UI seam to be backed by the opened production session.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add test-only composition coverage around `cmd/sqloid`, `internal/connection`, and `internal/ui` that opens a real temporary SQLite database, reads its initial `schema.Catalog`, constructs the initial `ui.Model`, and inspects or exercises the resulting seams. Require the model to retain the loaded catalog and wire schema-version reads, catalog refresh, first-page SELECT, complete-result count, later paging, destructive estimates, transactional writes, destination picking, and atomic saves to the same owned `*connection.DB` and real filesystem implementations. Prove connection outcomes map to the existing typed UI result, cancellation, health-terminal, and write-phase contracts rather than leaking driver types into `internal/ui`; require initial catalog failure to stop before Bubble Tea starts. Keep this task test-only, follow the adapters implied by `internal/ui/model.go`, `first_select.go`, `count.go`, `paging.go`, `schema_validation.go`, `schema_refresh.go`, `destructive_prep.go`, `write_exec.go`, `filepicker.go`, and `save_write.go`, and do not add a second startup or database-opening path.

---

### 2. Implement the production composition root and adapters

**Type**: GREEN  
**Output**: A successfully opened database and catalog produce one fully wired production `ui.Model` for either startup mode.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the smallest production composition layer between `cmd/sqloid/main.go`, `internal/cli`, `internal/connection/startup.go`, and `internal/ui` that satisfies Task 1. Retain ownership of the `*connection.DB` returned by `connection.Open`, synchronously load the initial catalog through the existing request boundary, initialize `ui.New` with that catalog, and provide thin adapters for version/catalog reads, first and later pages, count, estimate, phased write execution, picker listing, destination inspection, and atomic persistence. Map `connection.RequestResult`, started request results, health classifications, cancellations, pages, counts, estimates, and write phases into the established UI types without duplicating SQL, transaction, history, or filesystem behavior. Keep both `sqloid sqlite <file>` and the D1-discovered path on this one session constructor, and implement only enough production wiring to make Task 1 pass.

---

### 3. Specify terminal lifecycle, cleanup, and status mapping

**Type**: RED  
**Output**: Failing lifecycle tests cover successful quit, terminal failure, startup/composition failure, request settlement, and database-close ordering.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic lifecycle tests at the composition and CLI boundaries using an injected Bubble Tea runner and observable session close hooks rather than a live terminal. Cover ordinary confirmed quit returning status 0, deletion/replacement/outcome-unknown terminal completion returning status 1, Bubble Tea runner failure returning one actionable error, and catalog/composition failure preventing the program from starting. Hold SELECT, validation, estimate, and write work at existing settlement barriers and require accepted quit to request only permitted cancellation, wait for UI cleanup and transaction/request settlement, then close the database exactly once; assert no lease, phase relay, or filesystem command continues after close. Preserve the CLI contract of exactly one stderr line and status 1 for startup/composition/runtime failures, no target creation on failed startup, and no duplicate diagnostic from nested layers. Keep this task test-only and reuse `internal/ui` quit/terminal tests, `internal/connection` settlement barriers, and `internal/cli` injected handler patterns.

---

### 4. Run Bubble Tea with ordered shutdown and exact outcomes

**Type**: GREEN  
**Output**: Both startup modes run the production Bubble Tea program and close resources only after the model has settled.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the production session path to run `tea.NewProgram` only after successful open, initial catalog load, and complete model wiring. Introduce only the narrow runner/result abstraction needed by Task 3 so tests can observe start, final model, error, and ordering while production still uses Bubble Tea normally. Translate normal confirmed quit to a nil handler error, translate terminal deletion/replacement/outcome-unknown and runner failure to one actionable handler error for CLI status 1, and preserve already formatted startup diagnostics. Ensure accepted quit lets the model's existing cancellation and write-settlement commands finish, waits for the program to return, retires any composition-owned resources, and only then closes `connection.DB` once. Route D1 discovery into the same session runner after it selects a path; do not alter usage status 2 or D1 discovery diagnostics.

---

### 5. Specify a headless production binary capability path

**Type**: RED  
**Output**: A failing unattended binary/integration test drives real startup, SELECT, confirmed write, export, quit, and persisted effects through the built `sqloid` binary under a PTY.
**Depends on**: 4 (Phase A landed)
**Vetted dependency**: `github.com/creack/pty v1.1.24` (test harness; Linux/macOS)

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add `github.com/creack/pty v1.1.24` with the Go package manager as a direct test dependency, then add a production-level integration test under `cmd/sqloid` that builds the shipped `sqloid` command and runs that binary under the PTY unattended in headless CI. Do not substitute an in-process Bubble Tea harness or the Go test binary. Run the built command against a real temporary SQLite database, assert it remains running after validation and renders the initial full-screen builder, then drive a baseline SELECT to visible rows, a deliberately confirmed UPDATE/DELETE/INSERT to a persisted database change, and Ctrl+S or Ctrl+X through the real picker and atomic save boundary to exact output bytes. Quit through the documented confirmation, assert status 0 and complete resource cleanup, and reopen the database/output to verify effects. Exercise both explicit `sqlite` routing and D1 discovery into the same TUI composition, while keeping one focused full interaction path if duplicating the whole script would be excessive. Require the test to fail when the program launch, any real adapter, or the TUI run is bypassed even if package fake-seam tests pass; use deterministic input synchronization and terminal markers rather than arbitrary sleeps.

---

### 6. Complete the binary smoke harness and production path

**Type**: GREEN  
**Output**: The headless production capability test passes reliably and detects bypassed TUI composition.  
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Make the minimum test-harness and production adjustments needed for Task 5 to pass without adding test-only behavior to the shipped binary. The harness must execute the built `sqloid` binary under `github.com/creack/pty`, not an in-process Bubble Tea program or the Go test binary, so it proves the production composition root is reached. Keep terminal setup, window sizing, input sequencing, output normalization, temporary working directory, D1 fixture layout, and timeouts inside the integration test or existing test seams; production must still use the real Bubble Tea program, connection adapters, `filepicker.OSFS`, and `export.OSSaveFS`. Ensure the script waits on observable UI state before each key sequence, confirms destructive writes and overwrite only through normal UI actions, and leaves no goroutines, leases, temporary save artifacts, or open database handles. Run the relevant package tests and the repository's established Go verification command, addressing flakiness with synchronization rather than retries or weakened assertions.

---

### 7. Document both production-composition delivery phases

**Type**: DOCUMENT  
**Output**: Wiki documentation separately records the Phase A composition/lifecycle contract and Phase B headless built-binary gate.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #57 implementation and tests from `cmd/sqloid`, `internal/cli`, `internal/connection`, `internal/ui`, `internal/filepicker`, and `internal/export` into the appropriate pages under `Notes/wiki`. Document the single explicit/D1 path from discovery or argument through open, initial catalog load, production model construction, every real executor, and Bubble Tea launch. Record ownership of `connection.DB`, mapping of request/write/filesystem outcomes, normal versus terminal process status, exactly-one stderr behavior, accepted-quit settlement, program completion, and database-close ordering. Describe the unattended production binary capability test, the real SELECT/write/export effects it proves, and why package fake seams cannot replace it. Cross-reference Issue #57 and the Startup validation, Execution and Result Lifecycle, Writes and commit boundary, File picker, Atomic saves, Module Design, Testing Decisions, and Acceptance Criteria sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the production-composition walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/057-08/code-walkthrough`.  
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/057-08/code-walkthrough`, with the main file named `walkthrough.md`. Trace both explicit SQLite and D1 discovery into the shared open/catalog/model/program path, enumerate every real database and filesystem adapter, and show that a valid database reaches the full-screen builder instead of returning after validation. Run the unattended production capability path against real temporary files, capturing a baseline SELECT, confirmed persisted write, saved/exported output, confirmed quit, status, cleanup, and database-close ordering; also demonstrate one startup/composition failure with exactly one stderr line and status 1. Reference Issue #57 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
