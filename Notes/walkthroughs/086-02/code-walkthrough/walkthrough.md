# Issue #086 Code Walkthrough: Document Value.Display as Grid-Only

*2026-09-04T01:06:06Z by Showboat 0.6.1*
<!-- showboat-id: d7621b7b-def9-470d-ad55-134c4de78136 -->

Issue #86 (Notes/tasks/086-document-display-as-grid-only.md, Notes/PRD-sqloid.md Grid rendering/cache and Export formats and values decisions) corrects the internal/result package and Value.Display documentation so the shared representation consumed by both the grid and exporters is named as the typed value set, while the visible tab/newline transformation, (NULL), and [BLOB n bytes] placeholder are identified as grid presentation policy rather than shared export tokens. No function bodies, constants, imports, value conversion, serialization, or tests were changed — only documentation comments moved. This walkthrough renders the updated documentation, traces one typed matrix containing TEXT with tabs/newlines, NULL, BLOB, finite REAL, and non-finite REAL through grid display and CSV/JSON serialization, and includes focused test output proving runtime rendering and serialization remain byte-for-byte unchanged. Reference: Issue #86 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/086-02/code-walkthrough/.

## The corrected package documentation

The internal/result package comment now names the shared representation consumed by both internal/ui's frozen-header grid and the CSV/JSON exporters in internal/export as the typed value set: result.Value with Value.Kind and the kind-specific fields Int (KindInteger), Float (KindReal), Str (KindText), and Bytes (KindBlob), plus the original driver output labels and the full-set output-name deduplication rule. It identifies the visible tab/newline control-character symbols (GridText), the (NULL) glyph (NullDisplay), and the [BLOB n bytes] placeholder as grid-facing rendering decisions that are NOT part of the shared export contract, and directs CSV/JSON serializers to inspect Kind and the typed payload fields directly so TEXT bytes and format-specific NULL, BLOB, numeric, and non-finite REAL policies remain under internal/export. The exact numeric tokens (IntegerToken, RealToken) remain shared across output formats because numeric identity must not diverge; the non-finite tokens (Inf, -Inf, NaN) are shared display tokens whose format-specific encoding (raw number versus quoted string) is owned by internal/export.

```bash
sed -n '/\/\/ Package result is Sqloid/,/^package result/p' /home/chris/sqloid/internal/result/result.go
```

```output
// Package result is Sqloid's shared, UI-independent representation of one
// SELECT result. The shared representation consumed by both internal/ui's
// frozen-header grid and the CSV/JSON exporters in internal/export is the
// typed value set: result.Value together with Value.Kind and the
// kind-specific fields Int (KindInteger), Float (KindReal), Str (KindText),
// and Bytes (KindBlob), plus the original driver output labels and the
// full-set output-name deduplication rule (DeduplicateNames). FromDriver
// converts only the plain driver value set (nil, int64, float64, string,
// []byte) once at the Connection boundary, applies maximal invalid UTF-8
// replacement to TEXT only (DecodeText), and records the InvalidUTF warning
// metadata; every other consumer works on the typed values here without
// re-decoding or re-normalizing. Generated SQL and driver column metadata
// are never altered; deduplication applies only to display and export names.
//
// The exact numeric tokens are shared across output formats because numeric
// identity must not diverge: IntegerToken (strconv.FormatInt decimal) and
// RealToken (shortest round-tripping finite 'g' token with .0 restoration,
// plus the Issue #23 non-finite tokens Inf, -Inf, NaN) are consumed by the
// grid and by exporters. The non-finite tokens are shared display tokens
// whose format-specific encoding (raw number versus quoted string) is owned
// by internal/export.
//
// The package also owns grid presentation policy that is NOT part of the
// shared export contract: the visible tab/newline control-character symbols
// applied by GridText, the (NULL) glyph (NullDisplay), and the
// [BLOB n bytes] placeholder are grid-facing rendering decisions. CSV and
// JSON serializers must not route TEXT, NULL, or BLOB values through these
// transformed display strings; instead they inspect Kind and the typed
// payload fields so TEXT bytes and format-specific NULL, BLOB, and
// non-finite REAL policies remain under internal/export.
//
// The package is independent of Bubble Tea, database-driver concrete types,
// and exporter formats.
package result
```

## The corrected Value.Display documentation

Value.Display is now documented as grid-facing rendering only. CSV and JSON serializers must not route values through the transformed display string; format-specific serializers inspect Kind and the typed payload fields (Int, Float, Str, Bytes) directly so TEXT bytes and CSV/JSON NULL, BLOB, numeric, and non-finite REAL policies remain under internal/export. The render seam never coerces a value into another type: numeric-looking TEXT stays text and non-finite REALs keep the REAL kind.

```bash
sed -n '/\/\/ Display returns the grid-facing presentation token/,/^}/p' /home/chris/sqloid/internal/result/result.go
```

```output
// Display returns the grid-facing presentation token for this value as
// rendered by the frozen grid header's cells: INTEGER uses IntegerToken,
// finite and non-finite REAL use RealToken, TEXT uses the decoded string
// transformed through the visible control-character symbols (GridText), BLOB
// renders exactly `[BLOB n bytes]`, and NULL renders NullDisplay. Display
// serves grid rendering only; CSV and JSON serializers must not route values
// through this transformed string. Format-specific serializers inspect Kind
// and the typed payload fields (Int, Float, Str, Bytes) directly so TEXT
// bytes and CSV/JSON NULL, BLOB, numeric, and non-finite REAL policies
// remain under internal/export. The render seam never coerces a value into
// another type: numeric-looking TEXT stays text and non-finite REALs keep
// the REAL kind.
func (v Value) Display() string {
	switch v.Kind {
	case KindNull:
		return NullDisplay
	case KindInteger:
		return IntegerToken(v.Int)
	case KindReal:
		return RealToken(v.Float)
	case KindText:
		return GridText(v.Str)
	case KindBlob:
		return fmt.Sprintf("[BLOB %d bytes]", len(v.Bytes))
	default:
		return fmt.Sprintf("(invalid kind %d)", int(v.Kind))
	}
}
```

## Tracing one typed matrix through grid display and CSV/JSON serialization

The matrix exercises every kind-relevant policy: TEXT with an embedded tab and newline, SQL NULL, a BLOB with non-UTF-8 bytes, a finite REAL, and the three non-finite REAL tokens. The grid (Value.Display) alone applies the visible control symbols (⇥/⏎), the (NULL) glyph, and the [BLOB n bytes] placeholder. The exporters (export.CSV and export.JSON) inspect Kind and the typed payload fields directly: TEXT bytes pass through raw (CSV quotes the field preserving the literal tab/newline; JSON escapes them as \t/\n), NULL becomes an empty CSV field and a JSON null, BLOB becomes lowercase CSV hex and base64 JSON, finite REAL is the shared exact token (raw JSON number), and non-finite REALs take their textual CSV form and quoted JSON string form. The demo program lives alongside this walkthrough.

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/086-02/code-walkthrough/ | tr -d '\r'
```

```output
=== Grid display: Value.Display (grid presentation policy) ===
headers: [text null blob finite_real pos_inf neg_inf nan]
row 1: [line1⇥col⇥2⏎line2 (NULL) [BLOB 6 bytes] 1.0 Inf -Inf NaN]

=== Exporters inspect Kind and typed payload fields directly ===
--- CSV (export.CSV) ---
text,null,blob,finite_real,pos_inf,neg_inf,nan
"line1	col	2
line2",,00ffe0a06f6b,1.0,Inf,-Inf,NaN
--- JSON (export.JSON) ---
[
  {
    "text": "line1\tcol\t2\nline2",
    "null": null,
    "blob": "AP/goG9r",
    "finite_real": 1.0,
    "pos_inf": "Inf",
    "neg_inf": "-Inf",
    "nan": "NaN"
  }
]
=== Kind/payload inspection summary (what exporters see) ===
col 1 (text): Kind=Text Str="line1\tcol\t2\nline2" (CSV/JSON receive raw bytes, no visible symbols)
col 2 (null): Kind=Null -> CSV empty field, JSON null
col 3 (blob): Kind=Blob Bytes=6 (CSV lowercase hex, JSON base64)
col 4 (finite_real): Kind=Real Float=1 token="1.0" (CSV textual, JSON raw or quoted if non-finite)
col 5 (pos_inf): Kind=Real Float=+Inf token="Inf" (CSV textual, JSON raw or quoted if non-finite)
col 6 (neg_inf): Kind=Real Float=-Inf token="-Inf" (CSV textual, JSON raw or quoted if non-finite)
col 7 (nan): Kind=Real Float=NaN token="NaN" (CSV textual, JSON raw or quoted if non-finite)
```

## Focused test output: runtime rendering and serialization unchanged

The focused internal/result tests covering tabs, newlines, NULL, BLOB, finite REAL, and non-finite REAL, and the focused internal/export tests covering the typed-value matrix for CSV and JSON, all pass green — proving the documentation change moved comments only and left runtime rendering and serialization byte-for-byte unchanged.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/result/ -run 'TestGridTextTransformation|TestDisplayTypedCellValues|TestNonFiniteRealDisplayTokens|TestNonFiniteRealTypeAndValueRetained|TestBlobBytesRetainedExactly|TestBlobNeverDecodedAsText|TestBlobBytesUnchangedWithInvalidUTFPatterns|TestTypeIdentityPreserved|TestRealToken$|TestRealTokenLocaleIndependent|TestRealTokenRoundTrips' 2>&1 | tail -5 | sed -E 's/[0-9]+\.[0-9]+s/Ns/'
```

```output
ok  	github.com/chris/sqloid/internal/result	Ns
```

```bash
cd /home/chris/sqloid && go test -count=1 -v ./internal/result/ -run 'TestGridTextTransformation|TestDisplayTypedCellValues|TestNonFiniteRealDisplayTokens' 2>&1 | tail -45 | sed -E 's/^(ok.*\t)[0-9]+\.[0-9]+s$/\1Ns/'
```

```output
=== RUN   TestDisplayTypedCellValues/null
=== RUN   TestDisplayTypedCellValues/integer
=== RUN   TestDisplayTypedCellValues/negative_integer
=== RUN   TestDisplayTypedCellValues/real_uses_exact_token
=== RUN   TestDisplayTypedCellValues/real_negative_zero
=== RUN   TestDisplayTypedCellValues/real_exponent
=== RUN   TestDisplayTypedCellValues/text_verbatim_after_transformation
=== RUN   TestDisplayTypedCellValues/text_that_looks_numeric_stays_text_verbatim
=== RUN   TestDisplayTypedCellValues/blob_placeholder
=== RUN   TestDisplayTypedCellValues/empty_blob_placeholder
--- PASS: TestDisplayTypedCellValues (0.00s)
    --- PASS: TestDisplayTypedCellValues/null (0.00s)
    --- PASS: TestDisplayTypedCellValues/integer (0.00s)
    --- PASS: TestDisplayTypedCellValues/negative_integer (0.00s)
    --- PASS: TestDisplayTypedCellValues/real_uses_exact_token (0.00s)
    --- PASS: TestDisplayTypedCellValues/real_negative_zero (0.00s)
    --- PASS: TestDisplayTypedCellValues/real_exponent (0.00s)
    --- PASS: TestDisplayTypedCellValues/text_verbatim_after_transformation (0.00s)
    --- PASS: TestDisplayTypedCellValues/text_that_looks_numeric_stays_text_verbatim (0.00s)
    --- PASS: TestDisplayTypedCellValues/blob_placeholder (0.00s)
    --- PASS: TestDisplayTypedCellValues/empty_blob_placeholder (0.00s)
=== RUN   TestNonFiniteRealDisplayTokens
=== RUN   TestNonFiniteRealDisplayTokens/positive_infinity
=== RUN   TestNonFiniteRealDisplayTokens/negative_infinity
=== RUN   TestNonFiniteRealDisplayTokens/quiet_NaN
=== RUN   TestNonFiniteRealDisplayTokens/NaN_payload_renders_same_token
=== RUN   TestNonFiniteRealDisplayTokens/negative_NaN_renders_same_token
=== RUN   TestNonFiniteRealDisplayTokens/finite_REAL_keeps_exact_token
=== RUN   TestNonFiniteRealDisplayTokens/finite_REAL_exponent_keeps_exact_token
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_Inf_stays_verbatim_text
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_-Inf_stays_verbatim_text
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_NaN_stays_verbatim_text
--- PASS: TestNonFiniteRealDisplayTokens (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/positive_infinity (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/negative_infinity (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/quiet_NaN (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/NaN_payload_renders_same_token (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/negative_NaN_renders_same_token (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/finite_REAL_keeps_exact_token (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/finite_REAL_exponent_keeps_exact_token (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_Inf_stays_verbatim_text (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_-Inf_stays_verbatim_text (0.00s)
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_NaN_stays_verbatim_text (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/result	Ns
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/export/ -run 'TestCSVNullAndEmptyTextIdentical|TestCSVIntegerTokens|TestCSVRealTokens|TestCSVBlobLowercaseHex|TestCSVTextControlsAndMultibyte|TestCSVInvalidUTF8Normalized|TestCSVTypedMatrixGolden|TestCSVTypedMatrixRealTokenEquality|TestJSONNullAndEmptyTextDistinct|TestJSONIntegerRawTokens|TestJSONFiniteRealRawTokens|TestJSONNonFiniteRealQuotedTokens|TestJSONBlobBase64|TestJSONTextEscaping|TestJSONInvalidUTF8Normalized|TestJSONTypedMatrixGolden|TestJSONTypedMatrixRealTokenEquality|TestJSONWarningCombinationsTyped|TestExporterOutputNamesMatchFullSetRule|TestExporterNamesMatchGridNames|TestExporterFiniteRealTokens|TestExporterRealIdentityRetained|TestExporterTypedNormalizationAndMetadata' 2>&1 | tail -5 | sed -E 's/[0-9]+\.[0-9]+s/Ns/'
```

```output
ok  	github.com/chris/sqloid/internal/export	Ns
```

```bash
cd /home/chris/sqloid && go build ./... && go test -count=1 ./... 2>&1 | tail -15 | sed -E 's/[0-9]+\.[0-9]+s/Ns/'
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/085-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/086-02/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	Ns
ok  	github.com/chris/sqloid/internal/cli	Ns
ok  	github.com/chris/sqloid/internal/connection	Ns
ok  	github.com/chris/sqloid/internal/d1	Ns
ok  	github.com/chris/sqloid/internal/export	Ns
ok  	github.com/chris/sqloid/internal/filepicker	Ns
ok  	github.com/chris/sqloid/internal/history	Ns
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
ok  	github.com/chris/sqloid/internal/result	Ns
ok  	github.com/chris/sqloid/internal/resultcache	Ns
ok  	github.com/chris/sqloid/internal/schema	Ns
ok  	github.com/chris/sqloid/internal/session	Ns
ok  	github.com/chris/sqloid/internal/ui	Ns
```

## Conclusion

Issue #86 is a documentation-only correction. The internal/result package comment and the Value.Display doc comment now state explicitly that the shared representation consumed by both the grid and exporters is the typed value set (Value/Kind/Int/Float/Str/Bytes) plus the shared numeric tokens (IntegerToken/RealToken), while the visible tab/newline transformation (GridText), (NULL) glyph (NullDisplay), and [BLOB n bytes] placeholder are grid presentation policy rather than shared export tokens. CSV and JSON serializers inspect Kind and typed payload fields directly so TEXT bytes and format-specific NULL, BLOB, numeric, and non-finite REAL policies remain under internal/export. The traced matrix proves the grid alone applies the visible control symbols, (NULL), and [BLOB n bytes], while exporters emit format-specific bytes (raw TEXT, empty CSV field / JSON null, lowercase-hex CSV / base64 JSON BLOB, raw or quoted non-finite REALs). The focused internal/result and internal/export tests and the full go build ./... and go test ./... all pass green, proving runtime rendering and serialization remain byte-for-byte unchanged. Reference: Issue #86 and Notes/PRD-sqloid.md (Export formats and values, Grid rendering/cache decisions). All artifacts are under Notes/walkthroughs/086-02/code-walkthrough/.
