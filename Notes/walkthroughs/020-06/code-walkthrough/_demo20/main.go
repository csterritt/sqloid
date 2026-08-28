// Demonstration program for the Issue #20 code walkthrough: minimal
// query-history append (stable ID + consecutive suppression). It exercises
// the internal/history store, the internal/querybuilder normalized state, and
// (via the seam message) the internal/ui append timing contract.

package main

import (
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris/sqloid/internal/history"
	"github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
	"github.com/chris/sqloid/internal/ui"
)

func state(name string) querybuilder.HistoryState {
	return querybuilder.HistoryState{
		Command:    querybuilder.CommandSelect,
		Table:      name,
		TableSet:   true,
		Projection: []querybuilder.HistoryProjectionEntry{{Kind: querybuilder.ProjectionWildcard}},
	}
}

func richState() querybuilder.HistoryState {
	return querybuilder.HistoryState{
		Command:       querybuilder.CommandUpdate,
		Table:         "items",
		TableSet:      true,
		WhereSet:      true,
		WhereColumn:   "name",
		WhereOperator: querybuilder.OpEq,
		WhereHasValue: true,
		WhereValue:    querybuilder.Value{Kind: querybuilder.KindInteger, Int: 7},
		WhereEntered:  "7",
		Groups:        []string{"name", "score"},
		Sets: []querybuilder.HistorySetAssignment{
			{Column: "name", Choice: querybuilder.SetChoiceValue, HasValue: true,
				Value: querybuilder.Value{Kind: querybuilder.KindText, Text: "x"}, Entered: "x"},
			{Column: "score", Choice: querybuilder.SetChoiceNull},
		},
	}
}

func section(title string) { fmt.Println("\n== " + title + " ==") }

func main() {
	// 1. Stable non-positional IDs surviving eviction.
	section("1. Stable non-positional IDs, capacity 20, oldest-first eviction")
	s := history.NewStore()
	ids := make([]history.EntryID, 0, 25)
	for i := 0; i < 25; i++ {
		ids = append(ids, s.Append(state(fmt.Sprintf("t%d", i))))
	}
	fmt.Printf("Capacity=%d  Len after 25 appends=%d\n", history.Capacity, s.Len())
	fmt.Println("first five IDs all evicted:", func() []bool {
		var out []bool
		for _, id := range ids[:5] {
			_, ok := s.Lookup(id)
			out = append(out, ok)
		}
		return out
	}())
	entries := s.Entries()
	fmt.Println("oldest surviving entry: index 5 -> ID", entries[0].ID, "want", ids[5], "table", entries[0].State.Table)

	// 2. Immutable stored copies after source and retrieval mutation.
	section("2. Immutable stored copies")
	full := history.NewStore()
	id := full.Append(richState())
	src := richState()
	src.Sets[0].Column = "MUTATED"
	src.Groups[0] = "MUTATED"
	src.WhereValue.Int = 99
	got, _ := full.Lookup(id)
	fmt.Println("source mutated, retained unchanged:", got.State.Equal(richState()))
	retrieved, _ := full.Lookup(id)
	retrieved.State.Sets[0].Column = "MUTATED"
	retrieved.State.Groups[0] = "MUTATED"
	got, _ = full.Lookup(id)
	fmt.Println("retrieval mutated, store unchanged:", got.State.Equal(richState()))

	// 3. Normalized comparison differences.
	section("3. Normalized comparison differences")
	base := richState()
	diffs := map[string]func(querybuilder.HistoryState) querybuilder.HistoryState{
		"command": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.Command = querybuilder.CommandInsert
			return x
		},
		"table": func(x querybuilder.HistoryState) querybuilder.HistoryState { x.Table = "logs"; return x },
		"projection order": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.Projection = []querybuilder.HistoryProjectionEntry{{Kind: querybuilder.ProjectionColumn, Column: "b"}, {Kind: querybuilder.ProjectionColumn, Column: "a"}}
			x.Table = "items2"
			return x
		},
		"where operator": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.WhereOperator = querybuilder.OpLt
			return x
		},
		"where entered representation": func(x querybuilder.HistoryState) querybuilder.HistoryState { x.WhereEntered = "07"; return x },
		"where bound type": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.WhereValue = querybuilder.Value{Kind: querybuilder.KindText, Text: "7"}
			return x
		},
		"group order": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.Groups = []string{"score", "name"}
			return x
		},
		"order direction": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.OrderSet = true
			x.OrderDirection = querybuilder.DirDesc
			return x
		},
		"limit empty vs number": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.LimitHas = true
			x.LimitValue = 7
			return x
		},
		"limit number": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.LimitHas = true
			x.LimitValue = 6
			return x
		},
		"update choice/value": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			sets := append([]querybuilder.HistorySetAssignment(nil), x.Sets...)
			sets[1] = querybuilder.HistorySetAssignment{Column: "score", Choice: querybuilder.SetChoiceValue, HasValue: true,
				Value: querybuilder.Value{Kind: querybuilder.KindText, Text: "9"}, Entered: "9"}
			x.Sets = sets
			return x
		},
		"insert ordered choices": func(x querybuilder.HistoryState) querybuilder.HistoryState {
			x.Inserts = []querybuilder.HistoryInsertColumn{
				{Column: "id", Choice: querybuilder.InsertChoiceValue, HasValue: true, Value: querybuilder.Value{Kind: querybuilder.KindInteger, Int: 42}, Entered: "42"},
				{Column: "name", Choice: querybuilder.InsertChoiceOmit},
			}
			return x
		},
	}
	names := make([]string, 0, len(diffs))
	for name := range diffs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		other := diffs[name](base)
		fmt.Printf("%-32s equal=%v\n", name, other.Equal(base))
	}
	// Two states that differ only in entered Limit bytes ("5" vs "05") with
	// the same accepted number compare equal: entered Limit bytes are
	// transient once accepted.
	five := base
	five.LimitHas = true
	five.LimitValue = 5
	zeroFive := five
	fmt.Printf("%-32s equal=%v (entered bytes are transient)\n", "limit 5 vs 05 accepted 5", five.Equal(zeroFive))

	// 4. Append policy: A→A→B→A.
	section("4. Append policy A→A→B→A")
	p := history.NewStore()
	a1, ok1 := p.AppendExecution(state("a"))
	a2, ok2 := p.AppendExecution(state("a"))
	b1, ok3 := p.AppendExecution(state("b"))
	a3, ok4 := p.AppendExecution(state("a"))
	fmt.Printf("A appended=%v id=%d; A again appended=%v id=%d (suppressed)\n", ok1, a1, ok2, a2)
	fmt.Printf("B appended=%v id=%d; A again appended=%v id=%d\n", ok3, b1, ok4, a3)
	fmt.Printf("Len=%d order:", p.Len())
	for _, e := range p.Entries() {
		fmt.Printf(" %d:%s", e.ID, e.State.Table)
	}
	fmt.Println()

	// 5. UI timing: no append before execution start; append at start;
	// a later failure cannot undo it.
	section("5. UI execution-start seam timing")
	cat := &schema.Catalog{Version: 1, Objects: []*schema.Object{{
		Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}, {Name: "email"}},
	}}}
	store := history.NewStore()
	m := ui.New()
	step := func(m ui.Model, msg tea.Msg) ui.Model { next, _ := m.Update(msg); return next.(ui.Model) }
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = step(m, ui.SchemaRefreshedMsg{Catalog: cat})
	// Install runnable wildcard-SELECT builder state through the immutable
	// QueryBuilder transitions and focus the opener-free Command base field.
	q := querybuilder.NewQuery().RefreshSchema(cat).SelectCommand(querybuilder.CommandSelect).SelectTable("users")
	q = q.AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionWildcard}).Builder
	m.QB = q
	fmt.Printf("focused field: %s report=%+v\n", m.Fields[m.Focus].Label, m.QB.RunnableReport())
	m.History = store
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(ui.Model)
	_, isPre := cmd().(ui.PreExecutionRequestedMsg)
	fmt.Printf("runnable Enter -> PreExecutionRequestedMsg=%v, history Len=%d (nothing appended)\n", isPre, store.Len())
	m = step(m, ui.PreExecutionRequestedMsg{})
	fmt.Printf("pre-execution seam handled, history Len=%d (still nothing)\n", store.Len())
	m = step(m, ui.ExecutionStartedMsg{})
	fmt.Printf("actual SELECT execution started, history Len=%d\n", store.Len())
	m = step(m, ui.ExecutionStartedMsg{})
	fmt.Printf("identical execution started again, history Len=%d (suppressed)\n", store.Len())
	// The execution later fails; no follow-up event removes the start append.
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	e := store.Entries()
	fmt.Printf("after failure only later, Len=%d retained entry id=%d command=%s\n", store.Len(), e[0].ID, e[0].State.Command)
	// UPDATE never appends without confirmation: build an UPDATE builder
	// through its transitions and emit the execution-start message directly.
	upd := querybuilder.NewQuery().RefreshSchema(cat).SelectCommand(querybuilder.CommandUpdate).SelectTable("users")
	upd, _ = upd.AcceptSetColumn("email")
	upd, _ = upd.ChooseSetAssignment("email", querybuilder.SetChoiceNull)
	m.QB = upd
	m = step(m, ui.ExecutionStartedMsg{})
	fmt.Printf("unconfirmed UPDATE execution-start message, history Len=%d (never appended)\n", store.Len())
}
