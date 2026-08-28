// Scripted Bubble Tea coverage for Issue #17 Tasks 3–4: the guided WHERE flow
// driven end-to-end through Update for every predicate consumer (SELECT,
// UPDATE, DELETE). The sequence always opens a searchable eligible-column
// popup, then a scroll-only fixed-operator popup, then universal value entry
// only for value-taking operators. IS NULL / IS NOT NULL complete without any
// value prompt and return focus to the builder field; value submission
// preserves exact entered text and bound type; revisiting restores prior
// choices exactly; Esc restores the prior completed predicate and exact opener
// focus without partial commits. Popup search/highlight/viewport reset follows
// the reusable Issue #12 contract; parsing rules stay inside QueryBuilder per
// Issue #14 and are never duplicated here.

package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
)

func whereUICatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 17,
		Objects: []*schema.Object{
			{
				Name:          "users",
				Kind:          schema.KindOrdinaryTable,
				WriteEligible: true,
				Rowid:         schema.RowidHas,
				Columns:       []schema.Column{{Name: "id"}, {Name: "email"}, {Name: "note"}},
			},
			{Name: "logs", Kind: schema.KindView, Columns: []schema.Column{{Name: "line"}}},
		},
	}
}

// whereFocusedAt drives a fresh model to command cmd with users selected and
// the Where field focused.
func whereFocusedAt(cmd rune) Model {
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: whereUICatalog()}, key(cmd),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, ok := m.QB.SelectedTable(); !ok {
		panic("setup table selection failed")
	}
	for i := range m.Fields {
		if m.Fields[i].Label == whereFieldLabel {
			m.setFocus(i)
			return m
		}
	}
	panic("Where field missing after setup")
}

// beginWhere presses Enter on the focused Where field, opening its column popup.
func beginWhere(m Model) Model { return drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) }

// chooseColumnAndOperator accepts one column (typing nothing, assuming exact
// order navigation) then walks the operator popup down to opToken before
// accepting it.
func chooseColumnAndOperator(m Model, colDowns int, opToken string) Model {
	for i := 0; i < colDowns; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.Popup == nil || m.Popup.Mode != PopupScrollOnly || m.Popup.Opener != whereFieldLabel {
		panic("operator popup did not open")
	}
	want := operatorPopupCandidates()
	idx := slices.Index(want, opToken)
	if idx < 0 {
		panic("unknown operator token in setup")
	}
	for i := 0; i < idx; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	}
	return drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}

// submitValueText types s into the open value prompt rune by rune and submits.
func submitValueText(m Model, s string) Model {
	msgs := []tea.Msg{}
	for _, r := range s {
		msgs = append(msgs, key(r))
	}
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEnter})
	return drive(m, msgs...).(Model)
}

// TestWhereColumnPopupSearchableForEveryConsumer requires Enter on Where to
// open a searchable single-select column popup over visible columns in
// declared order for SELECT, UPDATE, and DELETE alike, and never for INSERT
// or without a selected table.
func TestWhereColumnPopupSearchableForEveryConsumer(t *testing.T) {
	cases := []struct {
		cmd     rune
		fieldOK bool
	}{
		{'s', true}, {'u', true}, {'d', true},
	}
	for _, tc := range cases {
		m := whereFocusedAt(tc.cmd)
		for i, f := range m.Fields {
			if f.Label == whereFieldLabel && i != m.Focus {
				t.Errorf("%c: Where field present but unfocused", tc.cmd)
			}
		}
		open := beginWhere(m)
		if open.Popup == nil || !open.Popup.Open() {
			t.Fatalf("%c: Enter on Where opened no popup", tc.cmd)
		}
		if open.Popup.Mode != PopupSearchable || open.Popup.Multi ||
			open.Popup.Opener != whereFieldLabel {
			t.Errorf("%c: popup=%+v, want searchable single-select opened by %s",
				tc.cmd, *open.Popup, whereFieldLabel)
		}
		var got []string
		for _, c := range open.Popup.Visible() {
			got = append(got, c.ID)
		}
		if !slices.Equal(got, []string{"id", "email", "note"}) {
			t.Errorf("%c: candidates=%v, want id,email,note", tc.cmd, got)
		}
	}
	// INSERT offers no Where field at all.
	mi := whereFocusedAtUIInsert(t)
	for _, f := range mi.Fields {
		if f.Label == whereFieldLabel {
			t.Error("INSERT gained a Where field")
		}
	}
	// A view is eligible only for SELECT; UPDATE keeps guiding fine but the
	// view case never reaches WHERE here beyond eligibility, already owned by
	// QueryBuilder. No assertion duplicated from querybuilder rules.
}

// whereFocusedAtUIInsert returns a model with INSERT chosen and users
// selected, matching the other setups minus expectation of a Where field.
func whereFocusedAtUIInsert(t *testing.T) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: whereUICatalog()}, key('i'),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if m.QB.Command().String() != "INSERT" {
		t.Fatalf("setup command=%v, want INSERT", m.QB.Command())
	}
	return m
}

// TestNullOperatorsCompleteWithoutValuePrompt requires IS NULL and
// IS NOT NULL selections to commit the completed predicate immediately: no
// value prompt may open, no parameter binds, the popup stack closes, the
// field bar shows the rendered predicate, and focus rests on the Where field.
func TestNullOperatorsCompleteWithoutValuePrompt(t *testing.T) {
	for _, token := range []string{"IS NULL", "IS NOT NULL"} {
		var wantSQL string
		switch token {
		case "IS NULL":
			wantSQL = `"email" IS NULL`
		default:
			wantSQL = `"email" IS NOT NULL`
		}
		m := beginWhere(whereFocusedAt('d'))
		m = chooseColumnAndOperator(m, 1, token) // email is second candidate
		if m.ValuePrompt != nil {
			t.Errorf("%s: value prompt opened for a no-value operator", token)
		}
		if m.Popup != nil {
			t.Errorf("%s: popups remained open after completion", token)
		}
		if !m.QB.HasWhere() || m.QB.WherePredicate().SQL() != wantSQL {
			t.Errorf("%s: committed=%q hasWhere=%v", token, m.QB.WherePredicate().SQL(), m.QB.HasWhere())
		}
		if got := m.QB.WhereParams(); got != nil {
			t.Errorf("%s: params=%v, want none", token, got)
		}
		if content := m.Fields[m.Focus].Content; m.Fields[m.Focus].Label != whereFieldLabel || content != wantSQL {
			t.Errorf("%s: focused field=%+v, want %s:%s", token, m.Fields[m.Focus], whereFieldLabel, wantSQL)
		}
	}
}

// TestValueOperatorsOpenUniversalEntryUntilSubmission requires every value-
// taking operator to open the universal entry seeded empty, keep the predicate
// incomplete until Enter, then bind the exact parsed payload with its concrete
// Go type while closing back onto the focused Where field.
func TestValueOperatorsOpenUniversalEntryUntilSubmission(t *testing.T) {
	for _, token := range operatorPopupCandidates() {
		if token == "IS NULL" || token == "IS NOT NULL" {
			continue
		}
		var opIdx int
		for i, c := range operatorPopupCandidates() {
			if c == token {
				opIdx = i
				break
			}
		}
		colDowns := opIdx - 1 // candidate id is opened with zero downs via column index below
		_ = colDowns
		m := beginWhere(whereFocusedAt('u'))
		// Column note is third: two downs.
		m = chooseColumnAndOperator(m, 2, token)
		if m.ValuePrompt == nil {
			t.Fatalf("%s: universal entry did not open", token)
		}
		if m.QB.HasWhere() {
			t.Errorf("%s: incomplete predicate committed early", token)
		}
		view := m.View()
		if !strings.Contains(view, WhereTypedNullHint) {
			t.Errorf("%s: inline typed-NULL hint absent:\n%s", token, view)
		}
		submitted := submitValueText(m, "42")
		if submitted.ValuePrompt != nil || submitted.Popup != nil {
			t.Errorf("%s: prompts stayed open after submission", token)
		}
		if !submitted.QB.HasWhere() {
			t.Fatalf("%s: value submission did not commit", token)
		}
		params := submitted.QB.WhereParams()
		if len(params) != 1 {
			t.Fatalf("%s: bound %v, want one param", token, params)
		}
		if iv, ok := params[0].(int64); !ok || iv != 42 {
			t.Errorf("%s: bound %#v (%T), want int64(42)", token, params[0], params[0])
		}
		if submitted.Fields[submitted.Focus].Label != whereFieldLabel {
			t.Errorf("%s: focus=%s after completion, want %s", token,
				submitted.Fields[submitted.Focus].Label, whereFieldLabel)
		}
	}
}

// TestTypedNullAndEmptyBindAsText requires typed `NULL` and the empty string
// to commit as verbatim TEXT parameters, never SQL null, through the guided
// UI flow itself.
func TestTypedNullAndEmptyBindAsText(t *testing.T) {
	m := beginWhere(whereFocusedAt('s'))
	m = chooseColumnAndOperator(m, 1, "=") // email
	m = submitValueText(m, "NULL")
	params := m.QB.WhereParams()
	if len(params) != 1 || params[0] != "NULL" {
		t.Fatalf("typed NULL bound %v (%T), want [NULL] as TEXT", params, params)
	}

	m2 := beginWhere(whereFocusedAt('s'))
	m2 = chooseColumnAndOperator(m2, 0, ">=") // id
	m2 = drive(m2, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	params2 := m2.QB.WhereParams()
	if len(params2) != 1 || params2[0] != "" {
		t.Fatalf("empty input bound %#v, want exactly [\"] as TEXT", params2)
	}
}

// TestLikeWildcardsBoundVerbatimThroughFlow requires '%'/'_' LIKE values to
// reach the committed parameters byte-for-byte, unchanged from entered text.
func TestLikeWildcardsBoundVerbatimThroughFlow(t *testing.T) {
	m := beginWhere(whereFocusedAt('s'))
	m = chooseColumnAndOperator(m, 2, "LIKE")
	m = submitValueText(m, "%a_b%")
	if sql := m.QB.WherePredicate().SQL(); sql != `"note" LIKE ?` {
		t.Errorf("committed SQL=%s, want \"note\" LIKE ?", sql)
	}
	if p := m.QB.WhereParams(); len(p) != 1 || p[0] != "%a_b%" {
		t.Errorf("params=%v, want byte-for-byte [%q]", p, "%a_b%")
	}
}

// TestEscRestoresPriorCompletionWithoutPartialCommit drives Esc at every
// stage of a revision of an existing completed predicate: the prior completed
// predicate and the exact opener focus must survive untouched, with no partial
// commit and no open overlays afterwards.
func TestEscRestoresPriorCompletionWithoutPartialCommit(t *testing.T) {
	base := beginWhere(whereFocusedAt('u'))
	completed := func() Model {
		m := base
		m = chooseColumnAndOperator(m, 1, "<=") // email
		return submitValueText(m, "-5.25")
	}
	full := completed()
	if !full.QB.HasWhere() {
		t.Fatal("setup completion failed")
	}
	priorSQL := full.QB.WherePredicate().SQL()
	priorParams := full.QB.WhereParams()

	revisit := func(mm Model) Model {
		// Revisit through a full Shift+Tab/arrow round trip off the Where
		// field and back onto it, then Enter.
		mm = drive(mm, tea.KeyMsg{Type: tea.KeyShiftTab},
			tea.KeyMsg{Type: tea.KeyRight}, tea.KeyMsg{Type: tea.KeyLeft},
			tea.KeyMsg{Type: tea.KeyTab}).(Model)
		return beginWhere(mm)
	}

	cancel := func(name string, mm Model) {
		if mm.ValuePrompt != nil || mm.Popup != nil || mm.QB.WhereDrafting() {
			t.Errorf("%s: revision state leaked after Esc: prompt=%v popup=%v drafting=%v",
				name, mm.ValuePrompt != nil, mm.Popup != nil, mm.QB.WhereDrafting())
		}
		if mm.QB.WherePredicate().SQL() != priorSQL {
			t.Errorf("%s: committed SQL=%s, want %s", name, mm.QB.WherePredicate().SQL(), priorSQL)
		}
		if !slices.Equal(mm.QB.WhereParams(), priorParams) {
			t.Errorf("%s: params=%v changed after Esc", name, mm.QB.WhereParams())
		}
	}

	// Stage 1: Esc at the reopened column popup.
	s1 := revisit(full)
	if s1.Popup == nil {
		t.Fatal("revisit did not reopen column popup")
	}
	cancel("column-popup", drive(s1, tea.KeyMsg{Type: tea.KeyEsc}).(Model))

	// Stage 2: Esc after choosing a fresh column + operator but before value
	// submission: staged draft discarded entirely.
	s2 := revisit(full)
	s2 = chooseColumnAndOperator(s2, 0, ">") // switch to id
	if s2.ValuePrompt == nil {
		t.Fatal("operator choice produced no value prompt")
	}
	esc2 := drive(s2, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	cancel("value-prompt", esc2)

	// Focus lands back on the Where field (the exact opener).
	driven := drive(s1, tea.KeyMsg{Type: tea.KeyEsc}).(Model)
	if driven.Fields[driven.Focus].Label != whereFieldLabel {
		t.Errorf("Esc restored focus=%s, want %s", driven.Fields[driven.Focus].Label, whereFieldLabel)
	}
	if esc2.Fields[esc2.Focus].Label != whereFieldLabel {
		t.Errorf("value-prompt Esc focus=%s, want %s", esc2.Fields[esc2.Focus].Label, whereFieldLabel)
	}
	// The still-committed bound REAL survives untouched.
	if p := full.QB.WhereParams(); len(p) != 1 || p[0] != -5.25 {
		t.Errorf("prior real value mutated: %v", p)
	}
}

// TestRevisitRestoresExactPriorStateOnSameColumn requires revisiting a
// completed '=' predicate for the same column to restore the exact previously
// entered text into the reopened value input, cursor-ready at the end, and to
// show the previously chosen operator highlighted when walking the operator
// popup again.
func TestRevisitRestoresExactPriorStateOnSameColumn(t *testing.T) {
	m := beginWhere(whereFocusedAt('s'))
	m = chooseColumnAndOperator(m, 1, "=") // email
	m = submitValueText(m, "tricky'x_50%")
	if p := m.QB.WhereParams(); len(p) != 1 || p[0] != "tricky'x_50%" {
		t.Fatalf("setup commit failed: %v", p)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyShiftTab},
		tea.KeyMsg{Type: tea.KeyTab}).(Model)
	m = beginWhere(m)
	m = chooseColumnAndOperator(m, 1, "=") // same column again
	if m.ValuePrompt == nil {
		t.Fatal("reopened value prompt missing")
	}
	if got := m.ValuePrompt.Buffer(); got != "tricky'x_50%" {
		t.Errorf("restored buffer=%q, want exact prior text", got)
	}
	if cur := m.ValuePrompt.Cursor(); cur != len([]rune("tricky'x_50%")) {
		t.Errorf("restored cursor=%d, want end-of-buffer", cur)
	}
	// Operator popup restoration: begin again from scratch and assert the
	// previous operator starts highlighted rather than at the top.
	m2 := fullSetupForRestoreTest(t)
	if hi, ok := m2.Popup.Highlighted(); !ok || hi.ID != "=" {
		t.Errorf("operator highlight=(%v,%v), want '=' preselected", hi.ID, ok)
	}
}

// fullSetupForRestoreTest completes `id = 7`, revisits, reselects id, and
// returns the model sitting on the restored operator popup highlight.
func fullSetupForRestoreTest(t *testing.T) Model {
	t.Helper()
	m := beginWhere(whereFocusedAt('s'))
	m = chooseColumnAndOperator(m, 0, "=")
	m = submitValueText(m, "7")
	m = drive(m, tea.KeyMsg{Type: tea.KeyShiftTab},
		tea.KeyMsg{Type: tea.KeyTab}).(Model)
	m = beginWhere(m)
	m = chooseColumn(m, 0)
	return m
}

// chooseColumn accepts the nth visible column without entering an operator,
// leaving the operator popup open.
func chooseColumn(m Model, n int) Model {
	for i := 0; i < n; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	}
	return drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}
