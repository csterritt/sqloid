// Directory navigation model for the save/export destination picker
// (Issue #52), per the File picker decision in Notes/PRD-sqloid.md. The
// model is UI-independent: every filesystem observation goes through the
// injected FS boundary, and all blocking work runs as caller-issued Nav
// Requests dispatched outside Update/View. Navigation never creates or
// removes directories, never lists regular files, and always lists the
// navigable parent `..` before the child directories in ascending
// case-sensitive bytewise order. Directory choice and filename text are
// separate states with separate focus; submission joins the validated,
// completed basename to the selected directory only after an issued
// verification request succeeds. Stale responses are rejected by their
// monotonic attempt identity, and every typed failure stays inline while
// retaining the current directory, highlight, filename, cursor, and format
// for deterministic retry or Esc cancellation.
package filepicker

import "path"

// Focus identifies which picker pane owns the keys.
type Focus int

const (
	// FocusDir routes keys to directory navigation; printable keys other
	// than navigation are consumed without effect.
	FocusDir Focus = iota
	// FocusFilename routes every printable key — including `?` and `q` —
	// into the filename buffer literally, ahead of any global key handling.
	FocusFilename
)

// NavRequest is one blocking filesystem request the caller must run outside
// Update/View and answer with the matching message carrying the same
// Attempt. Verify selects the destination-verification read performed at
// submission instead of a navigation listing.
type NavRequest struct {
	Path    string
	Attempt uint64
	Verify  bool
}

// ListedMsg answers one non-verify NavRequest. Dirs carries the child
// directory basenames of Path; the model sorts them, so callers may return
// any order. Err is the typed or wrapped cause of the failed read.
type ListedMsg struct {
	Path    string
	Attempt uint64
	Dirs    []string
	Err     error
}

// VerifiedMsg answers one verify NavRequest. Path is the joined destination
// that was verified; on success the picker completes with exactly that path.
type VerifiedMsg struct {
	Path    string
	Attempt uint64
	Err     error
}

// Model is the pure picker state. Construct through Start; the zero value is
// not meaningful. The model performs no I/O of its own: it only records
// issued requests and applies the caller's answers.
type Model struct {
	fs     FS
	format Format

	start string // captured process working directory the picker began at
	dir   string // current directory (the start until a listing arrives)

	dirs       []string // child directory basenames, sorted bytewise
	hasParent  bool     // false only at the filesystem root, where `..` is absent
	highlight  int      // index into Listing()
	focus      Focus
	filename   []rune // entered filename, verbatim
	cursor     int    // insertion index into filename, in runes
	err        error  // current inline error, nil when none
	attempt    uint64 // monotonic request identity guarding stale responses
	pending    bool   // exactly one request outstanding at any time
	pendingReq NavRequest

	done bool   // verification succeeded: completion is available
	path string // completed destination path when done
}

// Start initializes the picker at the caller-captured process working
// directory with the opener's closed save format and returns the first
// navigation request the caller must issue. The returned model lists nothing
// until the matching ListedMsg applies.
func Start(fs FS, cwd string, format Format) (Model, NavRequest) {
	m := Model{fs: fs, format: format, start: cwd, dir: cwd, attempt: 1, pending: true}
	req := NavRequest{Path: cwd, Attempt: m.attempt}
	m.pendingReq = req
	return m, req
}

// Format returns the closed save format the opener supplied.
func (m Model) Format() Format { return m.format }

// CurrentDir returns the picker's current directory, which stays unchanged
// through failures so navigation can resume where it was.
func (m Model) CurrentDir() string { return m.dir }

// StartDir returns the process working directory the picker began at.
func (m Model) StartDir() string { return m.start }

// Listing returns the directory list the picker presents: the navigable
// parent `..` first whenever a parent exists, followed by the child
// directories in ascending case-sensitive bytewise order. Regular files are
// never included.
func (m Model) Listing() []string {
	n := len(m.dirs)
	if m.hasParent {
		n++
	}
	list := make([]string, 0, n)
	if m.hasParent {
		list = append(list, "..")
	}
	list = append(list, m.dirs...)
	return list
}

// Highlight returns the highlighted index into Listing().
func (m Model) Highlight() int { return m.highlight }

// Highlighted returns the highlighted entry's text.
func (m Model) Highlighted() string {
	list := m.Listing()
	if m.highlight < 0 || m.highlight >= len(list) {
		return ""
	}
	return list[m.highlight]
}

// Focus returns which pane currently owns the keys.
func (m Model) Focus() Focus { return m.focus }

// Filename returns the entered filename verbatim, byte for byte.
func (m Model) Filename() string { return string(m.filename) }

// Cursor returns the filename insertion index in runes.
func (m Model) Cursor() int { return m.cursor }

// Error returns the current inline error, nil when none.
func (m Model) Error() error { return m.err }

// Pending reports whether one filesystem request is outstanding. While
// pending, navigation and submission are consumed without issuing duplicates.
func (m Model) Pending() bool { return m.pending }

// Completed reports whether verification succeeded and returns the exact
// completed destination path.
func (m Model) Completed() (string, bool) { return m.path, m.done }

// Apply answers one ListedMsg. A stale or unexpected response is inert. On
// success the current directory moves to the response's path, child
// directories are sorted bytewise with `..` first when the parent is
// navigable, and the highlight resets to the first entry. On failure the
// error is typed inline and the previous directory, highlight, filename,
// cursor, and format are all retained untouched.
func (m *Model) Apply(msg ListedMsg) {
	if !m.pending || m.pendingReq.Verify || msg.Attempt != m.attempt {
		return
	}
	m.pending = false
	if msg.Err != nil {
		m.err = &Error{Kind: KindRead, Path: msg.Path, Err: msg.Err}
		return
	}
	dirs := append([]string(nil), msg.Dirs...)
	SortChildDirs(dirs)
	m.dirs = dirs
	m.dir = msg.Path
	m.hasParent = path.Clean(msg.Path) != "/"
	m.highlight = 0
	m.err = nil
}

// ApplyVerified answers one VerifiedMsg. A stale or unexpected response is
// inert. On success the picker completes with exactly the verified path. On
// failure the error is typed inline and every picker state is retained so
// correction or retry needs no recapture.
func (m *Model) ApplyVerified(msg VerifiedMsg) {
	if !m.pending || !m.pendingReq.Verify || msg.Attempt != m.attempt {
		return
	}
	m.pending = false
	if msg.Err != nil {
		m.err = &Error{Kind: KindVerify, Path: msg.Path, Err: msg.Err}
		return
	}
	m.done = true
	m.path = msg.Path
	m.err = nil
}

// MoveHighlight moves the highlight by delta, consuming boundary presses as
// no-ops. Navigating never edits the filename buffer.
func (m *Model) MoveHighlight(delta int) {
	list := m.Listing()
	if len(list) == 0 {
		return
	}
	next := m.highlight + delta
	if next < 0 {
		next = 0
	}
	if next >= len(list) {
		next = len(list) - 1
	}
	m.highlight = next
}

// SetFocus switches which pane owns the keys.
func (m *Model) SetFocus(f Focus) { m.focus = f }

// clearErrorOnEdit drops the inline error when the user edits, navigates, or
// retries, per the retry/cancel lifecycle.
func (m *Model) clearErrorOnEdit() { m.err = nil }

// InsertRunes inserts printable text at the cursor. Any inserted text clears
// the inline error; the buffer keeps every byte verbatim.
func (m *Model) InsertRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	s := make([]rune, 0, len(m.filename)+len(runes))
	s = append(s, m.filename[:m.cursor]...)
	s = append(s, runes...)
	s = append(s, m.filename[m.cursor:]...)
	m.filename = s
	m.cursor += len(runes)
	m.clearErrorOnEdit()
}

// Backspace deletes the rune before the cursor, if any.
func (m *Model) Backspace() {
	if m.cursor == 0 {
		return
	}
	m.filename = append(m.filename[:m.cursor-1], m.filename[m.cursor:]...)
	m.cursor--
	m.clearErrorOnEdit()
}

// Delete deletes the rune under the cursor, if any.
func (m *Model) Delete() {
	if m.cursor >= len(m.filename) {
		return
	}
	m.filename = append(m.filename[:m.cursor], m.filename[m.cursor+1:]...)
	m.clearErrorOnEdit()
}

// Left moves the cursor one rune left, consuming the boundary as a no-op.
func (m *Model) Left() {
	if m.cursor > 0 {
		m.cursor--
	}
}

// Right moves the cursor one rune right, consuming the boundary as a no-op.
func (m *Model) Right() {
	if m.cursor < len(m.filename) {
		m.cursor++
	}
}

// Home moves the cursor to the start of the filename buffer.
func (m *Model) Home() { m.cursor = 0 }

// End moves the cursor to the end of the filename buffer.
func (m *Model) End() { m.cursor = len(m.filename) }

// EnterDir enters the highlighted entry: `..` navigates to the parent, any
// other highlighted entry is a child directory. It issues exactly one
// navigation request while idle and clears any inline error; while a request
// is pending it issues nothing. Entering never touches the filename buffer.
func (m *Model) EnterDir() (NavRequest, bool) {
	if m.pending {
		return NavRequest{}, false
	}
	entry := m.Highlighted()
	if entry == "" {
		return NavRequest{}, false
	}
	var target string
	if entry == ".." {
		target = path.Dir(path.Clean(m.dir))
	} else {
		target = path.Join(m.dir, entry)
	}
	return m.issue(NavRequest{Path: target}), true
}

// Submit validates the filename and, when valid, issues exactly one
// destination-verification request for the joined path. An invalid basename
// records its typed inline error, issues nothing, and reaches no filesystem,
// serializer, or overwrite prompt. While any request is pending Submit
// issues nothing.
func (m *Model) Submit() (NavRequest, bool) {
	if m.pending {
		return NavRequest{}, false
	}
	name := m.Filename()
	if err := ValidateFilename(name); err != nil {
		m.err = &Error{Kind: KindValidate, Path: "", Err: err}
		return NavRequest{}, false
	}
	dest := JoinDestination(m.dir, name, m.format)
	return m.issue(NavRequest{Path: dest, Verify: true}), true
}

// Retry re-issues the most recent failed request — a failed navigation
// listing or a failed destination verification — with a fresh attempt
// identity, clearing the inline error. It retains every other state. Retry
// is a no-op while a request is pending or when nothing failed.
func (m *Model) Retry() (NavRequest, bool) {
	if m.pending || m.err == nil {
		return NavRequest{}, false
	}
	return m.issue(m.pendingReq), true
}

// issue records and returns one outgoing request with a fresh attempt
// identity, clearing any inline error (navigation and retry are the user's
// forward action).
func (m *Model) issue(req NavRequest) NavRequest {
	m.attempt++
	req.Attempt = m.attempt
	m.pending = true
	m.pendingReq = req
	m.err = nil
	return req
}
