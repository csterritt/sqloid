// Stable-ID selection primitives over the finalized result-history list
// (Issue #36): pure read-only lookups mirroring the query-history cursor
// rules. Navigation never appends, allocates IDs, evicts, or reorders — the
// sole append path remains AppendFinalized, and selection identity is the
// stable entry ID, never a slice index. Every returned entry is freshly
// deep-copied, so browsing can never mutate retained history.
package history

// copyResultEntry returns an entry whose every mutable slice is freshly
// allocated, so returned values never alias retained storage.
func copyResultEntry(e ResultEntry) ResultEntry {
	out := e
	out.Columns = append([]string(nil), e.Columns...)
	out.Rows = copyRows(e.Rows)
	return out
}

// lookupIndex returns the slice index of the retained entry with the given
// stable ID, or -1 when the ID is not retained — including evicted,
// never-allocated, and the none ID.
func (s *ResultStore) lookupIndex(id EntryID) int {
	for i := range s.entries {
		if s.entries[i].ID == id {
			return i
		}
	}
	return -1
}

// Oldest returns the oldest retained result entry, freshly copied. The
// boolean is false and the entry zero on an empty store.
func (s *ResultStore) Oldest() (ResultEntry, bool) {
	if len(s.entries) == 0 {
		return ResultEntry{}, false
	}
	return copyResultEntry(s.entries[0]), true
}

// Newest returns the newest retained result entry, freshly copied. The
// boolean is false and the entry zero on an empty store.
func (s *ResultStore) Newest() (ResultEntry, bool) {
	if len(s.entries) == 0 {
		return ResultEntry{}, false
	}
	return copyResultEntry(s.entries[len(s.entries)-1]), true
}

// Lookup returns the entry with the given stable ID, freshly copied. The
// boolean is false and the entry zero when no retained entry carries that ID
// — including IDs evicted or never allocated — with no error distinction.
func (s *ResultStore) Lookup(id EntryID) (ResultEntry, bool) {
	i := s.lookupIndex(id)
	if i < 0 {
		return ResultEntry{}, false
	}
	return copyResultEntry(s.entries[i]), true
}

// OlderThan returns the entry immediately older than the retained entry with
// the given stable ID, freshly copied. The boolean is false when the ID is
// not retained — including evicted, never-allocated, and the none ID — or
// when the identified entry is already the oldest; the cursor must never
// cross a boundary or resolve through a missing backing entry.
func (s *ResultStore) OlderThan(id EntryID) (ResultEntry, bool) {
	i := s.lookupIndex(id)
	if i <= 0 {
		return ResultEntry{}, false
	}
	return copyResultEntry(s.entries[i-1]), true
}

// NewerThan returns the entry immediately newer than the retained entry with
// the given stable ID, freshly copied. The boolean is false when the ID is
// not retained or when the identified entry is already the newest.
func (s *ResultStore) NewerThan(id EntryID) (ResultEntry, bool) {
	i := s.lookupIndex(id)
	if i < 0 || i+1 >= len(s.entries) {
		return ResultEntry{}, false
	}
	return copyResultEntry(s.entries[i+1]), true
}
