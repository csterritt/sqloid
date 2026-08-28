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

	valuePromptStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder())
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
	if m.terminalState != TerminalNone {
		// Deletion/replacement ended the session: the exact terminal message
		// replaces every region, overlay, and stale indicator.
		if m.terminalState == TerminalReplaced {
			return ReplacedSessionEndedMessage
		}
		return DeletedSessionEndedMessage
	}
	if m.Width <= 0 || m.Height <= 0 {
		return ""
	}
	l := CalculateLayout(m.Height, m.Fields)
	out := strings.Join([]string{
		m.renderResults(m.Width, l.ResultsHeight),
		m.renderBuilder(m.Width, l.BuilderHeight),
		m.renderFooter(m.Width),
	}, "\n")
	if m.Popup != nil {
		// Issue #8 overlay pattern: the popup draws over the results region
		// and never reflows any region's border or content rows.
		out = m.drawPopupOverlay(out)
	} else if m.ValuePrompt != nil {
		// Same overlay pattern for the universal value entry: exclusive with
		// popups, drawn without reflowing any region or growing the shell.
		out = m.drawValuePromptOverlay(out)
	}
	return out
}

// drawValuePromptOverlay composites the universal-entry box over the composed
// shell inside the results region, sized to its widest guidance line but
// capped to the terminal width minus one border column on each side.
func (m Model) drawValuePromptOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	lines := m.ValuePrompt.PromptLines(maxWidth, WhereTypedNullHint, WhereNullHelpLines())
	longest := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > longest {
			longest = w
		}
	}
	w := longest + 2
	if w < 4 {
		w = 4
	}
	if w > maxWidth {
		w = maxWidth
	}
	box := valuePromptStyle.Width(w).Height(len(lines)).Render(strings.Join(lines, "\n"))
	return composeOverlay(base, box, 1, 1)
}

// drawPopupOverlay composites the open popup box over the composed shell,
// anchored inside the results region just below its top border.
func (m Model) drawPopupOverlay(base string) string {
	maxWidth := m.Width - popupBorderCols
	if maxWidth < 1 {
		maxWidth = 1
	}
	extra := staleStatusLines(m.schemaStale, m.staleCause)
	content := renderPopupLines(m.Popup, maxWidth, extra...)
	box := RenderPopupBox(m.Popup, maxWidth, len(content)+popupBorderRows, extra)
	return composeOverlay(base, box, 1, 1)
}

// renderResults renders the independently bordered results region. Its fixed
// owned rows are the top/bottom border, one status/count line, and one frozen
// header row. While the stale-schema flow is active with no popup open it
// leads content with exactly the persistent stale status and inline cause
// lines; otherwise settled tracer output or the pre-execution placeholder is
// shown as before.
func (m Model) renderResults(width, height int) string {
	header := ""
	var content []string
	if m.schemaStale && m.Popup == nil && !m.validating {
		content = staleStatusLines(true, m.staleCause)
	} else if m.validating {
		// Issue #21: validation owns the results content — exact
		// `cancelling…` after a Ctrl+W request, the stale indicators after an
		// ordinary refresh failure, or `validating…` while a request is in
		// flight.
		content = m.validationStatusLines()
	} else {
		status := "Select a command (S/U/D/I) to begin"
		content = []string{status}
		if m.Trace != nil && m.Trace.Settled {
			content = []string{"tracer"}
			if m.Trace.Err != "" {
				content = append(content, m.Trace.Err)
			} else if g := m.Trace.Grid; g != nil {
				header = resultsHeaderStyle.Render(strings.Join(g.Headers, " | "))
				if header != "" {
					content = append(content, header)
				}
				for _, row := range g.Rows {
					content = append(content, strings.Join(row, " | "))
				}
			}
		}
	}
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
