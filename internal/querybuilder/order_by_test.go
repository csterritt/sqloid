// Pure table-driven tests for SELECT ORDER BY state (Issue #18 Task 3):
// context-derived candidate identity, single immutable selection with atomic
// replacement, the closed ASC/DESC direction with the ASC default, Up/Down
// toggle seams, and exact safely quoted SQL rendering. Candidate eligibility
// is authoritative at the QueryBuilder boundary; popup behavior stays in
// internal/ui.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// orderFixture returns a SELECT builder on a two-visible-column users table,
// sharing the grouping test's shape so both features exercise one identity
// model over the same stable column set.
func orderFixture() QueryBuilder { return groupFixture() }

func TestOrderByUngroupedOffersVisibleSchemaColumns(t *testing.T) {
	q := orderFixture()
	cands := q.OrderByCandidates()
	if len(cands) != 2 {
		t.Fatalf("candidates=%+v, want id then email in Schema order", cands)
	}
	for i, want := range []string{"id", "email"} {
		if cands[i].Key != "order-column:"+want || cands[i].Display != want {
			t.Fatalf("candidate %d = %+v, want column %s", i, cands[i], want)
		}
	}
	// A fresh snapshot has no committed expression yet.
	if _, _, ok := q.OrderBySelection(); ok {
		t.Fatalf("fresh snapshot reported a committed ORDER BY")
	}
}

func TestOrderByGroupedContextOffersOnlyGroupedAndSelectedAggregates(t *testing.T) {
	q := orderFixture()
	q = commitProjection(t, q, "id", AggCount)
	q = commitProjection(t, q, "email", AggregateValue)
	q, _ = q.AcceptGroupColumn("id")
	// id is grouped, email is a projected plain column but not grouped, and
	// COUNT(id) is a selected aggregate: only those two identities are
	// offered, never the ungrouped nonaggregate email column.
	var got []string
	for _, c := range q.OrderByCandidates() {
		got = append(got, c.Key)
	}
	want := []string{"order-column:id", "order-aggregate:id:COUNT"}
	if !equalStrings(got, want) {
		t.Fatalf("candidates=%v, want %v", got, want)
	}
}

func TestOrderByGroupedContextIncludesBareCountStar(t *testing.T) {
	q := orderFixture()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	q, _ = q.AcceptGroupColumn("email")
	var got []string
	for _, c := range q.OrderByCandidates() {
		got = append(got, c.Key)
	}
	want := []string{"order-column:email", "order-count-star"}
	if !equalStrings(got, want) {
		t.Fatalf("candidates=%v, want grouped column then bare COUNT(*) (got %v)", want, got)
	}
	if c := q.OrderByCandidates()[1]; c.Display != "COUNT(*)" {
		t.Fatalf("sentinel display=%q, want COUNT(*)", c.Display)
	}
}

func TestOrderByCandidateKeysDistinguishEqualLabels(t *testing.T) {
	// A visible column literally named COUNT(id) beside the aggregate
	// COUNT(id) produces two candidates with identical display text but
	// distinct reserved-prefix identities.
	obj := &schema.Object{Name: "users", Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "COUNT(id)"}, {Name: "id"}}}
	q := selectBuilderFor(obj)
	q = commitProjection(t, q, "id", AggCount)
	q, _ = q.AcceptGroupColumn("COUNT(id)")
	cands := q.OrderByCandidates()
	if len(cands) != 2 || cands[0].Display != "COUNT(id)" || cands[1].Display != "COUNT(id)" {
		t.Fatalf("candidates=%+v, want two equal-label rows", cands)
	}
	if cands[0].Key == cands[1].Key {
		t.Fatalf("equal labels shared one identity: %q", cands[0].Key)
	}
}

func TestOrderByExcludesWildcardAndUnselectedAggregates(t *testing.T) {
	// A wildcard-only ungrouped SELECT offers table columns, never the
	// wildcard itself or any synthetic identity.
	q := orderFixture().AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
	for _, c := range q.OrderByCandidates() {
		if c.Key != "order-column:"+c.col {
			t.Fatalf("wildcard context offered non-column candidate %+v", c)
		}
	}

	// In a grouped context an aggregate absent from the projection is never
	// offered, and neither is any stale table column outside the groups.
	q = orderFixture()
	q = commitProjection(t, q, "email", AggCount)
	q, _ = q.AcceptGroupColumn("email")
	for _, c := range q.OrderByCandidates() {
		if c.Key == "order-aggregate:id:COUNT" || c.Key == "order-column:id" {
			t.Fatalf("grouped context offered unselected identity %+v", c)
		}
	}
}

func TestOrderByContextChangeMakesCommittedSelectionStale(t *testing.T) {
	q := orderFixture()
	q2, ok := q.AcceptOrderBy("order-column:id")
	if !ok {
		t.Fatalf("valid candidate rejected")
	}
	// The email column vanishes from the catalog: the committed identity is
	// no longer offered and becomes a precise first-invalid report.
	stale := q2.RefreshSchema(&schema.Catalog{Version: 8, Objects: []*schema.Object{
		{Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true,
			Rowid: schema.RowidHas, Columns: []schema.Column{{Name: "email"}}},
	}})
	if _, _, still := stale.OrderBySelection(); still {
		t.Fatalf("stale identity still resolved as a selection")
	}
	issue, invalid := stale.FirstInvalidIssue()
	if !invalid || issue.Field != FieldIdentityOrderBy || issue.Reason != StaleOrderByExpressionReason {
		t.Fatalf("stale order reported %+v, want %s/%s",
			issue, FieldIdentityOrderBy, StaleOrderByExpressionReason)
	}
}

func TestAcceptOrderByRejectsArbitraryAndStaleKeys(t *testing.T) {
	q := orderFixture()
	if _, ok := q.AcceptOrderBy("garbage"); ok {
		t.Fatalf("arbitrary text accepted as an ORDER BY expression")
	}
	if _, ok := q.AcceptOrderBy("order-aggregate:id:COUNT"); ok {
		t.Fatalf("unselected aggregate accepted as an ORDER BY expression")
	}
	if _, ok := q.AcceptOrderBy("wildcard:*"); ok {
		t.Fatalf("wildcard identity accepted as an ORDER BY expression")
	}
	if _, _, ok := q.OrderBySelection(); ok {
		t.Fatalf("rejected accepts left a selection behind")
	}
}

func TestAcceptOrderByDefaultsToAscAndReplacesAtomically(t *testing.T) {
	q := orderFixture()
	q, ok := q.AcceptOrderBy("order-column:email")
	if !ok {
		t.Fatalf("valid candidate rejected")
	}
	if _, dir, ok := q.OrderBySelection(); !ok || dir != DirAsc {
		t.Fatalf("fresh selection direction=%v ok=%v, want ASC", dir, ok)
	}
	// Replacement is atomic: a new acceptance always resets to the ASC
	// default rather than keeping the previous direction.
	q = q.SetOrderDirection(DirDesc)
	q2, ok := q.AcceptOrderBy("order-column:id")
	if !ok {
		t.Fatalf("replacement candidate rejected")
	}
	if _, dir, ok := q2.OrderBySelection(); !ok || dir != DirAsc {
		t.Fatalf("replacement kept direction %v, want ASC", dir)
	}
	if _, dir, ok := q.OrderBySelection(); !ok || dir != DirDesc {
		t.Fatalf("accept mutated the receiver snapshot")
	}
}

func TestOrderDirectionToggleAndClear(t *testing.T) {
	q, _ := orderFixture().AcceptOrderBy("order-column:id")
	flipped := q.ToggleOrderDirection()
	if _, dir, ok := flipped.OrderBySelection(); !ok || dir != DirDesc {
		t.Fatalf("toggle produced %v ok=%v, want DESC", dir, ok)
	}
	back := flipped.ToggleOrderDirection()
	if _, dir, _ := back.OrderBySelection(); dir != DirAsc {
		t.Fatalf("second toggle produced %v, want ASC", dir)
	}
	// Toggling without a committed selection is an unchanged no-op.
	idle := orderFixture().ToggleOrderDirection()
	if _, _, ok := idle.OrderBySelection(); ok {
		t.Fatalf("toggle on empty selection committed one")
	}
	// Clearing removes the whole selection; a further toggle stays inert.
	cleared := q.ClearOrderBy()
	if _, _, ok := cleared.OrderBySelection(); ok {
		t.Fatalf("clear left a selection behind")
	}
	if _, dir, ok := cleared.ToggleOrderDirection().OrderBySelection(); ok || dir != 0 {
		t.Fatalf("toggle after clear committed %v", dir)
	}
}

func TestOrderBySQLRendering(t *testing.T) {
	q := orderFixture()
	q = commitProjection(t, q, "email", AggregateValue)
	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptOrderBy("order-column:email")
	if got, want := q.SelectSQL(), `SELECT "email" FROM "users" GROUP BY "email" ORDER BY "email" ASC`; got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
	q = q.ToggleOrderDirection()
	if got, want := q.SelectSQL(), `SELECT "email" FROM "users" GROUP BY "email" ORDER BY "email" DESC`; got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}

	// The bare sentinel renders COUNT(*) as the ordered expression.
	q = orderFixture()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptOrderBy("order-count-star")
	want := `SELECT COUNT(*) FROM "users" GROUP BY "email" ORDER BY COUNT(*) ASC`
	if got := q.SelectSQL(); got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
}

func TestOrderBySQLQuotesEmbeddedQuotes(t *testing.T) {
	obj := &schema.Object{Name: `we"ird`, Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: `col"x`}, {Name: "b"}}}
	q := selectBuilderFor(obj)
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: `col"x`}).Builder
	q = q.CompleteProjectionAggregate(`col"x`, AggregateValue).Builder
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "b"}).Builder
	q = q.CompleteProjectionAggregate("b", AggregateValue).Builder
	q, _ = q.AcceptOrderBy(`order-column:col"x`)
	want := `SELECT "col""x", "b" FROM "we""ird" ORDER BY "col""x" ASC`
	if got := q.SelectSQL(); got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
}

func TestOrderByAddsNoParamsAndKeepsProjectionOrder(t *testing.T) {
	q := orderFixture()
	q = commitProjection(t, q, "id", AggCount)
	q = commitProjection(t, q, "email", AggregateValue)
	before := q.ProjectionEntries()
	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptOrderBy("order-aggregate:id:COUNT")
	if !sameEntries(before, q.ProjectionEntries()) {
		t.Fatalf("ordering changed projection entries")
	}
	if params := q.SelectParams(); len(params) != 0 {
		t.Fatalf("ordering introduced params %v", params)
	}
}

func equalStrings(a, b []string) bool {
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
