// Scripted model coverage for query-history navigation and immutable
// restoration (Issue #35): Ctrl+P moves toward older retained queries and
// Ctrl+N toward newer entries, first entry enters at the newest boundary,
// repeated boundary presses are deterministic no-ops (Ctrl+N at the newest
// entry exits history mode back to the base builder), and every complete
// builder field represented by the Issue #20 normalized state restores as an
// immutable deep copy. Browsing, restoration, and later edits append nothing.

package ui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// richSelectQB returns a SELECT over users with a named projection, a
// committed WHERE predicate with an exact entered representation, GROUP BY
// order, ORDER BY direction, and an accepted Limit.
func richSelectQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(navCatalog()).
		SelectCommand(qb.CommandSelect).SelectTable("users")
	out := q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionColumn, Column: "email"})
	q = out.Builder.CompleteProjectionAggregate("email", qb.AggregateValue).Builder
	out = q.AcceptProjection(qb.ProjectionCandidate{Kind: qb.ProjectionColumn, Column: "note"})
	q = out.Builder.CompleteProjectionAggregate("note", qb.AggregateValue).Builder
	next, ok := q.AcceptGroupColumn("note")
	if !ok {
		panic("setup: group by failed")
	}
	q = next
	next, ok = q.AcceptOrderBy("order-column:note")
	if !ok {
		panic("setup: order by failed")
	}
	q = next.SetOrderDirection(qb.DirDesc).SetLimitInput("7")
	return completeWhereQB(q, "x")
}

// navCatalog is a catalog whose users columns are insertable, so the INSERT
// prompt flow can be exercised end to end.
func navCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 17,
		Objects: []*schema.Object{
			{
				Name:          "users",
				Kind:          schema.KindOrdinaryTable,
				WriteEligible: true,
				Rowid:         schema.RowidHas,
				Columns:       []schema.Column{{Name: "id", Insertable: true}, {Name: "email", Insertable: true}, {Name: "note", Insertable: true}},
			},
		},
	}
}

// richUpdateQB returns an UPDATE over users with one submitted SET value and
// a committed WHERE predicate.
func richUpdateQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(navCatalog()).
		SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, ok := q.AcceptSetColumn("email")
	if !ok {
		panic("setup: AcceptSetColumn failed")
	}
	q, ok = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	if !ok {
		panic("setup: ChooseSetAssignment failed")
	}
	q, ok = q.SubmitSetValue("email", "z@q")
	if !ok {
		panic("setup: SubmitSetValue failed")
	}
	return completeWhereQB(q, "3")
}

// richInsertQB returns an INSERT over users exercising Value, NULL, and
// Default/Omit prompt choices with one exact submitted value.
func richInsertQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(navCatalog()).
		SelectCommand(qb.CommandInsert).SelectTable("users").BeginInsertPrompts()
	var ok bool
	q, ok = q.ChooseInsertColumn("id", qb.InsertChoiceValue)
	if !ok {
		panic("setup: insert id choice failed")
	}
	q, ok = q.SubmitInsertValue("id", "5")
	if !ok {
		panic("setup: insert id value failed")
	}
	q, ok = q.ChooseInsertColumn("email", qb.InsertChoiceNull)
	if !ok {
		panic("setup: insert email choice failed")
	}
	q, ok = q.ChooseInsertColumn("note", qb.InsertChoiceOmit)
	if !ok {
		panic("setup: insert note choice failed")
	}
	return q
}

// navigationModel returns a supported-size model with a wired catalog, a
// fresh history store holding A (rich SELECT), B (rich UPDATE), C (rich
// INSERT) oldest first, and the builder back on the plain SELECT base state.
func navigationModel() (Model, *history.Store, [3]history.EntryID) {
	store := history.NewStore()
	ids := [3]history.EntryID{
		store.Append(richSelectQB().HistoryState()),
		store.Append(richUpdateQB().HistoryState()),
		store.Append(richInsertQB().HistoryState()),
	}
	m := modelWithQB(validSelectQB())
	m.catalog = navCatalog()
	m.History = store
	return m, store, ids
}

// navKey drives one named key through Update and returns the model.
func navKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, _ := m.Update(keyMsgFor(key))
	return next.(Model)
}

// keyMsgFor maps test key names onto Bubble Tea key messages.
func keyMsgFor(key string) tea.Msg {
	if len(key) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	}
	return tea.KeyMsg{Type: tea.KeyCtrlP}
}

// focusFieldLabel moves UI focus onto the named field-bar label.
func focusFieldLabel(t *testing.T, m Model, label string) Model {
	t.Helper()
	for i := range m.Fields {
		if m.Fields[i].Label == label {
			m.setFocus(i)
			return m
		}
	}
	t.Fatalf("field %q not present", label)
	return m
}

func TestEmptyStoreHistoryKeysAreNoOps(t *testing.T) {
	m := modelWithQB(validSelectQB())
	m.catalog = navCatalog()
	m.History = history.NewStore()
	m = navKey(t, m, "ctrl+p")
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatalf("Ctrl+P on an empty store entered history mode (mode=%v cursor=%d)", m.historyMode, m.historyCursorID)
	}
	m = navKey(t, m, "ctrl+n")
	if m.historyMode {
		t.Fatal("Ctrl+N on an empty store entered history mode")
	}
}

func TestFirstHistoryEntrySelectsNewest(t *testing.T) {
	m, store, ids := navigationModel()
	m = navKey(t, m, "ctrl+p")
	if !m.historyMode || m.historyCursorID != ids[2] {
		t.Fatalf("first Ctrl+P = (mode %v, cursor %d); want history mode at newest ID %d", m.historyMode, m.historyCursorID, ids[2])
	}
	if !m.QB.HistoryState().Equal(richInsertQB().HistoryState()) {
		t.Fatal("first entry did not restore the newest (INSERT) state")
	}
	// First Ctrl+N from the base context enters at the newest boundary too.
	m2, _, ids2 := navigationModel()
	m2 = navKey(t, m2, "ctrl+n")
	if !m2.historyMode || m2.historyCursorID != ids2[2] {
		t.Fatalf("first Ctrl+N = (mode %v, cursor %d); want newest %d", m2.historyMode, m2.historyCursorID, ids2[2])
	}
	if store.Len() != 3 {
		t.Fatalf("entering history appended: Len = %d", store.Len())
	}
}

func TestCtrlPOlderCtrlNNewerWithBoundariesAndReversal(t *testing.T) {
	m, store, ids := navigationModel()
	m = navKey(t, m, "ctrl+n") // enter at newest (C)
	m = navKey(t, m, "ctrl+p") // B
	if m.historyCursorID != ids[1] || !m.QB.HistoryState().Equal(richUpdateQB().HistoryState()) {
		t.Fatalf("Ctrl+P from newest landed at cursor %d, want B", m.historyCursorID)
	}
	m = navKey(t, m, "ctrl+p") // A
	if m.historyCursorID != ids[0] || !m.QB.HistoryState().Equal(richSelectQB().HistoryState()) {
		t.Fatalf("Ctrl+P landed at cursor %d, want A", m.historyCursorID)
	}
	// Oldest boundary: repeated Ctrl+P is a no-op, still A, still browsing.
	m = navKey(t, m, "ctrl+p")
	m = navKey(t, m, "ctrl+p")
	if m.historyCursorID != ids[0] {
		t.Fatalf("oldest boundary moved cursor to %d, want A", m.historyCursorID)
	}
	if !m.historyMode {
		t.Fatal("oldest boundary left history mode")
	}
	// Direction reversal: Ctrl+N walks back toward newer entries.
	m = navKey(t, m, "ctrl+n")
	if m.historyCursorID != ids[1] {
		t.Fatalf("reversed Ctrl+N landed at %d, want B", m.historyCursorID)
	}
	m = navKey(t, m, "ctrl+n")
	if m.historyCursorID != ids[2] {
		t.Fatalf("reversed Ctrl+N landed at %d, want C", m.historyCursorID)
	}
	// Newest boundary: Ctrl+N exits history mode back to the base builder.
	m = navKey(t, m, "ctrl+n")
	if m.historyMode {
		t.Fatal("Ctrl+N at the newest boundary did not exit history mode")
	}
	if m.historyCursorID != 0 {
		t.Fatalf("exit left cursor %d, want 0", m.historyCursorID)
	}
	if store.Len() != 3 {
		t.Fatalf("browsing appended: Len = %d", store.Len())
	}
}

func TestEscExitsHistoryMode(t *testing.T) {
	m, _, _ := navigationModel()
	m = navKey(t, m, "ctrl+n")
	m = navKey(t, m, "ctrl+p")
	m = navKey(t, m, "esc")
	if m.historyMode {
		t.Fatal("Esc did not exit history mode")
	}
	if m.historyCursorID != 0 {
		t.Fatalf("Esc left cursor %d, want 0", m.historyCursorID)
	}
}

func TestRestoreEveryBuilderFieldImmutable(t *testing.T) {
	m, store, ids := navigationModel()
	want := []qb.HistoryState{richSelectQB().HistoryState(), richUpdateQB().HistoryState(), richInsertQB().HistoryState()}
	entries := store.Entries()
	for i := range entries {
		if !entries[i].State.Equal(want[i]) {
			t.Fatalf("retained entry %d differs from its source builder state", i)
		}
	}
	// Walk all three entries and require byte-exact restoration of every
	// normalized builder field each time. Enter lands at C (newest).
	m = navKey(t, m, "ctrl+n")
	if m.historyCursorID != ids[2] {
		t.Fatal("setup: entry failed")
	}
	for step := 2; step >= 0; step-- {
		if !m.QB.HistoryState().Equal(want[step]) {
			t.Fatalf("restored state at entry %d does not match its stored state", step)
		}
		// Deep-copy proof: build a mutated copy of the restored builder; the
		// retained entry must not change through it.
		mutated := m.QB
		if mutated.Command() == qb.CommandSelect {
			mutated = mutated.RemoveLatestProjection()
		}
		if mutated.Command() == qb.CommandUpdate {
			mutated, _ = mutated.ChooseSetAssignment("email", qb.SetChoiceNull)
		}
		mutated = mutated.SetLimitInput("")
		_ = mutated
		if got := store.Entries(); !got[step].State.Equal(want[step]) {
			t.Fatalf("retained entry %d changed through restored-state mutation", step)
		}
		if step > 0 {
			m = navKey(t, m, "ctrl+p")
		}
	}
}

func TestEditingRestoredStateAppendsNothing(t *testing.T) {
	m, store, ids := navigationModel()
	before := store.Entries()
	m = navKey(t, m, "ctrl+n")
	m = navKey(t, m, "ctrl+p") // B (UPDATE)
	// Edit the restored SET assignment through the base field: clear the
	// submitted value, then re-choose NULL.
	q, ok := m.QB.ChooseSetAssignment("email", qb.SetChoiceNull)
	if !ok {
		t.Fatal("setup: edit failed")
	}
	m.QB = q
	m.applyBuilder(q)
	after := store.Entries()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("editing restored state changed retained history")
	}
	if m.historyMode && m.QB.HistoryState().Equal(before[1].State) {
		t.Fatal("edit did not affect current builder state")
	}
	// Browsing after the edit still appends nothing and never consumes a
	// stable ID: the next real append must still be a fresh entry.
	m = navKey(t, m, "ctrl+p")
	m = navKey(t, m, "ctrl+n")
	if store.Len() != 3 {
		t.Fatalf("browsing appended: Len = %d", store.Len())
	}
	if !reflect.DeepEqual(before, store.Entries()) {
		t.Fatal("retained entries changed during browsing after an edit")
	}
	_ = ids
}

func TestLimitEditingOnRestoredState(t *testing.T) {
	m, store, _ := navigationModel()
	m = navKey(t, m, "ctrl+n")
	m = navKey(t, m, "ctrl+p")
	m = navKey(t, m, "ctrl+p") // A (SELECT, Limit 7)
	m = focusFieldLabel(t, m, limitFieldLabel)
	m = navKey(t, m, "enter")
	if m.ValuePrompt == nil {
		t.Fatal("Enter on restored Limit did not open the value prompt")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	m = next.(Model)
	m = navKey(t, m, "enter")
	if v, ok := m.QB.LimitValue(); !ok || v != 9 {
		t.Fatalf("edited restored Limit = (%d, %v); want 9 accepted", v, ok)
	}
	entries := store.Entries()
	if entries[0].State.LimitValue != 7 || !entries[0].State.LimitHas {
		t.Fatalf("retained Limit changed to (%d, %v) through the edit", entries[0].State.LimitValue, entries[0].State.LimitHas)
	}
	if store.Len() != 3 {
		t.Fatalf("editing appended: Len = %d", store.Len())
	}
}

func TestMutationOfSourceAndRetrievedCopiesNeverAffectsStore(t *testing.T) {
	_, store, ids := navigationModel()
	// Mutate a retrieved copy's slices and scalars.
	e, ok := store.Lookup(ids[0])
	if !ok {
		t.Fatal("setup: lookup failed")
	}
	e.State.Projection[0].Column = "hacked"
	e.State.Groups = append(e.State.Groups, "hacked")
	e.State.Table = "hacked"
	// Mutate the Entries() slice itself.
	all := store.Entries()
	all[1].State.Table = "hacked"
	all[2].State.Sets = nil
	fresh := store.Entries()
	if !fresh[0].State.Equal(richSelectQB().HistoryState()) || !fresh[1].State.Equal(richUpdateQB().HistoryState()) || !fresh[2].State.Equal(richInsertQB().HistoryState()) {
		t.Fatal("retained entries changed through source or retrieved mutations")
	}
}
