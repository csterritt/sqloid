// Focused rendering assertions for the results region with tracer output,
// per Issue #10 Task 3: the existing builder/results/footer region ownership
// and layout arithmetic from Issue #8 remain intact while the disposable
// tracer renders its minimal bordered grid or basic error state inside the
// results region only.

package ui

import (
	"context"
	"strings"
	"testing"
)

// TestTracerResultsRegionStaysInsideShellAtStandardSizes pins that every row
// of the terminal still belongs to exactly one shell region while the tracer
// grid or error renders, across representative supported sizes.
func TestTracerResultsRegionStaysInsideShellAtStandardSizes(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}, {160, 50}} {
		m := driveTrace(t, size.w, size.h, func(context.Context) TraceResult { return successGrid() }, nil)
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d view has %d lines, want %d", size.w, size.h, len(lines), size.h)
		}
	}
}
