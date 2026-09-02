// Ctrl+X request gating and non-tabular rejection coverage for Issue #49,
// per the Result export scope, Export warnings, Context/Action matrix, and
// Testing Decisions in Notes/PRD-sqloid.md. Every request-bearing state —
// schema validation/refresh, estimate, SELECT first/later page, count-only
// work, cancellation settlement, write beginning/executing/rollback/commit —
// routes Ctrl+X through the existing generic pending gate with explanatory
// request feedback, no capture, no picker, and no additional database work.
// At idle, every non-tabular selection (empty/missing-backed, errors, write
// summaries, outcome-unknown entries, cancelled-before-rows markers) reports
// exactly the one shared `selected result has no tabular data to export`
// definition owned by internal/export, while retained-row cancelled/failed
// snapshots and zero-row tabular SELECT snapshots remain exportable controls.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/schema"
)

// pendingState is one request-bearing scripted state with its executor
// fake for the no-additional-work assertion.
type pendingState struct {
	name  string
	build func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher)
}

// countOnlyAfterSettled builds a count-only pending model on an already
// settled first page.
func countOnlyAfterSettled(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
	t.Helper()
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 7}
	m := pendingCountOnly(t, exec, count)
	refresh := &fakeRefresher{}
	page := &fakePageExecutor{rowsShown: 3}
	m.Page = page.page
	m.Refresher = refresh
	return m, exec, count, page, refresh
}

// TestExportGatedDuringEveryPendingState proves the generic pending gate
// consumes Ctrl+X during every request-bearing state with the exact shared
// feedback, no capture, no picker, and no additional database work.
func TestExportGatedDuringEveryPendingState(t *testing.T) {
	tests := []pendingState{
		{
			name: "schema validation pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}
				refresh := &fakeRefresher{}
				m := selectModel(reader, refresh)
				m.Select = (&fakeSelectExecutor{page: threeRowPage()}).selectPage
				m = focusField(m, commandFieldLabel)
				next, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				mm := next.(Model)
				if enterCmd == nil {
					t.Fatal("setup: runnable Enter issued no command")
				}
				next, _ = mm.Update(enterCmd())
				mm = next.(Model)
				if !mm.validating || !mm.validationPending {
					t.Fatal("setup: validation workflow not pending")
				}
				return mm, nil, nil, nil, refresh
			},
		},
		{
			name: "schema refresh pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m := modelWithQB(validSelectQB())
				refresh := &fakeRefresher{}
				m.Refresher = refresh
				m.refreshPending = true
				return m, nil, nil, nil, refresh
			},
		},
		{
			name: "estimate pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m, cmd := openPreparation(t, prepUpdateQB(true), &prepFakeEstimator{})
				if !m.prepPending {
					t.Fatal("setup: estimate not pending")
				}
				if cmd != nil {
					// Invoke the estimate request so the fake dispatches; the
					// settled message is deliberately not applied so the
					// request stays pending for the gate.
					if msg := cmd(); msg == nil {
						t.Fatal("setup: estimate command produced nothing")
					}
				}
				return m, nil, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "select first page pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				exec := &fakeSelectExecutor{page: threeRowPage()}
				return pendingFirstPage(t, exec), exec, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "select later page pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
				pageExec := &fakePageExecutor{rowsShown: 11}
				m := pendingLaterPage(t, exec, pageExec)
				return m, exec, nil, pageExec, &fakeRefresher{}
			},
		},
		{name: "count-only pending", build: countOnlyAfterSettled},
		{
			name: "cancellation settlement pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				exec := &fakeSelectExecutor{page: threeRowPage()}
				m := pendingFirstPage(t, exec)
				// Ctrl+W marks the visible cancelling state while the held
				// first page still owns its request slot.
				next, cmd := pressKey(m, ctrlKey(tea.KeyCtrlW))
				if cmd == nil {
					t.Fatal("setup: Ctrl+W issued no cancellation dispatch")
				}
				if !next.selectCancelling || !next.firstPagePending {
					t.Fatal("setup: cancellation settlement state not reached")
				}
				return next, exec, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "write beginning pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m := heldWriteModel(t, committedNeverRunFake())
				return m, nil, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "write executing pending",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m := heldWriteModel(t, committedNeverRunFake())
				return holdWritePhase(t, m, connection.WritePhaseExecuting), nil, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "write rollback cleanup noncancellable",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m := heldWriteModel(t, committedNeverRunFake())
				return holdWritePhase(t, m, connection.WritePhaseRollbackCleanup), nil, nil, nil, &fakeRefresher{}
			},
		},
		{
			name: "write committing noncancellable",
			build: func(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
				t.Helper()
				m := heldWriteModel(t, committedNeverRunFake())
				return holdWritePhase(t, m, connection.WritePhaseCommitting), nil, nil, nil, &fakeRefresher{}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, sel, count, page, refresh := tc.build(t)
			var before [4]int
			if sel != nil {
				before[0] = sel.calls
			}
			if count != nil {
				before[1] = count.calls
			}
			if page != nil {
				before[2] = page.issued
			}
			if refresh != nil {
				before[3] = refresh.calls
			}
			nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
			if cmd != nil {
				t.Fatalf("gated Ctrl+X returned %v; the held request could stack", cmd)
			}
			if nm.inFlightNotice != ExportBlockedFeedback {
				t.Fatalf("feedback = %q, want exactly %q", nm.inFlightNotice, ExportBlockedFeedback)
			}
			if !strings.Contains(nm.View(), ExportBlockedFeedback) {
				t.Fatalf("feedback not rendered:\n%s", nm.View())
			}
			if nm.exportPrepared != nil || nm.exportWarningsOpen {
				t.Fatal("gated Ctrl+X captured or opened the export flow")
			}
			if nm.Popup != nil {
				t.Fatal("gated Ctrl+X opened a picker")
			}
			if sel != nil && sel.calls != before[0] {
				t.Fatalf("gated Ctrl+X dispatched extra work: %d -> %d", before[0], sel.calls)
			}
			if count != nil && count.calls != before[1] {
				t.Fatalf("gated Ctrl+X dispatched count work: %d -> %d", before[1], count.calls)
			}
			if page != nil && page.issued != before[2] {
				t.Fatalf("gated Ctrl+X dispatched page work: %d -> %d", before[2], page.issued)
			}
			if refresh != nil && refresh.calls != before[3] {
				t.Fatalf("gated Ctrl+X dispatched refresh work: %d -> %d", before[3], refresh.calls)
			}
		})
	}
}

// nonTabularSelection is one idle selection Ctrl+X must reject with the one
// shared Issue #49 definition.
type nonTabularSelection struct {
	name  string
	build func(t *testing.T) Model
}

// seedEntry appends one finalized non-tabular result entry.
func seedEntry(m Model, kind history.ResultKind, reason string) Model {
	m.ResultHistory.AppendFinalized(history.ResultEntry{ExecutionID: nextProbeExecutionID(), Kind: kind, Reason: reason, Summary: reason})
	return m
}

// nextProbeExecutionID hands out fresh distinct execution IDs for seeded
// probe entries so AppendFinalized always retains them.
var probeExecutionID uint64

func nextProbeExecutionID() uint64 {
	probeExecutionID++
	return probeExecutionID + 1000
}

// TestIdleNonTabularExportRejection proves every ordinary and terminal
// non-tabular selection reports exactly the shared message, with no capture,
// no picker, no command, and no database work.
func TestIdleNonTabularExportRejection(t *testing.T) {
	tests := []nonTabularSelection{
		{
			name:  "nothing displayed",
			build: func(t *testing.T) Model { return sized(New(), 80, 24).(Model) },
		},
		{
			name:  "ordinary error view",
			build: func(t *testing.T) Model { return resultModel(t, nil, contextErr()) },
		},
		{
			name: "selected error entry",
			build: func(t *testing.T) Model {
				m := resultModel(t, threeRowPage(), nil)
				m = seedEntry(m, history.KindError, "no such table: gone")
				m.enterResultHistoryMode()
				m.resultHistoryStep(false) // move off the newest tabular entry
				return m
			},
		},
		{
			name: "selected cancelled-before-rows entry",
			build: func(t *testing.T) Model {
				m := resultModel(t, threeRowPage(), nil)
				m = seedEntry(m, history.KindCancelled, "interrupted")
				m.enterResultHistoryMode()
				m.resultHistoryStep(false)
				return m
			},
		},
		{
			name: "selected write summary entry",
			build: func(t *testing.T) Model {
				m := resultModel(t, threeRowPage(), nil)
				m = seedEntry(m, history.KindWrite, "committed 1 row")
				m.enterResultHistoryMode()
				m.resultHistoryStep(false)
				return m
			},
		},
		{
			name: "selected outcome-unknown entry",
			build: func(t *testing.T) Model {
				m, _ := updateUnresolvedUpdate(t)
				return m
			},
		},
		{
			name: "missing-backed selection over an empty store",
			build: func(t *testing.T) Model {
				m := sized(New(), 80, 24).(Model)
				m.resultHistoryMode = true
				m.resultHistoryCursorID = history.EntryID(9999)
				return m
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.build(t)
			nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
			if cmd != nil {
				t.Fatal("rejected Ctrl+X issued a command")
			}
			if nm.exportNotice != export.NoTabularDataMessage {
				t.Fatalf("rejection = %q, want exactly %q", nm.exportNotice, export.NoTabularDataMessage)
			}
			if nm.terminalState == TerminalNone && nm.resultHistoryNotice == "" && !strings.Contains(nm.View(), export.NoTabularDataMessage) {
				t.Fatalf("rejection not rendered:\n%s", nm.View())
			}
			if nm.exportPrepared != nil || nm.exportWarningsOpen || nm.Popup != nil {
				t.Fatal("rejected Ctrl+X captured, opened warnings, or opened a picker")
			}
		})
	}
}

// TestRetainedRowsRemainExportable proves terminal outcome and emptiness are
// never confused with data shape: cancelled/failed snapshots that retained
// rows and zero-row tabular SELECT snapshots stay eligible and capture.
func TestRetainedRowsRemainExportable(t *testing.T) {
	// A failed snapshot that retained rows stays exportable.
	meta, err := history.NewSnapshotMetadata(history.CacheFacts{HasRetainedRange: true, Start: 1, End: 2},
		history.Lifecycle{Outcome: history.OutcomeFailed, Reason: "disk full", FailurePosition: 2, HasFailurePosition: true, InvalidUTF: true})
	if err != nil {
		t.Fatal(err)
	}
	m := resultModel(t, threeRowPage(), nil)
	if _, ok := m.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID: nextProbeExecutionID(), Kind: history.KindTabular,
		Columns: []string{"id", "v"}, Rows: [][]result.Value{
			{result.NewInteger(1), result.NewText("a")},
			{result.NewInteger(2), result.NewNull()},
		},
		Metadata: meta, Completeness: history.Completeness{Partial: true, Truncated: true},
	}); !ok {
		t.Fatal("setup: failed snapshot rejected")
	}
	m.enterResultHistoryMode()
	m.resultHistoryStep(false) // select the failed tabular snapshot
	if m.resultHistoryCursorID == 0 {
		t.Fatal("setup: selection missing")
	}
	nm, cmd := pressKey(m, ctrlKey(tea.KeyCtrlX))
	if cmd != nil || nm.exportPrepared == nil {
		t.Fatalf("retained-row failed snapshot not exportable: %q", nm.exportNotice)
	}

	// A zero-row SELECT snapshot with tabular columns stays exportable.
	m2 := resultModel(t, &result.Page{Columns: []string{"id"}}, nil)
	m2 = seedEntry(m2, history.KindTabular, "")
	if _, ok := m2.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID: nextProbeExecutionID(), Kind: history.KindTabular,
		Columns: []string{"id"},
	}); !ok {
		t.Fatal("setup: zero-row snapshot rejected")
	}
	m2.enterResultHistoryMode()
	m2.resultHistoryStep(false)
	nm2, cmd := pressKey(m2, ctrlKey(tea.KeyCtrlX))
	if cmd != nil || nm2.exportPrepared == nil {
		t.Fatalf("zero-row tabular snapshot not exportable: %q", nm2.exportNotice)
	}
	if len((*nm2.exportPrepared).Payload.Rows) != 0 {
		t.Errorf("zero-row capture rows = %d, want 0", len((*nm2.exportPrepared).Payload.Rows))
	}
	if !reflectNames((*nm2.exportPrepared).Payload.Names, []string{"id"}) {
		t.Errorf("zero-row capture names = %q, want [id]", (*nm2.exportPrepared).Payload.Names)
	}
}
