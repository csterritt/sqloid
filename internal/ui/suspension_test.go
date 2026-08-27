package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// lipglossLineCount counts rendered rows of a lipgloss block.
func lipglossLineCount(s string) int {
	return len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

type sizeCase struct {
	name          string
	width, height int
}

func tooSmallCases() []sizeCase {
	return []sizeCase{
		{"both below", 79, 23},
		{"width only", 79, 24},
		{"height only", 80, 23},
	}
}

// nontrivialModel returns a model with several fields, focus deep in the list,
// nonzero scroll context, and ownership of active cancellable work. cancelled
// is closed over by the injected generic cancellation command.
func nontrivialModel(w, h int, cancelled *bool) Model {
	m := New()
	m.Fields = largeFields(30)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)
	for _, k := range []string{"tab", "tab", "tab"} {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	m.ActiveCancellable = true
	m.CancelCommand = func() tea.Msg { *cancelled = true; return cancellationRequested{} }
	return m
}

// stateSnapshot captures the user-visible application context for exactness
// comparisons across the minimum-size boundary.
func stateSnapshot(m Model) Model {
	copied := m
	copied.Fields = append([]Field(nil), m.Fields...)
	copied.suspendedModel = nil
	copied.suspended = false
	copied.Width, copied.Height = 0, 0
	// Cancellation commands are never deep-comparable; routing is asserted
	// separately through explicit command invocation.
	copied.CancelCommand = nil
	return copied
}

// cancellationRequested is a test-local message produced by the injected
// generic cancellation command.
type cancellationRequested struct{}

func TestFocusedFieldScrolling(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		moves         []string // focus moves applied from initial Command focus
	}{
		{"down past cap at 80x24", 80, 24, []string{"tab", "tab"}},
		{"up back toward start at 100x30", 100, 30, []string{"tab", "tab", "shift+tab"}},
		{"multiline bottom field at 160x50", 160, 50, []string{"tab", "tab", "down", "down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.Fields = largeFields(40) // builder grows far beyond its floor(H/3) cap
			l := CalculateLayout(tt.height, m.Fields)
			viewport := l.BuilderViewport()
			starts, counts := fieldSpans(m.Fields)
			total := sumFieldLines(m.Fields)
			if total <= viewport {
				t.Fatalf("builder content %d does not exceed viewport %d; test setup invalid", total, viewport)
			}

			var model tea.Model = m
			model, _ = model.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			for _, k := range tt.moves {
				var cmd tea.Cmd
				model, cmd = model.Update(keyMsg(k))
				got := model.(Model)

				fs, fc := starts[got.Focus], counts[got.Focus]
				if fs < got.Scroll || fs+fc > got.Scroll+viewport {
					t.Errorf("after %q focused field [%d,%d) not fully inside visible [%d,%d)",
						k, fs, fs+fc, got.Scroll, got.Scroll+viewport)
				}
				if got.Scroll < 0 || (viewport < total && got.Scroll > total-viewport) {
					t.Errorf("scroll %d outside [0,%d]", got.Scroll, total-viewport)
				}
				if cmd != nil && tt.moves[len(tt.moves)-1] == k {
					t.Errorf("focus move %q unexpectedly returned a command", k)
				}
			}
		})
	}
}

func TestTooSmallExactView(t *testing.T) {
	for _, tc := range tooSmallCases() {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := false
			var model tea.Model = nontrivialModel(100, 40, &cancelled)
			model, _ = model.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			if got := model.View(); got != TooSmallMessage {
				t.Errorf("undersized view = %q, want exactly %q", got, TooSmallMessage)
			}
		})
	}
}

func TestTooSmallPreservesAndRestoresState(t *testing.T) {
	for _, tc := range tooSmallCases() {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := false
			supported := sizeCase{name: "supported", width: 100, height: 40}
			var model tea.Model = nontrivialModel(supported.width, supported.height, &cancelled)
			before := stateSnapshot(model.(Model))

			model, _ = model.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			small := model.(Model)
			if !reflect.DeepEqual(before, stateSnapshot(small)) {
				t.Fatalf("%s: hidden state changed when entering undersized terminal:\nbefore %+v\nafter %+v",
					tc.name, before, stateSnapshot(small))
			}

			// Ordinary keys must be ignored without exposing or mutating hidden state.
			for _, k := range []string{"a", "1", " ", "tab", "down", "up"} {
				var cmd tea.Cmd
				model, cmd = small.Update(keyMsg(k))
				small = model.(Model)
				if cmd != nil {
					t.Errorf("%s: key %q produced a command while undersized", tc.name, k)
				}
				if got := small.View(); got != TooSmallMessage {
					t.Fatalf("%s: after key %q view = %q, want exactly %q", tc.name, k, got, TooSmallMessage)
				}
			}
			if !reflect.DeepEqual(before, stateSnapshot(small)) {
				t.Errorf("%s: ignored keys mutated hidden state:\nbefore %+v\nafter %+v",
					tc.name, before, stateSnapshot(small))
			}

			// Resizing back restores the exact prior context and focus, then lays out normally.
			model, _ = small.Update(tea.WindowSizeMsg{Width: supported.width, Height: supported.height})
			restored := model.(Model)
			if !reflect.DeepEqual(before, stateSnapshot(restored)) {
				t.Errorf("%s: restored context differs from exact prior context:\nbefore %+v\nafter %+v",
					tc.name, before, stateSnapshot(restored))
			}
			view := restored.View()
			if view == TooSmallMessage || view == "" || len(strings.Split(view, "\n")) != supported.height {
				t.Errorf("%s: restored view did not apply normal layout: %q", tc.name, view)
			}
		})
	}
}

func TestTooSmallCtrlWRouting(t *testing.T) {
	t.Run("with active cancellable work routes to cancellation flow", func(t *testing.T) {
		cancelled := false
		for _, tc := range tooSmallCases() {
			var model tea.Model = nontrivialModel(100, 40, &cancelled)
			before := stateSnapshot(model.(Model))
			model, _ = model.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			small := model.(Model)
			if !reflect.DeepEqual(before, stateSnapshot(small)) {
				t.Fatalf("%s: hidden state exposed or changed while undersized", tc.name)
			}
			next, cmd := small.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
			routed := next.(Model)
			if cmd == nil {
				t.Fatalf("%s: Ctrl+W returned nil command while hidden state owns cancellable work", tc.name)
			}
			msg := cmd()
			if msg == nil {
				t.Errorf("%s: routed command did not enter the cancellation flow", tc.name)
			} else if _, ok := msg.(cancellationRequested); !ok {
				t.Errorf("%s: unexpected cancellation message %T", tc.name, msg)
			}
			if !cancelled {
				t.Errorf("%s: generic cancellation command was not invoked", tc.name)
			}
			if got := routed.View(); got != TooSmallMessage {
				t.Errorf("%s: Ctrl+W exposed UI state while undersized: %q", tc.name, got)
			}
		}
	})

	t.Run("without cancellable work Ctrl+W is ignored", func(t *testing.T) {
		cancelled := false
		for _, tc := range tooSmallCases() {
			var model tea.Model = nontrivialModel(100, 40, &cancelled)
			nm := model.(Model)
			nm.ActiveCancellable = false
			model, _ = nm.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			var cmd tea.Cmd
			model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
			if cmd != nil {
				t.Errorf("%s: Ctrl+W returned a command with no cancellable work owned", tc.name)
			}
			if cancelled {
				t.Errorf("%s: cancellation command invoked with no cancellable work owned", tc.name)
			}
		}
	})
}
