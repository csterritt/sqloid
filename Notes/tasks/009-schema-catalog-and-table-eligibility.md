# Tasks for #9: Schema catalog and table eligibility

Parent issue: #9
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify main-schema object cataloging

**Type**: RED
**Output**: Failing catalog/integration tests cover ordinary tables, virtual tables, views, schema version, and exclusion of `sqlite_%` and `_cf_METADATA`.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create table-driven contract tests in `internal/schema/catalog_test.go` and SQLite-backed coverage in `internal/schema/catalog_integration_test.go`, with Connection-boundary test support in `internal/connection/schema_test.go` where database requests must be exercised. Follow the table-driven and `modernc.org/sqlite` integration patterns required by the Testing Decisions in `Notes/PRD-sqloid.md` and the request-boundary patterns established by Issues #5 and #7. Build fixtures containing ordinary tables, virtual tables, views, SQLite-owned objects, and `_cf_METADATA`; require cataloging from `main.sqlite_master` only, accurate object kinds, the current `PRAGMA schema_version`, deterministic eligible results, and both required exclusions. Keep this task test-only, independent of `internal/ui`, and focused on externally observable Schema and Connection contracts.

---

### 2. Implement schema object cataloging

**Type**: GREEN
**Output**: The UI-independent Schema package returns eligible typed objects and schema version.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create the Schema boundary and catalog types in `internal/schema/schema.go` and `internal/schema/catalog.go`, and add only the database request plumbing required by those contracts in `internal/connection/schema.go`. Follow the typed-error, request-boundary health-check, pooled-connection ownership, and driver-hiding patterns established in `internal/connection` by Issues #2, #5, and #7. Query only the main schema, return schema version with typed ordinary-table, virtual-table, and view records, exclude `sqlite_%` and `_cf_METADATA`, preserve deterministic behavior required by the tests, and expose no Bubble Tea or `internal/ui` dependency. Implement only enough production behavior to pass Task 1.

---

### 3. Specify column, rowid, and insertability metadata

**Type**: RED
**Output**: Failing tests cover declared types, visible/hidden/generated columns, rowid capability/shadowing, write eligibility, and SELECT-only views.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/schema/catalog_test.go` and `internal/schema/catalog_integration_test.go`, and update `internal/connection/schema_test.go` only where Connection request behavior needs direct proof. Add fixtures for ordinary and virtual tables, views, `WITHOUT ROWID` tables, declared columns named `rowid`, `_rowid_`, or `oid`, hidden virtual-table columns, generated columns, and zero-insertable-column cases. Require each object to report rowid capability and declared-rowid shadowing, each column to report name, declared type, visibility-related capability, and insertability from `PRAGMA table_xinfo`, views to remain SELECT-only, and write eligibility to distinguish views from eligible ordinary and virtual tables. Assert that hidden and generated columns are noninsertable and that declared type remains metadata rather than type-specific input behavior; keep the task test-only and UI-independent.

---

### 4. Implement object and column capabilities

**Type**: GREEN
**Output**: Complete schema metadata tests pass without introducing type-specific input behavior.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Complete `internal/schema/schema.go`, `internal/schema/catalog.go`, and a focused `internal/schema/metadata.go`, updating `internal/connection/schema.go` only for the SQLite requests needed to satisfy the tested Schema contract. Follow the PRD's Schema module boundary and existing Connection request/error patterns. Populate object kind, write eligibility, rowid capability, declared-rowid shadowing, and column name, declared type, hidden/generated capability, and insertability from authoritative SQLite metadata; treat views as SELECT-only, represent virtual-table behavior explicitly, and handle `WITHOUT ROWID` and all declared rowid aliases correctly. Do not add input widgets, coercion, affinity-based controls, or any dependency on `internal/ui` or `internal/querybuilder`; implement only enough to make Tasks 1 and 3 pass.

---

### 5. Document schema metadata contracts

**Type**: DOCUMENT
**Output**: Wiki documentation records catalog queries, object kinds, column capabilities, exclusions, and UI independence.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #9 files from `internal/schema` and the related `internal/connection` implementation and tests into the appropriate pages under `Notes/wiki`. Document main-schema catalog queries and schema-version capture, ordinary-table/virtual-table/view kinds, write eligibility, rowid capability and shadowing, column declared type and visibility/generated/insertability capabilities, exclusion of `sqlite_%` and `_cf_METADATA`, and the strict absence of UI and type-specific input behavior. Cross-reference Issue #9 and the Schema scope, Schema metadata, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md`, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the schema-catalog walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/009-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/009-06/code-walkthrough`. Demonstrate the completed `internal/schema` catalog and `internal/connection/schema.go` boundary against fixtures containing ordinary tables, virtual tables, views, `WITHOUT ROWID` tables, generated and hidden columns, SQLite system objects, and `_cf_METADATA`. Show evidence for schema version, object kinds, exclusions, write eligibility, rowid capability/shadowing, declared types, and insertability while confirming UI independence and the absence of type-specific input behavior. Reference Issue #9 and the relevant sections of `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---

### 7. Review schema fixtures

**Type**: REVIEW
**Output**: Human confirms ordinary, virtual, view, WITHOUT ROWID, generated, hidden, system, and `_cf_METADATA` behavior.
**Depends on**: 6

Review `internal/schema/schema.go`, `internal/schema/catalog.go`, `internal/schema/metadata.go`, the related `internal/connection/schema.go` seam, tests, wiki updates, and walkthrough against Issue #9 and the PRD Schema contracts. Open and inspect fixtures for every listed object and column category, confirm exact catalog inclusion and exclusion, object kind, schema version, rowid capability/shadowing, write eligibility, declared type, visibility/generated status, and insertability, and verify views are SELECT-only. Confirm that Schema remains independent of `internal/ui` and `internal/querybuilder` and that no declared-type-specific input behavior was introduced before approving the issue.

---
