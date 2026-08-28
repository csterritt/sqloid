// Pure table-driven tests for the shared standalone SQL literal renderer, per
// Issue #14 Task 5 and the Numeric value parsing and rendering, SQL safety,
// and Query save targeting decisions in Notes/PRD-sqloid.md: INTEGER in exact
// canonical decimal form, finite REAL with REAL-identity-preserving shortest
// round-trip tokens plus ".0", TEXT with single-quote doubling, exactly NULL,
// and BLOB as X'hex' with an uppercase X and lowercase hex payload. Typed
// non-finite REAL values are rejected; BLOB remains unsupported as universal
// user input but is renderable from typed data. The renderer depends on no UI,
// model, or modal state.

package querybuilder

import (
	"math"
	"testing"
)

// TestRenderIntegerLiterals pins exact canonical decimal INTEGER output at
// signed-int64 boundaries and ordinary values.
func TestRenderIntegerLiterals(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{42, "42"},
		{-7, "-7"},
		{math.MaxInt64, "9223372036854775807"},
		{math.MinInt64, "-9223372036854775808"},
	}
	for _, tc := range cases {
		got, err := RenderSQLLiteral(Literal{Kind: LiteralInteger, Int: tc.in})
		if err != nil || got != tc.want {
			t.Errorf("RenderSQLLiteral(INTEGER %d) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
		}
	}
}

// TestRenderRealLiterals pins PRD REAL formatting: locale-independent shortest
// round-trip token with `.0` appended when the token contains none of `.`, `e`,
// or `E`, preserving REAL identity for integral values, negative zero, and
// subnormals; non-finite input is rejected.
func TestRenderRealLiterals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1, "1.0"},                     // integral REAL keeps REAL identity
		{math.Copysign(0, -1), "-0.0"}, // negative zero identity
		{4, "4.0"},                     // hexadecimal float input result
		{3.14, "3.14"},                 // decimal passthrough
		{0.5, "0.5"},                   // leading-dot input
		{1e20, "1e+20"},                // exponent output
		{1e-05, "1e-05"},               // negative exponent
		{5e-324, "5e-324"},             // smallest positive subnormal
		{math.MaxFloat64, "1.7976931348623157e+308"}, // largest finite
	}
	for _, tc := range cases {
		got, err := RenderSQLLiteral(Literal{Kind: LiteralReal, Real: tc.in})
		if err != nil || got != tc.want {
			t.Errorf("RenderSQLLiteral(REAL %v) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
		}
	}

	for _, f := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if tok, err := RenderSQLLiteral(Literal{Kind: LiteralReal, Real: f}); err == nil || tok != "" {
			t.Errorf("RenderSQLLiteral(REAL %v) = (%q, %v), want typed rejection with empty token", f, tok, err)
		}
	}
}

// TestRenderTextLiterals pins single-quoted TEXT with every embedded single
// quote doubled, preserving emptiness, whitespace, NUL bytes, and
// injection-shaped content verbatim inside the quoted token.
func TestRenderTextLiterals(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"'''", "''''''''"},
		{" spaces ", "' spaces '"},
		{"line\nbreak\ttab", "'line\nbreak\ttab'"},
		{"nul\x00byte", "'nul\x00byte'"},
		{"'; DROP TABLE users--", "'''; DROP TABLE users--'"},
		{"NULL", "'NULL'"},
	}
	for _, tc := range cases {
		got, err := RenderSQLLiteral(Literal{Kind: LiteralText, Text: tc.in})
		if err != nil || got != tc.want {
			t.Errorf("RenderSQLLiteral(TEXT %q) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
		}
	}
}

// TestRenderNullAndBlobLiterals pins exactly NULL for typed SQL null and the
// uppercase-X lowercase-hex `X'hex'` BLOB form including empty payloads.
func TestRenderNullAndBlobLiterals(t *testing.T) {
	cases := []struct {
		name    string
		lit     Literal
		want    string
		wantErr bool
	}{
		{name: "null keyword", lit: Literal{Kind: LiteralNull}, want: "NULL"},
		{name: "empty blob", lit: Literal{Kind: LiteralBlob}, want: "X''"},
		{name: "blob zero byte", lit: Literal{Kind: LiteralBlob, Blob: []byte{0x00}}, want: "X'00'"},
		{name: "blob mixed bytes", lit: Literal{Kind: LiteralBlob, Blob: []byte{0x01, 0xAB, 0xFF}}, want: "X'01abff'"},
		{name: "invalid kind rejected", lit: Literal{}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderSQLLiteral(tc.lit)
			if tc.wantErr {
				if err == nil || got != "" {
					t.Errorf("RenderSQLLiteral(%v) = (%q, %v), want typed rejection", tc.lit.Kind, got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("RenderSQLLiteral = (%q, %v), want (%q, nil)", got, err, tc.want)
			}
		})
	}
}

// TestValueConvertsToLiteralWithoutReparsing pins that parsed Values reach the
// literal renderer through a conversion into typed literals without any second
// parse pass changing classification.
func TestValueConvertsToLiteralWithoutReparsing(t *testing.T) {
	cases := []struct {
		input  string
		sql    string
		reject bool
	}{
		{input: "007", sql: "7"},
		{input: "-9223372036854775808", sql: "-9223372036854775808"},
		{input: "0x1p2", sql: "4.0"},
		{input: "-0.0", sql: "-0.0"},
		{input: "", sql: "''"},
		{input: "NULL", sql: "'NULL'"},
		{input: "'; --", sql: `'''; --'`},
	}
	for _, tc := range cases {
		v := ParseValue(tc.input)
		got, err := RenderSQLLiteral(v.Literal())
		if tc.reject {
			if err == nil {
				t.Errorf("RenderSQLLiteral(Value %q) accepted, want rejection", tc.input)
			}
			continue
		}
		if err != nil || got != tc.sql {
			t.Errorf("RenderSQLLiteral(Value %q) = (%q, %v), want (%q, nil)", tc.input, got, err, tc.sql)
		}
	}
}
