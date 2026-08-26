# Tasks for #17: Guided WHERE predicates and SQL NULL semantics

Parent issue: #17
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify reusable WHERE predicate state

**Type**: RED
**Output**: Failing QueryBuilder tests cover eligible columns, every operator, no-value null operators, bound values, typed `NULL`, empty text, and verbatim LIKE wildcards.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create UI-independent, table-driven predicate tests in `internal/querybuilder/predicate_test.go`, using eligible typed column identities from `internal/schema` and the closed operator and universal-value contracts from Issue #14. Cover every documented operator for every eligible column, deterministic operator ordering, and structurally distinct absent, incomplete, and complete predicate states. Require `IS NULL` and `IS NOT NULL` to become complete immediately after operator selection, emit no placeholder and no parameter, and reject or discard any stale value state; require all other operators to remain incomplete until value submission and then emit one placeholder with the exact parsed bound value and concrete bound type. Assert safely quoted schema-derived identifiers and reusable predicate SQL/parameter output for SELECT, UPDATE, and DELETE consumers. Include typed `NULL` and empty input as TEXT parameters, ordinary comparisons and LIKE retaining SQLite SQL-NULL behavior rather than rewriting to null operators, and LIKE `%` and `_` text bound byte-for-byte without escaping or interpolation. Keep this task test-only, preserve immutable before/after state, and exclude multiple predicates, AND/OR, IN, type-based filtering, and database execution.

---

### 2. Implement WHERE predicate construction

**Type**: GREEN
**Output**: Pure predicate SQL/parameter tests pass for SELECT, UPDATE, and DELETE consumers.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the reusable immutable predicate state and transitions in `internal/querybuilder/predicate.go`, integrating command-owned optional WHERE state and SQL generation through `internal/querybuilder/builder.go` and the existing safe SQL atom/value APIs from Issue #14. Derive column choices from eligible `internal/schema` metadata, expose the complete fixed operator set without declared-type filtering, and model value-required versus no-value operators structurally rather than through UI labels. Render safely quoted column plus fixed operator, append exactly one bound parameter for completed value operators, and append none for `IS NULL` or `IS NOT NULL`; preserve entered text and parsed concrete type for typed `NULL`, empty TEXT, and LIKE wildcard input. Provide one predicate rendering/parameter contract consumed unchanged by SELECT and by later UPDATE/DELETE construction, preserve deterministic parameter ordering at each consumer boundary, reject invalid typed identities or unknown operators defensively, avoid imports from `internal/ui`, and implement only enough to make Task 1 pass.

---

### 3. Specify the guided WHERE UI flow

**Type**: RED
**Output**: Failing scripted tests cover column→operator→value navigation, conditional value entry, inline SQL-NULL guidance, restoration, and completion.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted Bubble Tea model tests in `internal/ui/where_flow_test.go` and focused view assertions in `internal/ui/where_view_test.go`, following Issue #12's reusable popup contract, Issue #14's universal value-entry contract, and the focus and key-precedence rules in `Notes/PRD-sqloid.md`. For SELECT, UPDATE, and DELETE WHERE consumers, require the guided sequence to open a searchable eligible-column popup, then a scroll-only operator popup containing every operator in deterministic order, then universal text entry only for value-taking operators. Require selecting either null operator to complete the predicate without opening value input and to return focus to the next command-specific builder field; require value submission to preserve exact entered text and bound type, complete the predicate, and advance identically. Assert Shift+Tab/arrow revisiting restores the selected column, operator, exact text, and cursor/input state appropriate to the established universal-entry seam; Esc restores the prior completed value and exact opener focus without partial commits. Require the value prompt to show inline guidance that typed `NULL` is TEXT and direct SQL-null intent to `IS NULL`/`IS NOT NULL`, with contextual help also explaining that ordinary comparisons and LIKE do not match actual SQL NULL values and that `%`/`_` keep SQLite wildcard meaning. Keep this task test-only, assert popup search/highlight/viewport reset and restoration behavior, and do not duplicate QueryBuilder validity or parsing rules in UI expectations.

---

### 4. Integrate WHERE popups and value entry

**Type**: GREEN
**Output**: End-to-end predicate flow tests pass using shared popup and universal parsing contracts.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire the shared QueryBuilder predicate transitions into `internal/ui/where_popup.go`, `internal/ui/value_input.go`, `internal/ui/model.go`, and the existing builder/view rendering seams. Populate WHERE columns from refreshed eligible `internal/schema` identities through QueryBuilder, configure column selection as searchable and operator selection as scroll-only using Issue #12's shared popup implementation, and route accepted choices by the predicate transition result rather than re-encoding operator behavior in the UI. Open Issue #14's universal value input only for value-taking operators, commit exact entered representation and parsed type through QueryBuilder, and let null operators complete with no value prompt or parameter. Restore completed state and exact opener focus when revisiting or cancelling, preserve overlay non-reflow and global key precedence, and render the inline SQL-NULL hint plus contextual comparison/LIKE guidance without treating typed `NULL` specially in parsing. Make Tasks 1 and 3 pass for SELECT and the reusable UPDATE/DELETE seams, while leaving write-specific assignment, execution, and destructive preparation behavior to later issues.

---

### 5. Document WHERE and NULL semantics

**Type**: DOCUMENT
**Output**: Wiki documentation records operators, typed TEXT `NULL`, SQL-null operators, comparisons, and LIKE behavior.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #17 implementation and tests from `internal/querybuilder`, `internal/ui`, and the consumed `internal/schema` and Issue #14 value contracts into the appropriate pages under `Notes/wiki`. Document the single optional predicate shape shared by SELECT/UPDATE/DELETE, eligible-column selection, the complete fixed operator list, conditional value entry, and the no-value/no-parameter behavior of `IS NULL` and `IS NOT NULL`. Explain that typed `NULL` and empty input are TEXT under universal parsing, ordinary comparisons and LIKE do not match actual SQL NULL values, SQL-null intent uses the explicit null operators, and LIKE binds `%` and `_` verbatim with SQLite wildcard semantics and no v1 escape mechanism. Record immutable state/restoration, safe identifier quoting and parameter binding, inline hint/help ownership, and the absence of type filtering, multiple predicates, AND/OR, and IN. Cross-reference Issues #12, #14, and #17 and the Query Grammar, Builder and Display Interaction, Numeric value parsing and rendering, SQL safety, QueryBuilder, UI, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the WHERE-flow walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/017-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/017-06/code-walkthrough`. Demonstrate the reusable SELECT/UPDATE/DELETE predicate state with an eligible column and every fixed operator, showing that only `IS NULL` and `IS NOT NULL` bypass value entry and produce no bound parameter. Walk through column → operator → value focus, popup acceptance/cancellation, completion, revisiting, and exact restoration. Show typed `NULL` and empty input remaining TEXT with exact binding types, the inline SQL-NULL hint and contextual explanation, ordinary comparison/LIKE SQL-null semantics, and LIKE values containing `%` and `_` bound verbatim and absent from SQL text. Include safely quoted identifiers and exact SQL/parameter evidence from pure and scripted tests, reference Issue #17 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
