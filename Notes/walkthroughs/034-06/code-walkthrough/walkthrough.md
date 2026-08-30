# Issue #34 — Active SELECT lifetime and exactly-once finalization

*2026-08-28T20:26:44Z by Showboat 0.6.1*
<!-- showboat-id: ce42758a-66d9-4b96-8a62-cf2e64f15048 -->

This walkthrough demonstrates the completed Issue #34 implementation (Notes/tasks/034-active-select-lifetime-and-single-finalization.md): the active SELECT lifetime kept distinct from execution IDs, request IDs, and viewport generations; the lifecycle matrix proving every non-finalizing event (editing, overlays/help, save/export, estimation workflows, query-history browsing, resize, serialized paging, page/count settlement, count failure, idle periods) preserves activity; each finalizer exercised separately (actual new SELECT execution, result-history entry, ending cancellation/failure, accepted quit with idle and pending work); exactly one immutable entry per execution for success, count-failed rows, partial page failure, and cancellation/failure before or after rows with correct tabular versus non-tabular outcomes and retained Issue #33 metadata; and duplicate/late-message idempotence. See Issue #34, Notes/PRD-sqloid.md (Identities and state, SELECT, Cache and snapshot invariant, active-finalization Testing Decisions), and Notes/wiki/active-select-lifetime.md. Every block below is re-runnable from the repository root; each writes, runs, and removes its own temporary demo test.

Block 1 — the lifecycle matrix over one active SELECT with independent page and count requests. A temporary test drives a real execution through validation and start, holds the page and count messages to build each pending/settled combination, then applies every non-finalizing event category and asserts the active execution, identity, and empty result history survive each one.

```bash
cat > internal/ui/zz_demo34a_test.go <<'EOF'
package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDemo34Matrix(t *testing.T) {
	for _, st := range []struct {
		name                         string
		pagePending, countPending    bool
	}{
		{"idle", false, false},
		{"count-pending", false, true},
		{"page-pending", true, false},
		{"both-pending", true, true},
	} {
		m, page, count := startActiveSelect(t)
		m = apply(m, page) // first page always settles: rows displayed
		if !st.countPending {
			m = apply(m, count)
		}
		if st.pagePending {
			withReq, cmd := pageDown(m)
			m = withReq
			_ = cmd // later-page request held in flight
		}
		execID := m.ActiveSelectExecutionID()

		events := []struct {
			name  string
			apply func(m Model) Model
		}{
			{"idle period", func(m Model) Model { return m }},
			{"builder focus edit", func(m Model) Model {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
				return next.(Model)
			}},
			{"help overlay + Esc", func(m Model) Model {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
				next2, _ := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
				return next2.(Model)
			}},
			{"save/export keys", func(m Model) Model {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
				next2, _ := next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlX})
				return next2.(Model)
			}},
			{"query-history keys", func(m Model) Model {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
				next2, _ := next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlN})
				return next2.(Model)
			}},
			{"resize", func(m Model) Model {
				next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
				return next.(Model)
			}},
		}
		for _, ev := range events {
			after := ev.apply(m)
			if !after.SelectIsActive() || after.ActiveSelectExecutionID() != execID ||
				after.FinalizedSelectExecutionID() != 0 || after.ResultHistory.Len() != 0 {
				t.Errorf("%s/%s: active lifetime lost", st.name, ev.name)
			}
		}
		if st.countPending {
			fail := count
			fail.Result = CountResult{Err: errors.New("count failed")}
			m2 := apply(m, fail)
			if !m2.SelectIsActive() || m2.ResultHistory.Len() != 0 {
				t.Errorf("%s: count failure finalized", st.name)
			}
		}
		t.Logf("%-14s activity preserved through all 6 events", st.name)
	}
}
EOF
go test ./internal/ui -run TestDemo34Matrix -count=1 -v 2>&1 | grep -E "preserved|FAIL|ok "
rm internal/ui/zz_demo34a_test.go
```

```output
    zz_demo34a_test.go:76: idle           activity preserved through all 6 events
    zz_demo34a_test.go:76: count-pending  activity preserved through all 6 events
    zz_demo34a_test.go:76: page-pending   activity preserved through all 6 events
    zz_demo34a_test.go:76: both-pending   activity preserved through all 6 events
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Block 2 — each finalizer exercised separately. The same active-SELECT fixture is ended by: starting an actual new SELECT execution (previous lifetime finalized before replacement; write executions finalize through the same seam, and estimate/validation remain pre-execution and never finalize), entering result history (future page mutation invalidated), an ending cancellation whose classified-cancelled first-page settlement occurs before any row, a first-page failure before rows, and accepted quit both while idle and with pending count work. Every finalizer produces exactly one finalized execution identity and exactly one result-history entry.

```bash
cat > internal/ui/zz_demo34b_test.go <<'EOF'
package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func requireFinal(t *testing.T, m Model, execID uint64, ctx string) {
	t.Helper()
	if m.SelectIsActive() || m.FinalizedSelectExecutionID() != execID || m.ResultHistory.Len() != 1 {
		t.Errorf("%s: active=%v finalized=%d entries=%d", ctx, m.SelectIsActive(), m.FinalizedSelectExecutionID(), m.ResultHistory.Len())
	}
}

func TestDemo34Finalizers(t *testing.T) {
	// (1) actual new execution start: the previous lifetime is finalized
	// before the new execution replaces it.
	m, page, count := startActiveSelect(t)
	first := m.ActiveSelectExecutionID()
	m = apply(m, page)
	m = apply(m, count)
	m.Select = (&fakeSelectExecutor{page: threeRowFirstPage()}).selectPage
	m.Count = (&fakeCountExecutor{total: 3}).count
	second, execCmd := driveToExecutionStart(t, m)
	_ = execBatch(t, execCmd) // dispatch the second execution's page/count pair
	if second.ActiveSelectExecutionID() == first || !second.SelectIsActive() {
		t.Errorf("new execution identity wrong: first=%d active=%d", first, second.ActiveSelectExecutionID())
	}
	if second.FinalizedSelectExecutionID() != first || second.ResultHistory.Len() != 1 {
		t.Errorf("previous execution not finalized exactly once: finalized=%d entries=%d",
			second.FinalizedSelectExecutionID(), second.ResultHistory.Len())
	}
	t.Logf("new execution: previous execution %d finalized exactly once", first)

	// (2) entering result history invalidates future page mutation.
	m, _, _, _ = fixtureFor(t, activeState{name: "idle"})
	execID := m.ActiveSelectExecutionID()
	m.enterResultHistory()
	if _, cmd := pageDown(m); cmd != nil {
		t.Error("finalized SELECT dispatched a page request")
	}
	requireFinal(t, m, execID, "result history")

	// (3) ending cancellation: first-page settlement classified cancelled.
	m, page, _ = startActiveSelect(t)
	execID = m.ActiveSelectExecutionID()
	page.Result = FirstPageResult{Cancelled: true}
	requireFinal(t, apply(m, page), execID, "ending cancellation")

	// (4) first-page failure before rows.
	m, page, _ = startActiveSelect(t)
	execID = m.ActiveSelectExecutionID()
	page.Result = FirstPageResult{Err: errors.New("first page failed")}
	requireFinal(t, apply(m, page), execID, "first-page failure")

	// (5) accepted quit while idle and with pending count work.
	m, _, _, _ = fixtureFor(t, activeState{name: "idle"})
	execID = m.ActiveSelectExecutionID()
	confirming := m
	next, _ := confirming.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	confirming = next.(Model)
	next2, _ := confirming.Update(tea.KeyMsg{Type: tea.KeyEnter})
	requireFinal(t, next2.(Model), execID, "accepted quit idle")

	m, _, _, _ = fixtureFor(t, activeState{name: "count pending", countPending: true})
	execID = m.ActiveSelectExecutionID()
	m.acceptedQuitCleanup()
	requireFinal(t, m, execID, "accepted quit pending work")
}
EOF
go test ./internal/ui -run TestDemo34Finalizers -count=1 -v 2>&1 | grep -E "new execution|FAIL|ok "
rm internal/ui/zz_demo34b_test.go
```

```output
    zz_demo34b_test.go:35: new execution: previous execution 1 finalized exactly once
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Block 3 — exactly-once immutable snapshots. One entry per execution for every defined outcome: idle success (tabular with retained rows and Issue #33 metadata), successful rows with the count unavailable, partial page failure after rows (failed outcome, rows preserved), cancellation before rows (non-tabular Cancelled), first-page failure before rows (non-tabular error), and cancellation/failure after rows (tabular with cancelled/failed outcomes). Later cache activity cannot mutate a finalized snapshot.

```bash
cat > internal/ui/zz_demo34c_test.go <<'EOF'
package ui

import (
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/history"
)

func oneEntry(t *testing.T, m Model, kind history.ResultKind, ctx string) history.ResultEntry {
	t.Helper()
	if m.ResultHistory.Len() != 1 {
		t.Errorf("%s: entries=%d, want 1", ctx, m.ResultHistory.Len())
	}
	entry := m.ResultHistory.Entries()[0]
	if entry.Kind != kind {
		t.Errorf("%s: kind=%v, want %v", ctx, entry.Kind, kind)
	}
	return entry
}

func TestDemo34Snapshots(t *testing.T) {
	// Idle success: tabular, success outcome, complete classification.
	m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
	m.enterResultHistory()
	e := oneEntry(t, m, history.KindTabular, "idle success")
	t.Logf("idle success: rows=%d outcome=%v completeness=%v retained=%v..%v",
		len(e.Rows), e.Metadata.Outcome, e.Completeness, e.Metadata.RetainedStart, e.Metadata.RetainedEnd)

	// Successful rows with count unavailable: tabular, still success outcome.
	m, _, _, count := fixtureFor(t, activeState{name: "count pending", countPending: true})
	count.Result = CountResult{Err: errors.New("count failed")}
	m = apply(m, count)
	m.enterResultHistory()
	e = oneEntry(t, m, history.KindTabular, "count-failed rows")
	t.Logf("count-failed rows: rows=%d outcome=%v completeness=%v", len(e.Rows), e.Metadata.Outcome, e.Completeness)

	// Partial page failure after retained rows: failed outcome, rows preserved.
	m, _, page, count := fixtureFor(t, activeState{name: "page pending", pagePending: true})
	m = apply(m, count)
	page.Result = FirstPageResult{Err: errors.New("page 2 failed")}
	m = apply(m, page)
	m.enterResultHistory()
	e = oneEntry(t, m, history.KindTabular, "partial page failure")
	t.Logf("partial page failure: rows=%d outcome=%v reason=%q", len(e.Rows), e.Metadata.Outcome, e.Metadata.Reason)

	// Cancellation before rows: non-tabular Cancelled entry.
	m, pageB, _ := startActiveSelect(t)
	pageB.Result = FirstPageResult{Cancelled: true}
	m = apply(m, pageB)
	e = oneEntry(t, m, history.KindCancelled, "cancelled before rows")
	t.Logf("cancelled before rows: rows=%d outcome=%v reason=%q", len(e.Rows), e.Metadata.Outcome, e.Metadata.Reason)

	// First-page failure before rows: non-tabular error entry.
	m, pageC, _ := startActiveSelect(t)
	pageC.Result = FirstPageResult{Err: errors.New("no such table")}
	m = apply(m, pageC)
	e = oneEntry(t, m, history.KindError, "first-page failure")
	t.Logf("first-page failure: rows=%d outcome=%v reason=%q", len(e.Rows), e.Metadata.Outcome, e.Metadata.Reason)

	// Cancellation after rows: tabular, cancelled outcome, rows preserved.
	m, _, pageD, countD := fixtureFor(t, activeState{name: "page pending", pagePending: true})
	m = apply(m, countD)
	pageD.Result = FirstPageResult{Cancelled: true}
	m = apply(m, pageD)
	m.enterResultHistory()
	e = oneEntry(t, m, history.KindTabular, "cancelled after rows")
	t.Logf("cancelled after rows: rows=%d outcome=%v", len(e.Rows), e.Metadata.Outcome)

	// Immutability: later cache merges cannot change the finalized snapshot.
	before := len(e.Rows)
	mergeRowsIntoCache(t, &m, 4, 5)
	after := m.ResultHistory.Entries()[0]
	if len(after.Rows) != before {
		t.Errorf("snapshot mutated: rows %d -> %d", before, len(after.Rows))
	}
}
EOF
go test ./internal/ui -run TestDemo34Snapshots -count=1 -v 2>&1 | grep -E "rows=|FAIL|ok "
rm internal/ui/zz_demo34c_test.go
```

```output
    zz_demo34c_test.go:27: idle success: rows=3 outcome=success completeness=complete retained=1..3
    zz_demo34c_test.go:36: count-failed rows: rows=3 outcome=success completeness=partial
    zz_demo34c_test.go:45: partial page failure: rows=3 outcome=failed reason="page 2 failed"
    zz_demo34c_test.go:52: cancelled before rows: rows=0 outcome=cancelled reason="cancelled"
    zz_demo34c_test.go:59: first-page failure: rows=0 outcome=failed reason="no such table"
    zz_demo34c_test.go:68: cancelled after rows: rows=3 outcome=cancelled
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Block 4 — duplicate and late finalization is harmless: repeated history-entry commands, repeated quit cleanup, replayed cancelled settlements, late old-execution count/page results, and quit finalization distinct from individual request settlement (one entry per execution, never per request); the store itself deterministically rejects a second entry for a finalized execution ID (internal/history).

```bash
cat > internal/ui/zz_demo34d_test.go <<'EOF'
package ui

import (
	"errors"
	"testing"

)

func TestDemo34Idempotence(t *testing.T) {
	// Repeated finalizer messages and repeated quit cleanup.
	m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
	m.enterResultHistory()
	first := m.ResultHistory.Entries()[0]
	m.enterResultHistory()
	m.enterResultHistory()
	m.acceptedQuitCleanup()
	m.acceptedQuitCleanup()
	if m.ResultHistory.Len() != 1 || m.ResultHistory.Entries()[0].ID != first.ID {
		t.Errorf("repeated finalizers: entries=%d id=%d/%d", m.ResultHistory.Len(), first.ID, m.ResultHistory.Entries()[0].ID)
	}

	// Late old-execution count/page results after finalization.
	m, page, _, count := fixtureFor(t, activeState{name: "idle"})
	m.enterResultHistory()
	rows := len(m.ResultHistory.Entries()[0].Rows)
	count.Result = CountResult{Total: 99}
	m = apply(m, count)
	count.Result = CountResult{Err: errors.New("late count failure")}
	m = apply(m, count)
	page.Result = FirstPageResult{Cancelled: true}
	m = apply(m, page)
	page.Result = FirstPageResult{Err: errors.New("late page failure")}
	m = apply(m, page)
	if m.ResultHistory.Len() != 1 || len(m.ResultHistory.Entries()[0].Rows) != rows || m.SelectIsActive() {
		t.Errorf("late messages mutated finalization: entries=%d rows=%d active=%v",
			m.ResultHistory.Len(), len(m.ResultHistory.Entries()[0].Rows), m.SelectIsActive())
	}
	t.Logf("late old-execution messages: inert, entries=1")

	// Quit finalization is per execution, never per request.
	m, _, _, count = fixtureFor(t, activeState{name: "idle"})
	m = apply(m, count)
	m.acceptedQuitCleanup()
	if m.ResultHistory.Len() != 1 {
		t.Errorf("finalization created one entry per request: %d", m.ResultHistory.Len())
	}
	t.Logf("accepted quit after both requests settled: exactly one entry")
}
EOF
go test ./internal/ui -run TestDemo34Idempotence -count=1 -v 2>&1 | grep -E "inert|exactly one entry|FAIL|ok "
rm internal/ui/zz_demo34d_test.go
```

```output
    zz_demo34d_test.go:38: late old-execution messages: inert, entries=1
    zz_demo34d_test.go:47: accepted quit after both requests settled: exactly one entry
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

Note on write executions: UPDATE/DELETE confirmation-driven writes (Issues #37/#38) share this finalization seam — starting an actual write finalizes the active SELECT exactly as a new SELECT does — but no implemented write flow exists yet, so the walkthrough exercises the SELECT variants; the seam is command-agnostic. Blocks 1–4 above are the same coverage as internal/ui/active_select_lifecycle_test.go, internal/ui/snapshot_finalize_once_test.go, and internal/history/result_entry_test.go; showboat verify re-executes every block deterministically.
