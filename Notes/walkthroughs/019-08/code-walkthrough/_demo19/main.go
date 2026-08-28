// Runnable demonstration for the Issue #19 walkthrough: it drives the real
// QueryBuilder transitions to produce the authoritative runnable reports for
// every SELECT, UPDATE, DELETE, and INSERT prerequisite and common gate,
// exercises the whole-value clearing transitions (mirroring both scripted
// Backspace/Delete paths), and shows the resulting validity. It exercises the
// forward-compatible UPDATE/INSERT Value seams without claiming those
// end-to-end write flows are complete.
package main

import (
	"fmt"

	"github.com/chris/sqloid/internal/querybuilder"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func catalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 19,
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
				Name: "blobs_only", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{{Name: "data", DeclaredType: "BLOB", Hidden: true}},
			},
			{
				Name: "vw", Kind: schema.KindView, Rowid: schema.RowidNotApplicable,
				Columns: []schema.Column{{Name: "line"}},
			},
		},
	}
}

// dropsScore returns a refreshed snapshot whose items table lost the score
// column, forcing stale committed identifiers.
func dropsScore() *schema.Catalog {
	c := catalog()
	kept := []*schema.Object{}
	for _, o := range c.Objects {
		if o.Name == "items" {
			o = &schema.Object{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "id", DeclaredType: "INTEGER", Insertable: true}},
				InsertableCount: 1,
			}
		}
		kept = append(kept, o)
	}
	return &schema.Catalog{Version: 20, Objects: kept}
}

func report(name string, q querybuilder.QueryBuilder) {
	r := q.RunnableReport()
	if r.Runnable {
		fmt.Printf("%-46s RUNNABLE\n", name)
		return
	}
	fmt.Printf("%-46s INVALID [%s] %s\n", name, r.Field, r.Reason)
}

func whereEq(q querybuilder.QueryBuilder, text string) querybuilder.QueryBuilder {
	next, ok := q.StartWhere("name")
	if !ok {
		panic("setup StartWhere")
	}
	draft, ok := next.WhereDraft().ChooseOperator(qb.OpEq)
	if !ok {
		panic("setup ChooseOperator")
	}
	next = next.ApplyWhereDraft(draft)
	draft, _ = draft.SubmitValue(text)
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		panic("setup CommitWhereDraft")
	}
	return next
}

func setSubmitted(q querybuilder.QueryBuilder, col, text string) querybuilder.QueryBuilder {
	next, ok := q.AcceptSetColumn(col)
	if !ok {
		panic("setup AcceptSetColumn")
	}
	next, ok = next.ChooseSetAssignment(col, qb.SetChoiceValue)
	if !ok {
		panic("setup ChooseSetAssignment")
	}
	next, ok = next.SubmitSetValue(col, text)
	if !ok {
		panic("setup SubmitSetValue")
	}
	return next
}

func selectBase() querybuilder.QueryBuilder {
	return querybuilder.NewQuery().RefreshSchema(catalog()).
		SelectCommand(querybuilder.CommandSelect).SelectTable("items")
}

func wildcard(q querybuilder.QueryBuilder) querybuilder.QueryBuilder {
	return q.AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionWildcard}).Builder
}

func plainName(q querybuilder.QueryBuilder) querybuilder.QueryBuilder {
	out := q.AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionColumn, Column: "name"})
	return out.Builder.CompleteProjectionAggregate("name", querybuilder.AggregateValue).Builder
}

func main() {
	fmt.Println("== 1. SELECT runnable reports in visual order ==")
	report("missing command", querybuilder.NewQuery())
	report("no table", querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandSelect))
	report("empty projection", selectBase())
	draft, _ := selectBase().StartWhere("name")
	report("open WHERE draft (with empty projection)", draft)
	report("multi-failure: projection+draft+limit -> first visual only", draft.SetLimitInput("abc"))
	agg := selectBase()
	out := agg.AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionColumn, Column: "name"})
	out = out.Builder.CompleteProjectionAggregate("name", querybuilder.AggregateValue)
	out = out.Builder.AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionColumn, Column: "score"})
	out = out.Builder.CompleteProjectionAggregate("score", querybuilder.AggMin)
	report("mixed agg/nonagg without GROUP BY", out.Builder)
	report("wildcard + GROUP BY", func() querybuilder.QueryBuilder {
		next, _ := wildcard(selectBase()).AcceptGroupColumn("name")
		return next
	}())
	report("stale grouped column after refresh", func() querybuilder.QueryBuilder {
		next, _ := plainName(selectBase()).AcceptGroupColumn("name")
		return next.RefreshSchema(dropsScore())
	}())
	report("stale ORDER BY after refresh", func() querybuilder.QueryBuilder {
		next, _ := plainName(selectBase()).AcceptOrderBy("order-column:name")
		return next.RefreshSchema(dropsScore())
	}())
	report("invalid Limit abc", wildcard(selectBase()).SetLimitInput("abc"))
	report("zero Limit", wildcard(selectBase()).SetLimitInput("0"))
	report("empty Limit (unbounded) + submitted empty TEXT WHERE", whereEq(wildcard(selectBase()).SetLimitInput(""), ""))
	report("fully valid SELECT", whereEq(wildcard(selectBase()).SetLimitInput("10"), "x"))

	fmt.Println()
	fmt.Println("== 2. UPDATE runnable reports ==")
	report("no SET assignments", func() querybuilder.QueryBuilder {
		return querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items")
	}())
	dup, _ := querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items").AcceptSetColumn("name")
	dup, ok := dup.AcceptSetColumn("name")
	if !ok {
		panic("duplicate accept")
	}
	dup, ok = dup.ChooseSetAssignment("name", qb.SetChoiceValue)
	if !ok {
		panic("choose dup")
	}
	dup, ok = dup.SubmitSetValue("name", "x")
	if !ok {
		panic("submit dup")
	}
	report("duplicate SET columns (first matching completed)", dup)
	inc, _ := querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items").AcceptSetColumn("name")
	report("incomplete Value/NULL choice", inc)
	unsub, _ := inc.ChooseSetAssignment("name", qb.SetChoiceValue)
	report("unsubmitted Value entry", unsub)
	nullc, _ := unsub.ChooseSetAssignment("name", qb.SetChoiceNull)
	report("SQL-NULL choice complete", nullc)
	typed, ok := nullc.ChooseSetAssignment("name", qb.SetChoiceValue)
	if !ok {
		panic("choose typed")
	}
	typed, _ = typed.SubmitSetValue("name", "NULL")
	v, _ := typed.SetAssignments()[0].SubmittedValue()
	report("typed TEXT NULL submission (Value choice)", typed)
	fmt.Printf("%-46s bound kind=%v text=%q (not SQL NULL)\n", "", v.Kind, v.Text)
	dw, _ := setSubmitted(querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items"), "name", "x").StartWhere("name")
	report("open WHERE draft behind complete SET", dw)
	report("valid UPDATE", whereEq(setSubmitted(querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items"), "name", "x"), "y"))

	fmt.Println()
	fmt.Println("== 3. DELETE runnable reports ==")
	del := querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandDelete).SelectTable("items")
	report("eligible table, absent WHERE", del)
	report("complete WHERE", whereEq(del, "x"))
	dd, _ := del.StartWhere("name")
	report("incomplete WHERE draft", dd)

	fmt.Println()
	fmt.Println("== 4. INSERT runnable reports ==")
	insert := querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandInsert).SelectTable("items")
	report("zero insertable columns (blobs_only)", querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandInsert).SelectTable("blobs_only"))
	report("view is never an INSERT target", querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandInsert).SelectTable("vw"))
	report("prompts not begun -> first insertable incomplete", insert)
	begun := insert.BeginInsertPrompts()
	report("begun, all incomplete (id first)", begun)
	c1, _ := begun.ChooseInsertColumn("id", qb.InsertChoiceNull)
	report("id NULL; name still incomplete", c1)
	c2, _ := c1.ChooseInsertColumn("name", qb.InsertChoiceValue)
	report("name Value unsubmitted", c2)
	c2, _ = c2.SubmitInsertValue("name", "")
	report("name Value = submitted empty TEXT", c2)
	c3, _ := c2.ChooseInsertColumn("score", qb.InsertChoiceOmit)
	report("all prompts complete", c3)
	allOmit := begun
	next := allOmit
	for _, c := range begun.InsertColumns() {
		var ok bool
		next, ok = next.ChooseInsertColumn(c.Column, qb.InsertChoiceOmit)
		if !ok {
			panic("omit")
		}
	}
	report("valid all-omit state (DEFAULT VALUES later)", next)

	fmt.Println()
	fmt.Println("== 5. whole-value clearing (Backspace and Delete share one transition) ==")
	completed := whereEq(wildcard(selectBase()).SetLimitInput("7"), "x")
	cleared := completed.ClearWhereValue()
	fmt.Printf("WHERE cleared: committed=%v drafting=%v -> report [%s] %s\n",
		cleared.HasWhere(), cleared.WhereDrafting(), func() qb.RunField {
			return cleared.RunnableReport().Field
		}(), cleared.RunnableReport().Reason)
	d := cleared.WhereDraft()
	col, _ := d.Column()
	op, _ := d.ChosenOperator()
	token, _ := op.SQLToken()
	fmt.Printf("  preserved: column=%q operator=%s; submission gone=%v\n", col.Name, token, func() bool {
		_, ok := d.SubmittedValue()
		return !ok
	}())
	absent := wildcard(selectBase())
	before := absent.RunnableReport()
	fmt.Printf("WHERE absent no-op: unchanged=%v\n", absent.ClearWhereValue().RunnableReport() == before)
	lim := wildcard(selectBase()).SetLimitInput("9")
	limCleared := lim.ClearLimitValue()
	_, has := limCleared.LimitValue()
	fmt.Printf("Limit '9' cleared: input=%q value accepted=%v runnable=%v\n", limCleared.LimitInput(), has, limCleared.RunnableReport().Runnable)
	badLim := wildcard(selectBase()).SetLimitInput("abc")
	badCleared := badLim.ClearLimitValue()
	fmt.Printf("Limit 'abc' cleared: input=%q runnable=%v\n", badCleared.LimitInput(), badCleared.RunnableReport().Runnable)
	fmt.Printf("empty Limit no-op: unchanged=%v\n", func() bool {
		e := wildcard(selectBase()).SetLimitInput("")
		after := e.ClearLimitValue()
		b, a := e.RunnableReport(), after.RunnableReport()
		return b == a && after.LimitInput() == ""
	}())
	upd := setSubmitted(querybuilder.NewQuery().RefreshSchema(catalog()).SelectCommand(querybuilder.CommandUpdate).SelectTable("items"), "name", "x")
	updCleared := upd.ClearSetValue("name")
	a := updCleared.SetAssignments()[0]
	_, submitted := a.SubmittedValue()
	fmt.Printf("UPDATE Value cleared: choice=%v submitted=%v -> report %s\n", a.Choice(), submitted, updCleared.RunnableReport().Reason)
	ins, _ := begun.ChooseInsertColumn("id", qb.InsertChoiceValue)
	ins, _ = ins.SubmitInsertValue("id", "1")
	insCleared := ins.ClearInsertValue("id")
	fmt.Printf("INSERT Value cleared: choice=%v submitted=%v -> report %s\n", func() qb.InsertChoice {
		for _, c := range insCleared.InsertColumns() {
			if c.Column == "id" {
				return c.Choice()
			}
		}
		return qb.InsertChoiceNone
	}(), func() bool {
		for _, c := range insCleared.InsertColumns() {
			if c.Column == "id" {
				_, ok := c.SubmittedValue()
				return ok
			}
		}
		return false
	}(), insCleared.RunnableReport().Reason)
	fmt.Println("  (UPDATE/INSERT end-to-end flows land in Issues #37/#39; the transitions are shared now)")
}
