// UI-independent table tests for the pre-execution revalidation transition
// (Issue #21, Task 1): a changed schema version refreshes the selected object
// and columns through the refreshed catalog, then revalidates object
// identity, command eligibility, every referenced identifier, INSERT column
// insertability, and rowid capability/shadowing. Only state transitively
// dependent on an invalidated prerequisite is cleared — unrelated completed
// builder state is preserved — and the authoritative runnable report
// identifies the first specific invalid field and reason deterministically.
// States are built only through QueryBuilder transitions plus the
// forward-compatible write-state seam; no UI, popup, or rendering is involved.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// revalidateCatalog returns the refreshed starting snapshot: an ordinary
// rowid table with three visible insertable columns.
func revalidateCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 40,
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
		},
	}
}

// revalidateMutation applies one transformation to the items object of a
// fresh revalidateCatalog and bumps the version, so every changed-schema
// fixture is visibly derived from the same base shape.
func revalidateMutation(mutate func(o *schema.Object)) *schema.Catalog {
	c := revalidateCatalog()
	c.Version = 41
	mutate(c.Objects[0])
	return c
}

// sel builds a SELECT over items with the given builder shaping applied.
func revalidateSelect(shaping func(QueryBuilder) QueryBuilder) QueryBuilder {
	return shaping(NewQuery().RefreshSchema(revalidateCatalog()).
		SelectCommand(CommandSelect).SelectTable("items"))
}

// addProjection commits one plain (column, AggregateValue) projection entry
// through the named-identity transition; panics keep fixtures exact.
func addProjection(q QueryBuilder, column string) QueryBuilder {
	out := q.CompleteProjectionAggregate(column, AggregateValue)
	if !out.ReopenColumns {
		panic("CompleteProjectionAggregate(" + column + ") did not commit")
	}
	return out.Builder
}

// completeWhereOn commits a complete WHERE predicate on name through the
// guided transitions.
func completeWhereOn(q QueryBuilder, name string) QueryBuilder {
	begin, ok := q.StartWhere(name)
	if !ok {
		panic("StartWhere rejected " + name)
	}
	draft, _ := begin.WhereDraft().ChooseOperator(OpEq)
	done, ok := draft.SubmitValue("5")
	if !ok {
		panic("SubmitValue rejected on " + name)
	}
	next, ok := begin.ApplyWhereDraft(done).CommitWhereDraft()
	if !ok {
		panic("CommitWhereDraft rejected on " + name)
	}
	return next
}

func TestRevalidateIdenticalCatalogPreservesEverything(t *testing.T) {
	refreshed := revalidateMutation(func(o *schema.Object) {}) // same shape, new version
	q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
		return completeWhereOn(addProjection(b, "id"), "id").SetLimitInput("5")
	})

	next, rep := q.Revalidate(refreshed)
	if rep.Cleared {
		t.Error("revalidation against an identical catalog cleared builder state")
	}
	if _, limitOK := next.LimitValue(); !limitOK {
		t.Error("Limit was not preserved")
	}
	if !next.HasWhere() {
		t.Error("WHERE was not preserved")
	}
	if len(next.ProjectionEntries()) != 1 {
		t.Errorf("projection entry count = %d, want 1", len(next.ProjectionEntries()))
	}
	if !rep.Report.Runnable {
		t.Errorf("post-repair report = %+v, want runnable", rep.Report)
	}
	if gen := next.DownstreamGeneration(); gen != q.DownstreamGeneration() {
		t.Errorf("downstream generation %d, want unchanged %d", gen, q.DownstreamGeneration())
	}
}

func TestRevalidateDroppedTableClearsOnlyDependentState(t *testing.T) {
	refreshed := &schema.Catalog{Version: 41, Objects: []*schema.Object{{
		Name: "other", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns:         []schema.Column{{Name: "a", Insertable: true}},
		InsertableCount: 1,
	}}}
	q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
		return completeWhereOn(addProjection(b, "id"), "id").SetLimitInput("5")
	})

	next, rep := q.Revalidate(refreshed)
	if !rep.Cleared {
		t.Error("dropped table did not report clearing")
	}
	if name, set := next.SelectedTable(); set || name != "" {
		t.Errorf("selected table = %q,%v, want cleared", name, set)
	}
	if !next.ProjectionEmpty() || next.HasWhere() {
		t.Error("downstream projection/WHERE survived the dropped table")
	}
	if _, limitOK := next.LimitValue(); limitOK {
		t.Error("limit survived a dropped table, want discarded with the table")
	}
	if cmd := next.Command(); cmd != CommandSelect {
		t.Errorf("command = %v, want preserved %v", cmd, CommandSelect)
	}
	if rep.Report.Runnable || rep.Report.Field != RunFieldTable || rep.Report.Reason != ReasonNoTable {
		t.Errorf("report = %+v, want {RunFieldTable, %q}", rep.Report, ReasonNoTable)
	}
	if gen := next.DownstreamGeneration(); gen <= q.DownstreamGeneration() {
		t.Errorf("downstream generation = %d, want bumped past %d", gen, q.DownstreamGeneration())
	}
}

func TestRevalidateEligibilityChangeClearsWriteCommandTable(t *testing.T) {
	refreshed := revalidateMutation(func(o *schema.Object) {
		o.Kind = schema.KindView
		o.WriteEligible = false
	})
	q := NewQuery().RefreshSchema(revalidateCatalog()).SelectCommand(CommandUpdate).SelectTable("items")
	var ok bool
	if q, ok = q.AcceptSetColumn("id"); !ok {
		t.Fatal("AcceptSetColumn(id) rejected")
	}
	if q, ok = q.ChooseSetAssignment("id", SetChoiceNull); !ok {
		t.Fatal("ChooseSetAssignment rejected")
	}

	next, rep := q.Revalidate(refreshed)
	if !rep.Cleared {
		t.Error("eligibility change did not report clearing")
	}
	if name, set := next.SelectedTable(); set || name != "" {
		t.Errorf("selected table = %q,%v, want cleared", name, set)
	}
	if len(next.SetAssignments()) != 0 {
		t.Errorf("SET assignments survived the eligibility change: %v", next.SetAssignments())
	}
	if rep.Report.Runnable || rep.Report.Field != RunFieldTable || rep.Report.Reason != ReasonNoTable {
		t.Errorf("report = %+v, want {RunFieldTable, %q}", rep.Report, ReasonNoTable)
	}
}

func TestRevalidateIdentifierInvalidationCases(t *testing.T) {
	t.Run("dropped projection column removes only that entry", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			o.Columns = o.Columns[:2] // drop score
			o.InsertableCount = 2
		})
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return addProjection(addProjection(b, "id"), "score").SetLimitInput("5")
		}).SetLimitInput("5")

		next, rep := q.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("dropped projected column did not report clearing")
		}
		entries := next.ProjectionEntries()
		if len(entries) != 1 || entries[0].Column != "id" {
			t.Errorf("projection = %+v, want exactly the surviving id entry", entries)
		}
		if _, limitOK := next.LimitValue(); !limitOK {
			t.Error("unrelated Limit was cleared by a projection-only invalidation")
		}
		if !rep.Report.Runnable {
			t.Errorf("report = %+v, want runnable after dependent-only repair", rep.Report)
		}
	})

	t.Run("emptied projection reports the first invalid reason", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			o.Columns = o.Columns[:2] // drop score
			o.InsertableCount = 2
		})
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return addProjection(b, "score")
		})

		next, rep := q.Revalidate(refreshed)
		if !next.ProjectionEmpty() {
			t.Error("projection was not emptied by the removal of its only column")
		}
		if rep.Report.Runnable || rep.Report.Field != RunFieldProjection || rep.Report.Reason != ReasonNoProjection {
			t.Errorf("report = %+v, want {RunFieldProjection, %q}", rep.Report, ReasonNoProjection)
		}
	})

	t.Run("dropped where column clears only the predicate", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			o.Columns = o.Columns[:2] // drop score
			o.InsertableCount = 2
		})
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			g, ok := addProjection(b, "id").AcceptGroupColumn("id")
			if !ok {
				panic("AcceptGroupColumn(id) rejected")
			}
			return g
		})
		q = completeWhereOn(q, "score").SetLimitInput("7")

		next, rep := q.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("dropped WHERE column did not report clearing")
		}
		if next.HasWhere() {
			t.Error("committed WHERE on a vanished column survived revalidation")
		}
		if got := next.GroupByEntries(); len(got) != 1 || got[0] != "id" {
			t.Errorf("GROUP BY = %v, want the unrelated [id] preserved", got)
		}
		if _, limitOK := next.LimitValue(); !limitOK {
			t.Error("unrelated Limit was cleared by a WHERE-only invalidation")
		}
		if !rep.Report.Runnable {
			t.Errorf("report = %+v, want runnable after repair", rep.Report)
		}
	})

	t.Run("dropped order column clears only the ordering", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			o.Columns = o.Columns[:2] // drop score
			o.InsertableCount = 2
		})
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return completeWhereOn(addProjection(b, "id"), "id")
		})
		key := "order-column:score"
		ordered, ok := q.AcceptOrderBy(key)
		if !ok {
			t.Fatalf("AcceptOrderBy(%q) rejected", key)
		}

		next, rep := ordered.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("dropped ORDER BY column did not report clearing")
		}
		if _, _, ok := next.OrderBySelection(); ok {
			t.Error("ORDER BY on a vanished column survived revalidation")
		}
		if !next.HasWhere() {
			t.Error("unrelated WHERE was cleared by an ORDER BY-only invalidation")
		}
		if !rep.Report.Runnable {
			t.Errorf("report = %+v, want runnable after repair", rep.Report)
		}
	})

	t.Run("first invalid reason is deterministic in visual order", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			o.Columns = o.Columns[:2] // drop score
			o.InsertableCount = 2
		})
		// Projection empties AND the WHERE predicate clears from one dropped
		// column; the visual-order report must name the projection first.
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return addProjection(b, "score")
		})
		q = completeWhereOn(q, "score")

		_, rep := q.Revalidate(refreshed)
		if rep.Report.Runnable || rep.Report.Field != RunFieldProjection || rep.Report.Reason != ReasonNoProjection {
			t.Errorf("report = %+v, want {RunFieldProjection, %q} before WHERE", rep.Report, ReasonNoProjection)
		}
	})
}

func TestRevalidateInsertabilityInvalidation(t *testing.T) {
	t.Run("hidden column clears only its INSERT prompt", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			for i := range o.Columns {
				if o.Columns[i].Name == "score" {
					o.Columns[i].Hidden = true
					o.Columns[i].Insertable = false
				}
			}
			o.InsertableCount = 2
		})
		q := NewQuery().RefreshSchema(revalidateCatalog()).SelectCommand(CommandInsert).SelectTable("items").
			BeginInsertPrompts()
		q, ok := q.ChooseInsertColumn("name", InsertChoiceOmit)
		if !ok {
			t.Fatal("ChooseInsertColumn(name) rejected")
		}
		if q, ok = q.ChooseInsertColumn("id", InsertChoiceOmit); !ok {
			t.Fatal("ChooseInsertColumn(id) rejected")
		}
		if q, ok = q.ChooseInsertColumn("score", InsertChoiceOmit); !ok {
			t.Fatal("ChooseInsertColumn(score) rejected")
		}

		next, rep := q.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("hidden insertability change did not report clearing")
		}
		for _, c := range next.InsertColumns() {
			if c.Column == "score" {
				t.Error("prompt for the now-hidden column survived revalidation")
			}
		}
		found := false
		for _, c := range next.InsertColumns() {
			if c.Column == "id" {
				found = true
			}
		}
		if !found {
			t.Error("unrelated prompt for id was cleared")
		}
		if !rep.Report.Runnable {
			t.Errorf("report = %+v, want runnable after repair", rep.Report)
		}
	})

	t.Run("zero insertable columns reports the exact block", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) {
			for i := range o.Columns {
				o.Columns[i].Hidden = true
				o.Columns[i].Insertable = false
			}
			o.InsertableCount = 0
		})
		q := NewQuery().RefreshSchema(revalidateCatalog()).SelectCommand(CommandInsert).SelectTable("items").
			BeginInsertPrompts()

		next, rep := q.Revalidate(refreshed)
		if len(next.InsertColumns()) != 0 {
			t.Errorf("prompts survived the zero-insertable refresh: %v", next.InsertColumns())
		}
		if rep.Report.Runnable || rep.Report.Field != RunFieldInsertColumns || rep.Report.Reason != ReasonNoInsertableColumns {
			t.Errorf("report = %+v, want {RunFieldInsertColumns, %q}", rep.Report, ReasonNoInsertableColumns)
		}
	})
}

func TestRevalidateRowidPropertyInvalidation(t *testing.T) {
	t.Run("WITHOUT ROWID change clears the committed ordering", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) { o.Rowid = schema.RowidWithout })
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return completeWhereOn(addProjection(b, "id"), "id")
		})
		ordered, ok := q.AcceptOrderBy("order-column:id")
		if !ok {
			t.Fatal("AcceptOrderBy(id) rejected")
		}

		next, rep := ordered.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("rowid capability change did not report clearing")
		}
		if _, _, ok := next.OrderBySelection(); ok {
			t.Error("committed ORDER BY survived a rowid capability change")
		}
		if !next.HasWhere() {
			t.Error("unrelated WHERE was cleared by a rowid-only invalidation")
		}
		if entries := next.ProjectionEntries(); len(entries) != 1 {
			t.Errorf("unrelated projection was cleared: %+v", entries)
		}
		if !rep.Report.Runnable {
			t.Errorf("report = %+v, want runnable after repair", rep.Report)
		}
	})

	t.Run("rowid shadowing change clears the committed ordering", func(t *testing.T) {
		refreshed := revalidateMutation(func(o *schema.Object) { o.RowidShadowed = true })
		q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
			return addProjection(b, "id")
		})
		ordered, ok := q.AcceptOrderBy("order-column:id")
		if !ok {
			t.Fatal("AcceptOrderBy(id) rejected")
		}

		next, rep := ordered.Revalidate(refreshed)
		if !rep.Cleared {
			t.Error("rowid shadowing change did not report clearing")
		}
		if _, _, ok := next.OrderBySelection(); ok {
			t.Error("committed ORDER BY survived a declared-rowid shadowing change")
		}
	})
}

func TestRevalidateNilCatalogIsNoOp(t *testing.T) {
	q := revalidateSelect(func(b QueryBuilder) QueryBuilder {
		return addProjection(b, "id").SetLimitInput("5")
	})
	next, rep := q.Revalidate(nil)
	if rep.Cleared {
		t.Errorf("nil catalog produced %+v, want an unchanged no-op report", rep)
	}
	if _, ok := next.LimitValue(); !ok {
		t.Error("nil catalog cleared unrelated Limit state")
	}
}
