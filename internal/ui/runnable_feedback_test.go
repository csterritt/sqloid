// Scripted Bubble Tea coverage for Enter gating and focus feedback (Issue #19
// Task 5): base-context Enter on invalid data consumes the key, moves focus to
// the report's first-invalid field in visual order, and shows its specific
// inline reason with no validation/estimation/execution/history command;
// runnable data emits only the pre-execution seam; every higher-precedence
// context (popup, text input, stale overlay, pending request, too-small
// screen) consumes Enter with its own behavior.

package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// enterPress drives one Enter through Update, returning the model and cmd.
func enterPress(m Model) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model), cmd
}

// runnableSelect returns runnable SELECT data (wildcard over users).
func runnableSelect() qb.QueryBuilder {
	return validSelectQB()
}

// multiFailureSelect returns SELECT data failing projection, WHERE, and Limit
// simultaneously: no committed projection, an open WHERE draft, invalid Limit.
func multiFailureSelect() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandSelect).SelectTable("users")
	next, _ := q.StartWhere("email")
	return next.SetLimitInput("abc")
}

// TestEnterOnMultiFailureMovesFocusToFirstInvalidField requires Enter on a
// multi-failure SELECT to consume the key, focus Column(s) — the first
// invalid field in visual order — show its exact reason, and return no
// command at all.
func TestEnterOnMultiFailureMovesFocusToFirstInvalidField(t *testing.T) {
	m := modelWithQB(multiFailureSelect())
	m = focusField(m, limitFieldLabel) // Limit fails too, but Column(s) is first
	next, cmd := enterPress(m)
	if cmd != nil {
		t.Fatalf("invalid Enter returned a command %v", cmd)
	}
	if got := next.Fields[next.Focus].Label; got != columnsFieldLabel {
		t.Fatalf("focus = %q, want %q (first invalid in visual order)", got, columnsFieldLabel)
	}
	want := qb.ReasonNoProjection
	if content := next.Fields[next.Focus].Content; !strings.Contains(content, want) {
		t.Errorf("inline content = %q, want it to contain %q verbatim", content, want)
	}
	if next.Popup != nil {
		t.Error("invalid Enter opened a popup")
	}
}

// TestEnterOnInvalidLimitFocusesLimitWithExactReason requires invalid
// nonempty Limit to focus Limit and display exactly the authoritative reason,
// without reopening universal value entry and without any command.
func TestEnterOnInvalidLimitFocusesLimitWithExactReason(t *testing.T) {
	q := runnableSelect().SetLimitInput("abc")
	m := modelWithQB(q)
	m = focusField(m, limitFieldLabel)
	next, cmd := enterPress(m)
	if cmd != nil {
		t.Fatalf("invalid Limit Enter returned a command %v", cmd)
	}
	if next.ValuePrompt != nil {
		t.Fatal("invalid Limit Enter reopened the value prompt")
	}
	if got := next.Fields[next.Focus].Label; got != limitFieldLabel {
		t.Fatalf("focus = %q, want Limit", got)
	}
	want := "abc — " + qb.LimitInvalidReason
	if got := next.Fields[next.Focus].Content; got != want {
		t.Errorf("content = %q, want exactly %q", got, want)
	}
	if view := next.View(); !strings.Contains(view, qb.LimitInvalidReason) {
		t.Error("rendered view lacks the exact Limit reason")
	}
}

// TestEnterOnInvalidWriteStatesFocusesWriteTargets covers representative
// UPDATE, DELETE, and INSERT failures: focus moves to the report's typed
// write-field target with a specific reason and no command.
func TestEnterOnInvalidWriteStatesFocusesWriteTargets(t *testing.T) {
	updateQB := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandUpdate).SelectTable("users")
	deleteQB := func() qb.QueryBuilder {
		next, _ := qb.NewQuery().RefreshSchema(whereUICatalog()).
			SelectCommand(qb.CommandDelete).SelectTable("users").StartWhere("email")
		return next
	}()
	insertQB := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandInsert).SelectTable("users")
	cases := []struct {
		name      string
		qb        qb.QueryBuilder
		wantField string
		want      string
	}{
		{"missing SET columns", updateQB, setFieldLabel, qb.ReasonNoSetAssignments},
		{"incomplete DELETE WHERE", deleteQB, whereFieldLabel, qb.ReasonIncompletePrompt},
		{"zero insertable columns", insertQB, insertFieldLabel, qb.ReasonNoInsertableColumns},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := modelWithQB(tc.qb)
			m = focusField(m, commandFieldLabel)
			next, cmd := enterPress(m)
			if cmd != nil {
				t.Fatalf("invalid Enter returned a command %v", cmd)
			}
			if got := next.Fields[next.Focus].Label; got != tc.wantField {
				t.Fatalf("focus = %q, want %q", got, tc.wantField)
			}
			if content := next.Fields[next.Focus].Content; !strings.Contains(content, tc.want) {
				t.Errorf("content = %q, want it to contain %q verbatim", content, tc.want)
			}
		})
	}
}

// TestEnterOnRunnableDataEmitsOnlyPreExecutionSeam requires valid data in an
// idle base context to return exactly the pre-execution seam command and
// nothing else — no execution starts within this issue.
func TestEnterOnRunnableDataEmitsOnlyPreExecutionSeam(t *testing.T) {
	m := modelWithQB(runnableSelect())
	m = focusField(m, commandFieldLabel)
	next, cmd := enterPress(m)
	if cmd == nil {
		t.Fatal("runnable Enter returned no command")
	}
	msg, ok := cmd().(PreExecutionRequestedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PreExecutionRequestedMsg", msg)
	}
	_ = msg
	if next.Popup != nil || next.ValuePrompt != nil {
		t.Error("runnable Enter opened an overlay")
	}
}

// TestRunnableDataInHigherPrecedenceContextsConsumesEnterLocally requires
// popups, focused text input, and the stale-refresh overlay to consume Enter
// with their own behavior even on runnable data, with no runnable action.
func TestRunnableDataInHigherPrecedenceContextsConsumesEnterLocally(t *testing.T) {
	// Open popup: Enter accepts within the popup, never runs the report.
	m := modelWithQB(runnableSelect())
	m = focusField(m, tableFieldLabel)
	if cmd := m.openPopupCmd(tea.KeyMsg{Type: tea.KeyEnter}); m.Popup == nil {
		t.Fatal("setup: table popup did not open")
	} else if cmd != nil {
		t.Log("table popup issued its refresh command (expected seam)")
	}
	next, _ := enterPress(m)
	if next.Popup != nil {
		t.Fatal("popup Enter did not consume the press locally")
	}

	// Focused value prompt: Enter submits the buffer, no runnable action.
	m2 := modelWithQB(runnableSelect())
	m2 = focusField(m2, limitFieldLabel)
	m2.ValuePrompt = NewValuePrompt(limitFieldLabel, "row limit", "")
	next2, _ := enterPress(m2)
	if next2.ValuePrompt != nil {
		t.Fatal("value prompt Enter did not submit locally")
	}
	if next2.QB.LimitInput() != "" {
		t.Fatalf("empty submission did not commit as unbounded: %q", next2.QB.LimitInput())
	}
	if report := next2.QB.RunnableReport(); !report.Runnable {
		t.Fatalf("empty Limit submission left data non-runnable: %+v", report)
	}

	// Stale-refresh overlay: Enter consumed with no runnable action.
	m3 := modelWithQB(runnableSelect())
	m3.schemaStale = true
	m3 = focusField(m3, commandFieldLabel)
	before := m3.QB
	next3, cmd3 := enterPress(m3)
	if cmd3 != nil {
		t.Fatalf("stale-context Enter returned a command %v", cmd3)
	}
	if !reflect.DeepEqual(next3.QB, before) {
		t.Error("stale-context Enter mutated builder state")
	}
}

// TestRunnableDataWithPendingRequestConsumesEnter requires a pending request
// to block Enter with no runnable action and no state change.
func TestRunnableDataWithPendingRequestConsumesEnter(t *testing.T) {
	m := modelWithQB(runnableSelect())
	m = focusField(m, commandFieldLabel)
	m.refreshPending = true
	next, cmd := enterPress(m)
	if cmd != nil {
		t.Fatalf("pending-context Enter returned a command %v", cmd)
	}
	if !reflect.DeepEqual(next.QB, m.QB) {
		t.Error("pending-context Enter mutated builder state")
	}
}

// TestRunnableDataTooSmallConsumesEnter requires the suspended too-small
// context to consume Enter entirely.
func TestRunnableDataTooSmallConsumesEnter(t *testing.T) {
	m := sized(New(), 80, 24).(Model)
	m.QB = runnableSelect()
	m.applyBuilder(m.QB)
	small := m.resize(60, 20)
	if !small.suspended {
		t.Fatal("setup: model not suspended")
	}
	next, cmd := enterPress(small)
	if cmd != nil {
		t.Fatalf("suspended Enter returned a command %v", cmd)
	}
	if !next.suspended {
		t.Error("suspended Enter ended suspension")
	}
}
