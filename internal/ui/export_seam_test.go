// Grid/exporter name-equivalence coverage for Issue #47: the frozen grid
// (internal/ui) and the exporter-facing boundary (internal/export) must
// receive exactly the same full-set deduplicated final names in column
// order, from the single internal/result calculation, without driver
// metadata changing.

package ui

import (
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func TestGridAndExporterNamesIdentical(t *testing.T) {
	page := &result.Page{
		Columns: []string{"COUNT(*)", "COUNT(*)", "", "v", "v_2"},
		Rows: [][]result.Value{{
			result.NewInteger(1), result.NewInteger(2), result.NewInteger(3), result.NewInteger(4), result.NewInteger(5),
		}},
	}
	m := resultModel(t, page, nil)
	view := m.View()

	exporterNames := export.OutputNames(*page)
	// The grid pads header cells to column width, so compare token-wise.
	var headerLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, exporterNames[0]) {
			headerLine = line
			break
		}
	}
	if headerLine == "" {
		t.Fatalf("grid header missing exporter-facing name %q:\n%s", exporterNames[0], view)
	}
	cells := strings.Split(headerLine, "|")
	if len(cells) != len(exporterNames) {
		t.Fatalf("grid header has %d cells, want %d:\n%s", len(cells), len(exporterNames), view)
	}
	for i := range exporterNames {
		if got := strings.Trim(cells[i], " ││"); got != exporterNames[i] {
			t.Errorf("grid header cell %d = %q, want exporter-facing name %q", i, got, exporterNames[i])
		}
	}
	// The original labels stay exactly as the driver returned them.
	wantColumns := []string{"COUNT(*)", "COUNT(*)", "", "v", "v_2"}
	for i, want := range wantColumns {
		if page.Columns[i] != want {
			t.Errorf("driver metadata mutated at %d: %q, want %q", i, page.Columns[i], want)
		}
	}
}
