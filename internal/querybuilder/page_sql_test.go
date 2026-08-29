// UI-independent table-driven tests for the page-request SQL and ordering
// policy (Issue #25 Task 1): adjacent page requests must preserve the base
// SELECT, ordered bound parameters, and the user's Limit semantics, and
// carry exact LIMIT/OFFSET ranges rendered only from integers — never
// interpolated user text. The implicit `ORDER BY rowid` fallback applies to
// exactly one case: an ordinary rowid table with no declared rowid, _rowid_,
// or oid shadow, no user ORDER BY, and no aggregate/group shape. Views,
// virtual tables, WITHOUT ROWID tables, every declared-shadow case,
// aggregate-only and grouped queries, and other ineligible shapes are
// explicit no-fallback cases with no stability claim. Fixtures build
// objects only through internal/schema metadata; no UI paging state and no
// schema classification live here.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// pageCatalog is the rowid-metadata fixture for paging tests: one eligible
// ordinary rowid table, a rowid-shadowed ordinary table for each alias
// spelling, a WITHOUT ROWID table, a virtual table, and a view.
func pageCatalog() *schema.Catalog {
	col := func(name string) schema.Column {
		return schema.Column{Name: name, DeclaredType: "INTEGER", Insertable: true}
	}
	return &schema.Catalog{
		Version: 25,
		Objects: []*schema.Object{
			{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{col("id"), col("name")},
				InsertableCount: 2,
			},
			{
				Name: "shadow_rowid", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas, RowidShadowed: true,
				Columns:         []schema.Column{col("rowid")},
				InsertableCount: 1,
			},
			{
				Name: "shadow_rowid2", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas, RowidShadowed: true,
				Columns:         []schema.Column{col("_rowid_")},
				InsertableCount: 1,
			},
			{
				Name: "shadow_oid", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas, RowidShadowed: true,
				Columns:         []schema.Column{col("oid")},
				InsertableCount: 1,
			},
			{
				Name: "nowrap", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidWithout,
				Columns:         []schema.Column{col("k")},
				InsertableCount: 1,
			},
			{
				Name: "vtab", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{col("title")},
				InsertableCount: 1,
			},
			{
				Name: "vw", Kind: schema.KindView, Rowid: schema.RowidNotApplicable,
				Columns: []schema.Column{{Name: "line"}},
			},
		},
	}
}

// pageSelect drives a fresh builder to SELECT over table with the wildcard
// projection committed.
func pageSelect(table string) QueryBuilder {
	return NewQuery().RefreshSchema(pageCatalog()).
		SelectCommand(CommandSelect).SelectTable(table).
		AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
}

// pageColumnSelect drives a fresh builder to SELECT over items with one
// named column committed.
func pageColumnSelect(column string) QueryBuilder {
	return NewQuery().RefreshSchema(pageCatalog()).
		SelectCommand(CommandSelect).SelectTable("items").
		AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: column}).Builder.
		CompleteProjectionAggregate(column, AggregateValue).Builder
}

func TestPageSQLEligibleFallbackRanges(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(QueryBuilder) QueryBuilder
		pageLimit  int64
		offset     int64
		want       string
		wantParams []any
	}{
		{
			name:      "first page appends rowid fallback",
			setup:     func(q QueryBuilder) QueryBuilder { return q },
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0`,
			pageLimit: 5,
			offset:    0,
		},
		{
			name:      "adjacent page exact range",
			setup:     func(q QueryBuilder) QueryBuilder { return q },
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 5`,
			pageLimit: 5,
			offset:    5,
		},
		{
			name:       "where params preserved in order",
			setup:      func(q QueryBuilder) QueryBuilder { return whereCompleteEq(q, "42") },
			want:       `SELECT * FROM "items" WHERE "name" = ? ORDER BY rowid LIMIT 5 OFFSET 0`,
			wantParams: []any{int64(42)},
			pageLimit:  5,
			offset:     0,
		},
		{
			name:      "page limit clamped to remaining user limit",
			setup:     func(q QueryBuilder) QueryBuilder { return q.SetLimitInput("8") },
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 3 OFFSET 5`,
			pageLimit: 5,
			offset:    5,
		},
		{
			name:      "offset beyond user limit yields no statement",
			setup:     func(q QueryBuilder) QueryBuilder { return q.SetLimitInput("8") },
			pageLimit: 5,
			offset:    8,
			want:      "",
		},
		{
			name:      "unbounded logical result keeps page limit",
			setup:     func(q QueryBuilder) QueryBuilder { return q.SetLimitInput("") },
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 10`,
			pageLimit: 5,
			offset:    10,
		},
		{
			name:      "page limit below remaining user limit",
			setup:     func(q QueryBuilder) QueryBuilder { return q.SetLimitInput("100") },
			want:      `SELECT * FROM "items" ORDER BY rowid LIMIT 4 OFFSET 5`,
			pageLimit: 4,
			offset:    5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.setup(selectWildcard(pageSelect("items")))
			got := q.PageSQL(tt.pageLimit, tt.offset)
			if got != tt.want {
				t.Errorf("PageSQL(%d, %d) = %q, want %q", tt.pageLimit, tt.offset, got, tt.want)
			}
			var wantParams []any
			if tt.wantParams != nil {
				wantParams = tt.wantParams
			}
			if got := q.PageParams(); !reflect.DeepEqual(got, wantParams) {
				t.Errorf("PageParams() = %v, want %v", got, wantParams)
			}
		})
	}
}

// pageAggregateSelect drives a fresh builder to SELECT COUNT("id") over
// items with the column projection committed (wildcard never coexists with
// aggregates).
func pageAggregateSelect() QueryBuilder {
	q := NewQuery().RefreshSchema(pageCatalog()).
		SelectCommand(CommandSelect).SelectTable("items").
		AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "id"}).Builder
	return q.CompleteProjectionAggregate("id", AggCount).Builder
}

func TestPageSQLDefaultRangeValues(t *testing.T) {
	// Zero offset with page limit 5 renders the canonical first range.
	q := pageSelect("items")
	want := `SELECT * FROM "items" ORDER BY rowid LIMIT 5 OFFSET 0`
	if got := q.PageSQL(5, 0); got != want {
		t.Errorf("PageSQL(5, 0) = %q, want %q", got, want)
	}
}

func TestPageSQLInvalidRangesRefused(t *testing.T) {
	q := pageSelect("items")
	for _, tt := range []struct {
		name      string
		pageLimit int64
		offset    int64
	}{
		{name: "zero page limit", pageLimit: 0, offset: 0},
		{name: "negative page limit", pageLimit: -3, offset: 0},
		{name: "negative offset", pageLimit: 5, offset: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := q.PageSQL(tt.pageLimit, tt.offset); got != "" {
				t.Errorf("PageSQL(%d, %d) = %q, want empty", tt.pageLimit, tt.offset, got)
			}
		})
	}
}

func TestPageSQLUserOrderByPreservedWithoutRowid(t *testing.T) {
	tests := []struct {
		name string
		dir  Direction
		want string
	}{
		{name: "ascending keeps exact expression", dir: DirAsc, want: `SELECT "id" FROM "items" ORDER BY "id" ASC LIMIT 5 OFFSET 0`},
		{name: "descending keeps exact direction", dir: DirDesc, want: `SELECT "id" FROM "items" ORDER BY "id" DESC LIMIT 5 OFFSET 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, ok := pageColumnSelect("id").AcceptOrderBy("order-column:id")
			if !ok {
				t.Fatal("AcceptOrderBy refused")
			}
			q = q.SetOrderDirection(tt.dir)
			if got := q.PageSQL(5, 0); got != tt.want {
				t.Errorf("PageSQL(5, 0) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPageSQLAggregateGroupedOrderByPreserved(t *testing.T) {
	build := func(dir Direction) QueryBuilder {
		q := pageAggregateSelect()
		q, ok := q.AcceptGroupColumn("name")
		if !ok {
			panic("setup: AcceptGroupColumn failed")
		}
		q, ok = q.AcceptOrderBy("order-aggregate:id:COUNT")
		if !ok {
			panic("setup: AcceptOrderBy failed")
		}
		return q.SetOrderDirection(dir)
	}
	for _, tt := range []struct {
		name string
		dir  Direction
		want string
	}{
		{name: "grouped aggregate ascending", dir: DirAsc, want: `SELECT COUNT("id") FROM "items" GROUP BY "name" ORDER BY COUNT("id") ASC LIMIT 5 OFFSET 0`},
		{name: "grouped aggregate descending", dir: DirDesc, want: `SELECT COUNT("id") FROM "items" GROUP BY "name" ORDER BY COUNT("id") DESC LIMIT 5 OFFSET 0`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := build(tt.dir).PageSQL(5, 0); got != tt.want {
				t.Errorf("PageSQL(5, 0) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPageSQLAggregateGroupedNoFallback(t *testing.T) {
	// Aggregate-only and grouped queries over the eligible table are never
	// implicitly ordered: the page request stays exactly unordered.
	aggOnly := pageAggregateSelect()
	want := `SELECT COUNT("id") FROM "items" LIMIT 5 OFFSET 5`
	if got := aggOnly.PageSQL(5, 5); got != want {
		t.Errorf("aggregate-only PageSQL(5, 5) = %q, want %q", got, want)
	}

	grouped := pageSelect("items")
	grouped, ok := grouped.AcceptGroupColumn("name")
	if !ok {
		t.Fatal("AcceptGroupColumn(\"name\") refused")
	}
	want = `SELECT * FROM "items" GROUP BY "name" LIMIT 5 OFFSET 0`
	if got := grouped.PageSQL(5, 0); got != want {
		t.Errorf("grouped PageSQL(5, 0) = %q, want %q", got, want)
	}
}

func TestPageSQLExcludedObjectsNoFallback(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		projection ProjectionCandidate
		want       string
	}{
		{
			name:       "view is unordered",
			table:      "vw",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "vw" LIMIT 5 OFFSET 0`,
		},
		{
			name:       "virtual table is unordered",
			table:      "vtab",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "vtab" LIMIT 5 OFFSET 0`,
		},
		{
			name:       "WITHOUT ROWID table is unordered",
			table:      "nowrap",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "nowrap" LIMIT 5 OFFSET 0`,
		},
		{
			name:       "rowid alias shadow is unordered",
			table:      "shadow_rowid",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "shadow_rowid" LIMIT 5 OFFSET 0`,
		},
		{
			name:       "_rowid_ alias shadow is unordered",
			table:      "shadow_rowid2",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "shadow_rowid2" LIMIT 5 OFFSET 0`,
		},
		{
			name:       "oid alias shadow is unordered",
			table:      "shadow_oid",
			projection: ProjectionCandidate{Kind: ProjectionWildcard},
			want:       `SELECT * FROM "shadow_oid" LIMIT 5 OFFSET 0`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQuery().RefreshSchema(pageCatalog()).
				SelectCommand(CommandSelect).SelectTable(tt.table).
				AcceptProjection(tt.projection).Builder
			if got := q.PageSQL(5, 0); got != tt.want {
				t.Errorf("PageSQL(5, 0) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPageSQLIneligibleShapesRefused(t *testing.T) {
	tests := []struct {
		name  string
		build func() QueryBuilder
	}{
		{
			name:  "unselected command",
			build: func() QueryBuilder { return NewQuery().RefreshSchema(pageCatalog()) },
		},
		{
			name: "update command over eligible table",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(pageCatalog()).
					SelectCommand(CommandUpdate).SelectTable("items")
			},
		},
		{
			name: "select with no table selected",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(pageCatalog()).
					SelectCommand(CommandSelect)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.build().PageSQL(5, 0); got != "" {
				t.Errorf("PageSQL(5, 0) = %q, want empty", got)
			}
		})
	}
}
