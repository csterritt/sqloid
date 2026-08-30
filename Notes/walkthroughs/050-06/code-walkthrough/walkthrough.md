# Issue #50: RFC 4180 CSV export

*2026-08-29T18:49:18Z by Showboat 0.6.1*
<!-- showboat-id: eebcea20-ef37-4141-b828-675a8f6c100b -->

Issue #50 (Notes/PRD-sqloid.md 'Export formats and values', 'Output names', 'Invalid UTF-8 TEXT', 'Result export scope', the 'Export Module Design', and 'Testing Decisions') implements the RFC 4180 CSV serializer in internal/export/csv.go over the Issue #49 immutable capture payload (immutable-export-capture.md). The fixture program below builds one deterministic tabular selection with duplicate and colliding labels — the original pre-suffixed name_2 forces the deduplicated duplicate of name to name_3 per the shared full-set rule — and three rows presented in nonascending source order (logical positions 3, 1, 2) containing ordinary ASCII, UTF-8, empty fields, SQL NULL, a bare tab, commas, double quotes, CR, LF, and CRLF. It serializes through the Issue #49 CaptureRows constructor and prints the exact Go-quoted bytes.

```bash
mkdir -p tmpsb050 && cat > tmpsb050/main.go <<'GOEOF'
package main

import (
	"fmt"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	cols := []string{"id", "name", "name", "note", "name_2", "count(*)"}
	pos1 := []result.Value{
		result.NewText("plain"), result.NewText(""), result.NewText("tab\there"),
		result.NewNull(), result.NewNull(), result.NewText("utf8 héllo"),
	}
	pos2 := []result.Value{
		result.NewText("a,b"), result.NewText("say \"hi\""), result.NewText("cr\rlf"),
		result.NewText("crlf\r\nend"), result.NewNull(), result.NewText(""),
	}
	pos3 := []result.Value{
		result.NewText(""), result.NewNull(), result.NewText(""),
		result.NewNull(), result.NewText(""), result.NewText(""),
	}
	payload := export.Payload{
		Names:     result.DeduplicateNames(cols),
		Positions: []int64{3, 1, 2},
		Rows:      [][]result.Value{pos3, pos1, pos2},
	}
	fmt.Printf("names: %q\n", payload.Names)
	fmt.Printf("csv:   %q\n", string(export.CSV(payload)))
}
GOEOF
go run ./tmpsb050 && rm -rf tmpsb050
```

```output
names: ["id" "name" "name_3" "note" "name_2" "count(*)"]
csv:   "id,name,name_3,note,name_2,count(*)\r\nplain,,tab\there,,,utf8 héllo\r\n\"a,b\",\"say \"\"hi\"\"\",\"cr\rlf\",\"crlf\r\nend\",,\r\n,,,,,\r\n"
```

Read the bytes: exactly one header record id,name,name_3,note,name_2,count(*) — the deduplicated output names alone, no warning column; then data records for logical positions 1, 2, 3 in ascending order despite the nonascending source order (3, 1, 2); CRLF after every record including the last; the bare tab in 'tab\there' preserved unquoted; 'a,b', the embedded CR and LF, and the CRLF each force a quoted field; the embedded double quote is doubled ('say ""hi""'); SQL NULL and empty TEXT serialize to the identical empty field (the documented accepted lossy limitation); and UTF-8 'utf8 héllo' passes through byte-exactly.

Now the full Issue #49 metadata warning matrix: every completeness label combination (complete; partial; truncated; partial+truncated), every terminal outcome (success; cancelled with reason; failed with reason), and every flag set (retained range; byte-cap truncation; row-cap eviction with cumulative count; invalid-UTF) — 60 combinations — capture the same tabular data. No warning, completeness, outcome, or UTF fact may become a row, column, prefix, comment, or altered header.

```bash
mkdir -p tmpsb050 && cat > tmpsb050/main.go <<'GOEOF'
package main

import (
	"fmt"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	cols := []string{"id", "name", "name", "note", "name_2", "count(*)"}
	pos1 := []result.Value{
		result.NewText("plain"), result.NewText(""), result.NewText("tab\there"),
		result.NewNull(), result.NewNull(), result.NewText("utf8 héllo"),
	}
	pos2 := []result.Value{
		result.NewText("a,b"), result.NewText("say \"hi\""), result.NewText("cr\rlf"),
		result.NewText("crlf\r\nend"), result.NewNull(), result.NewText(""),
	}
	pos3 := []result.Value{
		result.NewText(""), result.NewNull(), result.NewText(""),
		result.NewNull(), result.NewText(""), result.NewText(""),
	}
	baseline := export.CSV(export.CaptureRows(cols, [][]result.Value{pos1, pos2, pos3}, 1, true,
		history.SnapshotMetadata{}, history.Completeness{Complete: true}).Payload)

	combos := []history.Completeness{
		{Complete: true}, {Partial: true}, {Truncated: true}, {Partial: true, Truncated: true},
	}
	metas := []history.SnapshotMetadata{
		{Outcome: history.OutcomeSuccess},
		{Outcome: history.OutcomeCancelled, Reason: "interrupted at row 2"},
		{Outcome: history.OutcomeFailed, Reason: "disk I/O error"},
	}
	flags := []history.SnapshotMetadata{
		{},
		{HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 3},
		{TruncatedByByteCap: true},
		{RowCapEvicted: true, RowCapEvictions: 7},
		{InvalidUTF: true},
	}
	count, identical := 0, 0
	for _, comp := range combos {
		for _, meta := range metas {
			for _, f := range flags {
				combined := meta
				combined.HasRetainedRange = f.HasRetainedRange
				combined.RetainedStart = f.RetainedStart
				combined.RetainedEnd = f.RetainedEnd
				combined.TruncatedByByteCap = f.TruncatedByByteCap
				combined.RowCapEvicted = f.RowCapEvicted
				combined.RowCapEvictions = f.RowCapEvictions
				combined.InvalidUTF = f.InvalidUTF
				count++
				captured := export.CaptureRows(cols, [][]result.Value{pos1, pos2, pos3}, 1, true, combined, comp)
				if string(export.CSV(captured.Payload)) == string(baseline) {
					identical++
				}
			}
		}
	}
	fmt.Printf("%d/%d Issue #49 metadata warning combinations produced byte-identical CSV\n", identical, count)
}
GOEOF
go run ./tmpsb050 && rm -rf tmpsb050
```

```output
60/60 Issue #49 metadata warning combinations produced byte-identical CSV
```

The complete typed-value policy: SQL NULL and empty TEXT as the identical empty field; the authoritative shared INTEGER token at the signed 64-bit boundary; finite REAL tokens identical to the grid (integral 1.0 gains .0, -0.0 keeps its sign, 1e+20 and the subnormal 5e-324 round-trip through the shared locale-independent formatter); pre-existing non-finite REALs as the exact textual tokens Inf, -Inf, and NaN; BLOB bytes as lowercase hexadecimal over unchanged retained bytes (empty BLOB as an empty field); multiple maximal invalid UTF-8 sequences normalized once through the shared internal/result policy to exactly one U+FFFD each; and NUL and control characters preserved, quoted only when an RFC-required character is also present.

```bash
mkdir -p tmpsb050 && cat > tmpsb050/main.go <<'GOEOF'
package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	typed := export.Payload{
		Names:     result.DeduplicateNames([]string{"n", "i", "r", "t", "b"}),
		Positions: []int64{1},
		Rows: [][]result.Value{{
			result.NewNull(),
			result.NewInteger(-9223372036854775808),
			result.NewReal(math.Inf(-1)),
			result.NewText("tab\tand,quote\"nl\r\n"),
			{Kind: result.KindBlob, Bytes: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
		}},
	}
	fmt.Printf("mixed: %q\n", string(export.CSV(typed)))

	finite := export.Payload{
		Names:     []string{"r"},
		Positions: []int64{1, 2, 3, 4},
		Rows: [][]result.Value{
			{result.NewReal(1.0)},
			{result.NewReal(math.Copysign(0, -1))},
			{result.NewReal(1e20)},
			{result.NewReal(math.SmallestNonzeroFloat64)},
		},
	}
	fmt.Printf("finite-reals (shared tokens): %q\n", string(export.CSV(finite)))

	nan := export.Payload{
		Names:     []string{"r"},
		Positions: []int64{1, 2, 3},
		Rows: [][]result.Value{
			{result.NewReal(math.Inf(1))},
			{result.NewReal(math.Inf(-1))},
			{result.NewReal(math.NaN())},
		},
	}
	fmt.Printf("non-finite (textual tokens): %q\n", string(export.CSV(nan)))

	blob := export.Payload{
		Names:     []string{"b"},
		Positions: []int64{1, 2, 3},
		Rows: [][]result.Value{
			{result.NewBlob(nil)},
			{result.NewBlob([]byte{0x00})},
			{result.NewBlob([]byte{0xDE, 0xAD, 0xBE, 0xEF})},
		},
	}
	fmt.Printf("blobs (lowercase hex): %q\n", string(export.CSV(blob)))

	norm, replaced := result.DecodeText("\xE1\x80\xC3")
	fmt.Printf("invalid-utf8: raw=%q normalized=%q replaced=%v one-fffd-per-maximal-subpart=%v\n",
		"\xE1\x80\xC3", norm, replaced, strings.Count(norm, "\uFFFD") == 2)

	ctrl := export.Payload{
		Names:     []string{"t"},
		Positions: []int64{1, 2},
		Rows: [][]result.Value{
			{result.NewText("\t")},
			{result.NewText("a\x00b\x1f")},
		},
	}
	fmt.Printf("controls: %q\n", string(export.CSV(ctrl)))
}
GOEOF
go run ./tmpsb050 && rm -rf tmpsb050
```

```output
mixed: "n,i,r,t,b\r\n,-9223372036854775808,-Inf,\"tab\tand,quote\"\"nl\r\n\",cafebabe\r\n"
finite-reals (shared tokens): "r\r\n1.0\r\n-0.0\r\n1e+20\r\n5e-324\r\n"
non-finite (textual tokens): "r\r\nInf\r\n-Inf\r\nNaN\r\n"
blobs (lowercase hex): "b\r\n\r\n00\r\ndeadbeef\r\n"
invalid-utf8: raw="\xe1\x80\xc3" normalized="��" replaced=true one-fffd-per-maximal-subpart=true
controls: "t\r\n\t\r\na\x00b\x1f\r\n"
```

Finally, the byte-golden test suites in internal/export prove the same contracts: structure (deduplicated header including pre-suffixed collision resolution, zero/one/multiple rows, nonascending source order serialized ascending without mutation, CRLF records, minimal quoting with quote doubling, tab-alone-unquoted, repeated-serialization byte identity) and values (the full matrix above plus five invalid-UTF-8 normalization cases with exact U+FFFD counts), and the warning-combination matrix in TestCSVWarningCombinations.

```bash
go test ./internal/export -count=1 -run 'TestCSV' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/\(([0-9.]+)s\)/(Ts)/'
```

```output
--- PASS: TestCSVStructureGolden (Ts)
--- PASS: TestCSVDoesNotMutateInput (Ts)
--- PASS: TestCSVZeroRows (Ts)
--- PASS: TestCSVOneRow (Ts)
--- PASS: TestCSVHeaderQuoting (Ts)
--- PASS: TestCSVTabAloneUnquoted (Ts)
--- PASS: TestCSVAscendingFromCaptureRows (Ts)
--- PASS: TestCSVWarningCombinations (Ts)
--- PASS: TestCSVOrderStability (Ts)
--- PASS: TestCSVNullAndEmptyTextIdentical (Ts)
--- PASS: TestCSVIntegerTokens (Ts)
--- PASS: TestCSVRealTokens (Ts)
--- PASS: TestCSVBlobLowercaseHex (Ts)
--- PASS: TestCSVTextControlsAndMultibyte (Ts)
--- PASS: TestCSVInvalidUTF8Normalized (Ts)
--- PASS: TestCSVTypedMatrixGolden (Ts)
--- PASS: TestCSVTypedMatrixRealTokenEquality (Ts)
ok  	github.com/chris/sqloid/internal/export	0.003s
```

Everything above is pinned by TestCSVStructureGolden, TestCSVWarningCombinations, and the typed-value goldens in internal/export/csv_test.go and csv_value_test.go, documented in Notes/wiki/csv-export.md. The JSON serializer remains owned by Issue #51.
