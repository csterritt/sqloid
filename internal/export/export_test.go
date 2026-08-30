// Consumer-contract tests for the exporter-facing internal/export boundary
// (Issue #47 Tasks 1, 3, and 4), per the Output names, Numeric value parsing
// and rendering, Invalid UTF-8 TEXT, Grid rendering/cache, and Export formats
// and values decisions in Notes/PRD-sqloid.md. They pin that an exporter
// receives exactly the same full-set deduplicated output names the frozen
// grid receives, the exact finite-REAL tokens with retained REAL identity,
// maximal-invalid-UTF-8 TEXT normalization with warning metadata, exact BLOB
// byte identity, and the NULL/empty-TEXT/empty-BLOB distinctions — all
// without obtaining the shared values ever mutating driver metadata, stored
// values, or generated SQL. CSV/JSON serialization itself is owned by Issues
// #50/#51 and deliberately absent here.

package export

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func negZero() float64 { return math.Copysign(0, -1) }

func addFloats(a, b float64) float64 { return a + b }

// TestExporterOutputNamesMatchFullSetRule pins one full-set, deterministic
// output-name calculation for empty, duplicate, pre-suffixed, and
// recursively colliding labels, received in column order.
func TestExporterOutputNamesMatchFullSetRule(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   []string
	}{
		{
			name:   "empty and duplicate labels",
			labels: []string{"", "id", "id", ""},
			want:   []string{"", "id", "id_2", "_2"},
		},
		{
			name:   "pre-suffixed label blocks its suffix",
			labels: []string{"v", "v", "v_2"},
			want:   []string{"v", "v_3", "v_2"},
		},
		{
			name:   "recursive collision chain",
			labels: []string{"c", "c_2", "c", "c_2", "c_3"},
			want:   []string{"c", "c_2", "c_4", "c_2_2", "c_3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]string(nil), tt.labels...)
			page := result.FromDriver(tt.labels, nil)
			got := OutputNames(page)
			if len(got) != len(tt.want) {
				t.Fatalf("OutputNames(%q) = %q, want %q", tt.labels, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("OutputNames(%q)[%d] = %q, want %q", tt.labels, i, got[i], tt.want[i])
				}
			}
			// The shared calculation must not write deduplicated labels back
			// into driver column metadata.
			for i := range original {
				if page.Columns[i] != original[i] {
					t.Errorf("driver metadata mutated at %d: %q, want original %q", i, page.Columns[i], original[i])
				}
			}
		})
	}
}

// TestExporterNamesMatchGridNames proves the grid-facing and exporter-facing
// consumers receive exactly the same final names in column order from one
// full-set calculation.
func TestExporterNamesMatchGridNames(t *testing.T) {
	page := result.FromDriver([]string{"COUNT(*)", "COUNT(*)", "", "v", "v", "v_2"}, [][]any{
		{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6)},
	})
	exporterNames := OutputNames(page)
	gridNames := page.HeaderNames()
	if len(exporterNames) != len(gridNames) {
		t.Fatalf("exporter %q and grid %q name counts differ", exporterNames, gridNames)
	}
	for i := range gridNames {
		if exporterNames[i] != gridNames[i] {
			t.Errorf("name %d: exporter %q, grid %q — consumers must share one final name set", i, exporterNames[i], gridNames[i])
		}
	}
	want := []string{"COUNT(*)", "COUNT(*)_2", "", "v", "v_3", "v_2"}
	for i := range want {
		if exporterNames[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, exporterNames[i], want[i])
		}
	}
}

// TestExporterFiniteRealTokens pins the exact locale-independent finite-REAL
// tokens across integral, negative-zero, exponent, subnormal, and adjacent
// precision-edge values, with REAL identity retained.
func TestExporterFiniteRealTokens(t *testing.T) {
	belowOne := math.Nextafter(1, 0)
	aboveOne := math.Nextafter(1, 2)
	belowMax := math.Nextafter(math.MaxFloat64, 0)
	tests := []struct {
		name  string
		v     float64
		token string
	}{
		{name: "integral gets .0", v: 1, token: "1.0"},
		{name: "negative integral gets .0", v: -100, token: "-100.0"},
		{name: "negative zero keeps sign", v: negZero(), token: "-0.0"},
		{name: "large exponent", v: 1e20, token: "1e+20"},
		{name: "smallest subnormal", v: math.Float64frombits(1), token: "5e-324"},
		{name: "largest subnormal", v: math.Nextafter(2.2250738585072014e-308, 0), token: "2.225073858507201e-308"},
		{name: "precision edge just below one", v: belowOne, token: "0.9999999999999999"},
		{name: "precision edge just above one", v: aboveOne, token: "1.0000000000000002"},
		{name: "precision edge below max", v: belowMax, token: "1.7976931348623155e+308"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CellToken(result.NewReal(tt.v)); got != tt.token {
				t.Errorf("CellToken(%v) = %q, want %q", tt.v, got, tt.token)
			}
			// The token must round-trip and never carry a locale separator.
			parsed, err := strconv.ParseFloat(tt.token, 64)
			if err != nil || math.Float64bits(parsed) != math.Float64bits(tt.v) {
				t.Errorf("token %q does not round-trip bit-exactly: %v %v", tt.token, parsed, err)
			}
			if strings.ContainsAny(tt.token, ", ;") {
				t.Errorf("token %q carries a locale separator", tt.token)
			}
		})
	}
}

// TestExporterRealIdentityRetained asserts REAL identity is retained even
// when the canonical token requires ".0" or preserves negative zero, and
// identical-looking INTEGER/TEXT values remain typed distinctly.
func TestExporterRealIdentityRetained(t *testing.T) {
	rows := [][]any{
		{1.0, negZero(), 1e20, int64(1), "1.0"},
	}
	page := result.FromDriver([]string{"r1", "r2", "r3", "i", "t"}, rows)
	if got := CellToken(page.Rows[0][0]); got != "1.0" {
		t.Errorf("REAL 1.0 token = %q, want \"1.0\"", got)
	}
	if got := CellToken(page.Rows[0][1]); got != "-0.0" {
		t.Errorf("REAL -0.0 token = %q, want \"-0.0\"", got)
	}
	if page.Rows[0][0].Kind != result.KindReal || page.Rows[0][0].Float != 1 {
		t.Errorf("REAL 1.0 lost identity: %+v", page.Rows[0][0])
	}
	if page.Rows[0][1].Kind != result.KindReal || math.Float64bits(page.Rows[0][1].Float) != math.Float64bits(negZero()) {
		t.Errorf("REAL -0.0 lost identity: %+v", page.Rows[0][1])
	}
	if page.Rows[0][2].Kind != result.KindReal || page.Rows[0][2].Float != 1e20 {
		t.Errorf("REAL 1e+20 lost identity: %+v", page.Rows[0][2])
	}
	kinds := []result.Kind{page.Rows[0][3].Kind, page.Rows[0][0].Kind, page.Rows[0][4].Kind}
	if kinds[0] != result.KindInteger || kinds[2] != result.KindText {
		t.Errorf("identical-looking values collapsed: kinds %v", kinds)
	}
	tokens := []string{CellToken(page.Rows[0][3]), CellToken(page.Rows[0][0]), CellToken(page.Rows[0][4])}
	if tokens[0] != "1" || tokens[1] != "1.0" || tokens[2] != "1.0" {
		t.Errorf("typed tokens diverged: %q", tokens)
	}
}

// TestExporterTypedNormalizationAndMetadata covers maximal invalid UTF-8
// sequences, warning metadata, exact BLOB bytes, NULL/empty distinctions,
// and retained controls in normalized TEXT.
func TestExporterTypedNormalizationAndMetadata(t *testing.T) {
	const fffd = string(rune(0xFFFD))
	rows := [][]any{
		// One U+FFFD per maximal invalid sequence: isolated continuation,
		// truncated multibyte prefix, overlong encoding, surrogate encoding,
		// and adjacent invalid runs.
		{"a\x80b", "\xC3", "\xC0\x80", "\xED\xA0\x80", "\xF0\x80\x80\x80z", "tab\tnew\nret\rnul\x00ctl"},
		{[]byte("a\x80b"), []byte{0xE0, 0x80, 0x80}, []byte{0x00, 0xFF}, []byte(nil), "", nil},
	}
	page := result.FromDriver([]string{"t1", "t2", "t3", "t4", "t5", "t6"}, rows)
	if !page.InvalidUTF {
		t.Error("invalid UTF-8 TEXT did not set warning metadata")
	}
	if page.Rows[0][0].Str != "a"+fffd+"b" {
		t.Errorf("isolated continuation = %q, want one U+FFFD", page.Rows[0][0].Str)
	}
	if page.Rows[0][1].Str != fffd {
		t.Errorf("truncated two-byte prefix = %q, want one U+FFFD", page.Rows[0][1].Str)
	}
	if page.Rows[0][2].Str != fffd+fffd {
		t.Errorf("overlong encoding = %q, want two U+FFFD subparts", page.Rows[0][2].Str)
	}
	if page.Rows[0][3].Str != fffd+fffd+fffd {
		t.Errorf("surrogate encoding = %q, want three U+FFFD subparts", page.Rows[0][3].Str)
	}
	if page.Rows[0][4].Str != fffd+fffd+fffd+fffd+"z" {
		t.Errorf("adjacent invalid run = %q, want four U+FFFD subparts then z", page.Rows[0][4].Str)
	}
	// Controls are retained in the normalized typed text; exporters receive
	// the raw policy input, not the grid display.
	wantControls := "tab\tnew\nret\rnul\x00ctl"
	if page.Rows[0][5].Str != wantControls {
		t.Errorf("normalized TEXT controls = %q, want %q", page.Rows[0][5].Str, wantControls)
	}
	// The visible grid rendering is Issue #22's, not what exporters must use.
	if got := page.Rows[0][5].Display(); got == wantControls {
		t.Error("grid display leaked controls verbatim; exporters must not infer type from display text")
	}

	// BLOBs holding the same bytes stay byte-for-byte unchanged.
	blobPayloads := [][]byte{[]byte("a\x80b"), {0xE0, 0x80, 0x80}, {0x00, 0xFF}, {}}
	for i, want := range blobPayloads {
		v := page.Rows[1][i]
		if v.Kind != result.KindBlob {
			t.Fatalf("blob %d kind = %d, want blob", i, v.Kind)
		}
		if string(v.Bytes) != string(want) {
			t.Errorf("blob %d bytes = %x, want %x", i, v.Bytes, want)
		}
	}

	// SQL NULL, empty TEXT, and empty BLOB are distinct typed values.
	nullCell := page.Rows[1][4]
	if nullCell.Kind != result.KindText || nullCell.Str != "" {
		t.Errorf("empty TEXT lost: %+v", nullCell)
	}
	if got := CellToken(nullCell); got != "" {
		t.Errorf("empty TEXT token = %q, want empty", got)
	}
	emptyBlob := result.NewBlob(nil)
	if emptyBlob.Kind != result.KindBlob || len(emptyBlob.Bytes) != 0 {
		t.Errorf("empty BLOB lost: %+v", emptyBlob)
	}
	if got := CellToken(emptyBlob); got != "[BLOB 0 bytes]" {
		t.Errorf("empty BLOB token = %q, want \"[BLOB 0 bytes]\"", got)
	}
	nullCell2 := result.NewNull()
	if got := CellToken(nullCell2); got != result.NullDisplay {
		t.Errorf("NULL token = %q, want %q", got, result.NullDisplay)
	}
}
