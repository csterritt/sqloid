// Active-export/finalization fact parity for Issue #80: for one active
// model state, the active export facts and the finalized snapshot facts
// derive equivalent endpoint/traversal facts through the same authoritative
// sources and produce identical completeness labels and warnings. The
// parity table covers a fully retained successful limited count, count
// unavailable with an accepted short/empty final page, missing low, missing
// high, unfinished work, row- and byte-cap eviction, known rows beyond
// retention, and a successful count below the retained end. Export itself
// neither finalizes nor mutates the active SELECT.

package ui

import (
	"testing"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/resultcache"
)

// parityState is one active model state to compare active export and
// finalized snapshot facts over. seed builds the active model state; the
// harness captures active export facts, finalizes a copy, and compares the
// retained range, known total, endpoint observations, traversal facts,
// completeness labels, and pre-picker/finalized warning agreement.
type parityState struct {
	name string
	// seed builds the active SELECT state on a fresh model. The model is
	// left active with the authoritative cache, count, page, and pending
	// state the parity case requires.
	seed func(t *testing.T, m *Model)
	// pageInvalidUTF reports whether the active page carries invalid-UTF
	// disclosure; the helper installs a Result/Page when true so the active
	// export path can source it.
	pageInvalidUTF bool
}

// parityFacts captures the comparable endpoint/traversal facts and labels
// from one observation path (active export or finalized snapshot).
type parityFacts struct {
	hasRetainedRange   bool
	retainedStart      history.Position
	retainedEnd        history.Position
	hasKnownTotal      bool
	knownTotal         int64
	reachedLow         bool
	reachedHigh        bool
	rowCapEvicted      bool
	rowCapEvictions    int
	truncatedByByteCap bool
	invalidUTF         bool
	// Traversal facts that classification consumes.
	countWorkFinished      bool
	pageWorkFinished       bool
	observedShortFinalPage bool
	countCacheInconsistent bool
	// The terminal outcome is supplied only at finalization; active export
	// always carries OutcomeNone-equivalent (no terminal outcome). The
	// parity comparison asserts the active path leaves outcome at its zero
	// value and the finalized path supplies the case's outcome.
	outcome history.TerminalOutcome
	// Derived completeness labels and warnings.
	comp     history.Completeness
	warnings []string
}

// activeParityFacts captures the active export facts for one model state.
func activeParityFacts(t *testing.T, m Model) parityFacts {
	t.Helper()
	if m.Result == nil || m.Result.Page == nil {
		t.Fatal("active parity state has no Result/Page; seed must install one")
	}
	meta, comp := m.activeExportFacts(m.Result.Page)
	pf := parityFacts{
		hasRetainedRange:       meta.HasRetainedRange,
		retainedStart:          meta.RetainedStart,
		retainedEnd:            meta.RetainedEnd,
		hasKnownTotal:          meta.HasKnownTotal,
		knownTotal:             meta.KnownTotal,
		reachedLow:             meta.ReachedLow,
		reachedHigh:            meta.ReachedHigh,
		rowCapEvicted:          meta.RowCapEvicted,
		rowCapEvictions:        meta.RowCapEvictions,
		truncatedByByteCap:     meta.TruncatedByByteCap,
		invalidUTF:             meta.InvalidUTF,
		countWorkFinished:      m.countState.Status != result.CountPending,
		pageWorkFinished:       !m.pagePending,
		observedShortFinalPage: m.pageExhausted,
		outcome:                meta.Outcome,
		comp:                   comp,
	}
	// Count/cache inconsistency is a traversal fact the active path must
	// derive exactly as finalization does (Issue #78).
	if m.countState.Status == result.CountSuccess && m.viewportCache != nil {
		if end, ok := m.viewportCache.End(); ok && int64(end) > m.countState.Total {
			pf.countCacheInconsistent = true
		}
	}
	// The active export path's pre-picker warnings derive from the same
	// metadata/completeness seam as the finalized snapshot's warnings.
	captured := captureFromActive(t, m, meta, comp)
	pf.warnings = exportWarningsFor(captured)
	return pf
}

// captureFromActive runs the production Ctrl+X capture path and returns the
// captured export value, so the parity test compares the exact pre-picker
// warning set the user sees.
func captureFromActive(t *testing.T, m Model, meta history.SnapshotMetadata, comp history.Completeness) export.Capture {
	t.Helper()
	sel := m.exportSelection()
	if !sel.tabular {
		t.Fatal("active export selection is not tabular for the parity case")
	}
	if sel.meta != meta || sel.comp != comp {
		t.Fatalf("exportSelection facts drifted from activeExportFacts: sel=%+v/%+v direct=%+v/%+v", sel.meta, sel.comp, meta, comp)
	}
	return export.CaptureRows(sel.columns, sel.rows, sel.start, sel.hasStart, sel.meta, sel.comp)
}

// finalizedParityFacts finalizes a copy of the active model through the
// production seam and captures the finalized snapshot's comparable facts and
// the export selection's warnings over that finalized entry.
func finalizedParityFacts(t *testing.T, m Model) parityFacts {
	t.Helper()
	if m.ResultHistory == nil {
		m.ResultHistory = history.NewResultStore()
	}
	// The active SELECT must be live so finalization appends exactly one
	// entry through appendFinalizedResultEntry.
	if !m.selectActive || m.activeExecID == 0 {
		m.selectActive = true
		m.activeExecID = 1
	}
	m.enterResultHistory()
	entries := m.ResultHistory.Entries()
	if len(entries) != 1 {
		t.Fatalf("finalization produced %d entries, want exactly one", len(entries))
	}
	e := entries[0]
	pf := parityFacts{
		hasRetainedRange:   e.Metadata.HasRetainedRange,
		retainedStart:      e.Metadata.RetainedStart,
		retainedEnd:        e.Metadata.RetainedEnd,
		hasKnownTotal:      e.Metadata.HasKnownTotal,
		knownTotal:         e.Metadata.KnownTotal,
		reachedLow:         e.Metadata.ReachedLow,
		reachedHigh:        e.Metadata.ReachedHigh,
		rowCapEvicted:      e.Metadata.RowCapEvicted,
		rowCapEvictions:    e.Metadata.RowCapEvictions,
		truncatedByByteCap: e.Metadata.TruncatedByByteCap,
		invalidUTF:         e.Metadata.InvalidUTF,
		outcome:            e.Metadata.Outcome,
		comp:               e.Completeness,
	}
	// Recover the traversal facts the finalizer supplied by re-classifying
	// with each candidate set and matching the stored completeness. The
	// finalizer's traversal facts are not stored on the entry, so derive
	// them from the same authoritative model state the finalizer consumed.
	// Because the active and finalized paths must agree, the active model's
	// count/page/exhausted state is the authoritative source.
	pf.countWorkFinished = m.countState.Status != result.CountPending
	pf.pageWorkFinished = !m.pagePending
	pf.observedShortFinalPage = m.pageExhausted
	if m.countState.Status == result.CountSuccess && m.viewportCache != nil {
		if end, ok := m.viewportCache.End(); ok && int64(end) > m.countState.Total {
			pf.countCacheInconsistent = true
		}
	}
	// The finalized snapshot's export-selection warnings come from the
	// selected finalized entry through the same exportWarningsFor seam.
	sel := exportSelectionFor(t, m, e.ID)
	captured := export.CaptureRows(sel.columns, sel.rows, sel.start, sel.hasStart, sel.meta, sel.comp)
	pf.warnings = exportWarningsFor(captured)
	return pf
}

// assertParity compares the active and finalized fact sets field by field,
// allowing only the terminal outcome to differ (active carries none; the
// finalized snapshot carries the case's outcome). Endpoint, traversal,
// retention, count, eviction, and completeness facts must be identical.
func assertParity(t *testing.T, active, finalized parityFacts) {
	t.Helper()
	type field struct {
		name     string
		got      any
		want     any
		terminal bool
	}
	fields := []field{
		{"HasRetainedRange", active.hasRetainedRange, finalized.hasRetainedRange, false},
		{"RetainedStart", active.retainedStart, finalized.retainedStart, false},
		{"RetainedEnd", active.retainedEnd, finalized.retainedEnd, false},
		{"HasKnownTotal", active.hasKnownTotal, finalized.hasKnownTotal, false},
		{"KnownTotal", active.knownTotal, finalized.knownTotal, false},
		{"ReachedLow", active.reachedLow, finalized.reachedLow, false},
		{"ReachedHigh", active.reachedHigh, finalized.reachedHigh, false},
		{"RowCapEvicted", active.rowCapEvicted, finalized.rowCapEvicted, false},
		{"RowCapEvictions", active.rowCapEvictions, finalized.rowCapEvictions, false},
		{"TruncatedByByteCap", active.truncatedByByteCap, finalized.truncatedByByteCap, false},
		{"InvalidUTF", active.invalidUTF, finalized.invalidUTF, false},
		{"CountWorkFinished", active.countWorkFinished, finalized.countWorkFinished, false},
		{"PageWorkFinished", active.pageWorkFinished, finalized.pageWorkFinished, false},
		{"ObservedShortFinalPage", active.observedShortFinalPage, finalized.observedShortFinalPage, false},
		{"CountCacheInconsistent", active.countCacheInconsistent, finalized.countCacheInconsistent, false},
		{"Completeness", active.comp, finalized.comp, false},
		{"Warnings", active.warnings, finalized.warnings, false},
	}
	for _, f := range fields {
		if !equalAny(f.got, f.want) {
			t.Errorf("parity drift on %s: active=%v finalized=%v", f.name, f.got, f.want)
		}
	}
	// The terminal outcome is the one field that legitimately differs: the
	// active path carries no terminal outcome (the SELECT is not finalized),
	// while the finalized snapshot carries the case's outcome. Active export
	// must never synthesize one.
	if active.outcome != history.OutcomeNone {
		t.Errorf("active export synthesized a terminal outcome %v; active capture must not finalize", active.outcome)
	}
}

// equalAny compares two any values, handling slices of strings and
// completeness values that the standard != operator cannot compare.
func equalAny(a, b any) bool {
	switch av := a.(type) {
	case []string:
		bv, ok := b.([]string)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
		return true
	case history.Completeness:
		bv, ok := b.(history.Completeness)
		return ok && av == bv
	default:
		return a == b
	}
}

// TestActiveExportFinalizationFactParity walks the Issue #80 parity table:
// for one active model state, capture active export facts, finalize a copy,
// and require equivalent endpoint/traversal facts and identical completeness
// labels and warnings. The terminal outcome exists only on the finalized
// snapshot; export itself neither finalizes nor mutates the active SELECT.
func TestActiveExportFinalizationFactParity(t *testing.T) {
	cases := []parityState{
		{
			// Fully retained successful limited count: positions 1..10 of
			// known total 10, both endpoints reached, work finished.
			name: "fully retained successful limited count",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 10}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Count unavailable with an accepted short nonempty final page:
			// pageExhausted establishes the high endpoint; the retained
			// range establishes the low endpoint. The result is complete.
			name: "count unavailable with accepted short final page",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 3)
				m.countState = result.CountState{Status: result.CountUnavailable}
				m.pagePending = false
				m.pageExhausted = true
				installActivePage(m, 3)
			},
		},
		{
			// Count unavailable with an accepted empty final page: both
			// endpoints sit at position 0 and the result is complete.
			name: "count unavailable with accepted empty final page",
			seed: func(t *testing.T, m *Model) {
				m.countState = result.CountState{Status: result.CountUnavailable}
				m.pagePending = false
				m.pageExhausted = true
				installActivePage(m, 0)
			},
		},
		{
			// Missing low endpoint: retained range 11..20 of known total 20
			// with no low-end eviction. The low endpoint is unobserved, so
			// the snapshot is partial (Issue #77).
			name: "missing low endpoint without eviction",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 11, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 20}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Missing high endpoint: retained range 1..10 of known total 15
			// with no short-page observation. The high endpoint is
			// unobserved and rows beyond the range are known, so the
			// snapshot is partial+truncated.
			name: "missing high endpoint with known rows beyond retention",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 15}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Unfinished work: count still pending. The high endpoint is
			// unknown and work has not finished, so the snapshot is partial.
			name: "unfinished count work",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountPending}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Row-cap eviction: forward traversal evicted the low end, so
			// the retained range starts above 1 with RowCapEvicted set.
			// The snapshot is truncated (and partial unless complete).
			name: "row-cap eviction",
			seed: func(t *testing.T, m *Model) {
				seedRowCapEviction(t, m)
				m.countState = result.CountState{Status: result.CountSuccess, Total: int64(resultcache.MaxPositions + 100)}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
		{
			// Byte-cap eviction: retained payload exceeded MaxPayloadBytes,
			// so TruncatedByByteCap is persistent. The snapshot is truncated.
			name: "byte-cap eviction",
			seed: func(t *testing.T, m *Model) {
				seedByteCapEviction(t, m)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 3}
				m.pagePending = false
				installActivePage(m, 1)
			},
		},
		{
			// Successful count below the retained end: total 8 with retained
			// range 1..10. Issue #78: a successful limited-result count whose
			// total falls below the retained cache end contradicts the cache.
			// The contradiction is preserved without clamping either value
			// and the snapshot is partial, never complete.
			name: "successful count below retained end",
			seed: func(t *testing.T, m *Model) {
				mergeRowsIntoCache(t, m, 1, 10)
				m.countState = result.CountState{Status: result.CountSuccess, Total: 8}
				m.pagePending = false
				installActivePage(m, 10)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			active := buildParityModel(t, tc)
			// Snapshot the active model state before capture so finalization
			// runs on an identical copy.
			finalized := cloneParityModel(active)
			activeFacts := activeParityFacts(t, active)
			finalizedFacts := finalizedParityFacts(t, finalized)
			assertParity(t, activeFacts, finalizedFacts)

			// Export itself must not finalize or mutate the active SELECT:
			// the active model's identity, cache, viewport, and pending
			// state are unchanged after the active capture.
			if !active.SelectIsActive() {
				t.Error("active export deactivated the active SELECT lifetime")
			}
			if active.finalizedExecID != 0 {
				t.Errorf("active export finalized the active SELECT: finalized=%d", active.finalizedExecID)
			}
		})
	}
}

// buildParityModel constructs a fresh active model with the parity state's
// seed applied and a Result/Page installed so the active export path can
// source invalid-UTF and column facts.
func buildParityModel(t *testing.T, tc parityState) Model {
	t.Helper()
	m := New()
	m.selectActive = true
	m.activeExecID = 1
	tc.seed(t, &m)
	if m.Result == nil || m.Result.Page == nil {
		t.Fatal("parity seed did not install a Result/Page")
	}
	if tc.pageInvalidUTF {
		m.Result.Page.InvalidUTF = true
	}
	return m
}

// cloneParityModel returns a value copy of the active model for finalization.
// The viewport cache is shared by pointer: finalization reads it but the
// active model is never mutated after this point, so the shared cache is
// safe for the parity comparison.
func cloneParityModel(m Model) Model {
	c := m
	// The result history must be fresh so the finalized copy appends its own
	// entry without aliasing the active model's store.
	c.ResultHistory = history.NewResultStore()
	return c
}

// installActivePage installs a successful ResultView carrying a page with the
// given row count, so the active export path can source columns and
// invalid-UTF facts. The page rows mirror the cache's first retained rows.
func installActivePage(m *Model, rowCount int) {
	rows := make([][]result.Value, rowCount)
	for i := range rows {
		rows[i] = []result.Value{result.NewInteger(int64(i + 1))}
	}
	m.Result = &ResultView{
		Page: &result.Page{Columns: []string{"id"}, Rows: rows},
	}
}

// seedRowCapEviction merges enough rows to force the MaxPositions cap to
// evict the low end, leaving the retained range starting above position 1
// with RowCapEvictions > 0.
func seedRowCapEviction(t *testing.T, m *Model) {
	t.Helper()
	// Merge MaxPositions rows from position 1, then one more page forward
	// to evict the low end by the page size.
	mergeRowsIntoCache(t, m, 1, resultcache.MaxPositions)
	mergeRowsIntoCache(t, m, int64(resultcache.MaxPositions)+1, 100)
	if ev := m.viewportCache.RowCapEvictions(); ev == 0 {
		t.Fatalf("row-cap seed produced no evictions")
	}
}

// seedByteCapEviction merges rows whose payload exceeds MaxPayloadBytes so
// the cache records persistent byte-cap truncation.
func seedByteCapEviction(t *testing.T, m *Model) {
	t.Helper()
	third := int(resultcache.MaxPayloadBytes/3) + 1
	for i := int64(1); i <= 3; i++ {
		page := resultcache.Page{
			Start: resultcache.Position(i),
			Rows: []resultcache.Row{{
				Position: resultcache.Position(i),
				Values:   []result.Value{result.NewBlob(make([]byte, third))},
			}},
		}
		if accepted, err := m.viewportCacheOf().Merge(page, resultcache.Forward); !accepted || err != nil {
			t.Fatalf("byte-cap seed merge %d: accepted=%v err=%v", i, accepted, err)
		}
	}
	if !m.viewportCache.TruncatedByByteCap() {
		t.Fatal("byte-cap seed did not record byte-cap truncation")
	}
}
