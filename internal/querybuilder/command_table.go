// Command and table selection lifecycle: the Command kind, builder field
// identity, and the pure S/U/D/I transitions shared by every later feature,
// per Issue #11 and the Builder lifecycle decision in Notes/PRD-sqloid.md.

package querybuilder

import "github.com/chris/sqloid/internal/schema"

// Command identifies one builder top-level statement choice.
type Command int

const (
	// CommandUnselected is the startup state before any S/U/D/I key is chosen;
	// it carries zero-value meaning for the initialized model only.
	CommandUnselected Command = iota
	// CommandSelect targets SELECT queries.
	CommandSelect
	// CommandUpdate targets UPDATE statements.
	CommandUpdate
	// CommandDelete targets DELETE statements.
	CommandDelete
	// CommandInsert targets INSERT statements.
	CommandInsert
)

// String renders the human-facing name of the command used in tests and UI
// copy; an unselected command renders empty so it can fill a blank field.
func (c Command) String() string {
	switch c {
	case CommandSelect:
		return "SELECT"
	case CommandUpdate:
		return "UPDATE"
	case CommandDelete:
		return "DELETE"
	case CommandInsert:
		return "INSERT"
	default:
		return ""
	}
}

// Selected reports whether any concrete command is chosen.
func (c Command) Selected() bool {
	return c != CommandUnselected
}

// Field names one builder field identity for focus decisions. Additional
// downstream fields join as later milestones add them.
type Field int

const (
	// FieldCommand is the top-level command field focused at startup.
	FieldCommand Field = iota
	// FieldTable is the table field newly required once a command is chosen.
	FieldTable
)

// QueryBuilder is one immutable snapshot of builder state. Copies share the
// Objects backing slice with prior snapshots because catalogs are owned
// immutably by their producer; all mutating paths go through this package's
// transitions.
//
// The zero value is not meaningful; construct builders through NewQuery and
// its transitions.
type QueryBuilder struct {
	command  Command
	table    string // selected object name exactly as cataloged; "" when none
	tableSet bool   // distinguishes a real selection from the empty name

	objects []*schema.Object // latest refreshed catalog snapshot

	focus       Field
	downstreamG uint64 // bumped each time downstream command-specific state is discarded
}

// NewQuery returns the initial idle builder: nothing selected yet and Command
// focused, matching startup before any S/U/D/I key has been pressed.
func NewQuery() QueryBuilder {
	return QueryBuilder{focus: FieldCommand}
}

// Command reports the currently selected statement kind.
func (q QueryBuilder) Command() Command { return q.command }

// Focus reports which builder field is next required: Command at startup,
// Table afterwards.
func (q QueryBuilder) Focus() Field { return q.focus }

// DownstreamGeneration counts how many times downstream command-specific
// state has been discarded. It lets callers observe the clearing rule — any
// command replacement clears everything below Table — without depending on
// the individual fields, most of which arrive with later milestones.
func (q QueryBuilder) DownstreamGeneration() uint64 { return q.downstreamG }

// SelectedTable reports the selected object name and whether one exists.
func (q QueryBuilder) SelectedTable() (string, bool) {
	return q.table, q.tableSet
}

// EligibleTables returns the objects of the latest refresh that are offered to
// the current command: every object for SELECT, only Schema-declared
// write-eligible kinds for UPDATE/DELETE/INSERT. The returned slice is fresh;
// callers may mutate it freely.
func (q QueryBuilder) EligibleTables() []*schema.Object {
	out := make([]*schema.Object, 0, len(q.objects))
	for _, o := range q.objects {
		if o.WriteEligible || q.command == CommandSelect {
			out = append(out, o)
		}
	}
	return out
}

// RefreshSchema installs a freshly refreshed catalog snapshot and re-applies
// eligibility against the current command. A selected name that vanished from
// the catalog is cleared (per the schema-revalidation requirement that
// identifiers must still exist); surviving selections keep their state.
func (q QueryBuilder) RefreshSchema(c *schema.Catalog) QueryBuilder {
	if c == nil {
		q.objects = nil
	} else {
		q.objects = c.Objects
	}
	if q.tableSet && q.findObject(q.table) == nil {
		q.table, q.tableSet = "", false
	}
	return q
}

// SelectCommand replaces the current command with cmd. Choosing any key
// immediately selects or replaces the command, discards all downstream
// command-specific state, recomputes the eligible-object list under the new
// command's Schema rules, retains the selected table only while still
// eligible, and focuses Table — the first newly required field.
func (q QueryBuilder) SelectCommand(cmd Command) QueryBuilder {
	q.command = cmd
	q.focus = FieldTable
	q.downstreamG++
	if q.tableSet && !q.selectedEligibleFor(cmd) {
		q.table, q.tableSet = "", false
	}
	return q
}

// SelectTable selects name when it appears in the current eligible list for
// the chosen command and is ignored otherwise: unknown names, views during
// write commands, and empty input are rejected. Selection keeps focus on
// Table, where the field lives.
func (q QueryBuilder) SelectTable(name string) QueryBuilder {
	if name == "" || q.findObject(name) == nil {
		return q
	}
	q.table, q.tableSet = name, true
	return q
}

// findObject locates name among the objects of the latest refresh, respecting
// the current command's eligibility filter.
func (q QueryBuilder) findObject(name string) *schema.Object {
	for _, o := range q.EligibleTables() {
		if o.Name == name {
			return o
		}
	}
	return nil
}

// selectedEligibleFor reports whether the selected object remains eligible for
// the given command under Schema metadata: views survive SELECT only, ordinary
// and virtual tables survive every command.
func (q QueryBuilder) selectedEligibleFor(cmd Command) bool {
	o := q.findObjectIn(q.objects, q.table)
	if o == nil {
		return false
	}
	return o.WriteEligible || cmd == CommandSelect
}

// findObjectIn locates name among an arbitrary snapshot, ignoring the
// command filter; used by retention checks that ask about a specific command.
func (q QueryBuilder) findObjectIn(objects []*schema.Object, name string) *schema.Object {
	for _, o := range objects {
		if o.Name == name {
			return o
		}
	}
	return nil
}
