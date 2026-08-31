# Tasks for #63: Report an actionable cause for atomic-save short writes

Parent issue: #63
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify nil-error short-write handling

**Type**: RED  
**Output**: Failing export/UI tests require `io.ErrShortWrite` for `(n < len(data), nil)` and preserve destination and cleanup behavior.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/export/save_write_test.go` with an injected `SaveFile.Write` result that returns a positive short byte count and nil error independently of the existing non-nil write-error case. Require `WriteAtomic` to return a `*StageError` at `StageWrite` whose cause matches `io.ErrShortWrite` through `errors.Is` and whose text contains an actionable short-write cause rather than `<nil>`. Assert sync, final close-as-success, and rename never run; the partially written destination-local temporary file is closed and removed; an existing destination remains byte-for-byte unchanged; and a previously missing destination remains absent. Retain complete-write and non-nil-error control rows. Drive the typed failure through `internal/ui/save_write.go` to require one inline failure with the captured destination/payload and intact retry/cancel behavior, never a success message. Keep this task test-only and extend the Issue #53 fake filesystem rather than using real disk exhaustion.

---

### 2. Convert nil-error short writes to `io.ErrShortWrite`

**Type**: GREEN  
**Output**: Atomic-save short writes carry a write-stage `io.ErrShortWrite` cause and follow existing pre-rename cleanup.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the single-write boundary in `internal/export/save_write.go` so a returned count smaller than the supplied immutable payload with no error is converted to `io.ErrShortWrite` before constructing `StageError`. Preserve a non-nil writer error as the cause when one exists, retain the `StageWrite` attribution, and route both cases through the exact existing close/remove cleanup before any sync or rename. Do not retry the write, expose partial output, touch the existing destination, alter complete-write behavior, or change UI retry/cancel identity. Implement only enough to make Task 1 pass, run focused export/UI tests and the established Go verification command, and leave the broader overwrite-intent boundary to blocked Issue #64.

---

### 3. Document atomic-save short-write failures

**Type**: DOCUMENT  
**Output**: Wiki documentation records short-write detection, `io.ErrShortWrite`, stage attribution, preservation, cleanup, and UI retry/cancel behavior.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #63 implementation and tests from `internal/export` and `internal/ui` into the atomic-save pages under `Notes/wiki`. Document that `(n < len(payload), nil)` is a failed write with an `io.ErrShortWrite` cause, remains a typed write-stage error, runs the same pre-rename temporary close/removal path as other write failures, preserves any existing destination, and cannot claim success. Record the distinction from a non-nil write error and a complete write, plus inline retry/cancel with the immutable capture. Cross-reference Issues #53 and #63, user story 72, and the Atomic saves, Export Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the short-write walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/063-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/063-04/code-walkthrough`, with the main file named `walkthrough.md`. Deterministically inject `(n < len(payload), nil)` and display the `StageWrite` error, `errors.Is(io.ErrShortWrite)`, actionable UI text, call ordering, absence of sync/rename, destination preservation, and temporary-file cleanup. Contrast a non-nil write error and a complete successful write, then show retry/cancel retaining the immutable save capture. Note that Issue #64 builds on this persistence boundary and must preserve the demonstrated behavior. Reference Issues #53, #63, and #64 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
