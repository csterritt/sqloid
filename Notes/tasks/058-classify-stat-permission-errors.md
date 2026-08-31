# Tasks for #58: Classify permission-denied stat failures as unreadable

Parent issue: #58
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify startup stat-error classification

**Type**: RED  
**Output**: Failing injected startup and CLI tests distinguish EACCES/EPERM stat failures from missing and unrelated stat errors.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic table-driven tests in `internal/connection` around the initial path-stat boundary, introducing the narrowest test seam needed to return wrapped `syscall.EACCES`, `syscall.EPERM`, `fs.ErrNotExist`, and an unrelated stat error without depending on the test user's permissions. Require EACCES and EPERM, including `*os.PathError` wrapping, to produce `*StartupError` with `FailureUnreadable`, the original cause available through `errors.Is`/`errors.As`, and exactly `<path>: permission denied`; require only `os.IsNotExist` errors to retain `FailureMissing` and the missing-file diagnostic. Pin the existing behavior for directories, header/readability failures, and raw non-not-existence stat causes so they are not silently relabeled missing. Extend the existing injected CLI startup tests to require one stderr line, status 1, the exact named path, and no file creation. Keep this task test-only and follow `internal/connection/opener_test.go`, `startup_test.go`, and `internal/cli/startup_test.go`.

---

### 2. Classify stat failures by their preserved cause

**Type**: GREEN  
**Output**: Startup reports permission-denied stat failures as unreadable while preserving every existing distinct classification.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Refactor only the initial stat step in `internal/connection/startup.go` and its minimal injectable boundary so Task 1 passes. Classify an error as `FailureMissing` only when `os.IsNotExist` matches; classify EACCES/EPERM permission failures as `FailureUnreadable` while retaining the wrapped OS cause; preserve unrelated errors in an actionable non-missing startup classification rather than fabricating absence. Continue rendering through `StartupError.Error` so permission failures produce the existing exact path-specific line and the CLI remains the sole printer. Do not reorder existence/readability/header/read-write/probe validation, alter directory or mode=rw behavior, add fallback creation, or change D1 discovery diagnostics. Run focused connection/CLI tests and the established Go verification command.

---

### 3. Document startup stat-error distinctions

**Type**: DOCUMENT  
**Output**: Wiki documentation records missing versus stat-time permission denial, preserved causes, ordered validation, and exact CLI behavior.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #58 implementation and tests from `internal/connection` and `internal/cli` into the startup/error pages under `Notes/wiki`. Document that only `os.IsNotExist` at stat is missing, EACCES/EPERM from the file or denied parent traversal is unreadable/permission denied, causes remain inspectable through wrapping, and unrelated stat failures are not converted to absence. Preserve the full validation order and distinguish stat/readability diagnostics from mode=rw open diagnostics. Record exactly-one stderr line, status 1, and no file creation. Cross-reference Issue #58, user stories 3 and 7, and the Startup validation and errors and CLI behavior sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the stat-permission walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/058-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/058-04/code-walkthrough`, with the main file named `walkthrough.md`. Run the deterministic stat-boundary cases for EACCES, EPERM, wrapped missing, and an unrelated failure; show typed classifications, preserved causes, and exact rendered lines. Exercise the CLI boundary to capture one permission-denied stderr line and status 1 without file creation, then contrast a genuinely missing path and one unchanged directory/header or mode=rw case. Reference Issue #58 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
