// Table-driven contract tests for the shared, UI-independent result
// representation seam (Issue #22 Tasks 1–2), per the Output names, Numeric
// value parsing and rendering, Invalid UTF-8 TEXT, and Grid rendering/cache
// decisions in Notes/PRD-sqloid.md. These tests pin the full-set output-name
// deduplication rule, the exact finite REAL token, visible grid
// control-character transformation, maximal invalid UTF-8 replacement with
// warning metadata, and exact BLOB retention/display — the contracts later
// exporters must share rather than copy.
package result

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func negZero() float64 { return math.Copysign(0, -1) }

func addFloats(a, b float64) float64 { return a + b }

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

func TestDeduplicateNames(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "no duplicates unchanged", input: []string{"id", "name", "v"}, want: []string{"id", "name", "v"}},
		{name: "empty set stays empty", input: []string{}, want: []string{}},
		{name: "empty label first occurrence unchanged", input: []string{""}, want: []string{""}},
		{name: "duplicate empty labels get suffixes", input: []string{"", "", ""}, want: []string{"", "_2", "_3"}},
		{name: "simple duplicate gets _2", input: []string{"id", "id"}, want: []string{"id", "id_2"}},
		{name: "triple duplicate counts up", input: []string{"id", "id", "id"}, want: []string{"id", "id_2", "id_3"}},
		{name: "repeated computed labels deduplicate", input: []string{"COUNT(*)", "COUNT(*)"}, want: []string{"COUNT(*)", "COUNT(*)_2"}},
		{
			name:  "pre-suffixed original name blocks colliding suffix",
			input: []string{"v", "v", "v_2"},
			want:  []string{"v", "v_3", "v_2"},
		},
		{
			name:  "later original name blocks earlier duplicate suffix",
			input: []string{"a", "a", "a_2"},
			want:  []string{"a", "a_3", "a_2"},
		},
		{
			name:  "collision chain across the full set",
			input: []string{"c", "c_2", "c", "c_2"},
			want:  []string{"c", "c_2", "c_3", "c_2_2"},
		},
		{
			name:  "duplicate collides with original even when far later",
			input: []string{"x", "x", "a", "b", "x_2"},
			want:  []string{"x", "x_3", "a", "b", "x_2"},
		},
		{
			name:  "two pairs deduplicate independently",
			input: []string{"a", "b", "a", "b"},
			want:  []string{"a", "b", "a_2", "b_2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeduplicateNames(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("DeduplicateNames(%q) = %q, want %q", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("DeduplicateNames(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDeduplicateNamesLeavesOriginalSliceUnchanged(t *testing.T) {
	input := []string{"id", "id"}
	_ = DeduplicateNames(input)
	if input[0] != "id" || input[1] != "id" {
		t.Errorf("input slice mutated in place: %q", input)
	}
}

func TestRealToken(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want string
	}{
		{name: "integral value gets .0", v: 1, want: "1.0"},
		{name: "negative integral value gets .0", v: -1, want: "-1.0"},
		{name: "zero gets .0", v: 0, want: "0.0"},
		{name: "negative zero gets .0", v: negZero(), want: "-0.0"},
		{name: "fractional", v: 0.5, want: "0.5"},
		{name: "negative fractional", v: -12.75, want: "-12.75"},
		{name: "large exponent", v: 1e20, want: "1e+20"},
		{name: "small exponent", v: 1e-20, want: "1e-20"},
		{name: "precision edge shortest round trip", v: addFloats(0.1, 0.2), want: "0.30000000000000004"},
		{name: "max finite", v: 1.7976931348623157e308, want: "1.7976931348623157e+308"},
		{name: "smallest subnormal", v: 5e-324, want: "5e-324"},
		{name: "integer-like with exponent token kept", v: 100, want: "100.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RealToken(tt.v); got != tt.want {
				t.Errorf("RealToken(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestRealTokenLocaleIndependent(t *testing.T) {
	// Thousands separators or decimal commas must never appear; the token is
	// purely strconv output plus the .0 suffix rule.
	for _, v := range []float64{1234.5, 1e6, 0.000123} {
		token := RealToken(v)
		if strings.ContainsAny(token, ",") {
			t.Errorf("RealToken(%v) = %q contains a locale separator", v, token)
		}
	}
}

func TestRealTokenRoundTrips(t *testing.T) {
	for _, v := range []float64{1, -1, 0.5, 1e20, 1e-20, 0.1 + 0.2, 1.7976931348623157e308} {
		parsed, err := parseFloat(RealToken(v))
		if err != nil {
			t.Errorf("RealToken(%v) = %q does not parse: %v", v, RealToken(v), err)
			continue
		}
		if parsed != v {
			t.Errorf("RealToken(%v) = %q parses to %v, not the original", v, RealToken(v), parsed)
		}
	}
}

func TestGridTextTransformation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text unchanged", input: "hello", want: "hello"},
		{name: "tab becomes visible symbol", input: "a\tb", want: "a" + TabSymbol + "b"},
		{name: "newline becomes visible symbol", input: "a\nb", want: "a" + NewlineSymbol + "b"},
		{name: "mixed control characters", input: "a\tb\nc", want: "a" + TabSymbol + "b" + NewlineSymbol + "c"},
		{name: "numeric-looking text stays text", input: "1.0", want: "1.0"},
		{name: "null-keyword text stays text", input: "NULL", want: "NULL"},
		{name: "valid multibyte UTF-8 unchanged", input: "héllo世", want: "héllo世"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GridText(tt.input); got != tt.want {
				t.Errorf("GridText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeTextMaximalInvalidSequences(t *testing.T) {
	const fffd = string(rune(0xFFFD))
	tests := []struct {
		name       string
		input      string
		want       string
		wantReplac bool // invalid UTF-8 required replacement
	}{
		{
			name:       "valid text passes through unchanged",
			input:      "hello héllo世",
			want:       "hello héllo世",
			wantReplac: false,
		},
		{
			name:       "lone continuation byte is one maximal invalid sequence",
			input:      "a\x80b",
			want:       "a" + fffd + "b",
			wantReplac: true,
		},
		{
			name:       "truncated two-byte sequence is one maximal invalid sequence",
			input:      "\xC3",
			want:       fffd,
			wantReplac: true,
		},
		{
			name:       "bad first continuation is one sequence per subpart",
			input:      "\xE0\x28\x80",
			want:       fffd + "(" + fffd,
			wantReplac: true,
		},
		{
			name:       "overlong pair replaced per subpart",
			input:      "\xC0\x80",
			want:       fffd + fffd,
			wantReplac: true,
		},
		{
			name:       "four-byte lead with bad first continuation",
			input:      "\xF0\x80\x80\x80",
			want:       fffd + fffd + fffd + fffd,
			wantReplac: true,
		},
		{
			name:       "maximal subpart consumed before valid tail",
			input:      "\xE0\xA0" + "ok",
			want:       fffd + "ok",
			wantReplac: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replaced := DecodeText(tt.input)
			if got != tt.want {
				t.Errorf("DecodeText(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if replaced != tt.wantReplac {
				t.Errorf("DecodeText(%q) replaced = %v, want %v", tt.input, replaced, tt.wantReplac)
			}
		})
	}
}

func TestDisplayTypedCellValues(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want string
	}{
		{name: "null", v: NewNull(), want: NullDisplay},
		{name: "integer", v: NewInteger(42), want: "42"},
		{name: "negative integer", v: NewInteger(-7), want: "-7"},
		{name: "real uses exact token", v: NewReal(1), want: "1.0"},
		{name: "real negative zero", v: NewReal(negZero()), want: "-0.0"},
		{name: "real exponent", v: NewReal(1e20), want: "1e+20"},
		{name: "text verbatim after transformation", v: NewText("a\nb"), want: "a" + NewlineSymbol + "b"},
		{name: "text that looks numeric stays text verbatim", v: NewText("1.0"), want: "1.0"},
		{name: "blob placeholder", v: NewBlob([]byte{0x00, 0xFF}), want: "[BLOB 2 bytes]"},
		{name: "empty blob placeholder", v: NewBlob(nil), want: "[BLOB 0 bytes]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Display(); got != tt.want {
				t.Errorf("Display() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNonFiniteRealDisplayTokens(t *testing.T) {
	nanQuiet := math.NaN()
	nanPayload := math.Float64frombits(math.Float64bits(nanQuiet) | 0x0007_5000_0000_0001)
	nanNegative := math.Float64frombits(math.Float64bits(nanQuiet) | 0x8000_0000_0000_0001)
	tests := []struct {
		name string
		v    Value
		want string
	}{
		{name: "positive infinity", v: NewReal(math.Inf(1)), want: "Inf"},
		{name: "negative infinity", v: NewReal(math.Inf(-1)), want: "-Inf"},
		{name: "quiet NaN", v: NewReal(nanQuiet), want: "NaN"},
		{name: "NaN payload renders same token", v: NewReal(nanPayload), want: "NaN"},
		{name: "negative NaN renders same token", v: NewReal(nanNegative), want: "NaN"},
		{name: "finite REAL keeps exact token", v: NewReal(1), want: "1.0"},
		{name: "finite REAL exponent keeps exact token", v: NewReal(1e20), want: "1e+20"},
		{name: "TEXT Inf stays verbatim text", v: NewText("Inf"), want: "Inf"},
		{name: "TEXT -Inf stays verbatim text", v: NewText("-Inf"), want: "-Inf"},
		{name: "TEXT NaN stays verbatim text", v: NewText("NaN"), want: "NaN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Display(); got != tt.want {
				t.Errorf("Display() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNonFiniteRealTypeAndValueRetained(t *testing.T) {
	rows := [][]any{
		{math.Inf(1), math.Inf(-1), math.NaN(), math.Float64frombits(math.Float64bits(math.NaN()) | 0x000A_5000_0000_0002), 1.5, "Inf", "NaN"},
	}
	page := FromDriver([]string{"pos", "neg", "nan", "nan2", "finite", "t1", "t2"}, rows)
	row := page.Rows[0]

	wantTokens := []string{"Inf", "-Inf", "NaN", "NaN", "1.5", "Inf", "NaN"}
	for i, want := range wantTokens {
		if got := row[i].Display(); got != want {
			t.Errorf("cell %d display = %q, want %q", i, got, want)
		}
	}
	// REAL cells keep their kind and exact float64 backing value even when
	// the token differs; TEXT cells keep their kind despite matching glyphs.
	for i, wantFloat := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), math.Float64frombits(math.Float64bits(math.NaN()) | 0x000A_5000_0000_0002), 1.5} {
		if row[i].Kind != KindReal {
			t.Errorf("cell %d kind = %d, want KindReal", i, row[i].Kind)
		}
		if math.IsNaN(wantFloat) {
			if !math.IsNaN(row[i].Float) || math.Float64bits(row[i].Float) != math.Float64bits(wantFloat) {
				t.Errorf("cell %d NaN bits changed: %#x, want %#x", i, math.Float64bits(row[i].Float), math.Float64bits(wantFloat))
			}
		} else if row[i].Float != wantFloat {
			t.Errorf("cell %d float = %v, want %v", i, row[i].Float, wantFloat)
		}
	}
	for i, wantStr := range []string{"Inf", "NaN"} {
		c := row[5+i]
		if c.Kind != KindText || c.Str != wantStr {
			t.Errorf("cell %d = (%d, %q), want text %q", 5+i, c.Kind, c.Str, wantStr)
		}
	}
	if page.InvalidUTF {
		t.Errorf("non-finite REALs set invalid-UTF metadata")
	}
}

func TestTypeIdentityPreserved(t *testing.T) {
	// INTEGER 1, REAL 1.0, and TEXT "1.0" must remain three distinct typed
	// values even though their tokens overlap.
	intVal := NewInteger(1)
	realVal := NewReal(1)
	textVal := NewText("1.0")
	if intVal.Display() != "1" {
		t.Errorf("INTEGER 1 displays as %q, want \"1\"", intVal.Display())
	}
	if realVal.Display() != "1.0" {
		t.Errorf("REAL 1.0 displays as %q, want \"1.0\"", realVal.Display())
	}
	if textVal.Display() != "1.0" {
		t.Errorf("TEXT \"1.0\" displays as %q, want \"1.0\"", textVal.Display())
	}
	if intVal.Kind != KindInteger || realVal.Kind != KindReal || textVal.Kind != KindText {
		t.Errorf("kinds lost: %d %d %d", intVal.Kind, realVal.Kind, textVal.Kind)
	}
	// A REAL and the TEXT that looks identical must be distinguishable by
	// kind, and the render seam must never coerce one into the other.
	if realVal.Kind == textVal.Kind {
		t.Error("REAL and identical-looking TEXT collapsed into one kind")
	}
}

func TestBlobBytesRetainedExactly(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xFF, 0xFE, 0xC3, 0x28, 0xE0, 0x80, 0x80}
	v := NewBlob(payload)
	if len(v.Bytes) != len(payload) {
		t.Fatalf("blob length = %d, want %d", len(v.Bytes), len(payload))
	}
	for i := range payload {
		if v.Bytes[i] != payload[i] {
			t.Fatalf("blob byte %d = %#x, want %#x", i, v.Bytes[i], payload[i])
		}
	}
	// Display never depends on the bytes and never leaks them.
	if got := v.Display(); got != "[BLOB 9 bytes]" {
		t.Errorf("blob display = %q, want \"[BLOB 9 bytes]\"", got)
	}
	// The value owns its bytes: mutating the caller's slice afterwards must
	// not alter the retained payload.
	payload[0] = 0x7F
	if v.Bytes[0] != 0x00 {
		t.Error("retained blob bytes alias caller storage")
	}
}

func TestBlobNeverDecodedAsText(t *testing.T) {
	// Invalid UTF-8 in a BLOB must not set invalid-UTF metadata or transform
	// display: BLOB handling is unchanged per the PRD.
	v := NewBlob([]byte{0xE0, 0x80, 0x80})
	if got := v.Display(); got != "[BLOB 3 bytes]" {
		t.Errorf("blob display = %q, want \"[BLOB 3 bytes]\"", got)
	}
}

func TestPageFromDriverTypedRows(t *testing.T) {
	rows := [][]any{
		{nil, int64(1), 1.5, "text", []byte{0xAB}},
		{nil, int64(-2), -0.5, "line\nbreak", nil},
	}
	page := FromDriver([]string{"id", "id", "ratio", "note", "data"}, rows)

	if len(page.Rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(page.Rows))
	}
	if page.Rows[0][0].Kind != KindNull {
		t.Errorf("NULL scanned as kind %d, want %d", page.Rows[0][0].Kind, KindNull)
	}
	if page.Rows[0][1].Kind != KindInteger || page.Rows[0][1].Int != 1 {
		t.Errorf("INTEGER scanned as %+v, want Integer 1", page.Rows[0][1])
	}
	if page.Rows[0][2].Kind != KindReal || page.Rows[0][2].Float != 1.5 {
		t.Errorf("REAL scanned as %+v, want Real 1.5", page.Rows[0][2])
	}
	if page.Rows[0][3].Kind != KindText || page.Rows[0][3].Str != "text" {
		t.Errorf("TEXT scanned as %+v, want Text text", page.Rows[0][3])
	}
	if page.Rows[0][4].Kind != KindBlob || string(page.Rows[0][4].Bytes) != string([]byte{0xAB}) {
		t.Errorf("BLOB scanned as %+v, want exact byte", page.Rows[0][4])
	}
	if page.InvalidUTF {
		t.Error("valid UTF-8 rows reported invalid-UTF metadata")
	}
	// Driver labels stay original; deduplication is a separate step.
	if page.Columns[0] != "id" || page.Columns[1] != "id" {
		t.Errorf("original labels lost: %q", page.Columns)
	}
}

func TestPageFromDriverCopiesBlobBytes(t *testing.T) {
	raw := []byte{0x01, 0x02}
	page := FromDriver([]string{"b"}, [][]any{{raw}})
	raw[0] = 0x99
	if page.Rows[0][0].Bytes[0] != 0x01 {
		t.Error("driver scan bytes aliased into result value")
	}
}

func TestPageInvalidUTFMetadataWithoutRowChange(t *testing.T) {
	rows := [][]any{
		{"ok", int64(1)},
		{"bad\xE0\x80\x80", int64(2)},
		{"fine", int64(3)},
	}
	page := FromDriver([]string{"t", "n"}, rows)
	if !page.InvalidUTF {
		t.Error("invalid UTF-8 TEXT did not set warning metadata")
	}
	if len(page.Rows) != 3 {
		t.Errorf("row count = %d after replacement, want unchanged 3", len(page.Rows))
	}
	for _, row := range page.Rows {
		if len(row) != 2 {
			t.Errorf("column count = %d after replacement, want unchanged 2", len(row))
		}
	}
	if got, _ := DecodeText("bad\xE0\x80\x80"); page.Rows[1][0].Str != got {
		t.Errorf("TEXT cell = %q, want decoded %q", page.Rows[1][0].Str, got)
	}
}

func TestPageFromDriverRejectsUnsupportedValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("unsupported driver value did not panic")
		}
	}()
	FromDriver([]string{"x"}, [][]any{{true}})
}

func TestHeaderDeduplicatedNames(t *testing.T) {
	page := FromDriver([]string{"COUNT(*)", "COUNT(*)", "id", "id"}, [][]any{})
	got := page.HeaderNames()
	want := []string{"COUNT(*)", "COUNT(*)_2", "id", "id_2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("HeaderNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
