// Demo application for the Issue #11 walkthrough (disposable, lives outside
// the production source tree): drives the query builder's command and table
// selection lifecycle through the Bubble Tea shell exactly as the tests do,
// printing observable state and rendered regions at each step.
package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/schema"
	"github.com/chris/sqloid/internal/ui"
)

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func shiftTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyShiftTab} }

func catalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 7,
		Objects: []*schema.Object{
			{Name: "logs_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable},
			{Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
			{Name: "vw_summary", Kind: schema.KindView, WriteEligible: false, Rowid: schema.RowidNotApplicable},
		},
	}
}

// apply routes messages through Update and returns the concrete model so the
// scripted steps stay readable.
func apply(m tea.Model, msgs ...tea.Msg) ui.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(ui.Model)
}

func step(name string) { fmt.Printf("\n=== %s ===\n", name) }

func dump(m ui.Model, width int) {
	fmt.Printf("UI focus=%d fields:", m.Focus)
	for _, f := range m.Fields {
		fmt.Printf(" [%s=%q]", f.Label, f.Content)
	}
	if name, ok := m.QB.SelectedTable(); ok {
		fmt.Printf(" selectedTable=%q downstreamGen=%d", name, m.QB.DownstreamGeneration())
	} else {
		fmt.Printf(" selectedTable=<none> downstreamGen=%d", m.QB.DownstreamGeneration())
	}
	var names []string
	for _, o := range m.QB.EligibleTables() {
		names = append(names, fmt.Sprintf("%s(%s)", o.Name, o.Kind))
	}
	fmt.Printf(" eligibles=[%s]\n", strings.Join(names, " "))
	lines := strings.Split(m.View(), "\n")
	for i, l := range lines[:12] {
		fmt.Printf("%2d %s\n", i+1, l)
	}
}

func main() {
	step("startup idle")
	m := ui.New()
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	dump(m, 80)

	step("press S — SELECT chosen, focus advances to Table")
	m = apply(m, key('s'))
	dump(m, 80)

	step("Shift+Tab back to Command, press U / D / I replacing the command")
	m = apply(m, shiftTab())
	m = apply(m, key('u'))
	dump(m, 80)
	m = apply(m, shiftTab())
	m = apply(m, key('d'))
	dump(m, 80)
	m = apply(m, shiftTab())
	m = apply(m, key('i'))
	dump(m, 80)

	step("refresh schema; back to SELECT; pick the view vw_summary as table")
	m = apply(m, ui.SchemaRefreshedMsg{Catalog: catalog()})
	m = apply(m, shiftTab())
	m = apply(m, key('s'))
	m.QB = m.QB.SelectTable("vw_summary")
	dump(m, 80)

	step("switch to INSERT: view cleared, Table focused, eligible write tables listed")
	m = apply(m, shiftTab())
	m = apply(m, key('i'))
	dump(m, 80)

	step("retain eligible ordinary table across UPDATE -> DELETE")
	m = apply(m, shiftTab())
	m = apply(m, key('u'))
	m.QB = m.QB.SelectTable("users")
	m = apply(m, shiftTab())
	m = apply(m, key('d'))
	dump(m, 80)

}
