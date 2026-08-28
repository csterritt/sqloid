// Main-schema catalog refresh lifecycle inside the UI, per Issue #13 and the
// Schema scope decisions in Notes/PRD-sqloid.md: every Table-popup open
// issues one fresh catalog request through the injected Connection seam
// before its refreshed result may replace what is presented; ordinary
// failures retain the exact prior catalog behind the persistent
// `Schema data is stale — retry or cancel` status plus the inline
// `could not refresh: <cause>` message, while accepting a candidate,
// advancing builder fields, and continuing toward execution stay blocked
// until retry succeeds, cancel closes the flow, or terminal health takes
// precedence. Database work never runs in Update or View: the refresher is
// invoked only inside returned tea.Cmd functions.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
)

// StaleSchemaStatus is the exact persistent status shown while schema data is
// stale, until retry succeeds, cancel closes the flow, or terminal health
// takes precedence.
const StaleSchemaStatus = "Schema data is stale — retry or cancel"

// staleCauseMessage renders the exact inline cause text for one failed
// refresh attempt: `could not refresh: <cause>`.
func staleCauseMessage(err error) string {
	return "could not refresh: " + err.Error()
}

// DeletedSessionEndedMessage is the exact terminal presentation when the
// startup database file no longer exists at the request boundary.
const DeletedSessionEndedMessage = "Database file no longer exists — session ended"

// ReplacedSessionEndedMessage is the exact terminal presentation when a
// different file took over the startup database path.
const ReplacedSessionEndedMessage = "Database file was replaced — session ended"

// TerminalState classifies the session-ending health overrides (Issue #13):
// each one replaces whatever workflow was active, including the stale-schema
// flow, and admits no further database work.
type TerminalState int

const (
	// TerminalNone means the session is live; only this zero value allows
	// continuation of database work.
	TerminalNone TerminalState = iota
	// TerminalDeleted renders DeletedSessionEndedMessage.
	TerminalDeleted
	// TerminalReplaced renders ReplacedSessionEndedMessage.
	TerminalReplaced
)

// CatalogRefresher performs one main-schema catalog refresh through the
// Connection boundary. Implementations run the boundary's own request rules
// (path-identity verification before work and error reclassification) and
// map typed connection.HealthError outcomes onto schema.RefreshDeleted /
// schema.RefreshReplaced attempts. Called only inside tea.Cmd functions.
type CatalogRefresher interface {
	RefreshCatalog() schema.Attempt
}

// RetrySchemaRefreshMsg requests one more catalog refresh for the active
// stale-schema flow; duplicates while an attempt is outstanding are ignored.
type RetrySchemaRefreshMsg struct{}

// CancelStaleRefreshMsg closes only the stale refresh flow and restores the
// captured opener plus pre-open state; it performs no continuation.
type CancelStaleRefreshMsg struct{}

// SchemaRefreshSettledMsg carries one settled refresh attempt back through
// Update together with the identity of the open that issued it, so late or
// superseded completions cannot mutate the model. Produced exclusively by
// commands created by the model itself.
type SchemaRefreshSettledMsg struct {
	Attempt uint64
	Result  schema.Attempt
}

// issueRefresh starts one identified refresh request against the wired seam.
// It advances the attempt identity whether or not a refresher is available so
// every open stands alone; a nil Refresher yields no command because there is
// no database work to do.
func (m *Model) issueRefresh() tea.Cmd {
	m.refreshAttempt++
	m.refreshPending = true
	refresher := m.Refresher
	if refresher == nil {
		m.refreshPending = false
		return nil
	}
	attempt := m.refreshAttempt
	return func() tea.Msg {
		return SchemaRefreshSettledMsg{Attempt: attempt, Result: refresher.RefreshCatalog()}
	}
}

// applyRefreshSettled transitions the model on one settled refresh result.
// Superseded attempts (an older identity than the latest issued) and invalid
// payloads are discarded wholesale. Successful installation swaps the whole
// catalog through QueryBuilder and clears both stale indicators atomically;
// ordinary failure keeps the prior catalog untouched and raises the
// indicators; the popup's offered candidates survive failure exactly as
// presented and adopt the refreshed eligible set on success, with search text
// preserved and highlight/viewport reset deterministically like any list
// replacement.
func (m *Model) applyRefreshSettled(msg SchemaRefreshSettledMsg) {
	m.refreshPending = false
	if msg.Attempt != m.refreshAttempt || msg.Attempt == 0 || !msg.Result.Valid() {
		return
	}
	if m.terminalState != TerminalNone {
		// Terminal health ended the session; no completion may revive work.
		return
	}
	switch msg.Result.Status {
	case schema.RefreshOK:
		m.catalog = msg.Result.Catalog
		m.applyBuilder(m.QB.RefreshSchema(msg.Result.Catalog))
		m.schemaStale = false
		m.staleCause = ""
		if m.Popup != nil {
			m.Popup.ReplaceCandidates(popupCandidates(m.QB.EligibleTables()))
			m.adjustScroll()
		}
	case schema.RefreshFailed:
		// Retain the exact prior catalog: no builder transition runs here.
		m.schemaStale = true
		m.staleCause = staleCauseMessage(msg.Result.Cause)
	case schema.RefreshDeleted, schema.RefreshReplaced:
		m.enterTerminal(statusToTerminal(msg.Result.Status))
	}
}

// statusToTerminal maps one settled deletion/replacement attempt onto the
// established terminal state kinds; ordinary statuses have no mapping and
// callers never pass them here.
func statusToTerminal(s schema.RefreshStatus) TerminalState {
	if s == schema.RefreshReplaced {
		return TerminalReplaced
	}
	return TerminalDeleted
}

// applyRetry issues one more identified catalog request through the same
// seam the original open used. It is refused while an attempt is outstanding,
// once terminal health ended the session, or without any refresher wired.
func (m *Model) applyRetry() tea.Cmd {
	if !m.schemaStale || m.refreshPending || m.terminalState != TerminalNone || m.Refresher == nil {
		return nil
	}
	return m.issueRefresh()
}

// applyCancel closes only the stale refresh flow: the popup disappears, the
// captured Table opener focus comes back, both indicators clear, and the
// attempt identity advances so any outstanding result can never mutate the
// restored pre-open state. No builder transition or execution runs.
func (m *Model) applyCancel() {
	if !m.schemaStale {
		return
	}
	opener := m.openerFocus
	m.closePopupRestore(opener)
	m.refreshAttempt++ // invalidate any still-outstanding attempt
	m.refreshPending = false
	m.schemaStale = false
	m.staleCause = ""
}

// enterTerminal transitions into one of the deletion/replacement terminal
// states, suppressing the stale workflow outright: controls and causes clear,
// the popup closes with its opener restored, and the attempt identity
// advances so pending or late completions are rejected on arrival.
func (m *Model) enterTerminal(state TerminalState) {
	opener := m.openerFocus
	if m.Popup != nil {
		m.closePopupRestore(opener)
	}
	m.refreshAttempt++
	m.refreshPending = false
	m.schemaStale = false
	m.staleCause = ""
	m.terminalState = state
}

// SchemaStale reports whether the exact stale-schema indicators are active.
func (m Model) SchemaStale() bool { return m.schemaStale }

// ContinuationBlocked reports whether downstream builder continuation —
// advancing to another field or executing — must stay gated. While the
// stale-schema flow is active, unchanged data must not become the basis of
// further work until retry succeeds, cancel closes the flow, or a terminal
// health classification ends the session.
func (m Model) ContinuationBlocked() bool {
	return m.schemaStale || m.terminalState != TerminalNone
}

// TerminalState reports the current deletion/replacement override; zero while
// the session remains live.
func (m Model) TerminalState() TerminalState { return m.terminalState }

// staleStatusLines returns the indicator lines for one surface, ordered
// status first then inline cause, exactly as rendered above either the
// popup's candidate rows (while open) or the results content (after close).
func staleStatusLines(stale bool, cause string) []string {
	if !stale {
		return nil
	}
	lines := []string{StaleSchemaStatus}
	if cause != "" {
		lines = append(lines, cause)
	}
	return lines
}
