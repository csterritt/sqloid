// Issue #66 Task 1 (RED): the SELECT renderer family — SelectSQL, PageSQL,
// CountSQL, and their parameter accessors — must emit empty output for every
// non-runnable SELECT class. The authoritative validity source is
// RunnableReport (Issue #19, extended by Issue #65's stale-projection gate).
// These table tests cover the full rejection matrix: missing command/table/
// projection, stale table, stale named Value and aggregate projections,
// incomplete or stale WHERE, every invalid grouping and ORDER BY class,
// malformed/zero/negative/overflow Limit, and any open value state. For each
// case the report is asserted non-runnable first, then every public
// SELECT-family SQL and parameter method must return empty/nil — including
// cases whose component state is locally formattable. The tests stay
// test-only and reuse the runnable_test.go builders as the authoritative
// validity source; no renderer validity rule is restated in test helpers.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

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
		t.Errorf("%s: SelectParams() = %v, want no parameters", name, got)
	}
	if got := q.PageParams(); len(got) != 0 {
		t.Errorf("%s: PageParams() = %v, want no parameters", name, got)
	}
	if got := q.CountParams(); len(got) != 0 {
		t.Errorf("%s: CountParams() = %v, want no parameters", name, got)
	}
}

// TestSelectRendererFamilyRejectsEveryNonRunnableClass walks the full
// non-runnable SELECT matrix from the authoritative RunnableReport, requiring
// SelectSQL, PageSQL, CountSQL, and their parameter accessors to emit nothing
// for each class — including cases whose component state is locally
// formattable.
func TestSelectRendererFamilyRejectsEveryNonRunnableClass(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
	}{
		// Missing command, table, and projection.
		{
			name:  "missing command",
			build: func() QueryBuilder { return NewQuery() },
		},
		{
			name: "command chosen but no table",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandSelect)
			},
		},
		{
			name:  "empty projection blocks",
			build: buildSelect,
		},
		// Stale table after a refresh drops the selected object entirely.
		{
			name: "stale table blocks",
			build: func() QueryBuilder {
				dropped := &schema.Catalog{
					Version: 20,
					Objects: []*schema.Object{
						{Name: "blobs_only", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
							Columns: []schema.Column{{Name: "data", DeclaredType: "BLOB", Hidden: true}}},
					},
				}
				return selectWildcard(buildSelect()).RefreshSchema(dropped)
			},
		},
		// Stale named Value and aggregate projections (Issue #65).
		{
			name: "stale named Value projection blocks",
			build: func() QueryBuilder {
				return buildScoreProjection(AggregateValue).RefreshSchema(itemsCatalogDropsScore())
			},
		},
		{
			name:  "stale named Count projection blocks",
			build: func() QueryBuilder { return buildScoreProjection(AggCount).RefreshSchema(itemsCatalogDropsScore()) },
		},
		{
			name:  "stale named Min projection blocks",
			build: func() QueryBuilder { return buildScoreProjection(AggMin).RefreshSchema(itemsCatalogDropsScore()) },
		},
		{
			name:  "stale named Max projection blocks",
			build: func() QueryBuilder { return buildScoreProjection(AggMax).RefreshSchema(itemsCatalogDropsScore()) },
		},
		{
			name:  "stale named Avg projection blocks",
			build: func() QueryBuilder { return buildScoreProjection(AggAvg).RefreshSchema(itemsCatalogDropsScore()) },
		},
		{
			name:  "stale named Sum projection blocks",
			build: func() QueryBuilder { return buildScoreProjection(AggSum).RefreshSchema(itemsCatalogDropsScore()) },
		},
		// Incomplete WHERE drafts at every open value state.
		{
			name: "open WHERE draft at column choice blocks",
			build: func() QueryBuilder {
				next, _ := selectWildcard(buildSelect()).StartWhere("name")
				return next
			},
		},
		{
			name: "open WHERE draft awaiting value blocks",
			build: func() QueryBuilder {
				next, _ := selectWildcard(buildSelect()).StartWhere("name")
				draft, _ := next.WhereDraft().ChooseOperator(OpEq)
				return next.ApplyWhereDraft(draft)
			},
		},
		// Stale WHERE column after a refresh drops the named column while the
		// projection survives.
		{
			name: "stale WHERE column blocks",
			build: func() QueryBuilder {
				return whereCompleteEqOn(t, selectWildcard(buildSelect()), "score", "x").RefreshSchema(itemsCatalogDropsScoreOnly())
			},
		},
		// Every invalid grouping class.
		{
			name: "mixed aggregate/nonaggregate projection without GROUP BY blocks",
			build: func() QueryBuilder {
				q := buildSelect()
				out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"})
				out = out.Builder.CompleteProjectionAggregate("name", AggregateValue)
				out = out.Builder.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "score"})
				out = out.Builder.CompleteProjectionAggregate("score", AggMin)
				return out.Builder
			},
		},
		{
			name: "wildcard beside GROUP BY blocks",
			build: func() QueryBuilder {
				next, ok := selectWildcard(buildSelect()).AcceptGroupColumn("name")
				if !ok {
					panic("setup: AcceptGroupColumn failed")
				}
				return next
			},
		},
		{
			name: "stale grouped column blocks after refresh",
			build: func() QueryBuilder {
				next, ok := buildAggSelect("name", AggCount).AcceptGroupColumn("score")
				if !ok {
					panic("setup: AcceptGroupColumn failed")
				}
				return next.RefreshSchema(itemsCatalogDropsScoreOnly())
			},
		},
		// Invalid ORDER BY class: a committed expression no longer offered.
		{
			name: "stale ORDER BY expression blocks after refresh",
			build: func() QueryBuilder {
				next, ok := buildPlainSelect().AcceptOrderBy("order-column:score")
				if !ok {
					panic("setup: AcceptOrderBy failed")
				}
				return next.RefreshSchema(itemsCatalogDropsScoreOnly())
			},
		},
		// Malformed, zero, negative, and overflow Limit.
		{
			name:  "malformed Limit blocks",
			build: func() QueryBuilder { return selectWildcard(buildSelect()).SetLimitInput("abc") },
		},
		{
			name:  "zero Limit blocks",
			build: func() QueryBuilder { return selectWildcard(buildSelect()).SetLimitInput("0") },
		},
		{
			name:  "negative Limit blocks",
			build: func() QueryBuilder { return selectWildcard(buildSelect()).SetLimitInput("-5") },
		},
		{
			name:  "overflow Limit blocks",
			build: func() QueryBuilder { return selectWildcard(buildSelect()).SetLimitInput("9223372036854775808") },
		},
		// Open value state combined with later invalid state: the renderer
		// must still emit nothing even though the projection is formattable.
		{
			name: "open WHERE draft with invalid Limit blocks entirely",
			build: func() QueryBuilder {
				next, _ := selectWildcard(buildSelect()).StartWhere("name")
				return next.SetLimitInput("abc")
			},
		},
		{
			name: "stale projection with invalid Limit blocks entirely",
			build: func() QueryBuilder {
				return buildScoreProjection(AggregateValue).SetLimitInput("0").RefreshSchema(itemsCatalogDropsScore())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selectRenderersEmpty(t, tc.name, tc.build())
		})
	}
}

// TestSelectRendererFamilyRejectsNonSelectCommands covers the
// non-SELECT-command contract: an UPDATE, DELETE, or INSERT builder whose
// fields could otherwise render as SELECT fragments stays empty across the
// whole SELECT renderer family, even when the write command itself is
// runnable.
func TestSelectRendererFamilyRejectsNonSelectCommands(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
	}{
		{
			name: "runnable UPDATE renders no SELECT family output",
			build: func() QueryBuilder {
				return whereCompleteEq(setSubmittedValue(buildUpdate(), "name", "x"), "y")
			},
		},
		{
			name:  "runnable DELETE renders no SELECT family output",
			build: func() QueryBuilder { return whereCompleteEq(buildDelete(), "x") },
		},
		{
			name:  "runnable INSERT renders no SELECT family output",
			build: func() QueryBuilder { return insertChoiceAllOmit(buildInsert()) },
		},
		{
			name:  "unselected command renders no SELECT family output",
			build: func() QueryBuilder { return NewQuery().RefreshSchema(runnableCatalog()) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.build()
			if got := q.SelectSQL(); got != "" {
				t.Errorf("%s: SelectSQL() = %q, want empty", tc.name, got)
			}
			if got := q.PageSQL(5, 0); got != "" {
				t.Errorf("%s: PageSQL(5, 0) = %q, want empty", tc.name, got)
			}
			if got := q.CountSQL(); got != "" {
				t.Errorf("%s: CountSQL() = %q, want empty", tc.name, got)
			}
			if got := q.SelectParams(); len(got) != 0 {
				t.Errorf("%s: SelectParams() = %v, want no parameters", tc.name, got)
			}
			if got := q.PageParams(); len(got) != 0 {
				t.Errorf("%s: PageParams() = %v, want no parameters", tc.name, got)
			}
			if got := q.CountParams(); len(got) != 0 {
				t.Errorf("%s: CountParams() = %v, want no parameters", tc.name, got)
			}
		})
	}
}
