// Immutable export capture and Ctrl+X eligibility (Issue #49), per the
// Result export scope, Export warnings, and Export Module Design decisions
// in Notes/PRD-sqloid.md. A Capture is the export-owned immutable instant
// copy taken synchronously from the currently viewed tabular result before
// any picker work or later model mutation can run: deduplicated output
// names, every row in ascending one-based logical position order, typed
// cells with exact BLOB bytes, and the snapshot metadata carried separately
// from the serializable payload. The payload is the only part later CSV/JSON
// serializers (Issues #50/#51) may see — metadata drives UI warnings only
// and never becomes a row, column, object, property, or synthetic value.

package export

import (
	"errors"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// NoTabularDataMessage is the exact single shared rejection for a Ctrl+X
// press whose selection carries no tabular data (Issue #49): empty/missing
// selections, errors, write summaries, outcome-unknown entries, and
// cancelled-before-rows markers all report it; retained-row cancelled/failed
// tabular snapshots and zero-row SELECT snapshots with tabular columns stay
// eligible.
const NoTabularDataMessage = "selected result has no tabular data to export"

// ErrNoTabularData is the typed error carrying NoTabularDataMessage.
var ErrNoTabularData = errors.New(NoTabularDataMessage)

// EligibilityInput is the typed export-eligibility contract input: whether
// the current selection resolves to a backed tabular result. Targeting
// state (active, historical, terminal) is resolved by the UI; eligibility
// itself depends only on this fact.
type EligibilityInput struct {
	// BackedTabular reports that the selection is a currently backed tabular
	// result: an ordinary active SELECT page, a retained tabular snapshot, or
	// a terminal in-memory tabular selection — including retained-row
	// cancelled/failed snapshots and zero-row snapshots with tabular columns.
	BackedTabular bool
}

// Check reports whether the selection may export. A non-tabular selection is
// rejected with exactly ErrNoTabularData; the caller must change no state,
// open no picker, and serialize nothing.
func (in EligibilityInput) Check() error {
	if !in.BackedTabular {
		return ErrNoTabularData
	}
	return nil
}

// Payload is the export-owned serializable row payload: the full-set
// deduplicated output names, the ascending one-based logical positions, and
// the typed rows with exact BLOB bytes. It is deliberately the only
// serializer-visible part of a Capture: no metadata, warning, completeness,
// terminal-outcome, or invalid-UTF information can reach CSV/JSON output.
type Payload struct {
	Names     []string
	Positions []int64
	Rows      [][]result.Value
}

// Capture is one immutable export-owned instant copy of a tabular result.
// Payload holds the serializable data; Metadata and Completeness are carried
// separately for UI warnings only. All mutable inputs are deep-copied at
// construction, so later mutation of the live cache, history store, selection,
// or original byte slices can never alter a captured value.
type Capture struct {
	Payload      Payload
	Metadata     history.SnapshotMetadata
	Completeness history.Completeness
}

// CaptureRows builds a Capture from a backed tabular selection: the original
// driver output labels, the typed rows in ascending logical-position order,
// the first row's one-based logical position (meaningful only when
// hasStart), and the snapshot metadata/completeness facts. Columns are
// deduplicated once through result.DeduplicateNames, rows and BLOB bytes are
// deep-copied, and positions ascend from start (1 when hasStart is false).
func CaptureRows(columns []string, rows [][]result.Value, start int64, hasStart bool, meta history.SnapshotMetadata, comp history.Completeness) Capture {
	if start < 1 {
		start = 1
	}
	payload := Payload{
		Names:     result.DeduplicateNames(columns),
		Positions: make([]int64, len(rows)),
		Rows:      copyExportRows(rows),
	}
	for i := range payload.Positions {
		payload.Positions[i] = start + int64(i)
	}
	return Capture{Payload: payload, Metadata: meta, Completeness: comp}
}

// copyExportRows deep-copies a typed rows slice, including every BLOB's
// bytes, so the capture never aliases caller storage.
func copyExportRows(rows [][]result.Value) [][]result.Value {
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
