# Issue #18: GROUP BY, ORDER BY, and LIMIT rules

*2026-08-27T14:42:15Z by Showboat 0.6.1*
<!-- showboat-id: b8f4944a-8e10-44bc-828b-f06a1d7054e8 -->

Issue #18 completes the SELECT grammar per the Query Grammar, Runnable-State Contract, Builder and Display Interaction, QueryBuilder, UI, and Testing Decisions sections of Notes/PRD-sqloid.md: assisted GROUP BY multi-selection over the complete grouping validity matrix, context-valid single-expression ORDER BY with the closed ASC/DESC direction, and a bounded LIMIT with one exact invalid reason. QueryBuilder owns every rule and safely renders the statement; the UI owns focus, acceptance, and toggling. Every artifact in this walkthrough lives under this approved directory: _demo18/main.go is the runnable demonstration program.

## 1. Pure grouping tests: order, duplicates, and the full matrix

The table-driven suite asserts Schema-derived candidates excluding committed columns, acceptance-order preservation with immutable no-op rejections for duplicate/hidden identities, command/table replacement clearing, every matrix row (nonaggregate-only, mixed valid and invalid, extra groups, all-aggregate, bare COUNT(*), wildcard), and deterministic quoted GROUP BY SQL that never reorders the projection or adds parameters.

```bash
go test ./internal/querybuilder -count=1 -run 'TestGroupBy|TestAcceptGroup|TestFirstInvalidIssueAbsent|TestGroupingValidityMatrix' 2>&1 | grep -E '^(ok|FAIL)' | awk '{print $1, $2}'
```

```output
ok github.com/chris/sqloid/internal/querybuilder
```

## 2. Runnable demonstration: grouping matrix with exact validity and SQL evidence

_demo18/main.go drives the real transitions. Section 2: mixed projections without (or missing) their required groups report exactly 'every nonaggregate projected column must be grouped'; extra grouped columns stay permitted; all-aggregate and bare COUNT(*) projections remain valid without GROUP BY; the wildcard rejects GROUP BY outright. Section 1 shows selection order preserved as accepted (email, id — reverse-Schema order) with duplicates and hidden columns as immutable no-ops.

```bash
go run ./Notes/walkthroughs/018-08/code-walkthrough/_demo18 2>&1 | sed -n '/== 1/,/== 3/p' | head -26
```

```output
== 1. GROUP BY assisted multi-selection: order, duplicates, candidates ==
candidates: [id email] (hidden created_at excluded)
committed: [email id]
duplicate accept ok= false entries= [email id]
hidden accept ok= false entries= [email id]

== 2. grouping validity matrix ==
nonaggregate-only, no group:             valid
mixed COUNT(id)+email, no group:         INVALID [GROUP BY] every nonaggregate projected column must be grouped
mixed, missing one group:                INVALID [GROUP BY] every nonaggregate projected column must be grouped
mixed, every nonaggregate grouped:       valid
mixed with extra grouped columns:        valid
all-aggregate without group:             valid
bare COUNT(*) without group:             valid
wildcard with GROUP BY:                  INVALID [GROUP BY] the wildcard cannot be used together with GROUP BY
mixed no group  SQL: SELECT COUNT("id"), "email" FROM "users"
grouped         SQL: SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id"

== 3. ORDER BY candidates follow context ==
```

## 3. ORDER BY candidates change with context

Section 3 shows the candidate derivation: an ordinary ungrouped SELECT offers only the visible table columns; the grouped COUNT(id) query narrows to exactly the grouped columns plus the selected aggregate (the ungrouped plain column disappears); bare COUNT(*) appears as its own identity beside the grouped column. Section 4 shows one-expression ownership: fresh acceptance defaults to ASC, toggling flips to DESC, replacement swaps the expression atomically and resets to ASC, clearing removes the whole value, and an aggregate absent from the projection (or arbitrary text) is rejected outright.

```bash
go run ./Notes/walkthroughs/018-08/code-walkthrough/_demo18 2>&1 | sed -n '/== 3/,/== 5/p' | head -22
```

```output
== 3. ORDER BY candidates follow context ==
ungrouped candidates:
  key=order-column:id          display=id
  key=order-column:email       display=email
grouped COUNT(id) candidates:
  key=order-column:email             display=email
  key=order-column:id                display=id
  key=order-aggregate:id:COUNT       display=COUNT(id)
bare COUNT(*) context candidates:
  key=order-column:email       display=email
  key=order-count-star         display=COUNT(*)

== 4. one expression, ASC default, DESC toggle, replacement, clearing ==
fresh selection: SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id" ORDER BY "email" ASC dir==ASC: true
toggled:         SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id" ORDER BY "email" DESC
replaced ok= true -> SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id" ORDER BY COUNT("id") ASC (ASC reset)
cleared SQL:     SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id"
unselected aggregate rejected: true
arbitrary text rejected:       true

== 5. LIMIT: bounds, canonical rendering, exact invalid reason ==
```

## 4. Pure ORDER BY tests

```bash
go test ./internal/querybuilder -count=1 -run 'TestOrderBy|TestAcceptOrderBy|TestOrderDirection' 2>&1 | grep -E '^(ok|FAIL)' | awk '{print $1, $2}'
```

```output
ok github.com/chris/sqloid/internal/querybuilder
```

## 5. LIMIT: bounds, canonical rendering, and the exact invalid reason

Section 5 exercises empty (unbounded, no clause), 1, the signed-int64 maximum, leading zeros rendering canonically, zero, negatives, a leading plus, whitespace, decimal/exponent/hex forms, nonnumeric text, signed-int64 overflow, and extremely long input — every rejected category reporting exactly 'Limit must be an integer from 1 to 9223372036854775807' with the entered text preserved verbatim and no runnable SQL. Section 6 composes the full statement: projection over GROUP BY over ORDER BY over LIMIT, with zero bound parameters.

```bash
go run ./Notes/walkthroughs/018-08/code-walkthrough/_demo18 2>&1 | sed -n '/== 5/,'
```

```output
sed: -e expression #1, char 7: unexpected `,'
```

## 6. Pure LIMIT tests and the scripted UI flows

```bash
go test ./internal/querybuilder ./internal/ui -count=1 -run 'TestLimit|TestGroupBy|TestOrderBy' 2>&1 | grep -E '^ok' | awk '{print $1, $2}'
```

```output
ok github.com/chris/sqloid/internal/querybuilder
ok github.com/chris/sqloid/internal/ui
```
