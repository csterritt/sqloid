# Issue #8 — responsive TUI shell and minimum-size restoration

*2026-08-27T00:19:17Z by Showboat 0.6.1*
<!-- showboat-id: f1cea7a4-5c79-4b01-8b8a-1695b01fcbce -->

Issue #8 (Notes/issues/008-responsive-tui-shell-and-minimum-size-restoration.md) implements the exact region arithmetic from the Resize/layout section of Notes/PRD-sqloid.md: one bottom global footer row, a builder desired height inclusive of its own border and padding capped at floor(H/3), every remaining row assigned to an independently bordered results region that exceeds half-height, and a complete-row page area after the results-owned border, status/count line, and frozen header.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui | sed -E "s/[0-9]+\.[0-9]+s//g"
```

```output
ok  	github.com/chris/sqloid/internal/ui	
```

Pure layout arithmetic evidence at the mandatory 80x24, 100x30, and 160x50 matrix sizes (committed as Example functions so showboat verify re-executes them):

Exact committed arithmetic evidence at the mandatory 80x24, 100x30, and 160x50 matrix sizes. These Example functions (internal/ui/example_test.go) pin the output so it is re-verifiable:

```bash
cd /home/chris/sqloid && sed -n "/Output:/p" internal/ui/example_test.go | sed "s/^[[:space:]]*//;s/\/\/ Output: //"
```

```output
H=24 footer=1 builder(desired=26 capped=8) results=15 pageRows=11
footer=1 builder=10 results=19 half=15 pageRows=15
footer=1 builder=16 results=33 exceedsHalf=true pageRows=29
terminal too small
rows=30 focus=0 fields=22 contentLines=62
```

At 80x24 the builder desired height (26 = 22 content lines + 2 border + 2 padding) is capped at floor(24/3)=8; results take the remaining 15 rows (>12) leaving 11 complete page rows after its owned fixed rows. At 100x30 results=19>15. At 160x50 the uncapped desired 16 fits; results=33>25.

Region arithmetic lives in pure code with explicit per-region fixed-row ownership — no border row is shared:

```bash
cd /home/chris/sqloid && sed -n "/resultsBorderRows/,/resultsFixedRows =/p" internal/ui/layout.go
```

```output
	resultsBorderRows  = 2 // top and bottom border rows owned by results
	resultsStatusRows  = 1 // status/count line inside the results border
	resultsHeaderRows  = 1 // frozen header inside the results border

	resultsFixedRows = resultsBorderRows + resultsStatusRows + resultsHeaderRows
```

Rendered-row ownership is tested at all three sizes: the whole View renders exactly H rows, the footer occupies exactly the last row, each box occupies exactly its owned height, and exactly two top-border corner rows exist in the full screen (independent borders, none shared):

```bash
cd /home/chris/sqloid && go test ./internal/ui -run TestViewRegionOwnership -v -count=1 | grep -E "^(=== RUN|--- |ok)"
```

```output
=== RUN   TestViewRegionOwnership
=== RUN   TestViewRegionOwnership/80x24
=== RUN   TestViewRegionOwnership/100x30
=== RUN   TestViewRegionOwnership/160x50
--- PASS: TestViewRegionOwnership (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.004s
```

Long builder content and focus movement: with content far beyond the floor(H/3) cap, every focus move keeps the complete multiline focused field inside the visible range through internal scrolling:

```bash
cd /home/chris/sqloid && go test ./internal/ui -run TestFocusedFieldScrolling -v -count=1 | grep -E "^(--- |ok)" && sed -n "/func (m \*Model) adjustScroll/,/^}/p" internal/ui/layout.go
```

```output
--- PASS: TestFocusedFieldScrolling (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
func (m *Model) adjustScroll() {
	starts, counts := fieldSpans(m.Fields)
	total := 0
	for _, c := range counts {
		total += c
	}
	viewport := CalculateLayout(m.Height, m.Fields).BuilderViewport()
	if viewport >= total {
		m.Scroll = 0
		return
	}
	maxScroll := total - viewport
	start, count := starts[m.Focus], counts[m.Focus]
	scroll := m.Scroll
	if start < scroll {
		scroll = start
	}
	if start+count > scroll+viewport {
		scroll = start + count - viewport
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	m.Scroll = scroll
}
```

```bash
cd /home/chris/sqloid && go test ./internal/ui -run TestFocusedFieldScrolling -v -count=1 2>&1 | grep -E "^--- " || true
```

```output
--- PASS: TestFocusedFieldScrolling (0.00s)
```

Below the exact 80x24 minimum the model suspends: View returns exactly 'terminal too small', hidden state (context, focus, scroll, cancellable-request ownership) is retained unchanged, ordinary keys are ignored without leaking or mutating anything, and resizing back restores context and focus exactly before applying normal layout:

```bash
cd /home/chris/sqloid && go test ./internal/ui -run "TestTooSmallExactView|TestTooSmallPreservesAndRestoresState" -v -count=1 | grep -E "^(--- |ok)"
```

```output
--- PASS: TestTooSmallExactView (0.00s)
--- PASS: TestTooSmallPreservesAndRestoresState (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.004s
```

Ctrl+W while undersized routes into the generic cancellation flow only when hidden state owns active cancellable work; with nothing cancellable it is ignored — and routing never exposes or mutates the hidden state. The global quit seam is preserved untouched for later key-precedence work:

```bash
cd /home/chris/sqloid && go test ./internal/ui -run TestTooSmallCtrlWRouting -v -count=1 | grep -E "^(=== RUN|--- |ok)"
```

```output
=== RUN   TestTooSmallCtrlWRouting
=== RUN   TestTooSmallCtrlWRouting/with_active_cancellable_work_routes_to_cancellation_flow
=== RUN   TestTooSmallCtrlWRouting/without_cancellable_work_Ctrl+W_is_ignored
--- PASS: TestTooSmallCtrlWRouting (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

showboat verify re-executes every captured block and confirms all outputs match (verified with exit status 0).
