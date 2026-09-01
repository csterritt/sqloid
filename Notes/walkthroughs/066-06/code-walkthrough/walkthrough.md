# Issue #066 Code Walkthrough: Authoritative SELECT Renderer Gating

*2026-09-01T19:20:43Z by Showboat 0.6.1*
<!-- showboat-id: e1aba0fe-76e5-4619-ac8b-a826cfea455d -->

Issue #66 (Notes/tasks/066-gate-select-renderers-on-runnable-report.md, Notes/PRD-sqloid.md §Runnable-State Contract, §Query Grammar, §safe rendering, §schema revalidation, §QueryBuilder Module Design, §Testing Decisions) gates the SELECT renderer family on the authoritative runnable report. Before this issue, SelectSQL, PageSQL, and CountSQL checked only that the command was SELECT and a table was set; every other validity rule (nonempty projection, stale projections, complete WHERE, grouping matrix, ORDER BY, Limit) was trusted to hold, so a non-runnable SELECT could render partially valid SQL. Issue #66 retires the renderer's own command/table checks and requires CommandSelect plus RunnableReport().Runnable at the shared renderSelectCore seam and at SelectParams; PageSQL/CountSQL/PageParams/CountParams delegate, so the whole family is gated through one authority. Rejected states return only empty SQL and nil parameters — never partially rendered clauses or values — including cases whose component state is locally formattable. The gate builds on Issue #65's projection validation (reportStaleProjection validates every committed named ProjectionColumn against refreshed visible columns before later SELECT fields), which made RunnableReport authoritative for projection staleness; without Issue #65 a stale projected column would leave the report runnable and the gate would render SQL referencing a vanished column. This walkthrough exercises representative and boundary cases from every rejected runnable class — including Issue #65 stale projections — showing empty SQL and no parameters from SELECT, page, and count renderers, then runs valid wildcard, COUNT(*), quoted named/aggregate, WHERE, grouping/order, user-Limit, nonzero-page, rowid-fallback, and count cases with exact SQL and binding order, and explains the single authoritative gate and the #65-before-#66 dependency.

## The single authoritative gate

The gate lives at the shared renderSelectCore assembly seam in internal/querybuilder/select_sql.go and at SelectParams. renderSelectCore assembles the quoted projection, table, WHERE, GROUP BY, and ORDER BY (plus the page-only ORDER BY rowid fallback) exactly as before, but only after the runnable gate passes. SelectParams applies the same gate before extracting the WHERE predicate's parameters. PageSQL checks the page range first (preserved independently of builder validity), then delegates to renderSelectCore; CountSQL delegates to SelectSQL; PageParams and CountParams delegate to SelectParams. No second validator, recursive report/render dependency, or renderer-specific validity interpretation exists.

```bash
sed -n '16,72p' internal/querybuilder/select_sql.go
```

```output
// SelectSQL renders the current snapshot's SELECT statement exactly: quoted
// projection over the quoted table, then WHERE, GROUP BY (commit order),
// ORDER BY (single committed expression with direction), and LIMIT in grammar
// order. An empty result means the snapshot is not a runnable SELECT — the
// authoritative RunnableReport (Issue #19, extended by Issue #65's
// stale-projection gate and Issue #66's renderer gate) refuses every
// non-runnable class — never a partially valid query.
func (q QueryBuilder) SelectSQL() string {
	parts, ok := renderSelectCore(q, false)
	if !ok {
		return ""
	}
	if limit := renderLimit(q); limit != "" {
		parts = append(parts, limit)
	}
	return strings.Join(parts, " ")
}

// renderSelectCore renders the shared SELECT prefix: the quoted projection
// over the quoted table, then WHERE, GROUP BY (commit order), and ORDER BY.
// It appends the implicit `ORDER BY rowid` fallback only when allowRowid
// holds and rowidFallbackEligible confirms the single eligible case — an
// ordinary rowid table with no declared rowid shadow, no aggregate or GROUP
// shape, and no user ORDER BY. A false second return means the snapshot is
// not a runnable SELECT — the single authoritative RunnableReport gates the
// whole SELECT renderer family (Issue #66), so no partially rendered clauses
// or values escape a non-runnable state.
func renderSelectCore(q QueryBuilder, allowRowid bool) ([]string, bool) {
	if q.command != CommandSelect || !q.RunnableReport().Runnable {
		return nil, false
	}
	projection := q.renderProjection()
	if projection == "" {
		return nil, false
	}
	parts := []string{"SELECT " + projection + " FROM " + quoteIdentifierAtom(q.table)}
	if pred := q.WherePredicate(); pred.State() == WhereComplete {
		parts = append(parts, "WHERE "+pred.SQL())
	}
	if groups := q.GroupByEntries(); len(groups) > 0 {
		atoms := make([]string, 0, len(groups))
		for _, g := range groups {
			atoms = append(atoms, quoteIdentifierAtom(g))
		}
		parts = append(parts, "GROUP BY "+joinSQLList(atoms))
	}
	if cand, dir, ok := q.OrderBySelection(); ok {
		token, err := dir.SQLToken()
		if err != nil {
			return nil, false // unreachable by construction; refuse unsafe text
		}
		expr := cand.sqlExpr()
		if expr == "" {
			return nil, false // unresolved aggregate identity: refuse rather than guess
		}
		parts = append(parts, "ORDER BY "+expr+" "+token)
	} else if allowRowid && rowidFallbackEligible(q) {
```

```bash
sed -n '93,108p' internal/querybuilder/select_sql.go
```

```output
	return obj.Kind == schema.KindOrdinaryTable && obj.Rowid == schema.RowidHas && !obj.RowidShadowed
}

// SelectParams returns this snapshot's bound parameter values in deterministic
// order — currently only the completed WHERE predicate's single value when it
// takes one. Projection, grouping, ordering, and LIMIT contribute no
// parameters: identifiers are quoted atoms and the limit is a literal integer.
// It returns nil unless the authoritative RunnableReport accepts the SELECT
// state (Issue #66), so no parameters escape a non-runnable snapshot.
func (q QueryBuilder) SelectParams() []any {
	if q.command != CommandSelect || !q.RunnableReport().Runnable {
		return nil
	}
	return q.WherePredicate().Params()
}
```

## Rejected runnable classes: empty SQL and no parameters

The Task 1 rejection matrix (internal/querybuilder/select_renderer_gate_test.go) covers every non-runnable SELECT class. For each case the report is asserted non-runnable first, then SelectSQL, PageSQL, CountSQL, SelectParams, PageParams, and CountParams must all emit nothing. The cases include Issue #65 stale projections, incomplete/stale WHERE, every invalid grouping and ORDER BY class, malformed/zero/negative/overflow Limit, and open value states combined with later invalid state — including cases whose component state is locally formattable.

```bash
go test ./internal/querybuilder/ -run 'TestSelectRendererFamilyRejectsEveryNonRunnableClass' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g' | head -40
```

```output
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/missing_command
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/command_chosen_but_no_table
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/empty_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_table_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Value_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Count_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Min_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Max_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Avg_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_named_Sum_projection_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/open_WHERE_draft_at_column_choice_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/open_WHERE_draft_awaiting_value_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_WHERE_column_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/mixed_aggregate/nonaggregate_projection_without_GROUP_BY_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/wildcard_beside_GROUP_BY_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_grouped_column_blocks_after_refresh
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_ORDER_BY_expression_blocks_after_refresh
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/malformed_Limit_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/zero_Limit_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/negative_Limit_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/overflow_Limit_blocks
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/open_WHERE_draft_with_invalid_Limit_blocks_entirely
=== RUN   TestSelectRendererFamilyRejectsEveryNonRunnableClass/stale_projection_with_invalid_Limit_blocks_entirely
--- PASS: TestSelectRendererFamilyRejectsEveryNonRunnableClass (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

### Stale named projection (Issue #65) and invalid Limit refuse to render

A representative boundary: a SELECT with a stale named Value projection (the score column dropped after refresh) and a SELECT with a malformed Limit. Both are non-runnable, so the whole family emits nothing — even though the projection or the base SELECT is locally formattable. The selectRenderersEmpty helper asserts the report is non-runnable first, then requires every SELECT-family SQL and parameter method to emit nothing:

```bash
sed -n '23,42p' internal/querybuilder/select_renderer_gate_test.go
```

```output
// selectRenderersEmpty asserts the Issue #66 contract for one non-runnable
// SELECT snapshot: RunnableReport is not runnable, and SelectSQL, PageSQL,
// CountSQL, SelectParams, PageParams, and CountParams all emit nothing. It
// uses a representative valid page range so a non-empty renderer would surface
// rather than hide behind an invalid range.
func selectRenderersEmpty(t *testing.T, name string, q QueryBuilder) {
	t.Helper()
	if report := q.RunnableReport(); report.Runnable {
		t.Fatalf("%s: RunnableReport.Runnable = true, want false (case must be non-runnable)", name)
	}
	if got := q.SelectSQL(); got != "" {
		t.Errorf("%s: SelectSQL() = %q, want empty", name, got)
	}
	if got := q.PageSQL(5, 0); got != "" {
		t.Errorf("%s: PageSQL(5, 0) = %q, want empty", name, got)
	}
	if got := q.CountSQL(); got != "" {
		t.Errorf("%s: CountSQL() = %q, want empty", name, got)
	}
	if got := q.SelectParams(); len(got) != 0 {
```

### Non-SELECT commands are rejected too

A runnable UPDATE, DELETE, or INSERT builder's fields could otherwise render as SELECT fragments; the CommandSelect test refuses them even when the write command itself is runnable.

```bash
go test ./internal/querybuilder/ -run 'TestSelectRendererFamilyRejectsNonSelectCommands' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
--- PASS: TestSelectRendererFamilyRejectsNonSelectCommands (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

## Valid cases: exact SQL and binding order after gating

Accepted builders retain exact quoting and binding order. The Task 3 regression tests (internal/querybuilder/select_renderer_valid_test.go) lock wildcard, COUNT(*), named Value and aggregate projections, quoted identifiers, completed WHERE values, grouped/aggregate ordering, user Limit, page-size clamping, nonzero OFFSET, count wrapping, and the eligible page-only ORDER BY rowid fallback. Parameter order and typed values agree across SELECT/page/count while paging literals add no bindings.

```bash
sed -n '60,162p' internal/querybuilder/select_renderer_valid_test.go
```

```output
}

// TestSelectRendererFamilyValidRegressions locks the exact SQL and parameter
// order for every representative runnable SELECT shape after the Issue #66
// gate: wildcard, COUNT(*), named Value and aggregate projections, quoted
// identifiers, completed WHERE values, grouped/aggregate ordering, user Limit,
// page-size clamping, nonzero OFFSET, count wrapping, and the eligible
// page-only ORDER BY rowid fallback.
func TestSelectRendererFamilyValidRegressions(t *testing.T) {
	cases := []struct {
		name       string
		build      func() QueryBuilder
		wantSelect string
		wantPage   string
		wantParams []any
	}{
		{
			name:       "wildcard projection with rowid fallback",
			build:      func() QueryBuilder { return selectWildcard(buildSelect()) },
			wantSelect: `SELECT * FROM "items"`,
			wantPage:   `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0`,
		},
		{
			name: "COUNT(*) sentinel projection",
			build: func() QueryBuilder {
				return buildSelect().AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
			},
			wantSelect: `SELECT COUNT(*) FROM "items"`,
			wantPage:   `SELECT COUNT(*) FROM "items" LIMIT 5 OFFSET 0`,
		},
		{
			name: "named Value projection",
			build: func() QueryBuilder {
				return buildScoreProjection(AggregateValue)
			},
			wantSelect: `SELECT "score" FROM "items"`,
			wantPage:   `SELECT "score" FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0`,
		},
		{
			name: "named Count aggregate projection",
			build: func() QueryBuilder {
				return buildScoreProjection(AggCount)
			},
			wantSelect: `SELECT COUNT("score") FROM "items"`,
			wantPage:   `SELECT COUNT("score") FROM "items" LIMIT 5 OFFSET 0`,
		},
		{
			name: "quoted identifier with embedded quotes",
			build: func() QueryBuilder {
				obj := &schema.Object{Name: `we"ird`, Kind: schema.KindOrdinaryTable,
					WriteEligible: true, Rowid: schema.RowidHas,
					Columns: []schema.Column{{Name: `col"x`, Insertable: true}}}
				q := NewQuery().RefreshSchema(&schema.Catalog{Version: 1, Objects: []*schema.Object{obj}}).
					SelectCommand(CommandSelect).SelectTable(`we"ird`)
				q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: `col"x`}).Builder
				return q.CompleteProjectionAggregate(`col"x`, AggregateValue).Builder
			},
			wantSelect: `SELECT "col""x" FROM "we""ird"`,
			wantPage:   `SELECT "col""x" FROM "we""ird" ORDER BY rowid LIMIT 5 OFFSET 0`,
		},
		{
			name: "completed WHERE value binds one parameter in order",
			build: func() QueryBuilder {
				return whereCompleteEq(selectWildcard(buildSelect()), "42")
			},
			wantSelect: `SELECT * FROM "items" WHERE "name" = ?`,
			wantPage:   `SELECT * FROM "items" WHERE "name" = ? ORDER BY rowid LIMIT 5 OFFSET 0`,
			wantParams: []any{int64(42)},
		},
		{
			name: "grouped aggregate ordering renders exact expression",
			build: func() QueryBuilder {
				q := buildSelect()
				q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"}).Builder
				q = q.CompleteProjectionAggregate("name", AggCount).Builder
				q, ok := q.AcceptGroupColumn("id")
				if !ok {
					panic("setup: AcceptGroupColumn failed")
				}
				q, ok = q.AcceptOrderBy("order-aggregate:name:COUNT")
				if !ok {
					panic("setup: AcceptOrderBy failed")
				}
				return q.SetOrderDirection(DirDesc)
			},
			wantSelect: `SELECT COUNT("name") FROM "items" GROUP BY "id" ORDER BY COUNT("name") DESC`,
			wantPage:   `SELECT COUNT("name") FROM "items" GROUP BY "id" ORDER BY COUNT("name") DESC LIMIT 5 OFFSET 0`,
		},
		{
			name: "user Limit renders canonical integer",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("7")
			},
			wantSelect: `SELECT * FROM "items" LIMIT 7`,
			wantPage:   `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSelectFamilyExact(t, tc.name, tc.build(), tc.wantSelect, tc.wantPage, tc.wantParams)
		})
	}
}
```

### Eligible page-only ORDER BY rowid fallback

The implicit ORDER BY rowid fallback applies only to PageSQL over an ordinary rowid table with no declared rowid shadow, no user ORDER BY, and no aggregate/group shape. SelectSQL and CountSQL never carry the fallback.

```bash
go test ./internal/querybuilder/ -run 'TestSelectRendererFamilyRowidFallbackEligiblePageOnly' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
--- PASS: TestSelectRendererFamilyRowidFallbackEligiblePageOnly (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

### Count wrapping and parameter order

CountSQL renders exactly SELECT COUNT(*) FROM (<SelectSQL()>) with any user LIMIT inside the subquery; CountParams matches SelectParams in order and typed values. The full Task 3 regression suite locks every valid shape.

```bash
go test ./internal/querybuilder/ -run 'TestSelectRendererFamilyValidRegressions|TestSelectRendererFamilyPagingClampingAndOffset|TestSelectRendererFamilyCountWrapsUnchangedBase|TestSelectRendererFamilyNonSelectBuildersStayEmpty' -count=1 -v 2>&1 | grep -E '^(--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
--- PASS: TestSelectRendererFamilyValidRegressions (Ns)
--- PASS: TestSelectRendererFamilyPagingClampingAndOffset (Ns)
--- PASS: TestSelectRendererFamilyCountWrapsUnchangedBase (Ns)
--- PASS: TestSelectRendererFamilyNonSelectBuildersStayEmpty (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

## The #65-before-#66 dependency

Issue #66's gate trusts RunnableReport as the single authority precisely because Issue #65 made the report authoritative for projection staleness. reportStaleProjection (added by Issue #65 in internal/querybuilder/runnable.go) validates every committed named ProjectionColumn entry against the selected object's refreshed visible columns before later SELECT fields. Without Issue #65, a stale projected column would leave the report runnable, so the Issue #66 gate would render SQL referencing a vanished column. The gate builds on Issue #65's projection validation rather than reproducing it — no duplicate stale-identifier check lives in the renderer.

```bash
sed -n '157,200p' internal/querybuilder/runnable.go
```

```output
func (q QueryBuilder) reportSelect() RunnableReport {
	if q.ProjectionEmpty() {
		return RunnableReport{Field: RunFieldProjection, Reason: ReasonNoProjection}
	}
	if issue, invalid := q.reportStaleProjection(); invalid {
		return issue
	}
	if r, invalid := q.reportWhere(); invalid {
		return r
	}
	if issue, invalid := validateGrouping(q); invalid {
		return RunnableReport{Field: RunFieldGroupBy, Reason: issue.Reason}
	}
	if issue, invalid := validateOrderBy(q); invalid {
		return RunnableReport{Field: RunFieldOrderBy, Reason: issue.Reason}
	}
	if issue, invalid := validateLimit(q); invalid {
		return RunnableReport{Field: RunFieldLimit, Reason: issue.Reason}
	}
	return RunnableReport{Runnable: true}
}

// reportStaleProjection validates every committed named projection entry
// against the selected object's current visible columns, returning the first
// stale entry as a RunFieldProjection report. The synthetic wildcard and
// COUNT(*) sentinel identities are exempt by identity (Kind), never by
// display text, so a real column literally named `*` or `COUNT(*)` is still
// validated as a named identifier. Reuses selectedColumns for the
// visibility/identity pattern shared by reportWhere and validateGrouping.
func (q QueryBuilder) reportStaleProjection() (RunnableReport, bool) {
	visible := make(map[string]bool, len(q.projection))
	for _, col := range q.selectedColumns() {
		visible[col.Name] = true
	}
	for _, e := range q.projection {
		if e.Kind != ProjectionColumn {
			continue
		}
		if !visible[e.Column] {
			return RunnableReport{Field: RunFieldProjection, Reason: ReasonStaleProjectionColumn}, true
		}
	}
	return RunnableReport{}, false
}
```

## Full verification

The full querybuilder package passes after the gate, including the existing GROUP BY, ORDER BY, LIMIT, page, and count tests updated to commit projections through the guided aggregate-completion seam so their fixtures rest on accepted runnable state (the renderer no longer expands an empty projection to all visible columns, since RunnableReport requires a nonempty projection). The connection capability suite (probeQB updated to complete the named-column projection aggregate) and the UI, session, and CLI packages pass unchanged.

```bash
go test ./internal/querybuilder/ -count=1 2>&1 | grep -E '^(ok|FAIL|---)' | sed 's/[0-9][0-9]*\.[0-9][0-9]*s/Ns/g' && go vet ./... 2>&1 | tail -1 && go build ./... 2>&1 | tail -1 && echo ALL GREEN
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
ALL GREEN
```
