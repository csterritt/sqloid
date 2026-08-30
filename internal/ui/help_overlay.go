// Contextual help overlay (Issue #54), per the Global Key Precedence and
// Context/Action Matrix in Notes/PRD-sqloid.md. One nonstacking help overlay
// opens only from eligible base contexts, classified from typed builder,
// result, and terminal state — never from display strings. While it is open
// it consumes every key above every other context; Esc restores the exact
// opener snapshot atomically. Repeated `?` never stacks a second overlay, and
// opening or closing help never mutates history, request, viewport, save, or
// export state and never finalizes an active SELECT.

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Typed opener-context identifiers for the contextual help overlay. They are
// selected from model state, never from rendered labels.
const (
	// helpKindBuilder is the ordinary builder context: no result is displayed
	// and no WHERE field is focused.
	helpKindBuilder = "builder"
	// helpKindWhere names a WHERE value/operator context, whose help carries
	// the required SQL-NULL guidance.
	helpKindWhere = "where"
	// helpKindResult is a result view with independent count state (or its
	// selected finalized snapshot), whose help explains count semantics.
	helpKindResult = "result"
)

// classifyHelpContext returns the typed help context for the current base
// state. Typed state gates decide — focus labels, retained result state, and
// browsing flags — never rendered content.
func (m Model) classifyHelpContext() string {
	if m.whereFocused() {
		return helpKindWhere
	}
	if m.Result != nil || m.resultHistoryMode {
		return helpKindResult
	}
	return helpKindBuilder
}

// openContextualHelp installs the single nonstacking help overlay over the
// exact current base context. The opener is captured as a complete immutable
// copy before any help state is set, so Esc can restore focus, cursor, search
// query, highlighted item, viewports, and selected history exactly without
// rebuilding anything from rendered text. It is a no-op while help is already
// open (help never stacks over itself or any other overlay) and while the
// terminal is suspended (too-small keys are consumed by the suspension gate).
func (m *Model) openContextualHelp() tea.Model {
	if m.helpOpen || m.suspended || m.Popup != nil || m.ValuePrompt != nil {
		return m
	}
	copied := *m
	m.helpKind = m.classifyHelpContext()
	m.helpOpener = &copied
	m.helpOpen = true
	return *m
}

// closeHelpRestore closes the help overlay and returns the exact opener copy
// captured at open time. Nothing is rebuilt from rendered state and the
// dismissal key is never applied beneath the closed overlay.
func (m *Model) closeHelpRestore() tea.Model {
	if m.helpOpener == nil {
		m.helpOpen = false
		m.helpKind = ""
		return m
	}
	restored := *m.helpOpener
	restored.helpOpen = false
	restored.helpKind = ""
	restored.helpOpener = nil
	return restored
}

// handleHelpKey dispatches one key while the contextual help overlay is open.
// Esc closes the overlay and restores the exact opener snapshot atomically;
// every other key — including a repeated `?`, printable text, navigation,
// history, save, and export keys — is consumed as an inert no-op so nothing
// leaks into the captured opener state below. q/Ctrl+C open the shared quit
// confirmation exactly like every other non-quit modal (Issue #27).
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.closeHelpRestore(), nil
	case "q", "ctrl+c":
		return m.openQuitConfirmation(), nil
	}
	// Every other key is a consumed no-op: help never mutates state and
	// repeated `?` never stacks a second overlay.
	return m, nil
}

// helpLines selects the exact rendered help lines for the typed opener
// context captured at open.
func (m Model) helpLines() []string {
	switch m.helpKind {
	case helpKindWhere:
		return whereContextHelpLines()
	case helpKindResult:
		return resultCountHelpLines()
	default:
		return builderHelpLines()
	}
}

// whereContextHelpLines are the required WHERE contextual help contents
// (Issue #54): typed NULL is TEXT, SQL-null intent routes to IS NULL /
// IS NOT NULL, and ordinary comparisons and LIKE do not match actual NULL
// values.
func whereContextHelpLines() []string {
	return []string{
		"WHERE help",
		"",
		"A typed token spelled NULL binds as literal TEXT, never as SQL NULL.",
		"To test SQL NULL directly, use the IS NULL or IS NOT NULL operator.",
		"Ordinary comparisons and LIKE do not match rows where the column",
		"actually holds NULL.",
		"'%' and '_' keep their SQLite wildcard meaning inside LIKE values.",
	}
}

// resultCountHelpLines are the required result-count help semantics (Issue
// #54): the count covers the complete executed SELECT including the user's
// Limit, is independent and unclamped, and may drift from the displayed page.
func resultCountHelpLines() []string {
	return []string{
		"Result count help",
		"",
		"The count covers the complete executed SELECT, including your Limit:",
		"it is not a table count and not a pre-Limit row count.",
		"It runs as an independent autocommit read, so it may drift from the",
		"rows currently shown or cached.",
		"The count never clamps fetched pages or the retained result cache.",
	}
}

// builderHelpLines are the general builder-context help contents: base keys
// only, with no database-starting suggestion beyond the established routes.
func builderHelpLines() []string {
	return []string{
		"Query builder help",
		"",
		"Tab / Shift+Tab     move between builder fields",
		"Enter               open the focused field's popup, or run a valid query",
		"Backspace/Delete    clear the focused field's committed value",
		"Ctrl+P / Ctrl+N     browse query history (idle only)",
		"Ctrl+E / Ctrl+Y     browse result history (idle only)",
		"Ctrl+S / Ctrl+X     save the last query / export the active result",
		"Ctrl+W              cancel an active cancellable request",
		"Esc                 clear a displayed error",
	}
}

// drawHelpOverlay composites the contextual help box over the composed shell
// inside the results region. Rendering is deterministic and never issues a
// request.
func (m Model) drawHelpOverlay(base string) string {
	lines := m.helpLines()
	longest := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > longest {
			longest = w
		}
	}
	box := valuePromptStyle.Width(longest + 2).Height(len(lines)).Render(strings.Join(lines, "\n"))
	return composeOverlay(base, box, 1, 1)
}
