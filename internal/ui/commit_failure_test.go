// Real driver-boundary COMMIT-failure UI routing coverage for Issue #61, per
// the Writes and commit boundary decisions in Notes/PRD-sqloid.md. A real
// tx.Commit() failure — induced by a deferred foreign-key constraint checked
// at COMMIT time — is driven through internal/ui/write_exec.go and must
// produce exactly one outcome-unknown entry selected in
// TerminalOutcomeUnknown, with commit-phase/error wording and non-persistence
// RowsAffected disclosure, no ordinary WriteFailed summary, and no
// rollback/untouched claim. A control case proves a pre-COMMIT statement
// failure with genuinely successful rollback remains a definite WriteFailed
// entry with confirmed-untouched wording.

package ui

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	_ "modernc.org/sqlite"
)

// commitFailureFixture creates a SQLite database with a deferred foreign-key
// constraint and returns its path. An INSERT into child referencing a
// non-existent parent succeeds at the statement level but fails at COMMIT
// time when foreign_keys is enabled on the connection.
func commitFailureFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "commit_failure.db")
	dbh, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := dbh.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE child (
    id INTEGER PRIMARY KEY,
    parent_id INTEGER NOT NULL REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED
);`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if err := dbh.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return path
}

// realCommitFailureExecutor returns a WriteExecutor that opens its own
// connection to the fixture with foreign_keys enabled, runs a real
// transaction whose COMMIT fails on a deferred FK constraint, and returns
// the real connection.WriteResult with the committing phase preserved. The
// phase messages are relayed synchronously before settlement, matching the
// production WriteExecutor contract.
func realCommitFailureExecutor(t *testing.T, path string) WriteExecutor {
	t.Helper()
	return func(ctx context.Context, execution uint64, stmt string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult {
		dbh, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)")
		if err != nil {
			t.Fatalf("open executor connection: %v", err)
		}
		defer dbh.Close()
		conn, err := dbh.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire executor connection: %v", err)
		}
		defer conn.Close()

		phase(connection.WritePhaseMsg{Execution: execution, Phase: connection.WritePhaseBeginning})
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}

		phase(connection.WritePhaseMsg{Execution: execution, Phase: connection.WritePhaseExecuting})
		res, err := tx.ExecContext(ctx, stmt, params...)
		if err != nil {
			phase(connection.WritePhaseMsg{Execution: execution, Phase: connection.WritePhaseRollbackCleanup})
			_ = tx.Rollback()
			return connection.WriteResult{Outcome: connection.WriteFailed, Err: err, Phase: connection.WritePhaseExecuting}
		}
		rows, _ := res.RowsAffected()

		phase(connection.WritePhaseMsg{Execution: execution, Phase: connection.WritePhaseCommitting})
		if err := tx.Commit(); err != nil {
			// Real COMMIT failure: the deferred FK constraint is violated
			// at COMMIT time. tx.Rollback() returns sql.ErrTxDone because
			// the transaction is no longer active, which does not prove
			// whether the commit persisted. Preserve the commit error and
			// committing phase; leave RollbackConfirmed false.
			phase(connection.WritePhaseMsg{Execution: execution, Phase: connection.WritePhaseRollbackCleanup})
			_ = tx.Rollback()
			return connection.WriteResult{
				Outcome:           connection.WriteFailed,
				Err:               err,
				RowsAffected:      rows,
				RollbackConfirmed: false,
				Phase:             connection.WritePhaseCommitting,
			}
		}
		return connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: rows}
	}
}

// TestRealCommitFailureRoutesThroughOutcomeUnknown drives a real
// driver-boundary COMMIT failure through the UI's write_exec.go finalization
// path and requires exactly one outcome-unknown entry selected in
// TerminalOutcomeUnknown, with commit-phase/error wording and non-persistence
// RowsAffected disclosure, no ordinary WriteFailed summary, and no
// rollback/untouched claim.
func TestRealCommitFailureRoutesThroughOutcomeUnknown(t *testing.T) {
	path := commitFailureFixture(t)
	execution := uint64(9001)

	m := New()
	m.Write = realCommitFailureExecutor(t, path)

	cmd := m.startWrite(execution, "INSERT",
		`INSERT INTO "child" ("id", "parent_id") VALUES (?, ?)`,
		[]any{int64(1), int64(999)})
	if cmd == nil {
		t.Fatal("startWrite produced no command")
	}
	m = dispatchWriteBatch(t, m, cmd, true)

	if m.terminalState != TerminalOutcomeUnknown {
		t.Fatalf("terminal state = %v, want TerminalOutcomeUnknown", m.terminalState)
	}
	if m.writePending {
		t.Fatal("settled unresolved write left pending state behind")
	}
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one outcome-unknown entry", m.ResultHistory.Len())
	}
	e, ok := m.ResultHistory.Newest()
	if !ok || e.ID != m.resultHistoryCursorID {
		t.Fatalf("selected cursor = %v, want the newest appended entry (ok=%v)", m.resultHistoryCursorID, ok)
	}
	if e.Kind != history.KindOutcomeUnknown {
		t.Errorf("entry kind = %v, want outcome-unknown", e.Kind)
	}
	if e.ExecutionID != execution {
		t.Errorf("entry execution = %d, want %d", e.ExecutionID, execution)
	}
	if e.Operation != "INSERT" {
		t.Errorf("entry operation = %q, want INSERT", e.Operation)
	}
	if e.Phase != history.UnknownPhaseCommit {
		t.Errorf("entry phase = %v, want commit", e.Phase)
	}
	if !strings.Contains(e.Summary, "commit did not resolve") {
		t.Errorf("summary %q lacks the commit-phase wording", e.Summary)
	}
	if !strings.Contains(strings.ToLower(e.Summary), "foreign key") {
		t.Errorf("summary %q lost the driver commit error", e.Summary)
	}
	if !strings.Contains(e.Summary, "does not prove persistence") {
		t.Errorf("summary %q lacks the non-persistence RowsAffected disclosure", e.Summary)
	}
	for _, forbidden := range []string{"committed", "untouched", "rollback confirmed", "rows added"} {
		if strings.Contains(strings.ToLower(e.Summary), strings.ToLower(forbidden)) {
			t.Errorf("summary %q claims %q the resolution never proved", e.Summary, forbidden)
		}
	}
}

// TestRealPreCommitFailureWithConfirmedRollback is the UI control case: a
// real pre-COMMIT statement failure (UNIQUE constraint) with genuinely
// successful rollback produces a definite WriteFailed entry with
// confirmed-untouched wording, not an outcome-unknown entry.
func TestRealPreCommitFailureWithConfirmedRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	dbh, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := dbh.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);
INSERT INTO users (id, email) VALUES (1, 'existing');`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if err := dbh.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	execution := uint64(9002)
	executor := func(ctx context.Context, exec uint64, stmt string, params []any, phase func(connection.WritePhaseMsg)) connection.WriteResult {
		dbh, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("open executor: %v", err)
		}
		defer dbh.Close()
		conn, err := dbh.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire conn: %v", err)
		}
		defer conn.Close()

		phase(connection.WritePhaseMsg{Execution: exec, Phase: connection.WritePhaseBeginning})
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		phase(connection.WritePhaseMsg{Execution: exec, Phase: connection.WritePhaseExecuting})
		_, err = tx.ExecContext(ctx, stmt, params...)
		if err != nil {
			phase(connection.WritePhaseMsg{Execution: exec, Phase: connection.WritePhaseRollbackCleanup})
			rbErr := tx.Rollback()
			return connection.WriteResult{
				Outcome:           connection.WriteFailed,
				Err:               err,
				Phase:             connection.WritePhaseExecuting,
				RollbackConfirmed: rbErr == nil,
			}
		}
		phase(connection.WritePhaseMsg{Execution: exec, Phase: connection.WritePhaseCommitting})
		if err := tx.Commit(); err != nil {
			phase(connection.WritePhaseMsg{Execution: exec, Phase: connection.WritePhaseRollbackCleanup})
			_ = tx.Rollback()
			return connection.WriteResult{Outcome: connection.WriteFailed, Err: err, Phase: connection.WritePhaseCommitting}
		}
		return connection.WriteResult{Outcome: connection.WriteCommitted}
	}

	m := New()
	m.Write = executor
	cmd := m.startWrite(execution, "INSERT",
		`INSERT INTO "users" ("id", "email") VALUES (?, ?)`,
		[]any{int64(1), "dupe"})
	if cmd == nil {
		t.Fatal("startWrite produced no command")
	}
	m = dispatchWriteBatch(t, m, cmd, true)

	if m.terminalState == TerminalOutcomeUnknown {
		t.Fatal("pre-COMMIT failure with confirmed rollback entered outcome-unknown; want a definite failed entry")
	}
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", m.ResultHistory.Len())
	}
	e, ok := m.ResultHistory.Newest()
	if !ok {
		t.Fatal("no entry was appended")
	}
	if e.Kind != history.KindWrite {
		t.Errorf("entry kind = %v, want write (definite failed)", e.Kind)
	}
	if !strings.Contains(e.Summary, "failed") {
		t.Errorf("summary %q lacks the definite failed wording", e.Summary)
	}
	if !strings.Contains(e.Summary, "rollback confirmed") {
		t.Errorf("summary %q lacks the confirmed-untouched wording for genuine rollback", e.Summary)
	}
}
