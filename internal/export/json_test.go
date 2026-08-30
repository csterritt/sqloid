// Deterministic array-of-objects JSON structure coverage for Issue #51,
// per the Export formats and values, Output names, Invalid UTF-8 TEXT,
// Export Module Design, and Testing Decisions decisions in
// Notes/PRD-sqloid.md. Every test pins exact output bytes over immutable
// inputs built from shared internal/result rows and full-set deduplicated
// output names: exactly one top-level JSON array, one object per retained
// row in ascending logical-position order, every object emitting the
// shared deduplicated keys in identical left-to-right column order (no
// map iteration or key reordering), a compact byte layout with no
// trailing whitespace or newline, and standards-compliant JSON string
// escaping. Metadata warning combinations run the same data and must
// leave the bytes unchanged, with no warning object, property, key, or
// wrapper. Typed SQLite value policy is covered in json_value_test.go.

package export

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// jsonStructureColumns carries duplicate labels, an original pre-suffixed
// label that collides with the deduplicated duplicate, and a computed
// SQLite label, exactly as an aggregate SELECT could emit them.
func jsonStructureColumns() []string {
	return []string{"id", "name", "name", "note", "name_2", "count(*)"}
}

// jsonStructureRows builds the same three tabular rows twice: once as raw
// input for direct Payload construction with deliberately nonascending
// positions, and once in capture order for CaptureRows. Rows are listed
// here in nonascending source order (position 3 first) to prove the
// serializer traverses ascending logical positions. All values are TEXT so
// this fixture pins pure structural policy only.
func jsonStructureRows() (unorderedRows, captureRows [][]result.Value) {
	// Position 3: plain ASCII only.
	pos3 := []result.Value{
		result.NewText("third"),
		result.NewText("r3a"),
		result.NewText("r3b"),
		result.NewText("r3c"),
		result.NewText("r3d"),
		result.NewText("r3e"),
	}
	// Position 1: JSON metacharacters, quotes, reverse solidus, controls,
	// tab, CR, LF, and multibyte Unicode.
	pos1 := []result.Value{
		result.NewText("say \"hi\""),
		result.NewText("back\\slash"),
		result.NewText("tab\there"),
		result.NewText("cr\rlf\nand\r\nend"),
		result.NewText("ctl\x00\x01\x1f"),
		result.NewText("utf8 héllo — ☃"),
	}
	// Position 2: solidus, embedded structural characters, and empty TEXT.
	pos2 := []result.Value{
		result.NewText("a/b"),
		result.NewText("[{}]"),
		result.NewText(""),
		result.NewText("line1\nline2"),
		result.NewText("emoji 🎉 ok"),
		result.NewText("quote\"inside"),
	}
	unorderedRows = [][]result.Value{pos3, pos1, pos2}
	captureRows = [][]result.Value{pos1, pos2, pos3}
	return unorderedRows, captureRows
}

// jsonStructureNames is the full-set deduplicated output name set for
// jsonStructureColumns, exactly as the frozen grid header renders them.
func jsonStructureNames() []string {
	return result.DeduplicateNames(jsonStructureColumns())
}

// jsonStructureGolden is the exact expected JSON for jsonStructureRows
// under ascending positions 1, 2, 3: one top-level array, one compact
// object per row, shared deduplicated keys in column order, JSON string
// escaping, and no trailing newline.
func jsonStructureGolden() []byte {
	return []byte(`[{"id":"say \"hi\"","name":"back\\slash","name_3":"tab\there","note":"cr\rlf\nand\r\nend","name_2":"ctl\u0000\u0001\u001f","count(*)":"utf8 héllo — ☃"},` +
		`{"id":"a/b","name":"[{}]","name_3":"","note":"line1\nline2","name_2":"emoji 🎉 ok","count(*)":"quote\"inside"},` +
		`{"id":"third","name":"r3a","name_3":"r3b","note":"r3c","name_2":"r3d","count(*)":"r3e"}]`)
}

// TestJSONStructureGolden pins the exact bytes: one top-level array, rows
// ordered by ascending logical position (the input was built in
// nonascending order), every object emitting the deduplicated keys in
// identical column order, the compact layout with no spaces and no
// trailing newline, and standards-compliant string escaping of quotes,
// reverse solidus, and controls.
func TestJSONStructureGolden(t *testing.T) {
	unorderedRows, _ := jsonStructureRows()
	payload := Payload{
		Names:     jsonStructureNames(),
		Positions: []int64{3, 1, 2},
		Rows:      unorderedRows,
	}
	got := JSON(payload)
	if want := jsonStructureGolden(); !bytes.Equal(got, want) {
		t.Fatalf("JSON bytes diverged:\n got %q\nwant %q", got, want)
	}
	// Control characters are escaped, never raw.
	for _, b := range got {
		if b < 0x20 {
			t.Errorf("raw control byte %#x in output %q", b, got)
		}
	}
}

// TestJSONParsedShape proves the exact bytes also parse as the required
// shape: one top-level array of one object per row, each object carrying
// exactly the deduplicated keys in column order with string values.
func TestJSONParsedShape(t *testing.T) {
	unorderedRows, _ := jsonStructureRows()
	got := JSON(Payload{Names: jsonStructureNames(), Positions: []int64{3, 1, 2}, Rows: unorderedRows})
	var arr []map[string]any
	if err := json.Unmarshal(got, &arr); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, got)
	}
	if len(arr) != 3 {
		t.Fatalf("parsed array length = %d, want 3", len(arr))
	}
	names := jsonStructureNames()
	// map iteration order is not observable, but the exact bytes pin the
	// emitted key order; parsing proves the key set and string types.
	for i, obj := range arr {
		if len(obj) != len(names) {
			t.Errorf("row %d has %d keys, want %d", i, len(obj), len(names))
		}
		for _, n := range names {
			v, ok := obj[n]
			if !ok {
				t.Errorf("row %d missing key %q", i, n)
				continue
			}
			if _, isStr := v.(string); !isStr {
				t.Errorf("row %d key %q value %#v, want string", i, n, v)
			}
		}
	}
	// Escaped controls decode back to the original strings.
	if got := arr[0]["note"]; got != "cr\rlf\nand\r\nend" {
		t.Errorf("row 1 note decoded = %#v, want the CR/LF string", got)
	}
	if got := arr[0]["name_2"]; got != "ctl\x00\x01\x1f" {
		t.Errorf("row 1 name_2 decoded = %#v, want the NUL/control string", got)
	}
}

// TestJSONZeroRows pins the empty array: zero retained rows yield exactly
// "[]", regardless of column count.
func TestJSONZeroRows(t *testing.T) {
	got := JSON(Payload{Names: jsonStructureNames(), Positions: nil, Rows: nil})
	if !bytes.Equal(got, []byte("[]")) {
		t.Fatalf("zero-row JSON = %q, want \"[]\"", got)
	}
}

// TestJSONOneRow pins a single-row array with no comma and identical key
// ordering.
func TestJSONOneRow(t *testing.T) {
	got := JSON(Payload{
		Names:     []string{"b", "a"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{result.NewText("1"), result.NewText("2")}},
	})
	want := []byte(`[{"b":"1","a":"2"}]`)
	if !bytes.Equal(got, want) {
		t.Fatalf("one-row JSON = %q, want %q", got, want)
	}
}

// TestJSONKeyOrderIdenticalAcrossRows proves every object emits the same
// keys in the same left-to-right column order — key order follows the
// deduplicated name set, never map iteration.
func TestJSONKeyOrderIdenticalAcrossRows(t *testing.T) {
	names := jsonStructureNames()
	rows := [][]result.Value{
		{result.NewText("v1"), result.NewText("v2"), result.NewText("v3"), result.NewText("v4"), result.NewText("v5"), result.NewText("v6")},
		{result.NewText("w1"), result.NewText("w2"), result.NewText("w3"), result.NewText("w4"), result.NewText("w5"), result.NewText("w6")},
	}
	got := string(JSON(Payload{Names: names, Positions: []int64{1, 2}, Rows: rows}))
	// Find each object's key sequence; every object must carry the
	// identical key sequence in column order.
	first := bytes.IndexByte([]byte(got), '{')
	objEnd := func(s string) int {
		depth := 0
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return i
				}
			}
		}
		return -1
	}
	var keySeqs []string
	for rest := got[first:]; len(rest) > 0; {
		end := objEnd(rest)
		if end < 0 {
			t.Fatalf("unbalanced object in %q", got)
		}
		keySeqs = append(keySeqs, rest[:end+1])
		rest = rest[end+1:]
		// Skip the array comma between objects and the closing bracket
		// after the last one.
		if len(rest) > 0 && (rest[0] == ',' || rest[0] == ']') {
			rest = rest[1:]
		}
	}
	if len(keySeqs) != 2 {
		t.Fatalf("object count = %d, want 2", len(keySeqs))
	}
	wantKeys := `{"id":`
	for _, seq := range keySeqs {
		if !bytes.HasPrefix([]byte(seq), []byte(wantKeys)) {
			t.Errorf("object %q does not start with keys in column order", seq)
		}
	}
	if keySeqs[0] == keySeqs[1] {
		t.Errorf("distinct rows produced identical objects — values must differ: %q", keySeqs[0])
	}
}

// TestJSONDoesNotMutateInput proves serialization leaves the immutable
// capture untouched: names, positions, rows, and every BLOB byte slice
// must compare equal after a repeated serialization, and repeated
// serialization must be byte-identical (map-order independence).
func TestJSONDoesNotMutateInput(t *testing.T) {
	unorderedRows, _ := jsonStructureRows()
	payload := Payload{
		Names:     jsonStructureNames(),
		Positions: []int64{3, 1, 2},
		Rows:      unorderedRows,
	}
	beforeRows := copyRowsForComparison(unorderedRows)
	beforePositions := append([]int64(nil), payload.Positions...)
	beforeNames := append([]string(nil), payload.Names...)

	first := JSON(payload)
	for i := 0; i < 5; i++ {
		again := JSON(payload)
		if !bytes.Equal(first, again) {
			t.Fatalf("run %d bytes diverged:\n got %q\nwant %q", i, again, first)
		}
	}
	if !reflect.DeepEqual(beforeRows, unorderedRows) {
		t.Errorf("rows mutated: got %+v, want %+v", unorderedRows, beforeRows)
	}
	if !reflect.DeepEqual(beforePositions, payload.Positions) {
		t.Errorf("positions mutated: %v, want %v", payload.Positions, beforePositions)
	}
	if !reflect.DeepEqual(beforeNames, payload.Names) {
		t.Errorf("names mutated: %v, want %v", payload.Names, beforeNames)
	}
}

// TestJSONAscendingFromCaptureRows proves CaptureRows-built payloads
// serialize identically to directly built ascending payloads.
func TestJSONAscendingFromCaptureRows(t *testing.T) {
	_, captureRows := jsonStructureRows()
	viaCapture := JSON(CaptureRows(jsonStructureColumns(), captureRows, 1, true,
		history.SnapshotMetadata{}, history.Completeness{Complete: true}).Payload)
	direct := JSON(Payload{Names: jsonStructureNames(), Positions: []int64{1, 2, 3}, Rows: captureRows})
	if !bytes.Equal(viaCapture, direct) {
		t.Fatalf("capture-built JSON = %q, want %q", viaCapture, direct)
	}
	if want := jsonStructureGolden(); !bytes.Equal(viaCapture, want) {
		t.Fatalf("capture-built JSON = %q, want %q", viaCapture, want)
	}
}

// TestJSONOrderStability proves the ascending traversal is by logical
// position, not source order: duplicate-valued or shuffled positions must
// still serialize ascending, and the source row slice order is preserved.
func TestJSONOrderStability(t *testing.T) {
	rows := [][]result.Value{
		{result.NewText("c")},
		{result.NewText("a")},
		{result.NewText("b")},
	}
	payload := Payload{
		Names:     []string{"v"},
		Positions: []int64{9, 2, 7},
		Rows:      rows,
	}
	want := []byte(`[{"v":"a"},{"v":"b"},{"v":"c"}]`)
	if got := JSON(payload); !bytes.Equal(got, want) {
		t.Fatalf("ordered JSON = %q, want %q", got, want)
	}
	// Input source order unchanged.
	if rows[0][0].Str != "c" || rows[1][0].Str != "a" || rows[2][0].Str != "b" {
		t.Errorf("source rows mutated: %+v", rows)
	}
}

// TestJSONWarningCombinations runs every Issue #49 metadata warning
// combination — every completeness label combination, every terminal
// outcome with and without a reason, retained ranges, byte-cap eviction,
// row-cap eviction, and the invalid-UTF flag — over the same tabular data
// and proves the exact JSON bytes are unchanged: no warning object, no
// warning property, no wrapper, and no extra key.
func TestJSONWarningCombinations(t *testing.T) {
	_, captureRows := jsonStructureRows()

	baseline := JSON(CaptureRows(jsonStructureColumns(), captureRows, 1, true,
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

				captured := CaptureRows(jsonStructureColumns(), captureRows, 1, true, combined, comp)
				got := JSON(captured.Payload)
				if !bytes.Equal(got, baseline) {
					t.Fatalf("metadata %+v / completeness %s changed the JSON:\n got %q\nwant %q",
						combined, comp, got, baseline)
				}
				assertNoWarningData(t, got)
				// Exactly one top-level array of len(rows) objects: no
				// wrapper objects or extra properties were added.
				var arr []map[string]any
				if err := json.Unmarshal(got, &arr); err != nil {
					t.Fatalf("output is not valid JSON: %v", err)
				}
				if len(arr) != len(captureRows) {
					t.Errorf("array length = %d, want %d", len(arr), len(captureRows))
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no warning combinations exercised")
	}
}
