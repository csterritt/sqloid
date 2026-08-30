// Package ui implements the Sqloid Bubble Tea model: responsive shell layout,
// the query-builder field bar, the results region, focus scrolling, and the
// below-minimum terminal suspension contract from Issue #8.
//
// Database behavior stays behind the Connection composition seam; this package
// contains no database logic.
package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
	"github.com/chris/sqloid/internal/schema"
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

	// Result holds the settled first-page state of the most recent SELECT
	// execution (Issue #22): a typed result.Page on success or an Err on
	// ordinary failure, replacing the idle results content once present. Raw
	// typed rows and warning metadata are preserved here; rendering happens
	// only in View through the internal/result seam. nil means nothing has
	// executed since startup.
	Result *ResultView

	// firstColumn is the first-visible output-column index of the result
	// grid (Issue #29) — the grid's only horizontal position. It is moved
	// one whole column per accepted horizontal key press, recomputed into
	// visible widths at every render pass, and clamped to the current
	// output columns on resize. No intra-cell offset exists.
	firstColumn int

	// Select performs one cancellable first-page SELECT execution through the
	// Connection boundary for the given safely rendered SQL and ordered bound
	// parameters. It runs only inside tea.Cmd functions — never in Update or
	// View — and maps health classifications onto the returned FirstPageResult.
	// nil means no execution is wired: the execution-start message still
	// appends query history, but no database work is issued.
	Select SelectExecutor

	// Count performs one cancellable complete-SELECT count execution through
	// the Connection boundary (Issue #24) for the safely rendered count SQL
	// and ordered parameters, concurrently with the first page on its own
	// dedicated lease. It runs only inside tea.Cmd functions and maps health
	// classifications onto the returned CountResult. nil leaves the count
	// state unset (page-only fixtures).
	Count CountExecutor

	// Page performs one cancellable paged-page SELECT execution through the
	// Connection boundary (Issue #25) for the safely rendered page SQL and
	// ordered parameters, with the exact LIMIT/OFFSET range built by
	// QueryBuilder's page API. It runs only inside tea.Cmd functions. nil
	// leaves the results display page-only: page keys are consumed without
	// issuing requests.
	Page PageExecutor

	// Serialized vertical-paging state (Issue #25): at most one page request
	// is pending at any time, tracked independently from the Issue #24 count
	// request. pageOffset is the absolute logical offset of the displayed
	// page's first row; pageRequested/pageRequestedSize record the exact
	// range of the in-flight request for boundary arithmetic at settlement;
	// pageExhausted marks the known high boundary once a page returned fewer
	// rows than requested. Only the paging seam in this package mutates them.
	pageOffset            int64
	pageRequested         int64
	pageRequestedSize     int64
	pagePending           bool
	pageRequestID         uint64
	pageExhausted         bool
	pageRequestExecution  uint64 // execution ID captured at page dispatch (Issue #26)
	pageRequestGeneration uint64 // viewport generation captured at page dispatch (Issue #26)

	// viewportCache is the active SELECT's authoritative contiguous dual-cap
	// result cache (Issues #30/#31): every accepted page response merges here
	// by absolute logical position before it becomes display state. Issue #32
	// reads its retained-range and endpoint metadata for resize recovery.
	viewportCache *resultcache.Cache

	// Resize-recovery fetch deferral (Issue #32): when a resize needs a fetch
	// while an old-size page request is still pending, the old request is
	// cancelled/invalidated and exactly one replacement request for the
	// containing page of the required row is deferred here until that old
	// work truly settles. Repeated resizes overwrite the row/size to the
	// latest decision; any other resize decision clears the deferral.
	resizeFetchPending bool
	resizeFetchRow     int64
	resizeFetchSize    int64

	// Generic in-flight gate state (Issue #27): firstPagePending records
	// ownership of the first-page request of the current SELECT execution,
	// selectCancelling is set from the Ctrl+W request until every owned read
	// request settles, and inFlightNotice holds the exact blocked-action
	// feedback rendered by View. quitConfirm/quitSuspended implement the
	// shared quit confirmation, which suspends the exact current context and
	// restores it on cancel. The gate reads these flags only — never rendered
	// phase-label strings.
	firstPagePending bool
	countPendingFlag bool // generic-gate claim on the independent count request
	selectCancelling bool

	// Scoped Ctrl+W cancellation handles (Issue #28): one derived-context
	// cancel function per in-flight SELECT request — the first page, the
	// independent count, and the one later page. Each handle is installed
	// when its command dispatches and cleared exactly at that request's
	// settlement, so the cancellation message requests an independent,
	// idempotent connection-scoped interrupt for each currently active
	// request and never touches settled or unrelated work.
	firstPageCancel   context.CancelFunc
	countCancel       context.CancelFunc
	pageRequestCancel context.CancelFunc
	inFlightNotice    string
	quitConfirm       bool
	quitSuspended     *Model

	// selectTracker guards the current SELECT execution's two concurrent
	// completions (Issue #24): a page or count completion mutates state only
	// when both its execution ID and role-specific request ID match, and each
	// role is consumed at most once.
	selectTracker result.SelectTracker

	// viewportGen is the current viewport generation (Issue #26): page
	// requests capture it at dispatch and their responses mutate state only
	// while it is still current. Resize, SELECT deactivation/finalization,
	// and each new execution advance it. Count responses track only their
	// Issue #24 identity; the generation is page-request state.
	viewportGen uint64

	// countState is the independent result-count presentation state (Issue
	// #24): pending, successful (with the executed builder's Limit metadata
	// for the exact wording), or unavailable. Rendering reads it explicitly
	// and never infers it from row length.
	countState result.CountState

	// QB is the authoritative query-builder state for command and table
	// selection. Its transitions own eligibility and downstream clearing;
	// Fields below re-render from it whenever a transition applies.
	QB qb.QueryBuilder

	// catalog is the retained cached schema catalog (Issue #9/#21 cache):
	// the exact pointer reused on unchanged pre-execution validation. Kept
	// current on every successful catalog install.
	catalog *schema.Catalog

	// Pre-execution validation workflow state (Issue #21): validating is
	// true while the distinct workflow is open; validationPending while a
	// schema-version request is outstanding; validationCancelling from the
	// Ctrl+W request until settlement; validationAttempt is the monotonic
	// preparation/request identity guarding late or superseded responses.
	VersionReader        VersionReader
	validating           bool
	validationPending    bool
	validationCancelling bool
	validationAttempt    uint64

	// Destructive preparation modal (Issue #40): Estimator performs the
	// independent matching-target estimate; prepOpen/prepPending/prepCancelling
	// carry the modal's phase; prepAttempt is the monotonic preparation/request
	// identity guarding late or superseded responses; prepOperation/prepTable/
	// prepSQL/prepNoWhere retain the continuously visible modal content;
	// prepEstimate/prepErr retain the settled estimate outcome for the later
	// confirmation seam. Opening, pending, success, failure, cancellation, and
	// dismissal append neither query nor result history and never start the
	// write.
	Estimator      EstimateExecutor
	prepOpen       bool
	prepOperation  string
	prepTable      string
	prepSQL        string
	prepNoWhere    bool
	prepPending    bool
	prepCancelling bool
	prepAttempt    uint64
	prepCancel     context.CancelFunc
	prepEstimate   int64
	prepErr        string

	// Deliberate confirmation state (Issue #41): writeAttempt is the
	// monotonic actual-write execution identity, allocated only at the
	// exactly-once confirmation transition and always distinct from the
	// preparation identity; confirmedExecution is the execution identity of
	// the most recently delivered WriteConfirmedMsg, so duplicate or stale
	// deliveries stay inert.
	writeAttempt       uint64
	confirmedExecution uint64

	// Actual transactional write lifecycle (Issue #42): Write performs the
	// sole phased write through the Connection boundary inside a tea.Cmd;
	// writeExecution/writeOperation/writeSQL retain the execution identity,
	// operation, and executed standalone SQL from the execution-start boundary
	// through finalization; writePending/writeCancelling/writePhase carry the
	// visible lifecycle; writeFinalized is the exactly-once finalization flag;
	// writePhases relays typed connection phases through Update and
	// writeCancel is the scoped cancellation handle retired at the commit
	// boundary. Duplicate, late, and stale phase or outcome messages are
	// inert; post-boundary interaction rendering stays with Issue #43.
	Write           WriteExecutor
	writeExecution  uint64
	writeOperation  string
	writeSQL        string
	writePending    bool
	writeCancelling bool
	writePhase      connection.WritePhase
	writeFinalized  bool
	writePhases     chan connection.WritePhaseMsg
	writeCancel     context.CancelFunc

	// writeNoncancellable is the typed commit-boundary state (Issue #43):
	// set exactly when the write's rollback-cleanup or committing phase has
	// begun, it permanently closes the Ctrl+W cancellation route for this
	// write. Ctrl+W routes by this state, never by phase-label text; phase
	// regressions and stale identities can never clear it backward.
	writeNoncancellable bool

	// quitWaitWrite is the accepted-quit wait state (Issue #43): true while
	// an accepted quit is waiting for the pending write to settle through
	// rollback resolution or commit. Exit is emitted only when settlement
	// finalizes; duplicate acceptances are idempotent no-ops.
	quitWaitWrite bool

	// History owns the session-only query-history store (Issue #20). Nil
	// means no history is wired; execution-start appends then no-op. Append
	// happens only through the ExecutionStartedMsg seam — never during
	// runnable evaluation, validation, estimation, cancellation, or
	// confirmation dismissal.
	History *history.Store

	// ResultHistory owns the session-only result-history store of finalized
	// SELECT snapshots (Issue #34). Non-nil even before any execution so
	// finalization has an append target; only the single idempotent
	// finalizeActiveSelect seam appends to it, exactly once per execution ID.
	ResultHistory *history.ResultStore

	// Query-history browsing state (Issue #35): historyMode is true while a
	// history entry is selected for viewing; historyCursorID is that entry's
	// stable ID (never a slice index), and historyNotice holds the exact
	// eviction feedback when the selected entry was evicted externally.
	// Browsing is read-only: nothing here appends or allocates IDs.
	historyMode     bool
	historyCursorID history.EntryID
	historyNotice   string

	// Result-history browsing state (Issue #36): resultHistoryMode is true
	// while a finalized result snapshot is selected for viewing;
	// resultHistoryCursorID is its stable ID (never a slice index),
	// resultHistoryView the pure local projection of that entry at the
	// current terminal height, and resultHistoryNotice the exact eviction
	// feedback when the selected entry was evicted externally. Browsing is
	// read-only: it never appends, never consults the live result cache, and
	// issues zero database requests.
	resultHistoryMode     bool
	resultHistoryCursorID history.EntryID
	resultHistoryView     *ResultView
	resultHistoryNotice   string

	// Active-SELECT lifetime state (Issue #34), kept distinct from request
	// identity: selectActive reports the active lifetime (not any request's
	// flight), activeExecID the owning execution ID, and finalizedExecID the
	// most recently finalized execution. pendingCancelReason/pendingFailure
	// carry a recorded ending cancellation or failure until finalization
	// consumes them into the snapshot's terminal outcome.
	selectActive        bool
	activeExecID        uint64
	finalizedExecID     uint64
	pendingCancelReason string
	pendingFailure      *selectFailure

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
	setCursor   int

	// insertCursor is the INSERT per-column prompt cursor (Issue #39): the
	// index into QB.InsertColumns() of the column whose shared choice popup
	// or value entry is currently open, or the revision target for clearing.
	insertCursor int
}

// New returns the initial model focused on the Command field with an idle,
// unselected query builder matching startup before any execution exists.
func New() Model {
	q := qb.NewQuery()
	return Model{
		Fields:        builderFields(q),
		QB:            q,
		Focus:         0,
		ResultHistory: history.NewResultStore(),
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
	// Issue #36: defensively resolve the selected stable ID after every
	// possible store mutation — including externally driven appends — so an
	// evicted entry is never rendered, restored, or executed through.
	if m.historyMode {
		m.validateHistorySelection()
	}
	if m.resultHistoryMode {
		m.validateResultHistorySelection()
	}
	switch msg := msg.(type) {
	case connection.WritePhaseMsg:
		cmd := m.applyWritePhase(msg)
		m.adjustScroll()
		return m, cmd
	case WriteSettledMsg:
		// Issue #42: a stale, duplicate, or late settlement message is an
		// inert no-op; Issue #43: only a settlement that truly finalized the
		// current write — proving the transaction and driver work ended — may
		// complete a waiting accepted quit.
		wasFinalized := m.writeFinalized
		m.applyWriteSettled(msg)
		m.adjustScroll()
		if !wasFinalized && m.quitWaitWrite && !m.writePending {
			// The accepted quit waited for this settlement; the write's work
			// has fully ended and its one result entry is final, so exit is
			// emitted exactly once.
			m.quitWaitWrite = false
			return m, tea.Quit
		}
		return m, nil
	case WriteCancelRequestedMsg:
		// The scoped cancellation closure dispatched the interrupt request
		// before this delivery; visible cancelling state holds until the
		// write settles through rollback cleanup or commit. Only a still-
		// pending cancellable write consumes the message; a late delivery is
		// an inert no-op.
		if m.writePending && m.ActiveCancellable {
			m.writeCancelling = true
		}
		return m, nil
	case tea.WindowSizeMsg:
		next := m.resize(msg.Width, msg.Height)
		// Issue #32: a visible resize (including suspension restoration) also
		// runs the vertical viewport recovery against the fresh page size.
		return next, next.applyResizeRecovery()
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
	case ValidationSettledMsg:
		cmd := m.applyValidationSettled(msg)
		m.adjustScroll()
		return m, cmd
	case PreExecutionRequestedMsg:
		// Issue #21 lifecycle: runnable Enter opens the distinct pre-execution
		// validation workflow and issues the schema-version request. Opening,
		// pending, failed, cancelled, and dismissed validation appends nothing;
		// the execution-start route is returned only by settled success.
		return m, m.beginValidation()
	case CancelValidationMsg:
		// Produced by the cancellation closure dispatched at Ctrl+W; the model
		// already entered its cancelling state before dispatching, so this
		// settles nothing on its own (the workflow closes at true settlement).
		return m, nil
	case EstimateSettledMsg:
		// Issue #40: one settled matching-target estimate, guarded by its
		// preparation identity so stale responses never mutate the modal.
		cmd := m.applyEstimateSettled(msg)
		m.adjustScroll()
		return m, cmd
	case WriteConfirmedMsg:
		// Issue #42: one delivered deliberate confirmation begins the sole
		// actual write — exiting either history first, appending the complete
		// query state at execution start, and dispatching the transactional
		// write of the retained rendered statement. Duplicate or stale
		// deliveries stay inert no-ops that start nothing and append nothing.
		var cmd tea.Cmd
		if m.applyWriteConfirmed(msg) {
			cmd = m.beginConfirmedWrite(msg)
		}
		m.adjustScroll()
		return m, cmd
	case CancelEstimateMsg:
		// Same settling handoff as validation cancellation: the estimate
		// settles through its own response; the modal dismisses there.
		return m, nil
	case ExecutionStartedMsg:
		// Issue #35: an actual execution exits history mode first, keeping the
		// current restored-and-possibly-edited builder state as the execution
		// input, then runs the unchanged Issue #20 append seam. Issue #36:
		// result-history selection and stale displayed rows clear before the
		// execution and its Issue #34 finalization proceed. Issue #42: a
		// runnable INSERT's dispatch is its sole actual transactional write;
		// SELECT continues to its concurrent page/count lifecycle.
		m.exitHistoryMode()
		m.exitResultHistoryMode()
		m.appendQueryHistoryAtExecutionStart()
		if m.QB.Command() == qb.CommandInsert {
			return m, m.startWrite(result.NextWriteExecutionID(), "INSERT", m.QB.InsertSQL(), m.QB.InsertParams())
		}
		return m, m.startSelectPage()
	case SelectSettledMsg:
		// Issues #24 and #26: a page completion mutates active state only when
		// its SELECT execution ID and first-page request ID match the current
		// identities, the role is unconsumed, and the viewport generation is
		// still current; stale, duplicated, wrong-role, superseded-generation,
		// and cancellation-classified responses are discarded untouched.
		req := result.SelectRequest{ExecutionID: msg.ExecutionID, Role: result.RoleFirstPage, RequestID: msg.RequestID}
		if m.selectTracker.Accept(req) {
			// Issue #27: the first-page ownership slot is released exactly when
			// the tracker consumes the role, so the generic gate settles with
			// the request whatever the outcome classification is.
			m.firstPagePending = false
			m.firstPageCancel = nil // Issue #28: the handle retires with the request
			m.clearSelectCancellingIfSettled()
			m.inFlightNotice = ""
			if msg.Generation == m.viewportGen {
				if msg.Result.Cancelled {
					// Issue #34: a first-page settlement classified cancelled
					// ended the execution before any row was retained — the
					// cancellation finalizer runs once, after settlement.
					m.noteSelectCancelled()
					m.finalizeActiveSelect()
					return m, nil
				}
				if msg.Result.Err != nil {
					// Issue #34: an ordinary first-page failure before rows ends
					// the SELECT and finalizes it with one error entry; the
					// ordinary result-error boundary still renders the cause.
					m.noteSelectFailed(msg.Result.Err.Error())
					m = m.applySelectSettled(msg.Result)
					m.finalizeActiveSelect()
					return m, nil
				}
				return m.applySelectSettled(msg.Result), nil
			}
		}
		return m, nil
	case CountSettledMsg:
		// Issue #24: the count role has its own request ID and the same
		// two-level guard; it settles into its own presentation state without
		// ever converting into a page failure. Issue #26: cancellation-
		// classified counts are equally inert — they are neither a total nor
		// the exact failure wording.
		req := result.SelectRequest{ExecutionID: msg.ExecutionID, Role: result.RoleCount, RequestID: msg.RequestID}
		if m.selectTracker.Accept(req) {
			// Issue #27: count settlement is request-ownership settlement for
			// the generic gate regardless of the outcome classification.
			m.countPendingFlag = false
			m.countCancel = nil // Issue #28: the handle retires with the request
			m.clearSelectCancellingIfSettled()
			m.inFlightNotice = ""
			if !msg.Result.Cancelled {
				return m.applyCountSettled(msg.Result), nil
			}
		}
		return m, nil
	case PageSettledMsg:
		// Issues #25 and #26: a page completion mutates state only under the
		// full identity rule in applyPageSettled; the count's independent
		// settlement above is unaffected either way. Issue #27: the pending
		// slot's release inside applyPageSettled also settles the generic
		// gate's cancelling feedback once nothing remains pending.
		current := msg.ExecutionID == m.selectTracker.ExecutionID() && m.pagePending && m.pageRequestID == msg.RequestID
		next := m.applyPageSettled(msg)
		if current {
			switch {
			case msg.Result.Err != nil:
				// Issue #34: a later-page ordinary failure after retained rows is
				// recorded as the execution's recorded ending; the SELECT stays
				// active across remaining events, and finalization classifies the
				// snapshot as failed while preserving the captured rows.
				next.noteSelectFailed(msg.Result.Err.Error())
			case msg.Result.Cancelled:
				// Issue #34: an interrupted later page after rows records the
				// ending; a later healthy page settlement clears it, and a
				// finalizer meanwhile types the snapshot cancelled-after-rows.
				next.noteSelectCancelled()
			default:
				// A healthy page means the execution continued past any recorded
				// ending: the terminal outcome returns to undecided.
				next.clearPendingEnding()
			}
		}
		next.clearSelectCancellingIfSettled()
		next.inFlightNotice = ""
		// Issue #32: once the cancelled/invalidated old-size page work has
		// truly settled, exactly one replacement request dispatches for the
		// page containing the required first row at the latest exact size.
		if next.resizeFetchPending && !next.pagePending {
			row, size := next.resizeFetchRow, next.resizeFetchSize
			next.resizeFetchPending = false
			return next, next.requestRecoveryPage(row, size)
		}
		return next, nil
	case SelectCancelRequestedMsg:
		// Issue #28: the Ctrl+W closure's message requests one scoped,
		// idempotent cancellation per currently active page/count request.
		// Each handle cancels only its own request's context — the in-flight
		// work settles through the Connection boundary, whose cancellation-
		// wins classification keeps the late result inert — and the model
		// holds `cancelling…` until every targeted request settles.
		for _, cancel := range []context.CancelFunc{m.firstPageCancel, m.countCancel, m.pageRequestCancel} {
			if cancel != nil {
				cancel()
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		// Issue #29: the terminal driver reports the raw xterm shift+Page
		// Down/Up CSI sequences as unknown messages rather than KeyMsgs; the
		// bridge routes them onto the same one-column bindings. Every other
		// message is inert here, as before.
		if dir, ok := shiftPageDirection(msg); ok {
			m.handleHorizontalKey(dir)
			return m, nil
		}
	}
	return m, nil
}

// clearSelectCancellingIfSettled drops the SELECT cancelling feedback once
// every owned read request has settled; while any request remains in flight
// the `cancelling…` handoff stays visible exactly until settlement. Issue
// #34: when the user requested cancellation through the generic Ctrl+W seam
// and every owned request has then settled, the cancellation ended the
// SELECT — the execution finalizes once (its snapshot carrying the recorded
// terminal outcome) instead of merely closing the cancellation seam.
func (m *Model) clearSelectCancellingIfSettled() {
	if !m.selectRequestPending() {
		m.selectCancelling = false
		// Issue #28: with every owned read request settled, the model owns no
		// cancellable work — the generic cancellation seam closes so later
		// Enter presses reach the runnable route again.
		m.ActiveCancellable = false
		m.CancelCommand = nil
	}
}

// bumpViewportGeneration advances the viewport generation so every page
// response dispatched under an older generation becomes inert (Issue #26).
func (m *Model) bumpViewportGeneration() { m.viewportGen++ }

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
			// Issue #26: becoming visible again is a resize — the viewport
			// generation advances so page responses from the hidden period
			// become inert.
			restored.bumpViewportGeneration()
			restored.clampScroll()
			// Issue #29: becoming visible again is a resize — the first-visible
			// output-column index is preserved when valid and clamped otherwise.
			restored.clampFirstColumnModel()
			return restored
		}
		m.suspended = false
		m.suspendedModel = nil
	}
	// Issue #26: resizing the visible shell advances the viewport generation,
	// so every page response dispatched before the resize — first or later
	// page — becomes inert regardless of its other identities. (Entering
	// suspension leaves hidden state exactly frozen; the restore above is the
	// generation-advancing resize.)
	m.bumpViewportGeneration()
	m.clampScroll()
	// Issue #29: resize preserves the first-visible output-column index when
	// valid and clamps it to the nearest valid boundary otherwise.
	m.clampFirstColumnModel()
	// Issue #36: while browsing result history, resize reslices the selected
	// immutable snapshot locally for the new complete-row capacity; no
	// snapshot is rewritten and no request is issued.
	if m.resultHistoryMode {
		m.projectSelectedHistoryEntry()
	}
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
	if m.quitConfirm {
		// Issue #27: the shared quit confirmation sits above every other
		// context and consumes all keys with no leakage until resolved.
		return m.handleQuitConfirmKey(msg)
	}
	if m.prepOpen {
		// Issue #40: the destructive preparation modal consumes keys above
		// every other context until dismissed; Enter/y stay disabled.
		return m.handlePreparationKey(msg)
	}
	if m.Popup != nil {
		return m.handlePopupKey(msg)
	}
	if m.ValuePrompt != nil {
		return m.handleValuePromptKey(msg)
	}
	// Issue #35: query-history mode owns Ctrl+P/N and Esc while browsing;
	// every other key falls through so the restored builder stays editable.
	if m.historyMode {
		switch msg.String() {
		case "ctrl+p":
			return m.historyStep(false)
		case "ctrl+n":
			return m.historyStep(true)
		case "esc":
			m.exitHistoryMode()
			m.adjustScroll()
			return m, nil
		}
	}
	// Issue #36: result-history mode owns Ctrl+E/Y and Esc while browsing a
	// finalized snapshot; every other key falls through. Nothing here can
	// issue a request — the only fresh-data path remains an actual rerun.
	if m.resultHistoryMode {
		switch msg.String() {
		case "ctrl+e":
			m.resultHistoryStep(false)
			return m, nil
		case "ctrl+y":
			m.resultHistoryStep(true)
			return m, nil
		case "esc":
			m.exitResultHistoryMode()
			m.adjustScroll()
			return m, nil
		}
	}
	// Issue #27: the generic request-in-flight gate sits between focused
	// input/overlays and base-context handling. Permitted local interaction
	// (horizontal one-column movement, serialized page keys, field navigation)
	// falls through to base handling; the gate itself derives pending state
	// from request ownership, not rendered labels.
	if m.selectRequestPending() {
		if next, cmd, handled := m.handleInFlightGate(msg); handled {
			return next, cmd
		}
	} else {
		// Issue #27: with no request in flight the base context owns every
		// key, so any gate feedback from a settled phase clears immediately.
		m.inFlightNotice = ""
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
	case "q", "ctrl+c":
		// Idle base context (Issue #27): both keys open the shared quit
		// confirmation, suspending the exact current context.
		return m.openQuitConfirmation(), nil
	case "pgup", "pgdown":
		// Issue #25: serialized adjacent-page navigation. While a page is
		// pending this consumes repeated and opposite keys without stacking
		// commands; at the known boundaries and beyond the user's Limit it
		// consumes the key without issuing anything.
		cmd := m.handlePageKey(msg.String() == "pgup")
		return m, cmd
	case ".":
		// Issue #29: portable Shift+Page Down alternate — one whole column
		// per press, purely local, boundary presses consumed as no-ops.
		m.handleHorizontalKey(1)
		return m, nil
	case ",":
		// Issue #29: portable Shift+Page Up alternate.
		m.handleHorizontalKey(-1)
		return m, nil
	case "ctrl+w":
		if m.prepOpen {
			// Issue #40: scoped estimate cancellation, exact `cancelling…`
			// until settlement; settlement then dismisses preparation.
			return m, m.requestEstimateCancellation()
		}
		if m.validating {
			// Issue #21: connection-scoped cancellation requested exactly once
			// per request; exact `cancelling…` renders until settlement.
			return m, m.requestValidationCancellation()
		}
		if m.writePending {
			// Issue #43: Ctrl+W routes by the typed commit-boundary state. In
			// the noncancellable rollback-cleanup/committing phases it is
			// ignored with the exact boundary feedback, and the work is never
			// mutated; in the cancellable phases the cancellation request is
			// deduplicated to exactly one per write.
			if m.writeNoncancellable {
				m.inFlightNotice = CommitBoundaryFeedback
				return m, nil
			}
			if !m.writeCancelling && m.CancelCommand != nil {
				m.writeCancelling = true
				return m, m.CancelCommand
			}
			return m, nil
		}
		if m.ActiveCancellable && m.CancelCommand != nil {
			return m, m.CancelCommand
		}
	case "ctrl+p", "ctrl+n":
		// Issue #35: entering query-history browsing from the base context
		// selects the newest retained entry; with no retained entries the key
		// is a no-op. While a request is in flight the gate above blocks this
		// first, as before.
		return m.enterHistoryMode()
	case "ctrl+e", "ctrl+y":
		// Issue #36: entering result-history browsing from the base context
		// finalizes the active SELECT once (the Issue #34 seam) and selects
		// the newest finalized snapshot; with no retained entries it is a
		// no-op. While a request is in flight the gate above blocks first.
		m.enterResultHistoryMode()
		m.adjustScroll()
		return m, nil
	case "esc":
		if m.validating && m.schemaStale && !m.validationCancelling {
			// Cancel closes the stale validation flow with the exact
			// pre-validation builder context and no execution.
			m.cancelValidation()
			m.adjustScroll()
			return m, nil
		}
		// Issue #36: Esc dismisses the displayed ordinary query error to the
		// base builder/result context without deleting any retained history.
		if m.Result != nil && m.Result.Err != nil {
			m.Result = nil
		}
		m.adjustScroll()
		return m, nil
	case "backspace", "delete":
		if m.setFocused() {
			m.clearCurrentSetValue()
			m.adjustScroll()
			return m, nil
		}
		if m.insertFocused() {
			// Base Insert field owns whole-value clearing (Issue #39): one
			// immutable transition removes the entire submitted value while
			// preserving the Value choice and column identity.
			m.clearCurrentInsertValue()
			m.adjustScroll()
			return m, nil
		}
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
		if m.validating && m.schemaStale && !m.validationCancelling {
			// Retry inside stale validation issues a fresh version request
			// under a new preparation identity; duplicates are refused.
			cmd := m.retryValidation()
			m.adjustScroll()
			return m, cmd
		}
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
		if m.moveSetCursor(-1) {
			m.adjustScroll()
			return m, nil
		}
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
		if m.moveSetCursor(1) {
			m.adjustScroll()
			return m, nil
		}
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

// handleWriteMsg routes the typed Issue #42 write lifecycle messages. Phase
// messages update retained state and keep the relay alive; a settled message
// finalizes exactly one result entry; a cancellation-requested message marks
// visible cancelling state until rollback cleanup or commit resolves it.
func (m Model) handleWriteMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case connection.WritePhaseMsg:
		if msg.Phase == connection.WritePhaseBeginning || msg.Phase == connection.WritePhaseExecuting {
			m.writePhase = msg.Phase
		}
		return m, m.applyWritePhase(msg), true
	case WriteSettledMsg:
		m.applyWriteSettled(msg)
		return m, nil, true
	case WriteCancelRequestedMsg:
		if m.writePending && m.ActiveCancellable {
			m.writeCancelling = true
		}
		return m, nil, true
	}
	return m, nil, false
}
