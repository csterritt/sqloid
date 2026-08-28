// Pure table-driven tests for safe SQL atoms, per Issue #14 Task 3 and the
// SQL safety decision in Notes/PRD-sqloid.md: identifiers arrive only through
// internal/schema object/column identity and are double-quoted atom-by-atom
// with embedded quotes doubled; predicate operators, projection aggregates,
// and ordering directions are closed typed choices rendering exact tokens;
// every user-entered value stays on the parameter list and never appears in
// executable SQL text.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

var identifierCases = []struct {
	name string // identifier exactly as it would come from the refreshed catalog
	want string
}{
	{name: "users", want: `"users"`},
	{name: "", want: `""`},
	{name: "with space", want: `"with space"`},
	{name: "select", want: `"select"`},
	{name: "FROM", want: `"FROM"`},
	{name: "main.users", want: `"main.users"`},
	{name: "tricky\"\"name", want: "\"tricky\"\"\"\"name\""},
	{name: `o'brien`, want: `"o'brien"`},
	{name: "; DROP TABLE users--", want: `"; DROP TABLE users--"`},
	{name: "?", want: `"?"`},
}

// TestObjectIdentifierQuotesOneAtom pins that table identifiers are quoted as
// one double-quoted atom with each embedded double quote doubled, regardless
// of keyword-, qualification-, or punctuation-shaped names.
func TestObjectIdentifierQuotesOneAtom(t *testing.T) {
	for _, tc := range identifierCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &schema.Object{Name: tc.name}
			got := ObjectIdentifier(obj)
			if got != tc.want {
				t.Errorf("ObjectIdentifier(%q) = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

// TestColumnIdentifierQuotesOneAtom pins the same contract for column
// identifiers derived from schema column identities.
func TestColumnIdentifierQuotesOneAtom(t *testing.T) {
	for _, tc := range identifierCases {
		t.Run(tc.name, func(t *testing.T) {
			col := schema.Column{Name: tc.name, DeclaredType: "TEXT", Insertable: true}
			got := ColumnIdentifier(col)
			if got != tc.want {
				t.Errorf("ColumnIdentifier(%q) = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

// TestOperatorSQLTokens pins the exhaustive fixed choice of v1 predicate
// operators and their exact SQL tokens, with invalid values rejected rather
// than passed through as arbitrary text.
func TestOperatorSQLTokens(t *testing.T) {
	cases := []struct {
		op   Operator
		want string
	}{
		{OpEq, "="},
		{OpNotEq, "!="},
		{OpLt, "<"},
		{OpLe, "<="},
		{OpGt, ">"},
		{OpGe, ">="},
		{OpIsNull, "IS NULL"},
		{OpIsNotNull, "IS NOT NULL"},
		{OpLike, "LIKE"},
	}
	for _, tc := range cases {
		got, err := tc.op.SQLToken()
		if err != nil || got != tc.want {
			t.Errorf("Operator(%d).SQLToken() = (%q, %v), want (%q, nil)", int(tc.op), got, err, tc.want)
		}
	}
	if tok, err := Operator(0).SQLToken(); err == nil || tok != "" {
		t.Errorf("Operator(0).SQLToken() = (%q, %v), want rejection with empty token", tok, err)
	}
	if tok, err := Operator(10).SQLToken(); err == nil || tok != "" {
		t.Errorf("Operator(10).SQLToken() = (%q, %v), want rejection with empty token", tok, err)
	}
}

// TestAggregateSQLTokens pins the exhaustive fixed choice of v1 projection
// aggregates and their exact SQL tokens, rejecting invalid values.
func TestAggregateSQLTokens(t *testing.T) {
	cases := []struct {
		agg  Aggregate
		want string
	}{
		{AggCount, "COUNT"},
		{AggMin, "MIN"},
		{AggMax, "MAX"},
		{AggAvg, "AVG"},
		{AggSum, "SUM"},
	}
	for _, tc := range cases {
		got, err := tc.agg.SQLToken()
		if err != nil || got != tc.want {
			t.Errorf("Aggregate(%d).SQLToken() = (%q, %v), want (%q, nil)", int(tc.agg), got, err, tc.want)
		}
	}
	if tok, err := Aggregate(0).SQLToken(); err == nil || tok != "" {
		t.Errorf("Aggregate(0).SQLToken() = (%q, %v), want rejection with empty token", tok, err)
	}
}

// TestDirectionSQLTokens pins the closed ordering-direction choice and its
// exact SQL tokens, rejecting invalid values.
func TestDirectionSQLTokens(t *testing.T) {
	for _, tc := range []struct {
		dir  Direction
		want string
	}{{DirAsc, "ASC"}, {DirDesc, "DESC"}} {
		got, err := tc.dir.SQLToken()
		if err != nil || got != tc.want {
			t.Errorf("Direction(%d).SQLToken() = (%q, %v), want (%q, nil)", int(tc.dir), got, err, tc.want)
		}
	}
	if tok, err := Direction(0).SQLToken(); err == nil || tok != "" {
		t.Errorf("Direction(0).SQLToken() = (%q, %v), want rejection with empty token", tok, err)
	}
	if tok, err := Direction(3).SQLToken(); err == nil || tok != "" {
		t.Errorf("Direction(3).SQLToken() = (%q, %v), want rejection with empty token", tok, err)
	}
}

// TestPredicateBindsValues pins that user-entered values in predicates remain
// bound parameters with unchanged parsed types, appearing as '?' placeholders
// and never interpolated into the SQL text.
func TestPredicateBindsValues(t *testing.T) {
	col := schema.Column{Name: "notes", Insertable: true}

	cases := []struct {
		name      string
		in        string
		sql       string
		bound     any
		boundType reflect.Type
	}{
		{
			name: "injection text", in: "' OR '1'='1",
			sql: `"notes" LIKE ?`, boundType: reflect.TypeOf(""),
		},
		{
			name: "injection integer shape", in: "1; DROP TABLE users--",
			sql: `"notes" = ?`, boundType: reflect.TypeOf(""),
		},
		{
			name: "typed NULL text", in: "NULL",
			sql: `"notes" = ?`, boundType: reflect.TypeOf(""),
		},
		{
			name: "integer", in: "-9223372036854775808",
			sql: `"notes" <= ?`, boundType: reflect.TypeOf(int64(0)),
		},
		{
			name: "real", in: "0x1p2",
			sql: `"notes" >= ?`, boundType: reflect.TypeOf(float64(0)),
		},
	}
	ops := []Operator{OpLike, OpEq, OpEq, OpLe, OpGe}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := ParseValue(tc.in)
			p, err := NewPredicate(col, ops[i], v)
			if err != nil {
				t.Fatalf("NewPredicate(%q) failed: %v", tc.in, err)
			}
			if p.SQL() != tc.sql {
				t.Errorf("predicate SQL = %s, want %s", p.SQL(), tc.sql)
			}
			params := p.Params()
			if len(params) != 1 {
				t.Fatalf("predicate has %d bound params, want exactly 1", len(params))
			}
			if reflect.TypeOf(params[0]) != tc.boundType || !reflect.DeepEqual(params[0], v.ParamValue()) {
				t.Errorf("bound param %#v (%T) does not match parsed value %#v unchanged", params[0], params[0], v.ParamValue())
			}
			for _, s := range []string{tc.in} {
				if s != "" && containsSubstring(p.SQL(), s) {
					t.Errorf("user-entered value %q leaked into SQL text %s", s, p.SQL())
				}
			}
		})
	}
}

// TestNullOperatorPredicatesTakeNoValue pins that IS NULL / IS NOT NULL render
// without placeholders and bind nothing.
func TestNullOperatorPredicatesTakeNoValue(t *testing.T) {
	col := schema.Column{Name: "gone"}
	for _, tc := range []struct {
		op  Operator
		sql string
	}{{OpIsNull, `"gone" IS NULL`}, {OpIsNotNull, `"gone" IS NOT NULL`}} {
		p, err := NewPredicate(col, tc.op, ParseValue(""))
		if err != nil {
			t.Fatalf("NewPredicate(%v) failed: %v", tc.op, err)
		}
		if p.SQL() != tc.sql {
			t.Errorf("SQL = %s, want %s", p.SQL(), tc.sql)
		}
		if n := len(p.Params()); n != 0 {
			t.Errorf("null operator bound %d params, want 0", n)
		}
	}
}

// containsSubstring reports whether substr occurs anywhere in s.
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
