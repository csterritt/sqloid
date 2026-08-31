# Tasks for #82: Settle malformed schema revalidation attempts

Parent issue: #82
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify malformed Attempt settlement

**Type**: RED  
**Output**: Failing schema tests require zero and unknown `Attempt` statuses to become refresh-failed `Revalidation` values with a concrete cause, nil catalog, and no panic.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/schema/revalidate_test.go` with table-driven cases that send changed-version `Revalidate` calls an exported `Attempt` whose status is zero or an out-of-range `RefreshStatus`, including contradictory payload fields where useful to prove the default status mapping is authoritative. Require `RevalidateRefreshFailed`, a non-nil diagnostic cause that identifies the malformed or unknown attempt status, a nil catalog, exactly one refresh invocation, and no panic. Keep the existing unchanged-pointer reuse, successful refresh, ordinary failure cause preservation, deletion, and replacement mappings pinned unchanged, and assert every constructor-produced attempt continues to produce its documented settled revalidation payload. Keep this task test-only and make the new malformed-status cases fail against the current empty `Revalidation` default branch.

---

### 2. Map malformed attempts to refresh failure

**Type**: GREEN  
**Output**: Every schema revalidation path returns an actionable settled status, and malformed attempts produce a concrete refresh-failed diagnostic without a catalog.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the defensive default branch of `Revalidate` in `internal/schema/revalidate.go` so zero and unknown `Attempt.Status` values map to `RevalidateRefreshFailed` with nil `Catalog` and a newly constructed non-nil diagnostic `Cause`. Follow the existing typed status and `String()` diagnostic conventions in `internal/schema/refresh.go` and `revalidate.go`; do not reinterpret ordinary `RefreshFailed` causes, alter successful catalog pointers, change terminal deletion/replacement payloads, or push malformed-state handling into UI consumers. Keep the result actionable under the existing stale-refresh workflow and implement only enough to make Task 1 and all existing schema and UI revalidation tests pass. Complete this issue before beginning Issue #83 so its invariant encodes this finalized mapping.

---

### 3. Document defensive revalidation settlement

**Type**: DOCUMENT  
**Output**: The wiki documents the complete Attempt-to-Revalidation status map, including malformed-status fallback and unchanged consumer behavior.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #82's completed `internal/schema/revalidate.go` change and `revalidate_test.go` regression coverage into the relevant schema-validation pages under `Notes/wiki`. Record the unchanged, refreshed, ordinary refresh-failed, deleted, and replaced mappings and explain that zero or unknown exported `Attempt` statuses defensively settle as refresh failed with a concrete cause and no catalog rather than leaking an unset status to consumers. Cross-reference Issue #82, Issues #13 and #21, the forthcoming Issue #83 invariant, and the Schema scope, cache, and validation and Testing Decisions sections of `Notes/PRD-sqloid.md`. Update `Notes/wiki/index.md` for any page additions or removals and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting history.

---

### 4. Create the malformed revalidation walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/082-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/082-04/code-walkthrough`, with the main file named `walkthrough.md`. Present the full typed Attempt-to-Revalidation mapping, run the zero and unknown status regression cases, and show each malformed attempt settling to refresh failed with a non-nil diagnostic cause and nil catalog while constructor-produced success, ordinary failure, deletion, and replacement behavior stays unchanged. Include focused passing schema verification and explain why consumers never receive an empty or unknown revalidation status. Reference Issue #82 and `Notes/PRD-sqloid.md`, and keep every showboat artifact inside the approved directory.

---
