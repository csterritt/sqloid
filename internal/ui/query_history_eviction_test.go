// Scripted model coverage for query-history execution exit and defensive
// selected-ID eviction (Issue #35): an actual execution start exits history
// mode before the unchanged Issue #20 append seam runs and executes the
// current restored-and-possibly-edited builder state; an externally evicted
// selection resolves to the new oldest retained entry with the exact
// eviction notice, or returns to the base builder on an empty store. Normal
// execution can never leave the selection pointing at an evicted entry, and
// a missing backing entry is never rendered, restored, or executed through.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// externalEvictingAppend simulates an externally driven store append that
// evicts the currently selected (oldest) entry: it fills the store to
// capacity and appends one more distinct state.
func externalEvictingAppend(t *testing.T, store *history.Store, label string) {
	t.Helper()
	for i := 0; store.Len() < history.Capacity; i++ {
		store.Append(cursorLabelState(label+"f", i))
	}
	store.Append(cursorLabelState(label, 9999))
}

// cursorLabelState builds a distinct normalized SELECT-over-users state per
// salt so every filler entry restores against the test catalog.
func cursorLabelState(label string, salt int) qb.HistoryState {
	state := qb.HistoryState{Command: qb.CommandSelect, Table: label, TableSet: true}
	state.Projection = []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}}
	state.LimitHas = salt != 0
	state.LimitValue = int64(salt)
	return state
}

// TestExecutionStartExitsHistoryAndAppendsCurrentState requires an execution
// start while a history entry is selected to exit history mode first and to
// append the current restored builder state through the unchanged seam.
func TestExecutionStartExitsHistoryAndAppendsCurrentState(t *testing.T) {
	m, store, _ := navigationModel()
	m, store, ids := navigationModel()
	m = navKey(t, m, "ctrl+n") // newest C (INSERT) restored
	// Edit the restored state so it is no longer consecutive-identical to
	// its backing entry: switch the id prompt to Default/Omit.
	q, ok := m.QB.ChooseInsertColumn("id", qb.InsertChoiceOmit)
	if !ok {
		t.Fatal("setup: edit failed")
	}
	m.QB = q
	m.applyBuilder(q)
	m = asModel(m.Update(ExecutionStartedMsg{}))
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatalf("execution start did not exit history mode (mode=%v cursor=%d)", m.historyMode, m.historyCursorID)
	}
	if store.Len() != 4 {
		t.Fatalf("history Len = %d after executing the restored INSERT state, want 4", store.Len())
	}
	entries := store.Entries()
	if entries[3].State.Command.String() != "INSERT" || entries[3].State.Table != "users" {
		t.Fatalf("appended state = %v over %q; want INSERT over users", entries[3].State.Command, entries[3].State.Table)
	}
	if !entries[3].State.Equal(m.QB.HistoryState()) {
		t.Fatal("execution did not run against the current restored state")
	}
	if entries[3].ID == ids[0] || entries[3].ID == ids[1] || entries[3].ID == ids[2] {
		t.Fatal("appended entry reused a retained stable ID")
	}
	// The retained C backing entry is untouched by the executed edit.
	backing, ok := store.Lookup(ids[2])
	if !ok || !backing.State.Equal(richInsertQB().HistoryState()) {
		t.Fatal("retained backing entry changed during execution")
	}
}

// TestExecutionStartAppendsEditedRestoredState requires the appended state to
// be the current edited builder state, not a fresh lookup or stale backing
// entry, and to obey the unchanged consecutive-suppression policy.
func TestExecutionStartAppendsEditedRestoredState(t *testing.T) {
	m, store, _ := navigationModel()
	m = navKey(t, m, "ctrl+n")
	m = navKey(t, m, "ctrl+p") // B (UPDATE)
	m = navKey(t, m, "ctrl+p") // A (SELECT)
	// Edit the restored state: drop the Limit entirely.
	q := m.QB.SetLimitInput("")
	m.QB = q
	m.applyBuilder(q)
	m = asModel(m.Update(ExecutionStartedMsg{}))
	// The edited SELECT state is consecutive-identical to the retained A
	// entry only in its normalized fields minus the Limit — here the Limit
	// differs (A has 7), so a new entry must append with the edited value.
	entries := store.Entries()
	if store.Len() != 4 {
		t.Fatalf("history Len = %d after executing the edited state, want 4", store.Len())
	}
	last := entries[3].State
	if last.LimitHas {
		t.Fatalf("appended edited state carries Limit %d; want the edited unbounded state", last.LimitValue)
	}
	if !last.Equal(m.QB.HistoryState()) {
		t.Fatal("appended state is not the current edited builder state")
	}
}

// TestExecutionStartSuppressionRegression proves the unchanged Issue #20
// A→A suppression still applies across a history-mode execution: executing
// the exact restored state of the immediately preceding entry appends
// nothing and consumes no stable ID.
func TestExecutionStartSuppressionRegression(t *testing.T) {
	store := history.NewStore()
	store.Append(validSelectQB().HistoryState())
	m := modelWithQB(validSelectQB())
	m.catalog = navCatalog()
	m.History = store
	m = navKey(t, m, "ctrl+p") // enter history mode, restoring the newest state
	if m.historyCursorID == 0 {
		t.Fatal("setup: history mode did not open")
	}
	m = asModel(m.Update(ExecutionStartedMsg{}))
	if store.Len() != 1 {
		t.Fatalf("A→A across history execution: Len = %d, want 1 (suppressed)", store.Len())
	}
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatal("execution start did not detach the history cursor")
	}
}

// TestEvictedSelectionFallsBackToNewOldest simulates an externally driven
// append that evicts the selected stable ID while history mode is open.
func TestEvictedSelectionFallsBackToNewOldest(t *testing.T) {
	m, store, ids := navigationModel()
	// Fill so the three viewed entries sit at the oldest end of the list.
	for i := 0; store.Len() < history.Capacity-1; i++ {
		store.Append(cursorLabelState("users", i))
	}
	m = navKey(t, m, "ctrl+n")
	for i := 0; m.historyCursorID != ids[0]; i++ {
		if i > history.Capacity {
			t.Fatal("setup: Ctrl+P walk never reached the oldest entry")
		}
		m = navKey(t, m, "ctrl+p")
	}
	// Externally drive appends that evict A (the selected oldest entry):
	// one fills the store to Capacity, the next evicts the oldest.
	store.Append(cursorLabelState("users", 9998))
	store.Append(cursorLabelState("users", 9999))
	if _, ok := store.Lookup(ids[0]); ok {
		t.Fatal("setup: selected oldest entry was not evicted")
	}
	// Any message delivery must resolve the selection defensively.
	m = asModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if !m.historyMode {
		t.Fatal("eviction with surviving entries must stay in history mode on the new oldest")
	}
	if m.historyNotice != "Previously viewed query was evicted from history" {
		t.Fatalf("notice = %q; want the exact eviction notice", m.historyNotice)
	}
	oldest, ok := store.Oldest()
	if !ok {
		t.Fatal("setup: no entries retained")
	}
	if m.historyCursorID != oldest.ID {
		t.Fatalf("cursor = %d; want the new oldest ID %d", m.historyCursorID, oldest.ID)
	}
	if !m.QB.HistoryState().Equal(oldest.State) {
		t.Fatal("fallback did not restore a copy of the new oldest entry")
	}
	// Surviving stable IDs remain unchanged (ids[0] was evicted).
	for _, id := range ids[1:] {
		if _, ok := store.Lookup(id); !ok {
			t.Fatalf("surviving ID %d vanished", id)
		}
	}
	if backing, ok := store.Lookup(m.historyCursorID); !ok || !m.QB.HistoryState().Equal(backing.State) {
		t.Fatal("rendered builder state does not match its retained backing entry")
	}
}

// TestEvictedSelectionEmptyStoreReturnsToBase simulates an external store
// replacement that empties history while an entry is selected.
func TestEvictedSelectionEmptyStoreReturnsToBase(t *testing.T) {
	m, store, _ := navigationModel()
	m = navKey(t, m, "ctrl+n")
	if !m.historyMode {
		t.Fatal("setup: history mode did not open")
	}
	// Replace the store wholesale: the selected stable ID has no backing
	// entry anywhere and nothing is retained.
	viewed, ok := store.Lookup(m.historyCursorID)
	if !ok {
		t.Fatal("setup: selected entry missing")
	}
	m.History = history.NewStore()
	m = asModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if m.historyMode {
		t.Fatal("empty store must exit history mode back to the base builder")
	}
	if m.historyNotice != "Previously viewed query was evicted from history" {
		t.Fatalf("notice = %q; want the exact eviction notice", m.historyNotice)
	}
	if m.historyCursorID != 0 {
		t.Fatalf("cursor = %d after empty-store return, want 0", m.historyCursorID)
	}
	// The builder state remains valid — the last viewed entry's data, not a
	// zeroed snapshot of the vanished backing entry.
	if m.QB.Command() != viewed.State.Command || m.QB.Command().String() != "INSERT" {
		t.Fatalf("builder command = %v after empty-store return", m.QB.Command())
	}
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("builder table vanished after empty-store return")
	}
}

// TestNormalExecutionNeverLeavesEvictedSelection proves that append-caused
// eviction during a real execution start can never strand the selection on a
// missing backing entry: mode is detached before the append seam runs.
func TestNormalExecutionNeverLeavesEvictedSelection(t *testing.T) {
	m, store, _ := navigationModel()
	for i := 0; store.Len() < history.Capacity; i++ {
		store.Append(cursorLabelState("fill", i))
	}
	m = navKey(t, m, "ctrl+n") // history mode open at newest
	m = asModel(m.Update(ExecutionStartedMsg{}))
	if m.historyMode {
		t.Fatal("execution start left history mode open")
	}
	// The append that just happened may have evicted the oldest entry — the
	// formerly selected one cannot be re-resolved by any later defensive
	// check because the cursor is already detached.
	if m.historyCursorID != 0 {
		t.Fatalf("cursor = %d after execution start, want 0", m.historyCursorID)
	}
	if store.Len() > history.Capacity {
		t.Fatalf("store Len = %d exceeded capacity", store.Len())
	}
	m = asModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if m.historyNotice != "" {
		t.Fatalf("unexpected notice %q after a normal execution", m.historyNotice)
	}
}
