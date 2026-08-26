# Tasks for #4: Exact D1 discovery diagnostics

Parent issue: #4
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify exact D1 failure diagnostics

**Type**: RED  
**Output**: Failing golden CLI tests cover missing, unreadable, empty, and multiple-candidate directories, including exact lines, stderr, hints, and exit status 1.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add golden tests in `internal/cli`, with typed discovery outcomes supplied by `internal/d1` and process behavior through `cmd/sqloid` where needed, for every Issue #4 failure fixture. Cover missing, unreadable, empty, candidate-free, and multiple-candidate Wrangler directories; assert exact spelling and line counts from the D1 discovery section of `Notes/PRD-sqloid.md`, stderr ownership, expected-path and `sqloid sqlite <file>` hints only for zero candidates, exit status 1, no call to `internal/connection`, and no database creation.

---

### 2. Implement D1 diagnostic mapping

**Type**: GREEN  
**Output**: Zero-candidate cases emit the exact two lines; multiple candidates emit only the exact single line; no database is created.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the mapping from typed `internal/d1` zero- and multiple-candidate outcomes to exact process-facing diagnostics in `internal/cli`, while leaving `cmd/sqloid` as the thin entrypoint. For missing, unreadable, empty, and candidate-free directories, emit exactly the required two stderr lines with the expected Wrangler path and explicit `sqloid sqlite <file>` recovery guidance; for multiple exact `.sqlite` candidates, emit only the required single stderr line with no hint. Return status 1 and bypass `internal/connection` without creating a target for every discovery failure.

---

### 3. Document D1 startup recovery

**Type**: DOCUMENT  
**Output**: Wiki documentation explains the expected path and when explicit opening is suggested.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #4 implementation and golden tests into the appropriate pages under `Notes/wiki`. Document the expected working-directory-relative Wrangler path, zero- and multiple-candidate outcomes, exact stderr line counts, status 1, when the explicit `sqloid sqlite <file>` recovery hint appears or is omitted, and the boundaries among `internal/d1`, `internal/cli`, `internal/connection`, and `cmd/sqloid`; update the wiki index and append-only log as required.

---

### 4. Create the D1 diagnostics walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough captures exact zero/multiple-candidate behavior at `Notes/walkthroughs/004-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/004-04/code-walkthrough`. Capture missing, unreadable, empty, candidate-free, and multiple-candidate fixtures; demonstrate the exact zero-candidate two-line output, the exact multiple-candidate single-line output, stderr, hint presence and absence, exit status 1, no shared-opener invocation, and non-creation with references to Issue #4 and `Notes/PRD-sqloid.md`.

---

### 5. Review D1 diagnostics

**Type**: REVIEW  
**Output**: Human approves exact spelling, line count, hint presence, streams, and statuses.  
**Depends on**: 4

Review the completed `internal/d1`, `internal/cli`, `internal/connection`, and `cmd/sqloid` behavior, golden tests, wiki updates, and walkthrough against Issue #4 and the D1 discovery section of `Notes/PRD-sqloid.md`. Confirm exact spelling, line count, stderr ownership, status 1, expected-path and explicit-open hint presence for zero candidates, hint absence for multiple candidates, no opening after discovery failure, and no database creation before approving the issue.

---
