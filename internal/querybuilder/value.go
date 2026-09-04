// This file adds the universal value parser and bound-parameter contract
// (Issue #14) to the querybuilder package.

package querybuilder

import (
	"math"
	"strconv"
	"strings"
)

// ParsedKind classifies one universally parsed text value.
type ParsedKind int

const (
	// KindText is the fallback for every token that is neither an
	// INTEGER nor a finite REAL, kept verbatim.
	KindText ParsedKind = iota + 1
	// KindInteger marks a `-?[0-9]+` token that fits signed int64.
	KindInteger
	// KindReal marks a finite float64 accepted by strconv.ParseFloat.
	KindReal
)

// String renders the human-facing name of the parsed kind used in tests,
// history metadata, and diagnostics.
func (k ParsedKind) String() string {
	switch k {
	case KindText:
		return "TEXT"
	case KindInteger:
		return "INTEGER"
	case KindReal:
		return "REAL"
	default:
		return "ParsedKind(" + strconv.Itoa(int(k)) + ")"
	}
}

// Value is one universally parsed user-entered value together with its exact
// typed payload. The original text is preserved verbatim; classification
// happens without any trimming or normalization. There is no declared-column
// coercion: parsing depends only on the entered characters.
type Value struct {
	Kind ParsedKind // which payload field carries the value

	Text string  // verbatim original text when Kind == KindText
	Int  int64   // when Kind == KindInteger
	Real float64 // finite float64 when Kind == KindReal
}

// ParseValue parses one universal text input according to the Numeric value
// parsing decision in Notes/PRD-sqloid.md: INTEGER first (`-?[0-9]+` fitting
// signed int64, leading zeros allowed), then REAL (any finite float64 accepted
// by strconv.ParseFloat, including hexadecimal floating-point forms but never
// a leading '+'), then exact TEXT verbatim. Input is not trimmed: whitespace
// anywhere makes the whole token TEXT. Typed NULL, NaN/Inf spellings,
// hexadecimal integers, and overflow beyond float64 all remain TEXT.
func ParseValue(input string) Value {
	if len(input) == 0 || input[0] == '+' {
		return Value{Kind: KindText, Text: input}
	}
	if i, ok := parseIntegerLiteral(input); ok {
		return Value{Kind: KindInteger, Int: i}
	}
	if f, err := strconv.ParseFloat(input, 64); err == nil && !isNonFinite(f) {
		return Value{Kind: KindReal, Real: f}
	}
	return Value{Kind: KindText, Text: input}
}

// parseIntegerLiteral reports whether s matches exactly `-?[0-9]+` and fits
// in a signed 64-bit integer, returning the parsed value on success. Leading
// zeros are allowed; a bare sign or overflowing digits fail.
func parseIntegerLiteral(s string) (int64, bool) {
	digits := s
	if s[0] == '-' {
		digits = s[1:]
		if digits == "" {
			return 0, false
		}
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

// isNonFinite reports whether f is NaN or ±Inf, which cannot be represented as
// bound SQLite values under v1 policy.
func isNonFinite(f float64) bool {
	return f != f || math.IsInf(f, 0)
}

// ParamValue returns the stable concrete Go type to hand to the database
// driver as this value's bound parameter: int64 for INTEGER, float64 for REAL,
// and the verbatim string for TEXT — including typed NULL and empty input,
// which stay strings rather than becoming SQL null.
func (v Value) ParamValue() any {
	switch v.Kind {
	case KindInteger:
		return v.Int
	case KindReal:
		return v.Real
	default:
		return v.Text
	}
}

// quoteTextLiteral encloses s in single quotes with every embedded single
// quote doubled, per the SQL safety decision in Notes/PRD-sqloid.md.
func quoteTextLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b.WriteString("''")
			continue
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('\'')
	return b.String()
}
