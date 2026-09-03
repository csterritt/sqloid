// Ctrl+X result-export targeting, immutable capture, and warning flow
// (Issue #49), per the Result export scope, Export warnings, Cache and
// snapshot invariant, and Global Key Precedence decisions in
// Notes/PRD-sqloid.md. Targeting resolves entirely from immutable in-memory
// state — the idle active tabular result, the selected retained result-history
// snapshot, or the terminal in-memory selection (Ctrl+E/Y changes it) — and
// never consults the database. Capture happens synchronously inside Update
// through the internal/export boundary, deep-copying deduplicated names,
// ascending logical positions, typed cells with BLOB bytes, and snapshot
// metadata before any picker work or later model mutation can run; capture
// neither finalizes nor deactivates an active SELECT and starts no
// page/count/health-check/other database work. A backed tabular selection
// opens the pre-destination warning flow; every non-tabular selection
// reports exactly export.NoTabularDataMessage and opens nothing.

package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// exportSelection is the in-memory Ctrl+X targeting outcome: the currently
// viewed result's backed-tabular fact and, when tabular, everything the
// immutable capture needs. A false tabular fact covers empty/missing-backed
// selections, error views, non-tabular history entries, and evictions.
type exportSelection struct {
	tabular  bool
	columns  []string
	rows     [][]result.Value
	start    int64
	hasStart bool
	meta     history.SnapshotMetadata
	comp     history.Completeness
}

// exportSelection resolves the current export target in memory, following
// the current in-memory result selection: the selected immutable history
// entry during result-history browsing (ordinary and terminal, including
// Ctrl+E/Y changes), otherwise the active settled tabular result. No
// request, validation, refresh, or health check can start here.
func (m Model) exportSelection() exportSelection {
	if m.resultHistoryMode {
		if m.ResultHistory == nil {
			return exportSelection{}
		}
		e, ok := m.ResultHistory.Lookup(m.resultHistoryCursorID)
		if !ok {
			// Evicted or otherwise missing-backed selection.
			return exportSelection{}
		}
		if e.Kind != history.KindTabular {
			// Errors, write summaries, outcome-unknown entries, and
			// cancelled-before-rows markers are all non-tabular.
			return exportSelection{}
		}
		start, hasStart := int64(1), false
		if e.Metadata.HasRetainedRange {
			start, hasStart = int64(e.Metadata.RetainedStart), true
		}
		return exportSelection{
			tabular:  true,
			columns:  e.Columns,
			rows:     e.Rows,
			start:    start,
			hasStart: hasStart,
			meta:     e.Metadata,
			comp:     e.Completeness,
		}
	}
	// Idle active settled result: tabular exactly when a settled page is
	// present (an ordinary error view is not tabular data). Rows come from
	// the authoritative viewport cache in ascending position order, falling
	// back to the displayed page with its absolute offset.
	if m.Result != nil && m.Result.Err == nil && m.Result.Page != nil {
		page := m.Result.Page
		meta, comp := m.activeExportFacts(page)
		if c := m.viewportCache; c != nil && c.Len() > 0 {
			cached := c.Rows()
			rows := make([][]result.Value, len(cached))
			start, _ := c.Start()
			for i, row := range cached {
				rows[i] = row.Values
			}
			return exportSelection{
				tabular:  true,
				columns:  page.Columns,
				rows:     rows,
				start:    int64(start),
				hasStart: true,
				meta:     meta,
				comp:     comp,
			}
		}
		return exportSelection{
			tabular:  true,
			columns:  page.Columns,
			rows:     page.Rows,
			start:    m.Result.Offset + 1,
			hasStart: m.Result.Offset >= 0,
			meta:     meta,
			comp:     comp,
		}
	}
	return exportSelection{}
}

// activeExportFacts converts the active SELECT's authoritative state into
// snapshot metadata and completeness facts without finalizing anything:
// the retained-range/eviction facts come from the viewport cache, the known
// total from the settled count state (the count of the complete SELECT
// including the user's Limit, so classification needs no raw builder Limit),
// and invalid-UTF from the page. The terminal outcome stays undecided (an
// active SELECT is not finalized), so no terminal-outcome warning can ever
// derive from it. Issue #80: endpoint observations and traversal facts
// derive through the same shared helper as finalization, so identical active
// state produces equivalent facts and matching completeness labels —
// including the count/cache contradiction from Issue #78, which the active
// path preserves without clamping exactly as finalization does.
func (m Model) activeExportFacts(page *result.Page) (history.SnapshotMetadata, history.Completeness) {
	facts := history.CacheFacts{}
	if c := m.viewportCache; c != nil {
		facts = history.FactsFromCache(c)
	}
	if m.Result.ByteTruncated {
		facts.TruncatedByByteCap = true
	}
	af := deriveAuthoritativeFacts(facts, m.countState, m.pagePending, m.pageExhausted)
	meta := history.SnapshotMetadata{
		HasRetainedRange:   facts.HasRetainedRange,
		RetainedStart:      facts.Start,
		RetainedEnd:        facts.End,
		HasKnownTotal:      m.countState.Status == result.CountSuccess,
		KnownTotal:         m.countState.Total,
		RowCapEvicted:      facts.RowCapEvictions > 0,
		RowCapEvictions:    facts.RowCapEvictions,
		TruncatedByByteCap: facts.TruncatedByByteCap,
		InvalidUTF:         page.InvalidUTF,
		ReachedLow:         af.ReachedLow,
		ReachedHigh:        af.ReachedHigh,
	}
	return meta, history.Classify(meta, af.Traversal)
}

// handleExportKey resolves one Ctrl+X press in memory. Any request still
// pending — validation or schema refresh included — is consumed by the
// authoritative pending gate before this seam can run; the re-check here is
// the same gate's contract, never a second decision. Eligible selections
// capture synchronously and open the pre-destination warning flow;
// non-tabular selections report exactly the shared Issue #49 rejection,
// open no picker, and prepare nothing.
func (m Model) handleExportKey() Model {
	if m.validationPending || m.refreshPending || m.selectRequestPending() || m.writePending {
		m.inFlightNotice = inFlightBlockedFeedback("ctrl+x")
		m.exportNotice = ""
		m.exportPrepared = nil
		m.exportWarnings = nil
		m.exportWarningsOpen = false
		return m
	}
	sel := m.exportSelection()
	if err := (export.EligibilityInput{BackedTabular: sel.tabular}).Check(); err != nil {
		m.exportNotice = err.Error()
		m.exportPrepared = nil
		m.exportWarnings = nil
		m.exportWarningsOpen = false
		return m
	}
	m.exportNotice = ""
	captured := export.CaptureRows(sel.columns, sel.rows, sel.start, sel.hasStart, sel.meta, sel.comp)
	m.exportPrepared = &captured
	m.exportWarnings = exportWarningsFor(captured)
	m.exportWarningsOpen = true
	return m
}

// exportWarningsFor derives the exact pre-destination export warnings from
// captured typed metadata in one deterministic order: completeness state
// first, then truncation details (the shared Issue #31 byte-cap definition
// is referenced, never copied), then terminal-outcome information, and
// invalid-UTF disclosure last. Absent facts add no warning; metadata never
// enters the export payload.
func exportWarningsFor(c export.Capture) []string {
	var w []string
	if c.Completeness.Complete {
		w = append(w, "Result is complete")
	}
	if c.Completeness.Partial {
		w = append(w, "Result is partial")
	}
	if c.Completeness.Truncated {
		w = append(w, "Result is truncated")
	}
	if c.Metadata.RowCapEvicted {
		w = append(w, fmt.Sprintf("Rows evicted by the position cap: %d", c.Metadata.RowCapEvictions))
	}
	if c.Metadata.TruncatedByByteCap {
		w = append(w, result.ByteCapWarning)
	}
	switch c.Metadata.Outcome {
	case history.OutcomeCancelled:
		w = append(w, warningOutcome("Cancelled", c.Metadata.Reason, c.Metadata.HasFailurePosition, c.Metadata.FailurePosition))
	case history.OutcomeFailed:
		w = append(w, warningOutcome("Failed", c.Metadata.Reason, c.Metadata.HasFailurePosition, c.Metadata.FailurePosition))
	}
	if c.Metadata.InvalidUTF {
		w = append(w, result.UTFWarning)
	}
	return w
}

// warningOutcome renders one terminal-outcome warning line with its verbatim
// reason and the optional one-based last failure position.
func warningOutcome(label, reason string, hasPosition bool, position int64) string {
	line := label
	if reason != "" {
		line += ": " + reason
	}
	if hasPosition {
		line += " — last failure at row " + fmt.Sprintf("%d", position)
	}
	return line
}

// handleExportWarningsKey consumes keys while the export warning flow is
// open above its opener. Enter proceeds to destination selection (owned by
// later issues; the placeholder completion closes the flow restoring the
// exact opener), Esc cancels with the same exact restoration, and every
// other key is consumed with no leakage.
func (m Model) handleExportWarningsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// Issue #55: q and Ctrl+C open the shared quit confirmation over the
		// intact warning flow; the warning overlay owns no focused text.
		return m.openQuitConfirmation(), nil
	case "enter":
		// Issue #52: Enter proceeds to destination selection. The picker
		// snapshot is captured first, so Esc from the picker restores the
		// intact warning flow with its opener untouched.
		cmd := m.openPicker(pickerFlowExport, m.exportFormat)
		m.exportWarningsOpen = false
		return m, cmd
	case "esc":
		return m.cancelExport(), nil
	}
	return m, nil
}

// cancelExport closes the export warning flow without mutating anything
// else: the opener's exact mode, focus, selection, viewport, builder, active
// SELECT identity/lifetime, and terminal state were preserved untouched for
// the flow's whole life, and the captured copy stays stable.
func (m Model) cancelExport() Model {
	m.exportPrepared = nil
	m.exportWarnings = nil
	m.exportWarningsOpen = false
	return m
}

// Issue #52: on completion the destination picker restores the exact
// opener exactly like cancellation; persistence is owned by later issues.
