# Tasks for #60: Classify cancellation before lease acquisition correctly

Parent issue: #60
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify cancellation while waiting for a lease

**Type**: RED  
**Output**: Failing synchronized tests require cancelled outcomes for `RunRequest`, started SELECT requests, and writes that are cancelled before lease acquisition.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic tests in `internal/connection` that acquire and hold both connections from the exact-two pool, start a third `RunRequest`, `startRequest`-backed first-page/page/count operation, or `StartWrite`, wait until it is queued for a lease using synchronization rather than sleeps, then cancel its context before releasing either holder. Require general and started requests to settle exactly once as `OutcomeCancelled`, writes as `WriteCancelled`, no operation callback, BEGIN, statement, transaction hook, or phase work to start, and no replacement work before settlement. Add direct classification rows for wrapped `context.Canceled`, a typed `HealthError`, and an ordinary lease failure to prove cancellation precedence without masking health or changing non-cancellation errors. Finally release the holders and require both pool connections and a subsequent request/write to remain usable. Keep this task test-only and extend `request_test.go`, `select_cancellation_test.go`, `started_request.go` coverage, and `write_test.go` barrier conventions.

---

### 2. Classify pre-lease cancellation at every entry point

**Type**: GREEN  
**Output**: General, started SELECT, and write acquisition failures consistently return their existing cancelled outcomes.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the lease-acquisition error branches in `internal/connection/health.go`, `started_request.go`, and `write.go` so an error matching `context.Canceled` is recognized before typed health and ordinary failure handling. Return the existing `OutcomeCancelled` or `WriteCancelled` shape with the cancellation cause preserved where the surrounding result contract permits it, while retaining health precedence for genuine `HealthError` values and unchanged handling for all other lease/configuration errors. Do not create a `Request`, emit write phases, begin transaction work, invoke execution hooks, dispatch an interrupt, or release a lease that was never acquired. Preserve exactly-once settlement channels and later pool usability. Implement only enough to make Task 1 pass, then run focused race-enabled connection tests and the established Go verification command.

---

### 3. Document pre-lease cancellation semantics

**Type**: DOCUMENT  
**Output**: Wiki documentation records cancellation classification and ordering before lease acquisition across every database entry point.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #60 implementation and tests from `internal/connection` into the request/cancellation pages under `Notes/wiki`. Document pool saturation, context cancellation during `database/sql` lease acquisition, classification ordering before health/general failures, and the existing outcome type used by synchronous requests, started SELECT work, and writes. State that pre-lease cancellation starts no callback, transaction, phase, replacement work, or interrupt, settles exactly once, and leaves later requests usable; distinguish wrapped cancellation, typed health errors, and ordinary acquisition failures. Cross-reference Issue #60, user stories 12, 14, and 82, and the cancellation, connection-pool, Identities and state, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the pre-lease-cancellation walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/060-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/060-04/code-walkthrough`, with the main file named `walkthrough.md`. Saturate both leases under deterministic barriers and show cancellation of queued `RunRequest`, first-page/page/count started work, and `StartWrite`, including exact outcomes, no operation/transaction phases, settlement before replacement, and successful later reuse. Contrast wrapped cancellation with health and ordinary lease failures. After Issue #57, demonstrate the shipped TUI cancellation presentation for one queued read or write and its successful follow-up request. Reference Issues #57 and #60 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
