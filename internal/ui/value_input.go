// Universal value-entry prompt state (Issue #14 seam consumed by Issue #17):
// one verbatim, universally parsed text input reused by every guided flow.
// The prompt is pure presentation-free state owned by the model while open;
// Enter hands the exact entered representation to the caller's accept hook,
// Esc invokes cancel, and neither performs parsing — classification lives in
// QueryBuilder's universal parser so typed `NULL` and empty input stay TEXT
// by construction rather than any special case here.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// valueCursorStyle renders the cell under the text-entry cursor; contrast
// comes from reversal, never color alone.
var valueCursorStyle = lipgloss.NewStyle().Reverse(true)

// ValuePrompt is one open universal text entry. The zero value is not
// meaningful; construct through NewValuePrompt.
type ValuePrompt struct {
	Opener string // identity label of the exact field that opened the prompt
	Label  string // human label rendered above the buffer (e.g. a staged predicate)

	runes  []rune // entered text kept verbatim, no trimming or normalization
	cursor int    // insertion index into runes, always within [0, len(runes)]
}

// NewValuePrompt opens universal entry over opener's field identity seeded
// with seed (an empty string for fresh entry), placing the cursor at the end
// of the restored text ready to append.
func NewValuePrompt(opener, label, seed string) *ValuePrompt {
	return &ValuePrompt{Opener: opener, Label: label, runes: []rune(seed), cursor: len([]rune(seed))}
}

// Buffer returns the entered text exactly as typed, byte-for-byte.
func (p *ValuePrompt) Buffer() string { return string(p.runes) }

// Cursor returns the current insertion index as an offset in runes.
func (p *ValuePrompt) Cursor() int { return p.cursor }

// HandleKey applies one key message to the entry buffer, reporting whether
// anything changed. Enter and Esc are intentionally not handled here: they are
// commit/cancel decisions owned by the model's hook plumbing.
func (p *ValuePrompt) HandleKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		s := make([]rune, 0, len(p.runes)+len(msg.Runes))
		s = append(s, p.runes[:p.cursor]...)
		s = append(s, msg.Runes...)
		s = append(s, p.runes[p.cursor:]...)
		p.runes = s
		p.cursor += len(msg.Runes)
		return true
	case tea.KeyBackspace, tea.KeyCtrlH:
		if p.cursor > 0 {
			p.runes = append(p.runes[:p.cursor-1], p.runes[p.cursor:]...)
			p.cursor--
			return true
		}
	case tea.KeyDelete:
		if p.cursor < len(p.runes) {
			p.runes = append(p.runes[:p.cursor], p.runes[p.cursor+1:]...)
			return true
		}
	case tea.KeyLeft:
		if p.cursor > 0 {
			p.cursor--
			return true
		}
	case tea.KeyRight:
		if p.cursor < len(p.runes) {
			p.cursor++
			return true
		}
	case tea.KeyHome, tea.KeyCtrlA:
		p.cursor = 0
		return true
	case tea.KeyEnd, tea.KeyCtrlE:
		p.cursor = len(p.runes)
		return true
	case tea.KeyCtrlU:
		p.runes = nil
		p.cursor = 0
		return true
	}
	return false
}

// PromptLines renders the prompt's content lines: the labeled buffer row with
// the visible cursor cell, then hint and help guidance lines supplied by the
// owning feature, then the fixed submit/cancel footer.
func (p *ValuePrompt) PromptLines(width int, hint string, help []string) []string {
	head := p.Opener + ": " + p.Label + ": "
	lines := make([]string, 0, 1+len(help)+3)
	lines = append(lines, p.renderBufferRow(head))
	lines = append(lines, truncateCell(hint, width))
	for _, h := range help {
		lines = append(lines, truncateCell(h, width))
	}
	lines = append(lines, truncateCell("Enter submits · Esc cancels", width))
	return lines
}

// renderBufferRow draws the buffer with the cursor cell reversed. The cursor
// sits after the cell named by Cursor, matching ordinary insertion semantics.
func (p *ValuePrompt) renderBufferRow(head string) string {
	before := ""
	if p.cursor > 0 {
		before = string(p.runes[:minInt(p.cursor, len(p.runes))])
	}
	var cur string
	if p.cursor < len(p.runes) {
		cur = valueCursorStyle.Render(string(p.runes[p.cursor]))
		tail := ""
		if p.cursor+1 <= len(p.runes) {
			tail = string(p.runes[p.cursor+1:])
		}
		return head + before + cur + tail
	}
	cur = valueCursorStyle.Render(" ")
	return head + before + cur
}
