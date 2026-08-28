// This file adds the safe SQL atom layer (Issue #14) to the querybuilder
// package: schema-derived identifier quoting and closed typed choices for
// predicate operators, projection aggregates, and ordering directions.

package querybuilder

import (
	"fmt"
	"strings"

	"github.com/chris/sqloid/internal/schema"
)

// quoteIdentifierAtom returns one SQL-standard double-quoted identifier atom
// with each embedded double quote doubled. It quotes exactly one atom: no
// schema qualification is parsed or preserved, and no user-authored SQL is
// ever accepted here.
func quoteIdentifierAtom(atom string) string {
	quoted := strings.ReplaceAll(atom, `"`, `""`)
	return `"` + quoted + `"`
}

// ObjectIdentifier returns the double-quoted SQL identifier for one refreshed
// schema object, quoted as a single atom with embedded double quotes doubled.
func ObjectIdentifier(obj *schema.Object) string {
	return quoteIdentifierAtom(obj.Name)
}

// ColumnIdentifier returns the double-quoted SQL identifier for one schema
// column identity, quoted as a single atom with embedded double quotes doubled.
func ColumnIdentifier(col schema.Column) string {
	return quoteIdentifierAtom(col.Name)
}

// Operator is a closed typed choice of v1 WHERE predicate operators per the
// SQL safety decision in Notes/PRD-sqloid.md. The zero value is not a valid
// operator; renderers reject it instead of emitting caller-controlled text.
type Operator int

const (
	// OpEq is the '=' predicate operator.
	OpEq Operator = iota + 1
	// OpNotEq is the '!=' predicate operator.
	OpNotEq
	// OpLt is the '<' predicate operator.
	OpLt
	// OpLe is the '<=' predicate operator.
	OpLe
	// OpGt is the '>' predicate operator.
	OpGt
	// OpGe is the '>=' predicate operator.
	OpGe
	// OpIsNull is the value-less 'IS NULL' predicate operator.
	OpIsNull
	// OpIsNotNull is the value-less 'IS NOT NULL' predicate operator.
	OpIsNotNull
	// OpLike is the 'LIKE' predicate operator.
	OpLike
)

// TakesValue reports whether the operator consumes a bound value; IS NULL and
// IS NOT NULL never do.
func (op Operator) TakesValue() bool {
	return op != OpIsNull && op != OpIsNotNull && op != 0
}

// SQLToken renders the exact fixed SQL token for the operator. Invalid zero or
// out-of-range operators return an empty token and a typed error rather than
// arbitrary text.
func (op Operator) SQLToken() (string, error) {
	switch op {
	case OpEq:
		return "=", nil
	case OpNotEq:
		return "!=", nil
	case OpLt:
		return "<", nil
	case OpLe:
		return "<=", nil
	case OpGt:
		return ">", nil
	case OpGe:
		return ">=", nil
	case OpIsNull:
		return "IS NULL", nil
	case OpIsNotNull:
		return "IS NOT NULL", nil
	case OpLike:
		return "LIKE", nil
	default:
		return "", fmt.Errorf("invalid operator %d", int(op))
	}
}

// Aggregate is a closed typed choice of v1 projection aggregates. The zero
// value represents an unaggregated projected column and has no token.
type Aggregate int

const (
	// AggCount is COUNT.
	AggCount Aggregate = iota + 1
	// AggMin is MIN.
	AggMin
	// AggMax is MAX.
	AggMax
	// AggAvg is AVG.
	AggAvg
	// AggSum is SUM.
	AggSum
)

// SQLToken renders the exact fixed SQL function name token for the aggregate.
// Invalid zero or out-of-range aggregates return an error.
func (agg Aggregate) SQLToken() (string, error) {
	switch agg {
	case AggCount:
		return "COUNT", nil
	case AggMin:
		return "MIN", nil
	case AggMax:
		return "MAX", nil
	case AggAvg:
		return "AVG", nil
	case AggSum:
		return "SUM", nil
	default:
		return "", fmt.Errorf("invalid aggregate %d", int(agg))
	}
}

// Direction is a closed typed choice of ordering directions with an explicit,
// invalid zero value so that unset directions are rejected by the renderer.
type Direction int

const (
	// DirAsc orders ascending.
	DirAsc Direction = iota + 1
	// DirDesc orders descending.
	DirDesc
)

// SQLToken renders the exact fixed SQL token for the direction. Invalid zero
// or out-of-range directions return an error.
func (dir Direction) SQLToken() (string, error) {
	switch dir {
	case DirAsc:
		return "ASC", nil
	case DirDesc:
		return "DESC", nil
	default:
		return "", fmt.Errorf("invalid direction %d", int(dir))
	}
}

// Predicate is one assembled WHERE predicate over a schema-owned column with a
// typed operator and a universally parsed value that stays on the parameter
// list, keeping deterministic parameter ordering for future query builders.
type Predicate struct {
	col   schema.Column // schema-derived column identity, never revalidated here
	op    Operator      // closed typed operator choice
	value Value         // universally parsed user value; unused by null operators
}

// NewPredicate validates the typed choices at construction time and returns a
// typed error when the operator or its compatibility with the value is invalid;
// it never accepts raw operator or identifier text.
func NewPredicate(col schema.Column, op Operator, v Value) (Predicate, error) {
	if _, err := op.SQLToken(); err != nil {
		return Predicate{}, err
	}
	if !op.TakesValue() {
		return Predicate{col: col, op: op}, nil
	}
	return Predicate{col: col, op: op, value: v}, nil
}

// SQL renders the predicate text with a '?' placeholder wherever a bound value
// belongs; user-entered values are never interpolated into this text.
func (p Predicate) SQL() string {
	token, _ := p.op.SQLToken()
	if !p.op.TakesValue() {
		return quoteIdentifierAtom(p.col.Name) + " " + token
	}
	return quoteIdentifierAtom(p.col.Name) + " " + token + " ?"
}

// Params returns this predicate's bound parameter values in deterministic
// order: none for IS NULL / IS NOT NULL, otherwise exactly the single parsed
// value's driver-facing parameter.
func (p Predicate) Params() []any {
	if !p.op.TakesValue() {
		return nil
	}
	return []any{p.value.ParamValue()}
}
