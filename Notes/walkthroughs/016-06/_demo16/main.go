// Command demo16 prints Issue #16 ordered-projection demonstrations for the
// code walkthrough. It lives under an underscore directory so the Go tooling
// never builds it into Sqloid.
package main

import (
	"fmt"
	"os"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func fresh() qb.QueryBuilder {
	obj := &schema.Object{
		Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}, {Name: "email"}, {Name: "created_at", Hidden: true}},
	}
	return qb.NewQuery().
		RefreshSchema(&schema.Catalog{Version: 3, Objects: []*schema.Object{obj}}).
		SelectCommand(qb.CommandSelect).SelectTable("users")
}

func entries(q qb.QueryBuilder) []string {
	var out []string
	for _, e := range q.ProjectionEntries() {
		switch e.Kind {
		case qb.ProjectionWildcard:
			out = append(out, "*")
		case qb.ProjectionCountStar:
			out = append(out, "COUNT(*)")
		default:
			if e.Aggregate != 0 {
				tok, _ := e.Aggregate.SQLToken()
				out = append(out, e.Column+"("+tok+")")
			} else {
				out = append(out, e.Column)
			}
		}
	}
	return out
}

func show(label string, q qb.QueryBuilder) {
	fmt.Println(label+":", fmt.Sprint(entries(q)))
}

func named(q qb.QueryBuilder, col string, agg qb.Aggregate) qb.QueryBuilder {
	return q.CompleteProjectionAggregate(col, agg).Builder
}

func main() {
	switch mode := os.Args[1]; mode {
	case "order":
		b := fresh()
		sentinel := b.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
		show("after COUNT(*) accepted directly", sentinel)
		mix := named(named(named(sentinel, "email", qb.AggregateValue), "id", qb.AggCount), "email", qb.AggMin)
		mix = named(named(named(mix, "id", qb.AggMax), "email", qb.AggAvg), "id", qb.AggSum)
		show("insertion order across Value/Count/Min/Max/Avg/Sum", mix)
	case "duplicate":
		d := named(fresh(), "email", qb.AggAvg)
		show("before duplicate attempt", d)
		rejected := d.CompleteProjectionAggregate("email", qb.AggAvg)
		fmt.Println("rejected duplicate requests reopen:", rejected.ReopenColumns)
		show("state after duplicate (email,Avg)", rejected.Builder)
		v0 := named(fresh(), "id", qb.AggregateValue)
		r2 := v0.CompleteProjectionAggregate("id", qb.AggregateValue).Builder
		fmt.Println("(id,Value) duplicate leaves identical pairs:",
			fmt.Sprint(entries(r2)) == fmt.Sprint(entries(v0)))
		next := r2.CompleteProjectionAggregate("id", qb.AggMin).Builder
		show("later distinct append still lands last", next)
	case "sentinel":
		b := fresh()
		s := b.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
		co := named(s, "email", qb.AggMin)
		show("sentinel coexisting with email(Min)", co)
		dup := co.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar})
		fmt.Println("direct duplicate-sentinel reopen request:", dup.ReopenColumns)
		show("state after direct duplicate-sentinel transition", dup.Builder)
	case "wildcard":
		b := fresh()
		p := named(named(b.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder,
			"id", qb.AggCount), "email", qb.AggMin)
		show("populated state", p)
		w := p.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
		show("after wildcard selection (atomic replacement)", w)
		g1 := w.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
		g2 := w.CompleteProjectionAggregate("id", qb.AggMin).Builder
		fmt.Printf("beside-wildcard appends blocked: sentinel=%v named=%v\n",
			fmt.Sprint(entries(g1)) == "[*]", fmt.Sprint(entries(g2)) == "[*]")
		w2 := w.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
		fmt.Println("re-accepted wildcard still sole:", len(w2.ProjectionEntries()) == 1)
		e := w.RemoveLatestProjection()
		fmt.Println("removing sole wildcard empties:", e.ProjectionEmpty())
		cands := e.ProjectionCandidates()
		fmt.Println("restored candidates:", cands[0].Display(), ",", cands[1].Display())
	case "remove":
		b := fresh()
		p := named(named(b.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder,
			"id", qb.AggMin), "email", qb.AggregateValue)
		for i := 1; i <= 4; i++ {
			p = p.RemoveLatestProjection()
			show(fmt.Sprintf("press %d (Backspace/Delete)", i), p)
		}
		cands := p.ProjectionCandidates()
		fmt.Println("empty popup candidates:", cands[0].Display(), ",", cands[1].Display())
	}
}
