// Stable-ID cursor navigation primitives (Issue #35): pure read-only
// lookups over the Issue #20 retained list. Navigation never appends,
// allocates IDs, evicts, or reorders — the sole append path remains
// AppendExecution, and selection identity is the stable entry ID, never a
// slice index. Every returned entry is a freshly deep-copied state so
// browsing can never mutate retained history.

package history

// Oldest returns the oldest retained entry, freshly copied. The boolean is
// false and the entry zero on an empty store.
func (s *Store) Oldest() (Entry, bool) {
	if len(s.entries) == 0 {
		return Entry{}, false
	}
	e := s.entries[0]
	return Entry{ID: e.ID, State: copyState(e.State)}, true
}

// Newest returns the newest retained entry, freshly copied. The boolean is
// false and the entry zero on an empty store.
func (s *Store) Newest() (Entry, bool) {
	if len(s.entries) == 0 {
		return Entry{}, false
	}
	e := s.entries[len(s.entries)-1]
	return Entry{ID: e.ID, State: copyState(e.State)}, true
}

// OlderThan returns the entry immediately older than the retained entry with
// the given stable ID, freshly copied. The boolean is false when the ID is
// not retained — including evicted, never-allocated, and the none ID — or
// when the identified entry is already the oldest; the cursor must never
// cross a boundary or resolve through a missing backing entry.
func (s *Store) OlderThan(id EntryID) (Entry, bool) {
	for i := 1; i < len(s.entries); i++ {
		if s.entries[i].ID == id {
			e := s.entries[i-1]
			return Entry{ID: e.ID, State: copyState(e.State)}, true
		}
	}
	return Entry{}, false
}

// NewerThan returns the entry immediately newer than the retained entry with
// the given stable ID, freshly copied. The boolean is false when the ID is
// not retained or when the identified entry is already the newest.
func (s *Store) NewerThan(id EntryID) (Entry, bool) {
	for i := 0; i+1 < len(s.entries); i++ {
		if s.entries[i].ID == id {
			e := s.entries[i+1]
			return Entry{ID: e.ID, State: copyState(e.State)}, true
		}
	}
	return Entry{}, false
}
