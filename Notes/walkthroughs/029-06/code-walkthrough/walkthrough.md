# Issue #29 — Whole-column horizontal scrolling code walkthrough

*2026-08-28T16:38:17Z by Showboat 0.6.1*
<!-- showboat-id: 787ab5c5-ffe6-4762-9ed4-0420ad6b54cc -->

Issue #29 makes the result grid scroll horizontally in whole-column units only (Notes/PRD-sqloid.md, Builder and Display Interaction; Global Key Precedence and Context/Action Matrix): the grid's horizontal position is exactly one first-visible output-column index, Shift+Page Down and `.` advance it exactly one whole column, Shift+Page Up and `,` retreat exactly one, boundaries are no-ops, widths are recomputed from the new first column, an oversized column is capped and ellipsized without intra-cell scrolling, and resize preserves then clamps the index. The pure seam lives in internal/ui/horizontal_layout.go, the bindings in internal/ui/horizontal_keys.go, and the rendering integration in internal/ui/results_grid.go.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestVisibleGridLayout|TestVisibleGridLayoutExposesNoIntraCellOffset|TestHorizontalStepBoundaries|TestClampFirstColumnOnResize' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed -E 's/[0-9]+(\.[0-9]+)?s\b/NT/g'
```

```output
=== RUN   TestVisibleGridLayout
=== RUN   TestVisibleGridLayout/all_columns_fit_on_a_wide_terminal
=== RUN   TestVisibleGridLayout/narrow_terminal_fits_only_the_first_column
=== RUN   TestVisibleGridLayout/layout_restarts_at_the_selected_first-visible_index
=== RUN   TestVisibleGridLayout/multiple_columns_fit_and_widths_are_recomputed_per_pass
=== RUN   TestVisibleGridLayout/exact-fit_boundary_packs_the_final_column
=== RUN   TestVisibleGridLayout/no_room_for_another_complete_column_excludes_it
=== RUN   TestVisibleGridLayout/unicode_display_widths_count_double-width_runes
=== RUN   TestVisibleGridLayout/unicode_columns_pack_by_display_width,_not_rune_count
=== RUN   TestVisibleGridLayout/oversized_first_column_is_capped_to_the_available_cell_area
=== RUN   TestVisibleGridLayout/a_column_capped_at_the_grid_cap_still_packs_following_columns
=== RUN   TestVisibleGridLayout/first_index_below_zero_clamps_to_the_first_column
=== RUN   TestVisibleGridLayout/first_index_beyond_the_last_column_clamps_to_it
=== RUN   TestVisibleGridLayout/empty_results_yield_a_zero_layout_at_any_index
--- PASS: TestVisibleGridLayout (NT)
=== RUN   TestVisibleGridLayoutExposesNoIntraCellOffset
--- PASS: TestVisibleGridLayoutExposesNoIntraCellOffset (NT)
=== RUN   TestHorizontalStepBoundaries
=== RUN   TestHorizontalStepBoundaries/advance_from_the_first_column
=== RUN   TestHorizontalStepBoundaries/advance_within_the_columns
=== RUN   TestHorizontalStepBoundaries/advance_at_the_last_column_is_a_no-op
=== RUN   TestHorizontalStepBoundaries/retreat_from_the_last_column
=== RUN   TestHorizontalStepBoundaries/retreat_at_the_first_column_is_a_no-op
=== RUN   TestHorizontalStepBoundaries/single_column_never_moves
=== RUN   TestHorizontalStepBoundaries/single_column_never_retreats
=== RUN   TestHorizontalStepBoundaries/no_columns_never_moves
=== RUN   TestHorizontalStepBoundaries/no_columns_never_retreats
--- PASS: TestHorizontalStepBoundaries (NT)
=== RUN   TestClampFirstColumnOnResize
=== RUN   TestClampFirstColumnOnResize/valid_first_index_preserved
=== RUN   TestClampFirstColumnOnResize/valid_last_index_preserved
=== RUN   TestClampFirstColumnOnResize/valid_middle_index_preserved
=== RUN   TestClampFirstColumnOnResize/index_beyond_a_shrunken_column_set_clamps_to_the_last
=== RUN   TestClampFirstColumnOnResize/index_reduced_to_one_column_clamps_to_it
=== RUN   TestClampFirstColumnOnResize/negative_index_clamps_to_the_first
=== RUN   TestClampFirstColumnOnResize/empty_results_clamp_to_zero
--- PASS: TestClampFirstColumnOnResize (NT)
PASS
ok  	github.com/chris/sqloid/internal/ui	NT
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestHorizontalKeysMoveExactlyOneColumn|TestShiftPageBridgeFromRawCSI|TestHorizontalKeysNoOpAtBoundaries|TestHorizontalMovementIssuesNoDatabaseCommand|TestHorizontalMovementStaysLocalWhileRequestsPending' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed -E 's/[0-9]+(\.[0-9]+)?s\b/NT/g'
```

```output
=== RUN   TestShiftPageBridgeFromRawCSI
--- PASS: TestShiftPageBridgeFromRawCSI (NT)
=== RUN   TestHorizontalKeysMoveExactlyOneColumn
=== RUN   TestHorizontalKeysMoveExactlyOneColumn/`.`
=== RUN   TestHorizontalKeysMoveExactlyOneColumn/Shift+Page_Down
=== RUN   TestHorizontalKeysMoveExactlyOneColumn/`,`
=== RUN   TestHorizontalKeysMoveExactlyOneColumn/Shift+Page_Up
--- PASS: TestHorizontalKeysMoveExactlyOneColumn (NT)
=== RUN   TestHorizontalKeysNoOpAtBoundaries
--- PASS: TestHorizontalKeysNoOpAtBoundaries (NT)
=== RUN   TestHorizontalMovementIssuesNoDatabaseCommand
--- PASS: TestHorizontalMovementIssuesNoDatabaseCommand (NT)
=== RUN   TestHorizontalMovementStaysLocalWhileRequestsPending
=== RUN   TestHorizontalMovementStaysLocalWhileRequestsPending/first_page_pending
=== RUN   TestHorizontalMovementStaysLocalWhileRequestsPending/later_page_pending
=== RUN   TestHorizontalMovementStaysLocalWhileRequestsPending/count_pending
--- PASS: TestHorizontalMovementStaysLocalWhileRequestsPending (NT)
PASS
ok  	github.com/chris/sqloid/internal/ui	NT
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestHorizontalKeysConsumedByHigherPrecedenceContexts|TestHorizontalMovementRendersNewColumns|TestHorizontalClampsFirstColumnOnResize' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed -E 's/[0-9]+(\.[0-9]+)?s\b/NT/g'
```

```output
=== RUN   TestHorizontalMovementRendersNewColumns
--- PASS: TestHorizontalMovementRendersNewColumns (NT)
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts/quit_confirmation
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts/popup_overlay
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts/focused_input
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts/too-small_screen
=== RUN   TestHorizontalKeysConsumedByHigherPrecedenceContexts/terminal_state
--- PASS: TestHorizontalKeysConsumedByHigherPrecedenceContexts (NT)
=== RUN   TestHorizontalClampsFirstColumnOnResize
=== RUN   TestHorizontalClampsFirstColumnOnResize/valid_index_preserved_across_widths
=== RUN   TestHorizontalClampsFirstColumnOnResize/invalid_index_clamped_to_the_last_column_on_resize
=== RUN   TestHorizontalClampsFirstColumnOnResize/single-column_result_clamps_to_zero
=== RUN   TestHorizontalClampsFirstColumnOnResize/empty_columns_clamp_to_zero
--- PASS: TestHorizontalClampsFirstColumnOnResize (NT)
PASS
ok  	github.com/chris/sqloid/internal/ui	NT
```

```bash
cd /home/chris/sqloid && grep -rn 'offset' internal/ui/horizontal_layout.go internal/ui/horizontal_keys.go | grep -v '^.*://' ; echo '--- layout struct fields:' && sed -n '/type gridVisibleLayout struct/,/^}/p' internal/ui/horizontal_layout.go
```

```output
internal/ui/horizontal_layout.go:55:			// cells ellipsize within it. No intra-cell offset is produced.
--- layout struct fields:
type gridVisibleLayout struct {
	First  int
	Widths []int
	Total  int
}
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ 2>&1 | tail -1 | sed -E 's/[0-9]+(\.[0-9]+)?s\b/NT/g' && gofmt -l internal/ui && go vet ./... && go build ./...
```

```output
ok  	github.com/chris/sqloid/internal/ui	NT
```

The executed runs capture the complete Issue #29 contract: the pure layout table drives narrow terminals (only the first column fits), wide terminals (multiple columns fit), Unicode double-width display widths, exact-fit boundaries, no-room-for-another-complete-column exclusion, oversized-first-column capping to the available cell area, grid-cap columns still packing followers, index clamping at both ends, idempotent recomputation (the layout struct carries only First/Widths/Total — no character or byte offset exists anywhere, so no intra-cell scroll state is possible), boundary stepping, and resize clamping. The scripted model runs capture all four bindings (Shift+Page Down and `.`, Shift+Page Up and `,`, plus the raw xterm CSI bridge) moving exactly one whole column per accepted press, both boundary no-ops, no command and unchanged page-request counts for accepted moves, movement staying local while first-page, later-page, and count work is held pending, higher-precedence context consumption (quit confirmation, popup overlay, focused input, too-small screen, terminal state), rendering that hides the abandoned first column and reveals the newly visible one after a move on a narrow viewport, and resize preservation with clamping at beyond-last, single-column, and empty results. The final run proves the full internal/ui suite, gofmt, go vet, and go build stay clean.
