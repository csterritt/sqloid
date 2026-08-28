// Package ui implements the Sqloid Bubble Tea model: responsive shell layout,
// the query-builder field bar, the results region, focus scrolling, and the
// below-minimum terminal suspension contract from Issue #8.
//
// Database behavior stays behind the Connection composition seam; this package
// contains no database logic.
package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

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

	// QB is the authoritative query-builder state for command and table
	// selection. Its transitions own eligibility and downstream clearing;
	// Fields below re-render from it whenever a transition applies.
	QB qb.QueryBuilder

	// History owns the session-only query-history store (Issue #20). Nil
	// means no history is wired; execution-start appends then no-op. Append
	// happens only through the ExecutionStartedMsg seam — never during
	// runnable evaluation, validation, estimation, cancellation, or
	// confirmation dismissal.
	History *history.Store

	// Refresher performs one main-schema catalog refresh through the
	// Connection boundary per Table-popup open (Issue #13). It is invoked
	// only inside returned tea.Cmd functions; nil means no database work is
	// wired, so opens present the current catalog without issuing requests.
	Refresher      CatalogRefresher
	schemaStale    bool          // stale indicators active after an ordinary refresh failure
	staleCause     string        // exact inline cause text while schemaStale
	refreshAttempt uint64        // identity of the most recently issued refresh request
	refreshPending bool          // a refresh command is outstanding
	terminalState  TerminalState // TerminalNone while the session stays live

	suspended      bool   // true while the terminal is below minimum size
	suspendedModel *Model // exact copy retained across the undersized period

	// Popup is the currently open reusable popup list (Issue #12), or nil
	// when closed. While non-nil it consumes navigation, Enter, Esc, and —
	// for searchable variants — printable search input before any
	// base-context handling. openerFocus records the exact UI focus captured
	// at open so both accept and cancel paths restore that opener rather
	// than inferring a default; popupAccept commits an accepted candidate ID
	// to its owning feature after close. Installed only via installPopup.
	Popup       *Popup
	openerFocus int
	popupAccept func(*Model, string)

	// ValuePrompt is the currently open universal text entry (Issues #14 and
	// #17), or nil when closed. While non-nil it consumes every key before any
	// popup or base-context handling; Enter submits the verbatim buffer to the
	// owning feature's hook and Esc cancels its whole open draft.
	ValuePrompt *ValuePrompt
}

// New returns the initial model focused on the Command field with an idle,
// unselected query builder matching startup before any execution exists.
func New() Model {
	q := qb.NewQuery()
	return Model{
		Fields: builderFields(q),
		QB:     q,
		Focus:  0,
	}
}

// installPopup opens p over the current focus, capturing it as the exact
// opener to restore, and installs accept as the commit hook invoked with the
// accepted candidate ID after the popup closes.
func (m *Model) installPopup(p *Popup, accept func(mm *Model, id string)) {
	m.Popup = p
	m.popupAccept = accept
	m.openerFocus = m.Focus
}

// closePopupRestore removes the open popup and restores the given opener
// focus index exactly (clamped only to remain valid). Completed multi-
// selections stay available on the dismissed value until callers drop it.
func (m *Model) closePopupRestore(opener int) {
	if m.Popup != nil {
		m.Popup.Esc()
	}
	m.Popup = nil
	m.popupAccept = nil
	m.setFocus(opener)
}

// Init implements tea.Model. Terminal dimensions arrive via WindowSizeMsg.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. It handles window resizes, builder focus
// movement, and the gated input allowed while the terminal is undersized.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height), nil
	case SchemaRefreshedMsg:
		next := m.applySchemaRefresh(msg.Catalog)
		return next, nil
	case SchemaRefreshSettledMsg:
		m.applyRefreshSettled(msg)
		m.adjustScroll()
		return m, nil
	case RetrySchemaRefreshMsg:
		return m, m.applyRetry()
	case CancelStaleRefreshMsg:
		m.applyCancel()
		m.adjustScroll()
		return m, nil
	case StartTraceMsg:
		return m, func() tea.Msg { return handleStartTrace(msg) }
	case traceSettledMsg:
		return m.applyTraceResult(msg.result), nil
	case PreExecutionRequestedMsg:
		// Issue #20 lifecycle: runnable evaluation and the pre-execution seam
		// append nothing. Schema validation, destructive estimation, and
		// confirmation (Issue #22 onward) must settle before any history
		// entry exists.
		return m, nil
	case ExecutionStartedMsg:
		m.appendQueryHistoryAtExecutionStart()
		return m, nil
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
	if m.terminalState != TerminalNone {
		// A terminal health classification ended the session: no key may open
		// popups, move builder state, or start any further database work.
		return m, nil
	}
	if m.Popup != nil {
		return m.handlePopupKey(msg)
	}
	if m.ValuePrompt != nil {
		return m.handleValuePromptKey(msg)
	}
	if m.schemaStale {
		// Stale-schema flow owns the context: navigating to another builder
		// field would continue past unchanged data, so movement keys are
		// ignored until retry succeeds or cancel closes the flow.
		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			return m, nil
		}
	}
	switch msg.String() {
	case "backspace", "delete":
		if m.columnsFocused() {
			// Base Column(s) field owns removal (Issue #16): the immutable
			// remove-latest transition deletes exactly one committed entry per
			// press, and applyBuilder re-renders from the authoritative snapshot.
			m.applyBuilder(m.QB.RemoveLatestProjection())
			m.adjustScroll()
			return m, nil
		}
		if m.whereFocused() {
			// Base Where field owns whole-value clearing (Issue #19): one
			// immutable transition removes the entire submitted value while
			// preserving the selected column and operator.
			m.applyBuilder(m.QB.ClearWhereValue())
			refocusField(&m, whereFieldLabel)
			m.adjustScroll()
			return m, nil
		}
		if m.groupByFocused() {
			// Base Group By field owns removal (Issue #18): one accepted group
			// column per press, in reverse selection order.
			removeLatestGroup(&m)
			m.adjustScroll()
			return m, nil
		}
		if m.orderByFocused() {
			// Base Order By field owns whole-value clearing (Issue #18).
			clearOrderByField(&m)
			m.adjustScroll()
			return m, nil
		}
		if m.limitFocused() {
			// Base Limit field owns whole-value clearing (Issue #18).
			clearLimitField(&m)
			m.adjustScroll()
			return m, nil
		}
	case "enter":
		// Base-context Enter consults the authoritative runnable report after
		// every higher-precedence context has been handled above (Issue #19).
		cmd := m.handleBaseEnter()
		m.adjustScroll()
		return m, cmd
	case "tab":
		m.setFocus(m.Focus + 1)
	case "shift+tab":
		m.setFocus(m.Focus - 1)
	case "up":
		if m.orderByFocused() {
			// Focused base Order By field toggles ASC/DESC deterministically
			// when a selection is committed, without opening a popup or moving
			// popup selection; uncommitted Up falls through to navigation.
			if _, _, ok := m.QB.OrderBySelection(); ok {
				toggleOrderDirectionInBaseField(&m)
				m.adjustScroll()
				return m, nil
			}
		}
		m.setFocus(m.Focus - 1)
	case "down":
		if m.orderByFocused() {
			if _, _, ok := m.QB.OrderBySelection(); ok {
				toggleOrderDirectionInBaseField(&m)
				m.adjustScroll()
				return m, nil
			}
		}
		m.setFocus(m.Focus + 1)
	default:
		if handleCommandKey(&m, msg) {
			return m, nil
		}
		cmd := m.openPopupCmd(msg)
		if m.Popup != nil {
			m.adjustScroll()
		}
		return m, cmd
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

// ExecutionStartedMsg is the actual-execution-start lifecycle seam (Issue
// #20): emitted only when a real SELECT/INSERT begins after successful
// pre-execution validation, or when a confirmed UPDATE/DELETE begins its sole
// actual write. Issue #22 owns emitting it; until then the message exists so
// the append entry point, suppression policy, and timing rules are wired and
// testable without any database execution.

type ExecutionStartedMsg struct{}

// appendQueryHistoryAtExecutionStart appends the current normalized builder
// state to the query-history store through the single append entry point,
// which suppresses consecutive-identical states without allocating an ID.
// SELECT and INSERT append here; UPDATE and DELETE append only at their
// confirmation-driven write start, which no implemented flow can emit yet, so
// those commands never append through this seam. A nil store is an unchanged
// no-op. Failed execution outcomes arrive later and cannot undo the append.
func (m *Model) appendQueryHistoryAtExecutionStart() {
	if m.History == nil {
		return
	}
	switch m.QB.Command() {
	case qb.CommandSelect, qb.CommandInsert:
		m.History.AppendExecution(m.QB.HistoryState())
	default:
		// UPDATE/DELETE: confirmation begins the sole actual write (Issues
		// #37/#38); estimation and dismissal append nothing.
	}
}
