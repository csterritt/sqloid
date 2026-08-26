# Tasks for #18: GROUP BY, ORDER BY, and LIMIT rules

Parent issue: #18
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the SELECT grouping matrix

**Type**: RED
**Output**: Failing pure tests cover grouped nonaggregates, mixed projections, all-aggregate queries, wildcard rejection, duplicates, and SQL generation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend UI-independent, table-driven SELECT tests in `internal/querybuilder/group_by_test.go`, using the ordered typed projection entries from Issues #15-16 and stable column identities from `internal/schema`. Cover empty GROUP BY, grouped nonaggregate-only projections, mixed aggregate/nonaggregate projections with every nonaggregate grouped, missing one or more required groups, extra grouped columns, all-aggregate projection with and without GROUP BY, bare `COUNT(*)`, wildcard with no GROUP BY, and wildcard with any GROUP BY. Require assisted multi-selection to preserve Schema-backed selection order, reject duplicate columns as immutable no-ops, and reject stale/foreign identities. Assert exact validity and specific first-invalid field/reason contracts at the QueryBuilder boundary, plus safely quoted SELECT SQL and deterministic GROUP BY order without changing projection order or parameters. Keep this task test-only, distinguish data validity from UI context, and cover the complete grouping boundaries required by the PRD rather than only happy-path SQL rendering.

---

### 2. Implement GROUP BY state and validation

**Type**: GREEN
**Output**: Multi-selection and every valid/invalid grouping combination pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement immutable GROUP BY selection, candidate derivation, SQL generation, and grouping validation in `internal/querybuilder/group_by.go`, integrating SELECT state through `internal/querybuilder/builder.go` and reusing typed projection and safe identifier APIs. Preserve committed group order, prevent exact column duplicates defensively, and derive choices only from current eligible table columns without allowing display labels or stale identities to become SQL. Enforce that every nonaggregate projected column is grouped whenever GROUP BY is nonempty, wildcard cannot coexist with GROUP BY, mixed aggregate/nonaggregate projection without GROUP BY is invalid, and all-aggregate projection without GROUP BY remains valid. Permit extra grouped columns when the matrix allows them, keep projection order unchanged, and append a deterministic quoted GROUP BY clause only when nonempty. Return precise QueryBuilder-owned invalid field/reason data for later runnable reporting, avoid imports from `internal/ui`, and implement only enough to make Task 1 pass.

---

### 3. Specify context-valid ORDER BY behavior

**Type**: RED
**Output**: Failing tests cover grouped columns, selected aggregate expressions, single selection, ASC default, direction toggling, and candidate exclusions.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create pure ORDER BY tests in `internal/querybuilder/order_by_test.go` and scripted base-field/popup tests in `internal/ui/order_by_test.go`, following Issue #12's popup contracts and the typed projection/GROUP BY identities. Require ordinary ungrouped SELECTs to offer only context-valid table-column or selected-expression choices defined by the PRD, while aggregate/grouped queries offer exactly grouped columns and selected aggregate entries, including bare `COUNT(*)`, with stable identities that distinguish equal labels and duplicate-looking expressions. Exclude wildcard, ungrouped nonaggregate columns, aggregates not present in the projection, stale columns, duplicates, and arbitrary expressions. Require at most one committed ORDER BY expression; replacement must be atomic, ASC must be the initial/default direction, and Up/Down in the focused base ORDER BY field must toggle deterministically between ASC and DESC without opening or moving popup selection. Assert exact safely rendered SQL, candidate order, popup accept/cancel/focus restoration, and clearing/reselection behavior. Keep this task test-only and make QueryBuilder candidate/validity assertions authoritative rather than encoding grouping rules in UI fixtures.

---

### 4. Implement ORDER BY candidates and direction

**Type**: GREEN
**Output**: ORDER BY state, candidate, direction, and SQL tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement typed ORDER BY expression identity, context-derived candidates, immutable single-selection state, and closed ASC/DESC direction in `internal/querybuilder/order_by.go`, integrating SQL and validity through `internal/querybuilder/builder.go`. Derive aggregate/grouped candidates from committed projection and GROUP BY state so grouped columns and selected aggregate entries are the only available expressions in those contexts; never accept wildcard, an unselected aggregate, arbitrary SQL text, or stale Schema identity. Default each newly selected expression to ASC, preserve or reset direction according to the tested replacement contract, and emit one safely quoted/fixed-token ORDER BY clause. Wire the reusable popup and base-field behavior through `internal/ui/order_by_popup.go` and `internal/ui/model.go`, letting QueryBuilder own candidate eligibility while UI owns exact focus, acceptance, cancellation, and Up/Down direction toggling in base context. Preserve key precedence and popup modality, and make Task 3 pass without implementing execution.

---

### 5. Specify bounded LIMIT behavior

**Type**: RED
**Output**: Failing tests cover empty, one, max int64, zero, negative, malformed, overflow, and the exact invalid reason.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent table-driven LIMIT tests in `internal/querybuilder/limit_test.go` and focused universal-input/model assertions in `internal/ui/limit_test.go`. Treat the entered representation as distinct state and require empty input to mean an unbounded logical result with no LIMIT clause, while exact decimal integers from `1` through `9223372036854775807` are accepted and rendered canonically as a LIMIT. Cover `1`, the signed-int64 maximum, zero in multiple representations, negatives, leading plus, whitespace, decimal/exponent/hex forms, nonnumeric text, signed-int64 overflow, extremely long input, and revision from valid to invalid or empty. Every nonempty invalid input must report exactly `Limit must be an integer from 1 to 9223372036854775807`, identify Limit as the invalid field, preserve the user's entered text for correction/history comparison, and produce no runnable SQL. Assert submission, Esc restoration, and later whole-value clearing seams without implementing Issue #19's general clearing behavior. Keep this task test-only and do not reuse universal value REAL/TEXT coercion as LIMIT validity.

---

### 6. Implement LIMIT parsing and validation

**Type**: GREEN
**Output**: LIMIT state and SQL tests pass with exact validity and reason contracts.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the bounded LIMIT state, parser, validity report, and SQL rendering in `internal/querybuilder/limit.go`, integrating it with SELECT state in `internal/querybuilder/builder.go`. Preserve exact entered text separately from the optional accepted integer, interpret only empty input as unbounded, accept only base-10 integer text representing `1` through signed-int64 maximum, and classify zero, negatives, malformed forms, whitespace, and overflow with the exact required reason. Emit no clause for empty state and a canonical numeric LIMIT clause for accepted state, without binding or interpolating invalid input. Wire the field to the existing universal text-entry and focus/rendering seams in `internal/ui/value_input.go` and `internal/ui/model.go`, preserving prior value on cancel and showing QueryBuilder's reason verbatim rather than maintaining a second UI validator. Make Task 5 pass, expose the whole-field transition needed by Issue #19, and do not start execution or focus invalid Enter yet.

---

### 7. Document grouping, ordering, and Limit

**Type**: DOCUMENT
**Output**: Wiki documentation records the grouping matrix, ORDER BY candidates, direction, bounds, and exact invalid feedback.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #18 implementation and tests from `internal/querybuilder`, `internal/ui`, and their `internal/schema` and projection dependencies into the appropriate pages under `Notes/wiki`. Document GROUP BY assisted multi-selection, stable order and duplicate prevention, every valid/invalid matrix combination for nonaggregates, mixed projections, all-aggregate projection, bare `COUNT(*)`, and wildcard. Record context-valid ORDER BY expression identity and exclusions, grouped-column and selected-aggregate candidates, one-expression ownership, ASC default, DESC toggling, and exact SQL behavior. Explain LIMIT's empty/unbounded state, accepted range `1` through `9223372036854775807`, all invalid categories, preservation of entered representation, and the exact reason `Limit must be an integer from 1 to 9223372036854775807`. Cross-reference Issues #15-18 and the Query Grammar, Runnable-State Contract, Builder and Display Interaction, QueryBuilder, UI, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the SELECT-rules walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/018-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/018-08/code-walkthrough`. Demonstrate GROUP BY multi-selection order and duplicate no-op behavior across grouped nonaggregate-only, valid and invalid mixed, all-aggregate without GROUP BY, bare `COUNT(*)`, and wildcard rejection cases, with exact validity and SQL evidence. Show ORDER BY candidates changing with context so only grouped columns and selected aggregate expressions remain for aggregate/grouped queries, then select one expression, verify ASC default, toggle to DESC with Up/Down in the base field, replace and clear it, and inspect SQL. Exercise empty Limit, `1`, `9223372036854775807`, zero, negative, malformed, whitespace, and overflow input, including exact invalid feedback and preserved text. Reference Issue #18 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
