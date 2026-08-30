// Scripted Bubble Tea coverage for the destructive preparation modal and the
// independent matching-target estimate (Issue #40 Tasks 3–4): opening from
// runnable validated UPDATE/DELETE states, continuously visible operation,
// table, canonical rendered SQL, and the prominent all-rows warning for
// no-WHERE statements only; exact `Estimating matching target rows…` while
// pending with confirmation disabled; identity-guarded success/failure
// retention; dismissal/cancellation restoring the builder; and no query or
// result history anywhere before an actual confirmed write.

package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// prepFakeEstimator records every estimate request the model dispatches and
// every cancellation it observes. The command runs only when the test
// executes it, so a fake that never settles leaves the modal pending.
type prepFakeEstimator struct {
	mu       sync.Mutex
	requests int
	sql      string
	params   []any
	cancels  int
	result   EstimateResult
}

func (f *prepFakeEstimator) ExecuteEstimate(ctx context.Context, sql string, params []any) EstimateResult {
	f.mu.Lock()
	f.requests++
	f.sql = sql
	f.params = params
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		f.mu.Lock()
		f.cancels++
		f.mu.Unlock()
		return EstimateResult{Cancelled: true}
	default:
		return f.result
	}
}

func (f *prepFakeEstimator) snapshot() (int, string, []any, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.sql, append([]any(nil), f.params...), f.cancels
}

func (f *prepFakeEstimator) cancelCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancels
}

// prepCatalog returns a small write-eligible users catalog for the
// destructive preparation flows.
func prepCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 40,
		Objects: []*schema.Object{{
			Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
			Columns: []schema.Column{{Name: "id", DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1}, {Name: "email", DeclaredType: "TEXT", Insertable: true}},
		}},
	}
}

// prepUpdateQB returns a runnable UPDATE over users with one submitted Value
// assignment, qualified or not.
func prepUpdateQB(qualified bool) qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(prepCatalog()).
		SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, ok := q.AcceptSetColumn("email")
	if !ok {
		panic("setup: AcceptSetColumn failed")
	}
	q, ok = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	if !ok {
		panic("setup: ChooseSetAssignment failed")
	}
	q, ok = q.SubmitSetValue("email", "new")
	if !ok {
		panic("setup: SubmitSetValue failed")
	}
	if qualified {
		q = completeWhereQBValue(q, "id", "5")
	}
	return q
}

// prepDeleteQB returns a runnable DELETE over users, qualified or not.
func prepDeleteQB(qualified bool) qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(prepCatalog()).
		SelectCommand(qb.CommandDelete).SelectTable("users")
	if qualified {
		q = completeWhereQBValue(q, "id", "5")
	}
	return q
}

// completeWhereQBValue commits an `=` predicate over the named column
// through the guided transitions (shared by the preparation flows).
func completeWhereQBValue(q qb.QueryBuilder, column, text string) qb.QueryBuilder {
	next, ok := q.StartWhere(column)
	if !ok {
		panic("setup: StartWhere failed")
	}
	draft, ok := next.WhereDraft().ChooseOperator(qb.OpEq)
	if !ok {
		panic("setup: ChooseOperator failed")
	}
	next = next.ApplyWhereDraft(draft)
	draft, ok = draft.SubmitValue(text)
	if !ok {
		panic("setup: SubmitValue failed")
	}
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		panic("setup: CommitWhereDraft failed")
	}
	return next
}

// prepModel returns a supported-size model whose builder is q, focused on the
// Set field, with the given estimator wired and an empty history store.
func prepModel(q qb.QueryBuilder, est *prepFakeEstimator) Model {
	m := modelWithQB(q)
	m = focusField(m, commandFieldLabel) // no opener: Enter reaches the runnable route
	m.Estimator = est.ExecuteEstimate
	m.History = history.NewStore()
	return m
}

// openPreparation drives one runnable model through Enter, the pre-execution
// seam, and a settled unchanged validation. The returned model has the
// preparation modal open and pending; the returned command (if any) is the
// estimate request still awaiting settlement.
func openPreparation(t *testing.T, q qb.QueryBuilder, est *prepFakeEstimator) (Model, tea.Cmd) {
	t.Helper()
	m := prepModel(q, est)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on runnable destructive state emitted no command")
	}
	msg := cmd()
	if _, ok := msg.(PreExecutionRequestedMsg); !ok {
		t.Fatalf("Enter command produced %T, want PreExecutionRequestedMsg", msg)
	}
	next, cmd = next.Update(msg)
	if cmd != nil {
		t.Fatalf("pre-execution seam returned unexpected command %v", cmd)
	}
	nm := next.(Model)
	if nm.validationAttempt != 1 {
		t.Fatalf("validationAttempt = %d, want 1", nm.validationAttempt)
	}
	next, cmd = nm.Update(ValidationSettledMsg{Preparation: 1,
		Result: schema.Revalidation{Status: schema.RevalidateUnchanged}})
	return next.(Model), cmd
}

// runEstimate executes the estimate command so the fake settles the pending
// request, then feeds the settled message back through Update.
func runEstimate(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no estimate command to run")
	}
	msg := cmd()
	settled, ok := msg.(EstimateSettledMsg)
	if !ok {
		t.Fatalf("estimate command produced %T, want EstimateSettledMsg", msg)
	}
	next, _ := m.Update(settled)
	return next.(Model)
}

func TestDestructivePreparationOpensFromValidatedWrites(t *testing.T) {
	tests := []struct {
		name       string
		qb         qb.QueryBuilder
		operation  string
		table      string
		wantSQL    string
		wantParams []any
	}{
		{
			name:       "unqualified update",
			qb:         prepUpdateQB(false),
			operation:  "UPDATE",
			table:      "users",
			wantSQL:    `UPDATE "users" SET "email" = 'new'`,
			wantParams: nil,
		},
		{
			name:       "qualified update",
			qb:         prepUpdateQB(true),
			operation:  "UPDATE",
			table:      "users",
			wantSQL:    `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`,
			wantParams: []any{int64(5)},
		},
		{
			name:       "unqualified delete",
			qb:         prepDeleteQB(false),
			operation:  "DELETE",
			table:      "users",
			wantSQL:    `DELETE FROM "users"`,
			wantParams: nil,
		},
		{
			name:       "qualified delete",
			qb:         prepDeleteQB(true),
			operation:  "DELETE",
			table:      "users",
			wantSQL:    `DELETE FROM "users" WHERE "id" = 5`,
			wantParams: []any{int64(5)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := &prepFakeEstimator{result: EstimateResult{Total: 2}}
			m, cmd := openPreparation(t, tt.qb, est)

			if !m.prepOpen {
				t.Fatal("preparation modal did not open after settled validation")
			}
			if m.prepAttempt != 1 {
				t.Errorf("preparation identity = %d, want 1", m.prepAttempt)
			}
			if m.prepOperation != tt.operation || m.prepTable != tt.table {
				t.Errorf("modal operation/table = %q/%q, want %q/%q", m.prepOperation, m.prepTable, tt.operation, tt.table)
			}
			if m.prepSQL != tt.wantSQL {
				t.Errorf("modal SQL = %q, want %q", m.prepSQL, tt.wantSQL)
			}
			if !m.prepPending {
				t.Error("modal is not pending immediately after opening")
			}
			requests, sql, params, _ := est.snapshot()
			if requests != 0 {
				t.Fatalf("estimate dispatched before its command ran: %d requests", requests)
			}
			if cmd == nil {
				t.Fatal("open preparation returned no estimate command")
			}
			cmd() // run the request against the fake
			requests, sql, params, _ = est.snapshot()
			if requests != 1 {
				t.Fatalf("estimate requests = %d, want exactly 1", requests)
			}
			wantEstimate := "SELECT COUNT(*) FROM \"users\""
			if tt.wantParams != nil {
				wantEstimate += ` WHERE "id" = ?`
			}
			if sql != wantEstimate {
				t.Errorf("estimate SQL = %q, want %q", sql, wantEstimate)
			}
			if tt.wantParams == nil && params != nil {
				t.Errorf("estimate params = %#v, want none", params)
			}
			if tt.wantParams != nil && fmt.Sprint(params) != fmt.Sprint(tt.wantParams) {
				t.Errorf("estimate params = %#v, want %#v", params, tt.wantParams)
			}
		})
	}
}

func TestDestructivePreparationEstimateExcludesSetValues(t *testing.T) {
	est := &prepFakeEstimator{}
	m, cmd := openPreparation(t, prepUpdateQB(true), est)
	cmd()
	_, sql, params, _ := est.snapshot()
	if strings.Contains(sql, "SET") {
		t.Errorf("estimate SQL = %q carries SET fragments", sql)
	}
	if fmt.Sprint(params) != fmt.Sprint([]any{int64(5)}) {
		t.Errorf("estimate params = %#v, want only the WHERE value", params)
	}
	if !m.prepOpen {
		t.Fatal("modal closed while estimate was dispatched")
	}
}

func TestDestructivePreparationShowsOperationTableSQLAndStatus(t *testing.T) {
	tests := []struct {
		name        string
		qb          qb.QueryBuilder
		wantSQL     string
		wantWarning bool
	}{
		{"qualified update", prepUpdateQB(true), `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`, false},
		{"unqualified update", prepUpdateQB(false), `UPDATE "users" SET "email" = 'new'`, true},
		{"qualified delete", prepDeleteQB(true), `DELETE FROM "users" WHERE "id" = 5`, false},
		{"unqualified delete", prepDeleteQB(false), `DELETE FROM "users"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := &prepFakeEstimator{}
			m, cmd := openPreparation(t, tt.qb, est)
			view := m.View()

			if !strings.Contains(view, tt.qb.Command().String()) || !strings.Contains(view, "users") {
				t.Errorf("view missing operation/table:\n%s", view)
			}
			if !strings.Contains(view, tt.wantSQL) {
				t.Errorf("view missing canonical SQL %q:\n%s", tt.wantSQL, view)
			}
			if !strings.Contains(view, "Estimating matching target rows…") {
				t.Errorf("view missing exact pending status:\n%s", view)
			}
			if strings.Contains(view, "Estimated matching target rows:") {
				t.Errorf("view claims a settled estimate while pending:\n%s", view)
			}
			if tt.wantWarning {
				if !strings.Contains(view, "WARNING") || !strings.Contains(view, "every row") {
					t.Errorf("no-WHERE statement lacks prominent all-rows warning:\n%s", view)
				}
			} else if strings.Contains(view, "every row") {
				t.Errorf("qualified statement shows a false all-rows warning:\n%s", view)
			}

			// Unrelated redraw traffic keeps every required line visible.
			next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			view = next.View()
			if !strings.Contains(view, tt.wantSQL) || !strings.Contains(view, "Estimating matching target rows…") {
				t.Errorf("resize lost SQL or pending status:\n%s", view)
			}
			if tt.wantWarning && !strings.Contains(view, "every row") {
				t.Errorf("resize lost the all-rows warning:\n%s", view)
			}
			cmd() // dispatch; still pending because the fake's message never enters Update
			view = m.View()
			if !strings.Contains(view, "Estimating matching target rows…") {
				t.Errorf("pending status vanished while the request was outstanding:\n%s", view)
			}
		})
	}
}

func TestDestructivePreparationBlocksConfirmationAndHistoryWhilePending(t *testing.T) {
	est := &prepFakeEstimator{}
	m, cmd := openPreparation(t, prepUpdateQB(false), est)

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune{'y'}},
	} {
		next, c := m.Update(key)
		if c != nil {
			t.Errorf("pending %v returned a command %v", key, c)
		}
		nm := next.(Model)
		if !nm.prepOpen || !nm.prepPending {
			t.Errorf("pending %v mutated the modal (open=%t pending=%t)", key, nm.prepOpen, nm.prepPending)
		}
		if nm.History.Len() != 0 {
			t.Errorf("pending %v appended query history", key)
		}
		if nm.ResultHistory.Len() != 0 {
			t.Errorf("pending %v appended result history", key)
		}
	}
	cmd()
}

func TestDestructivePreparationRetainsSuccessAndFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		est := &prepFakeEstimator{result: EstimateResult{Total: 7}}
		m, cmd := openPreparation(t, prepUpdateQB(false), est)
		m = runEstimate(t, m, cmd)

		if m.prepPending {
			t.Error("modal still pending after settled estimate")
		}
		if !m.prepOpen {
			t.Fatal("modal closed after a settled successful estimate")
		}
		view := m.View()
		if !strings.Contains(view, "Estimated matching target rows: 7") {
			t.Errorf("view missing retained estimate:\n%s", view)
		}
		if !strings.Contains(view, `UPDATE "users" SET "email" = 'new'`) || !strings.Contains(view, "every row") {
			t.Errorf("success lost SQL or warning:\n%s", view)
		}
		next, c := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if c == nil {
			t.Error("settled confirmation emitted no write command")
		} else if msg := c(); func() bool {
			_, ok := msg.(WriteConfirmedMsg)
			return !ok
		}() {
			t.Errorf("settled confirmation produced %T, want WriteConfirmedMsg", msg)
		}
		if next.(Model).History.Len() != 0 {
			t.Error("confirmation appended query history before execution start")
		}
	})

	t.Run("failure", func(t *testing.T) {
		est := &prepFakeEstimator{result: EstimateResult{Err: errors.New("database is locked")}}
		m, cmd := openPreparation(t, prepUpdateQB(true), est)
		m = runEstimate(t, m, cmd)

		if m.prepPending {
			t.Error("modal still pending after settled estimate")
		}
		view := m.View()
		if !strings.Contains(view, "Estimate failed: database is locked") {
			t.Errorf("view missing retained failure:\n%s", view)
		}
		if !strings.Contains(view, `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`) || strings.Contains(view, "every row") {
			t.Errorf("failure lost SQL or showed a false warning:\n%s", view)
		}
		next, c := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if c == nil {
			t.Error("failed-estimate confirmation emitted no write command")
		} else if msg := c(); func() bool {
			_, ok := msg.(WriteConfirmedMsg)
			return !ok
		}() {
			t.Errorf("failed-estimate confirmation produced %T, want WriteConfirmedMsg", msg)
		}
		if next.(Model).History.Len() != 0 {
			t.Error("confirmation appended query history before execution start")
		}
	})
}

func TestDestructivePreparationRejectsStaleEstimateResponses(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m, cmd := openPreparation(t, prepUpdateQB(false), est)
	cmd()

	// A response carrying a superseded identity mutates nothing.
	next, _ := m.Update(EstimateSettledMsg{Preparation: m.prepAttempt + 1, Result: est.result})
	nm := next.(Model)
	if !nm.prepPending {
		t.Fatal("stale success mutated the pending modal")
	}
	if strings.Contains(nm.View(), "Estimated matching target rows") {
		t.Fatal("stale success was retained")
	}

	// The current response still settles afterwards.
	m = runEstimate(t, m, cmd)
	if m.prepPending || !strings.Contains(m.View(), "Estimated matching target rows: 3") {
		t.Fatal("current estimate response was not retained after a stale one")
	}
}

func TestDestructivePreparationEscDismissesWithCancellation(t *testing.T) {
	est := &prepFakeEstimator{}
	m, cmd := openPreparation(t, prepUpdateQB(true), est)
	opener := m.Focus

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(Model)
	if nm.prepOpen {
		t.Fatal("Esc left the modal open")
	}
	if nm.prepPending {
		t.Fatal("Esc left the estimate pending")
	}
	if nm.Focus != opener {
		t.Errorf("Esc focus = %d, want opener %d", nm.Focus, opener)
	}
	if nm.History.Len() != 0 || nm.ResultHistory.Len() != 0 {
		t.Fatal("dismissal appended history")
	}
	cmd() // cancellation observed by the fake
	if est.cancelCount() == 0 {
		t.Error("dismissal of a pending estimate did not cancel the request")
	}
}

func TestDestructivePreparationCancelThenSettleDismisses(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 4}}
	m, cmd := openPreparation(t, prepDeleteQB(false), est)

	// Ctrl+W requests cancellation: exact `cancelling…` until settlement.
	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if cancelCmd == nil {
		t.Fatal("Ctrl+W dispatched no cancellation command")
	}
	nm := next.(Model)
	if !nm.prepCancelling || !strings.Contains(nm.View(), "cancelling…") {
		t.Fatal("Ctrl+W did not show exact cancelling status")
	}

	// Settlement of the cancelled request dismisses without history.
	msg := cancelCmd()
	if _, ok := msg.(CancelEstimateMsg); !ok {
		t.Fatalf("cancellation command produced %T", msg)
	}
	m = runEstimate(t, nm, cmd)
	if m.prepOpen || m.prepPending {
		t.Fatal("cancelled estimate did not dismiss preparation")
	}
	if m.History.Len() != 0 || m.ResultHistory.Len() != 0 {
		t.Fatal("cancellation appended history")
	}
}

func TestDestructivePreparationNeverAppendsHistory(t *testing.T) {
	for _, tc := range []struct {
		name  string
		qb    qb.QueryBuilder
		res   EstimateResult
		stage func(t *testing.T, m Model, cmd tea.Cmd) Model
	}{
		{"open", prepUpdateQB(false), EstimateResult{}, func(t *testing.T, m Model, cmd tea.Cmd) Model { return m }},
		{"success", prepUpdateQB(false), EstimateResult{Total: 1}, func(t *testing.T, m Model, cmd tea.Cmd) Model { return runEstimate(t, m, cmd) }},
		{"failure", prepUpdateQB(false), EstimateResult{Err: errors.New("boom")}, func(t *testing.T, m Model, cmd tea.Cmd) Model { return runEstimate(t, m, cmd) }},
		{"dismiss", prepUpdateQB(false), EstimateResult{}, func(t *testing.T, m Model, cmd tea.Cmd) Model {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			return next.(Model)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			est := &prepFakeEstimator{result: tc.res}
			m, cmd := openPreparation(t, tc.qb, est)
			m = tc.stage(t, m, cmd)
			if m.History.Len() != 0 || m.ResultHistory.Len() != 0 {
				t.Errorf("%s stage appended query or result history", tc.name)
			}
		})
	}
}
