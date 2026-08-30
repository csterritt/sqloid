# Issue #51: Typed array-of-objects JSON export

*2026-08-29T19:35:25Z by Showboat 0.6.1*
<!-- showboat-id: 9b952e2b-1b2f-4cf9-8d8d-c4c446a3c1a6 -->

Issue #51 (Notes/PRD-sqloid.md 'Export formats and values', 'Output names', 'Invalid UTF-8 TEXT', the 'Export Module Design', and 'Testing Decisions') implements the array-of-objects JSON serializer in internal/export/json.go over the Issue #49 immutable capture payload (immutable-export-capture.md). This walkthrough serializes deterministic fixtures covering duplicate and colliding labels, deliberately nonascending source positions, JSON metacharacters and controls, SQL NULL, empty TEXT, INTEGER boundaries, finite and non-finite REALs, empty and nonempty BLOBs, Unicode, and multiple invalid UTF-8 sequences — capturing exact bytes, parsed types, and map-order independence — and then proves every Issue #49 metadata warning combination leaves the bytes unchanged.

### Structure: ascending rows, shared keys in column order, compact bytes

```bash
mkdir -p tmpsb051 && cat > tmpsb051/main.go <<'GOEOF'
package main

import (
	"bytes"
	"fmt"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	// Duplicate and colliding labels: the original pre-suffixed name_2
	// forces the deduplicated duplicate of name to name_3, and the
	// aggregate keeps its SQLite label count(*), per the shared
	// full-set rule.
	cols := []string{"id", "name", "name", "note", "name_2", "count(*)"}
	names := result.DeduplicateNames(cols)
	fmt.Println("names:", names)

	// Rows presented in nonascending source order: logical positions 3, 1, 2.
	pos1 := []result.Value{ // JSON metacharacters, quotes, controls, Unicode
		result.NewText("say \"hi\""), result.NewText("back\\slash"),
		result.NewText("tab\there"), result.NewText("cr\rlf\nand\r\nend"),
		result.NewText("ctl\x00\x01\x1f"), result.NewText("utf8 héllo — ☃"),
	}
	pos2 := []result.Value{ // solidus, structural chars, empty TEXT, NULL mix
		result.NewText("a/b"), result.NewText("[{}]"), result.NewText(""),
		result.NewNull(), result.NewNull(), result.NewText("quote\"inside"),
	}
	pos3 := []result.Value{ // plain ASCII
		result.NewText("third"), result.NewText("r3a"), result.NewText("r3b"),
		result.NewText("r3c"), result.NewText("r3d"), result.NewText("r3e"),
	}
	// Direct payload: deliberately nonascending positions [3, 1, 2]
	// over the same source-order rows.
	payload := export.Payload{
		Names:     names,
		Positions: []int64{3, 1, 2},
		Rows:      [][]result.Value{pos3, pos1, pos2},
	}
	fmt.Println("source positions:", payload.Positions)

	first := export.JSON(payload)
	fmt.Printf("JSON: %q\n", first)

	// Repeated serialization must be byte-identical: the writer emits
	// each object directly in column order, never through a map.
	for i := 0; i < 5; i++ {
		if !bytes.Equal(first, export.JSON(payload)) {
			fmt.Println("MISMATCH on run", i)
			return
		}
	}
	fmt.Println("repeat serialization: byte-identical across 6 runs")

	// Zero rows and one row.
	fmt.Printf("zero rows: %q\n", export.JSON(export.Payload{Names: names}))
	fmt.Printf("one row:   %q\n", export.JSON(export.Payload{
		Names:     []string{"b", "a"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{result.NewText("1"), result.NewText("2")}},
	}))

	// Deliberately shuffled positions still serialize ascending; the
	// source rows stay untouched.
	rows := [][]result.Value{{result.NewText("c")}, {result.NewText("a")}, {result.NewText("b")}}
	shuffled := export.JSON(export.Payload{Names: []string{"v"}, Positions: []int64{9, 2, 7}, Rows: rows})
	fmt.Printf("shuffled ascending: %q\n", shuffled)
	fmt.Println("source order kept:", rows[0][0].Str, rows[1][0].Str, rows[2][0].Str)
}
GOEOF
go run ./tmpsb051 && rm -rf tmpsb051
```

```output
names: [id name name_3 note name_2 count(*)]
source positions: [3 1 2]
JSON: "[{\"id\":\"say \\\"hi\\\"\",\"name\":\"back\\\\slash\",\"name_3\":\"tab\\there\",\"note\":\"cr\\rlf\\nand\\r\\nend\",\"name_2\":\"ctl\\u0000\\u0001\\u001f\",\"count(*)\":\"utf8 héllo — ☃\"},{\"id\":\"a/b\",\"name\":\"[{}]\",\"name_3\":\"\",\"note\":null,\"name_2\":null,\"count(*)\":\"quote\\\"inside\"},{\"id\":\"third\",\"name\":\"r3a\",\"name_3\":\"r3b\",\"note\":\"r3c\",\"name_2\":\"r3d\",\"count(*)\":\"r3e\"}]"
repeat serialization: byte-identical across 6 runs
zero rows: "[]"
one row:   "[{\"b\":\"1\",\"a\":\"2\"}]"
shuffled ascending: "[{\"v\":\"a\"},{\"v\":\"b\"},{\"v\":\"c\"}]"
source order kept: c a b
```

The names resolve per the shared full-set rule (the original pre-suffixed name_2 forces the deduplicated duplicate of name to name_3), the source positions [3, 1, 2] serialize ascending (say "hi" row first), every object carries the identical key sequence id/name/name_3/note/name_2/count(*) written directly in column order — never through an unordered map — controls use short-form and \u00XX escapes, the solidus stays verbatim, and repeated serialization is byte-identical. Zero rows give exactly [] and the input rows are untouched.

### Parsed types: NULL, INTEGER, REAL, quoted non-finite, TEXT, BLOB

```bash
mkdir -p tmpsb051 && cat > tmpsb051/main.go <<'GOEOF'
package main

import (
	"encoding/json"
	"math"
	"fmt"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	// Same fixture family as the structure demo: NULL, empty TEXT,
	// escaped TEXT, INTEGER, REAL, non-finite REAL, and BLOB together.
	cols := []string{"n", "i", "r", "r2", "t", "b"}
	payload := export.Payload{
		Names:     result.DeduplicateNames(cols),
		Positions: []int64{1},
		Rows: [][]result.Value{{
			result.NewNull(),
			result.NewInteger(-9223372036854775808),
			result.NewReal(1e20),
			result.NewReal(math.Inf(-1)),
			result.NewText("tab\tand,quote\"nl\r\n"),
			{Kind: result.KindBlob, Bytes: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
		}},
	}
	got := export.JSON(payload)
	fmt.Printf("JSON: %q\n", got)

	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(got, &arr); err != nil {
		fmt.Println("not valid JSON:", err)
		return
	}
	obj := arr[0]
	for _, k := range []string{"n", "i", "r", "r2", "t", "b"} {
		raw := obj[k]
		var v any
		json.Unmarshal(raw, &v)
		fmt.Printf("key %-3s raw %-28s decoded %#v (%T)\n", k, string(raw), v, v)
	}
}
GOEOF
go run ./tmpsb051 && rm -rf tmpsb051
```

```output
JSON: "[{\"n\":null,\"i\":-9223372036854775808,\"r\":1e+20,\"r2\":\"-Inf\",\"t\":\"tab\\tand,quote\\\"nl\\r\\n\",\"b\":\"yv66vg==\"}]"
key n   raw null                         decoded <nil> (<nil>)
key i   raw -9223372036854775808         decoded -9.223372036854776e+18 (float64)
key r   raw 1e+20                        decoded 1e+20 (float64)
key r2  raw "-Inf"                       decoded "-Inf" (string)
key t   raw "tab\tand,quote\"nl\r\n"     decoded "tab\tand,quote\"nl\r\n" (string)
key b   raw "yv66vg=="                   decoded "yv66vg==" (string)
```

Parsed back, the objects show the full typed policy: JSON null for SQL NULL (distinctly from empty TEXT's ""), the raw unquoted INTEGER token at the signed 64-bit boundary, the raw finite REAL token 1e+20 identical to the grid/CSV RealToken, the pre-existing non-finite REAL as the exact quoted string "-Inf", the escaped TEXT string, and the standard base64 BLOB string yv66vg== encoding the unchanged CA FE BA BE bytes.

### Numeric and BLOB tokens in detail

```bash
mkdir -p tmpsb051 && cat > tmpsb051/main.go <<'GOEOF'
package main

import (
	"fmt"
	"math"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	one := func(v result.Value) string { return string(export.JSON(export.Payload{
		Names: []string{"v"}, Positions: []int64{1}, Rows: [][]result.Value{{v}},
	})) }

	// INTEGER raw tokens at the signed 64-bit boundaries.
	for _, i := range []int64{0, -1, math.MaxInt64, math.MinInt64} {
		fmt.Printf("INTEGER %-22d -> %s\n", i, one(result.NewInteger(i)))
	}

	// Finite REAL raw tokens equal to the shared grid/CSV RealToken.
	reals := []struct {
		v float64
		s string
	}{
		{1.0, "1.0"}, {math.Copysign(0, -1), "-0.0"}, {0.5, "0.5"},
		{1e-5, "1e-05"}, {math.SmallestNonzeroFloat64, "5e-324"},
		{math.MaxFloat64, "max"}, {0.30000000000000004, "edge"},
	}
	for _, r := range reals {
		token := result.RealToken(r.v)
		fmt.Printf("REAL %-26s -> %s (RealToken %q)\n", r.s, one(result.NewReal(r.v)), token)
	}

	// Pre-existing non-finite REALs become exact quoted strings.
	for _, inf := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		fmt.Printf("non-finite -> %s\n", one(result.NewReal(inf)))
	}

	// BLOBs: empty, NUL-containing, arbitrary — standard base64.
	for _, b := range [][]byte{nil, {0x00}, {0xDE, 0xAD, 0xBE, 0xEF}} {
		fmt.Printf("BLOB %-14x -> %s\n", b, one(result.Value{Kind: result.KindBlob, Bytes: b}))
	}
}
GOEOF
go run ./tmpsb051 && rm -rf tmpsb051
```

```output
INTEGER 0                      -> [{"v":0}]
INTEGER -1                     -> [{"v":-1}]
INTEGER 9223372036854775807    -> [{"v":9223372036854775807}]
INTEGER -9223372036854775808   -> [{"v":-9223372036854775808}]
REAL 1.0                        -> [{"v":1.0}] (RealToken "1.0")
REAL -0.0                       -> [{"v":-0.0}] (RealToken "-0.0")
REAL 0.5                        -> [{"v":0.5}] (RealToken "0.5")
REAL 1e-05                      -> [{"v":1e-05}] (RealToken "1e-05")
REAL 5e-324                     -> [{"v":5e-324}] (RealToken "5e-324")
REAL max                        -> [{"v":1.7976931348623157e+308}] (RealToken "1.7976931348623157e+308")
REAL edge                       -> [{"v":0.30000000000000004}] (RealToken "0.30000000000000004")
non-finite -> [{"v":"Inf"}]
non-finite -> [{"v":"-Inf"}]
non-finite -> [{"v":"NaN"}]
BLOB                -> [{"v":""}]
BLOB 00             -> [{"v":"AA=="}]
BLOB deadbeef       -> [{"v":"3q2+7w=="}]
```

Every finite REAL emits the shared raw RealToken — integral values keep .0, negative zero keeps its sign, subnormals and exponent forms round-trip bit-exactly — while non-finite REALs can never be JSON numbers and become quoted Inf, -Inf, and NaN. BLOBs are standard base64 (empty bytes encode to ""), with source bytes unchanged.

### Invalid UTF-8: one U+FFFD per maximal invalid sequence

```bash
mkdir -p tmpsb051 && cat > tmpsb051/main.go <<'GOEOF'
package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/chris/sqloid/internal/result"
)

func main() {
	// Raw TEXT bytes run through the shared result.DecodeText policy:
	// exactly one U+FFFD per maximal invalid byte sequence.
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"overlong C0 80", "\xC0\x80", 2},
		{"surrogate ED A0 80", "\xED\xA0\x80", 3},
		{"truncated E0 A0", "\xE0\xA0", 1},
		{"interior sequences", "ok\xE1\x80mid\xC0\x80end", 3},
	}
	for _, c := range cases {
		normalized, replaced := result.DecodeText(c.raw)
		count := strings.Count(normalized, "\uFFFD")
		status := "OK"
		if !replaced || count != c.want {
			status = "WRONG"
		}
		fmt.Printf("%-22s replaced=%v U+FFFDs=%d want=%d %s\n", c.name, replaced, count, c.want, status)
	}
	// Valid multibyte TEXT passes through untouched.
	valid, replaced := result.DecodeText("héllo — ☃ 世界")
	fmt.Printf("valid multibyte: replaced=%v unchanged=%v\n", replaced, valid == "héllo — ☃ 世界")
	_ = bytes.MinRead
}
GOEOF
go run ./tmpsb051 && rm -rf tmpsb051
```

```output
overlong C0 80         replaced=true U+FFFDs=2 want=2 OK
surrogate ED A0 80     replaced=true U+FFFDs=3 want=3 OK
truncated E0 A0        replaced=true U+FFFDs=1 want=1 OK
interior sequences     replaced=true U+FFFDs=3 want=3 OK
valid multibyte: replaced=false unchanged=true
```

Normalization happened once upstream in result.DecodeText; the JSON writer renormalizes nothing and emits the already-decoded TEXT through the shared escaper, so each maximal invalid sequence (overlong C0 80, surrogate ED A0 80, truncated E0 A0) contributes exactly one U+FFFD while valid multibyte TEXT passes through untouched.

### Issue #49 warnings: metadata never reaches the data

```bash
mkdir -p tmpsb051 && cat > tmpsb051/main.go <<'GOEOF'
package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	cols := []string{"id", "name", "name", "note", "name_2", "count(*)"}
	pos1 := []result.Value{
		result.NewText("say \"hi\""), result.NewText("back\\slash"),
		result.NewText("tab\there"), result.NewText("cr\rlf\nend"),
		result.NewText("ctl\x00"), result.NewText("utf8 ☃"),
	}
	pos2 := []result.Value{
		result.NewText("a/b"), result.NewNull(), result.NewText(""),
		result.NewInteger(-9223372036854775808), result.NewReal(math_Inf(-1)), result.NewText(""),
	}
	rows := [][]result.Value{pos1, pos2}
	baseline := export.JSON(export.CaptureRows(cols, rows, 1, true,
		history.SnapshotMetadata{}, history.Completeness{Complete: true}).Payload)
	fmt.Printf("baseline: %q\n", baseline)

	complete := []history.Completeness{
		{Complete: true}, {Partial: true}, {Truncated: true}, {Partial: true, Truncated: true},
	}
	outcomes := []history.SnapshotMetadata{
		{Outcome: history.OutcomeSuccess},
		{Outcome: history.OutcomeCancelled, Reason: "interrupted at row 2"},
		{Outcome: history.OutcomeFailed, Reason: "disk I/O error"},
		{Outcome: history.OutcomeCancelled},
	}
	flags := []history.SnapshotMetadata{
		{},
		{HasRetainedRange: true, RetainedStart: 1, RetainedEnd: 3},
		{TruncatedByByteCap: true},
		{RowCapEvicted: true, RowCapEvictions: 7},
		{InvalidUTF: true},
	}
	count, leaks := 0, 0
	for _, comp := range complete {
		for _, meta := range outcomes {
			for _, f := range flags {
				combined := meta
				combined.HasRetainedRange = f.HasRetainedRange
				combined.RetainedStart = f.RetainedStart
				combined.RetainedEnd = f.RetainedEnd
				combined.TruncatedByByteCap = f.TruncatedByByteCap
				combined.RowCapEvicted = f.RowCapEvicted
				combined.RowCapEvictions = f.RowCapEvictions
				combined.InvalidUTF = f.InvalidUTF
				got := export.JSON(export.CaptureRows(cols, rows, 1, true, combined, comp).Payload)
				count++
				if !bytes.Equal(got, baseline) {
					leaks++
				}
				for _, w := range []string{result.UTFWarning, "partial", "truncated", "cancelled", "failed"} {
					if strings.Contains(string(got), w) {
						leaks++
					}
				}
			}
		}
	}
	fmt.Printf("checked %d metadata warning combinations: %d byte differences, %d warning-string leaks\n", count, leaks, leaks)
}
GOEOF
# math_Inf helper shim so the fixture stays dependency-light
sed -i 's/math_Inf(-1)/math.Inf(-1)/' tmpsb051/main.go
sed -i 's/\t"strings"/\t"math"\n\t"strings"/' tmpsb051/main.go
go run ./tmpsb051 && rm -rf tmpsb051
```

```output
baseline: "[{\"id\":\"say \\\"hi\\\"\",\"name\":\"back\\\\slash\",\"name_3\":\"tab\\there\",\"note\":\"cr\\rlf\\nend\",\"name_2\":\"ctl\\u0000\",\"count(*)\":\"utf8 ☃\"},{\"id\":\"a/b\",\"name\":null,\"name_3\":\"\",\"note\":-9223372036854775808,\"name_2\":\"-Inf\",\"count(*)\":\"\"}]"
checked 80 metadata warning combinations: 0 byte differences, 0 warning-string leaks
```

All 80 metadata combinations — 4 completeness label sets × 4 terminal outcomes (with and without reasons) × 5 flag sets (retained range, byte-cap truncation, row-cap eviction, invalid UTF) — serialize byte-for-byte identically to the metadata-free baseline, with no warning object, property, key, wrapper, or warning string anywhere. This is pinned by TestJSONWarningCombinations and TestJSONWarningCombinationsTyped in internal/export.

The exact bytes and parsed types above are pinned by internal/export/json_test.go and json_value_test.go (go test ./internal/export/), documented in Notes/wiki/json-export.md.
