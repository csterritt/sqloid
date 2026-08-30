// RFC 4180 CSV byte-golden structure coverage for Issue #50, per the
// Export formats and values, Output names, Invalid UTF-8 TEXT, Export
// Module Design, and Testing Decisions decisions in Notes/PRD-sqloid.md.
// Every test pins exact output bytes over immutable inputs built from
// shared internal/result rows and full-set deduplicated output names:
// exactly one deduplicated header record, rows serialized by ascending
// logical position without mutating the input, CRLF after every record,
// minimal quoting only for fields containing a comma, double quote, CR,
// or LF, quote doubling, and preserved embedded content — tabs alone stay
// unquoted. Metadata warning combinations run the same data and must
// leave the bytes unchanged, with no warning row, column, prefix,
// comment, or altered header. Typed SQLite value policy is covered in
// csv_value_test.go.

package export

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// structureColumns carries duplicate labels, an original pre-suffixed
// label that collides with the deduplicated duplicate, and a computed
// SQLite label, exactly as an aggregate SELECT could emit them.
func structureColumns() []string {
	return []string{"id", "name", "name", "note", "name_2", "count(*)"}
}

// structureRows builds the same three tabular rows twice: once as raw
// input for direct Payload construction with deliberately nonascending
// positions, and once in capture order for CaptureRows. Rows are listed
// here in nonascending source order (position 3 first) to prove the
// serializer traverses ascending logical positions.
func structureRows() (unorderedRows [][]result.Value, captureRows [][]result.Value) {
	// Position 3: empty and NULL fields only.
	pos3 := []result.Value{
		result.NewText(""),
		result.NewNull(),
		result.NewText(""),
		result.NewNull(),
		result.NewText(""),
		result.NewText(""),
	}
	// Position 1: ordinary ASCII, UTF-8, empty fields, and a bare tab.
	pos1 := []result.Value{
		result.NewText("plain"),
		result.NewText(""),
		result.NewText("tab\there"),
		result.NewNull(),
		result.NewNull(),
		result.NewText("utf8 héllo"),
	}
	// Position 2: commas, quotes, CR, LF, CRLF, and a quote-doubling case.
	pos2 := []result.Value{
		result.NewText("a,b"),
		result.NewText("say \"hi\""),
		result.NewText("cr\rlf"),
		result.NewText("crlf\r\nend"),
		result.NewNull(),
		result.NewText(""),
	}
	unorderedRows = [][]result.Value{pos3, pos1, pos2}
	captureRows = [][]result.Value{pos1, pos2, pos3}
	return unorderedRows, captureRows
}

// structureNames is the full-set deduplicated output name set for
// structureColumns, exactly as the frozen grid header renders them.
func structureNames() []string {
	return result.DeduplicateNames(structureColumns())
}

// structureGolden is the exact expected CSV for structureRows under the
// ascending positions 1, 2, 3: one header, then rows 1, 2, 3, each record
// CRLF-terminated, with minimal quoting and quote doubling.
func structureGolden() []byte {
	var b bytes.Buffer
	b.WriteString("id,name,name_3,note,name_2,count(*)\r\n")
	b.WriteString("plain,,tab\there,,,utf8 héllo\r\n")
	b.WriteString("\"a,b\",\"say \"\"hi\"\"\",\"cr\rlf\",\"crlf\r\nend\",,\r\n")
	b.WriteString(",,,,,\r\n")
	return b.Bytes()
}

// TestCSVStructureGolden pins the exact bytes: one deduplicated header
// record, rows ordered by ascending logical position (the input was built
// in nonascending order), CRLF after every record, minimal quoting only
// for fields containing comma, quote, CR, or LF, quote doubling, embedded
// tabs preserved unquoted, and NULL/empty both as empty fields.
func TestCSVStructureGolden(t *testing.T) {
	unorderedRows, _ := structureRows()
	names := structureNames()
	positions := []int64{3, 1, 2}
	payload := Payload{Names: names, Positions: positions, Rows: unorderedRows}
	got := CSV(payload)
	if want := structureGolden(); !bytes.Equal(got, want) {
		t.Fatalf("CSV bytes diverged:\n got %q\nwant %q", got, want)
	}
}

// TestCSVDoesNotMutateInput proves serialization leaves the immutable
// capture untouched: names, positions, rows, and every BLOB byte slice
// must compare equal after a repeated serialization, and repeated
// serialization must be byte-identical.
func TestCSVDoesNotMutateInput(t *testing.T) {
	unorderedRows, _ := structureRows()
	payload := Payload{
		Names:     structureNames(),
		Positions: []int64{3, 1, 2},
		Rows:      unorderedRows,
	}
	beforeRows := copyRowsForComparison(unorderedRows)
	beforePositions := append([]int64(nil), payload.Positions...)
	beforeNames := append([]string(nil), payload.Names...)

	first := CSV(payload)
	second := CSV(payload)
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated serialization diverged:\nfirst %q\nsecond %q", first, second)
	}
	if !reflect.DeepEqual(beforeNames, payload.Names) {
		t.Errorf("names mutated: %q", payload.Names)
	}
	if !reflect.DeepEqual(beforePositions, payload.Positions) {
		t.Errorf("positions mutated: %v", payload.Positions)
	}
	if !reflect.DeepEqual(beforeRows, unorderedRows) {
		t.Error("rows mutated by serialization")
	}
}

// copyRowsForComparison deep-copies typed rows so pre/post serialization
// comparisons cannot alias the payload's storage.
func copyRowsForComparison(rows [][]result.Value) [][]result.Value {
	out := make([][]result.Value, len(rows))
	for i, row := range rows {
		copied := make([]result.Value, len(row))
		for j, v := range row {
			if v.Kind == result.KindBlob {
				v.Bytes = append([]byte(nil), v.Bytes...)
			}
			copied[j] = v
		}
		out[i] = copied
	}
	return out
}

// TestCSVZeroRows requires exactly one header record and no data records
// for a zero-row capture: the output is only the deduplicated header's
// CRLF-terminated record.
func TestCSVZeroRows(t *testing.T) {
	payload := Payload{
		Names:     []string{"a", "b,c"},
		Positions: []int64{},
		Rows:      [][]result.Value{},
	}
	want := []byte("a,\"b,c\"\r\n")
	if got := CSV(payload); !bytes.Equal(got, want) {
		t.Fatalf("zero-row CSV = %q, want %q", got, want)
	}
}

// TestCSVOneRow covers the single-row shape: header plus exactly one
// CRLF-terminated data record.
func TestCSVOneRow(t *testing.T) {
	payload := Payload{
		Names:     []string{"x"},
		Positions: []int64{1},
		Rows:      [][]result.Value{{result.NewText("only")}},
	}
	want := []byte("x\r\nonly\r\n")
	if got := CSV(payload); !bytes.Equal(got, want) {
		t.Fatalf("one-row CSV = %q, want %q", got, want)
	}
}

// TestCSVHeaderQuoting requires the header to follow the same minimal
// quoting rule as data fields: a header label containing a comma, quote,
// CR, or LF is quoted with doubled quotes; ordinary labels (including
// tabs) stay unquoted.
func TestCSVHeaderQuoting(t *testing.T) {
	payload := Payload{
		Names:     []string{"plain", "with,comma", "with\"quote", "with\r\nnewline", "with\ttab"},
		Positions: []int64{},
		Rows:      [][]result.Value{},
	}
	want := []byte("plain,\"with,comma\",\"with\"\"quote\",\"with\r\nnewline\",with\ttab\r\n")
	if got := CSV(payload); !bytes.Equal(got, want) {
		t.Fatalf("header CSV = %q, want %q", got, want)
	}
}

// TestCSVTabAloneUnquoted pins the minimal-quoting boundary: a field
// consisting of only a tab — and any field whose special content is only
// tabs — is never quoted, while the tab bytes are preserved exactly.
func TestCSVTabAloneUnquoted(t *testing.T) {
	payload := Payload{
		Names:     []string{"t"},
		Positions: []int64{1, 2},
		Rows: [][]result.Value{
			{result.NewText("\t")},
			{result.NewText("a\tb\tc")},
		},
	}
	want := []byte("t\r\n\t\r\na\tb\tc\r\n")
	if got := CSV(payload); !bytes.Equal(got, want) {
		t.Fatalf("tab CSV = %q, want %q", got, want)
	}
}

// TestCSVAscendingFromCaptureRows runs the same tabular data through the
// Issue #49 CaptureRows constructor, whose positions ascend from the
// selection's first retained position, and requires identical bytes to
// the direct-payload golden.
func TestCSVAscendingFromCaptureRows(t *testing.T) {
	_, captureRows := structureRows()
	captured := CaptureRows(structureColumns(), captureRows, 1, true,
		history.SnapshotMetadata{}, history.Completeness{Complete: true})
	if got, want := CSV(captured.Payload), structureGolden(); !bytes.Equal(got, want) {
		t.Fatalf("captured CSV = %q, want %q", got, want)
	}
}

// TestCSVWarningCombinations runs every Issue #49 metadata warning
// combination — every completeness label combination, every terminal
// outcome with and without a reason, retained ranges, byte-cap eviction,
// row-cap eviction, and the invalid-UTF flag — over the same tabular data
// and proves the exact CSV bytes are unchanged: no warning row, no
// warning column, no prefix, no comment, and no altered header.
func TestCSVWarningCombinations(t *testing.T) {
	_, captureRows := structureRows()

	baseline := CSV(CaptureRows(structureColumns(), captureRows, 1, true,
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

				captured := CaptureRows(structureColumns(), captureRows, 1, true, combined, comp)
				got := CSV(captured.Payload)
				if !bytes.Equal(got, baseline) {
					t.Fatalf("metadata %+v / completeness %s changed the CSV:\n got %q\nwant %q",
						combined, comp, got, baseline)
				}
				assertNoWarningData(t, got)
				// Column count: the header record is the deduplicated names
				// alone, with exactly len(names) comma-separated fields.
				header := string(got[:bytes.Index(got, []byte("\r\n"))])
				if want := strings.Join(structureNames(), ","); header != want {
					t.Errorf("header = %q, want the deduplicated names alone %q", header, want)
				}
				// Row count: one header plus one record per data row. The
				// structure data embeds one CRLF inside a quoted field, so the
				// byte count is 4 terminators plus that 1 preserved CRLF.
				if n := bytes.Count(got, []byte("\r\n")); n != 5 {
					t.Errorf("record terminator count = %d, want 4 records + 1 embedded CRLF", n)
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no warning combinations exercised")
	}
}

// assertNoWarningData proves no designated warning or metadata string
// from the shared result/history packages appears anywhere in the bytes.
func assertNoWarningData(t *testing.T, got []byte) {
	t.Helper()
	forbidden := []string{
		result.ByteCapWarning,
		result.UTFWarning,
		"Result is complete",
		"Result is partial",
		"Result is truncated",
		"Rows evicted",
		"Cancelled",
		"interrupted",
		"disk I/O",
		"complete",
	}
	for _, f := range forbidden {
		if strings.Contains(string(got), f) {
			t.Errorf("CSV leaked warning metadata %q in output %q", f, got)
		}
	}
}

// TestCSVOrderStability proves the ascending traversal is by logical
// position, not source order: duplicate-valued or shuffled positions must
// still serialize ascending, and the source row slice order is preserved.
func TestCSVOrderStability(t *testing.T) {
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
	want := []byte("v\r\na\r\nb\r\nc\r\n")
	if got := CSV(payload); !bytes.Equal(got, want) {
		t.Fatalf("ordered CSV = %q, want %q", got, want)
	}
	// Input source order unchanged.
	if rows[0][0].Str != "c" || rows[1][0].Str != "a" || rows[2][0].Str != "b" {
		t.Errorf("source rows mutated: %+v", rows)
	}
}
