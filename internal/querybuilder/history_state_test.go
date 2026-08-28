// Exhaustive normalized comparison tests for the history-ready execution
// state (Issue #20 Task 3): every significant field — command, stable table
// identity, ordered projection entries, WHERE presence/column/operator/
// entered value/parsed bound type, GROUP BY order, ORDER BY expression/
// direction, Limit empty-versus-number, and ordered UPDATE assignments and
// INSERT choices and values — proves equality-significant, while transient
// focus, drafts, and entered-but-transient text are excluded.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// historyCatalog is the shared refreshed snapshot for these tests: two
// ordinary write-eligible tables and a view, with visible columns.
func historyCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 21,
		Objects: []*schema.Object{
			{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: "id", DeclaredType: "INTEGER", Insertable: true},
					{Name: "name", DeclaredType: "TEXT", Insertable: true},
					{Name: "score", DeclaredType: "REAL", Insertable: true},
				},
				InsertableCount: 3,
			},
			{
				Name: "logs", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: "line", DeclaredType: "TEXT", Insertable: true},
					{Name: "level", DeclaredType: "TEXT", Insertable: true},
				},
				InsertableCount: 2,
			},
			{Name: "vw", Kind: schema.KindView, Columns: []schema.Column{{Name: "line"}}},
		},
	}
}

// selOn drives a builder to SELECT over table with the wildcard projection.
func selOn(table string) QueryBuilder {
	return selectWildcard(NewQuery().RefreshSchema(historyCatalog()).SelectCommand(CommandSelect).SelectTable(table))
}

// whereComplete completes a WHERE draft with op over column and text.
func whereComplete(q QueryBuilder, column string, op Operator, text string) QueryBuilder {
	next, ok := q.StartWhere(column)
	if !ok {
		panic("setup: StartWhere failed")
	}
	draft, ok := next.WhereDraft().ChooseOperator(op)
	if !ok {
		panic("setup: ChooseOperator failed")
	}
	next = next.ApplyWhereDraft(draft)
	if op.TakesValue() {
		draft, ok = draft.SubmitValue(text)
		if !ok {
			panic("setup: SubmitValue failed")
		}
		next = next.ApplyWhereDraft(draft)
	}
	next, ok = next.CommitWhereDraft()
	if !ok {
		panic("setup: CommitWhereDraft failed")
	}
	return next
}

// historySelect is the base full state: SELECT items wildcard, WHERE
// name = INTEGER "7", groups [name score], ORDER name ASC, Limit 5.
func historySelect() QueryBuilder {
	q := whereComplete(selOn("items"), "name", OpEq, "7")
	q, _ = q.AcceptGroupColumn("name")
	q, _ = q.AcceptGroupColumn("score")
	q, ok := q.AcceptOrderBy("order-column:name")
	if !ok {
		panic("setup: AcceptOrderBy failed")
	}
	return q.SetLimitInput("5")
}

// historyUpdate returns an UPDATE with one submitted Value and one NULL SET.
func historyUpdate() QueryBuilder {
	q := setSubmittedValue(buildUpdate(), "name", "x")
	next, ok := q.AcceptSetColumn("score")
	if !ok {
		panic("setup: AcceptSetColumn failed")
	}
	next, ok = next.ChooseSetAssignment("score", SetChoiceNull)
	if !ok {
		panic("setup: ChooseSetAssignment failed")
	}
	return next
}

// historyInsert returns an INSERT with one Value and one Default/Omit choice.
func historyInsert() QueryBuilder {
	q := buildInsert()
	next, ok := q.ChooseInsertColumn("id", InsertChoiceValue)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	next, ok = next.SubmitInsertValue("id", "42")
	if !ok {
		panic("setup: SubmitInsertValue failed")
	}
	next, ok = next.ChooseInsertColumn("name", InsertChoiceOmit)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	return next
}

// TestHistoryStatePreservesSignificantFields requires the snapshot to carry
// every normalized field of a rich state: exact entered representation,
// parsed bound kind, structural choices, and all orderings.
func TestHistoryStatePreservesSignificantFields(t *testing.T) {
	state := historySelect().HistoryState()
	if state.Command != CommandSelect || !state.TableSet || state.Table != "items" {
		t.Fatalf("command/table = %v, %v %q; want SELECT over items", state.Command, state.TableSet, state.Table)
	}
	wantProjection := []HistoryProjectionEntry{{Kind: ProjectionWildcard}}
	if !reflect.DeepEqual(state.Projection, wantProjection) {
		t.Errorf("projection = %+v, want wildcard", state.Projection)
	}
	if !state.WhereSet || state.WhereColumn != "name" || state.WhereOperator != OpEq {
		t.Fatalf("where identity = (%v, %q, %v); want name =", state.WhereSet, state.WhereColumn, state.WhereOperator)
	}
	if !state.WhereHasValue || state.WhereValue.Kind != KindInteger || state.WhereValue.Int != 7 {
		t.Errorf("where value = %+v; want INTEGER 7", state.WhereValue)
	}
	if state.WhereEntered != "7" {
		t.Errorf("where entered = %q, want exact \"7\"", state.WhereEntered)
	}
	if !reflect.DeepEqual(state.Groups, []string{"name", "score"}) {
		t.Errorf("groups = %v, want acceptance order [name score]", state.Groups)
	}
	if !state.OrderSet || state.OrderExpression != "order-column:name" || state.OrderDirection != DirAsc {
		t.Errorf("order = (%v, %q, %v); want name ASC", state.OrderSet, state.OrderExpression, state.OrderDirection)
	}
	if !state.LimitHas || state.LimitValue != 5 {
		t.Errorf("limit = (%v, %d), want accepted 5", state.LimitHas, state.LimitValue)
	}
	if len(state.Sets) != 0 || len(state.Inserts) != 0 {
		t.Errorf("SELECT state carried write state: %+v %+v", state.Sets, state.Inserts)
	}
}

// TestHistoryStateExcludesTransientState requires two builders differing only
// in transient UI-owned state — open drafts and leading-zero entered Limit
// text with identical accepted identity — to produce normalized-equal
// snapshots.
func TestHistoryStateExcludesTransientState(t *testing.T) {
	base := historySelect()
	drafted, ok := base.StartWhere("name")
	if !ok {
		panic("setup: StartWhere failed")
	}
	if drafted.WhereDrafting() == base.WhereDrafting() {
		t.Fatal("setup: draft state did not differ")
	}
	if !drafted.HistoryState().Equal(base.HistoryState()) {
		t.Error("open WHERE draft changed the normalized state")
	}
	five := base
	zeroFive := five.SetLimitInput("05")
	if !five.HistoryState().Equal(zeroFive.HistoryState()) {
		t.Error("accepted 5 vs 05 differed; entered Limit representation is transient")
	}
}

// TestNormalizedEqualityDifferences requires every significant field to be
// equality-significant even where rendered SQL or bound values could match.
func TestNormalizedEqualityDifferences(t *testing.T) {
	base := historySelect().HistoryState()
	cases := map[string]qbStateMaker{
		"command":                      func() QueryBuilder { return buildUpdate() },
		"table identity":               func() QueryBuilder { return whereComplete(selOn("logs"), "line", OpEq, "7") },
		"projection order":             func() QueryBuilder { return whereComplete(buildPlainSelect(), "name", OpEq, "7") },
		"where presence":               func() QueryBuilder { return selOn("items") },
		"where column":                 func() QueryBuilder { return whereComplete(selOn("items"), "score", OpEq, "7") },
		"where operator":               func() QueryBuilder { return whereComplete(selOn("items"), "name", OpLt, "7") },
		"where entered representation": func() QueryBuilder { return whereComplete(selOn("items"), "name", OpEq, "07") },
		"where bound type":             func() QueryBuilder { return whereComplete(selOn("items"), "name", OpEq, "7.0") },
		"group order": func() QueryBuilder {
			q := whereComplete(selOn("items"), "name", OpEq, "7")
			q, _ = q.AcceptGroupColumn("score")
			q, _ = q.AcceptGroupColumn("name")
			q, _ = q.AcceptOrderBy("order-column:name")
			return q.SetLimitInput("5")
		},
		"order direction": func() QueryBuilder { return historySelect().ToggleOrderDirection() },
		"order expression": func() QueryBuilder {
			q := historySelect()
			q, _ = q.AcceptOrderBy("order-column:score")
			return q
		},
		"limit empty versus number": func() QueryBuilder { return historySelect().ClearLimitValue() },
		"limit number":              func() QueryBuilder { return historySelect().SetLimitInput("6") },
	}
	for name, make := range cases {
		if make().HistoryState().Equal(base) {
			t.Errorf("%s: mutated state compared equal to base", name)
		}
	}
}

// qbStateMaker builds one fresh QueryBuilder for a difference case.
type qbStateMaker func() QueryBuilder

// TestCommandDifferenceIsSignificant proves command participation.
func TestCommandDifferenceIsSignificant(t *testing.T) {
	sel := historySelect().HistoryState()
	upd := historyUpdate().HistoryState()
	if upd.Equal(sel) {
		t.Error("UPDATE compared equal to SELECT")
	}
	if upd.Command != CommandUpdate {
		t.Fatalf("command = %v, want UPDATE", upd.Command)
	}
}

// TestWriteStateNormalizedEquality requires ordered UPDATE assignments and
// INSERT choices/values to be significant, including submission presence and
// entered representation.
func TestWriteStateNormalizedEquality(t *testing.T) {
	upd := historyUpdate()
	state := upd.HistoryState()
	wantSets := []HistorySetAssignment{
		{Column: "name", Choice: SetChoiceValue, HasValue: true, Value: Value{Kind: KindText, Text: "x"}, Entered: "x"},
		{Column: "score", Choice: SetChoiceNull},
	}
	if !reflect.DeepEqual(state.Sets, wantSets) {
		t.Fatalf("sets = %+v, want %+v", state.Sets, wantSets)
	}
	alt := setSubmittedValue(buildUpdate(), "name", "x ")
	alt, _ = alt.AcceptSetColumn("score")
	alt, _ = alt.ChooseSetAssignment("score", SetChoiceNull)
	if alt.HistoryState().Equal(state) {
		t.Error("entered representation difference in SET Value compared equal")
	}
	ins := historyInsert()
	state = ins.HistoryState()
	wantInserts := []HistoryInsertColumn{
		{Column: "id", Choice: InsertChoiceValue, HasValue: true, Value: Value{Kind: KindInteger, Int: 42}, Entered: "42"},
		{Column: "name", Choice: InsertChoiceOmit},
		{Column: "score", Choice: InsertChoiceNone},
	}
	if !reflect.DeepEqual(state.Inserts, wantInserts) {
		t.Fatalf("inserts = %+v, want %+v", state.Inserts, wantInserts)
	}
	insNull := buildInsert()
	next, ok := insNull.ChooseInsertColumn("id", InsertChoiceNull)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	next, ok = next.ChooseInsertColumn("name", InsertChoiceOmit)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	if next.HistoryState().Equal(ins.HistoryState()) {
		t.Error("typed NULL submission compared equal to the SQL-NULL choice")
	}
}

// TestHistoryStateDeepCopiesSlices requires the snapshot's slices to be fresh
// allocations: mutating a returned snapshot cannot alter a re-derived
// snapshot of the same builder, and HistoryState never mutates the receiver.
func TestHistoryStateDeepCopiesSlices(t *testing.T) {
	q := historySelect()
	snap := q.HistoryState()
	snap.Projection[0].Kind = ProjectionColumn
	snap.Groups[0] = "mutated"
	upd := historyUpdate()
	snap = upd.HistoryState()
	snap.Sets[0].Column = "mutated"
	ins := historyInsert()
	snap = ins.HistoryState()
	snap.Inserts[0].Column = "mutated"
	if fresh := q.HistoryState(); fresh.Projection[0].Kind != ProjectionWildcard || fresh.Groups[0] != "name" {
		t.Errorf("mutating a returned snapshot altered builder-derived state: %+v", fresh)
	}
	if fresh := upd.HistoryState(); fresh.Sets[0].Column != "name" {
		t.Errorf("mutating a returned snapshot altered SET state: %+v", fresh.Sets)
	}
	if fresh := ins.HistoryState(); fresh.Inserts[0].Column != "id" {
		t.Errorf("mutating a returned snapshot altered INSERT state: %+v", fresh.Inserts)
	}
}
