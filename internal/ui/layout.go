package ui

// Layout is the pure row arithmetic of the responsive shell. Every row of the
// terminal belongs to exactly one region: the results region owns its border,
// status/count line, and frozen header; the builder owns its border and
// padding; one global footer row sits at the bottom. No border row is shared
// or overlapping.
type Layout struct {
	TotalHeight int

	// BuilderDesired is the builder's full desired height including its own
	// border and padding.
	BuilderDesired int
	// BuilderHeight is BuilderDesired capped at floor(TotalHeight/3).
	BuilderHeight int

	// ResultsHeight covers every remaining row after footer and builder,
	// including the results-owned border. It always exceeds half of
	// TotalHeight at supported heights.
	ResultsHeight int

	// PageRows is the exact number of complete data rows available for paging:
	// ResultsHeight minus its owned fixed rows (top/bottom border, status/count
	// line, frozen header).
	PageRows int
}

// Fixed layout constants owned by each region.
const (
	builderBorderRows  = 2 // top and bottom border rows owned by the builder
	builderPaddingRows = 2 // one interior padding row above and below content
	resultsBorderRows  = 2 // top and bottom border rows owned by results
	resultsStatusRows  = 1 // status/count line inside the results border
	resultsHeaderRows  = 1 // frozen header inside the results border

	resultsFixedRows = resultsBorderRows + resultsStatusRows + resultsHeaderRows
)

// CalculateLayout computes the exact shell arithmetic for a supported total
// height and the current builder fields.
func CalculateLayout(totalHeight int, fields []Field) Layout {
	desired := DesiredBuilderHeight(fields)
	capped := totalHeight / 3
	builder := desired
	if builder > capped {
		builder = capped
	}
	if builder < 1 {
		builder = 1
	}
	results := totalHeight - FooterHeight - builder
	page := results - resultsFixedRows
	if page < 0 {
		page = 0
	}
	return Layout{
		TotalHeight:    totalHeight,
		BuilderDesired: desired,
		BuilderHeight:  builder,
		ResultsHeight:  results,
		PageRows:       page,
	}
}

// DesiredBuilderHeight returns the builder's desired height including its own
// border (2 rows) and padding (2 rows) around the summed field content lines.
func DesiredBuilderHeight(fields []Field) int {
	content := 0
	for _, f := range fields {
		content += f.Lines()
	}
	h := content + builderBorderRows + builderPaddingRows
	if h < 1 {
		h = 1
	}
	return h
}

// BuilderViewport returns the number of interior content lines visible when
// the builder renders at its capped height: the height minus its border and
// padding.
func (l Layout) BuilderViewport() int {
	v := l.BuilderHeight - builderBorderRows - builderPaddingRows
	if v < 0 {
		v = 0
	}
	return v
}

// tooSmall reports whether either supported-size threshold is violated.
func tooSmall(w, h int) bool {
	return w < MinWidth || h < MinHeight
}

// fieldSpans returns, for the given fields, the start line index and line
// count of every field within the builder's interior content area.
func fieldSpans(fields []Field) (starts, counts []int) {
	starts = make([]int, len(fields))
	counts = make([]int, len(fields))
	at := 0
	for i, f := range fields {
		starts[i] = at
		counts[i] = f.Lines()
		at += counts[i]
	}
	return starts, counts
}

// adjustScroll moves the scroll offset so the complete focused field —
// including its full multiline extent — remains visible inside the builder's
// viewport, clamping to [0, maxScroll].
func (m *Model) adjustScroll() {
	starts, counts := fieldSpans(m.Fields)
	total := 0
	for _, c := range counts {
		total += c
	}
	viewport := CalculateLayout(m.Height, m.Fields).BuilderViewport()
	if viewport >= total {
		m.Scroll = 0
		return
	}
	maxScroll := total - viewport
	start, count := starts[m.Focus], counts[m.Focus]
	scroll := m.Scroll
	if start < scroll {
		scroll = start
	}
	if start+count > scroll+viewport {
		scroll = start + count - viewport
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	m.Scroll = scroll
}

// clampScroll keeps the offset valid after a resize shrinks the viewport.
func (m *Model) clampScroll() {
	m.adjustScroll()
}
