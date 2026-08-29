// Query-history UI integration (Issue #35): Ctrl+P/N navigation over the
// Issue #20 query-history store, immutable copy-on-restore into the builder,
// execution-time mode exit, and defensive selected-ID eviction fallback.
// Browsing is strictly read-only: it never appends, never allocates a stable
// ID, and never reorders retained entries — the sole append path remains the
// Issue #20 ExecutionStartedMsg seam. Selection identity is the stable entry
// ID, never a slice index, so an externally evicted entry is detected by
// lookup and can never be rendered, restored, or executed through.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// HistoryEvictedNotice is the exact feedback shown when the selected stable
// ID was evicted from the query-history store.
const HistoryEvictedNotice = "Previously viewed query was evicted from history"

// restoreHistoryEntry deep-copies the stored state through RestoreBuilder and
// installs it as the current builder, recording the entry's stable ID. The
// stored entry itself is never touched, so later builder edits affect only
// current UI state. A false result means the stored state no longer resolves
// against the current catalog; the caller must not render through it.
func (m *Model) restoreHistoryEntry(e history.Entry) bool {
	next, ok := qb.RestoreBuilder(e.State, m.catalog)
	if !ok {
		return false
	}
	m.historyCursorID = e.ID
	m.applyBuilder(next)
	return true
}

// exitHistoryMode detaches the history cursor: history mode ends while the
// current — restored and possibly edited — builder state remains exactly as
// the user left it, ready to execute.
func (m *Model) exitHistoryMode() {
	m.historyMode = false
	m.historyCursorID = 0
	m.historyNotice = ""
}

// enterHistoryMode opens query-history browsing at the newest retained entry
// (both Ctrl+P and Ctrl+N enter here). An empty store or a nil store is a
// deterministic no-op; nothing appends.
func (m Model) enterHistoryMode() (tea.Model, tea.Cmd) {
	defer m.adjustScroll()
	if m.History == nil || m.History.Len() == 0 {
		return m, nil
	}
	e, ok := m.History.Newest()
	if !ok {
		return m, nil
	}
	m.historyMode = true
	m.historyNotice = ""
	if !m.restoreHistoryEntry(e) {
		m.historyMode = false
		m.historyCursorID = 0
	}
	return m, nil
}

// historyStep moves the cursor one entry older (newer == false, Ctrl+P) or
// newer (newer == true, Ctrl+N). Each step restores an immutable deep copy of
// the entry's complete builder state. Boundary presses are deterministic
// no-ops; Ctrl+N at the newest entry exits history mode back to the base
// builder view.
func (m Model) historyStep(newer bool) (tea.Model, tea.Cmd) {
	if m.History == nil {
		return m, nil
	}
	e, ok := m.History.NewerThan(m.historyCursorID)
	if !newer {
		e, ok = m.History.OlderThan(m.historyCursorID)
	}
	if !ok {
		if newer {
			// The newest boundary: leave history mode; the restored (and
			// possibly edited) builder state stays current.
			m.exitHistoryMode()
			m.adjustScroll()
		}
		return m, nil
	}
	m.historyNotice = ""
	if !m.restoreHistoryEntry(e) {
		// Defensive: a backing state that fails restoration is never rendered
		// through; the cursor detaches instead.
		m.exitHistoryMode()
	}
	m.adjustScroll()
	return m, nil
}

// validateHistorySelection resolves the selected stable ID against the
// retained entries after any history mutation, including externally driven
// appends. When the selection was evicted it moves to the new oldest
// retained entry (restoring a copy) with the exact eviction notice, or —
// when nothing is retained — exits history mode back to the base builder
// with the same notice. A missing backing entry is never rendered,
// restored, or executed through.
func (m *Model) validateHistorySelection() {
	if !m.historyMode || m.History == nil {
		return
	}
	if _, ok := m.History.Lookup(m.historyCursorID); ok {
		return
	}
	if m.History.Len() == 0 {
		m.exitHistoryMode()
		m.historyNotice = HistoryEvictedNotice
		return
	}
	e, ok := m.History.Oldest()
	if !ok {
		m.exitHistoryMode()
		return
	}
	if !m.restoreHistoryEntry(e) {
		m.exitHistoryMode()
		m.historyNotice = HistoryEvictedNotice
		return
	}
	m.historyNotice = HistoryEvictedNotice
}
