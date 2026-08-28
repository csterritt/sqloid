// Scripted Bubble Tea model coverage for Issue #15 Tasks 3–4: the SELECT
// Column(s) popup flow driven end-to-end through Update — empty-projection
// ordering with `*` highlighted, bare COUNT(*) accepted directly with an
// immediate Column(s) reopen, named-column routing to the scroll-only
// Value/Count/Min/Max/Avg/Sum aggregate popup, aggregate completion reopening
// Column(s), and sentinel reappearance when removal empties the projection.
// Projection rules are never duplicated here: they come from the QueryBuilder
// transitions covered by internal/querybuilder/projection_test.go.

package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func projectionUICatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 9,
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

// openColumnsPopup drives a fresh model through SELECT, accepting the users
// table, then tabs to the Column(s) field and presses Enter to open its
// popup over an empty projection.
func openColumnsPopup(t *testing.T) Model {
	t.Helper()
	m := sized(New(), 80, 24).(Model)
	m = drive(m, SchemaRefreshedMsg{Catalog: projectionUICatalog()}, key('s')).(Model)
	if _, ok := m.QB.SelectedTable(); ok {
		t.Fatal("setup already selected a table")
	}
	// Enter opens the Table popup, search narrows to users, Enter commits.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}, key('s'), key('e'), key('r'),
		tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if name, ok := m.QB.SelectedTable(); !ok || name != "users" {
		t.Fatalf("setup selected (%q,%v), want users", name, ok)
	}
	if got := len(m.Fields); got != 3 || m.Fields[2].Label != columnsFieldLabel {
		t.Fatalf("field bar=%v, want exactly three fields ending in %v", labels(m.Fields), columnsFieldLabel)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyTab}).(Model)
	if m.Focus != 2 {
		t.Fatalf("setup focus=%d, want the Column(s) field at index 2", m.Focus)
	}
	return drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}

// labels lists field-bar labels, keeping failure messages small.
func labels(fields []Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Label
	}
	return out
}

func visibleCandidates(m Model) []string {
	var out []string
	for _, c := range m.Popup.Visible() {
		out = append(out, c.ID)
	}
	return out
}

// TestColumnsEnterOpensEmptyProjectionPopupWithWildcardHighlighted requires
// the empty-SELECT popup: exact ordering `*`, COUNT(*), id, email; default
// highlight on `*`; a searchable single-select opened by Column(s).
func TestColumnsEnterOpensEmptyProjectionPopupWithWildcardHighlighted(t *testing.T) {
	m := openColumnsPopup(t)
	if m.Popup == nil || !m.Popup.Open() {
		t.Fatal("Enter on Column(s) did not open its popup")
	}
	if m.Popup.Mode != PopupSearchable || m.Popup.Multi {
		t.Errorf("popup mode=%v multi=%v, want searchable single-select", m.Popup.Mode, m.Popup.Multi)
	}
	if m.Popup.Opener != columnsFieldLabel {
		t.Errorf("opener=%q, want %q", m.Popup.Opener, columnsFieldLabel)
	}
	want := []string{qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}.Key(),
		qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key(),
		qb.ProjectionCandidate{Kind: qb.ProjectionColumn, Column: "id"}.Key(),
		qb.ProjectionCandidate{Kind: qb.ProjectionColumn, Column: "email"}.Key()}
	if got := visibleCandidates(m); !slices.Equal(got, want) {
		t.Errorf("candidates=%v, want %v", got, want)
	}
	view := RenderPopup(m.Popup, 60, 12)
	// The caret marks exactly the first row (`*`); nothing else is `> `-marked.
	if strings.Count(view, popupSelectedPrefix+"*") != 1 ||
		strings.Contains(view, popupSelectedPrefix+"COUNT(*)") ||
		strings.Contains(view, popupSelectedPrefix+"id") ||
		strings.Contains(view, popupSelectedPrefix+"email") {
		t.Errorf("wildcard not the single default-highlighted row:\n%s", view)
	}
}

// TestCountStarAcceptsDirectlyAndReopensColumns requires accepting bare
// COUNT(*) to commit the sentinel without any aggregate step, close the
// accept popup, immediately reopen Column(s) with exact focus and reset
// search/highlight state, and drop the synthetic items from the now-nonempty
// list.
func TestCountStarAcceptsDirectlyAndReopensColumns(t *testing.T) {
	m := openColumnsPopup(t)
	next := drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if next.Popup == nil || !next.Popup.Open() {
		t.Fatal("COUNT(*) acceptance did not reopen Column(s)")
	}
	if next.Popup.Mode != PopupSearchable || next.Popup.Opener != columnsFieldLabel {
		t.Errorf("reopened popup=%+v, want searchable Column(s)", *next.Popup)
	}
	if next.Popup.Search != "" {
		t.Errorf("reopened popup kept search %q, want deterministic reset", next.Popup.Search)
	}
	got := visibleCandidates(next)
	sentinel := qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key()
	wildcard := qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}.Key()
	for _, c := range got {
		if c == sentinel || c == wildcard {
			t.Errorf("synthetic %q leaked into nonempty candidates %v", c, got)
		}
	}
	if got[0] != (qb.ProjectionCandidate{Kind: qb.ProjectionColumn, Column: "id"}.Key()) {
		t.Errorf("nonempty default candidate=%q, want id", got[0])
	}
	if content := next.Fields[next.Focus].Content; content != "COUNT(*)" {
		t.Errorf("Column(s) content=%q, want COUNT(*) with focus=%v", content, labels(next.Fields)[next.Focus])
	}
	entries := next.QB.ProjectionEntries()
	if len(entries) != 1 || entries[0].Kind != qb.ProjectionCountStar {
		t.Errorf("entries=%v, want the bare sentinel committed directly", entries)
	}
}

// TestNamedColumnRoutesThroughAggregatePopup requires a named-column choice
// from both an empty and a populated projection to open the scroll-only
// Value/Count/Min/Max/Avg/Sum aggregate popup without committing, and its
// acceptance to reopen Column(s) preserving completed entries.
func TestNamedColumnRoutesThroughAggregatePopup(t *testing.T) {
	starts := map[string]func(Model) Model{}
	starts["empty"] = func(m Model) Model { return m }
	starts["populated"] = func(m Model) Model {
		// Highlight the sentinel (one Down from `*`) and accept it directly.
		return drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	}
	for name, prepare := range starts {
		m := prepare(openColumnsPopup(t))
		// Move past the synthetic rows to the `id` row: two Downs from an
		// empty projection, none from a populated one (whose reopened popup
		// already defaults to the first named column) — then accept it.
		downs := 2
		if name == "populated" {
			downs = 0
		}
		for i := 0; i < downs; i++ {
			m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
		}
		m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
		if m.Popup == nil || !m.Popup.Open() {
			t.Fatalf("%s: named acceptance produced no popup", name)
		}
		if m.Popup.Mode != PopupScrollOnly || m.Popup.Opener != columnsFieldLabel {
			t.Errorf("%s: aggregate popup=%+v, want scroll-only opened by Column(s)", name, *m.Popup)
		}
		forbidden := []string{"MIN(*)", "MAX(*)", "AVG(*)", "SUM(*)"}
		for _, bad := range forbidden {
			if strings.Contains(RenderPopup(m.Popup, 60, 12), bad) {
				t.Errorf("%s: unsupported aggregate-on-wildcard %q rendered", name, bad)
			}
		}
		beforeEntries := m.QB.ProjectionEntries()
		if got := m.QB.ProjectionEmpty(); (name == "empty") != got {
			t.Errorf("%s: named selection committed before aggregate: empty=%v", name, got)
		}
		gotAggs := visibleCandidates(m)
		wantAggs := []string{"Value", "Count", "Min", "Max", "Avg", "Sum"}
		if !slices.Equal(gotAggs, wantAggs) {
			t.Errorf("%s: aggregate choices=%v, want %v", name, gotAggs, wantAggs)
		}
		// Walk to Avg and accept.
		for i := 0; i < 4; i++ {
			m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
		}
		m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
		if m.Popup == nil || !m.Popup.Open() || m.Popup.Mode != PopupSearchable ||
			m.Popup.Opener != columnsFieldLabel {
			t.Fatalf("%s: aggregate completion did not reopen Column(s): %+v", name, m.Popup)
		}
		entries := m.QB.ProjectionEntries()
		if len(entries) != len(beforeEntries)+1 {
			t.Errorf("%s: entries=%v did not grow by one aggregate completion", name, entries)
		}
		last := entries[len(entries)-1]
		if last.Kind != qb.ProjectionColumn || last.Column != "id" || last.Aggregate != qb.AggAvg {
			t.Errorf("%s: completed entry=%+v, want id(AVG)", name, last)
		}
	}
}

// TestWildcardNeverOpensAggregatePopup requires wildcard acceptance to
// complete the sole projection directly: no aggregate popup may ever open,
// and the popup closes with the wildcard committed to the field bar.
func TestWildcardNeverOpensAggregatePopup(t *testing.T) {
	m := openColumnsPopup(t)
	next := drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if next.Popup != nil {
		t.Fatalf("wildcard acceptance left a popup open: %+v", *next.Popup)
	}
	if content := next.Fields[2].Content; content != "*" {
		t.Errorf("Column(s) content=%q, want * committed directly", content)
	}
	if entries := next.QB.ProjectionEntries(); len(entries) != 1 || entries[0].Kind != qb.ProjectionWildcard {
		t.Errorf("entries=%v, want the wildcard identity committed", entries)
	}
}

// TestRemovalToEmptyRestoresSentinel removes every entry back to an empty
// projection through the existing builder seam and requires the reopened
// Column(s) popup to offer the sentinel again in second position.
func TestRemovalToEmptyRestoresSentinel(t *testing.T) {
	m := openColumnsPopup(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // COUNT(*)
	if m.QB.ProjectionEmpty() {
		t.Fatal("setup did not populate the projection")
	}
	// Issue #16 owns ordered removal keys; use the existing applyBuilder seam.
	for !m.QB.ProjectionEmpty() {
		m.applyBuilder(m.QB.RemoveProjection(len(m.QB.ProjectionEntries()) - 1))
	}
	m.reopenColumnsPopup()
	if m.Popup == nil || !m.Popup.Open() || m.Popup.Opener != columnsFieldLabel {
		t.Fatalf("reopened popup=%+v, want Column(s)", m.Popup)
	}
	got := visibleCandidates(m)
	if len(got) != 4 {
		t.Fatalf("emptied candidates=%v, want all four identities restored", got)
	}
	if got[1] != (qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key()) {
		t.Errorf("restored candidate[1]=%q, want the sentinel in second position", got[1])
	}
}
