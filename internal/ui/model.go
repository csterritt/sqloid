// Package ui implements the Sqloid Bubble Tea model: responsive shell layout,
// the query-builder field bar, the results region, focus scrolling, and the
// below-minimum terminal suspension contract from Issue #8.
//
// Database behavior stays behind the Connection composition seam; this package
// contains no database logic.
package ui

import tea "github.com/charmbracelet/bubbletea"

// Minimum supported terminal dimensions. Below either threshold the shell
// suspends its state behind an exact message instead of rendering a malformed
// layout.
const (
	MinWidth  = 80
	MinHeight = 24

	// TooSmallMessage is rendered verbatim while the terminal is undersized.
	TooSmallMessage = "terminal too small"
)

// FooterHeight is the exact number of bottom global footer rows reserved at
// every supported height.
const FooterHeight = 1

// Field is one labeled builder field. Content may contain multiple lines;
// each line counts toward the builder's desired height.
type Field struct {
	Label   string
	Content string
}

// Lines returns the number of display lines the field's content occupies.
func (f Field) Lines() int {
	if f.Content == "" {
		return 1
	}
	n := 1
	for _, r := range f.Content {
		if r == '\n' {
			n++
		}
	}
	return n
}

// Model is the top-level Bubble Tea model. While the terminal is undersized,
// suspended holds a complete copy of the pre-suspension model so an exact
// resize restoration can replace the whole model without reconstruction.
type Model struct {
	Width  int
	Height int

	Fields []Field
	Focus  int // index into Fields of the focused field
	Scroll int // first visible interior content line of the builder

	// ActiveCancellable reports whether hidden state owns active cancellable
	// work owned through the future Connection seam. Ctrl+W routes to
	// CancelCommand only while it is true.
	ActiveCancellable bool
	// CancelCommand produces the generic cancellation message for the owned
	// request. It is invoked as a tea.Cmd, never directly inside Update.
	CancelCommand func() tea.Msg

	// Trace holds the disposable Issue #10 tracer state (hardcoded SELECT *
	// integration milestone), isolated from all builder and shell state so
	// Issue #22 can replace it wholesale. nil before any trace started.
	Trace *TraceView

	suspended      bool   // true while the terminal is below minimum size
	suspendedModel *Model // exact copy retained across the undersized period
}

// New returns the initial model focused on the Command field.
func New() Model {
	return Model{
		Fields: []Field{{Label: "Command", Content: ""}},
		Focus:  0,
	}
}

// Init implements tea.Model. Terminal dimensions arrive via WindowSizeMsg.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. It handles window resizes, builder focus
// movement, and the gated input allowed while the terminal is undersized.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height), nil
	case StartTraceMsg:
		return m, func() tea.Msg { return handleStartTrace(msg) }
	case traceSettledMsg:
		return m.applyTraceResult(msg.result), nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// resize applies new terminal dimensions. Below minimum it preserves the
// entire current model unchanged behind TooSmallMessage; returning to
// supported dimensions restores that exact state and then lays out normally.
func (m Model) resize(w, h int) Model {
	m.Width, m.Height = w, h
	if tooSmall(w, h) {
		if !m.suspended {
			copied := m
			m.suspended = true
			m.suspendedModel = &copied
		}
		return m
	}
	if m.suspended {
		if m.suspendedModel != nil {
			restored := *m.suspendedModel
			restored.suspended = false
			restored.suspendedModel = nil
			restored.Width, restored.Height = w, h
			restored.clampScroll()
			return restored
		}
		m.suspended = false
		m.suspendedModel = nil
	}
	m.clampScroll()
	return m
}

// handleKey dispatches key input according to the suspension gate. Ordinary
// keys are ignored while undersized so hidden state can neither leak through
// nor be mutated; Ctrl+W alone may route to cancellation when hidden state
// owns active cancellable work.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.suspended {
		if msg.String() == "ctrl+w" && m.ActiveCancellable && m.CancelCommand != nil {
			return m, m.CancelCommand
		}
		return m, nil
	}
	switch msg.String() {
	case "tab":
		m.setFocus(m.Focus + 1)
	case "shift+tab":
		m.setFocus(m.Focus - 1)
	case "up":
		m.setFocus(m.Focus - 1)
	case "down":
		m.setFocus(m.Focus + 1)
	default:
		return m, nil
	}
	m.adjustScroll()
	return m, nil
}

// setFocus moves focus within bounds without mutating anything else.
func (m *Model) setFocus(i int) {
	if i < 0 {
		i = 0
	}
	if i >= len(m.Fields) {
		i = len(m.Fields) - 1
	}
	m.Focus = i
}
