// Reusable popup interaction state for Issue #12: searchable candidate lists
// with case-insensitive subsequence filtering and scroll-only lists without
// any search input, per Issue #12 and the Builder and Display Interaction
// section of Notes/PRD-sqloid.md.
//
// Candidate identity (ID) is kept separate from displayed text so acceptance
// commits identity, never presentation. Ordering always follows the source
// candidate slice; filtering preserves it. Every actual search-text change
// deterministically resets the highlight to the first visible result and the
// viewport to its top. Searchable and scroll-only variants share this state
// so later fields (columns, GROUP BY, aggregates, operators) reuse it without
// new interaction rules; connecting them is outside Issue #12's scope.

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PopupMode distinguishes the two reusable popup variants.
type PopupMode int

const (
	// PopupSearchable filters candidates against typed case-insensitive
	// subsequence search text.
	PopupSearchable PopupMode = iota
	// PopupScrollOnly offers an unfiltered list navigable with Up/Down and
	// carries no search-input modality.
	PopupScrollOnly
)

// NoMatchesMessage is the exact status text rendered when a searchable
// filter (or empty candidate data) leaves nothing visible.
const NoMatchesMessage = "no matches"

// PopupCandidate pairs one candidate's committed identity with its displayed
// text; matching happens against Display.
type PopupCandidate struct {
	ID      string
	Display string
}

// EnterOutcome classifies what one Enter press did to the popup.
type EnterOutcome int

const (
	// EnterNone means Enter was ignored: the list was empty or matched
	// nothing, or the popup was closed.
	EnterNone EnterOutcome = iota
	// EnterAccepted commits the highlighted candidate in a single-select.
	EnterAccepted
	// EnterAdded added a previously absent candidate to a multi-selection;
	// the popup stays open for another choice.
	EnterAdded
	// EnterDuplicate found the highlighted candidate already completed in a
	// multi-selection; completed selections stay unique and unchanged.
	EnterDuplicate
)

// EnterResult reports the outcome of Enter plus the affected candidate ID
// (empty for EnterNone).
type EnterResult struct {
	Outcome EnterOutcome
	ID      string
}

// Popup is the pure interaction state of one open or closed popup list. The
// zero value is not meaningful; construct through NewSearchablePopup,
// NewMultiSearchablePopup, or NewScrollOnlyPopup.
type Popup struct {
	Mode   PopupMode
	Multi  bool   // multi-select Enter adds and reopens instead of accepting
	Opener string // identity label of the exact field that opened the popup

	Search string // current search text; always empty for scroll-only popups

	candidates     []PopupCandidate
	filtered       []int // indices into candidates currently visible, in order
	highlightIndex int   // index into filtered of the highlighted row
	viewportTop    int   // first filtered index shown inside the viewport

	viewportHeight int // maximum visible rows; <=0 shows everything

	completed    []string        // completed multi-selections, insertion order
	completedSet map[string]bool // duplicate guard over completed

	open bool
}

// NewSearchablePopup returns an open single-select searchable popup offering
// candidates under opener's field identity. Candidates are retained in the
// given order; a nil slice yields a permanently open no-match popup.
func NewSearchablePopup(opener string, candidates []PopupCandidate) *Popup {
	p := &Popup{Mode: PopupSearchable, Opener: opener, completedSet: map[string]bool{}}
	p.install(candidates)
	return p
}

// NewMultiSearchablePopup returns an open multi-select searchable popup whose
// Enter adds nonduplicate completions and stays open until Esc closes it.
func NewMultiSearchablePopup(opener string, candidates []PopupCandidate) *Popup {
	p := NewSearchablePopup(opener, candidates)
	p.Multi = true
	return p
}

// NewScrollOnlyPopup returns an open popup that ignores all search input and
// presents every candidate in source order.
func NewScrollOnlyPopup(opener string, candidates []PopupCandidate) *Popup {
	p := &Popup{Mode: PopupScrollOnly, Opener: opener, completedSet: map[string]bool{}}
	p.install(candidates)
	return p
}

// install adopts candidates, refilters, and repositions on the first result
// so every constructor starts deterministically at the same place.
func (p *Popup) install(candidates []PopupCandidate) {
	p.candidates = append([]PopupCandidate(nil), candidates...)
	if len(p.candidates) == 0 {
		// Keep candidate identity separate from slices callers may mutate.
		p.candidates = nil
	}
	p.refilter()
	p.highlightIndex, p.viewportTop = 0, 0
	p.open = true
}

// ReplaceCandidates adopts the given slice as the popup's complete offered
// list, preserving the current search text while resetting highlight and
// viewport deterministically — the same contract install applies to a fresh
// open. Only whole-catalog refresh replacement may use this; no partial
// substitution is ever applied.
func (p *Popup) ReplaceCandidates(candidates []PopupCandidate) {
	p.install(candidates)
}

// Open reports whether the popup is still open.
func (p *Popup) Open() bool { return p.open }

// Visible returns the currently visible candidates in source order: all of
// them when the search matches everything or the variant is scroll-only.
func (p *Popup) Visible() []PopupCandidate {
	out := make([]PopupCandidate, 0, len(p.filtered))
	for _, i := range p.filtered {
		out = append(out, p.candidates[i])
	}
	return out
}

// MatchCount reports how many candidates match the current search state.
func (p *Popup) MatchCount() int { return len(p.filtered) }

// NoMatch reports whether the popup is open with zero visible candidates:
// either a nonmatching search or empty candidate data.
func (p *Popup) NoMatch() bool { return p.open && len(p.filtered) == 0 }

// StatusMessages returns the presentation status lines the popup requires in
// its current state, ordered above the candidate rows.
func (p *Popup) StatusMessages() []string {
	if p.NoMatch() {
		return []string{NoMatchesMessage}
	}
	return nil
}

// Highlighted returns the highlighted candidate, if any is visible.
func (p *Popup) Highlighted() (PopupCandidate, bool) {
	if p.highlightIndex < 0 || p.highlightIndex >= len(p.filtered) {
		return PopupCandidate{}, false
	}
	return p.candidates[p.filtered[p.highlightIndex]], true
}

// SetViewportHeight caps the number of visible rows used by scrolling and
// rendering; heights below one show everything unwindowed.
func (p *Popup) SetViewportHeight(h int) {
	if h < 0 {
		h = 0
	}
	p.viewportHeight = h
	p.scrollIntoView()
}

// ViewportHeight returns the configured viewport height (0 means unbounded).
func (p *Popup) ViewportHeight() int { return p.viewportHeight }

// viewHeight is the effective visible-row count right now.
func (p *Popup) viewHeight() int {
	if p.viewportHeight <= 0 {
		return len(p.filtered)
	}
	return p.viewportHeight
}

// SetSearch replaces the search text of a searchable popup. Any actual change
// refilters and resets both highlight and viewport deterministically;
// identical text changes nothing. Scroll-only popups ignore search entirely.
func (p *Popup) SetSearch(s string) {
	if !p.open || p.Mode != PopupSearchable || s == p.Search {
		return
	}
	p.Search = s
	p.refilter()
	p.highlightIndex, p.viewportTop = 0, 0
}

// AppendSearchRune appends r to the search text with full reset semantics.
func (p *Popup) AppendSearchRune(r rune) { p.SetSearch(p.Search + string(r)) }

// BackspaceSearch removes the last rune from the search text with full reset
// semantics; backspacing an empty search is a no-op.
func (p *Popup) BackspaceSearch() {
	if p.Search == "" {
		return
	}
	r := []rune(p.Search)
	p.SetSearch(string(r[:len(r)-1]))
}

// refilter recomputes the visible index set, preserving source order.
func (p *Popup) refilter() {
	p.filtered = p.filtered[:0]
	for i, c := range p.candidates {
		if p.Mode == PopupScrollOnly || matchesSubsequence(p.Search, c.Display) {
			p.filtered = append(p.filtered, i)
		}
	}
}

// Down moves the highlight one row toward the end, clamping there as a no-op,
// and scrolls only when needed to keep the highlight visible.
func (p *Popup) Down() { p.step(1) }

// Up moves the highlight one row toward the start, clamping there as a no-op,
// and scrolls only when needed to keep the highlight visible.
func (p *Popup) Up() { p.step(-1) }

// step moves the highlight by delta within bounds and keeps it inside the
// viewport window; boundaries clamp rather than wrap for determinism.
func (p *Popup) step(delta int) {
	if !p.open {
		return
	}
	n := len(p.filtered)
	next := p.highlightIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	if next != p.highlightIndex && n > 0 {
		p.highlightIndex = next
		p.scrollIntoView()
	}
}

// scrollIntoView shifts viewportTop minimally so the highlight stays inside
// the visible window, clamping at both ends of the list.
func (p *Popup) scrollIntoView() {
	h := p.viewHeight()
	if h <= 0 || p.highlightIndex < p.viewportTop {
		p.viewportTop = maxInt(p.highlightIndex, 0)
	}
	if last := len(p.filtered) - h; p.viewportTop > last {
		p.viewportTop = maxInt(last, 0)
	}
	if p.highlightIndex >= p.viewportTop+h {
		p.viewportTop = p.highlightIndex - h + 1
	}
}

// maxInt returns the larger of two ints without importing math solely for it.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Enter applies Enter semantics: single-select accepts the highlighted
// candidate (the caller closes the popup); multi-select adds a nonduplicate
// completion and stays open for another choice. Enter with no matches or no
// candidates does nothing and the popup remains open.
func (p *Popup) Enter() EnterResult {
	c, ok := p.Highlighted()
	if !ok || !p.open {
		return EnterResult{}
	}
	if p.Multi {
		if p.completedSet[c.ID] {
			return EnterResult{Outcome: EnterDuplicate, ID: c.ID}
		}
		p.completedSet[c.ID] = true
		p.completed = append(p.completed, c.ID)
		return EnterResult{Outcome: EnterAdded, ID: c.ID}
	}
	return EnterResult{Outcome: EnterAccepted, ID: c.ID}
}

// Esc closes the popup on the cancel path. Only unfinished work (search
// edits, uncommitted moves) is discarded; Completed remains intact for the
// caller to preserve across reopening.
func (p *Popup) Esc() { p.open = false }

// Completed returns the completed multi-selections in insertion order with
// duplicates excluded by construction. Callers receive a fresh slice.
func (p *Popup) Completed() []string {
	out := make([]string, len(p.completed))
	copy(out, p.completed)
	return out
}

// matchesSubsequence reports whether every lower-cased rune of query appears
// somewhere in display after earlier runes — the PRD's case-insensitive
// subsequence rule shared by every searchable popup.
func matchesSubsequence(query, display string) bool {
	q := strings.ToLower(query)
	dr := []rune(strings.ToLower(display))
	i := 0
	for _, qr := range q {
		for i < len(dr) && dr[i] != qr {
			i++
		}
		if i == len(dr) {
			return false
		}
		i++
	}
	return true
}

// Key routing through the model. These paths implement the popup row of the
// global key-precedence matrix: while a popup is open, it consumes Up/Down,
// Enter, Esc, and — for searchable variants — printable search input before
// any base-context handling, so no popup key leaks into builder shortcuts.

// handlePopupKey routes one key message into the open popup. Single-select
// Enter closes the popup and invokes the opener's accept hook with the
// accepted candidate ID; multi-select Enter adds and stays open; Esc closes
// unchanged, preserving completed multi-selections for the caller.
func (m Model) handlePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Popup.Opener == setFieldLabel && !m.Popup.Multi {
		switch msg.String() {
		case "tab":
			if m.setCursor+1 < len(m.QB.SetAssignments()) {
				m.openSetChoice(m.setCursor + 1)
			}
			return m.adjustScrollAndReturn()
		case "shift+tab":
			if m.setCursor > 0 {
				m.openSetChoice(m.setCursor - 1)
			}
			return m.adjustScrollAndReturn()
		}
	}
	if m.Popup.Opener == insertFieldLabel && !m.Popup.Multi {
		switch msg.String() {
		case "tab":
			if m.insertCursor+1 < len(m.QB.InsertColumns()) {
				m.beginInsertChoice(m.insertCursor + 1)
			}
			return m.adjustScrollAndReturn()
		case "shift+tab":
			if m.insertCursor > 0 {
				m.beginInsertChoice(m.insertCursor - 1)
			}
			return m.adjustScrollAndReturn()
		}
	}
	switch msg.String() {
	case "up":
		m.Popup.Up()
	case "down":
		m.Popup.Down()
	case "enter":
		res := m.Popup.Enter()
		if res.Outcome == EnterAdded && !m.schemaStale && m.Popup.Opener == setFieldLabel {
			if accept := m.popupAccept; accept != nil {
				accept(&m, res.ID)
			}
			return m.adjustScrollAndReturn()
		}
		if res.Outcome == EnterAccepted && !m.schemaStale {
			opener, id, accept := m.openerFocus, res.ID, m.popupAccept
			m.closePopupRestore(opener)
			if accept != nil {
				accept(&m, id)
			}
			return m.adjustScrollAndReturn()
		}
	case "esc":
		setSelection := !m.schemaStale && m.Popup.Opener == setFieldLabel && m.Popup.Multi
		if m.schemaStale {
			// Esc under stale schema is the cancel path: close only the stale
			// refresh flow and restore the captured pre-open state unchanged.
			m.applyCancel()
		} else {
			m.closePopupRestore(m.openerFocus)
			if setSelection {
				m.finishSetSelection()
			}
		}
		return m.adjustScrollAndReturn()
	case "backspace":
		m.Popup.BackspaceSearch()
	case "ctrl+c":
		// Issue #55: Ctrl+C opens the shared quit confirmation from every
		// popup variant; the popup stays suspended behind it untouched.
		return m.openQuitConfirmation(), nil
	case "q":
		// Issue #55: q is literal search input while the focused search owns
		// it; a scroll-only popup owns no search, so q opens the confirmation.
		if m.Popup.Mode == PopupScrollOnly {
			return m.openQuitConfirmation(), nil
		}
		m.Popup.AppendSearchRune('q')
	default:
		if msg.Type == tea.KeySpace {
			m.Popup.AppendSearchRune(' ')
		} else if len(msg.Runes) > 0 {
			for _, r := range msg.Runes {
				m.Popup.AppendSearchRune(r)
			}
		}
		// Scroll-only variants ignore every printable append inside the
		// popup itself, so nothing else is needed here.
	}
	return m.adjustScrollAndReturn()
}

// adjustScrollAndReturn keeps the builder viewport consistent after popup
// interaction possibly changed focus, then returns the model as tea.Model.
func (m Model) adjustScrollAndReturn() (tea.Model, tea.Cmd) {
	m.adjustScroll()
	return m, nil
}
