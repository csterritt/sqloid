# Tasks for #2: SQLite startup validation and read-write errors

Parent issue: #2
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Pin the SQLite driver

**Type**: CONFIG  
**Output**: A vetted exact `modernc.org/sqlite` version is added through Go tooling.  
**Depends on**: none

Review the Language and stack decision in `Notes/PRD-sqloid.md`, vet an appropriate release of the pure-Go SQLite driver, and add that exact `modernc.org/sqlite` version to the existing Go module through Go tooling. Keep this task limited to dependency configuration and verification; do not implement the `internal/connection` startup path or hand-edit generated module checksums.

---

### 2. Specify non-creating file validation

**Type**: RED  
**Output**: Failing tests cover existence, readability, directory/header rejection, validation order, and no target creation or modification.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven filesystem tests in `internal/connection` for Issue #2 and the Startup validation and errors section of `Notes/PRD-sqloid.md`. Require checks for existence and readability before rejecting directories and files without the exact 16-byte SQLite header, assert the mandated validation order and structured failure classes, and prove pre-open validation neither creates a missing target nor modifies any existing target.

---

### 3. Implement filesystem and SQLite-header validation

**Type**: GREEN  
**Output**: Pre-open validation tests pass in the Connection package.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the ordered, non-mutating pre-open validation path in `internal/connection`, which owns SQLite connection startup for both explicit and discovered paths. Follow the exact check order in Issue #2 and `Notes/PRD-sqloid.md`, identify the exact SQLite header without opening through the driver, classify directories and invalid headers appropriately, and retain structured causes for later rendering by `internal/cli`.

---

### 4. Specify read-write opening and error classification

**Type**: RED  
**Output**: Failing integration/CLI tests cover `mode=rw`, schema probe, unchanged journal mode, EACCES/EPERM, EROFS, raw driver causes, one-line stderr, and exit status 1.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused integration tests in `internal/connection` and CLI-facing tests in `internal/cli`, using `cmd/sqloid` process behavior where needed, for the remaining Issue #2 contract. Require opening with `mode=rw`, a harmless schema probe, unchanged journal mode, no read-only fallback, preserved EACCES and EPERM permission causes, EROFS classification, and raw driver causes for other failures; assert exact one-line stderr diagnostics, exit status 1, silent success, and no target creation or modification.

---

### 5. Implement the shared read-write opener

**Type**: GREEN  
**Output**: Opening, probing, and exact diagnostic tests pass with no read-only fallback.  
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the shared opener in `internal/connection` with the pinned `modernc.org/sqlite` driver, applying the pre-open checks before opening validated paths in read-write, non-creating mode and probing the schema without changing journal mode. Preserve wrapped OS and raw driver causes, and connect the `sqlite` handler in `internal/cli` to render the exact Issue #2 one-line failures and status 1 while `cmd/sqloid` remains a thin entrypoint; do not add a read-only fallback or duplicate connection ownership.

---

### 6. Document startup validation and recovery

**Type**: DOCUMENT  
**Output**: Wiki documentation records validation order, failure classes, and read-write-only behavior.  
**Depends on**: 5

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #2 implementation and tests into the appropriate pages under `Notes/wiki`. Document `internal/connection` ownership, the exact pre-open validation order, failure classes and one-line recovery diagnostics, `mode=rw`, the schema probe, journal-mode preservation, cause handling, no creation or modification, and the absence of read-only fallback; update the wiki index and append-only log as required.

---

### 7. Create the startup-validation walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough demonstrates valid and rejected startup fixtures at `Notes/walkthroughs/002-07/code-walkthrough`.  
**Depends on**: 6

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/002-07/code-walkthrough`. Demonstrate representative `internal/connection`, `internal/cli`, and `cmd/sqloid` verification for valid, missing, unreadable, directory, invalid-header, and read-only fixtures, including validation order, `mode=rw`, probing, exact one-line diagnostics, status 1, and non-creation or modification, with references to Issue #2 and `Notes/PRD-sqloid.md`.

---
