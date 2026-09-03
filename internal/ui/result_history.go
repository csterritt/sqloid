// Result-history UI browsing (Issue #36), per the Execution and Result
// Lifecycle, History Module Design, and Testing Decisions in
// Notes/PRD-sqloid.md. Browsing is strictly read-only: entering, stepping,
// resizing, and rendering resolve the selected snapshot by stable ID, project
// its immutable rows locally for the current terminal height, and issue zero
// database, page, or count requests — the only fresh-data path remains an
// actual rerun. Selection identity is the stable entry ID, never a slice
// index, so an externally evicted entry is detected by lookup and can never
// be rendered.

package ui

import (
	"errors"

	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// ResultEvictedNotice is the exact feedback shown when the selected stable ID
// was evicted from the result-history store (Issue #36).
const ResultEvictedNotice = "Previously viewed result was evicted from history"

// projectHistoryEntry is the pure local projection of one immutable history
// snapshot for the current terminal height's complete-row capacity. Tabular
// entries reslice their rows locally — the stored entry is never touched and
// the live result cache is never consulted — with the absolute offset taken
// from the entry's retained-range metadata. Non-tabular error and cancelled
// entries project to the ordinary error boundary carrying their exact reason
// with no rows.
func projectHistoryEntry(e history.ResultEntry, pageRows int) *ResultView {
	switch e.Kind {
	case history.KindTabular:
		rows := e.Rows
		if pageRows >= 0 && pageRows < len(rows) {
			rows = rows[:pageRows]
		}
		var offset int64
		if e.Metadata.HasRetainedRange {
			offset = int64(e.Metadata.RetainedStart) - 1
		}
		// Issue #75: restore the immutable snapshot's warning metadata onto
		// the projected view so the shared UTF and byte-cap warnings render
		// truthfully at every terminal page size. InvalidUTF is restored
		// onto the new result.Page (the grid's invalid-UTF truth source)
		// and TruncatedByByteCap onto ResultView.ByteTruncated (the
		// persistent byte-cap disclosure). Offset, rows, columns, and BLOB
		// copies are preserved; the stored entry is never touched.
		return &ResultView{
			Page: &result.Page{
				Columns:    e.Columns,
				Rows:       rows,
				InvalidUTF: e.Metadata.InvalidUTF,
			},
			Offset:        offset,
			ByteTruncated: e.Metadata.TruncatedByByteCap,
		}
	default:
		// KindError and KindCancelled display through the ordinary
		// result-error boundary exactly as recorded at finalization.
		return &ResultView{Err: errors.New(e.Reason)}
	}
}

// historyPageSize returns the current terminal height's complete data-row
// capacity for the results region.
func (m Model) historyPageSize() int {
	return CalculateLayout(m.Height, m.Fields).PageRows
}

// projectSelectedHistoryEntry reprojects the selected entry's snapshot for
// the current terminal height. The backing entry is looked up by stable ID
// first; a missing backing entry produces no projection at all so an evicted
// entry can never be rendered.
func (m *Model) projectSelectedHistoryEntry() {
	if m.ResultHistory == nil {
		m.resultHistoryView = nil
		return
	}
	e, ok := m.ResultHistory.Lookup(m.resultHistoryCursorID)
	if !ok {
		m.resultHistoryView = nil
		return
	}
	m.resultHistoryView = projectHistoryEntry(e, m.historyPageSize())
}

// exitResultHistoryMode detaches the result-history selection: the base
// builder/result context resumes with no historical rows and no stale
// selected rows; all retained entries stay in the store for later navigation.
func (m *Model) exitResultHistoryMode() {
	m.resultHistoryMode = false
	m.resultHistoryCursorID = 0
	m.resultHistoryView = nil
	m.resultHistoryNotice = ""
}

// enterResultHistoryMode opens result-history browsing at the newest retained
// entry (both Ctrl+E and Ctrl+Y enter here). It first exits any open history
// mode and finalizes the active SELECT exactly once (the Issue #34 seam), so
// an execution ends and appends its one snapshot before selection resolves.
// An empty or nil store is a deterministic no-op; nothing fetches.
func (m *Model) enterResultHistoryMode() {
	m.exitHistoryMode()
	m.exitResultHistoryMode()
	m.enterResultHistory() // Issue #34: finalize the active SELECT exactly once
	if m.ResultHistory == nil || m.ResultHistory.Len() == 0 {
		return
	}
	e, ok := m.ResultHistory.Newest()
	if !ok {
		return
	}
	m.resultHistoryMode = true
	m.resultHistoryCursorID = e.ID
	m.projectSelectedHistoryEntry()
}

// resultHistoryStep moves the selection one entry older (newer == false,
// Ctrl+E) or newer (newer == true, Ctrl+Y) through the stable-ID cursor
// primitives. Boundary presses are deterministic no-ops; Ctrl+Y at the newest
// entry exits result-history mode back to the base builder/result context.
func (m *Model) resultHistoryStep(newer bool) {
	if m.ResultHistory == nil {
		return
	}
	e, ok := m.ResultHistory.NewerThan(m.resultHistoryCursorID)
	if !newer {
		e, ok = m.ResultHistory.OlderThan(m.resultHistoryCursorID)
	}
	if !ok {
		if newer {
			m.exitResultHistoryMode()
		}
		return
	}
	m.resultHistoryCursorID = e.ID
	m.resultHistoryNotice = ""
	m.projectSelectedHistoryEntry()
}

// validateResultHistorySelection resolves the selected stable ID against the
// retained entries after every possible store mutation, including externally
// driven appends (Issue #36 Tasks 5–6). When the selection was evicted it
// moves to the new oldest retained entry — projecting only that entry's
// immutable rows at the current terminal height — with the exact eviction
// notice; when nothing is retained it clears result-history mode and returns
// to the base builder/result fallback. No evicted entry's rows, columns,
// metadata, or errors can ever be rendered, and no request is issued.
func (m *Model) validateResultHistorySelection() {
	if !m.resultHistoryMode || m.ResultHistory == nil {
		return
	}
	if _, ok := m.ResultHistory.Lookup(m.resultHistoryCursorID); ok {
		return
	}
	if m.ResultHistory.Len() == 0 {
		m.exitResultHistoryMode()
		m.resultHistoryNotice = ResultEvictedNotice
		return
	}
	e, ok := m.ResultHistory.Oldest()
	if !ok {
		m.exitResultHistoryMode()
		return
	}
	m.resultHistoryCursorID = e.ID
	m.projectSelectedHistoryEntry()
	m.resultHistoryNotice = ResultEvictedNotice
}
