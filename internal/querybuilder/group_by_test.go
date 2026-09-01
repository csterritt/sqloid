// Pure table-driven tests for SELECT GROUP BY state and validation
// (Issue #18 Task 1): assisted multi-selection order preservation, duplicate
// and stale-identity rejection, the complete grouping validity matrix over
// grouped nonaggregates / mixed projections / all-aggregate projections /
// bare COUNT(*) / wildcard, and safely quoted deterministic SQL generation.
//
// Data validity is asserted at the QueryBuilder boundary through exact
// first-invalid field/reason contracts; popup behavior stays in internal/ui.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// groupFixture returns a SELECT builder on a two-visible-column users table.
func groupFixture() QueryBuilder {
	obj := &schema.Object{
		Name:          "users",
		Kind:          schema.KindOrdinaryTable,
		WriteEligible: true,
		Rowid:         schema.RowidHas,
		Columns: []schema.Column{
			{Name: "id"},
			{Name: "email"},
			{Name: "created_at", Hidden: true},
		},
	}
	return selectBuilderFor(obj)
}

// commitProjection commits one named column with the given aggregate onto the
// fixture projection, panicking if the setup step fails so tests fail loudly.
func commitProjection(t *testing.T, q QueryBuilder, column string, agg Aggregate) QueryBuilder {
	t.Helper()
	out := q.CompleteProjectionAggregate(column, agg)
	if out.ReopenColumns == false && agg != AggregateValue {
		t.Fatalf("setup projection of %s failed", column)
	}
	return out.Builder
}

func groupEntriesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGroupByCandidatesFollowVisibleSchemaColumns(t *testing.T) {
	q := groupFixture()
	got := q.GroupByCandidates()
	want := []string{"id", "email"}
	if !groupEntriesEqual(got, want) {
		t.Fatalf("GroupByCandidates()=%v, want %v (hidden columns excluded)", got, want)
	}
}

func TestGroupByCandidatesRequireSelectWithTable(t *testing.T) {
	if got := NewQuery().GroupByCandidates(); len(got) != 0 {
		t.Fatalf("unselected command offered %v", got)
	}
	obj := &schema.Object{Name: "users", Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}}}
	q := NewQuery().
		RefreshSchema(&schema.Catalog{Version: 1, Objects: []*schema.Object{obj}}).
		SelectCommand(CommandSelect)
	if got := q.GroupByCandidates(); len(got) != 0 {
		t.Fatalf("SELECT without a table offered %v", got)
	}
}

func TestAcceptGroupColumnPreservesSelectionOrderAndRejectsDuplicates(t *testing.T) {
	q := groupFixture()
	var err bool
	for _, name := range []string{"email", "id"} { // deliberately reverse-schema order
		q, err = q.AcceptGroupColumn(name)
		if !err {
			t.Fatalf("AcceptGroupColumn(%q) rejected", name)
		}
	}
	if got := q.GroupByEntries(); !groupEntriesEqual(got, []string{"email", "id"}) {
		t.Fatalf("entries=%v, want selection order [email id]", got)
	}

	// An exact duplicate is an immutable no-op: state unchanged, no error.
	before := q.GroupByEntries()
	q2, err := q.AcceptGroupColumn("email")
	if err || !groupEntriesEqual(q2.GroupByEntries(), before) {
		t.Fatalf("duplicate accept changed state: entries=%v ok=%v", q2.GroupByEntries(), err)
	}

	// A foreign identity that was never among eligible columns is rejected.
	if _, err := q.AcceptGroupColumn("created_at"); err {
		t.Fatalf("hidden column accepted as a group")
	}
	if _, err := q.AcceptGroupColumn(""); err {
		t.Fatalf("empty identity accepted as a group")
	}
}

func TestAcceptGroupColumnRejectsStaleIdentityAfterRefresh(t *testing.T) {
	q := groupFixture()
	bare := &schema.Object{Name: "users", Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}}} // email vanished from the catalog
	q = q.RefreshSchema(&schema.Catalog{Version: 4, Objects: []*schema.Object{bare}})
	if _, ok := q.AcceptGroupColumn("email"); ok {
		t.Fatalf("stale/foreign identity committed as a group after refresh")
	}
	if len(q.GroupByEntries()) != 0 {
		t.Fatalf("rejected stale accept changed state: %v", q.GroupByEntries())
	}
}

func TestCommandAndTableReplacementClearsGroups(t *testing.T) {
	q, _ := groupFixture().AcceptGroupColumn("id")
	next := q.SelectCommand(CommandUpdate)
	if len(next.GroupByEntries()) != 0 {
		t.Fatalf("command replacement left groups behind: %v", next.GroupByEntries())
	}
	q, _ = groupFixture().AcceptGroupColumn("id")
	vanished := q.RefreshSchema(&schema.Catalog{Version: 9, Objects: nil})
	if len(vanished.GroupByEntries()) != 0 {
		t.Fatalf("table vanishing left groups behind: %v", vanished.GroupByEntries())
	}
}

// groupingCase drives one matrix row: commits the named projection entries,
// adds the named groups, then reports first-invalid at the QueryBuilder
// boundary. rows use map[string]Aggregate for compact setup.
type groupingCase struct {
	name       string
	projection []struct {
		col string
		agg Aggregate
	}
	groups     []string
	wantValid  bool
	wantField  string // when invalid
	wantReason string // when invalid
}

func runGroupingCase(t *testing.T, tc groupingCase) {
	t.Helper()
	q := groupFixture()
	for _, p := range tc.projection {
		if p.col == "*" {
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
			continue
		}
		if p.col == "" { // synthetic bare COUNT(*) sentinel
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
			continue
		}
		q = commitProjection(t, q, p.col, p.agg)
	}
	for _, g := range tc.groups {
		q, _ = q.AcceptGroupColumn(g)
	}
	issue, invalid := q.FirstInvalidIssue()
	if tc.wantValid && invalid {
		t.Fatalf("%s: unexpected first-invalid %+v", tc.name, issue)
	}
	if !tc.wantValid {
		if !invalid {
			t.Fatalf("%s: expected invalid (%s/%s), got none", tc.name, tc.wantField, tc.wantReason)
		}
		if issue.Field != tc.wantField || issue.Reason != tc.wantReason {
			t.Fatalf("%s: first-invalid=(%q,%q), want=(%q,%q)",
				tc.name, issue.Field, issue.Reason, tc.wantField, tc.wantReason)
		}
	}
}

func TestGroupingValidityMatrix(t *testing.T) {
	mixedReason := MixedAggregationNeedsGroupReason
	wildcardReason := WildcardGroupedByReason
	cases := []groupingCase{
		{
			name: "nonaggregate-only without group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggregateValue}},
			groups:    nil,
			wantValid: true,
		},
		{
			name: "mixed aggregate/nonaggregate without group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggCount}, {"email", AggregateValue}},
			groups:    nil,
			wantValid: false, wantField: FieldIdentityGroupBy, wantReason: mixedReason,
		},
		{
			name: "missing one required group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggMin}, {"email", AggregateValue}},
			// email stays ungrouped while id is grouped: the mixed projection
			// leaves exactly one required group missing.
			groups:    []string{"id"},
			wantValid: false, wantField: FieldIdentityGroupBy, wantReason: mixedReason,
		},
		{
			name: "extra grouped columns are permitted",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggregateValue}},
			groups:    []string{"id", "email"},
			wantValid: true,
		},
		{
			name: "every nonaggregate grouped",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggMax}, {"email", AggregateValue}},
			groups:    []string{"email"},
			wantValid: true,
		},
		{
			name: "all-aggregate projection without group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"id", AggSum}, {"email", AggAvg}},
			groups:    nil,
			wantValid: true,
		},
		{
			name: "bare COUNT(*) without group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"", 0}},
			groups:    nil,
			wantValid: true,
		},
		{
			name: "wildcard with any group",
			projection: []struct {
				col string
				agg Aggregate
			}{{"*", 0}},
			groups:    []string{"id"},
			wantValid: false, wantField: FieldIdentityGroupBy, wantReason: wildcardReason,
		},
	}
	for _, tc := range cases {
		runGroupingCase(t, tc)
	}
}

func TestFirstInvalidIssueAbsentWhenClean(t *testing.T) {
	q, _ := groupFixture().AcceptGroupColumn("id")
	if issue, invalid := q.FirstInvalidIssue(); invalid {
		t.Fatalf("clean query reported %+v", issue)
	}
}

func TestGroupBySQLRendering(t *testing.T) {
	q := groupFixture()
	q = commitProjection(t, q, "id", AggregateValue)
	q = commitProjection(t, q, "email", AggregateValue)
	got := q.SelectSQL()
	want := `SELECT "id", "email" FROM "users"`
	if got != want {
		t.Fatalf("SelectSQL()=%q, want %q (no GROUP BY clause)", got, want)
	}

	q = groupFixture()
	q = commitProjection(t, q, "id", AggCount)
	q = commitProjection(t, q, "email", AggregateValue)
	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptGroupColumn("id")
	got = q.SelectSQL()
	want = `SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id"`
	if got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
}

func TestGroupBySQLEscapesEmbeddedQuotes(t *testing.T) {
	obj := &schema.Object{Name: `we"ird`, Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: `col"x`}, {Name: "b"}}}
	q := selectBuilderFor(obj).RefreshSchema(&schema.Catalog{Version: 1, Objects: []*schema.Object{obj}}).
		SelectCommand(CommandSelect).SelectTable(`we"ird`)
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: `col"x`}).Builder
	q = q.CompleteProjectionAggregate(`col"x`, AggregateValue).Builder
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "b"}).Builder
	q = q.CompleteProjectionAggregate("b", AggregateValue).Builder
	q, _ = q.AcceptGroupColumn(`col"x`)
	q, _ = q.AcceptGroupColumn("b")
	want := `SELECT "col""x", "b" FROM "we""ird" GROUP BY "col""x", "b"`
	if got := q.SelectSQL(); got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
}

// TestGroupByKeepsProjectionOrderAndParams pins that grouping state never
// reorders, replaces, or repaginates the committed projection: identical
// projection SQL text before and after grouping, entries untouched, and no
// parameters introduced by grouping itself.
func TestGroupByKeepsProjectionOrderAndParams(t *testing.T) {
	q := groupFixture()
	q = commitProjection(t, q, "id", AggCount)
	q = commitProjection(t, q, "email", AggregateValue)
	before := q.ProjectionEntries()
	// The pre-group state is a mixed aggregate/nonaggregate projection
	// without GROUP BY, so the authoritative runnable gate (Issue #66)
	// refuses to render it; projection order is observed through entries,
	// and the post-group state renders the exact ordered SQL.
	if params := q.SelectParams(); len(params) != 0 {
		t.Fatalf("ungrouped query carried params %v", params)
	}

	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptGroupColumn("id")
	after := q.ProjectionEntries()
	if !sameEntries(before, after) {
		t.Fatalf("grouping changed projection entries: %v -> %v", before, after)
	}
	wantPost := `SELECT COUNT("id"), "email" FROM "users" GROUP BY "email", "id"`
	if got := q.SelectSQL(); got != wantPost {
		t.Fatalf("post-group SQL=%q, want %q", got, wantPost)
	}
	if params := q.SelectParams(); len(params) != 0 {
		t.Fatalf("grouping introduced params %v", params)
	}
}
