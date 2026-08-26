# Tasks for #1: CLI shell and usage contracts

Parent issue: #1
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Initialize the Go CLI project

**Type**: CONFIG  
**Output**: `go.mod`, `cmd/sqloid/main.go`, and initial `internal/cli` package exist; `go build ./...` and `go vet ./...` pass.  
**Depends on**: none

Initialize the Go module and minimal package layout required by Issue #1. Create `go.mod`, keep `cmd/sqloid/main.go` limited to the executable entrypoint, establish `internal/cli` for command construction and dispatch, and add the PRD-mandated `mow.cli` dependency through Go tooling without implementing startup behavior assigned to later tasks. Finish by running `go build ./...` and `go vet ./...`.

---

### 2. Specify command routing and usage failures

**Type**: RED  
**Output**: Failing table-driven tests cover `sqlite <file>`, `d1`, missing arguments, unexpected arguments, help, output streams, and exit status 2.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused table-driven tests in `internal/cli` for the command-routing and usage contracts in Issue #1 and the CLI behavior and Language and stack sections of `Notes/PRD-sqloid.md`. Cover `sqlite <file>` and `d1` dispatch, missing and unexpected arguments, help behavior, stdout versus stderr, and usage failures returning exit status 2. Structure the tests around the PRD-mandated `mow.cli` command model while keeping database startup behind injectable handlers and leaving `cmd/sqloid` as a thin process boundary.

---

### 3. Implement the mow.cli command shell

**Type**: GREEN  
**Output**: Routing and usage tests pass using the PRD-mandated `mow.cli` command structure.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement only enough command construction, argument parsing, help handling, and dispatch in `internal/cli` to make the routing and usage tests pass with the PRD-mandated `mow.cli` structure. Preserve injectable handlers for `sqlite` and `d1`, keep status and stream ownership explicit, and use `cmd/sqloid` only to invoke the CLI package so later work can connect `internal/connection` and `internal/d1` without replacing the shell.

---

### 4. Specify version and silent-startup output

**Type**: RED  
**Output**: Failing tests require exact `sqloid <version>\n` output and no CLI-added success output.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the `internal/cli` and process-boundary tests around `cmd/sqloid` to specify the remaining Issue #1 output contracts. Require version requests to write exactly `sqloid <version>\n` to the correct stream and require successful `sqlite <file>` and `d1` dispatch to add no CLI-authored output, while retaining the established help, usage, and exit-status behavior from the `mow.cli` command shell.

---

### 5. Implement version and startup output contracts

**Type**: GREEN  
**Output**: Exact version and silent-startup tests pass.  
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the version and successful-startup output behavior in `internal/cli`, with `cmd/sqloid` continuing to act only as the executable entrypoint. Produce the exact version line required by Issue #1, avoid adding any output after successful handler dispatch, and preserve the PRD-mandated `mow.cli` routing, help, stream, and usage-failure contracts already covered by tests.

---

### 6. Document the CLI contract

**Type**: DOCUMENT  
**Output**: Wiki documentation describes commands, flags, streams, and exit statuses.  
**Depends on**: 5

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #1 implementation and tests into the appropriate pages under `Notes/wiki`. Document the `sqlite <file>` and `d1` commands, supported help and version flags, exact stream ownership, exit statuses, silent successful startup, the roles of `internal/cli` and `cmd/sqloid`, and the PRD-mandated `mow.cli` structure; update the wiki index and append-only log as required by the wiki rules.

---

### 7. Create the CLI code walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists under `Notes/walkthroughs/001-07/code-walkthrough`.  
**Depends on**: 6

Use showboat, consulting `uvx showboat --help`, to create the walkthrough under `Notes/walkthroughs/001-07/code-walkthrough`. Demonstrate the relevant `internal/cli` and `cmd/sqloid` tests and commands, including routing, help, exact version output, usage failures and status 2, stream selection, silent successful dispatch, and successful `go build ./...` and `go vet ./...`, with references to Issue #1 and `Notes/PRD-sqloid.md`.

---
