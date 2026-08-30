# Issue #15: SELECT wildcard and COUNT(*) projection path — code walkthrough

*2026-08-27T08:54:52Z by Showboat 0.6.1*
<!-- showboat-id: ca772020-619f-48c6-8762-4655441c7a93 -->


Evidence for Issue #15 per Notes/tasks/015-select-wildcard-and-count-projection-path.md, implementing the empty Column(s) sequence and sentinel visibility rules of the Query Grammar in Notes/PRD-sqloid.md: `*` default-highlighted first, bare `COUNT(*)` second, named columns after both; direct sentinel commits that immediately reopen Column(s); named-column routing through the scroll-only Value/Count/Min/Max/Avg/Sum aggregate popup; sentinel hiding while entries exist and reappearance when removal empties the projection; wildcard as a direct sole-projection commit; distinct identities for real columns named `*` or `COUNT(*)`; and `MIN(*)`/`MAX(*)`/`AVG(*)`/`SUM(*)` absent by construction. Workdir: sqloid repo root. Blocks 1-2 mount a short-lived scripted test file inside internal/ui and remove it again within the same command; every go-test timing string is normalized so blocks re-execute byte-identically.


## 1. Empty Column(s): `*` default-highlighted first, bare `COUNT(*)` second, names after — searchable modality on

```bash
cat > internal/ui/zz_demo_stage1_test.go <<'SQLOID_DEMO_EOF'
package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// TestDemoEmptyColumnsPopup renders the real empty Column(s) popup exactly as
// Issue #15 Task 6 requires: * first and default-highlighted, bare COUNT(*)
// second, named columns after both, searchable modality active, and no
// aggregate-on-wildcard choice anywhere in the render.
func TestDemoEmptyColumnsPopup(t *testing.T) {
	m := openColumnsPopup(t)
	fmt.Println("-- rendered empty Column(s) popup --")
	fmt.Println(RenderPopup(m.Popup, 60, 12))
	got := visibleCandidates(m)
	wildcard := qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}.Key()
	sentinel := qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key()
	if got[0] != wildcard || got[1] != sentinel {
		t.Fatalf("ordering wrong: %v", got)
	}
	if hc, _ := m.Popup.Highlighted(); hc.ID != wildcard {
		t.Fatalf("default highlight %+v, want wildcard *", hc)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	view := RenderPopup(m.Popup, 60, 12)
	for _, bad := range []string{"MIN(*)", "MAX(*)", "AVG(*)", "SUM(*)"} {
		if strings.Contains(view, bad) {
			t.Fatalf("unsupported wildcard aggregate %q rendered:\n%s", bad, view)
		}
	}
	fmt.Println("-- search narrows to id while rows stay sourced from Schema order --")
	fmt.Println(view)
	m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}).(Model)
	fmt.Println(RenderPopup(m.Popup, 60, 12))
}
SQLOID_DEMO_EOF
go test ./internal/ui -count=1 -v -run TestDemoEmptyColumnsPopup 2>&1 | grep -E . | sed -E 's/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g; s/\t[0-9]+\.[0-9]+s$/\t(t)/'
rc=$?
rm -f internal/ui/zz_demo_stage1_test.go
exit $rc
```

```output
=== RUN   TestDemoEmptyColumnsPopup
-- rendered empty Column(s) popup --
╭────────────╮
│Search: _   │
│> *         │
│  COUNT(*)  │
│  id        │
│  email     │
╰────────────╯
-- search narrows to id while rows stay sourced from Schema order --
╭────────────╮
│Search: _   │
│  *         │
│> COUNT(*)  │
│  id        │
│  email     │
╰────────────╯
╭─────────────╮
│Search: id_  │
│> id         │
╰─────────────╯
--- PASS: TestDemoEmptyColumnsPopup (t)
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```


## 2. Sentinel commit and immediate reopen, aggregate routing, preservation, restoration

```bash
cat > internal/ui/zz_demo_stage2_test.go <<'SQLOID_DEMO_EOF'
package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// TestDemoSentinelAndAggregateFlow walks the real scripted flow: bare
// COUNT(*) commits directly and reopens Column(s) without any aggregate step;
// a named column routes to the scroll-only Value/Count/Min/Max/Avg/Sum popup;
// Avg completion reopens Column(s) preserving the completed entry; removal
// back to empty restores the sentinel in second position.
func TestDemoSentinelAndAggregateFlow(t *testing.T) {
	m := openColumnsPopup(t)
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	fmt.Println("-- after accepting bare COUNT(*): committed directly, popup reopened --")
	if entries := m.QB.ProjectionEntries(); len(entries) != 1 || entries[0].Kind != qb.ProjectionCountStar {
		t.Fatalf("sentinel not committed directly: %v", entries)
	}
	if content := m.Fields[2].Content; content != "COUNT(*)" {
		t.Fatalf("field bar content %q, want COUNT(*)", content)
	}
	fmt.Println(RenderPopup(m.Popup, 60, 12))

	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // id -> aggregate choice
	aggView := RenderPopup(m.Popup, 60, 12)
	fmt.Println("-- named id routed to the scroll-only aggregate popup --")
	fmt.Println(aggView)
	if strings.Contains(aggView, "Search:") {
		t.Fatal("aggregate popup must be scroll-only")
	}
	for _, bad := range []string{"MIN(*)", "MAX(*)", "AVG(*)", "SUM(*)"} {
		if strings.Contains(aggView, bad) {
			t.Fatalf("unsupported wildcard aggregate %q rendered:\n%s", bad, aggView)
		}
	}
	for i := 0; i < 4; i++ {
		m = drive(m, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	}
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model) // Avg accepted
	fmt.Println("-- Avg completion reopened Column(s), preserving COUNT(*) --")
	if content := m.Fields[2].Content; content != "COUNT(*), id(AVG)" {
		t.Fatalf("field bar content %q, want COUNT(*), id(AVG)", content)
	}
	fmt.Println(RenderPopup(m.Popup, 60, 12))

	for !m.QB.ProjectionEmpty() { // Issue #16 owns removal keys; use the seam
		m.applyBuilder(m.QB.RemoveProjection(len(m.QB.ProjectionEntries()) - 1))
	}
	m.reopenColumnsPopup()
	got := visibleCandidates(m)
	wildcard := qb.ProjectionCandidate{Kind: qb.ProjectionWildcard}.Key()
	sentinel := qb.ProjectionCandidate{Kind: qb.ProjectionCountStar}.Key()
	if len(got) != 4 || got[0] != wildcard || got[1] != sentinel {
		t.Fatalf("emptied candidates=%v, want sentinel restored in second position", got)
	}
	fmt.Println("-- removal back to empty restores * first and COUNT(*) second --")
	fmt.Println(RenderPopup(m.Popup, 60, 12))
}

// TestDemoWildcardDirectCommit proves wildcard commits as the sole projection
// straight from the Column(s) popup with no aggregate step and no reopen.
func TestDemoWildcardDirectCommit(t *testing.T) {
	m := openColumnsPopup(t)
	next := drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if next.Popup != nil {
		t.Fatal("wildcard acceptance must not leave a popup open")
	}
	if content := next.Fields[2].Content; content != "*" {
		t.Fatalf("field bar content %q, want sole wildcard committed directly", content)
	}
	if cands := next.QB.ProjectionCandidates(); len(cands) != 0 {
		t.Fatalf("wildcard left candidates %v, want none", cands)
	}
	fmt.Println("-- wildcard committed directly as sole projection; popup closed --")
}
SQLOID_DEMO_EOF
go test ./internal/ui -count=1 -v -run 'TestDemo(SentinelAndAggregateFlow|WildcardDirectCommit)' 2>&1 | grep -E . | sed -E 's/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g; s/\t[0-9]+\.[0-9]+s$/\t(t)/'
rc=$?
rm -f internal/ui/zz_demo_stage2_test.go
exit $rc
```

```output
=== RUN   TestDemoSentinelAndAggregateFlow
-- after accepting bare COUNT(*): committed directly, popup reopened --
╭───────────╮
│Search: _  │
│> id       │
│  email    │
╰───────────╯
-- named id routed to the scroll-only aggregate popup --
╭─────────╮
│> Value  │
│  Count  │
│  Min    │
│  Max    │
│  Avg    │
│  Sum    │
╰─────────╯
-- Avg completion reopened Column(s), preserving COUNT(*) --
╭───────────╮
│Search: _  │
│> id       │
│  email    │
╰───────────╯
-- removal back to empty restores * first and COUNT(*) second --
╭────────────╮
│Search: _   │
│> *         │
│  COUNT(*)  │
│  id        │
│  email     │
╰────────────╯
--- PASS: TestDemoSentinelAndAggregateFlow (t)
=== RUN   TestDemoWildcardDirectCommit
-- wildcard committed directly as sole projection; popup closed --
--- PASS: TestDemoWildcardDirectCommit (t)
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```


## 3. Identity separation and unrepresentable wildcard aggregates

Pure querybuilder evidence: real columns named `*` and `COUNT(*)` keep distinct identities despite colliding display text, and every attempt to build MIN(*)/MAX(*)/AVG(*)/SUM(*) is rejected by construction.

```bash
go test ./internal/querybuilder -count=1 -v -run 'TestSentinelLikeColumnNamesKeepDistinctIdentity|TestWildcardAggregatesAreUnrepresentableByConstruction' 2>&1 | grep -E . | sed -E 's/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g; s/\t[0-9]+\.[0-9]+s$/\t(t)/'
```

```output
=== RUN   TestSentinelLikeColumnNamesKeepDistinctIdentity
--- PASS: TestSentinelLikeColumnNamesKeepDistinctIdentity (t)
=== RUN   TestWildcardAggregatesAreUnrepresentableByConstruction
--- PASS: TestWildcardAggregatesAreUnrepresentableByConstruction (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
```


## 4. Full suite green

Every querybuilder and UI test passes, including all Issue #15 red-green contracts from Tasks 1-3.

```bash
go test ./internal/querybuilder ./internal/ui 2>&1 | sed -E 's/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g; s/\t[0-9]+\.[0-9]+s$/\t(t)/'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
ok  	github.com/chris/sqloid/internal/ui	(t)
```

