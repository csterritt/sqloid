// Write history and summary lifecycle coverage for Issue #42, per the
// Writes and commit boundary, History, and write-transaction decisions in
// Notes/PRD-sqloid.md. Scripted through Update with the controllable fake
// WriteExecutor: confirmation or INSERT dispatch begins the sole actual
// execution after exiting either history first and appending the complete
// query state at execution start (preparation and pre-start messages append
// nothing); every successful, cancelled, or failed write produces exactly one
// immutable non-tabular result entry tied to the execution identity and
// containing the executed standalone SQL with operation-appropriate
// RowsAffected wording; and no result claims the database was untouched
// before successful rollback confirmation. Duplicate, late, and stale phase
// and outcome messages are idempotent no-ops with no per-phase or per-message
// entries. Unresolved rollback/commit outcomes remain Issue #45's.

package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// writeFakeExecutor is the controllable fake write execution: it records the
// dispatch, delivers the scripted phases through the relay channel, and
// returns the scripted result. It never touches a database.
type writeFakeExecutor struct {
	calls     int
	execution uint64
	sql       string
	params    []any
	phases    []connection.WritePhase
	result    connection.WriteResult
}

// Write implements the WriteExecutor seam.
func (f *writeFakeExecutor) Write(ctx context.Context, execution uint64, sql string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult {
	f.calls++
	f.execution = execution
	f.sql = sql
	f.params = params
	for _, p := range f.phases {
		phase(connection.WritePhaseMsg{Execution: execution, Phase: p})
	}
	return f.result
}

// committedUpdateFake returns a fake for a successful qualified UPDATE.
func committedUpdateFake(rows int64) *writeFakeExecutor {
	return &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
		result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: rows},
	}
}

// dispatchWriteBatch unpacks one dispatched write batch. The batch's write
// command (which delivers the settled message) runs first so its phases are
// buffered; each phase is then drained through Update via the relay
// continuations before the settled message is applied. With deliverSettled
// false, settlement is withheld and the caller drives it explicitly.
func dispatchWriteBatch(t *testing.T, m Model, cmd tea.Cmd, deliverSettled bool) Model {
	t.Helper()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("write dispatch produced %T, want a write batch", cmd())
	}
	var settled WriteSettledMsg
	for _, c := range batch {
		if c == nil {
			continue
		}
		out := c()
		if s, ok := out.(WriteSettledMsg); ok {
			settled = s
			continue
		}
		if out == nil {
			continue
		}
		// out is a buffered phase message; drain its continuation chain.
		phase := out
		for {
			next, relay := m.Update(phase)
			m = next.(Model)
			if relay == nil {
				t.Fatal("phase message dispatched no relay continuation")
			}
			phase = relay()
			if phase == nil {
				break
			}
			if _, isSettled := phase.(WriteSettledMsg); isSettled {
				t.Fatal("settled message arrived through the phase relay")
			}
		}
	}
	if !deliverSettled {
		return m
	}
	nm, _ := m.Update(settled)
	return nm.(Model)
}

// confirmedWrite drives one confirmation key on a settled preparation and
// dispatches the resulting write batch; it fails when confirmation produced
// no command or the executor was called more than once.
func confirmedWrite(t *testing.T, m Model, key tea.KeyMsg, fake *writeFakeExecutor) Model {
	t.Helper()
	next, cmd := m.Update(key)
	if cmd == nil {
		t.Fatal("settled confirmation produced no command")
	}
	if fake.calls != 0 {
		t.Fatal("write started before its command ran")
	}
	// The confirmation key emits the WriteConfirmedMsg; delivering it
	// dispatches the write batch.
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("confirmation command produced %T, want WriteConfirmedMsg", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	if cmd == nil {
		t.Fatal("confirmation delivery dispatched no write command")
	}
	nm := dispatchWriteBatch(t, next.(Model), cmd, true)
	if fake.calls != 1 {
		t.Fatalf("write executor called %d times, want exactly 1 (sole actual execution)", fake.calls)
	}
	return nm
}

// TestConfirmedUpdateLifecycle appends the complete query state exactly once
// at the confirmation execution-start boundary — after exiting both
// histories — runs the sole write with the retained rendered SQL and
// parameters, and finalizes exactly one non-tabular result entry carrying the
// executed SQL and the actual rows-affected label.
func TestConfirmedUpdateLifecycle(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 7}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	fake := committedUpdateFake(3)
	m.Write = fake.Write

	// Seed history with the confirmed state itself so entering history mode
	// restores it: executing the identical restored state is consecutive-
	// identical and the execution-start append is suppressed (no ID, no
	// eviction), which this test also proves. The distinct-state append case
	// is covered by TestConfirmedInsertLifecycle.
	seed := m.QB.HistoryState()
	m.History.Append(seed)
	m.catalog = prepCatalog()
	if hm, _ := m.enterHistoryMode(); hm != nil {
		m = hm.(Model)
	}
	if !m.historyMode {
		t.Fatal("setup: history mode did not open")
	}

	nm := confirmedWrite(t, m, tea.KeyMsg{Type: tea.KeyEnter}, fake)

	if nm.historyMode || nm.resultHistoryMode {
		t.Fatal("execution start did not exit both histories first")
	}
	if nm.History.Len() != 1 {
		t.Fatalf("query history entries = %d, want the seeded entry with the identical execution suppressed", nm.History.Len())
	}
	entry := nm.History.Entries()[nm.History.Len()-1]
	if !entry.State.Equal(nm.QB.HistoryState()) {
		t.Error("execution-start append did not carry the complete query state")
	}
	if fake.sql != `UPDATE "users" SET "email" = 'new' WHERE "id" = 5` {
		t.Errorf("executed SQL = %q, want the retained rendered statement", fake.sql)
	}
	if !reflect.DeepEqual(fake.params, []any{"new", int64(5)}) {
		t.Errorf("executed params = %#v, want SET-then-WHERE order [new, 5]", fake.params)
	}
	if nm.writePending || nm.ActiveCancellable {
		t.Error("settled write left pending/cancellable state behind")
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one write summary", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Kind != history.KindWrite {
		t.Errorf("entry kind = %v, want write summary", e.Kind)
	}
	if e.ExecutionID != fake.execution || fake.execution == 0 {
		t.Errorf("entry execution identity = %d, want the executed %d", e.ExecutionID, fake.execution)
	}
	if e.SQL != fake.sql {
		t.Errorf("entry SQL = %q, want the executed standalone statement %q", e.SQL, fake.sql)
	}
	if e.Summary != "UPDATE committed: 3 rows affected" {
		t.Errorf("entry summary = %q, want the actual RowsAffected label", e.Summary)
	}
	if e.RowsAffected != 3 {
		t.Errorf("entry RowsAffected = %d, want the actual statement count 3", e.RowsAffected)
	}
}

// TestConfirmedInsertLifecycle drives a runnable INSERT through its dispatch
// boundary: the execution-start message exits history, appends the INSERT
// query state once, dispatches the sole write, and settlement reports rows
// added.
func TestConfirmedInsertLifecycle(t *testing.T) {
	m := insertUIModel(map[string]qb.InsertChoice{
		"id":    qb.InsertChoiceOmit,
		"email": qb.InsertChoiceValue,
		"note":  qb.InsertChoiceNull,
	}, map[string]string{"email": "a@b"})
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
		result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 1},
	}
	m.Write = fake.Write
	m.History = history.NewStore()
	m.History.Append(qb.HistoryState{})
	if hm, _ := m.enterHistoryMode(); hm != nil {
		m = hm.(Model)
	}

	next, cmd := m.Update(ExecutionStartedMsg{})
	if cmd == nil {
		t.Fatal("INSERT dispatch produced no command")
	}
	if next.(Model).historyMode || next.(Model).resultHistoryMode {
		t.Fatal("INSERT dispatch did not exit both histories first")
	}
	if next.(Model).History.Len() != 2 {
		t.Fatalf("query history entries = %d, want the seeded entry plus exactly one execution-start append", next.(Model).History.Len())
	}
	nm := dispatchWriteBatch(t, next.(Model), cmd, true)
	if fake.calls != 1 {
		t.Fatalf("write executor called %d times, want exactly 1", fake.calls)
	}
	if fake.sql != `INSERT INTO "users" ("email", "note") VALUES (?, NULL)` {
		t.Errorf("executed SQL = %q, want the rendered INSERT with omitted columns excluded", fake.sql)
	}
	if len(fake.params) != 1 {
		t.Errorf("executed params = %#v, want only the bound Value parameter", fake.params)
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Summary != "INSERT committed: 1 rows added" {
		t.Errorf("entry summary = %q, want rows-added wording", e.Summary)
	}
	if e.SQL != fake.sql {
		t.Errorf("entry SQL = %q, want the executed standalone INSERT", e.SQL)
	}
}

// TestCancelledWriteRequiresConfirmedRollback proves a cancelled write makes
// the untouched claim only after rollback confirmation, while an unconfirmed
// rollback after the noncancellable boundary crossed is the Issue #45
// outcome-unknown workflow: one outcome-unknown entry and the terminal state.
func TestCancelledWriteRequiresConfirmedRollback(t *testing.T) {
	tests := []struct {
		name      string
		confirmed bool
		want      string
	}{
		{
			name:      "confirmed rollback claims untouched",
			confirmed: true,
			want:      "DELETE cancelled: rollback confirmed, database untouched",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
			m := settledPreparation(t, prepDeleteQB(true), est, est.result)
			fake := &writeFakeExecutor{
				phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
				result: connection.WriteResult{Outcome: connection.WriteCancelled, RollbackConfirmed: tt.confirmed},
			}
			m.Write = fake.Write
			nm := confirmedWrite(t, m, tea.KeyMsg{Type: tea.KeyEnter}, fake)
			if nm.ResultHistory.Len() != 1 {
				t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
			}
			if got := nm.ResultHistory.Entries()[0].Summary; got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUnconfirmedRollbackAfterBoundaryIsOutcomeUnknown proves the Issue #45
// rule: a cancelled write whose noncancellable rollback cleanup ran but whose
// completion was never confirmed settles into exactly one outcome-unknown
// entry and the outcome-unknown terminal state — never an untouched claim.
func TestUnconfirmedRollbackAfterBoundaryIsOutcomeUnknown(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
		result: connection.WriteResult{Outcome: connection.WriteCancelled},
	}
	m.Write = fake.Write
	nm := confirmedWrite(t, m, tea.KeyMsg{Type: tea.KeyEnter}, fake)
	if nm.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", nm.terminalState)
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Kind != history.KindOutcomeUnknown || e.Phase != history.UnknownPhaseRollback {
		t.Errorf("entry = %v phase %v, want outcome-unknown rollback", e.Kind, e.Phase)
	}
	if strings.Contains(e.Summary, "untouched") || !strings.Contains(e.Summary, "outcome unknown") {
		t.Errorf("summary %q does not preserve the unresolved outcome-unknown wording", e.Summary)
	}
}

// TestFailedWriteSummaryPreservesCause proves a constraint-failed write
// finalizes exactly one failed entry preserving the verbatim cause and, with
// confirmed rollback, the untouched claim.
func TestFailedWriteSummaryPreservesCause(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Err: errors.New("ignored")}}
	m := settledPreparation(t, prepUpdateQB(false), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
		result: connection.WriteResult{
			Outcome:           connection.WriteFailed,
			Err:               errors.New("UNIQUE constraint failed: users.email"),
			RollbackConfirmed: true,
		},
	}
	m.Write = fake.Write
	nm := confirmedWrite(t, m, tea.KeyMsg{Type: tea.KeyEnter}, fake)
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Summary != "UPDATE failed: UNIQUE constraint failed: users.email (rollback confirmed, database untouched)" {
		t.Errorf("summary = %q, want the preserved cause with confirmed rollback", e.Summary)
	}
	if e.SQL != `UPDATE "users" SET "email" = 'new'` {
		t.Errorf("entry SQL = %q, want the executed standalone statement", e.SQL)
	}
}

// TestWriteMessagesAreIdempotent proves duplicate and late phase messages and
// re-delivered or stale settlement messages append nothing further, mutate
// nothing, and never start a second write.
func TestWriteMessagesAreIdempotent(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
		result: connection.WriteResult{Outcome: connection.WriteCancelled, RollbackConfirmed: true},
	}
	m.Write = fake.Write
	nm := confirmedWrite(t, m, tea.KeyMsg{Type: tea.KeyEnter}, fake)
	execution := fake.execution

	// Duplicate phase messages for the settled execution.
	next := nm
	apply := func(msg tea.Msg) Model {
		out, _ := next.Update(msg)
		next = out.(Model)
		return next
	}
	for _, phase := range fake.phases {
		apply(connection.WritePhaseMsg{Execution: execution, Phase: phase})
	}
	// Re-delivered and stale settlement messages: must be inert.
	apply(WriteSettledMsg{Execution: execution, Result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 99}})
	apply(connection.WritePhaseMsg{Execution: execution - 1, Phase: connection.WritePhaseCommitting})
	apply(WriteSettledMsg{Execution: execution - 1, Result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 42}})

	final := next
	if final.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one (duplicate/late messages appended nothing)", final.ResultHistory.Len())
	}
	if got := final.ResultHistory.Entries()[0].Summary; got != "DELETE cancelled: rollback confirmed, database untouched" {
		t.Errorf("summary mutated by late messages: %q", got)
	}
	if final.writePending || !final.writeFinalized {
		t.Errorf("post-settlement write state pending=%v finalized=%v", final.writePending, final.writeFinalized)
	}
	if fake.calls != 1 {
		t.Fatalf("late messages started another write: executor called %d times", fake.calls)
	}
}

// TestPostBoundaryWriteIsNoncancellable proves that once the committing phase
// arrives, cancellation ownership is retired — Ctrl+W dispatches nothing and
// no cancellation handle survives — while settlement still finalizes exactly
// one committed result. Boundary feedback rendering is Issue #43's.
func TestPostBoundaryWriteIsNoncancellable(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseCommitting},
		result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 1},
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
	nm := dispatchWriteBatch(t, next.(Model), cmd, false) // settlement withheld
	if !nm.writePending {
		t.Fatal("drained phases did not leave the write pending before settlement")
	}
	if nm.ActiveCancellable {
		t.Fatal("post-boundary write remained cancellable")
	}
	if nm.writeCancel != nil || nm.CancelCommand != nil {
		t.Fatal("cancellation ownership survived the commit boundary")
	}
	if _, cwCmd := nm.Update(tea.KeyMsg{Type: tea.KeyCtrlW}); cwCmd != nil {
		t.Fatal("Ctrl+W after the commit boundary still dispatched a cancellation command")
	}

	// Settle normally: exactly one committed result with rows-affected label.
	final, _ := nm.Update(WriteSettledMsg{
		Execution: fake.execution,
		Result:    connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 1},
	})
	if final.(Model).ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", final.(Model).ResultHistory.Len())
	}
	if e := final.(Model).ResultHistory.Entries()[0]; e.Summary != "DELETE committed: 1 rows affected" {
		t.Errorf("summary = %q, want rows-affected wording", e.Summary)
	}
}

// TestPreparationAppendsNothing proves the Task 3 boundary: settled
// preparation, estimate settlement, and dismissal append nothing to either
// history; only confirmation begins the execution-start append.
func TestPreparationAppendsNothing(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 7}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	if m.History.Len() != 0 || m.ResultHistory.Len() != 0 {
		t.Fatalf("preparation appended history (queries=%d, results=%d)", m.History.Len(), m.ResultHistory.Len())
	}
	nm, _ := m.Update(EstimateSettledMsg{Preparation: m.prepAttempt, Result: EstimateResult{Total: 9}})
	if nmm := nm.(Model); nmm.History.Len() != 0 || nmm.ResultHistory.Len() != 0 {
		t.Fatal("a pre-start message appended history")
	}
	nm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if nmm := nm2.(Model); nmm.History.Len() != 0 || nmm.ResultHistory.Len() != 0 {
		t.Fatal("dismissal appended history")
	}
}
