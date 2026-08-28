// Pre-execution schema-version validation inside the UI (Issue #21), per the
// Execution and Result Lifecycle and Schema scope decisions in
// Notes/PRD-sqloid.md. Enter on runnable authoritative builder data opens a
// distinct validation workflow under its own preparation identity and reads
// PRAGMA schema_version through the dedicated VersionReader seam before any
// execution command. An unchanged version reuses the exact cached catalog
// without issuing a catalog refresh; a changed version refreshes once through
// the established CatalogRefresher seam and repairs only builder state that
// transitively depends on an invalidated object, identifier, insertability
// fact, or rowid property, focusing the authoritative runnable report's first
// specific reason when repair leaves the data non-runnable. Ordinary refresh
// failures retain the stale cache behind the Issue #13 indicators with retry
// (fresh preparation identity) and cancel (exact restoration, no execution);
// deletion/replacement classifications take terminal precedence. Ctrl+W
// requests connection-scoped cancellation exactly once and renders exact
// `cancelling…` until settlement; any late success is discarded as cancelled.
// Validation appends neither query nor result history; only a settled
// successful validation returns the execution-start route, which Issue #22
// replaces with the real execution command. Database work runs only inside
// returned tea.Cmd functions; schema.Revalidate and QueryBuilder.Revalidate
// carry the typed logic, never error-string inference.

package ui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
)

// ValidationPreparingStatus renders while a validation request is in flight
// and no cancellation has been requested.
const ValidationPreparingStatus = "validating…"

// ValidationCancellingStatus is the exact status rendered from a Ctrl+W
// cancellation request until the in-flight validation settles.
const ValidationCancellingStatus = "cancelling…"

// VersionReader performs one PRAGMA schema_version read through the
// Connection boundary. Implementations run the boundary's own request rules
// (path-identity verification before work and error reclassification) and map
// health kinds onto typed schema.VersionAttempt outcomes. Called only inside
// tea.Cmd functions.
type VersionReader interface {
	ReadSchemaVersion() schema.VersionAttempt
}

// ValidationSettledMsg carries one settled schema-version revalidation back
// through Update together with the preparation identity that issued it, so
// late or superseded completions cannot mutate the model. Produced
// exclusively by commands the model created.
type ValidationSettledMsg struct {
	Preparation uint64
	Result      schema.Revalidation
}

// CancelValidationMsg is produced by the CancelCommand closure issued at
// validation open: the composition root's connection-scoped interrupt. The
// model handles it only to settle an already-cancelling workflow; it never
// dispatches work of its own.
type CancelValidationMsg struct{}

// beginValidation opens the pre-execution validation workflow and issues the
// schema-version request under a fresh preparation identity. It is refused
// once terminal health ended the session, while any validation is already
// open (a replacement may not start before the prior lease settles), or
// while a stale-refresh request is outstanding. On refusal the returned
// command is nil and nothing opens.
func (m *Model) beginValidation() tea.Cmd {
	if m.terminalState != TerminalNone || m.validating || m.refreshPending {
		return nil
	}
	m.validationAttempt++
	m.validating = true
	m.validationPending = true
	m.validationCancelling = false
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return CancelValidationMsg{} }
	return m.issueVersionRead()
}

// issueVersionRead starts one identified version read against the wired seam.
// A nil VersionReader yields no command because there is no database work to
// do; the pending flag then clears so the model cannot wedge. The cached
// catalog snapshot is captured at issue time and travels with the command, so
// the settled message is complete before it re-enters Update.
func (m *Model) issueVersionRead() tea.Cmd {
	reader := m.VersionReader
	prior := m.catalog
	attempt := m.validationAttempt
	if reader == nil {
		m.validationPending = false
		m.ActiveCancellable = false
		return nil
	}
	m.validationPending = true
	return func() tea.Msg {
		res := reader.ReadSchemaVersion()
		var r schema.Revalidation
		switch res.Status {
		case schema.RefreshOK:
			r = schema.Revalidate(prior, res.Version, func() schema.Attempt {
				if refresher, ok := refresherOf(m); ok {
					return refresher.RefreshCatalog()
				}
				return schema.NewFailure(errNoRefresherWired)
			})
		case schema.RefreshDeleted:
			r = schema.Revalidation{Status: schema.RevalidateDeleted}
		case schema.RefreshReplaced:
			r = schema.Revalidation{Status: schema.RevalidateReplaced}
		default:
			r = schema.Revalidation{Status: schema.RevalidateRefreshFailed, Cause: res.Cause}
		}
		return ValidationSettledMsg{Preparation: attempt, Result: r}
	}
}

// errNoRefresherWired surfaces inside an ordinary refresh failure when a
// changed version cannot be refreshed because no CatalogRefresher is wired.
var errNoRefresherWired = errors.New("no refresher wired")

// refresherOf recovers the wired CatalogRefresher for use inside a command.
func refresherOf(m *Model) (CatalogRefresher, bool) {
	if m.Refresher == nil {
		return nil, false
	}
	return m.Refresher, true
}

// applyValidationSettled transitions the model on one settled revalidation.
// Superseded preparations, invalid terminal-state arrivals, and completions
// after a cancellation request are discarded; a cancelling settlement closes
// the workflow with no execution and no builder mutation (cancellation wins
// over late success). Unchanged versions reuse the cache and return the
// execution-start route; refreshed versions repair the builder atomically and
// return the route only when the repaired data is still runnable.
func (m *Model) applyValidationSettled(msg ValidationSettledMsg) tea.Cmd {
	m.validationPending = false
	m.ActiveCancellable = false
	m.CancelCommand = nil
	if msg.Preparation != m.validationAttempt || msg.Preparation == 0 {
		return nil
	}
	if m.terminalState != TerminalNone {
		return nil
	}
	if m.validationCancelling {
		// Cancellation requested before settlement: the response — success
		// included — is classified as cancelled and discarded wholesale.
		m.endValidation()
		return nil
	}
	switch msg.Result.Status {
	case schema.RevalidateUnchanged:
		m.endValidation()
		return m.executionRoute()
	case schema.RevalidateRefreshed:
		m.endValidation()
		m.catalog = msg.Result.Catalog
		repaired, report := m.QB.Revalidate(msg.Result.Catalog)
		m.applyBuilder(repaired)
		if report.Report.Runnable {
			return m.executionRoute()
		}
		m.focusRunnableField(report.Report.Field)
		m.showRunnableReason(report.Report.Reason)
		return nil
	case schema.RevalidateRefreshFailed:
		// Retain the exact prior cache; raise the Issue #13 indicators.
		m.schemaStale = true
		m.staleCause = staleCauseMessage(msg.Result.Cause)
		return nil
	default:
		state := TerminalDeleted
		if msg.Result.Status == schema.RevalidateReplaced {
			state = TerminalReplaced
		}
		m.endValidation()
		m.enterTerminal(state)
		return nil
	}
}

// executionRoute returns the command Issue #22's execution workflow replaces.
// Successful validation alone reaches it: the command emits the
// execution-start lifecycle seam, which is the only path that appends query
// history.
func (m *Model) executionRoute() tea.Cmd {
	return func() tea.Msg { return ExecutionStartedMsg{} }
}

// endValidation closes the validation workflow without touching builder or
// stale state: pending, cancelling, and cancellability flags clear and the
// preparation identity stays for superseded-response guards.
func (m *Model) endValidation() {
	m.validating = false
	m.validationPending = false
	m.validationCancelling = false
	m.ActiveCancellable = false
	m.CancelCommand = nil
}

// retryValidation issues a fresh schema-version request under a new
// preparation identity after an ordinary refresh failure. Duplicate retries
// while a request is outstanding, once terminal health ended the session, or
// without a wired reader are refused.
func (m *Model) retryValidation() tea.Cmd {
	if !m.validating || !m.schemaStale || m.validationPending ||
		m.terminalState != TerminalNone || m.VersionReader == nil {
		return nil
	}
	m.schemaStale = false
	m.staleCause = ""
	// A retry is a fresh preparation: the identity advances so any response
	// from the superseded attempt is discarded on arrival.
	m.validationAttempt++
	// Re-arm cancellability: the retry is a new in-flight request owned by
	// the same validation workflow.
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { return CancelValidationMsg{} }
	return m.issueVersionRead()
}

// cancelValidation closes the validation flow after an ordinary failure:
// stale indicators clear, the exact pre-validation builder context stands,
// and the attempt identity advances so an outstanding response can never
// mutate the restored state. No execution runs.
func (m *Model) cancelValidation() {
	if !m.validating {
		return
	}
	m.validationAttempt++ // invalidate any still-outstanding attempt
	m.endValidation()
	m.schemaStale = false
	m.staleCause = ""
}

// requestValidationCancellation handles Ctrl+W during an in-flight
// validation: it marks the workflow cancelling (exact `cancelling…` until
// settlement, no replacement request) and returns the CancelCommand so the
// connection-scoped interrupt dispatches exactly once. Repeated presses and
// idle models dispatch nothing.
func (m *Model) requestValidationCancellation() tea.Cmd {
	if !m.validating || !m.validationPending || m.validationCancelling || m.CancelCommand == nil {
		return nil
	}
	m.validationCancelling = true
	cmd := m.CancelCommand
	return cmd
}

// validationStatusLines returns the presentation lines for the results
// region while validation owns it, cancelling status taking precedence over
// the preparing status; stale indicators render through the Issue #13 path.
func (m Model) validationStatusLines() []string {
	if !m.validating {
		return nil
	}
	if m.validationCancelling {
		return []string{ValidationCancellingStatus}
	}
	if m.schemaStale {
		return staleStatusLines(true, m.staleCause)
	}
	return []string{ValidationPreparingStatus}
}
