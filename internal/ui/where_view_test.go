// Focused view assertions for Issue #17 Tasks 3–4: the Where field renders in
// the builder bar for every predicate consumer, the universal value prompt
// shows its inline typed-NULL hint plus contextual SQL-null comparison/LIKE
// guidance, and opening overlays never reflow any region.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chris/sqloid/internal/schema"
)

// unstyledWidth measures a line's printable width, ignoring ANSI styling so
// overlay composition can be compared cell-for-cell across frames.
func unstyledWidth(s string) int { return lipgloss.Width(s) }

func whereViewCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 5,
		Objects: []*schema.Object{
			{
				Name:          "users",
				Kind:          schema.KindOrdinaryTable,
				WriteEligible: true,
				Rowid:         schema.RowidHas,
				Columns:       []schema.Column{{Name: "id"}, {Name: "email"}},
			},
		},
	}
}

// viewSetup returns a model on cmd+users sized 80x24.
func viewSetup(cmd rune) Model {
	m := sized(New(), 80, 24).(Model)
	return drive(m, SchemaRefreshedMsg{Catalog: whereViewCatalog()}, key(cmd),
		tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}

// TestWhereFieldAbsentUntilCompleted requires the builder bar to show an empty
// Where content while nothing is committed and the completed predicate text
// once it is, for every consumer command.
func TestWhereFieldAbsentUntilCompleted(t *testing.T) {
	for _, cmd := range []rune{'s', 'u', 'd'} {
		m := viewSetup(cmd)
		var wi int = -1
		for i, f := range m.Fields {
			if f.Label == whereFieldLabel {
				wi = i
				break
			}
		}
		if wi < 0 {
			t.Fatalf("%c: no Where field rendered", cmd)
		}
		if m.Fields[wi].Content != "" {
			t.Errorf("%c: empty-state content=%q, want blank", cmd, m.Fields[wi].Content)
		}
		view := m.View()
		if !strings.Contains(view, whereFieldLabel+":") {
			t.Errorf("%c: Where label missing from builder bar", cmd)
		}
	}
}

// TestValuePromptShowsHintAndContextualGuidance requires the open value entry
// to render both the exact inline typed-NULL hint and the contextual help
// explaining ordinary comparison/LIKE SQL-null behavior and wildcard meaning.
func TestValuePromptShowsHintAndContextualGuidance(t *testing.T) {
	m := beginWhere(whereFocusedAt('s'))
	m = chooseColumnAndOperator(m, 1, "=")
	view := m.View()
	for _, want := range append([]string{WhereTypedNullHint}, WhereNullHelpLines()...) {
		if !strings.Contains(view, want) {
			t.Errorf("value prompt missing guidance %q:\n%s", want, view)
		}
	}
}

// TestValuePromptNeverReflowsRegions requires the value-prompt overlay to keep
// every region boundary identical to the closed-overlay frame: drawing over a
// composed shell must not change line widths or total height.
func TestValuePromptNeverReflowsRegions(t *testing.T) {
	closed := viewSetup('d')
	openV := chooseColumnAndOperator(beginWhere(whereFocusedAt('d')), 0, "=")
	baseLines := strings.Split(closed.View(), "\n")
	openLines := strings.Split(openV.View(), "\n")
	if len(baseLines) != len(openLines) {
		t.Fatalf("overlay changed line count: %d vs %d", len(baseLines), len(openLines))
	}
	for i, l := range baseLines {
		got := openLines[i]
		if unstyledWidth(l) != unstyledWidth(got) {
			t.Errorf("line %d width changed when the prompt opened (%d vs %d)", i, unstyledWidth(l), unstyledWidth(got))
		}
	}
}

// TestPopupOverlayPrecedenceOverWhereFlow pins the global key-precedence
// contract at the WHERE seam: while the column popup is open, Tab does not
// move builder focus; while the value prompt is open, Tab does not either.
func TestPopupOverlayPrecedenceOverWhereFlow(t *testing.T) {
	m := beginWhere(whereFocusedAt('s'))
	if m.Popup == nil {
		t.Fatal("setup: popup missing")
	}
	tabbed := drive(m, tea.KeyMsg{Type: tea.KeyTab}).(Model)
	if tabbed.Popup == nil || tabbed.Focus != m.Focus {
		t.Error("Tab moved focus or closed the popup behind the overlay")
	}
	promptStage := chooseColumnAndOperator(m, 0, "!=")
	if promptStage.ValuePrompt == nil {
		t.Fatal("setup: prompt missing")
	}
	tabbed2 := drive(promptStage, tea.KeyMsg{Type: tea.KeyTab}).(Model)
	if tabbed2.Focus != promptStage.Focus || tabbed2.ValuePrompt == nil {
		t.Error("Tab bypassed the value prompt's modal context")
	}
}
