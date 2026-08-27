package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Example_shellRows80x24 prints the exact region rows the shell assigns
// at the minimum supported terminal size with a growing builder.
func Example_shellRows80x24() {
	l := CalculateLayout(24, largeFields(20))
	fmt.Printf("H=24 footer=%d builder(desired=%d capped=%d) results=%d pageRows=%d",
		FooterHeight, l.BuilderDesired, l.BuilderHeight, l.ResultsHeight, l.PageRows)
	// Output: H=24 footer=1 builder(desired=26 capped=8) results=15 pageRows=11
}

// Example_shellRows100x30 prints the exact region rows at 100x30 with a
// builder far beyond its floor(H/3) cap.
func Example_shellRows100x30() {
	l := CalculateLayout(30, largeFields(40))
	fmt.Printf("footer=%d builder=%d results=%d half=%d pageRows=%d",
		FooterHeight, l.BuilderHeight, l.ResultsHeight, l.TotalHeight/2, l.PageRows)
	// Output: footer=1 builder=10 results=19 half=15 pageRows=15
}

// Example_shellRows160x50 prints the exact region rows at 160x50 where
// the desired builder height fits beneath its cap.
func Example_shellRows160x50() {
	l := CalculateLayout(50, largeFields(10))
	fmt.Printf("footer=%d builder=%d results=%d exceedsHalf=%v pageRows=%d",
		FooterHeight, l.BuilderHeight, l.ResultsHeight,
		l.ResultsHeight > l.TotalHeight/2, l.PageRows)
	// Output: footer=1 builder=16 results=33 exceedsHalf=true pageRows=29
}

// Example_tooSmallExactView prints exactly what View returns below the minimum while
// hidden state stays retained behind the message.
func Example_tooSmallExactView() {
	m := New()
	m.Fields = []Field{{Label: "Command", Content: "SELECT"}}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 79, Height: 23})
	fmt.Print(next.View())
	// Output: terminal too small
}

// Example_restoredLayout reports how the restored shell lays out after coming
// back above the minimum: normal arithmetic applies without state reset.
func Example_restoredLayout() {
	m := New()
	m.Fields = largeFields(60)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 79, Height: 23})
	small := next.(Model)
	next, _ = small.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	back := next.(Model)
	fmt.Printf("rows=%d focus=%d fields=%d contentLines=%d",
		strings.Count(back.View(), "\n")+1, back.Focus, len(back.Fields),
		sumFieldLines(back.Fields))
	// Output: rows=30 focus=0 fields=22 contentLines=62
}
