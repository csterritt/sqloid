package querybuilder

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

func updateCatalog() *schema.Catalog {
	return &schema.Catalog{Version: 37, Objects: []*schema.Object{{
		Name: `order"items`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{
			{Name: `first"name`, DeclaredType: "TEXT", Insertable: true},
			{Name: "score", DeclaredType: "REAL", Insertable: true},
			{Name: "note", DeclaredType: "TEXT", Insertable: true},
			{Name: "literal", DeclaredType: "TEXT", Insertable: true},
			{Name: "generated", DeclaredType: "TEXT", Hidden: true},
		},
	}}}
}

func updateBuilder() QueryBuilder {
	return NewQuery().RefreshSchema(updateCatalog()).SelectCommand(CommandUpdate).SelectTable(`order"items`)
}

func addSetValue(t *testing.T, q QueryBuilder, column, text string) QueryBuilder {
	t.Helper()
	next, ok := q.AcceptSetColumn(column)
	if !ok {
		t.Fatalf("AcceptSetColumn(%q) rejected", column)
	}
	next, ok = next.ChooseSetAssignment(column, SetChoiceValue)
	if !ok {
		t.Fatalf("ChooseSetAssignment(%q, Value) rejected", column)
	}
	next, ok = next.SubmitSetValue(column, text)
	if !ok {
		t.Fatalf("SubmitSetValue(%q, %q) rejected", column, text)
	}
	return next
}

func addSetNull(t *testing.T, q QueryBuilder, column string) QueryBuilder {
	t.Helper()
	next, ok := q.AcceptSetColumn(column)
	if !ok {
		t.Fatalf("AcceptSetColumn(%q) rejected", column)
	}
	next, ok = next.ChooseSetAssignment(column, SetChoiceNull)
	if !ok {
		t.Fatalf("ChooseSetAssignment(%q, NULL) rejected", column)
	}
	return next
}

func TestUpdateSetSelectionIsUniqueOrderedAndSchemaDerived(t *testing.T) {
	q := updateBuilder()
	var ok bool
	if q, ok = q.AcceptSetColumn("score"); !ok {
		t.Fatal("score was rejected")
	}
	before := q
	if next, accepted := q.AcceptSetColumn("score"); accepted || !reflect.DeepEqual(next, before) {
		t.Fatal("duplicate SET selection changed the builder")
	}
	if q, ok = q.AcceptSetColumn(`first"name`); !ok {
		t.Fatal("quoted visible column was rejected")
	}
	if _, ok = q.AcceptSetColumn("generated"); ok {
		t.Fatal("hidden generated column was accepted")
	}
	if _, ok = q.AcceptSetColumn("missing"); ok {
		t.Fatal("unknown column was accepted")
	}
	got := q.SetAssignments()
	if len(got) != 2 || got[0].Column != "score" || got[1].Column != `first"name` {
		t.Fatalf("SET order = %#v, want score then first\"name", got)
	}
}

func TestUpdateSQLAndParams(t *testing.T) {
	tests := []struct {
		name       string
		build      func(*testing.T) QueryBuilder
		wantSQL    string
		wantParams []any
	}{
		{
			name: "all values preserve SET order and universal types",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "1.5")
				return addSetValue(t, q, `first"name`, "NULL")
			},
			wantSQL:    `UPDATE "order""items" SET "score" = ?, "first""name" = ?`,
			wantParams: []any{float64(1.5), "NULL"},
		},
		{
			name: "mixed values and SQL NULL skip NULL parameters",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), `first"name`, "")
				q = addSetNull(t, q, "score")
				return addSetValue(t, q, "note", "42")
			},
			wantSQL:    `UPDATE "order""items" SET "first""name" = ?, "score" = NULL, "note" = ?`,
			wantParams: []any{"", int64(42)},
		},
		{
			name: "all NULL has no parameters",
			build: func(t *testing.T) QueryBuilder {
				q := addSetNull(t, updateBuilder(), "score")
				return addSetNull(t, q, "note")
			},
			wantSQL: `UPDATE "order""items" SET "score" = NULL, "note" = NULL`,
		},
		{
			name: "WHERE value follows SET values",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "7")
				q = addSetNull(t, q, "note")
				return whereCompleteEqOn(t, q, `first"name`, "alice")
			},
			wantSQL:    `UPDATE "order""items" SET "score" = ?, "note" = NULL WHERE "first""name" = ?`,
			wantParams: []any{int64(7), "alice"},
		},
		{
			name: "null WHERE operator adds no parameter",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "7")
				return whereCompleteNullOn(t, q, "note")
			},
			wantSQL:    `UPDATE "order""items" SET "score" = ? WHERE "note" IS NULL`,
			wantParams: []any{int64(7)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("RunnableReport() = %+v", report)
			}
			if got := q.UpdateSQL(); got != tt.wantSQL {
				t.Errorf("UpdateSQL() = %q, want %q", got, tt.wantSQL)
			}
			if got := q.UpdateParams(); !reflect.DeepEqual(got, tt.wantParams) {
				t.Errorf("UpdateParams() = %#v, want %#v", got, tt.wantParams)
			}
		})
	}
}

func TestSubmitSetValueDoesNotMutatePriorSnapshot(t *testing.T) {
	q, ok := updateBuilder().AcceptSetColumn("score")
	if !ok {
		t.Fatal("AcceptSetColumn(score) rejected")
	}
	q, ok = q.ChooseSetAssignment("score", SetChoiceValue)
	if !ok {
		t.Fatal("ChooseSetAssignment(score, Value) rejected")
	}
	prior := q
	next, ok := q.SubmitSetValue("score", "9")
	if !ok {
		t.Fatal("SubmitSetValue(score, 9) rejected")
	}
	if _, submitted := prior.SetAssignments()[0].SubmittedValue(); submitted {
		t.Fatal("SubmitSetValue mutated the prior snapshot")
	}
	if value, submitted := next.SetAssignments()[0].SubmittedValue(); !submitted || value.Int != 9 {
		t.Fatalf("next snapshot value = (%+v, %v)", value, submitted)
	}
}

func TestMalformedUpdateAssignmentsAreNotRunnable(t *testing.T) {
	tests := []struct {
		name string
		sets []SetAssignment
		want string
	}{
		{"stale column", []SetAssignment{{Column: "missing", choice: SetChoiceNull}}, ReasonStaleSetColumn},
		{"invalid choice", []SetAssignment{{Column: "score", choice: SetChoice(99)}}, "complete the choice for column score"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := updateBuilder().WithSetAssignments(tt.sets)
			report := q.RunnableReport()
			if report.Runnable || report.Field != RunFieldSetAssignments || report.Reason != tt.want {
				t.Fatalf("RunnableReport() = %+v, want SET reason %q", report, tt.want)
			}
			if q.UpdateSQL() != "" || q.UpdateParams() != nil {
				t.Fatal("malformed assignment produced an executable request")
			}
		})
	}
}

func TestIncompleteUpdateProducesNoExecutableRequest(t *testing.T) {
	q, ok := updateBuilder().AcceptSetColumn("score")
	if !ok {
		t.Fatal("AcceptSetColumn(score) rejected")
	}
	q, ok = q.ChooseSetAssignment("score", SetChoiceValue)
	if !ok {
		t.Fatal("ChooseSetAssignment(score, Value) rejected")
	}
	if got := q.UpdateSQL(); got != "" {
		t.Errorf("UpdateSQL() = %q, want empty", got)
	}
	if got := q.UpdateParams(); got != nil {
		t.Errorf("UpdateParams() = %#v, want nil", got)
	}
}

func whereCompleteEqOn(t *testing.T, q QueryBuilder, column, text string) QueryBuilder {
	t.Helper()
	next, ok := q.StartWhere(column)
	if !ok {
		t.Fatalf("StartWhere(%q) rejected", column)
	}
	draft, ok := next.WhereDraft().ChooseOperator(OpEq)
	if !ok {
		t.Fatal("ChooseOperator(=) rejected")
	}
	next = next.ApplyWhereDraft(draft)
	draft, ok = draft.SubmitValue(text)
	if !ok {
		t.Fatal("SubmitValue rejected")
	}
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		t.Fatal("CommitWhereDraft rejected")
	}
	return next
}

func ExampleQueryBuilder_UpdateSQL() {
	q := updateBuilder()
	q = mustSetExample(q, "score", SetChoiceValue, "2.5")
	q = mustSetExample(q, `first"name`, SetChoiceValue, "")
	q = mustSetExample(q, "note", SetChoiceValue, "NULL")
	_, duplicate := q.AcceptSetColumn("score")
	q = mustSetExample(q, "literal", SetChoiceNull, "")
	next, _ := q.StartWhere("score")
	draft, _ := next.WhereDraft().ChooseOperator(OpEq)
	draft, _ = draft.SubmitValue("7")
	q, _ = next.ApplyWhereDraft(draft).CommitWhereDraft()

	fmt.Println("duplicate accepted:", duplicate)
	fmt.Println(q.UpdateSQL())
	fmt.Println("runnable:", q.RunnableReport().Runnable)
	for _, param := range q.UpdateParams() {
		fmt.Printf("%T=%v\n", param, param)
	}

	// Output:
	// duplicate accepted: false
	// UPDATE "order""items" SET "score" = ?, "first""name" = ?, "note" = ?, "literal" = NULL WHERE "score" = ?
	// runnable: true
	// float64=2.5
	// string=
	// string=NULL
	// int64=7
}

func mustSetExample(q QueryBuilder, column string, choice SetChoice, text string) QueryBuilder {
	next, ok := q.AcceptSetColumn(column)
	if !ok {
		return q
	}
	next, ok = next.ChooseSetAssignment(column, choice)
	if !ok {
		return q
	}
	if choice == SetChoiceValue {
		next, _ = next.SubmitSetValue(column, text)
	}
	return next
}

func whereCompleteNullOn(t *testing.T, q QueryBuilder, column string) QueryBuilder {
	t.Helper()
	next, ok := q.StartWhere(column)
	if !ok {
		t.Fatalf("StartWhere(%q) rejected", column)
	}
	draft, ok := next.WhereDraft().ChooseOperator(OpIsNull)
	if !ok {
		t.Fatal("ChooseOperator(IS NULL) rejected")
	}
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		t.Fatal("CommitWhereDraft rejected")
	}
	return next
}
