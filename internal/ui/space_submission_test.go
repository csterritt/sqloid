// Issue #68 Task 3 (RED): scripted UI coverage for tea.KeySpace submission
// through the universal value prompt and the existing field-specific
// ownership boundaries. WHERE/LIKE/SET/INSERT Value choices must preserve the
// exact entered representation including leading, internal, trailing, and
// all-space text adjacent to Unicode; Limit text containing spaces or
// whitespace-only text receives the existing exact field-specific invalid
// reason; searchable popup space retains its search behavior; base-context
// space retains its no-op behavior. Every space here is sent as tea.KeySpace,
// never as a synthetic tea.KeyRunes carrying a space rune.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// typeWithSpaces types s into the open value prompt one rune at a time,
// sending tea.KeySpace for each U+0020 and tea.KeyRunes for every other
// rune. This is the Issue #68 contract: spaces reach the prompt through
// KeySpace, not a synthetic KeyRunes containing a space.
func typeWithSpaces(m Model, s string) Model {
	msgs := []tea.Msg{}
	for _, r := range s {
		if r == ' ' {
			msgs = append(msgs, spaceKey())
			continue
		}
		msgs = append(msgs, key(r))
	}
	return drive(m, msgs...).(Model)
}

// submitValueWithSpaces types s (using KeySpace for spaces) into the open
// value prompt and submits with Enter.
func submitValueWithSpaces(m Model, s string) Model {
	m = typeWithSpaces(m, s)
	return drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}

// spaceValueCases enumerates the leading, internal, trailing, all-space, and
// Unicode-adjacent space buffers the submission contract must preserve.
func spaceValueCases() []string {
	return []string{
		" leading",    // leading space
		"int ernal",   // internal spaces
		"trailing ",   // trailing space
		"   ",         // all spaces
		" 世界 ",        // spaces adjacent to multibyte Unicode
		"hello world", // ordinary internal space
		"  double  ",  // multiple leading, internal, and trailing
	}
}

// TestWhereEqualityPreservesSpacesThroughKeySpace requires the guided WHERE
// `=` flow to bind the exact entered representation — including leading,
// internal, trailing, all-space, and Unicode-adjacent spaces — as a verbatim
// TEXT parameter when every space is typed through tea.KeySpace.
func TestWhereEqualityPreservesSpacesThroughKeySpace(t *testing.T) {
	for _, text := range spaceValueCases() {
		t.Run(text, func(t *testing.T) {
			m := beginWhere(whereFocusedAt('s'))
			m = chooseColumnAndOperator(m, 1, "=") // email
			if m.ValuePrompt == nil {
				t.Fatal("value prompt did not open for = operator")
			}
			m = submitValueWithSpaces(m, text)
			if m.ValuePrompt != nil || m.Popup != nil {
				t.Fatalf("submission left overlays open: prompt=%v popup=%v", m.ValuePrompt != nil, m.Popup != nil)
			}
			if !m.QB.HasWhere() {
				t.Fatal("submission did not commit the WHERE predicate")
			}
			params := m.QB.WhereParams()
			if len(params) != 1 {
				t.Fatalf("bound %d params, want 1", len(params))
			}
			if got, ok := params[0].(string); !ok || got != text {
				t.Fatalf("bound param = %#v (%T), want verbatim TEXT %q", params[0], params[0], text)
			}
			if sql := m.QB.WherePredicate().SQL(); sql != `"email" = ?` {
				t.Errorf("committed SQL = %q, want \"email\" = ?", sql)
			}
		})
	}
}

// TestWhereLikePreservesSpacesThroughKeySpace requires the guided WHERE LIKE
// flow to bind the exact entered representation including spaces as a
// verbatim TEXT parameter, with the LIKE wildcards and spaces preserved
// byte-for-byte.
func TestWhereLikePreservesSpacesThroughKeySpace(t *testing.T) {
	for _, text := range []string{
		" %a b% ", // wildcards with internal and surrounding spaces
		"  _  ",   // underscore wildcard with spaces
		"世界 like", // Unicode then space then ASCII
	} {
		t.Run(text, func(t *testing.T) {
			m := beginWhere(whereFocusedAt('s'))
			m = chooseColumnAndOperator(m, 2, "LIKE") // note
			if m.ValuePrompt == nil {
				t.Fatal("value prompt did not open for LIKE operator")
			}
			m = submitValueWithSpaces(m, text)
			if !m.QB.HasWhere() {
				t.Fatal("LIKE submission did not commit")
			}
			params := m.QB.WhereParams()
			if len(params) != 1 {
				t.Fatalf("bound %d params, want 1", len(params))
			}
			if got, ok := params[0].(string); !ok || got != text {
				t.Fatalf("LIKE bound param = %#v (%T), want verbatim %q", params[0], params[0], text)
			}
			if sql := m.QB.WherePredicate().SQL(); sql != `"note" LIKE ?` {
				t.Errorf("committed SQL = %q, want \"note\" LIKE ?", sql)
			}
		})
	}
}

// TestUpdateSetPreservesSpacesThroughKeySpace requires the guided UPDATE SET
// Value flow to bind the exact entered representation including spaces as a
// verbatim TEXT parameter when every space is typed through tea.KeySpace.
func TestUpdateSetPreservesSpacesThroughKeySpace(t *testing.T) {
	for _, text := range spaceValueCases() {
		t.Run(text, func(t *testing.T) {
			m := updateUIModel()
			m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // open SET popup
			m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // accept id column
			m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})   // close selection, open choice
			m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // Value choice
			if m.ValuePrompt == nil {
				t.Fatal("SET Value choice did not open universal entry")
			}
			m = submitValueWithSpaces(m, text)
			assignments := m.QB.SetAssignments()
			if len(assignments) != 1 {
				t.Fatalf("SET assignments = %d, want 1", len(assignments))
			}
			value, ok := assignments[0].SubmittedValue()
			if !ok || assignments[0].Choice() != qb.SetChoiceValue {
				t.Fatalf("assignment choice = %v ok=%v, want Value", assignments[0].Choice(), ok)
			}
			if value.Kind != qb.KindText || value.Text != text {
				t.Fatalf("SET bound value = %+v, want verbatim TEXT %q", value, text)
			}
			params := m.QB.UpdateParams()
			if len(params) != 1 {
				t.Fatalf("UpdateParams = %v, want one param", params)
			}
			if got, ok := params[0].(string); !ok || got != text {
				t.Fatalf("UpdateParams[0] = %#v (%T), want verbatim %q", params[0], params[0], text)
			}
		})
	}
}

// TestInsertValuePreservesSpacesThroughKeySpace requires the guided INSERT
// Value flow to bind the exact entered representation including spaces as a
// verbatim TEXT parameter when every space is typed through tea.KeySpace.
func TestInsertValuePreservesSpacesThroughKeySpace(t *testing.T) {
	for _, text := range spaceValueCases() {
		t.Run(text, func(t *testing.T) {
			m := insertUIModel(map[string]qb.InsertChoice{}, map[string]string{})
			m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // open id prompt
			m = insertModel(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // Value choice
			if m.ValuePrompt == nil {
				t.Fatal("INSERT Value choice did not open universal entry")
			}
			m = submitValueWithSpaces(m, text)
			cols := m.QB.InsertColumns()
			if len(cols) < 1 || cols[0].Choice() != qb.InsertChoiceValue {
				t.Fatalf("first INSERT choice = %v, want Value", cols[0].Choice())
			}
			value, ok := cols[0].SubmittedValue()
			if !ok {
				t.Fatal("INSERT Value not submitted")
			}
			if value.Kind != qb.KindText || value.Text != text {
				t.Fatalf("INSERT bound value = %+v, want verbatim TEXT %q", value, text)
			}
			if got := value.ParamValue(); got != text {
				t.Fatalf("INSERT ParamValue = %#v (%T), want verbatim %q", got, got, text)
			}
		})
	}
}

// TestLimitSpacesReceiveExistingInvalidReason requires Limit text containing
// spaces or whitespace-only text — typed through tea.KeySpace — to receive
// the existing exact field-specific invalid reason, with the entered text
// preserved verbatim and no accepted value.
func TestLimitSpacesReceiveExistingInvalidReason(t *testing.T) {
	for _, text := range []string{
		" ",   // single space
		"  ",  // multiple spaces
		" 5",  // leading space before digit
		"5 ",  // trailing space after digit
		"1 0", // internal space
		" \t", // space and tab (tab via KeyRunes control)
	} {
		t.Run(text, func(t *testing.T) {
			m := limitFocused(t)
			m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // open prompt
			if m.ValuePrompt == nil {
				t.Fatal("Enter did not open the Limit value prompt")
			}
			// Type the text: spaces via KeySpace, other runes via KeyRunes.
			msgs := []tea.Msg{}
			for _, r := range text {
				if r == ' ' {
					msgs = append(msgs, spaceKey())
					continue
				}
				msgs = append(msgs, key(r))
			}
			m = drive(m, msgs...).(Model)
			if got := m.ValuePrompt.Buffer(); got != text {
				t.Fatalf("prompt buffer = %q, want %q", got, text)
			}
			m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // submit
			if m.QB.LimitInput() != text {
				t.Fatalf("LimitInput = %q, want the entered text preserved", m.QB.LimitInput())
			}
			if _, ok := m.QB.LimitValue(); ok {
				t.Fatal("space-containing Limit produced an accepted value")
			}
			issue, invalid := m.QB.FirstInvalidIssue()
			if !invalid || issue.Field != qb.FieldIdentityLimit || issue.Reason != qb.LimitInvalidReason {
				t.Fatalf("first-invalid = %+v, want Limit/%s", issue, qb.LimitInvalidReason)
			}
			want := text + " — " + qb.LimitInvalidReason
			if got := m.Fields[findField(t, m, limitFieldLabel)].Content; got != want {
				t.Fatalf("content = %q, want %q", got, want)
			}
			if view := m.View(); !strings.Contains(view, qb.LimitInvalidReason) {
				t.Fatal("rendered view lacks the exact invalid reason")
			}
		})
	}
}

// TestSearchablePopupSpaceRetainsSearchBehavior is a control proving that
// tea.KeySpace inside an open searchable popup still appends one space to the
// popup search query and does not leak into any value prompt or builder
// state.
func TestSearchablePopupSpaceRetainsSearchBehavior(t *testing.T) {
	m := baseSized(t)
	before := m.QB.Command()
	focusBefore := m.Focus
	m.installPopup(NewSearchablePopup(testOpenerLabel, candidates("alpha", "beta")), nil)
	m.Popup.SetViewportHeight(testPopupViewport)
	next := drive(m, key('a'), key('l'), spaceKey(), key('b')).(Model)
	if next.Popup == nil {
		t.Fatal("space closed the searchable popup")
	}
	if got := next.Popup.Search; got != "al b" {
		t.Fatalf("popup search = %q, want \"al b\" (space appended to search)", got)
	}
	if next.QB.Command() != before {
		t.Errorf("command mutated under popup: %v -> %v", before, next.QB.Command())
	}
	if next.Focus != focusBefore {
		t.Errorf("builder focus moved under popup: %d -> %d", focusBefore, next.Focus)
	}
	if next.ValuePrompt != nil {
		t.Fatal("space inside a searchable popup opened a value prompt")
	}
}

// TestBaseContextSpaceIsNoOp is a control proving that tea.KeySpace in the
// base builder context (no overlay, no focused input) is a no-op: it does
// not open a popup, move focus, mutate builder state, or open a value prompt.
func TestBaseContextSpaceIsNoOp(t *testing.T) {
	m := baseSized(t)
	before := m.QB
	focusBefore := m.Focus
	next, cmd := m.Update(spaceKey())
	if cmd != nil {
		t.Fatalf("base-context space returned a command %v", cmd)
	}
	got := next.(Model)
	if !sameBuilder(got.QB, before) {
		t.Errorf("base-context space mutated builder state")
	}
	if got.Focus != focusBefore {
		t.Errorf("base-context space moved focus: %d -> %d", focusBefore, got.Focus)
	}
	if got.Popup != nil {
		t.Fatal("base-context space opened a popup")
	}
	if got.ValuePrompt != nil {
		t.Fatal("base-context space opened a value prompt")
	}
}

// sameBuilder reports whether two QueryBuilders are deep-equal for the
// purposes of the base-context no-op control.
func sameBuilder(a, b qb.QueryBuilder) bool {
	return a.SelectSQL() == b.SelectSQL() &&
		a.Command() == b.Command() &&
		a.LimitInput() == b.LimitInput()
}
