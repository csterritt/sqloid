// JSON typed-value byte-golden coverage for Issue #51, per the Numeric
// value parsing and rendering, Invalid UTF-8 TEXT, Export formats and
// values, Output names, and Testing Decisions decisions in
// Notes/PRD-sqloid.md. The complete SQLite typed-value matrix supplied by
// internal/result is pinned byte-exactly and by parsed JSON type: INTEGER
// (signed 64-bit boundaries) and every finite REAL as raw unquoted number
// tokens using the shared locale-independent formatting with REAL-identity
// ".0" rules, pre-existing non-finite REALs as the exact quoted strings
// "Inf", "-Inf", and "NaN", SQL NULL as the JSON literal null, empty and
// nonempty TEXT as distinct escaped JSON strings, and empty/arbitrary BLOB
// bytes as standard base64 JSON strings whose source bytes are unchanged.
// Invalid UTF-8 sequences were already normalized by the shared
// internal/result policy to exactly one U+FFFD per maximal invalid
// sequence. Every Issue #49 warning combination must leave the bytes
// unchanged, and finite REAL tokens must equal the shared grid/CSV
// formatting exactly.

package export

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// jsonOneValue serializes a single-column, single-row payload carrying v
// and returns the exact bytes plus the column name "v".
func jsonOneValue(t *testing.T, v result.Value) []byte {
	t.Helper()
	return JSON(Payload{
		Names:     []string{"v"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{v}},
	})
}

// TestJSONNullAndEmptyTextDistinct pins the typed distinction CSV loses:
// SQL NULL emits the JSON literal null while empty TEXT emits the empty
// JSON string "".
func TestJSONNullAndEmptyTextDistinct(t *testing.T) {
	nullGot := jsonOneValue(t, result.NewNull())
	if want := []byte(`[{"v":null}]`); !bytes.Equal(nullGot, want) {
		t.Fatalf("NULL JSON = %q, want %q", nullGot, want)
	}
	emptyGot := jsonOneValue(t, result.NewText(""))
	if want := []byte(`[{"v":""}]`); !bytes.Equal(emptyGot, want) {
		t.Fatalf("empty-TEXT JSON = %q, want %q", emptyGot, want)
	}
	if bytes.Equal(nullGot, emptyGot) {
		t.Fatal("NULL and empty TEXT must serialize to distinct JSON")
	}
	var parsed []map[string]any
	if err := json.Unmarshal(nullGot, &parsed); err != nil {
		t.Fatalf("NULL output is not valid JSON: %v", err)
	}
	if v, ok := parsed[0]["v"]; !ok || v != nil {
		t.Errorf("parsed NULL = %#v, want JSON null", v)
	}
	if err := json.Unmarshal(emptyGot, &parsed); err != nil {
		t.Fatalf("empty-TEXT output is not valid JSON: %v", err)
	}
	if v, ok := parsed[0]["v"].(string); !ok || v != "" {
		t.Errorf("parsed empty TEXT = %#v, want empty string", v)
	}
}

// TestJSONIntegerRawTokens covers representative and boundary INTEGERs as
// raw unquoted number tokens with their exact strconv tokens, including
// the signed 64-bit boundaries.
func TestJSONIntegerRawTokens(t *testing.T) {
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
			got := jsonOneValue(t, result.NewInteger(tt.value))
			want := `[{"v":` + tt.want + `}]`
			if string(got) != want {
				t.Fatalf("INTEGER %d JSON = %q, want %q", tt.value, got, want)
			}
			var parsed []map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			if f, ok := parsed[0]["v"].(float64); !ok || f != float64(tt.value) {
				t.Errorf("parsed value = %#v, want %d", parsed[0]["v"], tt.value)
			}
		})
	}
}

// TestJSONFiniteRealRawTokens covers finite REAL identity as raw unquoted
// number tokens: integral values keep ".0", negative zero keeps its sign,
// subnormals and exponent forms use the shortest round-trip 'g' -1 token,
// and every token equals the shared result.RealToken used by the grid and
// CSV, locale-independently.
func TestJSONFiniteRealRawTokens(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonOneValue(t, result.NewReal(tt.value))
			want := `[{"v":` + tt.want + `}]`
			if string(got) != want {
				t.Fatalf("REAL %v JSON = %q, want %q", tt.value, got, want)
			}
			if token := result.RealToken(tt.value); token != tt.want {
				t.Fatalf("JSON token %q diverges from shared RealToken %q", tt.want, token)
			}
			// The token must round-trip to the identical float64.
			parsed, err := strconv.ParseFloat(tt.want, 64)
			if err != nil || math.Float64bits(parsed) != math.Float64bits(tt.value) {
				t.Errorf("token %q does not round-trip: %v, %v", tt.want, parsed, err)
			}
		})
	}
}

// TestJSONNonFiniteRealQuotedTokens pins pre-existing non-finite REALs to
// the exact quoted JSON strings "Inf", "-Inf", and "NaN" — never a raw
// number and never strconv's +Inf spelling.
func TestJSONNonFiniteRealQuotedTokens(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "positive infinity", value: math.Inf(1), want: `"Inf"`},
		{name: "negative infinity", value: math.Inf(-1), want: `"-Inf"`},
		{name: "not a number", value: math.NaN(), want: `"NaN"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonOneValue(t, result.NewReal(tt.value))
			want := `[{"v":` + tt.want + `}]`
			if string(got) != want {
				t.Fatalf("non-finite REAL JSON = %q, want %q", got, want)
			}
			var parsed []map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			if s, ok := parsed[0]["v"].(string); !ok || s != strings.Trim(tt.want, `"`) {
				t.Errorf("parsed value = %#v, want %q", parsed[0]["v"], strings.Trim(tt.want, `"`))
			}
		})
	}
}

// TestJSONBlobBase64 pins empty, nil, and arbitrary BLOBs to standard
// base64 JSON strings and proves the retained bytes are unchanged by
// encoding.
func TestJSONBlobBase64(t *testing.T) {
	payloads := []struct {
		name string
		blob []byte
	}{
		{name: "empty blob", blob: []byte{}},
		{name: "nil blob", blob: nil},
		{name: "dead beef", blob: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
		{name: "nul byte", blob: []byte{0x00}},
		{name: "high bytes", blob: []byte{0xFF, 0x10, 0x7F}},
	}
	for _, tt := range payloads {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]byte(nil), tt.blob...)
			got := jsonOneValue(t, result.Value{Kind: result.KindBlob, Bytes: tt.blob})
			want := `[{"v":"` + base64.StdEncoding.EncodeToString(tt.blob) + `"}]`
			if string(got) != want {
				t.Fatalf("BLOB %x JSON = %q, want %q", tt.blob, got, want)
			}
			var parsed []map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			decoded, err := base64.StdEncoding.DecodeString(parsed[0]["v"].(string))
			if err != nil || !bytes.Equal(decoded, tt.blob) {
				t.Errorf("decoded BLOB = %x (%v), want %x", decoded, err, tt.blob)
			}
			if !bytes.Equal(tt.blob, original) {
				t.Errorf("BLOB bytes mutated by encoding: %x, want %x", tt.blob, original)
			}
		})
	}
}

// TestJSONTextEscaping pins exact TEXT string escaping: quotes, reverse
// solidus, tab, CR, LF, other controls (short forms and \u00XX), solidus
// verbatim, empty and multibyte Unicode, and decoded round-trip equality.
func TestJSONTextEscaping(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string // the escaped JSON string token, including quotes
	}{
		{name: "empty", value: "", want: `""`},
		{name: "quote", value: `say "hi"`, want: `"say \"hi\""`},
		{name: "reverse solidus", value: `back\slash`, want: `"back\\slash"`},
		{name: "solidus verbatim", value: "a/b", want: `"a/b"`},
		{name: "tab", value: "tab\there", want: `"tab\there"`},
		{name: "cr", value: "a\rb", want: `"a\rb"`},
		{name: "lf", value: "a\nb", want: `"a\nb"`},
		{name: "crlf", value: "a\r\nb", want: `"a\r\nb"`},
		{name: "backspace formfeed", value: "a\bf\fc", want: `"a\bf\fc"`},
		{name: "other controls", value: "a\x00\x01\x1f\x7f", want: `"a\u0000\u0001\u001f` + "\x7f" + `"`},
		{name: "multibyte", value: "héllo \U0001F600 世界", want: `"héllo 😀 世界"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonOneValue(t, result.NewText(tt.value))
			want := `[{"v":` + tt.want + `}]`
			if string(got) != want {
				t.Fatalf("TEXT %q JSON = %q, want %q", tt.value, got, want)
			}
			var parsed []map[string]any
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			if decoded := parsed[0]["v"]; decoded != tt.value {
				t.Errorf("decoded TEXT = %#v, want %#v", decoded, tt.value)
			}
		})
	}
}

// TestJSONInvalidUTF8Normalized runs raw TEXT bytes through the shared
// result.DecodeText policy — exactly one U+FFFD per maximal invalid byte
// sequence — and pins the exact JSON bytes of the normalized text.
func TestJSONInvalidUTF8Normalized(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantFFFDs  int
		wantString string
	}{
		{name: "overlong pair", raw: "\xC0\x80", wantFFFDs: 2, wantString: "\uFFFD\uFFFD"},
		{name: "truncated then lone lead", raw: "\xE1\x80\xC3", wantFFFDs: 2, wantString: "\uFFFD\uFFFD"},
		{name: "truncated three-byte", raw: "\xE0\xA0", wantFFFDs: 1, wantString: "\uFFFD"},
		{name: "surrogate lead", raw: "\xED\xA0\x80", wantFFFDs: 3, wantString: "\uFFFD\uFFFD\uFFFD"},
		{name: "interior sequences", raw: "ok\xE1\x80mid\xC0\x80end", wantFFFDs: 3, wantString: "ok\uFFFDmid\uFFFD\uFFFDend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, replaced := result.DecodeText(tt.raw)
			if !replaced {
				t.Fatalf("DecodeText(%q) reported no replacement", tt.raw)
			}
			if got := strings.Count(normalized, "\uFFFD"); got != tt.wantFFFDs {
				t.Fatalf("U+FFFD count = %d, want one per maximal invalid sequence (%d)", got, tt.wantFFFDs)
			}
			got := jsonOneValue(t, result.NewText(normalized))
			want := `[{"v":"` + tt.wantString + `"}]`
			if string(got) != want {
				t.Fatalf("normalized TEXT JSON = %q, want %q", got, want)
			}
		})
	}
}

// TestJSONTypedMatrixGolden pins one multi-column row combining every
// storage class — NULL, INTEGER edge, finite and non-finite REAL, empty
// TEXT, multibyte TEXT, controls, quoted specials, and BLOB base64 —
// against the exact full-output bytes and parsed types, with no warning
// data anywhere.
func TestJSONTypedMatrixGolden(t *testing.T) {
	columns := []string{"n", "i", "r", "r2", "t", "b"}
	names := result.DeduplicateNames(columns)
	row := []result.Value{
		result.NewNull(),
		result.NewInteger(math.MinInt64),
		result.NewReal(1e20),
		result.NewReal(math.Inf(-1)),
		result.NewText("tab\tand,quote\"nl\r\n"),
		{Kind: result.KindBlob, Bytes: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
	}
	got := JSON(Payload{Names: names, Positions: []int64{1}, Rows: [][]result.Value{row}})
	want := `[{"n":null,` +
		`"i":` + strconv.FormatInt(math.MinInt64, 10) +
		`,"r":1e+20` +
		`,"r2":"-Inf"` +
		`,"t":"tab\tand,quote\"nl\r\n"` +
		`,"b":"yv66vg=="}]`
	if string(got) != want {
		t.Fatalf("typed-matrix JSON:\n got %q\nwant %q", got, want)
	}
	assertNoWarningData(t, got)
	var arr []map[string]any
	if err := json.Unmarshal(got, &arr); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("parsed array length = %d, want 1", len(arr))
	}
	if v, ok := arr[0]["n"]; !ok || v != nil {
		t.Errorf("parsed NULL = %#v, want JSON null", v)
	}
	if v, ok := arr[0]["i"].(float64); !ok || v != float64(math.MinInt64) {
		t.Errorf("parsed INTEGER = %#v, want %d", arr[0]["i"], int64(math.MinInt64))
	}
	if v, ok := arr[0]["r"].(float64); !ok || v != 1e20 {
		t.Errorf("parsed REAL = %#v, want 1e+20", arr[0]["r"])
	}
	if v, ok := arr[0]["r2"].(string); !ok || v != "-Inf" {
		t.Errorf("parsed non-finite REAL = %#v, want \"-Inf\"", arr[0]["r2"])
	}
	if v, ok := arr[0]["t"].(string); !ok || v != "tab\tand,quote\"nl\r\n" {
		t.Errorf("parsed TEXT = %#v", arr[0]["t"])
	}
	decoded, err := base64.StdEncoding.DecodeString(arr[0]["b"].(string))
	if err != nil || !bytes.Equal(decoded, []byte{0xCA, 0xFE, 0xBA, 0xBE}) {
		t.Errorf("decoded BLOB = %x (%v), want cafebabe", decoded, err)
	}
}

// TestJSONTypedMatrixRealTokenEquality proves every finite REAL cell in
// the mixed matrix serializes to exactly the shared result token used by
// the grid and CSV, with no private formatter copy.
func TestJSONTypedMatrixRealTokenEquality(t *testing.T) {
	values := []float64{1.0, -0.0, 0.5, 1e20, 1e-5, math.SmallestNonzeroFloat64, math.MaxFloat64, 0.30000000000000004}
	for _, v := range values {
		got := jsonOneValue(t, result.NewReal(v))
		want := `[{"v":` + result.RealToken(v) + `}]`
		if string(got) != want {
			t.Fatalf("REAL %v JSON = %q, want shared token %q", v, got, want)
		}
	}
}

// TestJSONWarningCombinationsTyped runs the same Issue #49 metadata
// warning combinations over typed (non-TEXT-only) data and proves the
// exact JSON bytes are unchanged: no warning object, property, key,
// wrapper, or byte may appear.
func TestJSONWarningCombinationsTyped(t *testing.T) {
	columns := []string{"n", "i", "r", "t", "b"}
	rows := [][]result.Value{{
		result.NewNull(),
		result.NewInteger(math.MaxInt64),
		result.NewReal(math.Inf(1)),
		result.NewText("héllo ☃"),
		{Kind: result.KindBlob, Bytes: []byte{0x01, 0x00, 0xFF}},
	}}

	baseline := JSON(CaptureRows(columns, rows, 1, true,
		history.SnapshotMetadata{}, history.Completeness{Complete: true}).Payload)

	complete := []history.Completeness{
		{Complete: true},
		{Partial: true},
		{Truncated: true},
		{Partial: true, Truncated: true},
	}
	outcomes := []history.SnapshotMetadata{
		{Outcome: history.OutcomeSuccess},
		{Outcome: history.OutcomeCancelled, Reason: "interrupted at row 2"},
		{Outcome: history.OutcomeFailed, Reason: "disk I/O error"},
		{Outcome: history.OutcomeCancelled},
	}

	count := 0
	for _, comp := range complete {
		for _, meta := range outcomes {
			for _, flags := range []history.SnapshotMetadata{
				{},
				{HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 3},
				{TruncatedByByteCap: true},
				{RowCapEvicted: true, RowCapEvictions: 7},
				{InvalidUTF: true},
			} {
				count++
				combined := meta
				combined.HasRetainedRange = flags.HasRetainedRange
				combined.RetainedStart = flags.RetainedStart
				combined.RetainedEnd = flags.RetainedEnd
				combined.TruncatedByByteCap = flags.TruncatedByByteCap
				combined.RowCapEvicted = flags.RowCapEvicted
				combined.RowCapEvictions = flags.RowCapEvictions
				combined.InvalidUTF = flags.InvalidUTF

				captured := CaptureRows(columns, rows, 1, true, combined, comp)
				got := JSON(captured.Payload)
				if !bytes.Equal(got, baseline) {
					t.Fatalf("metadata %+v / completeness %s changed the JSON:\n got %q\nwant %q",
						combined, comp, got, baseline)
				}
				assertNoWarningData(t, got)
			}
		}
	}
	if count == 0 {
		t.Fatal("no warning combinations exercised")
	}
}
