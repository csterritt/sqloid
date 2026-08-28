// Command demo17 prints Issue #17 guided-WHERE demonstrations for the code
// walkthrough. It lives under an underscore directory so the Go tooling never
// builds it into Sqloid; run it explicitly with `go run ./_demo17`.
package main

import (
	"fmt"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func builder(cmd qb.Command) qb.QueryBuilder {
	obj := &schema.Object{
		Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true,
		Rowid: schema.RowidHas, InsertableCount: 3,
		Columns: []schema.Column{
			{Name: "id", DeclaredType: "INTEGER"},
			{Name: "email", DeclaredType: "TEXT"},
			{Name: `weird""col`, DeclaredType: ""}, // untyped punctuation-shaped identity
		},
	}
	return qb.NewQuery().SelectCommand(cmd).
		RefreshSchema(&schema.Catalog{Version: 7, Objects: []*schema.Object{obj}}).
		SelectTable("users")
}

func operatorToken(op qb.Operator) string {
	tok, err := op.SQLToken()
	if err != nil {
		panic(err)
	}
	return tok
}

// complete walks column → operator → value through one draft per consumer.
func complete(b qb.QueryBuilder, col, token, input string) (qb.QueryBuilder, bool) {
	b2, ok := b.StartWhere(col)
	if !ok {
		return b, false
	}
	var chosen qb.Operator
	for _, op := range b.FixedOperators() {
		if operatorToken(op) == token {
			chosen = op
		}
	}
	pred, ok := b2.WhereDraft().ChooseOperator(chosen)
	if !ok {
		return b, false
	}
	b3 := b2.ApplyWhereDraft(pred)
	pred, ok = pred.SubmitValue(input)
	return b3.ApplyWhereDraft(pred).CommitWhereDraft()
}

func main() {
	fmt.Println("== 1. eligible columns + closed operator set (no type filtering) ==")
	for _, cmd := range []qb.Command{qb.CommandSelect, qb.CommandUpdate, qb.CommandDelete} {
		b := builder(cmd)
		names := []string{}
		for _, c := range b.WhereCandidates() {
			names = append(names, c.Name)
		}
		toks := []string{}
		for _, op := range b.FixedOperators() {
			toks = append(toks, operatorToken(op))
		}
		fmt.Printf("%s columns=%v\n%13s operators=%v\n", cmd, names, "", toks)
	}

	fmt.Println("\n== 2. null operators: complete immediately, no placeholder, no parameter ==")
	b := builder(qb.CommandDelete)
	for _, token := range []string{"IS NULL", "IS NOT NULL"} {
		c, ok := complete(b, "email", token, "STALE JUNK THAT MUST VANISH")
		if !ok {
			panic("commit failed")
		}
		fmt.Printf("committed SQL=%q  params=%v\n", c.WherePredicate().SQL(), c.WhereParams())
	}

	fmt.Println("\n== 3. typed NULL, empty input, LIKE wildcards: exact bound types ==")
	c1, _ := complete(b, "id", "=", "NULL")
	fmt.Printf("SQL=%q  param=%#v (%T)\n", c1.WherePredicate().SQL(),
		c1.WhereParams()[0], c1.WhereParams()[0])
	c2, _ := complete(b, "id", ">=", "")
	fmt.Printf("SQL=%q  param=%#v (%T)\n", c2.WherePredicate().SQL(),
		c2.WhereParams()[0], c2.WhereParams()[0])
	c3, ok := complete(b, `weird""col`, "LIKE", "%a_b%")
	if !ok {
		panic("LIKE commit failed")
	}
	fmt.Printf("SQL=%q  param=%#v (%T)   [wildcard text absent from SQL: %v]\n",
		c3.WherePredicate().SQL(), c3.WhereParams()[0], c3.WhereParams()[0],
		!contains(c3.WherePredicate().SQL(), "%a_b%"))

	fmt.Println("\n== 4. value operators stay incomplete until submission ==")
	s, ok := builder(qb.CommandSelect).StartWhere(`weird""col`)
	if !ok {
		panic("start failed")
	}
	var eq qb.Operator
	for _, op := range s.FixedOperators() {
		if operatorToken(op) == "=" {
			eq = op
		}
	}
	awaiting, ok := s.WhereDraft().ChooseOperator(eq)
	fmt.Printf("after choosing '=': State=%v  SQL=%q\n", awaiting.State(), awaiting.SQL())
	done, ok := awaiting.SubmitValue("-7")
	if !ok || done.State() != qb.WhereComplete {
		panic("submit failed")
	}
	fmt.Printf("after submitting '-7': State=%v  SQL=%q  param=%#v (%T)\n",
		done.State(), done.SQL(), done.Params()[0], done.Params()[0])

	fmt.Println("\n== 5. same-column revision restores; Esc-style cancel preserves ==")
	r, ok := complete(builder(qb.CommandUpdate), "email", "=", "tricky'x_50%")
	if !ok || r.WherePredicate().SQL() != `"email" = ?` {
		panic("setup commit failed")
	}
	rv, ok := r.StartWhere("email")
	if !ok {
		panic("revisit refused")
	}
	entered, has := rv.WhereDraft().Entered()
	fmt.Printf("revisit draft input=%q restored=%v\n", entered, has)
	cancelled := rv.CancelWhereDraft()
	fmt.Printf("cancel → HasWhere=%v SQL=%q params=%#v\n",
		cancelled.HasWhere(), cancelled.WherePredicate().SQL(), cancelled.WhereParams())

	fmt.Println("\n== 6. identical rendering/parameter contract across consumers ==")
	for _, cmd := range []qb.Command{qb.CommandSelect, qb.CommandUpdate, qb.CommandDelete} {
		c, ok := complete(builder(cmd), "email", "!=", "42")
		if !ok {
			panic("commit failed")
		}
		p := c.WhereParams()
		fmt.Printf("%s  SQL=%q  param=%d %#v (%T)\n", cmd, c.WherePredicate().SQL(), len(p), p[0], p[0])
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
