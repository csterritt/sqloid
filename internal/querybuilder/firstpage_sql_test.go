// Exact SQL and bound-parameter contract for the first production SELECT
// path (Issue #22 Task 3): QueryBuilder is the sole source of safely quoted
// SQL and ordered parameters. These focused tests pin representative
// runnable states — wildcard, duplicate-label aggregate projections, WHERE
// values, GROUP/ORDER choices, and user Limit — so the execution route
// consumes exactly this text, unaltered.

package querybuilder

import (
	"reflect"
	"testing"
)

func TestSelectSQLWildcard(t *testing.T) {
	q := buildSelect().AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
	want := `SELECT * FROM "items"`
	if got := q.SelectSQL(); got != want {
		t.Errorf("SelectSQL() = %q, want %q", got, want)
	}
	if params := q.SelectParams(); len(params) != 0 {
		t.Errorf("SelectParams() = %v, want none", params)
	}
}

func TestSelectSQLDuplicateLabelAggregates(t *testing.T) {
	// Two aggregate entries over different columns produce two COUNT labels
	// that will deduplicate downstream; the SQL text itself is unaltered.
	q := buildSelect()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "id"}).Builder
	q = q.CompleteProjectionAggregate("id", AggCount).Builder
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"}).Builder
	q = q.CompleteProjectionAggregate("name", AggCount).Builder
	want := `SELECT COUNT("id"), COUNT("name") FROM "items"`
	if got := q.SelectSQL(); got != want {
		t.Errorf("SelectSQL() = %q, want %q", got, want)
	}
}

func TestSelectSQLWhereParamsOrdered(t *testing.T) {
	q := whereCompleteEq(buildSelect().AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder, "42")
	want := `SELECT * FROM "items" WHERE "name" = ?`
	if got := q.SelectSQL(); got != want {
		t.Errorf("SelectSQL() = %q, want %q", got, want)
	}
	wantParams := []any{int64(42)}
	if got := q.SelectParams(); !reflect.DeepEqual(got, wantParams) {
		t.Errorf("SelectParams() = %v, want %v", got, wantParams)
	}
}

func TestSelectSQLGroupOrderLimit(t *testing.T) {
	q := buildSelect()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"}).Builder
	q = q.CompleteProjectionAggregate("name", AggCount).Builder
	q, ok := q.AcceptGroupColumn("id")
	if !ok {
		t.Fatal("AcceptGroupColumn(\"id\") refused")
	}
	q, ok = q.AcceptOrderBy("order-aggregate:name:COUNT")
	if !ok {
		t.Fatalf("AcceptOrderBy refused; candidates: %+v", q.OrderByCandidates())
	}
	q = q.SetOrderDirection(DirDesc)
	q = q.SetLimitInput("7")
	want := `SELECT COUNT("name") FROM "items" GROUP BY "id" ORDER BY COUNT("name") DESC LIMIT 7`
	if got := q.SelectSQL(); got != want {
		t.Errorf("SelectSQL() = %q, want %q", got, want)
	}
	if params := q.SelectParams(); len(params) != 0 {
		t.Errorf("SelectParams() = %v, want none", params)
	}
}
