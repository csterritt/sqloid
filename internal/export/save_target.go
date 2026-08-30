// UI-independent Ctrl+S save-target resolution (Issue #48), per the Query
// save targeting decision in Notes/PRD-sqloid.md. Resolution is a pure
// function over immutable in-memory state only — the viewed historical
// result's backing query-history entry (Issue #36's immutable association),
// the current builder with its authoritative runnable verdict, the last
// actual execution, and the terminal Ctrl+P/N selection. It never validates,
// refreshes schema, opens a connection, or issues any database work, and it
// never opens a picker or serializes anything: on failure callers receive
// the exact `no runnable query to save` typed feedback and nothing moves
// forward; on success the caller receives one immutable complete query state
// to hand onward.
package export

import (
	"errors"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// ErrNoRunnableQuery is the exact user-facing no-target feedback for Ctrl+S:
// shown inline with no picker opened and no serialization started.
var ErrNoRunnableQuery = errors.New("no runnable query to save")

// SQLSaveSource identifies which immutable candidate a resolved save target
// came from, in ordinary priority order first.
type SQLSaveSource int

const (
	// SaveFromViewedResult marks the query associated with the currently
	// viewed historical result through its backing immutable history entry.
	SaveFromViewedResult SQLSaveSource = iota + 1
	// SaveFromRunnableBuilder marks the current runnable builder state.
	SaveFromRunnableBuilder
	// SaveFromLastExecution marks the last actual execution's query.
	SaveFromLastExecution
	// SaveFromTerminalSelection marks the Ctrl+P/N-selected immutable query
	// in a terminal state.
	SaveFromTerminalSelection
)

// String renders the source name for tests and diagnostics.
func (s SQLSaveSource) String() string {
	switch s {
	case SaveFromViewedResult:
		return "viewed-result-query"
	case SaveFromRunnableBuilder:
		return "runnable-builder"
	case SaveFromLastExecution:
		return "last-execution"
	case SaveFromTerminalSelection:
		return "terminal-selection"
	default:
		return "SQLSaveSource(out-of-range)"
	}
}

// SQLSaveTarget is one resolved Ctrl+S target: an immutable complete query
// state ready for standalone serialization, plus its provenance. Resolution
// copies nothing from live database state; the state's slices are freshly
// allocated, so callers may retain or mutate the value freely.
type SQLSaveTarget struct {
	State  qb.HistoryState
	Source SQLSaveSource
}

// SQLSaveInput collects the immutable in-memory candidates for one Ctrl+S
// press. Every field is a value or pointer into freshly deep-copied history
// state; the resolver never reaches back into stores, schema, or a database.
type SQLSaveInput struct {
	// ViewedResultQuery is the query-history state associated with the
	// currently viewed historical result, when one is being viewed and its
	// association still resolves. Ordinary priority uses it first; terminal
	// priority ignores it entirely.
	ViewedResultQuery *qb.HistoryState
	// Builder is the current builder's normalized state. It contributes only
	// when BuilderRunnable reports the Issue #19 authoritative runnable
	// verdict; a present but non-runnable builder is skipped.
	Builder         *qb.HistoryState
	BuilderRunnable bool
	// LastExecution is the query-history state of the last actual execution.
	LastExecution *qb.HistoryState
	// Terminal selects terminal-only priority (deletion, replacement, and
	// outcome-unknown states): only the Ctrl+P/N selection and the last
	// actual execution are consulted, never the builder or the viewed result.
	Terminal bool
	// TerminalSelection is the Ctrl+P/N-selected immutable query state, when
	// a selection exists and still resolves.
	TerminalSelection *qb.HistoryState
}

// ResolveSQLSaveTarget resolves the one Ctrl+S target from immutable
// in-memory input. Ordinary states resolve in exact viewed-result-query,
// runnable-builder, last-execution order; terminal states resolve only the
// selected immutable query, then the last actual execution, deliberately
// ignoring builder and viewed-result candidates. When no candidate exists it
// returns ErrNoRunnableQuery and no target: callers must show the exact
// inline feedback and never open a picker. Resolution starts no validation,
// schema refresh, connection, or database work.
func ResolveSQLSaveTarget(in SQLSaveInput) (SQLSaveTarget, error) {
	if in.Terminal {
		if in.TerminalSelection != nil {
			return SQLSaveTarget{State: *in.TerminalSelection, Source: SaveFromTerminalSelection}, nil
		}
		if in.LastExecution != nil {
			return SQLSaveTarget{State: *in.LastExecution, Source: SaveFromLastExecution}, nil
		}
		return SQLSaveTarget{}, ErrNoRunnableQuery
	}
	if in.ViewedResultQuery != nil {
		return SQLSaveTarget{State: *in.ViewedResultQuery, Source: SaveFromViewedResult}, nil
	}
	if in.Builder != nil && in.BuilderRunnable {
		return SQLSaveTarget{State: *in.Builder, Source: SaveFromRunnableBuilder}, nil
	}
	if in.LastExecution != nil {
		return SQLSaveTarget{State: *in.LastExecution, Source: SaveFromLastExecution}, nil
	}
	return SQLSaveTarget{}, ErrNoRunnableQuery
}
