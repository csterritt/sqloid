// INSERT prompt-flow UI contract tests, per Issue #39 Tasks 5–6: one shared
// scroll-only choice popup per insertable column in schema order with exactly
// Value, NULL, Default/Omit; the INTEGER PRIMARY KEY omission hint only on
// the applicable column; universal entry only for Value; exact restoration
// of choice, entered representation, bound type, highlight, and focus across
// revision, cancellation, and whole-value clearing; exact history-ready
// state; and zero-insertable-column blocking with the exact message, no
// popup, and no execution or history command. All behavior is scripted
// through Update without sleeps.

package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func insertUICatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 21,
		Objects: []*schema.Object{
			{
				Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: "id", DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "email", DeclaredType: "TEXT", Insertable: true},
					{Name: "note", DeclaredType: "TEXT", Insertable: true},
				},
				InsertableCount: 3,
				PrimaryKeyCount: 1,
			},
			{
				Name: "shadowed", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "gen", DeclaredType: "INTEGER", Hidden: true}},
				InsertableCount: 0,
			},
		},
	}
}

func insertUIModel(choices map[string]qb.InsertChoice, values map[string]string) Model {
	q := qb.NewQuery().RefreshSchema(insertUICatalog()).
		SelectCommand(qb.CommandInsert).SelectTable("users").BeginInsertPrompts()
	for _, c := range q.InsertColumns() {
		choice, ok := choices[c.Column]
		if !ok {
			continue
		}
		next, ok := q.ChooseInsertColumn(c.Column, choice)
		if !ok {
			panic("setup: ChooseInsertColumn failed")
		}
		q = next
		if choice == qb.InsertChoiceValue {
			next, ok := q.SubmitInsertValue(c.Column, values[c.Column])
			if !ok {
				panic("setup: SubmitInsertValue failed")
			}
			q = next
		}
	}
	return focusField(modelWithQB(q), insertFieldLabel)
}

func insertModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("Update(%T) returned unexpected command", msg)
	}
	return next.(Model)
}

func insertPopupCandidates(t *testing.T, m Model) []PopupCandidate {
	t.Helper()
	if m.Popup == nil || m.Popup.Multi || m.Popup.Mode != PopupScrollOnly {
		t.Fatalf("expected the shared scroll-only choice popup, got %#v", m.Popup)
	}
	return m.Popup.Visible()
}

func highlightedID(t *testing.T, m Model) string {
	t.Helper()
	if m.Popup == nil {
		t.Fatal("no popup open")
	}
	if c, ok := m.Popup.Highlighted(); ok {
		return c.ID
	}
	return ""
}

func wantInsertChoices(t *testing.T, m Model, column string) {
	t.Helper()
	got := insertPopupCandidates(t, m)
	want := []PopupCandidate{
		{ID: "Value", Display: "Value"},
		{ID: "NULL", Display: "NULL"},
		{ID: "Default/Omit", Display: "Default/Omit"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("choices for %q = %#v, want exactly %#v", column, got, want)
	}
}

// TestInsertPromptFlowChoicesHintAndCompletion scripts the whole flow: Enter
// on the Insert field opens the first column's choice popup showing the exact
// three choices and the INTEGER PRIMARY KEY hint only for id; mixed choices
// complete every column in schema order with universal entry only for Value;
// completion returns focus to the Insert field with the builder runnable.
func TestInsertPromptFlowChoicesHintAndCompletion(t *testing.T) {
	m := insertUIModel(map[string]qb.InsertChoice{}, map[string]string{})

	// First prompt: id, with the exact omission hint in the popup status.
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	wantInsertChoices(t, m, "id")
	if view := m.View(); !strings.Contains(view, "Column: id "+schema.InsertOmissionHint) {
		t.Fatalf("id prompt view missing exact hint:\n%s", view)
	}

	// Value opens universal entry; submit empty text (empty TEXT).
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt == nil {
		t.Fatal("Value choice did not open universal entry")
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := m.QB.InsertColumns()[0].SubmittedValue(); !ok {
		t.Fatalf("empty submission did not complete %q as empty TEXT", m.QB.InsertColumns()[0].Column)
	}

	// Second prompt: email, without the hint; explicit NULL completes it
	// immediately with no text entry.
	wantInsertChoices(t, m, "email")
	if view := m.View(); strings.Contains(view, "Column: email "+schema.InsertOmissionHint) {
		t.Fatalf("email prompt wrongly carries the omission hint:\n%s", view)
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyDown}) // NULL
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt != nil {
		t.Fatal("NULL choice opened universal entry")
	}

	// Third prompt: note, Default/Omit completes immediately.
	wantInsertChoices(t, m, "note")
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.Popup != nil || m.ValuePrompt != nil {
		t.Fatal("completed flow left an overlay open")
	}
	if got := m.Fields[m.Focus].Label; got != insertFieldLabel {
		t.Fatalf("completed flow focus = %q, want %q", got, insertFieldLabel)
	}
	cols := m.QB.InsertColumns()
	if len(cols) != 3 || cols[0].Choice() != qb.InsertChoiceValue || cols[1].Choice() != qb.InsertChoiceNull || cols[2].Choice() != qb.InsertChoiceOmit {
		t.Fatalf("final choices = %#v", cols)
	}
	if !m.QB.RunnableReport().Runnable {
		t.Fatalf("mixed complete INSERT not runnable: %q", m.QB.RunnableReport().Reason)
	}
	if params := m.QB.InsertParams(); !reflect.DeepEqual(params, []any{""}) {
		t.Fatalf("InsertParams() = %#v, want one empty TEXT", params)
	}
}

// TestInsertPromptRevisionRestoresEverythingExact revisits every prompt after
// completion and requires exact restoration of choice, original entered text
// including emptiness and whitespace, parsed bound type and value, popup
// highlight, and opener focus — without reparsing.
func TestInsertPromptRevisionRestoresEverythingExact(t *testing.T) {
	m := insertUIModel(
		map[string]qb.InsertChoice{"id": qb.InsertChoiceValue, "email": qb.InsertChoiceValue, "note": qb.InsertChoiceNull},
		map[string]string{"id": "42", "email": "  padded\t"},
	)

	// Reopen the first prompt: the Value choice is pre-highlighted.
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := insertPopupCandidates(t, m); highlightedID(t, m) != "Value" {
		t.Fatalf("reopened highlight = %q, want Value with rows %#v", highlightedID(t, m), got)
	}

	// Enter re-opens universal entry seeded with the exact entered bytes.
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ValuePrompt == nil {
		t.Fatal("revision did not reopen universal entry")
	}
	if got := m.ValuePrompt.Buffer(); got != "42" {
		t.Fatalf("restored buffer = %q, want %q", got, "42")
	}
	if got := m.ValuePrompt.Cursor(); got != len("42") {
		t.Fatalf("restored cursor = %d, want %d", got, len("42"))
	}
	// Esc cancels: the prior submission survives untouched.
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if v, ok := m.QB.InsertColumns()[0].SubmittedValue(); !ok || v.Kind != qb.KindInteger || v.Int != 42 {
		t.Fatalf("Esc mutated the committed id submission: (%#v, %v)", v, ok)
	}
	if entered, ok := m.QB.InsertColumns()[0].Entered(); !ok || entered != "42" {
		t.Fatalf("Esc mutated the entered representation: (%q, %v)", entered, ok)
	}

	// Email's entered whitespace is restored byte-for-byte; reopen the id
	// prompt and Tab to email, then revise to a different text.
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // reopen id choice
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyTab})   // email choice
	if id := highlightedID(t, m); id != "Value" {
		t.Fatalf("email highlight = %q, want Value", id)
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.ValuePrompt.Buffer(); got != "  padded\t" {
		t.Fatalf("email buffer = %q, want %q", got, "  padded\t")
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // reopen email choice
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // accept Value, reopen entry
	for range len("  padded\t") {                         // clear the restored revision seed first
		m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("NULL")})
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if v, ok := m.QB.InsertColumns()[1].SubmittedValue(); !ok || v.Kind != qb.KindText || v.Text != "NULL" {
		t.Fatalf("email resubmission = (%#v, %v), want TEXT NULL", v, ok)
	}

	// Note's NULL choice pre-highlights on revision: submitting email
	// advances directly to the note choice popup.
	if id := highlightedID(t, m); id != "NULL" {
		t.Fatalf("note highlight = %q, want NULL", id)
	}
	if v, ok := m.QB.InsertColumns()[2].SubmittedValue(); ok {
		t.Fatalf("NULL choice reported a submission: %#v", v)
	}
	if entered, ok := m.QB.InsertColumns()[0].Entered(); !ok || entered != "42" {
		t.Fatalf("revisions corrupted earlier prompts: (%q, %v)", entered, ok)
	}
}

// TestInsertWholeValueClearingKeepsChoice proves Backspace and Delete on a
// completed Value prompt clear the exact text, parsed bound type/value, and
// submission atomically while preserving the Value choice and column
// identity, keeping NULL and omission structurally distinct.
func TestInsertWholeValueClearingKeepsChoice(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyBackspace, tea.KeyDelete} {
		m := insertUIModel(
			map[string]qb.InsertChoice{"id": qb.InsertChoiceValue, "email": qb.InsertChoiceNull, "note": qb.InsertChoiceOmit},
			map[string]string{"id": "42"},
		)
		m = insertModel(t, m, tea.KeyMsg{Type: key})
		cols := m.QB.InsertColumns()
		if cols[0].Choice() != qb.InsertChoiceValue {
			t.Fatalf("clearing changed choice to %v", cols[0].Choice())
		}
		if cols[0].Choice() == qb.InsertChoiceValue && cols[0].Choice() != qb.InsertChoiceNull {
			if _, submitted := cols[0].SubmittedValue(); submitted {
				t.Fatalf("clearing kept a submission: %#v", cols[0])
			}
		}
		if _, ok := cols[0].Entered(); ok {
			t.Fatalf("clearing kept entered text: %#v", cols[0])
		}
		if cols[1].Choice() != qb.InsertChoiceNull || cols[2].Choice() != qb.InsertChoiceOmit {
			t.Fatalf("clearing disturbed other choices: %#v", cols)
		}
		if got := m.Fields[m.Focus].Label; got != insertFieldLabel {
			t.Fatalf("clearing moved focus to %q", got)
		}
	}
}

// TestInsertZeroInsertableColumnsBlockExactUI proves a zero-insertable-column
// table shows the exact visible message on Enter, opens no choice or value
// popup, issues no validation or connection execution command, and appends no
// history.
func TestInsertZeroInsertableColumnsBlockExactUI(t *testing.T) {
	q := qb.NewQuery().RefreshSchema(insertUICatalog()).
		SelectCommand(qb.CommandInsert).SelectTable("shadowed")
	m := focusField(modelWithQB(q), insertFieldLabel)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("blocked Enter issued a command (%T)", cmd)
	}
	if m.Popup != nil || m.ValuePrompt != nil {
		t.Fatal("blocked zero-column table opened a popup")
	}
	if got := m.Fields[m.Focus].Content; got != "table has no insertable columns" {
		t.Fatalf("visible content = %q, want exact blocking reason", got)
	}
	if len(m.QB.InsertColumns()) != 0 {
		t.Fatalf("prompt states = %#v, want none", m.QB.InsertColumns())
	}
	if m.QB.HistoryState().Command == qb.CommandInsert && len(m.QB.HistoryState().Inserts) != 0 {
		t.Fatalf("history state gained INSERT entries: %#v", m.QB.HistoryState().Inserts)
	}
	if m.History != nil && m.History.Len() != 0 {
		t.Fatalf("history appended: %d entries", m.History.Len())
	}
}

// TestInsertHistoryReadyStateExact proves a complete mixed or all-omit builder
// exposes the exact history-ready snapshot: every column with its choice and,
// for submitted Value choices, the parsed value and entered representation.
func TestInsertHistoryReadyStateExact(t *testing.T) {
	m := insertUIModel(
		map[string]qb.InsertChoice{"id": qb.InsertChoiceValue, "email": qb.InsertChoiceNull, "note": qb.InsertChoiceOmit},
		map[string]string{"id": "42"},
	)
	state := m.QB.HistoryState()
	want := []qb.HistoryInsertColumn{
		{Column: "id", Choice: qb.InsertChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindInteger, Int: 42}, Entered: "42"},
		{Column: "email", Choice: qb.InsertChoiceNull},
		{Column: "note", Choice: qb.InsertChoiceOmit},
	}
	if !reflect.DeepEqual(state.Inserts, want) {
		t.Fatalf("history state inserts = %#v, want %#v", state.Inserts, want)
	}

	omitOnly := insertUIModel(
		map[string]qb.InsertChoice{"id": qb.InsertChoiceOmit, "email": qb.InsertChoiceOmit, "note": qb.InsertChoiceOmit},
		nil,
	)
	if got := omitOnly.QB.HistoryState().Inserts; len(got) != 3 || got[0].Choice != qb.InsertChoiceOmit {
		t.Fatalf("all-omit history state = %#v", got)
	}
}

// TestInsertTabNavigationBetweenPrompts proves Tab/Shift+Tab move between
// column choice popups with their state intact, matching the prompt order.
func TestInsertTabNavigationBetweenPrompts(t *testing.T) {
	m := insertUIModel(map[string]qb.InsertChoice{}, map[string]string{})
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // open id choice
	if view := m.View(); !strings.Contains(view, "Column: id") {
		t.Fatalf("first prompt not id:\n%s", view)
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if view := m.View(); !strings.Contains(view, "Column: email") {
		t.Fatalf("Tab did not advance to email:\n%s", view)
	}
	m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if view := m.View(); !strings.Contains(view, "Column: id") {
		t.Fatalf("Shift+Tab did not return to id:\n%s", view)
	}
}

// TestInsertUISeedsPromptsOnTableSelection proves the full UI path — INSERT
// command, then table acceptance — automatically presents one prompt per
// insertable column in schema order, visible in the rendered Insert field,
// with zero insertable columns showing no prompts.
func TestInsertUISeedsPromptsOnTableSelection(t *testing.T) {
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: insertUICatalog()},
		key('i'), tea.KeyMsg{Type: tea.KeyEnter}, key('u'), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if got := m.QB.InsertColumns(); len(got) != 3 || got[0].Column != "id" {
		t.Fatalf("seeded prompts = %#v, want id/email/note in schema order", got)
	}
	if view := m.View(); !strings.Contains(view, "id: incomplete") {
		t.Fatalf("Insert field content missing incomplete prompts:\n%s", view)
	}

	// A zero-insertable-column table seeds nothing.
	m = drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: insertUICatalog()},
		key('i'), tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('h'), key('a'), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("setup: zero-column table not selected")
	}
	if got := m.QB.InsertColumns(); len(got) != 0 {
		t.Fatalf("zero-column prompts = %#v, want none", got)
	}
}
