// Issue #66 Task 3 (RED): exact regression tests locking valid SELECT, paging,
// count, Limit/OFFSET, quoting, rowid fallback, and parameter order after the
// runnable gate. Every positive case uses accepted RunnableReport state as a
// prerequisite, requires unchanged exact SQL and fresh parameter slices across
// SelectSQL/PageSQL/CountSQL and SelectParams/PageParams/CountParams, and
// covers wildcard, COUNT(*), named Value and aggregate projections, quoted
// identifiers, completed WHERE values, grouped/aggregate ordering, user Limit,
// page-size clamping, nonzero OFFSET, count wrapping, and the eligible
// page-only ORDER BY rowid fallback. Non-SELECT UPDATE/DELETE/INSERT builders
// whose fields could otherwise render as SELECT fragments are required to keep
// the full SELECT family empty. Parameter order and typed values must agree
// across SELECT/page/count while paging literals add no bindings.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// assertRunnablePrerequisite confirms a positive case rests on accepted
// RunnableReport state before asserting any rendered output, so a regression
// that silently makes the case non-runnable surfaces here rather than as an
// empty-string mismatch.
func assertRunnablePrerequisite(t *testing.T, name string, q QueryBuilder) {
	t.Helper()
	if report := q.RunnableReport(); !report.Runnable {
		t.Fatalf("%s: RunnableReport = %+v, want runnable prerequisite", name, report)
	}
}

// assertSelectFamilyExact locks the SQL and parameter contract across the
// whole SELECT renderer family for one runnable snapshot: SelectSQL, PageSQL,
// CountSQL, and their parameter accessors must return the exact expected
// values, with paging literals adding no bindings and count wrapping the
// unchanged base SELECT.
func assertSelectFamilyExact(t *testing.T, name string, q QueryBuilder, wantSelect, wantPage string, wantParams []any) {
	t.Helper()
	assertRunnablePrerequisite(t, name, q)
	if got := q.SelectSQL(); got != wantSelect {
		t.Errorf("%s: SelectSQL() = %q, want %q", name, got, wantSelect)
	}
	if got := q.PageSQL(5, 0); got != wantPage {
		t.Errorf("%s: PageSQL(5, 0) = %q, want %q", name, got, wantPage)
	}
	if got := q.CountSQL(); got != "SELECT COUNT(*) FROM ("+wantSelect+")" {
		t.Errorf("%s: CountSQL() = %q, want SELECT COUNT(*) FROM (%s)", name, got, wantSelect)
	}
	if got := q.SelectParams(); !reflect.DeepEqual(got, wantParams) {
		t.Errorf("%s: SelectParams() = %v, want %v", name, got, wantParams)
	}
	if got := q.PageParams(); !reflect.DeepEqual(got, wantParams) {
		t.Errorf("%s: PageParams() = %v, want %v (must match SelectParams)", name, got, wantParams)
	}
	if got := q.CountParams(); !reflect.DeepEqual(got, wantParams) {
		t.Errorf("%s: CountParams() = %v, want %v (must match SelectParams)", name, got, wantParams)
	}
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

// TestSelectRendererFamilyPagingClampingAndOffset locks the page-range
// semantics after gating: page limit clamped to the remaining user Limit,
// nonzero OFFSET, offset beyond the user Limit yielding empty, and paging
// literals adding no bindings while parameter order matches SelectParams.
func TestSelectRendererFamilyPagingClampingAndOffset(t *testing.T) {
	q := selectWildcard(buildSelect()).SetLimitInput("8")
	assertRunnablePrerequisite(t, "user limit 8", q)
	cases := []struct {
		name       string
		pageLimit  int64
		offset     int64
		want       string
		wantParams []any
	}{
		{
			name:      "page limit clamped to remaining user limit",
			pageLimit: 5,
			offset:    5,
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 3 OFFSET 5`,
		},
		{
			name:      "nonzero offset within user limit",
			pageLimit: 4,
			offset:    2,
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 4 OFFSET 2`,
		},
		{
			name:      "offset beyond user limit yields empty",
			pageLimit: 5,
			offset:    8,
			want:      "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := q.PageSQL(tc.pageLimit, tc.offset); got != tc.want {
				t.Errorf("PageSQL(%d, %d) = %q, want %q", tc.pageLimit, tc.offset, got, tc.want)
			}
			// Paging literals add no bindings: PageParams matches SelectParams.
			if got := q.PageParams(); len(got) != 0 {
				t.Errorf("PageParams() = %v, want no parameters (paging literals bind nothing)", got)
			}
		})
	}
}

// TestSelectRendererFamilyCountWrapsUnchangedBase locks the count-wrapping
// contract: CountSQL is exactly SELECT COUNT(*) FROM (<SelectSQL()>) including
// any user Limit inside the subquery, and CountParams matches SelectParams in
// order and typed values.
func TestSelectRendererFamilyCountWrapsUnchangedBase(t *testing.T) {
	cases := []struct {
		name       string
		build      func() QueryBuilder
		wantSelect string
		wantParams []any
	}{
		{
			name:       "wildcard no limit",
			build:      func() QueryBuilder { return selectWildcard(buildSelect()) },
			wantSelect: `SELECT * FROM "items"`,
		},
		{
			name: "wildcard with user limit inside subquery",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("3")
			},
			wantSelect: `SELECT * FROM "items" LIMIT 3`,
		},
		{
			name: "bound where value with limit keeps parameter order",
			build: func() QueryBuilder {
				return whereCompleteEq(selectWildcard(buildSelect()).SetLimitInput("5"), "42")
			},
			wantSelect: `SELECT * FROM "items" WHERE "name" = ? LIMIT 5`,
			wantParams: []any{int64(42)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.build()
			assertRunnablePrerequisite(t, tc.name, q)
			base := q.SelectSQL()
			if base != tc.wantSelect {
				t.Errorf("SelectSQL() = %q, want %q", base, tc.wantSelect)
			}
			wantCount := "SELECT COUNT(*) FROM (" + tc.wantSelect + ")"
			if got := q.CountSQL(); got != wantCount {
				t.Errorf("CountSQL() = %q, want %q", got, wantCount)
			}
			selParams := q.SelectParams()
			countParams := q.CountParams()
			if !reflect.DeepEqual(selParams, countParams) {
				t.Errorf("CountParams() = %v differs from SelectParams() = %v", countParams, selParams)
			}
			if !reflect.DeepEqual(selParams, tc.wantParams) {
				t.Errorf("SelectParams() = %v, want %v", selParams, tc.wantParams)
			}
		})
	}
}

// TestSelectRendererFamilyRowidFallbackEligiblePageOnly locks the eligible
// page-only ORDER BY rowid fallback: PageSQL appends it over an ordinary rowid
// table with no user ORDER BY and no aggregate/group shape, while SelectSQL
// and CountSQL never carry the fallback.
func TestSelectRendererFamilyRowidFallbackEligiblePageOnly(t *testing.T) {
	q := selectWildcard(buildSelect())
	assertRunnablePrerequisite(t, "eligible rowid fallback", q)
	if got := q.SelectSQL(); got != `SELECT * FROM "items"` {
		t.Errorf("SelectSQL() = %q, want no rowid fallback on base SELECT", got)
	}
	if got := q.CountSQL(); got != `SELECT COUNT(*) FROM (SELECT * FROM "items")` {
		t.Errorf("CountSQL() = %q, want no rowid fallback on count", got)
	}
	if got := q.PageSQL(5, 0); got != `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0` {
		t.Errorf("PageSQL(5, 0) = %q, want rowid fallback on eligible page only", got)
	}
}

// TestSelectRendererFamilyNonSelectBuildersStayEmpty locks that non-SELECT
// UPDATE/DELETE/INSERT builders whose fields could otherwise render as SELECT
// fragments keep the full SELECT family empty even when the write command is
// runnable.
func TestSelectRendererFamilyNonSelectBuildersStayEmpty(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
	}{
		{
			name:  "runnable UPDATE",
			build: func() QueryBuilder { return whereCompleteEq(setSubmittedValue(buildUpdate(), "name", "x"), "y") },
		},
		{
			name:  "runnable DELETE",
			build: func() QueryBuilder { return whereCompleteEq(buildDelete(), "x") },
		},
		{
			name:  "runnable INSERT all-omit",
			build: func() QueryBuilder { return insertChoiceAllOmit(buildInsert()) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.build()
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("%s: write command RunnableReport = %+v, want runnable", tc.name, report)
			}
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
				t.Errorf("%s: SelectParams() = %v, want empty", tc.name, got)
			}
			if got := q.PageParams(); len(got) != 0 {
				t.Errorf("%s: PageParams() = %v, want empty", tc.name, got)
			}
			if got := q.CountParams(); len(got) != 0 {
				t.Errorf("%s: CountParams() = %v, want empty", tc.name, got)
			}
		})
	}
}
