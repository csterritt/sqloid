// Ctrl+X export-capture, eligibility, and payload-separation coverage for
// Issue #49, per the Result export scope, Export warnings, and Export Module
// Design decisions in Notes/PRD-sqloid.md. The tests pin the immutable
// instant-copy contract: deduplicated output names, ascending one-based
// logical positions, every typed value including exact BLOB bytes, and
// snapshot metadata carried separately from the serializable payload — so
// later mutation of the original rows, byte slices, or live sources can
// never alter a captured copy, and metadata can never reach CSV/JSON
// serializers. The exact shared non-tabular rejection
// `selected result has no tabular data to export` is defined only here.

package export

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// captureFixture builds one tabular selection's inputs: duplicate and empty
// labels, typed cells with NULL/empty distinctions, a BLOB whose payload
// aliases src (so later caller mutation can be proven harmless), and an
// invalid-UTF TEXT value.
func captureFixture() (columns []string, rows [][]result.Value, blobSrc []byte) {
	blobSrc = []byte{0xDE, 0xAD, 0xBE, 0xEF}
	columns = []string{"", "id", "id", "blob", "txt", "v", "v_2"}
	rows = [][]result.Value{{
		result.NewText("a\x80b"),
		result.NewInteger(1),
		result.NewReal(1.0),
		{Kind: result.KindBlob, Bytes: blobSrc},
		result.NewText("t"),
		result.NewNull(),
		result.NewText(""),
	}, {
		result.NewText(""),
		result.NewInteger(2),
		result.NewReal(-0.0),
		{Kind: result.KindBlob, Bytes: []byte(nil)},
		result.NewText("t"),
		result.NewInteger(3),
		result.NewNull(),
	}}
	return columns, rows, blobSrc
}

// wantNames is the full-set deduplicated name set for captureFixture's
// columns, exactly as the frozen grid renders them.
func wantNames() []string {
	return result.DeduplicateNames([]string{"", "id", "id", "blob", "txt", "v", "v_2"})
}

// TestCaptureRowsImmutableSnapshot pins the capture contract: synchronous
// deep-copy of names, positions, and typed values including BLOB bytes; an
// ascending one-based position sequence starting at the selection's first
// retained position; and full independence from later caller mutation.
func TestCaptureRowsImmutableSnapshot(t *testing.T) {
	columns, rows, blobSrc := captureFixture()
	start := int64(7)
	captured := CaptureRows(columns, rows, start, true, history.SnapshotMetadata{
		HasRetainedRange: true, RetainedStart: 7, RetainedEnd: 8,
		InvalidUTF: true, TruncatedByByteCap: true,
		Outcome: history.OutcomeFailed, Reason: "disk full",
	}, history.Completeness{Partial: true, Truncated: true})

	// Mutate every original source after the capture: labels, row slices,
	// cells, the BLOB's original byte slice, and the rows slice itself.
	columns[1] = "mutated"
	rows[0][1] = result.NewInteger(999)
	rows[0][3].Bytes[0] = 0x00
	blobSrc[0] = 0x00
	rows[0] = nil
	rows = append(rows, []result.Value{result.NewInteger(4)})

	wantRows := captureFixtureRows()
	if !reflect.DeepEqual(captured.Payload.Names, wantNames()) {
		t.Errorf("captured names = %q, want %q", captured.Payload.Names, wantNames())
	}
	if captured.Payload.Names[0] != "" || captured.Payload.Names[2] != "id_2" || captured.Payload.Names[5] != "v" || captured.Payload.Names[6] != "v_2" {
		t.Errorf("deduplication wrong in capture: %q", captured.Payload.Names)
	}
	for i, p := range captured.Payload.Positions {
		if p != start+int64(i) {
			t.Fatalf("position %d = %d, want ascending from %d", i, p, start)
		}
	}
	if !reflect.DeepEqual(captured.Payload.Rows, wantRows) {
		t.Errorf("captured rows diverged from the pre-mutation values:\n got %+v\nwant %+v", captured.Payload.Rows, wantRows)
	}
	blob := captured.Payload.Rows[0][3]
	if blob.Kind != result.KindBlob || string(blob.Bytes) != "\xDE\xAD\xBE\xEF" {
		t.Errorf("captured BLOB bytes = %x, want deadbeef", blob.Bytes)
	}
	if !captured.Metadata.InvalidUTF || captured.Metadata.Outcome != history.OutcomeFailed {
		t.Errorf("captured metadata lost: %+v", captured.Metadata)
	}
	if !captured.Completeness.Partial || !captured.Completeness.Truncated {
		t.Errorf("captured completeness lost: %+v", captured.Completeness)
	}
}

// captureFixtureRows rebuilds the fixture's rows exactly as they stood at
// capture time, for the immutability comparison above.
func captureFixtureRows() [][]result.Value {
	_, rows, _ := captureFixture()
	return rows
}

// TestCaptureRowsDefaultPositions requires an unanchored selection's
// positions to ascend from one.
func TestCaptureRowsDefaultPositions(t *testing.T) {
	_, rows, _ := captureFixture()
	captured := CaptureRows([]string{"id"}, rows[:1], 0, false, history.SnapshotMetadata{}, history.Completeness{Complete: true})
	if len(captured.Payload.Positions) != 1 || captured.Payload.Positions[0] != 1 {
		t.Errorf("positions = %v, want [1]", captured.Payload.Positions)
	}
}

// TestExportEligibility is the pure eligibility matrix: only a backed
// tabular selection is eligible; every non-tabular selection is rejected
// with exactly the one shared Issue #49 error.
func TestExportEligibility(t *testing.T) {
	tests := []struct {
		name    string
		input   EligibilityInput
		wantErr bool
	}{
		{name: "backed tabular selection", input: EligibilityInput{BackedTabular: true}},
		{name: "empty or missing-backed selection", input: EligibilityInput{}, wantErr: true},
		{name: "error view", input: EligibilityInput{}, wantErr: true},
		{name: "write summary", input: EligibilityInput{}, wantErr: true},
		{name: "outcome-unknown entry", input: EligibilityInput{}, wantErr: true},
		{name: "cancelled-before-rows marker", input: EligibilityInput{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Check()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Check(%+v) = %v, want nil", tt.input, err)
				}
				return
			}
			if err == nil {
				t.Fatal("Check accepted a non-tabular selection")
			}
			if err != ErrNoTabularData || err.Error() != NoTabularDataMessage {
				t.Fatalf("error = %v, want the single shared definition %q", err, NoTabularDataMessage)
			}
			if NoTabularDataMessage != "selected result has no tabular data to export" {
				t.Fatalf("rejection literal = %q, want the exact Issue #49 text", NoTabularDataMessage)
			}
		})
	}
}

// TestCapturePayloadExcludesMetadata is the serializer-spy contract: the
// Payload handed to later CSV/JSON writers carries only names, positions,
// and rows — never the metadata, completeness, warning, terminal-outcome, or
// invalid-UTF facts, none of which may appear as a row, column, property, or
// synthetic value.
func TestCapturePayloadExcludesMetadata(t *testing.T) {
	// Structural: the payload type owns exactly the serializable fields.
	pt := reflect.TypeOf(Payload{})
	if pt.NumField() != 3 {
		t.Fatalf("Payload has %d fields, want exactly names/positions/rows", pt.NumField())
	}
	for _, want := range []string{"Names", "Positions", "Rows"} {
		if _, ok := pt.FieldByName(want); !ok {
			t.Errorf("Payload lacks field %s", want)
		}
	}

	columns, rows, _ := captureFixture()
	captured := CaptureRows(columns, rows, 1, true, history.SnapshotMetadata{
		HasRetainedRange:   true,
		RetainedStart:      1,
		RetainedEnd:        2,
		RowCapEvicted:      true,
		RowCapEvictions:    3,
		TruncatedByByteCap: true,
		InvalidUTF:         true,
		Outcome:            history.OutcomeCancelled,
		Reason:             "interrupted",
	}, history.Completeness{Partial: true, Truncated: true})

	// Behavioral: no warning/outcome/completeness string appears anywhere in
	// the payload's names or cell tokens.
	forbidden := []string{
		result.ByteCapWarning, result.UTFWarning,
		"Result is complete", "Result is partial", "Result is truncated",
		"Rows evicted by the position cap",
		"Cancelled", "interrupted", "last failure at row",
	}
	spy := func(cells []string) {
		for _, cell := range cells {
			for _, f := range forbidden {
				if strings.Contains(cell, f) {
					t.Errorf("payload leaked warning metadata %q as a serializer value", f)
				}
			}
		}
	}
	spy(captured.Payload.Names)
	for _, row := range captured.Payload.Rows {
		tokens := make([]string, len(row))
		for i, v := range row {
			tokens[i] = v.Display()
		}
		spy(tokens)
	}
	if len(captured.Payload.Positions) != 2 || captured.Payload.Positions[0] != 1 || captured.Payload.Positions[1] != 2 {
		t.Errorf("positions = %v, want [1 2]", captured.Payload.Positions)
	}
}
