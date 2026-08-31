# Tasks for #62: Pass the leased connection to the rollback test hook

Parent issue: #62
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify rollback-hook leased-connection identity

**Type**: RED  
**Output**: Failing barrier tests require begin, execute, commit, and rollback hooks for one write to observe the same non-nil leased connection.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the barrier-based phased-write tests in `internal/connection/write_test.go` and `commit_boundary_test.go` so every test hook records the exact `*sql.Conn` identity. Drive statement failure, pre-COMMIT cancellation, rollback failure/outcome unknown, and any existing failed-COMMIT cleanup path into `beforeWriteRollback`; require its argument to be non-nil and pointer-identical to the connection observed by `beforeWriteBegin`, `beforeWriteExec`, `beforeWriteCommit` when reached, and `writeLeaseHook` for the same execution. Retain assertions for phase order, exactly one rollback-hook call, true settlement before lease release, confirmed versus unresolved rollback classification, scoped cancellation, and later lease reuse. Keep this task test-only and use existing channels/barriers rather than timing sleeps.

---

### 2. Pass the transaction's lease into rollback cleanup

**Type**: GREEN  
**Output**: Rollback hooks receive the owning leased connection with no production behavior change.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the rollback helper and its callers in `internal/connection/write.go` to carry the same `*sql.Conn` already passed into the phased write runner, and invoke `beforeWriteRollback` with that connection instead of `nil`. Do not reacquire a connection, expose a pooled replacement, move the hook outside rollback-cleanup phase, or change the production transaction sequence. Preserve context, noncancellable boundary, phase delivery, commit/rollback error precedence, `RollbackConfirmed`, cancellation classification, settlement, and lease release exactly as covered by Task 1. Keep the seam test-only and nil in production, then run focused connection tests with the race detector and the established Go verification command.

---

### 3. Document phased-write hook identity

**Type**: DOCUMENT  
**Output**: Wiki documentation records one leased connection across all write phases and the rollback barrier's observability-only role.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #62 implementation and tests from `internal/connection/write.go` and its barrier suites into the write-transaction pages under `Notes/wiki`. Document that BEGIN, statement execution, pre-COMMIT, and rollback cleanup for one write remain on one dedicated leased `*sql.Conn`; explain that all phase hooks observe that identical connection solely for deterministic tests and are nil in production. Record unchanged cancellation, noncancellable cleanup, confirmed/unresolved rollback, settlement, release, and reuse behavior. Cross-reference Issue #62, user stories 45 and 82, and the Writes and commit boundary, Connection Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the rollback-hook walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/062-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/062-04/code-walkthrough`, with the main file named `walkthrough.md`. Run the synchronized write cases for statement failure, pre-COMMIT cancellation, confirmed rollback, and unresolved rollback; display the non-nil connection identity seen by each reached begin/execute/commit/rollback hook and prove all identities match the owning lease. Capture unchanged phase order, classification, settlement-before-release, and successful later reuse. Reference Issue #62 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
