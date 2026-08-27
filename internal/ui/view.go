package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Semantic styles. Color is never the only signal; borders and labels keep the
// regions distinguishable with color disabled.
var (
	builderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 0)

	resultsStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder())

	resultsHeaderStyle = lipgloss.NewStyle().Bold(true)
)

// View implements tea.Model. Rendering is deterministic for a given model and
// dimensions: results region on top (owning its border, status line, and
// frozen header), builder below it (owning its border and padding), and
// exactly one global footer row at the bottom. While suspended, View returns
// exactly TooSmallMessage and performs no state access.
func (m Model) View() string {
	if m.suspended {
		return TooSmallMessage
	}
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	l := CalculateLayout(m.Height, m.Fields)
	return strings.Join([]string{
		m.renderResults(m.Width, l.ResultsHeight),
		m.renderBuilder(m.Width, l.BuilderHeight),
		m.renderFooter(m.Width),
	}, "\n")
}

// renderResults renders the independently bordered results region. Its fixed
// owned rows are the top/bottom border, one status/count line, and one frozen
// header row.
func (m Model) renderResults(width, height int) string {
	status := "Select a command (S/U/D/I) to begin"
	header := ""
	content := []string{status}
	if header != "" {
		content = append(content, resultsHeaderStyle.Render(header))
	}
	content = append(content, "")
	box := resultsStyle.
		Width(width - resultsBorderRows).
		Height(height - resultsBorderRows).
		Render(strings.Join(content, "\n"))
	return box
}

// renderBuilder renders the bordered, padded builder bar showing the visible
// window of fields starting at Scroll so the complete focused field stays in
// view.
func (m Model) renderBuilder(width, height int) string {
	starts, _ := fieldSpans(m.Fields)
	viewport := height - builderBorderRows - builderPaddingRows
	lines := make([]string, 0, viewport)
	for i, f := range m.Fields {
		fieldLines := renderFieldLines(f)
		for j, fl := range fieldLines {
			lineIndex := starts[i] + j
			if lineIndex < m.Scroll || lineIndex >= m.Scroll+viewport {
				continue
			}
			if i == m.Focus {
				fl = "> " + fl
			} else {
				fl = "  " + fl
			}
			lines = append(lines, fl)
		}
	}
	for len(lines) < viewport {
		lines = append(lines, "")
	}
	return builderStyle.
		Width(width - builderBorderRows).
		Height(height - builderBorderRows).
		Render(strings.Join(lines, "\n"))
}

// renderFieldLines splits a field into its display lines, prefixing its label.
func renderFieldLines(f Field) []string {
	lines := strings.Split(f.Content, "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		if i == 0 {
			out[i] = f.Label + ": " + l
		} else {
			out[i] = l
		}
	}
	return out
}

// renderFooter renders the single global footer row.
func (m Model) renderFooter(width int) string {
	text := " q quit   ? help "
	return lipgloss.PlaceHorizontal(width, lipgloss.Left, text)
}
