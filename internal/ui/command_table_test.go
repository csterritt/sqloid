// Scripted Bubble Tea model tests for the command and table selection
// lifecycle integration, per Issue #11 Tasks 3–4: startup focus on Command,
// one plain S/U/D/I key selecting/replacing the command and advancing focus to
// Table, revisiting Command, Schema-driven table retention and view-to-write
// clearing reflected through the UI, and idle-state distinction from an
// executed-empty result. No database behavior runs here.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func uiCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 7,
		Objects: []*schema.Object{
			{Name: "logs_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable},
			{Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
			{Name: "vw_summary", Kind: schema.KindView, WriteEligible: false, Rowid: schema.RowidNotApplicable},
		},
	}
}

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// drive sends msgs through Update, returning the final model.
func drive(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func sized(m tea.Model, w, h int) tea.Model {
	return drive(m, tea.WindowSizeMsg{Width: w, Height: h})
}

// TestStartupFocusesCommand pins that the initial model focuses Command with
// exactly the pre-selection builder fields.
func TestStartupFocusesCommand(t *testing.T) {
	m := sized(New(), 80, 24).(Model)
	if m.Focus != 0 || m.Fields[0].Label != "Command" {
		t.Errorf("startup focus=%d first field=%q, want Command focused", m.Focus, m.Fields[0].Label)
	}
	if got := len(m.Fields); got != 1 {
		t.Errorf("startup fields=%d, want only Command before any selection", got)
	}
	if m.QB.Command() != querybuilder.CommandUnselected || m.QB.Focus() != querybuilder.FieldCommand {
		t.Errorf("startup builder = command %v focus %v, want unselected/Command",
			m.QB.Command(), m.QB.Focus())
	}
}

// TestOneKeyCommandsAdvanceToTable presses each plain S/U/D/I key once from a
// fresh model and requires the command selection plus focus advance to Table.
func TestOneKeyCommandsAdvanceToTable(t *testing.T) {
	cases := []struct {
		key rune
		cmd querybuilder.Command
	}{
		{'s', querybuilder.CommandSelect},
		{'u', querybuilder.CommandUpdate},
		{'d', querybuilder.CommandDelete},
		{'i', querybuilder.CommandInsert},
	}
	for _, tc := range cases {
		m := drive(sized(New(), 80, 24), key(tc.key)).(Model)
		if m.QB.Command() != tc.cmd {
			t.Errorf("%q left command %v, want %v", tc.key, m.QB.Command(), tc.cmd)
		}
		if m.QB.Focus() != querybuilder.FieldTable || m.Focus != 1 {
			t.Errorf("%q gave builder focus %v / UI focus %d, want Table/%d",
				tc.key, m.QB.Focus(), m.Focus, 1)
		}
		if n := len(m.Fields); n != 2 || m.Fields[1].Label != "Table" {
			t.Errorf("%q rendered fields %#v, want Command then Table", tc.key, m.Fields)
		}
	}
}

// TestRevisitCommandReplacesCommand tabs back to Command (Shift+Tab from
// Table), replaces the command with another plain key, and lands on Table with
// all downstream state cleared.
func TestRevisitCommandReplacesCommand(t *testing.T) {
	m := drive(sized(New(), 80, 24),
		key('s'), tea.KeyMsg{Type: tea.KeyShiftTab}, key('d')).(Model)
	if m.QB.Command() != querybuilder.CommandDelete {
		t.Errorf("revisit left command %v, want DELETE", m.QB.Command())
	}
	if m.Focus != 1 || m.QB.Focus() != querybuilder.FieldTable {
		t.Errorf("after replacement focus builder=%v ui=%d, want Table/1", m.QB.Focus(), m.Focus)
	}
}

// TestCommandKeysIgnoredWhileTableFocused routes S/U/D/I only while Command is
// focused: once Table holds focus, those plain letters do nothing.
func TestCommandKeysIgnoredWhileTableFocused(t *testing.T) {
	m := drive(sized(New(), 80, 24), key('s'), key('d')).(Model)
	if m.QB.Command() != querybuilder.CommandSelect {
		t.Errorf("'d' leaked into Table-focused context: command %v", m.QB.Command())
	}
}

// TestRefreshedCatalogPopulatesEligibles injects Schema eligibility metadata
// and requires the refreshed eligible list to surface through the builder:
// every object under SELECT, ordinary plus virtual tables only under writes.
func TestRefreshedCatalogPopulatesEligibles(t *testing.T) {
	m := drive(sized(New(), 80, 24),
		SchemaRefreshedMsg{Catalog: uiCatalog()}, key('s')).(Model)
	got := m.QB.EligibleTables()
	names := []string{}
	for _, o := range got {
		names = append(names, o.Name)
	}
	if len(names) != 3 {
		t.Fatalf("SELECT eligibles = %v, want three cataloged objects", names)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyShiftTab}, key('u')).(Model)
	got = m.QB.EligibleTables()
	if len(got) != 2 {
		t.Fatalf("UPDATE eligibles = %v, want two write tables", namesOf(got))
	}
	for _, o := range got {
		if o.Name == "vw_summary" || !o.WriteEligible {
			t.Errorf("write eligibility includes %s kind=%v", o.Name, o.Kind)
		}
	}
}

func namesOf(objs []*schema.Object) []string {
	var out []string
	for _, o := range objs {
		out = append(out, o.Name)
	}
	return out
}

// TestViewClearedByWriteCommandThroughUI selects a view during SELECT, then
// switches to a write command via the UI path: the view must clear while the
// refreshed eligible write-table list stays populated.
func TestViewClearedByWriteCommandThroughUI(t *testing.T) {
	m := drive(sized(New(), 80, 24),
		SchemaRefreshedMsg{Catalog: uiCatalog()}, key('s')).(Model)
	m.QB = m.QB.SelectTable("vw_summary")
	if name, ok := m.QB.SelectedTable(); !ok || name != "vw_summary" {
		t.Fatalf("setup failed: selected (%q,%v)", name, ok)
	}
	m2 := drive(m, tea.KeyMsg{Type: tea.KeyShiftTab}, key('i')).(Model)
	if _, ok := m2.QB.SelectedTable(); ok {
		t.Error("INSERT kept the selected view, want cleared")
	}
	retained := namesOf(m2.QB.EligibleTables())
	if len(retained) != 2 {
		t.Errorf("eligible tables after switching to INSERT = %v, want the two write tables", retained)
	}
}

// TestSelectedOrdinaryTableRetainedThroughWriteSwitch verifies the retention
// rule end-to-end: an eligible ordinary table survives an UPDATE→DELETE
// replacement routed through the UI.
func TestSelectedOrdinaryTableRetainedThroughWriteSwitch(t *testing.T) {
	m := drive(sized(New(), 80, 24),
		SchemaRefreshedMsg{Catalog: uiCatalog()},
		key('u'),
	).(Model)
	m.QB = m.QB.SelectTable("users")
	m2 := drive(m, tea.KeyMsg{Type: tea.KeyShiftTab}, key('d')).(Model)
	name, ok := m2.QB.SelectedTable()
	if !ok || name != "users" {
		t.Errorf("DELETE retained (%q,%v), want (users,true)", name, ok)
	}
}

// TestIdleDiffersFromExecutedEmpty pins that idle output never carries the
// executed-empty marker or any result-only decoration.
func TestIdleDiffersFromExecutedEmpty(t *testing.T) {
	idle := sized(New(), 80, 24).View()
	for _, marker := range []string{"No rows", "Result count"} {
		if strings.Contains(idle, marker) {
			t.Errorf("idle view contains %q: resembles an executed result", marker)
		}
	}
	traced := drive(sized(New(), 80, 24), traceSettledMsg{}).View()
	if traced == idle {
		t.Error("settled-tracer presentation identical to idle; states indistinguishable")
	}
}
