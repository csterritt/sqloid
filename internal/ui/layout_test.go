package ui

import (
	"strings"
	"testing"
)

// largeFields returns builder fields whose content spans extraLines extra
// display lines distributed across separate multiline fields, each shorter
// than any supported viewport so focus scrolling can always show one whole.
func largeFields(extraLines int) []Field {
	fields := []Field{
		{Label: "Command", Content: "SELECT"},
		{Label: "Table", Content: "users"},
	}
	for i := 1; i <= extraLines; {
		lines := 3
		if remaining := extraLines - i + 1; remaining < lines {
			lines = remaining
		}
		var b strings.Builder
		for j := 1; j < lines; j++ {
			b.WriteString("\ncontinuation")
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fields = append(fields, Field{Label: "Column(s)", Content: strings.TrimPrefix(b.String(), "\n")})
		i += lines
	}
	return fields
}

func sumFieldLines(fields []Field) int {
	n := 0
	for _, f := range fields {
		n += f.Lines()
	}
	return n
}

func TestCalculateLayout(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		fields func() []Field
	}{
		{name: "80x24 minimal builder", width: 80, height: 24, fields: func() []Field { return []Field{{Label: "Command"}} }},
		{name: "100x30 minimal builder", width: 100, height: 30, fields: func() []Field { return []Field{{Label: "Command"}} }},
		{name: "160x50 minimal builder", width: 160, height: 50, fields: func() []Field { return []Field{{Label: "Command"}} }},
		{name: "80x24 growing builder", width: 80, height: 24, fields: func() []Field { return largeFields(6) }},
		{name: "100x30 growing builder", width: 100, height: 30, fields: func() []Field { return largeFields(10) }},
		{name: "160x50 growing builder", width: 160, height: 50, fields: func() []Field { return largeFields(20) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := tt.fields()
			h := tt.height

			if FooterHeight != 1 {
				t.Fatalf("FooterHeight = %d, want exactly one global footer row", FooterHeight)
			}

			l := CalculateLayout(h, fields)

			// Desired height includes the builder's own border (2) and padding (2).
			wantDesired := sumFieldLines(fields) + 2 + 2
			if l.BuilderDesired != wantDesired {
				t.Errorf("BuilderDesired = %d, want %d (content %d + border 2 + padding 2)",
					l.BuilderDesired, wantDesired, sumFieldLines(fields))
			}

			// Capped at floor(H/3).
			wantBuilder := wantDesired
			if cap := h / 3; wantBuilder > cap {
				wantBuilder = cap
			}
			if l.BuilderHeight != wantBuilder {
				t.Errorf("BuilderHeight = %d, want min(desired %d, floor(%d/3)) = %d",
					l.BuilderHeight, wantDesired, h, wantBuilder)
			}

			// Every remaining row goes to results; rows partition exactly.
			wantResults := h - FooterHeight - wantBuilder
			if l.ResultsHeight != wantResults {
				t.Errorf("ResultsHeight = %d, want H - footer 1 - builder %d = %d",
					l.ResultsHeight, wantBuilder, wantResults)
			}
			if FooterHeight+l.BuilderHeight+l.ResultsHeight != h {
				t.Errorf("regions %d+%d+%d do not partition H=%d",
					FooterHeight, l.BuilderHeight, l.ResultsHeight, h)
			}

			// Results remain greater than half-height at supported sizes.
			if !(l.ResultsHeight*2 > h) {
				t.Errorf("ResultsHeight %d does not exceed half of %d", l.ResultsHeight, h)
			}

			// Complete-row page area subtracts results' owned fixed rows:
			// top+bottom border, status/count line, frozen header.
			wantPage := wantResults - 4
			if l.PageRows != wantPage {
				t.Errorf("PageRows = %d, want ResultsHeight %d minus fixed rows 4 = %d",
					l.PageRows, wantResults, wantPage)
			}

			// The builder's interior viewport excludes its own border and padding.
			if got, want := l.BuilderViewport(), wantBuilder-4; got != want {
				t.Errorf("BuilderViewport = %d, want %d", got, want)
			}
		})
	}
}

func TestViewRegionOwnership(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		fields []Field
	}{
		{"80x24", 80, 24, largeFields(6)},
		{"100x30", 100, 30, largeFields(10)},
		{"160x50", 160, 50, largeFields(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.Fields = tt.fields
			m.Width, m.Height = tt.width, tt.height
			l := CalculateLayout(tt.height, m.Fields)

			view := m.View()
			lines := strings.Split(view, "\n")

			// Exactly one bottom global footer row and no extra rendering rows.
			if got := len(lines); got != tt.height {
				t.Fatalf("View renders %d rows, want exactly H=%d", got, tt.height)
			}
			if !strings.Contains(lines[len(lines)-1], "quit") || lines[len(lines)-1] == "" {
				t.Errorf("last row is not the one-row global footer: %q", lines[len(lines)-1])
			}

			builderBox := m.renderBuilder(tt.width, l.BuilderHeight)
			if got := lipglossLineCount(builderBox); got != l.BuilderHeight {
				t.Errorf("builder box occupies %d rows, want its owned %d", got, l.BuilderHeight)
			}
			resultsBox := m.renderResults(tt.width, l.ResultsHeight)
			if got := lipglossLineCount(resultsBox); got != l.ResultsHeight {
				t.Errorf("results box occupies %d rows, want its owned %d", got, l.ResultsHeight)
			}

			// Border ownership: only the rounded corners of each region appear
			// once per region edge; no border row is shared between regions.
			borderRows := 0
			for _, ln := range lines {
				if strings.Contains(ln, "╭") && strings.Contains(ln, "╮") {
					borderRows++
				}
			}
			if borderRows != 2 {
				t.Errorf("found %d top-border rows across View, want exactly two independently owned regions", borderRows)
			}

			// Page area: complete data rows available for paging stay consistent
			// with the frozen header and status line rendered inside results.
			resultsInner := lipglossLineCount(resultsBox) - 2
			if got := resultsInner - resultsStatusRows - resultsHeaderRows; got != l.PageRows {
				t.Errorf("results inner %d minus status/header = %d data rows, want PageRows %d",
					resultsInner, got, l.PageRows)
			}
		})
	}
}
