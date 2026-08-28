package querybuilder

import "testing"

// countFixture builds a runnable SELECT builder snapshot with the given WHERE
// value, group entries, and limit input so count-request tests exercise the
// complete production rendering path.
func countFixture(t *testing.T, whereValue, limit string) QueryBuilder {
	t.Helper()

	q := buildSelect().AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
	if whereValue != "" {
		q = whereCompleteEq(q, whereValue)
	}
	if limit != "" {
		q = q.SetLimitInput(limit)
	}
	return q
}

// TestCountSQLCountsTheCompleteSelect covers the Issue #24 count construction
// contract: the count statement wraps the builder's complete SELECT —
// including any user Limit — as a subquery, so rows beyond the Limit are
// irrelevant and the wording never implies a table count or pre-Limit count.
// Bound parameters and their order are preserved unchanged.
func TestCountSQLCountsTheCompleteSelect(t *testing.T) {
	tests := []struct {
		name       string
		whereValue string
		limit      string
		wantSQL    string
		wantParams []any
	}{
		{
			name:       "no limit",
			wantSQL:    `SELECT COUNT(*) FROM (SELECT * FROM "items")`,
			wantParams: nil,
		},
		{
			name:       "user limit stays inside the subquery",
			limit:      "3",
			wantSQL:    `SELECT COUNT(*) FROM (SELECT * FROM "items" LIMIT 3)`,
			wantParams: nil,
		},
		{
			name:       "bound where value",
			whereValue: "42",
			wantSQL:    `SELECT COUNT(*) FROM (SELECT * FROM "items" WHERE "name" = ?)`,
			wantParams: []any{int64(42)},
		},
		{
			name:       "bound where value with limit keeps parameter order",
			whereValue: "42",
			limit:      "5",
			wantSQL:    `SELECT COUNT(*) FROM (SELECT * FROM "items" WHERE "name" = ? LIMIT 5)`,
			wantParams: []any{int64(42)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := countFixture(t, tc.whereValue, tc.limit)

			if got := q.CountSQL(); got != tc.wantSQL {
				t.Errorf("CountSQL() = %q, want exactly %q", got, tc.wantSQL)
			}
			gotParams := q.CountParams()
			if len(gotParams) != len(tc.wantParams) {
				t.Fatalf("CountParams() = %v, want %v", gotParams, tc.wantParams)
			}
			for i := range tc.wantParams {
				if gotParams[i] != tc.wantParams[i] {
					t.Errorf("CountParams()[%d] = %v, want %v (parameter order must be preserved)", i, gotParams[i], tc.wantParams[i])
				}
			}
			// The count derives from the unchanged complete SELECT: identical
			// predicates, projection, grouping, ordering, and parameter order.
			if base := q.SelectSQL(); q.CountSQL() != "SELECT COUNT(*) FROM ("+base+")" {
				t.Errorf("CountSQL() is not the complete SELECT subquery of %q", base)
			}
			// The count parameters are exactly the SELECT's own parameters.
			if selParams := q.SelectParams(); len(selParams) != len(gotParams) {
				t.Errorf("CountParams() = %v differs from SelectParams() = %v", gotParams, selParams)
			}
		})
	}
}

// TestCountSQLAggregateAndGroupedSelect covers aggregate/grouped SELECTs:
// the count counts the complete grouped result, never the table.
func TestCountSQLAggregateAndGroupedSelect(t *testing.T) {
	q := buildSelect()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"}).Builder
	q = q.CompleteProjectionAggregate("name", AggCount).Builder
	q, ok := q.AcceptGroupColumn("id")
	if !ok {
		t.Fatal("AcceptGroupColumn(\"id\") refused")
	}
	q = q.SetLimitInput("9")

	want := `SELECT COUNT(*) FROM (SELECT COUNT("name") FROM "items" GROUP BY "id" LIMIT 9)`
	if got := q.CountSQL(); got != want {
		t.Errorf("CountSQL() = %q, want exactly %q", got, want)
	}
}

// TestCountSQLRequiresSelectCommand covers that a non-SELECT snapshot cannot
// be counted at all: empty output means the snapshot is not countable, never
// a partially valid statement.
func TestCountSQLRequiresSelectCommand(t *testing.T) {
	q := buildUpdate()
	if got := q.CountSQL(); got != "" {
		t.Errorf("CountSQL() = %q for non-SELECT, want empty", got)
	}
}
