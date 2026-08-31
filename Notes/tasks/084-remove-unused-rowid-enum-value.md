# Tasks for #84: Remove the unused rowid enum value

Parent issue: #84
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the three-value RowidCapability contract

**Type**: RED  
**Output**: Failing schema tests require exactly three typed nonzero rowid capabilities, zero/unknown diagnostics, and unchanged object classification.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/schema/catalog_test.go` or add a focused schema enum contract test that inspects the `RowidCapability` declaration in `internal/schema/schema.go` and proves the public meaningful set contains only `RowidHas`, `RowidWithout`, and `RowidNotApplicable`. Require `RowidHas` to be explicitly typed as `RowidCapability` and start at the first nonzero value, the remaining constants to continue that typed sequence, and the obsolete `RowidApplicable` identifier to be absent. Pin exact `String()` output for all three values plus diagnostic output for zero and a representative unknown value. Retain or expand catalog fixtures to prove ordinary tables, WITHOUT ROWID tables, virtual tables, and views keep their current classifications. Keep the task test-only and ensure the source-contract assertion fails against the current alignment-only constant.

---

### 2. Remove the unused rowid capability constant

**Type**: GREEN  
**Output**: `RowidCapability` exposes only the three PRD-defined typed values with zero reserved as unset, and all schema classification tests pass unchanged.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the `RowidCapability` constant block and comments in `internal/schema/schema.go`: remove `RowidApplicable`, explicitly type `RowidHas` as `RowidCapability` beginning at `iota + 1`, and preserve the existing values' string forms and zero as an unknown/unset sentinel. Adjust only tests or references that directly depended on the obsolete identifier; do not change `BuildCatalog` classification in `internal/schema/catalog.go`, rowid-shadow detection, write eligibility, ordering behavior, or serialized/user-facing names. Use the project's `ObjectKind`, `RefreshStatus`, and `RevalidateStatus` typed enum patterns as references. Run focused `internal/schema` tests and the repository-wide Go test and build commands so compile-time uses of the removed symbol and inferred untyped constants are caught.

---

### 3. Document the rowid capability cleanup

**Type**: DOCUMENT  
**Output**: The wiki presents the exact three-value rowid capability model, zero sentinel behavior, and unchanged catalog classifications.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #84's enum cleanup and verification from `internal/schema/schema.go`, `catalog.go`, and the affected schema tests into the appropriate schema pages under `Notes/wiki`. Record that the only meaningful values are has-rowid, without-rowid, and not-applicable; zero is an unset/unknown diagnostic sentinel; and ordinary, WITHOUT ROWID, virtual-table, and view classification remains unchanged. Cross-reference Issues #9, #21, and #84 and the Schema scope, cache, and validation, Paging consistency, and Testing Decisions sections of `Notes/PRD-sqloid.md`. Update `Notes/wiki/index.md` for page additions or removals and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting earlier history.

---

### 4. Create the rowid enum walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/084-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/084-04/code-walkthrough`, with the main file named `walkthrough.md`. Show the typed three-value constant declaration, demonstrate exact strings and zero/unknown diagnostics, and run catalog fixtures for an ordinary rowid table, WITHOUT ROWID table, virtual table, and view to prove classifications did not move. Include a repository search showing `RowidApplicable` is absent and focused passing schema verification. Reference Issue #84 and `Notes/PRD-sqloid.md`, and keep every generated artifact within the approved walkthrough directory.

---
