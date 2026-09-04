package main

// Issue #85 walkthrough demonstration: render representative finite and
// non-finite REAL values through every consumer that shares the canonical
// internal/result.RealToken, proving identical finite tokens and the
// distinct non-finite policies (query literals reject; result formatting
// retains Inf/-Inf/NaN). Reference: Issue #85 and Notes/PRD-sqloid.md.

import (
	"fmt"
	"math"

	"github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	fmt.Println("=== Finite REAL tokens: identical across result.RealToken and RenderSQLLiteral ===")
	values := []struct {
		name string
		v    float64
	}{
		{"1.0 (integral REAL)", 1.0},
		{"-0.0 (negative zero)", math.Copysign(0, -1)},
		{"1e+20 (exponent)", 1e20},
		{"5e-324 (smallest subnormal)", math.SmallestNonzeroFloat64},
		{"1.7976931348623157e+308 (max finite)", math.MaxFloat64},
		{"0.30000000000000004 (precision edge)", 0.1 + 0.2},
		{"nextafter(1, 0) (adjacent)", math.Nextafter(1, 0)},
		{"nextafter(1, 2) (adjacent)", math.Nextafter(1, 2)},
	}
	fmt.Printf("%-38s %-28s %-28s %s\n", "value", "result.RealToken", "RenderSQLLiteral", "match")
	for _, c := range values {
		rt := result.RealToken(c.v)
		sql, err := querybuilder.RenderSQLLiteral(querybuilder.Literal{Kind: querybuilder.LiteralReal, Real: c.v})
		if err != nil {
			fmt.Printf("%-38s %-28s ERROR: %v\n", c.name, rt, err)
			continue
		}
		match := "OK"
		if rt != sql {
			match = "MISMATCH"
		}
		fmt.Printf("%-38s %-28s %-28s %s\n", c.name, rt, sql, match)
	}
	fmt.Println()
	fmt.Println("=== Non-finite REAL: query literals reject, result formatting retains tokens ===")
	nonFinite := []struct {
		name string
		v    float64
	}{
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"not a number", math.NaN()},
	}
	fmt.Printf("%-22s %-18s %-12s %s\n", "value", "result.RealToken", "SQL token", "query policy")
	for _, c := range nonFinite {
		rt := result.RealToken(c.v)
		sql, err := querybuilder.RenderSQLLiteral(querybuilder.Literal{Kind: querybuilder.LiteralReal, Real: c.v})
		policy := fmt.Sprintf("rejects (err=%v)", err)
		fmt.Printf("%-22s %-18s %-12q %s\n", c.name, rt, sql, policy)
	}
}
