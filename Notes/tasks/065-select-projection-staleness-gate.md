# Tasks for #65: Gate stale SELECT projections through the runnable report

Parent issue: #65
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify stale projection runnable-report behavior

**Type**: RED  
**Output**: Failing QueryBuilder tests cover stale value and aggregate projections with exact projection-field feedback while valid projection forms remain runnable.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the table-driven runnable-state coverage in `internal/querybuilder/runnable_test.go`, using its refreshed-catalog fixtures and the projection transitions in `internal/querybuilder/projection_test.go` as references. Commit named Value and each named aggregate projection, replace the catalog with one where that visible column is absent, and require `RunnableReport` to return `Runnable=false`, `RunFieldProjection`, and one specific stale-projection reason before later WHERE, GROUP BY, ORDER BY, or Limit failures. Add controls for valid named Value and aggregate entries, wildcard, and the synthetic `COUNT(*)` sentinel, including schemas with literal `*` and `COUNT(*)` column names so sentinel identity is not confused with named identifiers. Keep this task test-only and construct committed state through existing QueryBuilder transitions rather than changing production validation.

---

### 2. Validate committed named projections against refreshed schema

**Type**: GREEN  
**Output**: The authoritative SELECT runnable report rejects every stale named projection and preserves wildcard and `COUNT(*)` behavior.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `internal/querybuilder/runnable.go` so `reportSelect` validates every committed `ProjectionColumn` entry against the selected object's current visible columns before evaluating WHERE and later SELECT fields. Add the stale-projection reason alongside the existing exact runnable reasons and return it with `RunFieldProjection`; validate both plain and aggregate-bearing named entries by their declared column identity while exempting `ProjectionWildcard` and `ProjectionCountStar`. Reuse the catalog/visibility and projection identity patterns already used by `reportWhere`, `validateGrouping`, and `projection.go` rather than parsing rendered labels or SQL. Make only the production changes needed for Task 1 and preserve visual-order first-invalid reporting.

---

### 3. Specify Enter gating and projection-focus repair

**Type**: RED  
**Output**: Failing Bubble Tea command tests prove stale value and aggregate projections start no request, append no history, and focus Column(s) with exact feedback.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model tests in `internal/ui/runnable_feedback_test.go`, following its `enterPress`, `modelWithQB`, focus, and no-command assertions. Seed builders with committed named Value and aggregate projections, refresh them against a catalog that no longer exposes the projected column without using the state-clearing revalidation helper, then press Enter from other focused fields. Require no schema-validation, estimate, execution, query-history, or result-history work; require focus to move to `columnsFieldLabel` and the inline/view feedback to contain the authoritative stale-projection reason verbatim. Include runnable wildcard, `COUNT(*)`, and current named-projection controls to prove their established pre-execution handoff remains unchanged. Keep this task test-only.

---

### 4. Route stale projection reports through existing UI feedback

**Type**: GREEN  
**Output**: Stale SELECT Enter behavior passes through the normal runnable-report gate with projection focus and no request.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Adjust only the necessary runnable-report-to-focus seam in `internal/ui` if Task 3 exposes a gap, using the existing `RunFieldProjection` mapping and invalid-Enter handling exercised by `internal/ui/runnable_feedback_test.go`. Treat the QueryBuilder report as authoritative: consume Enter, focus Column(s), render its exact reason, and return no command or history mutation. Do not duplicate schema membership checks in the UI, clear the stale projection silently, or alter popup precedence and valid pre-execution routing. If the existing generic mapping already satisfies the tests after Task 2, retain it and limit production changes to comments or no-op cleanup needed to keep the contract explicit.

---

### 5. Document stale SELECT projection gating

**Type**: DOCUMENT  
**Output**: Wiki documentation records refreshed-schema projection validation, sentinel exemptions, first-invalid order, and UI repair behavior.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #65 implementation and tests from `internal/querybuilder/runnable.go`, projection/runnable tests, and `internal/ui/runnable_feedback_test.go` into the appropriate pages under `Notes/wiki`. Document that every committed named Value or aggregate projection is checked against refreshed visible columns before later SELECT fields, while wildcard and synthetic `COUNT(*)` are identity-based valid forms; record the exact typed `RunFieldProjection` result, specific stale feedback, Enter no-request/no-history behavior, and Column(s) repair focus. Cross-reference Issue #65 and the SELECT projection, authoritative runnable-state, schema revalidation, QueryBuilder/UI module, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for page changes and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the stale-projection walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/065-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/065-06/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate a valid named Value projection and named aggregate projection, refresh each against a schema where its column vanished, and show the non-runnable `RunFieldProjection` report with specific feedback. Drive Enter to prove no request or history starts and focus moves to Column(s), then show valid wildcard, synthetic `COUNT(*)`, current named Value, and current named aggregate controls still reaching pre-execution. Include literal unusual column names that distinguish sentinels from named identifiers, reference Issue #65 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
