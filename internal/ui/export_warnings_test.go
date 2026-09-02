// Export metadata-to-warning presentation coverage for Issue #49, per the
// Export warnings, Cache and snapshot invariant, and Testing Decisions
// decisions in Notes/PRD-sqloid.md. One deterministic order is presented
// before any destination selection or confirmation: completeness state
// first, truncation details next (reusing Issue #31's shared exact
// `Result truncated: 64 MiB cache limit` definition rather than copying it),
// terminal-outcome information then, and invalid-UTF disclosure last. Absent
// facts add no warning; metadata never appears as a serializer row, column,
// object, property, or synthetic value; and cancel/complete restore the
// exact opener from active, historical, and terminal openers with the
// captured data stable and zero database work.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
)

// warningsCase is one metadata/completeness combination with the exact
// warning set expected, in canonical order.
type warningsCase struct {
	name    string
	meta    history.SnapshotMetadata
	comp    history.Completeness
	want    []string
	wantLen int
}

// TestExportWarningMatrix pins the deterministic metadata-to-warning
// presentation for exclusive and coexisting labels, both caps, all three
// terminal outcomes with reasons and failure positions, and invalid UTF.
func TestExportWarningMatrix(t *testing.T) {
	byteCap := result.ByteCapWarning // Issue #31's shared exact definition
	tests := []warningsCase{
		{
			name: "complete is exclusive",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeSuccess},
			comp: history.Completeness{Complete: true},
			want: []string{"Result is complete"},
		},
		{
			name: "partial",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeSuccess},
			comp: history.Completeness{Partial: true},
			want: []string{"Result is partial"},
		},
		{
			name: "truncated",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeSuccess},
			comp: history.Completeness{Truncated: true},
			want: []string{"Result is truncated"},
		},
		{
			name: "partial plus truncated",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeSuccess},
			comp: history.Completeness{Partial: true, Truncated: true},
			want: []string{"Result is partial", "Result is truncated"},
		},
		{
			name: "row-cap eviction",
			meta: history.SnapshotMetadata{RowCapEvicted: true, RowCapEvictions: 3},
			want: []string{"Rows evicted by the position cap: 3"},
		},
		{
			name: "byte-cap truncation reuses the shared definition",
			meta: history.SnapshotMetadata{TruncatedByByteCap: true},
			want: []string{byteCap},
		},
		{
			name: "cancelled with reason",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeCancelled, Reason: "user interrupt"},
			want: []string{"Cancelled: user interrupt"},
		},
		{
			name: "failed with reason and failure position",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeFailed, Reason: "disk full", HasFailurePosition: true, FailurePosition: 7},
			want: []string{"Failed: disk full — last failure at row 7"},
		},
		{
			name: "success adds no warning",
			meta: history.SnapshotMetadata{Outcome: history.OutcomeSuccess},
		},
		{
			name: "invalid UTF",
			meta: history.SnapshotMetadata{InvalidUTF: true},
			want: []string{result.UTFWarning},
		},
		{
			name: "all truthful facts together in canonical order",
			meta: history.SnapshotMetadata{
				RowCapEvicted: true, RowCapEvictions: 12, TruncatedByByteCap: true,
				InvalidUTF: true,
				Outcome:    history.OutcomeFailed, Reason: "broken", HasFailurePosition: true, FailurePosition: 4,
			},
			comp: history.Completeness{Partial: true, Truncated: true},
			want: []string{
				"Result is partial", "Result is truncated",
				"Rows evicted by the position cap: 12", byteCap,
				"Failed: broken — last failure at row 4",
				result.UTFWarning,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exportWarningsFor(export.CaptureRows([]string{"id"}, nil, 1, false, tt.meta, tt.comp))
			if len(got) != len(tt.want) {
				t.Fatalf("warnings = %q, want %q", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("warning %d = %q, want %q (canonical order)", i, got[i], tt.want[i])
				}
			}
		})
	}
	// The byte-cap literal must be referenced from internal/result, never
	// copied by the presentation layer.
	if byteCap != "Result truncated: 64 MiB cache limit" {
		t.Fatalf("shared definition = %q, want Issue #31's exact wording", byteCap)
	}
}

// openerScenario builds one export opener per kind with counting fakes, plus
// its expected warning set.
type openerScenario struct {
	name    string
	warning string
	open    func(t *testing.T) Model
}

// TestExportWarningFlowBeforeDestinationSelection drives Ctrl+X from active,
// historical, and terminal openers, proving the warnings render before any
// destination selection, the captured payload stays free of metadata, cancel
// (Esc) and successful completion restore the exact opener, and no database
// work occurs.
func TestExportWarningFlowBeforeDestinationSelection(t *testing.T) {
	scenarios := []openerScenario{
		{
			name:    "active opener with partial truncation",
			warning: "Result is partial",
			open: func(t *testing.T) Model {
				m := resultModel(t, firstPageRows(defaultPageRows), nil)
				m.Result.ByteTruncated = true
				return m
			},
		},
		{
			name:    "historical opener with failed outcome",
			warning: "Failed",
			open: func(t *testing.T) Model {
				m := resultModel(t, threeRowPage(), nil)
				meta, err := history.NewSnapshotMetadata(
					history.CacheFacts{HasRetainedRange: true, Start: 1, End: 2},
					history.Lifecycle{Outcome: history.OutcomeFailed, Reason: "mid-page failure"})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := m.ResultHistory.AppendFinalized(history.ResultEntry{
					ExecutionID: nextProbeExecutionID(), Kind: history.KindTabular,
					Columns: []string{"id", "v"}, Metadata: meta,
					Rows: [][]result.Value{
						{result.NewInteger(1), result.NewText("a")},
						{result.NewInteger(2), result.NewNull()},
					},
					Completeness: history.Completeness{Partial: true},
				}); !ok {
					t.Fatal("setup: entry rejected")
				}
				m.enterResultHistoryMode()
				m.resultHistoryStep(false)
				return m
			},
		},
		{
			name:    "terminal opener with byte-cap truncation",
			warning: result.ByteCapWarning,
			open: func(t *testing.T) Model {
				m, _ := updateUnresolvedUpdate(t)
				meta, err := history.NewSnapshotMetadata(
					history.CacheFacts{HasRetainedRange: true, Start: 1, End: 1, TruncatedByByteCap: true},
					history.Lifecycle{Outcome: history.OutcomeSuccess})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := m.ResultHistory.AppendFinalized(history.ResultEntry{
					ExecutionID: nextProbeExecutionID(), Kind: history.KindTabular,
					Columns: []string{"id", "v"}, Metadata: meta,
					Rows:         [][]result.Value{{result.NewInteger(1), result.NewText("a")}},
					Completeness: history.Completeness{Truncated: true},
				}); !ok {
					t.Fatal("setup: entry rejected")
				}
				m, _ = pressKey(m, ctrlKey(tea.KeyCtrlY))
				return m
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			m := sc.open(t)
			baseline := openerFingerprint(m)
			nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
			if cmd != nil {
				t.Fatal("Ctrl+X issued a command")
			}
			if nm.exportPrepared == nil || !nm.exportWarningsOpen {
				t.Fatalf("flow not open: %q", nm.exportNotice)
			}
			view := nm.View()
			if !strings.Contains(view, "Export result") {
				t.Fatalf("warnings not rendered before destination selection:\n%s", view)
			}
			// The exact derived warnings render in canonical order (wrapping
			// aside, the model owns the verified line list).
			if !strings.Contains(strings.Join(nm.exportWarnings, "\n"), sc.warning) {
				t.Fatalf("warning %q not derived: %q", sc.warning, nm.exportWarnings)
			}
			// Metadata never appears as a serializer record: the export-owned
			// payload carries only names/positions/values, and no warning text
			// can leak into any name or cell token.
			payload := nm.exportPrepared.Payload
			for _, cell := range payload.Names {
				if strings.Contains(cell, "Result") || strings.Contains(cell, "Failed") || strings.Contains(cell, "UTF-8") {
					t.Errorf("payload name %q carries warning metadata", cell)
				}
			}
			for _, row := range payload.Rows {
				for _, v := range row {
					token := v.Display()
					if strings.Contains(token, "Result") || strings.Contains(token, "Failed") || strings.Contains(token, "UTF-8") {
						t.Errorf("payload value token %q carries warning metadata", token)
					}
				}
			}
			// Snapshot the captured payload; it must stay byte-identical
			// through cancel, later live mutation, and completion.
			captured := *nm.exportPrepared
			// Cancel with Esc: exact opener restoration, captured data stable.
			cancelled, cmd := pressKey(nm, tea.KeyMsg{Type: tea.KeyEsc})
			if cmd != nil {
				t.Fatal("export cancel issued a command")
			}
			if cancelled.exportWarningsOpen || cancelled.exportPrepared != nil || cancelled.exportWarnings != nil {
				t.Fatal("Esc did not close the export flow")
			}
			if fp := openerFingerprint(cancelled); fp != baseline {
				t.Errorf("Esc opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
			}
			if len(captured.Payload.Positions) == 0 {
				t.Error("captured data lost before completion")
			}

			// Reopen and complete successfully: the same exact restoration.
			reopened, _ := pressKey(m, ctrlKey(tea.KeyCtrlX))
			if !reopened.exportWarningsOpen {
				t.Fatal("setup: flow did not reopen")
			}
			completed, cmd := pressKey(reopened, tea.KeyMsg{Type: tea.KeyEnter})
			// Issue #52: Enter now proceeds to the destination picker, whose
			// start listing is issued as a command.
			if cmd == nil || !completed.pickerOpen {
				t.Fatal("export completion did not open the destination picker")
			}
			if completed.exportWarningsOpen || completed.exportPrepared == nil {
				t.Fatal("completion mishandled the export flow context")
			}
			// Esc from the picker restores the exact opener — the intact
			// export warning flow — before Esc closes that flow like before.
			completed, _ = pressKey(completed, tea.KeyMsg{Type: tea.KeyEscape})
			if !completed.exportWarningsOpen || completed.exportPrepared == nil {
				t.Fatal("picker Esc did not restore the intact export flow")
			}
			completed, _ = pressKey(completed, tea.KeyMsg{Type: tea.KeyEscape})
			if completed.exportWarningsOpen || completed.exportPrepared != nil {
				t.Fatal("completion did not close the export flow")
			}
			if fp := openerFingerprint(completed); fp != baseline {
				t.Errorf("completion opener fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
			}
		})
	}
}

// openerFingerprint summarizes every state exact restoration must preserve:
// mode, focus, selections, viewport, builder, active SELECT identity and
// lifetime, and terminal state.
func openerFingerprint(m Model) string {
	return strings.Join([]string{
		string(rune(m.Focus)), string(rune(m.Scroll)),
		string(rune(len(m.Fields))),
		string(rune(int(m.QB.Command()))), string(rune(int(m.QB.DownstreamGeneration()))),
		boolStr(m.Result != nil), boolStr(m.resultHistoryMode),
		string(rune(int(m.resultHistoryCursorID))),
		boolStr(m.historyMode),
		boolStr(m.SelectIsActive()), string(rune(int(m.activeExecID))),
		string(rune(int(m.finalizedExecID))),
		m.countState.Header(),
		string(rune(int(m.viewportGen))),
		string(rune(int(m.terminalState))),
		boolStr(m.terminalHelpOpen),
		boolStr(m.pagePending), boolStr(m.firstPagePending), boolStr(m.countPendingFlag),
	}, "|")
}

func boolStr(b bool) string {
	if b {
		return "t"
	}
	return "f"
}

// TestActiveExportShortFirstPageCompleteWithCountUnavailable proves that
// active export of a short nonempty first page with count unavailable
// classifies complete: pageExhausted feeds ObservedShortFinalPage and both
// endpoints are established (Issue #73 AC3).
func TestActiveExportShortFirstPageCompleteWithCountUnavailable(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(3)}
	count := &fakeCountExecutor{err: errors.New("count failed")}
	m := firstSelectModel(exec)
	m.Count = count.count
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	if !m.pageExhausted {
		t.Fatal("short first page did not set pageExhausted")
	}
	sel := m.exportSelection()
	if !sel.tabular {
		t.Fatal("active export selection not tabular")
	}
	if !sel.comp.Complete {
		t.Errorf("active export completeness = %v, want Complete (short first page, count unavailable)", sel.comp)
	}
}

// TestActiveExportEmptyFirstPageCompleteWithCountUnavailable proves that
// active export of an empty first page with count unavailable classifies
// complete: pageExhausted feeds ObservedShortFinalPage and both endpoints
// are established at position 0 (Issue #73 AC3).
func TestActiveExportEmptyFirstPageCompleteWithCountUnavailable(t *testing.T) {
	exec := &fakeSelectExecutor{page: firstPageRows(0)}
	count := &fakeCountExecutor{err: errors.New("count failed")}
	m := firstSelectModel(exec)
	m.Count = count.count
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	if !m.pageExhausted {
		t.Fatal("empty first page did not set pageExhausted")
	}
	sel := m.exportSelection()
	if !sel.tabular {
		t.Fatal("active export selection not tabular")
	}
	if !sel.comp.Complete {
		t.Errorf("active export completeness = %v, want Complete (empty first page, count unavailable)", sel.comp)
	}
}
