// Pure table-driven tests for the universal value parser and its bound
// parameter values, per Issue #14 Task 1 and the Numeric value parsing and
// rendering and SQL safety decisions in Notes/PRD-sqloid.md.
//
// The contract: input is verbatim with no trimming or normalization;
// INTEGER is recognized first only for `-?[0-9]+` fitting signed int64;
// otherwise REAL requires a finite float64 accepted by strconv.ParseFloat
// (including hexadecimal floating-point forms such as 0x1p2 but not leading
// '+'); everything else falls back to exact TEXT. Typed NULL and empty input
// remain TEXT strings; there is no declared-column-type coercion, no SQL NULL,
// and no BLOB input parsing. Bound types are the stable concrete Go types
// handed to the SQLite driver.

package querybuilder

import (
	"math"
	"reflect"
	"strconv"
	"testing"
)

// valueCase is one universal-parsing contract row: the exact classification,
// typed payload, and driver-facing bound value expected from ParseValue.
type valueCase struct {
	name string
	in   string

	wantKind   ParsedKind
	wantText   string // exact original text when kind == KindText
	wantInt    int64  // when kind == KindInteger
	wantReal   uint64 // IEEE-754 bits of the finite float64 when kind == KindReal (bit comparison distinguishes ±0)
	boundValue any    // exact dynamic type and value returned by Value.ParamValue
}

func floatBits(f float64) uint64 { return math.Float64bits(f) }

var parseValueCases = []valueCase{
	// Verbatim integers, including leading zeros, signs, and boundaries.
	{name: "plain integer", in: "42", wantKind: KindInteger, wantInt: 42, boundValue: int64(42)},
	{name: "zero", in: "0", wantKind: KindInteger, wantInt: 0, boundValue: int64(0)},
	{name: "negative integer", in: "-7", wantKind: KindInteger, wantInt: -7, boundValue: int64(-7)},
	{name: "leading zeros", in: "007", wantKind: KindInteger, wantInt: 7, boundValue: int64(7)},
	{name: "negative leading zeros", in: "-009", wantKind: KindInteger, wantInt: -9, boundValue: int64(-9)},
	{name: "negative zero integer", in: "-0", wantKind: KindInteger, wantInt: 0, boundValue: int64(0)},
	{name: "max int64", in: "9223372036854775807", wantKind: KindInteger, wantInt: math.MaxInt64, boundValue: int64(math.MaxInt64)},
	{name: "min int64", in: "-9223372036854775808", wantKind: KindInteger, wantInt: math.MinInt64, boundValue: int64(math.MinInt64)},

	// Integer overflow falls through to REAL when ParseFloat still accepts it.
	{
		name: "int64 overflow max+1 becomes REAL", in: "9223372036854775808",
		wantKind: KindReal, wantReal: floatBits(9223372036854775808.0), boundValue: 9223372036854775808.0,
	},
	{
		name: "int64 overflow min-1 becomes REAL", in: "-9223372036854775809",
		wantKind: KindReal, wantReal: floatBits(-9223372036854775808.0), boundValue: -9223372036854775808.0,
	},

	// Finite REALs: decimal, exponent, bare-dot forms, and hexadecimal floats.
	{name: "decimal", in: "3.14", wantKind: KindReal, wantReal: floatBits(3.14), boundValue: 3.14},
	{name: "trailing dot", in: "1.", wantKind: KindReal, wantReal: floatBits(1), boundValue: 1.0},
	{name: "leading dot", in: ".5", wantKind: KindReal, wantReal: floatBits(0.5), boundValue: 0.5},
	{name: "decimal zero", in: "0.0", wantKind: KindReal, wantReal: floatBits(0), boundValue: 0.0},
	{name: "negative decimal zero keeps sign bit", in: "-0.0", wantKind: KindReal, wantReal: floatBits(math.Copysign(0, -1)), boundValue: math.Copysign(0, -1)},
	{name: "positive exponent", in: "1e20", wantKind: KindReal, wantReal: floatBits(1e20), boundValue: 1e20},
	{name: "negative exponent uppercase E", in: "1E-5", wantKind: KindReal, wantReal: floatBits(1e-05), boundValue: 1e-05},
	{name: "hexadecimal float", in: "0x1p2", wantKind: KindReal, wantReal: floatBits(4), boundValue: 4.0},
	{name: "negative hexadecimal float", in: "-0x1p2", wantKind: KindReal, wantReal: floatBits(-4), boundValue: -4.0},
	{name: "hexadecimal float fractional mantissa", in: "0x1.8p1", wantKind: KindReal, wantReal: floatBits(3), boundValue: 3.0},

	// Leading '+' is rejected at both stages.
	{name: "leading plus integer is TEXT", in: "+1", wantKind: KindText, wantText: "+1", boundValue: "+1"},
	{name: "leading plus float is TEXT", in: "+1.5", wantKind: KindText, wantText: "+1.5", boundValue: "+1.5"},
	{name: "leading plus hexadecimal float is TEXT", in: "+0x1p2", wantKind: KindText, wantText: "+0x1p2", boundValue: "+0x1p2"},

	// Float overflow is non-finite and therefore TEXT fallback.
	{name: "float overflow to +Inf is TEXT", in: "1e400", wantKind: KindText, wantText: "1e400", boundValue: "1e400"},
	{name: "negative float overflow is TEXT", in: "-1e400", wantKind: KindText, wantText: "-1e400", boundValue: "-1e400"},

	// Non-finite spellings remain TEXT verbatim.
	{name: "NaN is TEXT", in: "NaN", wantKind: KindText, wantText: "NaN", boundValue: "NaN"},
	{name: "Inf is TEXT", in: "Inf", wantKind: KindText, wantText: "Inf", boundValue: "Inf"},
	{name: "-Inf is TEXT", in: "-Inf", wantKind: KindText, wantText: "-Inf", boundValue: "-Inf"},
	{name: "Infinity is TEXT", in: "Infinity", wantKind: KindText, wantText: "Infinity", boundValue: "Infinity"},

	// Hexadecimal integers are not INTEGER and not valid hex floats.
	{name: "hexadecimal integer is TEXT", in: "0x1A", wantKind: KindText, wantText: "0x1A", boundValue: "0x1A"},
	{name: "malformed hexadecimal float is TEXT", in: "0x1p", wantKind: KindText, wantText: "0x1p", boundValue: "0x1p"},

	// Whitespace in every position prevents numeric recognition entirely.
	{name: "leading space", in: " 1", wantKind: KindText, wantText: " 1", boundValue: " 1"},
	{name: "trailing space", in: "1 ", wantKind: KindText, wantText: "1 ", boundValue: "1 "},
	{name: "internal space", in: "1 .5", wantKind: KindText, wantText: "1 .5", boundValue: "1 .5"},
	{name: "space-only input", in: "   ", wantKind: KindText, wantText: "   ", boundValue: "   "},
	{name: "tab and newline padding", in: "\t1\n", wantKind: KindText, wantText: "\t1\n", boundValue: "\t1\n"},
	{name: "space around negative integer", in: " -7 ", wantKind: KindText, wantText: " -7 ", boundValue: " -7 "},

	// Empty input and typed NULL stay TEXT strings, never SQL null.
	{name: "empty text", in: "", wantKind: KindText, wantText: "", boundValue: ""},
	{name: "typed NULL is TEXT", in: "NULL", wantKind: KindText, wantText: "NULL", boundValue: "NULL"},
	{name: "lowercase typed null is TEXT", in: "null", wantKind: KindText, wantText: "null", boundValue: "null"},

	// Injection-looking input is preserved exactly as TEXT.
	{name: "quote injection", in: "' OR '1'='1", wantKind: KindText, wantText: "' OR '1'='1", boundValue: "' OR '1'='1"},
	{name: "statement injection", in: "1; DROP TABLE users--", wantKind: KindText, wantText: "1; DROP TABLE users--", boundValue: "1; DROP TABLE users--"},
}

// TestParseValueClassifiesVerbatimText pins the full classification table of
// the universal text parser.
func TestParseValueClassifiesVerbatimText(t *testing.T) {
	for _, tc := range parseValueCases {
		t.Run(tc.name, func(t *testing.T) {
			v := ParseValue(tc.in)

			if v.Kind != tc.wantKind {
				t.Fatalf("ParseValue(%q).Kind = %v, want %v", tc.in, v.Kind, tc.wantKind)
			}
			switch tc.wantKind {
			case KindText:
				if v.Text != tc.wantText {
					t.Errorf("ParseValue(%q).Text = %q, want exact %q", tc.in, v.Text, tc.wantText)
				}
			case KindInteger:
				if v.Int != tc.wantInt {
					t.Errorf("ParseValue(%q).Int = %d (%s), want %d", tc.in, v.Int, strconv.FormatInt(v.Int, 10), tc.wantInt)
				}
			case KindReal:
				got := math.Float64bits(v.Real)
				if got != tc.wantReal {
					t.Errorf("ParseValue(%q).Real bits = %#x (%g), want %#x (%g)", tc.in, got, v.Real, tc.wantReal, math.Float64frombits(tc.wantReal))
				}
				if v.Real != v.Real || math.IsInf(v.Real, 0) {
					t.Errorf("ParseValue(%q).Real = non-finite %v, want finite", tc.in, v.Real)
				}
			}
		})
	}
}

// TestParseValueParamValues pins the exact dynamic type and value of each
// driver-facing bound parameter; typed NULL and empty input stay strings.
func TestParseValueParamValues(t *testing.T) {
	for _, tc := range parseValueCases {
		t.Run(tc.name, func(t *testing.T) {
			bound := ParseValue(tc.in).ParamValue()

			gotType := reflect.TypeOf(bound)
			wantType := reflect.TypeOf(tc.boundValue)
			if gotType != wantType {
				t.Fatalf("ParseValue(%q).ParamValue() type = %v, want %v", tc.in, gotType, wantType)
			}
			if !reflect.DeepEqual(bound, tc.boundValue) {
				t.Errorf("ParseValue(%q).ParamValue() = %#v, want %#v", tc.in, bound, tc.boundValue)
			}
		})
	}
}
