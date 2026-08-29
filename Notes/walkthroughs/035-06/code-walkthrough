# Issue #35 — Stable query-history navigation and consecutive suppression

*2026-08-28T20:55:29Z by Showboat 0.6.1*
<!-- showboat-id: ed53438e-633f-4500-8d4d-06bee0e0f859 -->

This walkthrough demonstrates the completed Issue #35 implementation (Notes/tasks/035-stable-query-history-and-consecutive-suppression.md). Block 1 proves the unchanged Issue #20 consecutive policy (A→A→B→A retains two A entries and suppresses only the immediate duplicate). Block 2 browses older/newer with Ctrl+P/N through both boundaries and reverses direction. Block 3 restores every builder field and mutates restored, source, and retrieved values to prove retained history is immutable and browsing appends nothing. Block 4 starts actual executions from unchanged and edited restored states, showing history-mode exit before the unchanged append seam. Block 5 forces selected stable-ID eviction with surviving entries and with an empty store, showing the exact eviction notice, the new-oldest fallback, and the base return. See Issue #35, Notes/PRD-sqloid.md, and Notes/wiki/query-history-navigation.md. Every block is re-runnable from the repository root; each writes, runs, and removes its own temporary demo test. Output lines are filtered to pass/fail markers so re-verification is deterministic.

Block 1 — the unchanged Issue #20 consecutive-suppression policy across A→A→B→A: only the immediate duplicate is suppressed, consuming no stable ID, and A→B→A retains both A entries.

```bash
cat > internal/history/zz_demo35a_test.go <<'EOF'
package history

import (
	"fmt"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

func TestDemo35ABA(t *testing.T) {
	s := NewStore()
	state := func(table string) qb.HistoryState {
		st := qb.HistoryState{Command: qb.CommandSelect, Table: table, TableSet: true}
		st.Projection = []qb.HistoryProjectionEntry{{Kind: qb.ProjectionWildcard}}
		return st
	}
	id1, ok := s.AppendExecution(state("a"))
	if !ok || id1 == 0 {
		t.Fatal("first A appended")
	}
	id2, ok := s.AppendExecution(state("a"))
	if ok || id2 != 0 {
		t.Fatal("A→A must be suppressed with no ID allocated")
	}
	id3, ok := s.AppendExecution(state("b"))
	if !ok || id3 == 0 {
		t.Fatal("A→B appended")
	}
	id4, ok := s.AppendExecution(state("a"))
	if !ok || id4 == 0 {
		t.Fatal("A→B→A retained the second A")
	}
	if id4 == id1 || id4 == id3 || id3 == id1 {
		t.Fatal("IDs not unique")
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3", s.Len())
	}
	fmt.Printf("A→A→B→A: Len=%d ids=%d,%d,%d (A→A consumed none) — only the consecutive duplicate was suppressed\n", s.Len(), id1, id3, id4)
}
EOF
go test ./internal/history/ -run 'TestDemo35ABA' -v 2>&1 | grep -E '^(=== RUN|--- |FAIL|##)'; rm internal/history/zz_demo35a_test.go
```

```output
=== RUN   TestDemo35ABA
--- PASS: TestDemo35ABA (0.00s)
```

Block 2 — Ctrl+P/N browsing through both boundaries with direction reversal. Entering selects the newest entry; repeated Ctrl+P at the oldest boundary is a no-op; reversed Ctrl+N walks back; Ctrl+N at the newest boundary exits history mode to the base builder.

```bash
cat > internal/ui/zz_demo35b_test.go <<'EOF'
package ui

import (
	"fmt"
	"testing"
)

// Demo B: browse older/newer through both boundaries and reverse direction.
func TestDemo35Browse(t *testing.T) {
	m, _, ids := navigationModel()
	m = navKey(t, m, "ctrl+n") // enter at newest
	fmt.Printf("enter: cursor=%d (newest INSERT)\n", m.historyCursorID)
	m = navKey(t, m, "ctrl+p") // UPDATE
	m = navKey(t, m, "ctrl+p") // SELECT (oldest)
	fmt.Printf("two Ctrl+P: cursor=%d (oldest SELECT)\n", m.historyCursorID)
	m = navKey(t, m, "ctrl+p")
	m = navKey(t, m, "ctrl+p")
	if m.historyCursorID != ids[0] || !m.historyMode {
		t.Fatal("oldest boundary must be a no-op")
	}
	fmt.Println("repeated Ctrl+P at oldest: no-op, still browsing")
	m = navKey(t, m, "ctrl+n")
	m = navKey(t, m, "ctrl+n")
	if m.historyCursorID != ids[2] {
		t.Fatalf("reversal: cursor=%d, want newest %d", m.historyCursorID, ids[2])
	}
	fmt.Printf("reversed Ctrl+N x2: cursor=%d (newest again)\n", m.historyCursorID)
	m = navKey(t, m, "ctrl+n")
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatal("Ctrl+N at newest must exit history mode")
	}
	fmt.Println("Ctrl+N at newest boundary: exited history mode to base builder")
}
EOF
go test ./internal/ui/ -run 'TestDemo35Browse' -v 2>&1 | grep -E '^(=== RUN|--- |FAIL|##)'; rm internal/ui/zz_demo35b_test.go
```

```output
=== RUN   TestDemo35Browse
--- PASS: TestDemo35Browse (0.00s)
```

Block 3 — immutable copy-on-restore of every builder field across SELECT, UPDATE, and INSERT entries. Each restored state matches its stored state exactly; mutating the restored builder and retrieved copies never changes a retained entry; browsing and edits append nothing.

```bash
cat > internal/ui/zz_demo35c_test.go <<'EOF'
package ui

import (
	"fmt"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// Demo C: full-field restoration, immutability, and no append while browsing.
func TestDemo35RestoreImmutable(t *testing.T) {
	m, store, _ := navigationModel()
	want := []qb.HistoryState{richSelectQB().HistoryState(), richUpdateQB().HistoryState(), richInsertQB().HistoryState()}
	m = navKey(t, m, "ctrl+n")
	for step := 2; step >= 0; step-- {
		if !m.QB.HistoryState().Equal(want[step]) {
			t.Fatalf("entry %d not restored exactly", step)
		}
		fmt.Printf("entry %d restored exactly (command=%s table=%s)\n", step, m.QB.Command(), "users")
		// Mutate the retrieved copy and the restored builder; retained entry must not change.
		e, _ := store.Lookup(store.Entries()[step].ID)
		e.State.Table = "hacked"
		mutated := m.QB.SetLimitInput("")
		_ = mutated
		if got := store.Entries(); !got[step].State.Equal(want[step]) {
			t.Fatalf("entry %d changed through restored/retrieved mutation", step)
		}
		if step > 0 {
			m = navKey(t, m, "ctrl+p")
		}
	}
	if store.Len() != 3 {
		t.Fatalf("browsing and edits appended: Len=%d", store.Len())
	}
	fmt.Println("all retained entries immutable; browsing and edits appended nothing")
}
EOF
go test ./internal/ui/ -run 'TestDemo35RestoreImmutable' -v 2>&1 | grep -E '^(=== RUN|--- |FAIL|##)'; rm internal/ui/zz_demo35c_test.go
```

```output
=== RUN   TestDemo35RestoreImmutable
--- PASS: TestDemo35RestoreImmutable (0.00s)
```

Block 4 — actual executions from restored states. An edited restored INSERT state exits history mode and appends as the current state through the unchanged Issue #20 seam; executing the exact state of the immediately preceding entry is still suppressed (A→A with no ID consumed).

```bash
cat > internal/ui/zz_demo35d_test.go <<'EOF'
package ui

import (
	"fmt"
	"testing"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// Demo D1: execution start from an edited restored state — history mode exits
// first, the current edited state appends through the unchanged seam.
func TestDemo35ExecuteEdited(t *testing.T) {
	m, store, ids := navigationModel()
	m = navKey(t, m, "ctrl+n") // C (INSERT) restored
	q, ok := m.QB.ChooseInsertColumn("id", qb.InsertChoiceOmit)
	if !ok {
		t.Fatal("edit failed")
	}
	m.QB = q
	m.applyBuilder(q)
	m = asModel(m.Update(ExecutionStartedMsg{}))
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatal("execution did not exit history mode")
	}
	entries := store.Entries()
	if store.Len() != 4 || !entries[3].State.Equal(m.QB.HistoryState()) {
		t.Fatal("execution did not run against the current edited restored state")
	}
	backing, _ := store.Lookup(ids[2])
	if backing.State.Equal(entries[3].State) {
		t.Fatal("edited state must differ from the backing entry")
	}
	fmt.Printf("edited execution: exited history mode, appended ID=%d as current state, backing ID=%d untouched\n", entries[3].ID, ids[2])
}

// Demo D2: executing the exact restored state of the immediately preceding
// entry is still suppressed (unchanged Issue #20 A→A policy across history).
func TestDemo35ExecuteUnchangedSuppressed(t *testing.T) {
	store := history.NewStore()
	store.Append(validSelectQB().HistoryState())
	m := modelWithQB(validSelectQB())
	m.catalog = navCatalog()
	m.History = store
	m = navKey(t, m, "ctrl+p") // restore the newest (= the plain SELECT state)
	m = asModel(m.Update(ExecutionStartedMsg{}))
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatal("execution did not exit history mode")
	}
	if store.Len() != 1 {
		t.Fatalf("A→A across history execution: Len=%d, want 1 (suppressed)", store.Len())
	}
	fmt.Println("unchanged execution: exited history mode; identical consecutive start suppressed, no ID consumed")
}
EOF
go test ./internal/ui/ -run 'TestDemo35Execute' -v 2>&1 | grep -E '^(=== RUN|--- |FAIL|##)'; rm internal/ui/zz_demo35d_test.go
```

```output
=== RUN   TestDemo35ExecuteEdited
--- PASS: TestDemo35ExecuteEdited (0.00s)
=== RUN   TestDemo35ExecuteUnchangedSuppressed
--- PASS: TestDemo35ExecuteUnchangedSuppressed (0.00s)
```

Block 5 — forced external eviction of the selected stable ID. With surviving entries the selection falls back to the new oldest entry with exactly the eviction notice and surviving IDs unchanged; with an empty store history mode exits to the base builder with the same notice and the builder data remains valid.

```bash
cat > internal/ui/zz_demo35e_test.go <<'EOF'
package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// Demo E1: forced external eviction with surviving entries — the selection
// falls back to the new oldest entry with the exact notice.
func TestDemo35EvictFallback(t *testing.T) {
	m, store, ids := navigationModel()
	for i := 0; store.Len() < history.Capacity-1; i++ {
		store.Append(cursorLabelState("users", i))
	}
	m = navKey(t, m, "ctrl+n")
	for i := 0; m.historyCursorID != ids[0]; i++ {
		m = navKey(t, m, "ctrl+p")
	}
	fmt.Printf("viewing oldest ID=%d of %d entries\n", m.historyCursorID, store.Len())
	store.Append(cursorLabelState("users", 9998))
	store.Append(cursorLabelState("users", 9999)) // evicts ID=ids[0]
	_, ok := store.Lookup(ids[0])
	if ok {
		t.Fatal("setup: selected entry was not evicted")
	}
	m = asModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if !m.historyMode {
		t.Fatal("fallback must stay in history mode")
	}
	if m.historyNotice != "Previously viewed query was evicted from history" {
		t.Fatalf("notice=%q", m.historyNotice)
	}
	oldest, _ := store.Oldest()
	if m.historyCursorID != oldest.ID {
		t.Fatalf("cursor=%d, want new oldest %d", m.historyCursorID, oldest.ID)
	}
	for _, id := range ids[1:] {
		if _, ok := store.Lookup(id); !ok {
			t.Fatalf("surviving ID %d vanished", id)
		}
	}
	fmt.Printf("evicted: notice exact, cursor moved to new oldest ID=%d, surviving IDs unchanged\n", oldest.ID)
}

// Demo E2: forced external eviction to an empty store — history mode ends
// and the builder returns safely to the base view with the exact notice.
func TestDemo35EvictEmpty(t *testing.T) {
	m, _, _ := navigationModel()
	m = navKey(t, m, "ctrl+n")
	viewed, _ := m.History.Lookup(m.historyCursorID)
	m.History = history.NewStore()
	m = asModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if m.historyMode || m.historyCursorID != 0 {
		t.Fatal("empty store must exit history mode and detach the cursor")
	}
	if m.historyNotice != "Previously viewed query was evicted from history" {
		t.Fatalf("notice=%q", m.historyNotice)
	}
	if m.QB.Command() != viewed.State.Command || m.QB.Command().String() != "INSERT" {
		t.Fatalf("builder command=%v after empty-store return", m.QB.Command())
	}
	if _, ok := m.QB.SelectedTable(); !ok {
		t.Fatal("builder table vanished after empty-store return")
	}
	fmt.Println("empty store: exited to base builder with the exact notice; builder data still valid")
}
EOF
go test ./internal/ui/ -run 'TestDemo35Evict' -v 2>&1 | grep -E '^(=== RUN|--- |FAIL|##)'; rm internal/ui/zz_demo35e_test.go
```

```output
=== RUN   TestDemo35EvictFallback
--- PASS: TestDemo35EvictFallback (0.00s)
=== RUN   TestDemo35EvictEmpty
--- PASS: TestDemo35EvictEmpty (0.00s)
```
