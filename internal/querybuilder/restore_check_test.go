package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

func testCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 17,
		Objects: []*schema.Object{
			{
				Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{{Name: "id"}, {Name: "email"}, {Name: "note"}},
			},
		},
	}
}

func buildRichSelect() QueryBuilder {
	q := NewQuery().RefreshSchema(testCatalog()).
		SelectCommand(CommandSelect).SelectTable("users")
	out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "email"})
	q = out.Builder.CompleteProjectionAggregate("email", AggregateValue).Builder
	out = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "note"})
	q = out.Builder.CompleteProjectionAggregate("note", AggregateValue).Builder
	next, ok := q.AcceptGroupColumn("note")
	if !ok {
		panic("group")
	}
	q = next
	next, ok = q.AcceptOrderBy("order-column:note")
	if !ok {
		panic("order")
	}
	q = next.SetOrderDirection(DirDesc).SetLimitInput("7")
	next, ok = q.StartWhere("email")
	if !ok {
		panic("where start")
	}
	draft, ok := next.WhereDraft().ChooseOperator(OpEq)
	if !ok {
		panic("op")
	}
	next = next.ApplyWhereDraft(draft)
	draft, ok = draft.SubmitValue("x")
	next = next.ApplyWhereDraft(draft)
	q, ok = next.CommitWhereDraft()
	if !ok {
		panic("commit")
	}
	return q
}

func TestRestoreRichSelectRoundTrip(t *testing.T) {
	src := buildRichSelect()
	state := src.HistoryState()
	restored, ok := RestoreBuilder(state, testCatalog())
	if !ok {
		t.Fatal("RestoreBuilder failed")
	}
	if !restored.HistoryState().Equal(state) {
		t.Fatalf("restored state differs:\n got %+v\nwant %+v", restored.HistoryState(), state)
	}
}
