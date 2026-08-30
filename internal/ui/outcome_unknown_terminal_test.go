// Barrier-controlled outcome-unknown write settlement coverage for Issue
// #45, per the Writes and commit boundary decisions in Notes/PRD-sqloid.md.
// Unresolved commit/rollback resolution is held until all transaction and
// driver work has ended (the settlement message arrives only after the phase
// channel closed); only then is exactly one immutable non-tabular
// outcome-unknown entry appended, selected as the newest result, and the
// outcome-unknown terminal view entered. No entry or terminal state is
// created while work remains pending, duplicate/late settlement messages and
// stale identities are inert, and the entry never claims the database was
// committed, rolled back, or untouched.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// settledUnresolvedWrite drives one confirmed UPDATE/DELETE (or dispatched
// INSERT) write through its phases while withholding settlement, asserts the
// pending state created no entry and no terminal state, then delivers the
// unresolved settlement and returns the settled model plus the delivered
// message identity.
func settledUnresolvedWrite(t *testing.T, m Model, begin tea.Cmd, fake *writeFakeExecutor, result connection.WriteResult) Model {
	t.Helper()
	nm := dispatchWriteBatch(t, m, begin, false)
	if !nm.writePending {
		t.Fatal("setup: write did not stay pending while settlement was withheld")
	}
	if nm.terminalState != TerminalNone {
		t.Fatalf("terminal state %v entered while transaction/driver work remained pending", nm.terminalState)
	}
	if nm.ResultHistory.Len() != 0 {
		t.Fatalf("result entries = %d while work remained pending, want none", nm.ResultHistory.Len())
	}
	settled, _ := nm.Update(WriteSettledMsg{Execution: fake.execution, Result: result})
	return settled.(Model)
}

// updateUnresolvedUpdate covers a confirmed UPDATE whose commit phase began
// but did not resolve: exactly one outcome-unknown entry is appended and
// initially selected, and the terminal state is entered.
func updateUnresolvedUpdate(t *testing.T) (Model, *writeFakeExecutor) {
	t.Helper()
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
	}
	m.Write = fake.Write
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmation produced no command")
	}
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("confirmation command produced %T, want WriteConfirmedMsg", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	if cmd == nil {
		t.Fatal("confirmation delivery dispatched no write command")
	}
	return settledUnresolvedWrite(t, next.(Model), cmd, fake, connection.WriteResult{
		Outcome:      connection.WriteFailed,
		Err:          errors.New("disk I/O error"),
		RowsAffected: 3,
	}), fake
}

func TestUnresolvedCommitSettlesIntoTerminalEntry(t *testing.T) {
	nm, fake := updateUnresolvedUpdate(t)

	if nm.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", nm.terminalState)
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one outcome-unknown entry", nm.ResultHistory.Len())
	}
	e, ok := nm.ResultHistory.Newest()
	if !ok || e.ID != nm.resultHistoryCursorID {
		t.Fatalf("selected cursor = %v, want the newest appended entry %v (ok=%v)", nm.resultHistoryCursorID, e.ID, ok)
	}
	if e.Kind != history.KindOutcomeUnknown {
		t.Errorf("entry kind = %v, want outcome-unknown", e.Kind)
	}
	if e.ExecutionID != fake.execution || fake.execution == 0 {
		t.Errorf("entry execution = %d, want the executed %d", e.ExecutionID, fake.execution)
	}
	if e.Operation != "UPDATE" || e.Table != "users" {
		t.Errorf("entry operation/table = %q/%q, want UPDATE/users", e.Operation, e.Table)
	}
	if e.Phase != history.UnknownPhaseCommit {
		t.Errorf("entry phase = %v, want commit", e.Phase)
	}
	if e.SQL != `UPDATE "users" SET "email" = 'new' WHERE "id" = 5` {
		t.Errorf("entry SQL = %q, want the exact executed statement", e.SQL)
	}
	if !strings.Contains(e.Summary, "disk I/O error") {
		t.Errorf("summary %q lost the driver error", e.Summary)
	}
	if !strings.Contains(e.Summary, "does not prove persistence") {
		t.Errorf("summary %q lacks the non-proving RowsAffected wording", e.Summary)
	}
	for _, forbidden := range []string{"committed", "untouched", "rollback confirmed"} {
		if strings.Contains(e.Summary, forbidden) {
			t.Errorf("summary %q claims %q the resolution never proved", e.Summary, forbidden)
		}
	}
	if nm.writePending {
		t.Error("settled unresolved write left pending state behind")
	}
}

func TestUnresolvedRollbackSettlesIntoTerminalEntry(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
	}
	m.Write = fake.Write
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmation produced no command")
	}
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("confirmation command produced %T, want WriteConfirmedMsg", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	nm := settledUnresolvedWrite(t, next.(Model), cmd, fake, connection.WriteResult{
		Outcome: connection.WriteCancelled,
		Err:     errors.New("rollback failed: locked"),
	})

	if nm.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", nm.terminalState)
	}
	e, ok := nm.ResultHistory.Newest()
	if !ok {
		t.Fatal("no outcome-unknown entry was appended")
	}
	if e.Kind != history.KindOutcomeUnknown || e.Operation != "DELETE" || e.Table != "users" {
		t.Errorf("entry = %v op %q table %q, want outcome-unknown DELETE/users", e.Kind, e.Operation, e.Table)
	}
	if e.Phase != history.UnknownPhaseRollback {
		t.Errorf("entry phase = %v, want rollback", e.Phase)
	}
	if e.SQL != `DELETE FROM "users" WHERE "id" = 5` {
		t.Errorf("entry SQL = %q, want the exact executed statement", e.SQL)
	}
	if !strings.Contains(e.Summary, "rollback did not resolve") {
		t.Errorf("summary %q lacks the unresolved-rollback phase wording", e.Summary)
	}
	if !strings.Contains(e.Summary, "rollback failed: locked") {
		t.Errorf("summary %q lost the driver error", e.Summary)
	}
}

func TestUnresolvedInsertSettlesIntoTerminalEntry(t *testing.T) {
	m := insertUIModel(map[string]qb.InsertChoice{
		"id":    qb.InsertChoiceOmit,
		"email": qb.InsertChoiceValue,
		"note":  qb.InsertChoiceNull,
	}, map[string]string{"email": "a@b"})
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
	}
	m.Write = fake.Write
	next, cmd := m.Update(ExecutionStartedMsg{})
	if cmd == nil {
		t.Fatal("INSERT dispatch produced no command")
	}
	nm := settledUnresolvedWrite(t, next.(Model), cmd, fake, connection.WriteResult{
		Outcome:      connection.WriteFailed,
		Err:          errors.New("commit did not resolve"),
		RowsAffected: 1,
	})

	if nm.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", nm.terminalState)
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e, ok := nm.ResultHistory.Newest()
	if !ok || e.ID != nm.resultHistoryCursorID {
		t.Fatalf("selected cursor = %v, want the newest appended entry (ok=%v)", nm.resultHistoryCursorID, ok)
	}
	if e.Kind != history.KindOutcomeUnknown || e.Operation != "INSERT" || e.Table != "users" {
		t.Errorf("entry = %v op %q table %q, want outcome-unknown INSERT/users", e.Kind, e.Operation, e.Table)
	}
	if e.Phase != history.UnknownPhaseCommit {
		t.Errorf("entry phase = %v, want commit", e.Phase)
	}
	if e.SQL != `INSERT INTO "users" ("email", "note") VALUES (?, NULL)` {
		t.Errorf("entry SQL = %q, want the exact executed INSERT", e.SQL)
	}
	if !strings.Contains(e.Summary, "does not prove persistence") {
		t.Errorf("summary %q lacks the non-proving RowsAffected wording", e.Summary)
	}
	for _, forbidden := range []string{"committed", "untouched", "rows added"} {
		if strings.Contains(e.Summary, forbidden) {
			t.Errorf("summary %q claims %q the resolution never proved", e.Summary, forbidden)
		}
	}
}

func TestUnresolvedSettlementIsIdempotentAndStaleProof(t *testing.T) {
	nm, fake := updateUnresolvedUpdate(t)
	execution := fake.execution

	// Duplicate and late settlement messages for the settled execution append
	// nothing further; a stale identity mutates nothing.
	dup, _ := nm.Update(WriteSettledMsg{Execution: execution, Result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 99}})
	stale, _ := dup.(Model).Update(WriteSettledMsg{Execution: execution - 1, Result: connection.WriteResult{Outcome: connection.WriteFailed, Err: errors.New("late")}})

	final := stale.(Model)
	if final.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one (duplicates appended nothing)", final.ResultHistory.Len())
	}
	e, _ := final.ResultHistory.Newest()
	if e.ExecutionID != execution || e.Kind != history.KindOutcomeUnknown {
		t.Errorf("retained entry = execution %d kind %v, want %d outcome-unknown", e.ExecutionID, e.Kind, execution)
	}
	if final.terminalState != TerminalOutcomeUnknown {
		t.Errorf("terminal state = %v, want TerminalOutcomeUnknown preserved", final.terminalState)
	}
	if final.ResultHistory.Entries()[0].Summary != e.Summary {
		t.Error("duplicate settlement mutated the retained summary")
	}
}
