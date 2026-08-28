// Package history owns Sqloid's session-only query-history storage (Issue
// #20): stable entry identities independent of list positions, chronological
// append order, immutable deep-copied complete QueryBuilder execution states,
// an exact 20-entry capacity with oldest-first eviction, and the
// consecutive-identical append suppression policy from the PRD's History
// decision. Navigation, cursors, restoration, and selected-entry eviction
// fallback are deferred to Issue #35; validation and execution timing stay
// with their owning packages. The package has no database and no Bubble Tea
// dependency.
package history

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// Capacity is the exact number of entries the store retains at once. Each
// append beyond it evicts exactly the oldest retained entry first.
const Capacity = 20

// EntryID is one stable query-history identity. IDs are nonzero, allocated
// monotonically, and never reused or renumbered: an entry keeps its ID for
// its whole lifetime even as older entries are evicted and its slice index
// changes. Zero is never allocated; functions return it to mean "none".
type EntryID uint64

// Entry is one retained query-history item: its stable identity together
// with an immutable deep copy of the execution state retained at append
// time. Later builder mutation never alters State.
type Entry struct {
	ID    EntryID
	State qb.HistoryState
}

// Store is the in-memory query-history list. Append and retrieval always
// deep-copy mutable slices, so neither the source state nor a returned list
// shares storage with the retained entries. It is not safe for concurrent
// use; the single Bubble Tea update loop is its only expected caller.
type Store struct {
	nextID  EntryID // next ID to allocate; entries never before it exist
	entries []Entry // chronological: oldest first
}

// NewStore returns an empty query-history store.
func NewStore() *Store { return &Store{} }

// Len reports the number of retained entries, from 0 up to Capacity.
func (s *Store) Len() int { return len(s.entries) }

// copyState returns a state whose every mutable slice is freshly allocated,
// so retained entries and returned values never alias caller storage.
func copyState(state qb.HistoryState) qb.HistoryState {
	out := state
	out.Projection = append([]qb.HistoryProjectionEntry(nil), state.Projection...)
	out.Groups = append([]string(nil), state.Groups...)
	out.Sets = append([]qb.HistorySetAssignment(nil), state.Sets...)
	out.Inserts = append([]qb.HistoryInsertColumn(nil), state.Inserts...)
	return out
}

// Append deep-copies state and retains it as the newest entry under a fresh
// stable ID, returning that ID. When Capacity is exceeded, exactly the oldest
// retained entry is evicted before the new list is exposed; all surviving
// IDs are preserved. Suppression is not applied here — execution-start
// appends go through AppendExecution, which owns the consecutive policy.
func (s *Store) Append(state qb.HistoryState) EntryID {
	s.nextID++
	if len(s.entries) == Capacity {
		s.entries = s.entries[1:]
	}
	s.entries = append(s.entries, Entry{ID: s.nextID, State: copyState(state)})
	return s.nextID
}

// Entries returns the retained entries in chronological order (oldest
// first) as a fresh slice of freshly copied states; callers may mutate it
// and the states freely without touching the store.
func (s *Store) Entries() []Entry {
	out := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		out[i] = Entry{ID: e.ID, State: copyState(e.State)}
	}
	return out
}

// Lookup returns the entry with the given stable ID, freshly copied. The
// boolean is false and the entry zero when no retained entry carries that ID
// — including IDs evicted or never allocated — with no error distinction.
func (s *Store) Lookup(id EntryID) (Entry, bool) {
	for _, e := range s.entries {
		if e.ID == id {
			return Entry{ID: e.ID, State: copyState(e.State)}, true
		}
	}
	return Entry{}, false
}
