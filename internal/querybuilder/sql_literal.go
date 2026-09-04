// This file adds the shared canonical SQL literal renderer (Issue #14) to the
// querybuilder package. It is the sole serializer for typed standalone SQL
// literal tokens, owned here so both destructive modal SQL and saved
// standalone SQL reuse one implementation; ordinary query execution continues
// to use bound parameters instead of rendered literals.

package querybuilder

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chris/sqloid/internal/result"
)

// LiteralKind classifies one typed standalone-SQL literal, extending the
// universal input kinds with explicit SQL NULL and BLOB payloads that can be
// constructed internally but never arrive as user text input.
type LiteralKind int

const (
	// LiteralNull renders exactly NULL; it is emitted only through explicit
	// popup/operator choices, never by parsing user text.
	LiteralNull LiteralKind = iota + 1
	// LiteralText is verbatim single-quoted TEXT with quote doubling.
	LiteralText
	// LiteralInteger is exact canonical decimal INTEGER.
	LiteralInteger
	// LiteralReal is a finite REAL in PRD shortest round-trip form.
	LiteralReal
	// LiteralBlob is X'hex' bytes with an uppercase X and lowercase hex payload.
	LiteralBlob
)

// String renders the human-facing name of the literal kind used in tests and
// diagnostics.
func (k LiteralKind) String() string {
	switch k {
	case LiteralNull:
		return "NULL"
	case LiteralText:
		return "TEXT"
	case LiteralInteger:
		return "INTEGER"
	case LiteralReal:
		return "REAL"
	case LiteralBlob:
		return "BLOB"
	default:
		return "LiteralKind(" + strconv.Itoa(int(k)) + ")"
	}
}

// Literal is one typed value prepared for standalone SQL rendering. Exactly
// one payload field is meaningful per Kind; RenderSQLLiteral owns validation.
type Literal struct {
	Kind LiteralKind
	Int  int64   // when Kind == LiteralInteger
	Real float64 // finite float64 when Kind == LiteralReal
	Text string  // when Kind == LiteralText
	Blob []byte  // retained bytes when Kind == LiteralBlob; callers own the slice
}

// Literal converts a universally parsed Value into its matching literal kind,
// reusing the parse classification without any second parsing pass. Values
// never become SQL null: typed NULL and empty input stay TEXT literals.
func (v Value) Literal() Literal {
	switch v.Kind {
	case KindInteger:
		return Literal{Kind: LiteralInteger, Int: v.Int}
	case KindReal:
		return Literal{Kind: LiteralReal, Real: v.Real}
	default:
		return Literal{Kind: LiteralText, Text: v.Text}
	}
}

// RenderSQLLiteral renders one typed literal into its exact standalone SQL
// token per Notes/PRD-sqloid.md: INTEGER as canonical decimal, REAL via the
// locale-independent shortest round-trip token with `.0` appended when it
// contains none of `.`, `e`, or `E`, TEXT double-quoted inside single quotes,
// exactly `NULL`, and BLOB as `X'hex'`. Finite REAL tokens delegate to the
// single canonical result.RealToken shared by grid, CSV, and JSON, so there
// is one finite REAL formatting implementation across consumers. Non-finite
// REAL values return a typed error rather than producing unsafe or
// nonportable SQL; this rejection happens before the shared call because
// result.RealToken intentionally returns display/export tokens (Inf, -Inf,
// NaN) for non-finite database values rather than rejecting them.
func RenderSQLLiteral(l Literal) (string, error) {
	switch l.Kind {
	case LiteralNull:
		return "NULL", nil
	case LiteralText:
		return quoteTextLiteral(l.Text), nil
	case LiteralInteger:
		return strconv.FormatInt(l.Int, 10), nil
	case LiteralReal:
		if isNonFinite(l.Real) {
			return "", fmt.Errorf("cannot render non-finite REAL %v", l.Real)
		}
		return result.RealToken(l.Real), nil
	case LiteralBlob:
		var b strings.Builder
		b.Grow(4 + 2*len(l.Blob))
		b.WriteString("X'")
		for _, c := range l.Blob {
			const hexdigits = "0123456789abcdef"
			b.WriteByte(hexdigits[c>>4])
			b.WriteByte(hexdigits[c&0x0f])
		}
		b.WriteByte('\'')
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported literal kind %d", int(l.Kind))
	}
}
