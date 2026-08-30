// Exact rendered-statement and estimate contracts for the destructive UPDATE
// and DELETE preparation (Issue #40): one canonical standalone rendered write
// SQL per command, produced only through Issue #14's shared identifier and
// typed literal atoms, plus the independent matching-target estimate request
// built exactly from the quoted target and the identical shared WHERE
// predicate. The estimate binds only WHERE values in predicate order and
// never carries UPDATE SET fragments or SET parameters.

package querybuilder

import (
	"reflect"
	"strings"
	"testing"
)

func TestUpdateRenderedSQLInlinesSharedLiterals(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*testing.T) QueryBuilder
		wantSQL string
	}{
		{
			name: "mixed value and NULL assignments with qualified WHERE",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "7")
				q = addSetNull(t, q, "note")
				return whereCompleteEqOn(t, q, `first"name`, "alice")
			},
			wantSQL: `UPDATE "order""items" SET "score" = 7, "note" = NULL WHERE "first""name" = 'alice'`,
		},
		{
			name: "typed TEXT NULL stays a quoted text literal",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), `first"name`, "NULL")
				return whereCompleteEqOn(t, q, "note", "42")
			},
			wantSQL: `UPDATE "order""items" SET "first""name" = 'NULL' WHERE "note" = 42`,
		},
		{
			name: "real and integer literals render canonically",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "1.5")
				return addSetValue(t, q, "literal", "x'y")
			},
			wantSQL: `UPDATE "order""items" SET "score" = 1.5, "literal" = 'x''y'`,
		},
		{
			name: "empty text renders a quoted empty literal",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), `first"name`, "")
				return addSetNull(t, q, "score")
			},
			wantSQL: `UPDATE "order""items" SET "first""name" = '', "score" = NULL`,
		},
		{
			name: "unqualified update over every row",
			build: func(t *testing.T) QueryBuilder {
				return addSetValue(t, updateBuilder(), "score", "0")
			},
			wantSQL: `UPDATE "order""items" SET "score" = 0`,
		},
		{
			name: "null-operator WHERE renders the keyword with no literal",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "7")
				return whereCompleteNullOn(t, q, "note")
			},
			wantSQL: `UPDATE "order""items" SET "score" = 7 WHERE "note" IS NULL`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("RunnableReport() = %+v", report)
			}
			if got := q.UpdateRenderedSQL(); got != tt.wantSQL {
				t.Errorf("UpdateRenderedSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestUpdateRenderedSQLRequiresRunnableUpdate(t *testing.T) {
	if got := NewQuery().UpdateRenderedSQL(); got != "" {
		t.Errorf("UpdateRenderedSQL() on empty builder = %q, want empty", got)
	}
	if got := deleteBuilder().UpdateRenderedSQL(); got != "" {
		t.Errorf("UpdateRenderedSQL() under DELETE = %q, want empty", got)
	}
}

func TestDeleteRenderedSQLInlinesSharedLiterals(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*testing.T) QueryBuilder
		wantSQL string
	}{
		{
			name:    "unqualified delete over every row",
			build:   func(t *testing.T) QueryBuilder { return deleteBuilder() },
			wantSQL: `DELETE FROM "order""items"`,
		},
		{
			name: "qualified delete with a bound value",
			build: func(t *testing.T) QueryBuilder {
				return deleteCompleteOn(t, deleteBuilder(), OpGt, "1.5")
			},
			wantSQL: `DELETE FROM "order""items" WHERE "score" > 1.5`,
		},
		{
			name: "null-operator predicate renders the keyword",
			build: func(t *testing.T) QueryBuilder {
				return deleteCompleteOn(t, deleteBuilder(), OpIsNotNull, "")
			},
			wantSQL: `DELETE FROM "order""items" WHERE "score" IS NOT NULL`,
		},
		{
			name: "typed text NULL stays a quoted text literal",
			build: func(t *testing.T) QueryBuilder {
				return deleteCompleteOn(t, deleteBuilder(), OpEq, "NULL")
			},
			wantSQL: `DELETE FROM "order""items" WHERE "score" = 'NULL'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("RunnableReport() = %+v", report)
			}
			if got := q.DeleteRenderedSQL(); got != tt.wantSQL {
				t.Errorf("DeleteRenderedSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestDeleteRenderedSQLRequiresRunnableDelete(t *testing.T) {
	if got := NewQuery().DeleteRenderedSQL(); got != "" {
		t.Errorf("DeleteRenderedSQL() on empty builder = %q, want empty", got)
	}
	if got := updateBuilder().DeleteRenderedSQL(); got != "" {
		t.Errorf("DeleteRenderedSQL() under UPDATE = %q, want empty", got)
	}
}

func TestEstimateSQLIsExactlyCountOverTargetAndPredicate(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*testing.T) QueryBuilder
		wantSQL string
	}{
		{
			name: "unqualified update estimate",
			build: func(t *testing.T) QueryBuilder {
				return addSetValue(t, updateBuilder(), "score", "7")
			},
			wantSQL: `SELECT COUNT(*) FROM "order""items"`,
		},
		{
			name: "qualified update estimate reuses the identical predicate",
			build: func(t *testing.T) QueryBuilder {
				q := addSetValue(t, updateBuilder(), "score", "7")
				return whereCompleteEqOn(t, q, `first"name`, "alice")
			},
			wantSQL: `SELECT COUNT(*) FROM "order""items" WHERE "first""name" = ?`,
		},
		{
			name:    "unqualified delete estimate",
			build:   func(t *testing.T) QueryBuilder { return deleteBuilder() },
			wantSQL: `SELECT COUNT(*) FROM "order""items"`,
		},
		{
			name: "qualified delete estimate reuses the identical predicate",
			build: func(t *testing.T) QueryBuilder {
				return deleteCompleteOn(t, deleteBuilder(), OpGt, "1.5")
			},
			wantSQL: `SELECT COUNT(*) FROM "order""items" WHERE "score" > ?`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build(t)
			if report := q.RunnableReport(); !report.Runnable {
				t.Fatalf("RunnableReport() = %+v", report)
			}
			got := q.EstimateSQL()
			if got != tt.wantSQL {
				t.Errorf("EstimateSQL() = %q, want %q", got, tt.wantSQL)
			}
			if strings.Contains(got, "SET") {
				t.Errorf("EstimateSQL() = %q carries an UPDATE SET fragment", got)
			}
		})
	}
}

func TestEstimateSQLRequiresDestructiveRunnableState(t *testing.T) {
	if got := NewQuery().EstimateSQL(); got != "" {
		t.Errorf("EstimateSQL() on empty builder = %q, want empty", got)
	}
	selectQ := NewQuery().RefreshSchema(deleteCatalog()).SelectCommand(CommandSelect).SelectTable(`order"items`)
	if got := selectQ.EstimateSQL(); got != "" {
		t.Errorf("EstimateSQL() under SELECT = %q, want empty", got)
	}
	if got := updateBuilder().EstimateSQL(); got != "" {
		t.Errorf("EstimateSQL() on non-runnable UPDATE = %q, want empty", got)
	}
	nonRunnable := deleteBuilder()
	nonRunnable, _ = nonRunnable.StartWhere("score") // open draft: incomplete prompt
	if got := nonRunnable.EstimateSQL(); got != "" {
		t.Errorf("EstimateSQL() on non-runnable DELETE = %q, want empty", got)
	}
}

func TestEstimateParamsBindOnlyWhereValues(t *testing.T) {
	t.Run("set values never enter the estimate parameters", func(t *testing.T) {
		q := addSetValue(t, updateBuilder(), "score", "7")
		q = addSetValue(t, q, "literal", "alice")
		q = whereCompleteEqOn(t, q, "note", "needle")
		want := []any{"needle"}
		got := q.EstimateParams()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("EstimateParams() = %#v, want %#v", got, want)
		}
		if !reflect.DeepEqual(got, q.WhereParams()) {
			t.Errorf("EstimateParams() = %#v differs from WhereParams() %#v", got, q.WhereParams())
		}
	})
	t.Run("null-operator where binds nothing", func(t *testing.T) {
		q := addSetValue(t, updateBuilder(), "score", "7")
		q = whereCompleteNullOn(t, q, "note")
		if got := q.EstimateParams(); got != nil {
			t.Errorf("EstimateParams() with IS NULL = %#v, want nil", got)
		}
	})
	t.Run("absent where binds nothing", func(t *testing.T) {
		q := addSetValue(t, updateBuilder(), "score", "7")
		if got := q.EstimateParams(); got != nil {
			t.Errorf("EstimateParams() without WHERE = %#v, want nil", got)
		}
	})
	t.Run("delete where values pass through in predicate order", func(t *testing.T) {
		q := deleteCompleteOn(t, deleteBuilder(), OpLt, "9")
		want := []any{int64(9)}
		if got := q.EstimateParams(); !reflect.DeepEqual(got, want) {
			t.Errorf("EstimateParams() = %#v, want %#v", got, want)
		}
	})
}
