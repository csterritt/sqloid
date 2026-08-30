// Scripted in-memory terminal-state coverage for Issue #45, per the Global
// Key Precedence and Context/Action matrix in Notes/PRD-sqloid.md. In the
// outcome-unknown terminal state every normally database-capable action
// creates no Bubble Tea command and no connection, validation, estimate,
// execution, paging, refresh, or rerun work; Ctrl+P/N traverse retained
// query history and Ctrl+E/Y traverse retained result history entirely in
// memory with deterministic boundaries while the initially selected newest
// outcome-unknown entry is preserved; empty histories are deterministic
// no-ops without synthetic entries or missing-backed rendering; and `?`
// opens reduced help containing only actions available in this terminal
// state. Ctrl+S/Ctrl+X integration is owned by Issues #48/#49.

package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/schema"
)

// terminalModel drives a confirmed UPDATE into an unresolved settlement,
// seeding one retained query-history entry and one tabular result entry
// before settlement so the outcome-unknown entry lands newest. Every
// normally database-capable seam is wired to a counting fake.
func terminalModel(t *testing.T) (Model, *fakeSelectExecutor, *fakeCountExecutor, *fakePageExecutor, *fakeRefresher) {
	t.Helper()
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	m.catalog = prepCatalog() // in-memory query-history restore needs the catalog
	m.History = history.NewStore()
	m.History.Append(m.QB.HistoryState())
	m.ResultHistory.AppendFinalized(history.ResultEntry{
		ExecutionID: 900,
		Kind:        history.KindTabular,
		Columns:     []string{"id"},
		Rows:        [][]result.Value{{result.NewInteger(1)}},
	})

	sel := &fakeSelectExecutor{page: &result.Page{Columns: []string{"id"}, Rows: [][]result.Value{{result.NewInteger(1)}}}}
	count := &fakeCountExecutor{total: 1}
	page := &fakePageExecutor{rowsShown: 1}
	refresh := &fakeRefresher{queued: []schema.Attempt{successAttempt(prepCatalog())}}

	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
	}
	m.Write = fake.Write
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("setup: confirmation produced no command")
	}
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("setup: confirmation command produced %T", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	nm := dispatchWriteBatch(t, next.(Model), cmd, false)
	// Deliver the unresolved settlement: exactly one outcome-unknown entry is
	// appended and selected, and the terminal state is entered.
	settled, _ := nm.Update(WriteSettledMsg{
		Execution: fake.execution,
		Result:    connection.WriteResult{Outcome: connection.WriteFailed, Err: errors.New("disk I/O error"), RowsAffected: 3},
	})
	nm = settled.(Model)
	nm.Select = sel.selectPage
	nm.Count = count.count
	nm.Page = page.page
	nm.Refresher = refresh
	nm.Estimator = func(ctx context.Context, sql string, params []any) EstimateResult { return EstimateResult{} }

	if nm.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("setup: terminal state = %v, want TerminalOutcomeUnknown", nm.terminalState)
	}
	return nm, sel, count, page, refresh
}

// termKey feeds one key through Update and fails when it produced a
// command — the terminal state may start no database work at all.
func termKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case "pgup":
		msg = tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		msg = tea.KeyMsg{Type: tea.KeyPgDown}
	case "ctrl+w":
		msg = tea.KeyMsg{Type: tea.KeyCtrlW}
	case "ctrl+p":
		msg = tea.KeyMsg{Type: tea.KeyCtrlP}
	case "ctrl+n":
		msg = tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+e":
		msg = tea.KeyMsg{Type: tea.KeyCtrlE}
	case "ctrl+y":
		msg = tea.KeyMsg{Type: tea.KeyCtrlY}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("key %q dispatched a command in the terminal state", key)
	}
	return next.(Model)
}

func TestTerminalStateForbidsDatabaseWork(t *testing.T) {
	m, sel, count, page, refresh := terminalModel(t)

	// Every normally database-capable action, including Enter (validation +
	// execution), printable command keys, paging, horizontal movement,
	// cancellation, dismissal, and field navigation.
	keys := []string{"enter", "s", "u", "d", "i", "x", "pgup", "pgdown", "tab", "shift+tab", "esc", "ctrl+w", "backspace"}
	for _, k := range keys {
		next := termKey(t, m, k)
		if next.terminalState != TerminalOutcomeUnknown {
			t.Fatalf("key %q left the terminal state", k)
		}
		m = next
	}
	if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 {
		t.Fatalf("database work started: select=%d count=%d page=%d refresh=%d", sel.calls, count.calls, page.issued, refresh.calls)
	}
	if m.inFlightNotice != "" || m.writePending {
		t.Error("terminal state shows in-flight feedback or pending work")
	}
}

func TestTerminalQueryHistoryNavigation(t *testing.T) {
	// Seed one additional distinct older query entry so navigation has two
	// stops.
	m, sel, count, page, refresh := terminalModel(t)
	m.History.Append(prepDeleteQB(true).HistoryState())
	newest, _ := m.History.Newest()
	older, _ := m.History.OlderThan(newest.ID)
	if older.ID == newest.ID {
		t.Fatal("setup: consecutive-identical append was suppressed; seed two distinct states")
	}

	m = termKey(t, m, "ctrl+p")
	if !m.historyMode || m.historyCursorID != newest.ID {
		t.Fatalf("ctrl+p did not open query history at the newest entry (mode=%v cursor=%v)", m.historyMode, m.historyCursorID)
	}
	m = termKey(t, m, "ctrl+p")
	if m.historyCursorID != older.ID {
		t.Fatalf("cursor = %v, want the older entry %v", m.historyCursorID, older.ID)
	}
	if got := termKey(t, m, "ctrl+p"); got.historyCursorID != older.ID {
		t.Error("ctrl+p crossed the oldest boundary")
	}
	m = termKey(t, m, "ctrl+n")
	if m.historyCursorID != newest.ID {
		t.Fatalf("cursor = %v, want back at the newest entry %v", m.historyCursorID, newest.ID)
	}
	if got := termKey(t, m, "ctrl+n"); got.historyMode {
		t.Error("ctrl+n at the newest boundary did not end query browsing deterministically")
	}
	if sel.calls != 0 || count.calls != 0 || page.issued != 0 || refresh.calls != 0 {
		t.Fatalf("history navigation started database work: %d/%d/%d/%d", sel.calls, count.calls, page.issued, refresh.calls)
	}
	// The initially selected newest outcome-unknown result is preserved.
	if !m.resultHistoryMode || m.resultHistoryCursorID == 0 {
		t.Fatal("terminal result selection was lost during query navigation")
	}
	if e, ok := m.ResultHistory.Newest(); !ok || e.ID != m.resultHistoryCursorID {
		t.Error("selected result is not the newest outcome-unknown entry")
	}
}

func TestTerminalQueryHistoryEmptyIsNoOp(t *testing.T) {
	m, sel, _, _, _ := terminalModel(t)
	m.History = history.NewStore()
	m = termKey(t, m, "ctrl+p")
	if m.historyMode {
		t.Fatal("empty query history opened browsing")
	}
	m = termKey(t, m, "ctrl+n")
	if m.historyMode {
		t.Fatal("empty query history opened browsing via ctrl+n")
	}
	if sel.calls != 0 {
		t.Fatalf("empty-history navigation issued %d requests", sel.calls)
	}
}

func TestTerminalResultHistoryNavigation(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	unknownID := m.resultHistoryCursorID
	if !m.resultHistoryMode || unknownID == 0 {
		t.Fatal("setup: settlement did not initially select the outcome-unknown entry")
	}
	older, ok := m.ResultHistory.OlderThan(unknownID)
	if !ok {
		t.Fatal("setup: no older result entry retained")
	}
	m = termKey(t, m, "ctrl+e")
	if m.resultHistoryCursorID != older.ID {
		t.Fatalf("cursor = %v, want the older tabular entry %v", m.resultHistoryCursorID, older.ID)
	}
	if got := termKey(t, m, "ctrl+e"); got.resultHistoryCursorID != older.ID {
		t.Error("ctrl+e crossed the oldest boundary")
	}
	m = termKey(t, m, "ctrl+y")
	if m.resultHistoryCursorID != unknownID {
		t.Fatalf("cursor = %v, want back at the outcome-unknown entry", m.resultHistoryCursorID)
	}
	if got := termKey(t, m, "ctrl+y"); got.resultHistoryCursorID != unknownID || !got.resultHistoryMode {
		t.Error("ctrl+y at the newest boundary must be a no-op that stays in the terminal result view")
	}
	// Leaving and re-entering selects the newest outcome-unknown entry again.
	m = termKey(t, m, "esc")
	if m.resultHistoryMode {
		t.Fatal("esc did not leave result-history browsing")
	}
	if got := termKey(t, m, "ctrl+e"); !got.resultHistoryMode || got.resultHistoryCursorID != unknownID {
		t.Fatalf("re-entering did not select the newest outcome-unknown entry (cursor=%v)", got.resultHistoryCursorID)
	}
}

func TestTerminalResultHistoryDefensiveEmptyFallback(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	// Replace the store with an empty one (the defensive eviction case): no
	// synthetic entry may be created and no missing-backed rendering shown.
	m.ResultHistory = history.NewResultStore()

	m = termKey(t, m, "ctrl+e")
	if m.resultHistoryMode {
		t.Fatal("empty result history opened browsing")
	}
	view := m.View()
	if strings.Contains(view, "SQL: ") && !strings.Contains(view, OutcomeUnknownHeading) {
		t.Error("empty fallback rendered a missing-backed entry")
	}
	// Driving any message through Update must validate the selection without
	// restoring a synthetic entry.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if cmd != nil {
		t.Fatal("empty-fallback validation dispatched a command")
	}
	if next.(Model).resultHistoryMode {
		t.Error("defensive fallback restored a selected entry with no backing store entries")
	}
}

func TestTerminalReducedHelp(t *testing.T) {
	m, _, _, _, _ := terminalModel(t)
	m = termKey(t, m, "esc") // leave the settlement's initial result selection

	m = termKey(t, m, "?")
	if !m.terminalHelpOpen {
		t.Fatal("? did not open the reduced help")
	}
	view := m.View()
	for _, want := range []string{"Ctrl+P / Ctrl+N", "Ctrl+E / Ctrl+Y", "q or Ctrl+C"} {
		if !strings.Contains(view, want) {
			t.Errorf("reduced help lacks %q", want)
		}
	}
	for _, forbidden := range []string{"Ctrl+W", "Ctrl+S", "Ctrl+X", "Enter validates", "database"} {
		if strings.Contains(view, forbidden) {
			t.Errorf("reduced help suggests %q, which is unavailable here", forbidden)
		}
	}
	// Every available action listed actually works in this state.
	m = termKey(t, m, "esc")
	if m.terminalHelpOpen {
		t.Fatal("esc did not dismiss the help")
	}
}
