# Tasks for #66: Gate SELECT renderers on the authoritative runnable report

Parent issue: #66
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify rejection across the SELECT renderer family

**Type**: RED  
**Output**: Failing table tests require SelectSQL, PageSQL, CountSQL, and their parameter accessors to emit empty output for every non-runnable SELECT class.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

After Issue #65 is complete, add a shared rejection matrix in the existing SELECT/page/count renderer tests under `internal/querybuilder`, using `internal/querybuilder/runnable_test.go` builders as the authoritative validity source. Cover missing command/table/projection, stale table, stale named Value and aggregate projections, incomplete or stale WHERE, every invalid grouping and ORDER BY class, malformed/zero/negative/overflow Limit, and any open value state. For each case first assert `RunnableReport().Runnable` is false, then require `SelectSQL`, `PageSQL`, and `CountSQL` to return empty strings and `SelectParams`, `PageParams`, and `CountParams` to return no parameters, including cases whose component state is locally formattable. Keep this task test-only and avoid independently restating renderer validity rules in test helpers.

---

### 2. Gate SELECT, page, and count output at one authority

**Type**: GREEN  
**Output**: Every SELECT-family renderer and parameter accessor refuses output unless RunnableReport accepts the SELECT state.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `internal/querybuilder/select_sql.go`, `internal/querybuilder/page_sql.go`, and the existing count renderer file so all public SELECT-family SQL and parameter methods first require the selected command to be SELECT and the same authoritative `RunnableReport` to be runnable. Preserve the shared `renderSelectCore` assembly seam for accepted state and make rejected methods return only empty SQL and nil parameters, never partially rendered clauses or values. Avoid recursive report/render dependencies, duplicate stale-identifier checks, or renderer-specific interpretations of validity. Implement only enough to make Task 1 pass, building on Issue #65's projection validation rather than reproducing it.

---

### 3. Specify valid SELECT rendering regressions

**Type**: RED  
**Output**: Exact regression tests lock valid SELECT, paging, count, Limit/OFFSET, quoting, rowid fallback, and parameter order after gating.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Expand the existing valid-case tests in `internal/querybuilder` for `SelectSQL`, `PageSQL`, and `CountSQL`. Require unchanged exact SQL and fresh parameter slices for wildcard, `COUNT(*)`, named Value and aggregate projections, quoted identifiers, completed WHERE values, grouped/aggregate ordering, user Limit, page-size clamping, nonzero OFFSET, count wrapping, and the eligible page-only `ORDER BY rowid` fallback. Include non-SELECT UPDATE/DELETE/INSERT builders whose fields could otherwise render as SELECT fragments and require the full SELECT family to remain empty. Verify parameter order and typed values agree across SELECT/page/count while paging literals add no bindings. Keep this task test-only and use accepted `RunnableReport` state as a prerequisite for every positive case.

---

### 4. Consolidate accepted-state renderer flow

**Type**: GREEN  
**Output**: Valid renderer regressions pass with one consistent accepted-state core and unchanged SQL semantics.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Refine the SELECT rendering implementation under `internal/querybuilder` only as needed for Task 3, keeping one internal core for projection/table/WHERE/GROUP BY/ORDER BY and thin public methods for base SELECT, page ranges, and count wrapping. Ensure the runnable gate executes before extracting parameters or applying rowid fallback, user Limit, page Limit/OFFSET, or count semantics, and that accepted builders retain exact quoting and binding order. Do not weaken `RunnableReport`, infer validity from nonempty rendered fragments, or introduce UI/connection dependencies. Run the querybuilder package tests after the consolidation and preserve all existing range validation for invalid page size/offset independently of builder validity.

---

### 5. Document authoritative SELECT renderer gating

**Type**: DOCUMENT  
**Output**: Wiki documentation records the single runnable authority, rejected output contract, and unchanged valid SELECT/page/count semantics.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #66 and the completed Issue #65 dependency from the SELECT, page, count, runnable, and test files under `internal/querybuilder` into the appropriate `Notes/wiki` pages. Document that every public SELECT-family SQL and parameter method emits nothing unless the selected SELECT passes `RunnableReport`, enumerate the rejected classes without creating a second validator, and record valid quoting, parameter order, Limit/OFFSET clamping, rowid fallback, and complete-limited-result count behavior. Cross-reference Issues #65-#66 and the Query Grammar, Runnable-State Contract, safe rendering, schema revalidation, QueryBuilder Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting history.

---

### 6. Create the SELECT-renderer gate walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/066-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/066-06/code-walkthrough`, with the main file named `walkthrough.md`. Exercise representative and boundary cases from every rejected runnable class, including Issue #65 stale projections, and show empty SQL plus no parameters from SELECT, page, and count renderers. Demonstrate that non-SELECT commands are also rejected, then run valid wildcard, `COUNT(*)`, quoted named/aggregate, WHERE, grouping/order, user-Limit, nonzero-page, rowid-fallback, and count cases with exact SQL and binding order. Explain the single authoritative gate and the #65-before-#66 dependency, reference Issue #66 and `Notes/PRD-sqloid.md`, and keep all generated artifacts in the approved directory.

---
