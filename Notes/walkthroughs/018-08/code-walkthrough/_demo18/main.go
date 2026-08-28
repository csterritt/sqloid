// Runnable demonstration for Issue #18 (Notes/PRD-sqloid.md): drives the real
// QueryBuilder transitions through the complete SELECT grouping matrix, the
// context-valid ORDER BY candidates with ASC default and direction toggling,
// and the bounded LIMIT parser, printing exact validity (FirstInvalidIssue
// field/reason pairs) and safely rendered SelectSQL evidence for every case.
package main

import (
	"fmt"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// users returns the two-visible-column fixture used throughout the demo.
func users() *schema.Object {
	return &schema.Object{
		Name: "users", Kind: schema.KindOrdinaryTable,
		WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}, {Name: "email"}, {Name: "created_at", Hidden: true}},
	}
}

func base() qb.QueryBuilder {
	return qb.NewQuery().
		RefreshSchema(&schema.Catalog{Version: 1, Objects: []*schema.Object{users()}}).
		SelectCommand(qb.CommandSelect).
		SelectTable("users")
}

func proj(q qb.QueryBuilder, col string, agg qb.Aggregate) qb.QueryBuilder {
	if col == "" { // bare COUNT(*) sentinel
		return q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
	}
	out := q.CompleteProjectionAggregate(col, agg)
	return out.Builder
}

func report(q qb.QueryBuilder) string {
	issue, invalid := q.FirstInvalidIssue()
	if !invalid {
		return "valid"
	}
	return fmt.Sprintf("INVALID [%s] %s", issue.Field, issue.Reason)
}

func main() {
	fmt.Println("== 1. GROUP BY assisted multi-selection: order, duplicates, candidates ==")
	q := base()
	fmt.Println("candidates:", q.GroupByCandidates(), "(hidden created_at excluded)")
	q, _ = q.AcceptGroupColumn("email") // deliberate reverse-Schema order
	q, _ = q.AcceptGroupColumn("id")
	fmt.Println("committed:", q.GroupByEntries())
	dup, ok := q.AcceptGroupColumn("email")
	fmt.Println("duplicate accept ok=", ok, "entries=", dup.GroupByEntries())
	hidden, ok := q.AcceptGroupColumn("created_at")
	fmt.Println("hidden accept ok=", ok, "entries=", hidden.GroupByEntries())

	fmt.Println()
	fmt.Println("== 2. grouping validity matrix ==")
	rows := []struct {
		label string
		q     qb.QueryBuilder
	}{
		{"nonaggregate-only, no group", base()},
		{"mixed COUNT(id)+email, no group", proj(proj(base(), "id", qb.AggCount), "email", qb.AggregateValue)},
		{"mixed, missing one group", func() qb.QueryBuilder {
			q := proj(proj(base(), "id", qb.AggMin), "email", qb.AggregateValue)
			q, _ = q.AcceptGroupColumn("id")
			return q
		}()},
		{"mixed, every nonaggregate grouped", func() qb.QueryBuilder {
			q := proj(proj(base(), "id", qb.AggMax), "email", qb.AggregateValue)
			q, _ = q.AcceptGroupColumn("email")
			return q
		}()},
		{"mixed with extra grouped columns", func() qb.QueryBuilder {
			q := proj(proj(base(), "id", qb.AggregateValue), "email", qb.AggregateValue)
			q, _ = q.AcceptGroupColumn("id")
			q, _ = q.AcceptGroupColumn("email")
			return q
		}()},
		{"all-aggregate without group", proj(proj(base(), "id", qb.AggSum), "email", qb.AggAvg)},
		{"bare COUNT(*) without group", proj(base(), "", 0)},
		{"wildcard with GROUP BY", func() qb.QueryBuilder {
			q := base().AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}).Builder
			q, _ = q.AcceptGroupColumn("id")
			return q
		}()},
	}
	for _, r := range rows {
		fmt.Printf("%-40s %s\n", r.label+":", report(r.q))
	}
	mixed := proj(proj(base(), "id", qb.AggCount), "email", qb.AggregateValue)
	fmt.Println("mixed no group  SQL:", mixed.SelectSQL())
	grouped := mixed
	grouped, _ = grouped.AcceptGroupColumn("email")
	grouped, _ = grouped.AcceptGroupColumn("id")
	fmt.Println("grouped         SQL:", grouped.SelectSQL())

	fmt.Println()
	fmt.Println("== 3. ORDER BY candidates follow context ==")
	fmt.Println("ungrouped candidates:")
	for _, c := range base().OrderByCandidates() {
		fmt.Printf("  key=%-24s display=%s\n", c.Key, c.Display)
	}
	fmt.Println("grouped COUNT(id) candidates:")
	for _, c := range grouped.OrderByCandidates() {
		fmt.Printf("  key=%-30s display=%s\n", c.Key, c.Display)
	}
	countStar := base().AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}).Builder
	countStar, _ = countStar.AcceptGroupColumn("email")
	fmt.Println("bare COUNT(*) context candidates:")
	for _, c := range countStar.OrderByCandidates() {
		fmt.Printf("  key=%-24s display=%s\n", c.Key, c.Display)
	}

	fmt.Println()
	fmt.Println("== 4. one expression, ASC default, DESC toggle, replacement, clearing ==")
	ord := grouped
	ord, _ = ord.AcceptOrderBy("order-column:email")
	_, dir, _ := ord.OrderBySelection()
	fmt.Println("fresh selection:", ord.SelectSQL(), "dir==ASC:", dir == qb.DirAsc)
	desc := ord.ToggleOrderDirection()
	fmt.Println("toggled:        ", desc.SelectSQL())
	replaced, ok := desc.AcceptOrderBy("order-aggregate:id:COUNT")
	fmt.Println("replaced ok=", ok, "->", replaced.SelectSQL(), "(ASC reset)")
	cleared := replaced.ClearOrderBy()
	fmt.Println("cleared SQL:    ", cleared.SelectSQL())
	if _, ok := base().AcceptOrderBy("order-aggregate:id:COUNT"); !ok {
		fmt.Println("unselected aggregate rejected: true")
	}
	if _, ok := grouped.AcceptOrderBy("garbage"); !ok {
		fmt.Println("arbitrary text rejected:       true")
	}

	fmt.Println()
	fmt.Println("== 5. LIMIT: bounds, canonical rendering, exact invalid reason ==")
	limits := []string{"", "1", "9223372036854775807", "007", "0", "-3", "+5", " 5", "5.5", "1e3", "0x10", "many", "9223372036854775808", "10000000000000000000000000000000000"}
	for _, input := range limits {
		lq := base().SetLimitInput(input)
		_, ok := lq.LimitValue()
		label := input
		if label == "" {
			label = "(empty)"
		}
		fmt.Printf("input=%-24s accepted=%-5v %s SQL=%s\n", label, ok, report(lq), lq.SelectSQL())
	}
	rev := base().SetLimitInput("5").SetLimitInput("x")
	fmt.Println("revision 5→x keeps entered text:", rev.LimitInput() == "x")

	fmt.Println()
	fmt.Println("== 6. full statement composition ==")
	full := grouped
	full, _ = full.AcceptOrderBy("order-column:email")
	full = full.SetOrderDirection(qb.DirDesc)
	full = full.SetLimitInput("10")
	fmt.Println(full.SelectSQL())
	fmt.Println("params:", full.SelectParams())
}
