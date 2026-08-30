# Issue #12: Searchable popup interaction contract walkthrough

*2026-08-27T06:30:09Z by Showboat 0.6.1*
<!-- showboat-id: 0516a67b-e842-4b5c-a01a-626f92ac7a22 -->

This walkthrough demonstrates the Issue #12 searchable popup interaction contract from Notes/issues/012-searchable-popup-interaction-contract.md and the Builder and Display Interaction section of Notes/PRD-sqloid.md. All claims below are backed by executable Go tests in internal/ui.

```bash
go test ./internal/ui -count=1 2>&1 | tail -1
```

```output
ok  	github.com/chris/sqloid/internal/ui	0.007s
```

1. Case-insensitive subsequence matching: every lower-cased query rune must appear after earlier ones in the display; gaps allowed, wrong order rejected, empty search matches everything.

```bash
go test ./internal/ui -run "^TestSubsequenceMatchingIsCaseInsensitive$" -v -count=1 2>&1 | tail -2
```

```output
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

2. Empty search shows all candidates in source order (including repeated case variants); a nonmatching search keeps the popup open with exactly `no matches`; empty candidate data is a permanent open no-match state for both variants.

```bash
go test ./internal/ui -run "^(TestEmptySearchShowsAllCandidatesInSourceOrder|TestNonmatchingSearchKeepsPopupOpenWithExactNoMatches|TestEmptyCandidatesAlwaysReportNoMatches)$" -v -count=1 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|ok|FAIL)"
```

```output
=== RUN   TestEmptySearchShowsAllCandidatesInSourceOrder
--- PASS: TestEmptySearchShowsAllCandidatesInSourceOrder (0.00s)
=== RUN   TestNonmatchingSearchKeepsPopupOpenWithExactNoMatches
--- PASS: TestNonmatchingSearchKeepsPopupOpenWithExactNoMatches (0.00s)
=== RUN   TestEmptyCandidatesAlwaysReportNoMatches
--- PASS: TestEmptyCandidatesAlwaysReportNoMatches (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

3. Deterministic highlight and viewport reset whenever the search text changes — including appends and backspaces while scrolled deep into a long list.

```bash
go test ./internal/ui -run "^TestSearchChangeResetsHighlightAndViewportDeterministically$" -v -count=1 2>&1 | tail -2
```

```output
PASS
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

4. Viewport scrolling clamps at both boundaries: Down stops at the last item with the window pinned to the bottom, Up stops at the first pinned to the top, and the highlight always stays visible after each step.

```bash
go test ./internal/ui -run "^(TestViewportScrollingAtBothBoundaries|TestHighlightedItemAlwaysVisibleAfterStep)$" -v -count=1 2>&1 | grep -E "^(--- (PASS|FAIL)|ok|FAIL)"
```

```output
--- PASS: TestViewportScrollingAtBothBoundaries (0.00s)
--- PASS: TestHighlightedItemAlwaysVisibleAfterStep (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

5. Rendering proves the presentation contract directly: the Search input line, > highlighted rows, exact no-matches text, viewport windows without off-screen leaks, scroll-only popups without any search input, and overlay composition that never reflows base regions.

```bash
go test ./internal/ui -run "^(TestRenderSearchablePopupShowsSearchLineAndRows|TestRenderPopupNoMatchesState|TestRenderPopupReflectsSearchChangeReset|TestRenderPopupWindowRespectsViewportHeight|TestRenderScrollOnlyPopupHasNoSearchInput|TestComposeOverlayNeverReflowsBase)$" -v -count=1 2>&1 | grep -cE "^--- PASS"
```

```output
6
```

6. Lifecycle through Bubble Tea Update: single-select Enter accepts the highlighted candidate, closes the popup, and restores the exact opener focus; printable keys become popup search text without leaking into builder shortcuts; multi-select Enter adds nonduplicate completions and stays open; Esc preserves only already completed multi-selections while restoring exact opener focus.

```bash
go test ./internal/ui -run "^(TestSingleSelectAcceptRestoresExactOpenerFocus|TestSearchInputDoesNotLeakIntoBuilderShortcuts|TestEscCancelRestoresExactOpenerFocus|TestMultiSelectAddReopenAndEscPreservation|TestScrollOnlyVariantThroughUpdate|TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight)$" -v -count=1 2>&1 | grep -E "^(--- PASS|--- FAIL|ok|FAIL)"
```

```output
--- PASS: TestSingleSelectAcceptRestoresExactOpenerFocus (0.00s)
--- PASS: TestSearchInputDoesNotLeakIntoBuilderShortcuts (0.00s)
--- PASS: TestEscCancelRestoresExactOpenerFocus (0.00s)
--- PASS: TestMultiSelectAddReopenAndEscPreservation (0.00s)
--- PASS: TestScrollOnlyVariantThroughUpdate (0.00s)
--- PASS: TestEnterAfterScrollingAndFilteringSelectsVisibleHighlight (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.002s
```

7. Table as first end-to-end searchable single-select consumer: Enter on the focused Table field opens a fresh popup from refreshed Schema eligibility per selected command; typing `ser` under UPDATE narrows to users; Enter commits object identity through the QueryBuilder transition and lands focus back on Table; Esc discards unchanged with exact Table opener restoration; an unrefreshed catalog opens in an inert no-match state until Esc restores; and the drawn overlay keeps total rendered height identical (no reflow).

```bash
go test ./internal/ui -run "^(TestTableEnterOpensFreshSearchablePopup|TestTableAcceptCommitsIdentityAndRestoresOpener|TestTableEscClosesUnchangedAndRestoresExactOpener|TestTablePopupEmptyCatalogStaysOpenWithNoMatches|TestViewDrawsPopupOverResultsWithoutReflow)$" -v -count=1 2>&1 | grep -E "^(--- PASS|--- FAIL|ok|FAIL)"
```

```output
--- PASS: TestTableEnterOpensFreshSearchablePopup (0.00s)
--- PASS: TestTableAcceptCommitsIdentityAndRestoresOpener (0.00s)
--- PASS: TestTableEscClosesUnchangedAndRestoresExactOpener (0.00s)
--- PASS: TestTablePopupEmptyCatalogStaysOpenWithNoMatches (0.00s)
--- PASS: TestViewDrawsPopupOverResultsWithoutReflow (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.003s
```

8. Final verification: gofmt clean, go vet clean, whole module green, build succeeds. Later column, GROUP BY, ORDER BY, aggregate, and operator flows are future consumers of this seam, not implemented scope; database access never enters internal/ui tests.

```bash
gofmt -l internal/ui; go vet ./... && go test ./... >/dev/null && go build ./... && echo "VERIFY OK"
```

```output
VERIFY OK
```
