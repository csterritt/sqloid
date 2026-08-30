// Table-driven Ctrl+S target-resolution coverage for Issue #48, per the
// Query save targeting decision in Notes/PRD-sqloid.md. The pure resolver in
// this package consumes only immutable in-memory state — the viewed result's
// backing query-history entry (Issue #36 association), the current runnable
// builder, the last actual execution, and the terminal Ctrl+P/N selection —
// and never opens a picker, validates, inspects schema, or issues database
// work. Ordinary priority is exactly viewed-result query, runnable builder,
// last execution; terminal priority is exactly selected immutable query,
// last execution. No target yields the exact `no runnable query to save`
// typed error and no target.

package export

import (
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// selectState returns a complete immutable SELECT history state over the
// named table.
func selectState(table string) qb.HistoryState {
	return qb.HistoryState{
		Command:  qb.CommandSelect,
		Table:    table,
		TableSet: true,
		Projection: []qb.HistoryProjectionEntry{
			{Kind: qb.ProjectionColumn, Column: "id"},
		},
	}
}

// viewedResult associates a finalized result entry with the query-history
// entry carrying the given stable ID, mirroring how a real finalized SELECT
// execution records its backing query-history entry. It returns the retained
// entry's stable ID (the ID the UI would browse to).
func viewedResult(t *testing.T, rs *history.ResultStore, execID uint64, queryID history.EntryID) history.EntryID {
	t.Helper()
	retained, ok := rs.AppendFinalized(history.ResultEntry{
		ExecutionID:  execID,
		Kind:         history.KindTabular,
		Columns:      []string{"id"},
		QueryEntryID: queryID,
	})
	if !ok {
		t.Fatal("setup: result entry was rejected")
	}
	return retained.ID
}

// viewedQueryThroughAssociation resolves the viewed result's query exactly as
// the UI must: result entry -> backing query-history entry, never through
// any rendered text. A false result means the association does not resolve.
func viewedQuery(t *testing.T, rs *history.ResultStore, s *history.Store, viewedID history.EntryID) (qb.HistoryState, bool) {
	t.Helper()
	entry, ok := rs.Lookup(viewedID)
	if !ok {
		return qb.HistoryState{}, false
	}
	if entry.QueryEntryID == 0 {
		return qb.HistoryState{}, false
	}
	qe, ok := s.Lookup(entry.QueryEntryID)
	if !ok {
		return qb.HistoryState{}, false
	}
	return qe.State, true
}

// linkedViewedResult wires a result store whose viewed entry (the ID the UI
// would browse to) is associated with the given query-history entry.
func linkedViewedResult(t *testing.T, queryID history.EntryID) (*history.ResultStore, history.EntryID) {
	t.Helper()
	rs := history.NewResultStore()
	viewedID := viewedResult(t, rs, 101, queryID)
	return rs, viewedID
}

// stateFor returns the immutable state behind one query-history entry ID.
// Callers run it from an active test; the entry must be retained.
func stateFor(s *history.Store, id history.EntryID) qb.HistoryState {
	e, ok := s.Lookup(id)
	if !ok {
		panic("setup: query entry not retained")
	}
	return e.State
}

func ptrState(s qb.HistoryState) *qb.HistoryState { return &s }

// TestOrdinarySaveTargetPriority covers every presence combination among the
// viewed-result query, a runnable builder, and the last actual execution,
// plus absent and non-runnable builders: the viewed historical result's
// associated query always wins, then the runnable builder, then the last
// execution; no target reports the exact no-runnable-query feedback.
func TestOrdinarySaveTargetPriority(t *testing.T) {
	viewed := selectState("viewed_table")
	builder := selectState("builder_table")
	last := selectState("last_table")

	cases := []struct {
		name       string
		viewed     bool
		builder    bool
		runnable   bool
		last       bool
		wantSource SQLSaveSource
		wantState  qb.HistoryState
		wantErr    error
	}{
		{name: "all present chooses viewed result query", viewed: true, builder: true, runnable: true, last: true, wantSource: SaveFromViewedResult, wantState: viewed},
		{name: "viewed beats runnable builder", viewed: true, builder: true, runnable: true, wantSource: SaveFromViewedResult, wantState: viewed},
		{name: "viewed beats last execution", viewed: true, last: true, wantSource: SaveFromViewedResult, wantState: viewed},
		{name: "viewed alone", viewed: true, wantSource: SaveFromViewedResult, wantState: viewed},
		{name: "runnable builder beats last execution", builder: true, runnable: true, last: true, wantSource: SaveFromRunnableBuilder, wantState: builder},
		{name: "builder alone", builder: true, runnable: true, wantSource: SaveFromRunnableBuilder, wantState: builder},
		{name: "non-runnable builder falls to last execution", builder: true, last: true, wantSource: SaveFromLastExecution, wantState: last},
		{name: "last execution alone", last: true, wantSource: SaveFromLastExecution, wantState: last},
		{name: "nothing to save", wantErr: ErrNoRunnableQuery},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := history.NewStore()
			viewedID := store.Append(viewed)
			lastID := store.Append(last)

			in := SQLSaveInput{}
			if tc.viewed {
				rs, viewedResultID := linkedViewedResult(t, viewedID)
				// The UI obtains the viewed association from the result
				// entry's backing immutable history entry, never from text.
				entry, ok := rs.Lookup(viewedResultID)
				if !ok {
					t.Fatal("setup: viewed result entry not retained")
				}
				qe, ok := store.Lookup(entry.QueryEntryID)
				if !ok {
					t.Fatal("setup: associated query entry not retained")
				}
				in.ViewedResultQuery = &qe.State
			}
			if tc.builder {
				state := builder
				in.Builder = &state
				in.BuilderRunnable = tc.runnable
			}
			if tc.last {
				e := stateFor(store, lastID)
				in.LastExecution = &e
			}

			got, err := ResolveSQLSaveTarget(in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSQLSaveTarget returned %v, want a target", err)
			}
			if got.Source != tc.wantSource {
				t.Fatalf("source = %v, want %v", got.Source, tc.wantSource)
			}
			if !got.State.Equal(tc.wantState) {
				t.Fatalf("target state = %+v, want the winning candidate", got.State)
			}
		})
	}
}

// TestOrdinarySaveTargetNoTargetFeedback requires the exact no-target error
// and no target when nothing exists to save: no picker is prepared and no
// serialization can start.
func TestOrdinarySaveTargetNoTargetFeedback(t *testing.T) {
	_, err := ResolveSQLSaveTarget(SQLSaveInput{})
	if err == nil || err.Error() != "no runnable query to save" {
		t.Fatalf("no-target error = %v, want exactly `no runnable query to save`", err)
	}
	if !errors.Is(err, ErrNoRunnableQuery) {
		t.Fatalf("error %v is not ErrNoRunnableQuery", err)
	}
}

// TestEvictedAssociationFallsThrough requires the viewed-result priority to
// fall through to the next candidate when the associated query-history entry
// was evicted, so the association no longer resolves in memory.
func TestEvictedAssociationFallsThrough(t *testing.T) {
	store := history.NewStore()
	viewedID := store.Append(selectState("viewed_table"))
	last := selectState("last_table")
	lastID := store.Append(last)
	// Evict the associated viewed-result query entry by exceeding capacity.
	for i := 0; i < history.Capacity-1; i++ {
		store.Append(selectState("filler"))
	}
	if _, ok := store.Lookup(viewedID); ok {
		t.Fatal("setup: viewed query entry was not evicted")
	}

	// The UI resolves the association through the result entry's ID; with
	// the backing entry evicted it supplies no viewed-result query.
	in := SQLSaveInput{LastExecution: ptrState(stateFor(store, lastID))}
	got, err := ResolveSQLSaveTarget(in)
	if err != nil {
		t.Fatalf("ResolveSQLSaveTarget returned %v, want last execution", err)
	}
	if got.Source != SaveFromLastExecution {
		t.Fatalf("source = %v, want SaveFromLastExecution", got.Source)
	}
	if got.State.Table != "last_table" {
		t.Fatalf("target table = %q, want last_table", got.State.Table)
	}
}

// TestTerminalSaveTargetPriority requires terminal states to ignore the
// viewed-result query and the current builder entirely: only the Ctrl+P/N
// selected immutable query wins, otherwise the last actual execution.
func TestTerminalSaveTargetPriority(t *testing.T) {
	viewed := selectState("viewed_table")
	builder := selectState("builder_table")
	last := deleteState("last_table")
	selected := selectState("selected_table")

	cases := []struct {
		name       string
		selection  bool
		last       bool
		wantSource SQLSaveSource
		wantTable  string
	}{
		{name: "selected query wins over last execution", selection: true, last: true, wantSource: SaveFromTerminalSelection, wantTable: "selected_table"},
		{name: "selected query alone", selection: true, wantSource: SaveFromTerminalSelection, wantTable: "selected_table"},
		{name: "no selection falls to last execution", last: true, wantSource: SaveFromLastExecution, wantTable: "last_table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := history.NewStore()
			store.Append(viewed)
			store.Append(builder)
			selID := store.Append(selected)
			lastID := store.Append(last)

			in := SQLSaveInput{Terminal: true}
			// The viewed-result association and the runnable builder are
			// deliberately supplied: terminal priority must ignore both.
			_, viewedResultID := linkedViewedResult(t, store.Entries()[0].ID)
			if tc.selection {
				e := stateFor(store, selID)
				in.TerminalSelection = &e
			}
			if tc.last {
				e := stateFor(store, lastID)
				in.LastExecution = &e
			}
			in.Builder = ptrState(builder)
			in.BuilderRunnable = true
			e := stateFor(store, store.Entries()[0].ID)
			in.ViewedResultQuery = &e
			_ = viewedResultID

			got, err := ResolveSQLSaveTarget(in)
			if err != nil {
				t.Fatalf("ResolveSQLSaveTarget returned %v", err)
			}
			if got.Source != tc.wantSource {
				t.Fatalf("source = %v, want %v", got.Source, tc.wantSource)
			}
			if got.State.Table != tc.wantTable {
				t.Fatalf("target table = %q, want %q", got.State.Table, tc.wantTable)
			}
		})
	}
}

// TestTerminalSaveTargetNoTargetFeedback requires the exact no-target
// feedback when a terminal state has neither a selected query nor a last
// actual execution.
func TestTerminalSaveTargetNoTargetFeedback(t *testing.T) {
	_, err := ResolveSQLSaveTarget(SQLSaveInput{Terminal: true})
	if err == nil || err.Error() != "no runnable query to save" {
		t.Fatalf("no-target error = %v, want exact `no runnable query to save`", err)
	}
}

// TestTargetResolutionIsImmutableMemoryOnly proves resolution consumes only
// immutable in-memory state: the resolved state is a fresh deep copy whose
// later mutation can never alter retained history, and the resolution issues
// zero validation, schema, connection, or database work by construction.
func TestTargetResolutionIsImmutableMemoryOnly(t *testing.T) {
	store := history.NewStore()
	lastID := store.Append(selectState("last_table"))

	in := SQLSaveInput{LastExecution: ptrState(stateFor(store, lastID))}
	got, err := ResolveSQLSaveTarget(in)
	if err != nil {
		t.Fatalf("resolution failed: %v", err)
	}
	// Mutating the resolved copy must not reach the retained entry.
	got.State.Table = "mutated"
	e, _ := store.Lookup(lastID)
	if e.State.Table != "last_table" {
		t.Fatalf("resolution aliased retained history: retained table = %q", e.State.Table)
	}
}
