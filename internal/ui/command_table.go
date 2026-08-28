// Command and table builder fields inside the Bubble Tea shell, per Issue #11:
// the QueryBuilder state renders into the existing field bar, plain S/U/D/I
// keys select or replace the command only while Command is focused, and focus
// advances to Table — preserving the Issue #8 focus-navigation, layout, and
// responsiveness seams. Database logic stays out of this package.

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// Field labels used by the builder's rendered field bar. The label constants
// also map QueryBuilder field identities onto their display positions.
const (
	commandFieldLabel = "Command"
	tableFieldLabel   = "Table"
)

// columnsFieldLabel is defined in projection_popup.go as both the field-bar
// label of the SELECT Column(s) field and the popup opener identity.

// SchemaRefreshedMsg carries a freshly refreshed schema catalog into the model
// so the builder's eligible-object list follows the latest Schema metadata.
type SchemaRefreshedMsg struct {
	Catalog *schema.Catalog
}

// commandKeys maps each one-key selection to its command: a single plain
// letter immediately selects or replaces the command while Command holds
// focus.
func commandKey(s string) (qb.Command, bool) {
	switch strings.ToLower(s) {
	case "s":
		return qb.CommandSelect, true
	case "u":
		return qb.CommandUpdate, true
	case "d":
		return qb.CommandDelete, true
	case "i":
		return qb.CommandInsert, true
	default:
		return qb.CommandUnselected, false
	}
}

// builderFields renders the builder's current state as field-bar entries: the
// Command field always exists, Table appears once any command is chosen, and
// a SELECT gains its Column(s) entry once a table is selected.
func builderFields(q qb.QueryBuilder) []Field {
	fields := []Field{{Label: commandFieldLabel, Content: q.Command().String()}}
	name, ok := q.SelectedTable()
	table := ""
	if ok {
		table = name
	}
	if q.Command().Selected() {
		fields = append(fields, Field{Label: tableFieldLabel, Content: table})
	}
	_, tableSet := q.SelectedTable()
	if q.Command() == qb.CommandSelect && tableSet {
		fields = append(fields, Field{Label: columnsFieldLabel,
			Content: projectionEntryLabels(q.ProjectionEntries())})
	}
	if q.WhereReady() {
		// Query Grammar order: WHERE trails every projected field it filters.
		fields = append(fields, Field{Label: whereFieldLabel,
			Content: q.WherePredicate().SQL()})
	}
	return fields
}

// applyBuilder installs a new QueryBuilder snapshot, rebuilds the rendered
// fields from it, and moves UI focus to match the builder's next required
// field when that field exists; otherwise the current focused label is kept
// when still present.
func (m *Model) applyBuilder(next qb.QueryBuilder) {
	previous := ""
	if m.Focus >= 0 && m.Focus < len(m.Fields) {
		previous = m.Fields[m.Focus].Label
	}
	m.QB = next
	m.Fields = builderFields(next)
	for i := range m.Fields {
		if wantFocusLabel(next.Focus()) == m.Fields[i].Label {
			m.Focus = i
			return
		}
	}
	for i := range m.Fields {
		if previous != "" && m.Fields[i].Label == previous {
			m.Focus = i
			return
		}
	}
	m.setFocus(m.Focus)
}

// wantFocusLabel maps a QueryBuilder field identity to its rendered label.
func wantFocusLabel(f qb.Field) string {
	switch f {
	case qb.FieldTable:
		return tableFieldLabel
	case qb.FieldColumns:
		return columnsFieldLabel
	default:
		return commandFieldLabel
	}
}

// handleCommandKey routes one-key S/U/D/I selection while the Command field is
// focused, applying the QueryBuilder's transition exactly — including
// downstream clearing, Schema-driven table retention/clearing, and the Table
// focus advance. It reports whether the message was consumed.
func handleCommandKey(m *Model, msg tea.KeyMsg) bool {
	cmd, ok := commandKey(msg.String())
	if !ok {
		return false
	}
	if !m.commandFocused() {
		return false
	}
	m.applyBuilder(m.QB.SelectCommand(cmd))
	return true
}

// applySchemaRefresh installs a refreshed catalog and rebuilds the rendered
// fields from the resulting builder snapshot, keeping UI focus on the same
// labeled field when it survives the refresh.
func (m *Model) applySchemaRefresh(c *schema.Catalog) Model {
	previous := ""
	if m.Focus >= 0 && m.Focus < len(m.Fields) {
		previous = m.Fields[m.Focus].Label
	}
	m.QB = m.QB.RefreshSchema(c)
	m.Fields = builderFields(m.QB)
	for i := range m.Fields {
		if previous != "" && m.Fields[i].Label == previous {
			m.Focus = i
			break
		}
	}
	return *m
}

// commandFocused reports whether the field bar currently has Command focused.
func (m *Model) commandFocused() bool {
	if m.suspended || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == commandFieldLabel
}
