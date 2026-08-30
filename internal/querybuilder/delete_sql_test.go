// Exact SQL and bound-parameter contract for the DELETE command (Issue #38):
// DELETE reuses Issue #9's table eligibility, Issue #14's quoting and
// operator atoms, Issue #17's optional WHERE predicate, and Issue #19's
// runnable report unchanged. An absent WHERE is a valid unqualified delete
// over every write-eligible row; a complete predicate qualifies it.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

func deleteCatalog() *schema.Catalog {
	return &schema.Catalog{Version: 38, Objects: []*schema.Object{{
		Name: `order"items`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{
			{Name: `first"name`, DeclaredType: "TEXT"},
			{Name: "score", DeclaredType: "REAL"},
			{Name: "note", DeclaredType: "TEXT"},
		},
	}, {
		Name: "logs", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
		Columns: []schema.Column{{Name: "body", DeclaredType: "TEXT"}},
	}, {
		Name: "reports", Kind: schema.KindView, Rowid: schema.RowidNotApplicable,
		Columns: []schema.Column{{Name: "body", DeclaredType: "TEXT"}},
	}}}
}

func deleteBuilder() QueryBuilder {
	return NewQuery().RefreshSchema(deleteCatalog()).SelectCommand(CommandDelete).SelectTable(`order"items`)
}

func deleteCompleteOn(t *testing.T, q QueryBuilder, op Operator, text string) QueryBuilder {
	t.Helper()
	next, ok := q.StartWhere("score")
	if !ok {
		t.Fatalf("StartWhere(score) rejected")
	}
	draft, ok := next.WhereDraft().ChooseOperator(op)
	if !ok {
		t.Fatalf("ChooseOperator(%v) rejected", op)
	}
	next = next.ApplyWhereDraft(draft)
	if op.TakesValue() {
		draft, ok = draft.SubmitValue(text)
		if !ok {
			t.Fatalf("SubmitValue(%q) rejected", text)
		}
		next = next.ApplyWhereDraft(draft)
	}
	next, ok = next.CommitWhereDraft()
	if !ok {
		t.Fatalf("CommitWhereDraft(%v) rejected", op)
	}
	return next
}

func TestDeleteWithoutWhereIsRunnableUnqualifiedSQL(t *testing.T) {
	for _, name := range []string{`order"items`, "logs"} {
		q := NewQuery().RefreshSchema(deleteCatalog()).SelectCommand(CommandDelete).SelectTable(name)
		if report := q.RunnableReport(); !report.Runnable {
			t.Fatalf("DELETE on %s: RunnableReport() = %+v, want runnable", name, report)
		}
		want := `DELETE FROM ` + quoteIdentifierAtom(name)
		if got := q.DeleteSQL(); got != want {
			t.Errorf("DeleteSQL() = %q, want %q", got, want)
		}
		if got := q.DeleteParams(); got != nil {
			t.Errorf("DeleteParams() = %#v, want nil", got)
		}
	}
}

func TestDeleteSQLAndParamsWithPredicate(t *testing.T) {
	tests := []struct {
		name       string
		op         Operator
		text       string
		wantSQL    string
		wantParams []any
	}{
		{
			name:       "integer value",
			op:         OpEq,
			text:       "7",
			wantSQL:    `DELETE FROM "order""items" WHERE "score" = ?`,
			wantParams: []any{int64(7)},
		},
		{
			name:       "real value",
			op:         OpLt,
			text:       "1.5",
			wantSQL:    `DELETE FROM "order""items" WHERE "score" < ?`,
			wantParams: []any{float64(1.5)},
		},
		{
			name:       "empty text value",
			op:         OpGe,
			text:       "",
			wantSQL:    `DELETE FROM "order""items" WHERE "score" >= ?`,
			wantParams: []any{""},
		},
		{
			name:       "typed NULL stays TEXT",
			op:         OpNotEq,
			text:       "NULL",
			wantSQL:    `DELETE FROM "order""items" WHERE "score" != ?`,
			wantParams: []any{"NULL"},
		},
		{
			name:       "LIKE binds verbatim wildcards",
			op:         OpLike,
			text:       `%a_b%`,
			wantSQL:    `DELETE FROM "order""items" WHERE "score" LIKE ?`,
			wantParams: []any{"%a_b%"},
		},
		{
			name:    "IS NULL binds no parameter",
			op:      OpIsNull,
			wantSQL: `DELETE FROM "order""items" WHERE "score" IS NULL`,
		},
		{
			name:    "IS NOT NULL binds no parameter",
			op:      OpIsNotNull,
			wantSQL: `DELETE FROM "order""items" WHERE "score" IS NOT NULL`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := deleteCompleteOn(t, deleteBuilder(), tt.op, tt.text)
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("RunnableReport() = %+v, want runnable", report)
			}
			if got := q.DeleteSQL(); got != tt.wantSQL {
				t.Errorf("DeleteSQL() = %q, want %q", got, tt.wantSQL)
			}
			if got := q.DeleteParams(); !reflect.DeepEqual(got, tt.wantParams) {
				t.Errorf("DeleteParams() = %#v, want %#v", got, tt.wantParams)
			}
		})
	}
}

func TestDeleteRejectsNonTableIdentities(t *testing.T) {
	tests := []struct {
		name       string
		build      func(*testing.T) QueryBuilder
		wantReason string
	}{
		{
			name: "view selection is ignored",
			build: func(t *testing.T) QueryBuilder {
				q := NewQuery().RefreshSchema(deleteCatalog()).SelectCommand(CommandDelete).SelectTable("reports")
				if _, selected := q.SelectedTable(); selected {
					t.Fatal("view was selected")
				}
				return q
			},
			wantReason: ReasonNoTable,
		},
		{
			name: "unknown system name is ignored",
			build: func(t *testing.T) QueryBuilder {
				q := NewQuery().RefreshSchema(deleteCatalog()).SelectCommand(CommandDelete).SelectTable("sqlite_sequence")
				if _, selected := q.SelectedTable(); selected {
					t.Fatal("unknown name was selected")
				}
				return q
			},
			wantReason: ReasonNoTable,
		},
		{
			name: "stale selection is cleared by refresh",
			build: func(t *testing.T) QueryBuilder {
				q := deleteBuilder()
				q = q.RefreshSchema(&schema.Catalog{Version: 39, Objects: nil})
				if _, selected := q.SelectedTable(); selected {
					t.Fatal("stale selection survived refresh")
				}
				return q
			},
			wantReason: ReasonNoTable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			report := q.RunnableReport()
			if report.Runnable || report.Field != RunFieldTable || report.Reason != tt.wantReason {
				t.Fatalf("RunnableReport() = %+v, want Table/%q", report, tt.wantReason)
			}
			if q.DeleteSQL() != "" || q.DeleteParams() != nil {
				t.Fatal("rejected identity produced an executable request")
			}
		})
	}
}

func TestIncompleteDeletePredicateIsNotRunnable(t *testing.T) {
	tests := []struct {
		name       string
		build      func(*testing.T) QueryBuilder
		wantReason string
	}{
		{
			name: "column chosen but no operator",
			build: func(t *testing.T) QueryBuilder {
				next, ok := deleteBuilder().StartWhere("score")
				if !ok {
					t.Fatal("StartWhere(score) rejected")
				}
				return next
			},
			wantReason: ReasonIncompletePrompt,
		},
		{
			name: "operator chosen but no value",
			build: func(t *testing.T) QueryBuilder {
				next, ok := deleteBuilder().StartWhere("score")
				if !ok {
					t.Fatal("StartWhere(score) rejected")
				}
				draft, ok := next.WhereDraft().ChooseOperator(OpEq)
				if !ok {
					t.Fatal("ChooseOperator(=) rejected")
				}
				return next.ApplyWhereDraft(draft)
			},
			wantReason: ReasonIncompletePrompt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			report := q.RunnableReport()
			if report.Runnable || report.Field != RunFieldWhere || report.Reason != tt.wantReason {
				t.Fatalf("RunnableReport() = %+v, want WHERE/%q", report, tt.wantReason)
			}
			if q.DeleteSQL() != "" || q.DeleteParams() != nil {
				t.Fatal("incomplete predicate produced an executable request")
			}
		})
	}
}
