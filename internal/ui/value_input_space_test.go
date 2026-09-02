// Issue #68 Task 1 (RED): KeySpace editing contract for the universal value
// prompt. tea.KeySpace must insert exactly one U+0020 at the rune cursor
// through the same insertion path as tea.KeyRunes, advance the cursor by one
// rune, leave every surrounding rune untouched, and report a change. The
// ordinary rune-aware editing model — Left/Right, Backspace/Delete,
// Home/End, Ctrl+A/Ctrl+E, and Ctrl+U — must keep working unchanged once a
// space has been inserted. These tests target ValuePrompt.HandleKey directly
// and never route base-context or popup space keys through the prompt.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// spaceKey is one tea.KeySpace press.
func spaceKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace} }

// vpSpaceCase is one focused KeySpace insertion scenario.
type vpSpaceCase struct {
	name   string
	seed   string // initial buffer (empty, ASCII, or multibyte Unicode)
	cursor int    // rune cursor before the space press (0 start, mid, end)
}

// vpSpaceCases enumerates the empty, ASCII, and multibyte Unicode buffers at
// the start, middle, and end cursor positions the contract must cover.
func vpSpaceCases() []vpSpaceCase {
	return []vpSpaceCase{
		{"empty/end", "", 0},
		{"ascii/start", "abc", 0},
		{"ascii/mid", "abc", 1},
		{"ascii/end", "abc", 3},
		{"unicode/start", "世界", 0},
		{"unicode/mid", "世界", 1},
		{"unicode/end", "世界", 2},
	}
}

// positionAtCursor moves the prompt cursor to the requested rune offset,
// starting from the end-of-seed position NewValuePrompt places it at.
func positionAtCursor(p *ValuePrompt, cursor int) {
	for p.Cursor() > cursor {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	}
	for p.Cursor() < cursor {
		p.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	}
}

// TestValuePromptKeySpaceInsertsOneRuneAtCursor requires tea.KeySpace to
// insert exactly one U+0020 at the current rune cursor, advance the cursor by
// exactly one rune, leave every surrounding rune byte-for-byte unchanged, and
// report a change — across empty, ASCII, and multibyte Unicode buffers at the
// start, middle, and end cursor positions.
func TestValuePromptKeySpaceInsertsOneRuneAtCursor(t *testing.T) {
	for _, tc := range vpSpaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			p := NewValuePrompt(limitFieldLabel, "row limit", tc.seed)
			if tc.cursor < 0 || tc.cursor > len([]rune(tc.seed)) {
				t.Fatalf("setup: cursor %d out of range for seed %q", tc.cursor, tc.seed)
			}
			positionAtCursor(p, tc.cursor)

			seedRunes := []rune(tc.seed)
			wantRunes := make([]rune, 0, len(seedRunes)+1)
			wantRunes = append(wantRunes, seedRunes[:tc.cursor]...)
			wantRunes = append(wantRunes, ' ')
			wantRunes = append(wantRunes, seedRunes[tc.cursor:]...)
			wantBuf := string(wantRunes)

			changed := p.HandleKey(spaceKey())
			if !changed {
				t.Fatalf("HandleKey(KeySpace) reported no change; want true")
			}
			if got := p.Buffer(); got != wantBuf {
				t.Fatalf("buffer = %q, want %q (one U+0020 at cursor %d)", got, wantBuf, tc.cursor)
			}
			if got := p.Cursor(); got != tc.cursor+1 {
				t.Fatalf("cursor = %d, want %d (one-rune advance)", got, tc.cursor+1)
			}
			// Exactly one U+0020 was inserted: count spaces increased by one.
			gotSpaces := 0
			for _, r := range p.runes {
				if r == ' ' {
					gotSpaces++
				}
			}
			wantSpaces := 0
			for _, r := range seedRunes {
				if r == ' ' {
					wantSpaces++
				}
			}
			if gotSpaces != wantSpaces+1 {
				t.Fatalf("space count = %d, want %d (exactly one U+0020 inserted)", gotSpaces, wantSpaces+1)
			}
		})
	}
}

// TestValuePromptKeySpaceMatchesKeyRunesSpace requires tea.KeySpace to behave
// identically to one tea.KeyRunes carrying a single space rune, for the same
// seed and cursor, on buffer text, cursor, and change report.
func TestValuePromptKeySpaceMatchesKeyRunesSpace(t *testing.T) {
	for _, tc := range vpSpaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			pSpace := NewValuePrompt(limitFieldLabel, "row limit", tc.seed)
			pRunes := NewValuePrompt(limitFieldLabel, "row limit", tc.seed)
			positionAtCursor(pSpace, tc.cursor)
			positionAtCursor(pRunes, tc.cursor)

			changedSpace := pSpace.HandleKey(spaceKey())
			changedRunes := pRunes.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
			if changedSpace != changedRunes {
				t.Fatalf("change report differs: KeySpace=%v KeyRunes=%v", changedSpace, changedRunes)
			}
			if pSpace.Buffer() != pRunes.Buffer() {
				t.Fatalf("buffer differs: KeySpace=%q KeyRunes=%q", pSpace.Buffer(), pRunes.Buffer())
			}
			if pSpace.Cursor() != pRunes.Cursor() {
				t.Fatalf("cursor differs: KeySpace=%d KeyRunes=%d", pSpace.Cursor(), pRunes.Cursor())
			}
		})
	}
}

// TestValuePromptKeySpaceFollowedByEditingKeys proves a space inserted by
// KeySpace uses the ordinary rune-aware editing model: Left/Right move by one
// rune across the inserted space, Backspace/Delete remove exactly the space
// rune when targeted, Home/End and Ctrl+A/Ctrl+E jump to the buffer
// boundaries, and Ctrl+U clears the whole buffer including the space.
func TestValuePromptKeySpaceFollowedByEditingKeys(t *testing.T) {
	t.Run("left/right cross the inserted space", func(t *testing.T) {
		p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
		// Insert mid-buffer: "a b".
		positionAtCursor(p, 1)
		if !p.HandleKey(spaceKey()) {
			t.Fatal("KeySpace reported no change")
		}
		if got := p.Buffer(); got != "a b" {
			t.Fatalf("buffer = %q, want \"a b\"", got)
		}
		if got := p.Cursor(); got != 2 {
			t.Fatalf("cursor = %d, want 2", got)
		}
		// Left moves back onto the space rune.
		if !p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft}) {
			t.Fatal("Left reported no change")
		}
		if got := p.Cursor(); got != 1 {
			t.Fatalf("cursor after Left = %d, want 1 (on the space)", got)
		}
		// Right moves forward past the space rune.
		if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRight}) {
			t.Fatal("Right reported no change")
		}
		if got := p.Cursor(); got != 2 {
			t.Fatalf("cursor after Right = %d, want 2", got)
		}
	})

	t.Run("backspace removes the inserted space", func(t *testing.T) {
		p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
		positionAtCursor(p, 1)
		p.HandleKey(spaceKey())
		// Cursor sits after the space; Backspace deletes the space rune.
		if !p.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace}) {
			t.Fatal("Backspace reported no change")
		}
		if got := p.Buffer(); got != "ab" {
			t.Fatalf("buffer after Backspace = %q, want \"ab\"", got)
		}
		if got := p.Cursor(); got != 1 {
			t.Fatalf("cursor after Backspace = %d, want 1", got)
		}
	})

	t.Run("delete removes the inserted space", func(t *testing.T) {
		p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
		positionAtCursor(p, 1)
		p.HandleKey(spaceKey())
		// Step left so the cursor is on the space rune, then Delete it.
		p.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
		if !p.HandleKey(tea.KeyMsg{Type: tea.KeyDelete}) {
			t.Fatal("Delete reported no change")
		}
		if got := p.Buffer(); got != "ab" {
			t.Fatalf("buffer after Delete = %q, want \"ab\"", got)
		}
		if got := p.Cursor(); got != 1 {
			t.Fatalf("cursor after Delete = %d, want 1", got)
		}
	})

	t.Run("home/ctrl+a and end/ctrl+e jump across the space", func(t *testing.T) {
		p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
		positionAtCursor(p, 1)
		p.HandleKey(spaceKey())
		// Buffer is "a b"; cursor at 2.
		for _, msg := range []tea.KeyMsg{
			{Type: tea.KeyHome},
			{Type: tea.KeyCtrlA},
		} {
			if !p.HandleKey(msg) {
				t.Fatalf("%v reported no change", msg.Type)
			}
			if got := p.Cursor(); got != 0 {
				t.Fatalf("cursor after %v = %d, want 0", msg.Type, got)
			}
		}
		for _, msg := range []tea.KeyMsg{
			{Type: tea.KeyEnd},
			{Type: tea.KeyCtrlE},
		} {
			if !p.HandleKey(msg) {
				t.Fatalf("%v reported no change", msg.Type)
			}
			if got := p.Cursor(); got != 3 {
				t.Fatalf("cursor after %v = %d, want 3", msg.Type, got)
			}
		}
	})

	t.Run("ctrl+u clears the whole buffer including the space", func(t *testing.T) {
		p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
		positionAtCursor(p, 1)
		p.HandleKey(spaceKey())
		if !p.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlU}) {
			t.Fatal("Ctrl+U reported no change")
		}
		if got := p.Buffer(); got != "" {
			t.Fatalf("buffer after Ctrl+U = %q, want empty", got)
		}
		if got := p.Cursor(); got != 0 {
			t.Fatalf("cursor after Ctrl+U = %d, want 0", got)
		}
	})
}

// TestValuePromptKeyRunesControlStillInserts is a control proving the
// existing tea.KeyRunes insertion path keeps its rune-aware behavior
// alongside the new KeySpace handling, so the contract is unchanged for
// ordinary printable input.
func TestValuePromptKeyRunesControlStillInserts(t *testing.T) {
	p := NewValuePrompt(limitFieldLabel, "row limit", "ab")
	positionAtCursor(p, 1)
	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")}) {
		t.Fatal("KeyRunes reported no change")
	}
	if got := p.Buffer(); got != "aXb" {
		t.Fatalf("buffer = %q, want \"aXb\"", got)
	}
	if got := p.Cursor(); got != 2 {
		t.Fatalf("cursor = %d, want 2", got)
	}
}
