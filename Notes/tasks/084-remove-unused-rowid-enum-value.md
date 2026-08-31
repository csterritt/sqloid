# Tasks for #84: Remove the unused rowid enum value

Parent issue: #84
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns a lightweight cleanup walkthrough with focused verification output; no interactive TUI demonstration is required.

## Tasks

### 1. Lock observable RowidCapability behavior

**Type**: REFACTOR
**Output**: Passing schema tests lock the three meaningful capability strings, zero/unknown diagnostics, and unchanged object classification before enum cleanup.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/schema/catalog_test.go` with observable enum behavior that pins exact `String()` output for `RowidHas`, `RowidWithout`, and `RowidNotApplicable`, plus diagnostic output for zero and a representative unknown value. Retain or expand catalog fixtures to prove ordinary tables, WITHOUT ROWID tables, virtual tables, and views keep their current classifications. Run this behavioral safety net before production edits and record that it is green. Do not inspect the declaration with an AST/source unit test; declaration typing, nonzero sequence, and removal of `RowidApplicable` are supplemental compile/static checks owned by Task 2.

---

### 2. Remove the unused rowid capability constant

**Type**: REFACTOR
**Output**: `RowidCapability` exposes only the three PRD-defined typed values with zero reserved as unset, and all schema classification tests pass unchanged.
**Verification obligation**: Task 1's schema behavior and classifications pass unchanged.
**Supplemental checks**: Repository build catches remaining removed-symbol references; focused source/static evidence confirms explicit typing at `iota + 1` and the obsolete identifier's absence.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the `RowidCapability` constant block and comments in `internal/schema/schema.go`: remove `RowidApplicable`, explicitly type `RowidHas` as `RowidCapability` beginning at `iota + 1`, and preserve the existing values' string forms and zero as an unknown/unset sentinel. Adjust only tests or references that directly depended on the obsolete identifier; do not change `BuildCatalog` classification in `internal/schema/catalog.go`, rowid-shadow detection, write eligibility, ordering behavior, or serialized/user-facing names. Use the project's `ObjectKind`, `RefreshStatus`, and `RevalidateStatus` typed enum patterns as references. Run the unchanged focused `internal/schema` behavioral tests and repository-wide Go test and build commands so compile-time uses of the removed symbol are caught. Use a focused source/static check as supplemental evidence that `RowidHas` is explicitly typed at `iota + 1`, the sequence remains typed, and `RowidApplicable` is absent; do not encode declaration shape in a unit test.

---

### 3. Check documentation after the rowid cleanup

**Type**: DOCUMENT  
**Output**: Existing schema documentation remains accurate, with only directly obsolete enum claims corrected.
**Depends on**: 2

Review the relevant schema pages under `Notes/wiki` against Issue #84's enum cleanup and verification. If an existing page names `RowidApplicable` or claims a four-value capability model, minimally correct that page to describe the three meaningful values and zero sentinel. Otherwise record that no documentation change is required and leave the wiki, index, and dated ingest log untouched; this mechanical refactor does not justify a new page or ceremonial ingest entry. Task 4's lightweight walkthrough is the durable implementation and verification artifact.

---

### 4. Create the rowid enum walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/084-04/code-walkthrough`.  
**Depends on**: 3

Create a lightweight cleanup walkthrough using showboat, consulting `uvx showboat --help`, at exactly `Notes/walkthroughs/084-04/code-walkthrough`, with the main file named `walkthrough.md`. Show the typed three-value constant declaration, demonstrate exact strings and zero/unknown diagnostics, and run catalog fixtures for an ordinary rowid table, WITHOUT ROWID table, virtual table, and view to prove classifications did not move. Include a repository search showing `RowidApplicable` is absent and focused passing schema verification. Reference Issue #84 and `Notes/PRD-sqloid.md`, and keep every generated artifact within the approved walkthrough directory.

---
