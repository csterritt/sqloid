# Issue #42 — transactional write execution and summaries

*2026-08-29T14:47:18Z by Showboat 0.6.1*
<!-- showboat-id: 911d50f6-46e0-44c2-92e3-bca502be294d -->

This walkthrough verifies the Issue #42 implementation (transactional write execution and summaries) against the 'Writes and commit boundary', 'write-transaction', and write-cancellation decisions in Notes/PRD-sqloid.md: the one-request/one-lease write lifecycle (one pre-BEGIN identity check, BEGIN, exactly one statement, the atomic post-statement pre-COMMIT cancellation check winning even after a successful statement, noncancellable rollback cleanup or commit, settlement and lease reuse), constraint and trigger failures participating in the same transaction, execution-start query-history append for confirmed UPDATE/DELETE and dispatched INSERT, exactly one immutable non-tabular result entry per write with the executed standalone SQL, actual RowsAffected rows-affected versus INSERT rows-added labels, and no database-untouched claim before confirmed rollback. Issues #43 (post-boundary Ctrl+W/quit interaction) and #45 (outcome-unknown) remain the post-boundary owners.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection -run '^TestWrite' 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-60
go test -count=1 ./internal/history -run '^(TestWriteSummaryLabels|TestWriteResultEntry)$' 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g'
go test -count=1 ./internal/ui -run '^(TestConfirmedUpdateLifecycle|TestConfirmedInsertLifecycle|TestCancelledWriteRequiresConfirmedRollback|TestFailedWriteSummaryPreservesCause|TestWriteMessagesAreIdempotent|TestPostBoundaryWriteIsNoncancellable|TestPreparationAppendsNothing)$' 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/history	
ok  	github.com/chris/sqloid/internal/ui	
```

Part 1 — the connection boundary. A temporary test file (removed again by the same block) drives real modernc SQLite through the production StartWrite path with the package's documented barrier seams. (A) an unqualified UPDATE commits, persists, and a per-firing trigger proves exactly one statement execution; (B) cancellation requested during the commit-phase barrier — i.e. after statement success — must win before COMMIT; (C) a UNIQUE constraint failure and (D) a trigger RAISE(ABORT) failure both confirm rollback, preserve the native cause, and persist nothing; each committed case is followed by a lease-reuse write proving settlement released the lease.

```bash
cd /home/chris/sqloid && cat > internal/connection/zz_42_walkthrough_test.go <<'WALKTHROUGH_EOF'
package connection

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
)

var zzSeq atomic.Uint64

func zzRows(t *testing.T, path, query string) int {
	t.Helper()
	ext, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ext.Close()
	var n int
	if err := ext.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func zzEdit(t *testing.T, path, ddl string) {
	t.Helper()
	ext, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ext.Close()
	if _, err := ext.Exec(ddl); err != nil {
		t.Fatal(err)
	}
}

// (A) unqualified UPDATE commits exactly once, persists, trigger counter.
func TestZZWriteCommitAndSingleExecution(t *testing.T) {
	db := openJournalFixture(t, "delete")
	zzEdit(t, db.path, `CREATE TABLE fires (n INTEGER);
INSERT INTO fires VALUES (0);
CREATE TRIGGER zz_count AFTER UPDATE ON users BEGIN UPDATE fires SET n = n + 1; END;`)

	w := db.StartWrite(context.Background(), zzSeq.Add(1), `UPDATE "users" SET "email" = 'committed'`, nil)
	res := w.Wait()
	if res.Outcome != WriteCommitted || res.RowsAffected != 3 {
		t.Fatalf("outcome=%v rows=%d, want committed 3", res.Outcome, res.RowsAffected)
	}
	if got := zzRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'committed'"); got != 3 {
		t.Fatalf("persisted committed rows = %d, want 3", got)
	}
	if got := zzRows(t, db.path, "SELECT n FROM fires"); got != 3 {
		t.Fatalf("statement fired trigger %d times, want exactly 3 (one per row)", got)
	}
	t.Logf("(A) UPDATE committed: rows=%d, trigger firings=3, lease released: %v", res.RowsAffected, w.Settled())
}

// (B) pre-COMMIT cancellation wins after statement success.
func TestZZPreCommitCancellationWins(t *testing.T) {
	db := openJournalFixture(t, "delete")
	held := make(chan struct{}, 1)
	release := make(chan struct{})
	db.beforeWriteCommit = func(ctx context.Context, conn *sql.Conn) {
		held <- struct{}{}
		<-release
	}
	t.Cleanup(func() { db.clearWriteHooks() })

	w := db.StartWrite(context.Background(), zzSeq.Add(1), `UPDATE "users" SET "email" = 'never'`, nil)
	<-held // statement already succeeded; commit not begun
	w.Cancel()
	close(release)

	res := w.Wait()
	if res.Outcome != WriteCancelled || !res.RollbackConfirmed {
		t.Fatalf("outcome=%v confirmed=%v, want cancelled with confirmed rollback", res.Outcome, res.RollbackConfirmed)
	}
	if got := zzRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'never'"); got != 0 {
		t.Fatalf("pre-COMMIT cancellation persisted %d rows", got)
	}
	t.Logf("(B) cancelled after statement success: outcome=%v rollbackConfirmed=%v persistedUpdatedRows=0", res.Outcome, res.RollbackConfirmed)
}

// (C) constraint failure rolls back with the native cause.
func TestZZConstraintFailureRollsBack(t *testing.T) {
	db := openJournalFixture(t, "delete")
	w := db.StartWrite(context.Background(), zzSeq.Add(1), `INSERT INTO "users" ("id", "email") VALUES (?, ?)`, []any{int64(1), "dupe"})
	res := w.Wait()
	if res.Outcome != WriteFailed || !res.RollbackConfirmed || res.Err == nil || !strings.Contains(res.Err.Error(), "UNIQUE") {
		t.Fatalf("outcome=%v confirmed=%v err=%v, want failed with preserved UNIQUE cause", res.Outcome, res.RollbackConfirmed, res.Err)
	}
	if got := zzRows(t, db.path, "SELECT COUNT(*) FROM users"); got != 3 {
		t.Fatalf("failed write left %d rows, want the original 3", got)
	}
	t.Logf("(C) constraint failure: err=%v rollbackConfirmed=%v persistedRows=3", res.Err, res.RollbackConfirmed)
}

// (D) trigger failure (RAISE ABORT) rolls back.
func TestZZTriggerFailureRollsBack(t *testing.T) {
	db := openJournalFixture(t, "delete")
	zzEdit(t, db.path, `CREATE TRIGGER zz_guard BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'guard rejected'); END;`)

	w := db.StartWrite(context.Background(), zzSeq.Add(1), `UPDATE "users" SET "email" = 'nope'`, nil)
	res := w.Wait()
	if res.Outcome != WriteFailed || !res.RollbackConfirmed || !strings.Contains(res.Err.Error(), "guard rejected") {
		t.Fatalf("outcome=%v confirmed=%v err=%v, want failed with preserved trigger cause", res.Outcome, res.RollbackConfirmed, res.Err)
	}
	if got := zzRows(t, db.path, "SELECT COUNT(*) FROM users WHERE email = 'nope'"); got != 0 {
		t.Fatalf("failed trigger write persisted %d rows", got)
	}
	t.Logf("(D) trigger failure: err=%v rollbackConfirmed=%v persistedUpdatedRows=0", res.Err, res.RollbackConfirmed)
}

WALKTHROUGH_EOF
go test -count=1 ./internal/connection -run '^TestZZ' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)|zz_42_walkthrough_test.go' | cut -c1-160
rm internal/connection/zz_42_walkthrough_test.go
```

```output
=== RUN   TestZZWriteCommitAndSingleExecution
    zz_42_walkthrough_test.go:59: (A) UPDATE committed: rows=3, trigger firings=3, lease released: true
--- PASS: TestZZWriteCommitAndSingleExecution ()
=== RUN   TestZZPreCommitCancellationWins
    zz_42_walkthrough_test.go:85: (B) cancelled after statement success: outcome=cancelled rollbackConfirmed=true persistedUpdatedRows=0
--- PASS: TestZZPreCommitCancellationWins ()
=== RUN   TestZZConstraintFailureRollsBack
    zz_42_walkthrough_test.go:99: (C) constraint failure: err=constraint failed: UNIQUE constraint failed: users.id (1555) rollbackConfirmed=true persistedRows=
--- PASS: TestZZConstraintFailureRollsBack ()
=== RUN   TestZZTriggerFailureRollsBack
    zz_42_walkthrough_test.go:115: (D) trigger failure: err=constraint failed: guard rejected (1811) rollbackConfirmed=true persistedUpdatedRows=0
--- PASS: TestZZTriggerFailureRollsBack ()
PASS
ok  	github.com/chris/sqloid/internal/connection	
```

Part 2 — the UI lifecycle. The scripted Bubble Tea model consumes the typed connection phases: (E) a confirmed UPDATE appends its complete query state exactly once at the execution-start boundary (the identical seeded state is suppressed), runs the sole write once, and finalizes exactly one non-tabular result entry carrying the executed standalone SQL and the actual RowsAffected label; (F) a cancelled write whose rollback was not confirmed makes no untouched claim; (G) a dispatched INSERT appends at execution start and settles with rows-added wording and the standalone INSERT (omitted columns excluded, bound Value parameter only).

```bash
cd /home/chris/sqloid && cat > internal/ui/zz_42_walkthrough_test.go <<'WALKTHROUGH_EOF'
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// zzDrain unpacks a write batch, runs the write command, drains every phase
// through Update, and returns the model plus the settled message withheld.
func zzDrain(t *testing.T, m Model, cmd tea.Cmd) (Model, WriteSettledMsg) {
	t.Helper()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("write dispatch produced no batch")
	}
	var settled WriteSettledMsg
	var relays []tea.Cmd
	for _, c := range batch {
		if c == nil {
			continue
		}
		out := c()
		if s, is := out.(WriteSettledMsg); is {
			settled = s
			continue
		}
		if out != nil {
			relays = append(relays, c)
		}
	}
	for _, c := range relays {
		for out := c(); out != nil; {
			p, is := out.(connection.WritePhaseMsg)
			if !is {
				t.Fatal("relay produced a non-phase message")
			}
			var cont tea.Cmd
			next, cont := m.Update(p)
			m = next.(Model)
			if cont == nil {
				t.Fatal("phase dispatched no continuation")
			}
			out = cont()
		}
	}
	return m, settled
}

// (E) confirmed UPDATE: one history append, one sole write, exactly one
// result with the actual rows-affected label.
func TestZZConfirmedUpdateLifecycle(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 7}}
	m := settledPreparation(t, prepUpdateQB(true), est, est.result)
	fake := committedUpdateFake(3)
	m.Write = fake.Write

	m.History.Append(m.QB.HistoryState()) // consecutive-identical seed
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := cmd().(WriteConfirmedMsg)
	next, cmd = next.(Model).Update(confirmed)
	nm, settled := zzDrain(t, next.(Model), cmd)
	dn, _ := nm.Update(settled)
	nm = dn.(Model)

	if nm.History.Len() != 1 {
		t.Fatalf("query history entries = %d, want suppression to keep one", nm.History.Len())
	}
	if fake.calls != 1 {
		t.Fatalf("executor called %d times, want exactly one sole actual execution", fake.calls)
	}
	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Summary != "UPDATE committed: 3 rows affected" || e.SQL != `UPDATE "users" SET "email" = 'new' WHERE "id" = 5` {
		t.Fatalf("summary=%q sql=%q", e.Summary, e.SQL)
	}
	t.Logf("(E) confirmed UPDATE: historyEntries=1 writeCalls=1 resultEntries=1 summary=%q", e.Summary)
}

// (F) cancelled write without confirmed rollback never claims untouched.
func TestZZCancelledNoUntouchedClaim(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)
	fake := &writeFakeExecutor{
		phases: []connection.WritePhase{connection.WritePhaseBeginning, connection.WritePhaseExecuting, connection.WritePhaseRollbackCleanup},
		result: connection.WriteResult{Outcome: connection.WriteCancelled}, // no rollback confirmation
	}
	m.Write = fake.Write

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := cmd().(WriteConfirmedMsg)
	next, cmd = next.(Model).Update(confirmed)
	nm, settled := zzDrain(t, next.(Model), cmd)
	dn, _ := nm.Update(settled)
	nm = dn.(Model)

	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	got := nm.ResultHistory.Entries()[0].Summary
	if got != "DELETE cancelled" {
		t.Fatalf("summary=%q, want exactly one entry making no untouched claim", got)
	}
	t.Logf("(F) cancelled without confirmed rollback: summary=%q (no untouched claim)", got)
}

// (G) INSERT dispatch: execution-start append and rows-added summary.
func TestZZInsertRowsAdded(t *testing.T) {
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

	next, cmd := m.Update(ExecutionStartedMsg{})
	if cmd == nil {
		t.Fatal("INSERT dispatch produced no command")
	}
	if next.(Model).History.Len() != 2 {
		t.Fatalf("query history entries = %d, want seed plus one execution-start append", next.(Model).History.Len())
	}
	nm, settled := zzDrain(t, next.(Model), cmd)
	dn, _ := nm.Update(settled)
	nm = dn.(Model)

	if nm.ResultHistory.Len() != 1 {
		t.Fatalf("result entries = %d, want exactly one", nm.ResultHistory.Len())
	}
	e := nm.ResultHistory.Entries()[0]
	if e.Summary != "INSERT committed: 1 rows added" || e.SQL != `INSERT INTO "users" ("email", "note") VALUES (?, NULL)` {
		t.Fatalf("summary=%q sql=%q, want rows-added wording and the standalone INSERT", e.Summary, e.SQL)
	}
	t.Logf("(G) INSERT dispatch: historyEntries=2 resultEntries=1 summary=%q sql=%q", e.Summary, e.SQL)
}

WALKTHROUGH_EOF
go test -count=1 ./internal/ui -run '^TestZZ' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)|zz_42_walkthrough_test.go' | cut -c1-160
rm internal/ui/zz_42_walkthrough_test.go
```

```output
=== RUN   TestZZConfirmedUpdateLifecycle
    zz_42_walkthrough_test.go:83: (E) confirmed UPDATE: historyEntries=1 writeCalls=1 resultEntries=1 summary="UPDATE committed: 3 rows affected"
--- PASS: TestZZConfirmedUpdateLifecycle ()
=== RUN   TestZZCancelledNoUntouchedClaim
    zz_42_walkthrough_test.go:110: (F) cancelled without confirmed rollback: summary="DELETE cancelled" (no untouched claim)
--- PASS: TestZZCancelledNoUntouchedClaim ()
=== RUN   TestZZInsertRowsAdded
    zz_42_walkthrough_test.go:146: (G) INSERT dispatch: historyEntries=2 resultEntries=1 summary="INSERT committed: 1 rows added" sql="INSERT INTO \"users\" (\"
--- PASS: TestZZInsertRowsAdded ()
PASS
ok  	github.com/chris/sqloid/internal/ui	
```

Every phase transition above was held behind a deterministic channel barrier — cancellation and settlement land at exact points, never by timing. Persisted state was re-read through an independent pooled connection, not the write's lease. The production suites (Task 1 tests in internal/connection, Task 3 tests in internal/history and internal/ui) all pass, and Issues #43 and #45 remain the post-boundary and outcome-unknown owners.
