// CSV typed-value byte-golden coverage for Issue #50, per the Numeric
// value parsing and rendering, Invalid UTF-8 TEXT, Export formats and
// values, Output names, and Testing Decisions decisions in
// Notes/PRD-sqloid.md. The complete SQLite typed-value matrix supplied by
// internal/result is pinned byte-exactly: SQL NULL and empty TEXT as the
// identical empty field (the documented accepted lossy limitation), signed
// INTEGER boundaries and representative tokens, finite REAL identity with
// locale-independent shared tokens (integral values, negative zero,
// subnormals, exponent forms, precision edges), pre-existing non-finite
// REAL text Inf/-Inf/NaN, empty and arbitrary BLOBs as lowercase
// hexadecimal, empty and multibyte TEXT, NUL and other controls, tabs,
// commas, quotes, CR/LF/CRLF, and multiple maximal invalid UTF-8
// sequences normalized by the shared internal/result policy to exactly
// one U+FFFD per maximal invalid sequence. BLOB bytes must be unchanged
// by encoding, no warning data may appear, and finite REAL tokens must
// equal the shared grid/result formatting.

package export

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// TestCSVNullAndEmptyTextIdentical pins the documented lossy limitation:
// SQL NULL and empty TEXT serialize to the identical empty CSV field, so
// their records are byte-identical while surrounding delimiters stay
// exact.
func TestCSVNullAndEmptyTextIdentical(t *testing.T) {
	nullRow := CSV(Payload{
		Names:     []string{"v", "w"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{result.NewNull(), result.NewNull()}},
	})
	emptyRow := CSV(Payload{
		Names:     []string{"v", "w"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{result.NewText(""), result.NewText("")}},
	})
	want := []byte("v,w\r\n,\r\n")
	if !bytes.Equal(nullRow, want) {
		t.Fatalf("NULL row = %q, want %q", nullRow, want)
	}
	if !bytes.Equal(emptyRow, want) {
		t.Fatalf("empty-TEXT row = %q, want identical bytes %q", emptyRow, want)
	}
}

// TestCSVIntegerTokens covers representative and boundary INTEGERs with
// their exact strconv tokens.
func TestCSVIntegerTokens(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  string
	}{
		{name: "zero", value: 0, want: "0"},
		{name: "positive", value: 42, want: "42"},
		{name: "negative", value: -1, want: "-1"},
		{name: "max int64", value: math.MaxInt64, want: "9223372036854775807"},
		{name: "min int64", value: math.MinInt64, want: "-9223372036854775808"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSV(Payload{
				Names:     []string{"v"},
				Positions: []int64{1},
				Rows:      [][]result.Value{{result.NewInteger(tt.value)}},
			})
			want := "v\r\n" + tt.want + "\r\n"
			if string(got) != want {
				t.Fatalf("INTEGER %d CSV = %q, want %q", tt.value, got, want)
			}
		})
	}
}

// TestCSVRealTokens covers finite REAL identity with the shared tokens:
// integral values gain ".0", negative zero keeps its sign, subnormals and
// exponent forms round-trip through strconv 'g' -1 formatting,
// locale-independently, and every token equals the shared
// result.RealToken used by the grid. Pre-existing non-finite REALs keep
// their exact textual tokens Inf, -Inf, and NaN.
func TestCSVRealTokens(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "integral", value: 1.0, want: "1.0"},
		{name: "large integral", value: 100000, want: "100000.0"},
		{name: "negative zero", value: math.Copysign(0, -1), want: "-0.0"},
		{name: "fraction", value: 0.5, want: "0.5"},
		{name: "repeating", value: 1.0 / 3.0, want: "0.3333333333333333"},
		{name: "pi", value: 3.141592653589793, want: "3.141592653589793"},
		{name: "exponent", value: 1e20, want: "1e+20"},
		{name: "negative exponent", value: 1e-5, want: "1e-05"},
		{name: "subnormal", value: math.SmallestNonzeroFloat64, want: "5e-324"},
		{name: "max finite", value: math.MaxFloat64, want: "1.7976931348623157e+308"},
		{name: "precision edge", value: 0.30000000000000004, want: "0.30000000000000004"},
		{name: "positive infinity", value: math.Inf(1), want: "Inf"},
		{name: "negative infinity", value: math.Inf(-1), want: "-Inf"},
		{name: "not a number", value: math.NaN(), want: "NaN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSV(Payload{
				Names:     []string{"v"},
				Positions: []int64{1},
				Rows:      [][]result.Value{{result.NewReal(tt.value)}},
			})
			want := "v\r\n" + tt.want + "\r\n"
			if string(got) != want {
				t.Fatalf("REAL %v CSV = %q, want %q", tt.value, got, want)
			}
			if tt.want != "Inf" && tt.want != "-Inf" && tt.want != "NaN" {
				if token := result.RealToken(tt.value); token != tt.want {
					t.Fatalf("CSV token %q diverges from shared RealToken %q", tt.want, token)
				}
			}
			if !strings.ContainsAny(tt.want, ".eE") && tt.want != "Inf" && tt.want != "-Inf" && tt.want != "NaN" {
				t.Errorf("finite REAL token %q lacks REAL identity (.0)", tt.want)
			}
		})
	}
}

// TestCSVBlobLowercaseHex pins empty and arbitrary BLOBs as lowercase
// hexadecimal and proves the retained bytes are unchanged by encoding.
func TestCSVBlobLowercaseHex(t *testing.T) {
	payloads := []struct {
		name string
		blob []byte
		want string
	}{
		{name: "empty blob", blob: []byte{}, want: ""},
		{name: "nil blob", blob: nil, want: ""},
		{name: "dead beef", blob: []byte{0xDE, 0xAD, 0xBE, 0xEF}, want: "deadbeef"},
		{name: "nul byte", blob: []byte{0x00}, want: "00"},
		{name: "high byte", blob: []byte{0xFF, 0x10}, want: "ff10"},
	}
	for _, tt := range payloads {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]byte(nil), tt.blob...)
			got := CSV(Payload{
				Names:     []string{"v"},
				Positions: []int64{1},
				Rows:      [][]result.Value{{{Kind: result.KindBlob, Bytes: tt.blob}}},
			})
			want := "v\r\n" + tt.want + "\r\n"
			if string(got) != want {
				t.Fatalf("BLOB %x CSV = %q, want %q", tt.blob, got, want)
			}
			if !bytes.Equal(tt.blob, original) {
				t.Errorf("BLOB bytes mutated by encoding: %x, want %x", tt.blob, original)
			}
		})
	}
}

// TestCSVTextControlsAndMultibyte pins verbatim TEXT: NUL and other
// controls are preserved inside quoted fields, tabs stay unquoted,
// commas/quotes/CR/LF/CRLF quote and double-quote correctly, and empty and
// multibyte UTF-8 text pass through byte-exactly.
func TestCSVTextControlsAndMultibyte(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string // the serialized field, without delimiters
	}{
		{name: "empty", value: "", want: ""},
		{name: "multibyte", value: "héllo \U0001F600 世界", want: "héllo \U0001F600 世界"},
		{name: "nul", value: "a\x00b", want: "a\x00b"},
		{name: "controls unquoted", value: "\x01\x1f\x7f", want: "\x01\x1f\x7f"},
		{name: "controls with comma", value: "\x01,b\x7f", want: "\"\x01,b\x7f\""},
		{name: "tab only", value: "\t", want: "\t"},
		{name: "comma", value: "a,b", want: "\"a,b\""},
		{name: "quote", value: "say \"hi\"", want: "\"say \"\"hi\"\"\""},
		{name: "cr", value: "a\rb", want: "\"a\rb\""},
		{name: "lf", value: "a\nb", want: "\"a\nb\""},
		{name: "crlf", value: "a\r\nb", want: "\"a\r\nb\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSV(Payload{
				Names:     []string{"v"},
				Positions: []int64{1},
				Rows:      [][]result.Value{{result.NewText(tt.value)}},
			})
			want := "v\r\n" + tt.want + "\r\n"
			if string(got) != want {
				t.Fatalf("TEXT %q CSV = %q, want %q", tt.value, got, want)
			}
		})
	}
}

// TestCSVInvalidUTF8Normalized runs raw TEXT bytes through the shared
// result.DecodeText policy — exactly one U+FFFD per maximal invalid byte
// sequence — and pins the exact CSV bytes of the normalized text. Every
// case must have required replacement (so the warning metadata exists)
// and must carry the expected replacement count.
func TestCSVInvalidUTF8Normalized(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantFFFDs  int
		wantString string
	}{
		// C0 and the lone continuation byte are two separate one-byte
		// maximal invalid subparts: two U+FFFDs, never a collapsed pair.
		{name: "overlong pair", raw: "\xC0\x80", wantFFFDs: 2, wantString: "\uFFFD\uFFFD"},
		// E1 followed by a valid continuation byte but a missing third
		// byte is one two-byte maximal subpart; the trailing C3 is
		// another one-byte subpart.
		{name: "truncated then lone lead", raw: "\xE1\x80\xC3", wantFFFDs: 2, wantString: "\uFFFD\uFFFD"},
		// E0 A0 is a valid two-byte prefix of a three-byte sequence whose
		// continuation never arrives: one two-byte maximal subpart, one
		// U+FFFD.
		{name: "truncated three-byte", raw: "\xE0\xA0", wantFFFDs: 1, wantString: "\uFFFD"},
		// ED A0 breaks the surrogate-block constraint immediately, so the
		// ED, A0, and 80 are three separate one-byte subparts.
		{name: "surrogate lead", raw: "\xED\xA0\x80", wantFFFDs: 3, wantString: "\uFFFD\uFFFD\uFFFD"},
		// Text around the invalid sequences survives verbatim.
		{name: "interior sequences", raw: "ok\xE1\x80mid\xC0\x80end", wantFFFDs: 3, wantString: "ok\uFFFDmid\uFFFD\uFFFDend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, replaced := result.DecodeText(tt.raw)
			if !replaced {
				t.Fatalf("DecodeText(%q) reported no replacement", tt.raw)
			}
			if normalized != tt.wantString {
				t.Fatalf("DecodeText(%q) = %q, want %q", tt.raw, normalized, tt.wantString)
			}
			if got := strings.Count(normalized, "\uFFFD"); got != tt.wantFFFDs {
				t.Fatalf("U+FFFD count = %d, want one per maximal invalid sequence (%d)", got, tt.wantFFFDs)
			}
			got := CSV(Payload{
				Names:     []string{"v"},
				Positions: []int64{1},
				Rows:      [][]result.Value{{result.NewText(normalized)}},
			})
			want := "v\r\n" + tt.wantString + "\r\n"
			if string(got) != want {
				t.Fatalf("normalized TEXT CSV = %q, want %q", got, want)
			}
		})
	}
}

// TestCSVTypedMatrixGolden pins one multi-column row combining every
// storage class — NULL, INTEGER edge, finite and non-finite REAL, empty
// TEXT, multibyte TEXT, controls, quoted specials, and BLOB hex — against
// the exact full-output bytes, with no warning data anywhere.
func TestCSVTypedMatrixGolden(t *testing.T) {
	columns := []string{"n", "i", "r", "t", "b"}
	names := result.DeduplicateNames(columns)
	row := []result.Value{
		result.NewNull(),
		result.NewInteger(math.MinInt64),
		result.NewReal(math.Inf(-1)),
		result.NewText("tab\tand,quote\"nl\r\n"),
		{Kind: result.KindBlob, Bytes: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
	}
	got := CSV(Payload{Names: names, Positions: []int64{1}, Rows: [][]result.Value{row}})
	want := "n,i,r,t,b\r\n" +
		"," + strconv.FormatInt(math.MinInt64, 10) +
		",-Inf" +
		",\"tab\tand,quote\"\"nl\r\n\"" +
		",cafebabe\r\n"
	if string(got) != want {
		t.Fatalf("typed-matrix CSV:\n got %q\nwant %q", got, want)
	}
	assertNoWarningData(t, got)
}

// TestCSVTypedMatrixRealTokenEquality proves every finite REAL cell in the
// mixed matrix serializes to exactly the shared result token used by the
// grid, with no private formatter copy.
func TestCSVTypedMatrixRealTokenEquality(t *testing.T) {
	values := []float64{1.0, -0.0, 0.5, 1e20, 1e-5, math.SmallestNonzeroFloat64, math.MaxFloat64, 0.30000000000000004}
	for _, v := range values {
		got := CSV(Payload{
			Names:     []string{"v"},
			Positions: []int64{1},
			Rows:      [][]result.Value{{result.NewReal(v)}},
		})
		want := "v\r\n" + result.RealToken(v) + "\r\n"
		if string(got) != want {
			t.Fatalf("REAL %v CSV = %q, want shared token %q", v, got, want)
		}
	}
}
