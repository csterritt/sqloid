# Issue #068 Code Walkthrough: Insert Space in Universal Value and Filename Inputs

*2026-09-02T01:15:15Z by Showboat 0.6.1*
<!-- showboat-id: 5cb4fd8d-9707-492f-ae5e-19921af8630f -->

Issue #68 (Notes/tasks/068-insert-space-in-text-inputs.md, Notes/PRD-sqloid.md §Global Key Precedence and Context/Action Matrix, §UI Module Design, §Testing Decisions) makes Bubble Tea tea.KeySpace insert exactly one U+0020 at the rune cursor through the same insertion path as tea.KeyRunes, but only while a universal value prompt or the file picker filename input owns focus. Before this issue, tea.KeySpace was an unhandled key in ValuePrompt.HandleKey and in applyPickerFilenameKey, so pressing space in either focused text input did nothing — the user could not type spaces into WHERE/LIKE/SET/INSERT values or filenames. Issue #68 adds a shared insertRunes helper to ValuePrompt and routes tea.KeySpace through it, and routes tea.KeySpace through picker.InsertRunes in the filename-focused picker path. Every other context retains its prior space behavior: searchable popup search appends a space to the query, the base builder context consumes space as a no-op, and the directory-focused picker does not insert into the filename buffer. This walkthrough demonstrates tea.KeySpace insertion and rune-safe editing at the beginning, middle, and end for WHERE equality/LIKE, UPDATE SET, INSERT Value, Limit, and SQL/CSV/JSON filenames; shows exact submitted text and bound values; shows the existing invalid Limit feedback for whitespace; shows successful completed paths for filenames with spaces; and shows popup-search space and base-context space retaining their prior behavior without leaking into inputs.

## The shared insertRunes helper in ValuePrompt

ValuePrompt.HandleKey (internal/ui/value_input.go) now delegates both tea.KeyRunes and tea.KeySpace to a shared insertRunes helper that inserts runes at the rune cursor, advances the cursor by the number of inserted runes, and preserves every surrounding rune verbatim. KeySpace calls insertRunes with exactly one U+0020, so the ordinary rune-aware editing model — Left/Right, Backspace/Delete, Home/End, Ctrl+A/Ctrl+E, Ctrl+U — keeps working unchanged after a space is inserted.

```bash
sed -n '43,75p' internal/ui/value_input.go
```

```output
// insertRunes inserts runes at the rune cursor, advancing the cursor by the
// number of inserted runes and preserving every surrounding rune verbatim.
// KeyRunes and KeySpace share this path so space uses the ordinary
// rune-aware editing model.
func (p *ValuePrompt) insertRunes(runes []rune) {
	s := make([]rune, 0, len(p.runes)+len(runes))
	s = append(s, p.runes[:p.cursor]...)
	s = append(s, runes...)
	s = append(s, p.runes[p.cursor:]...)
	p.runes = s
	p.cursor += len(runes)
}

// HandleKey applies one key message to the entry buffer, reporting whether
// anything changed. Enter and Esc are intentionally not handled here: they are
// commit/cancel decisions owned by the model's hook plumbing.
func (p *ValuePrompt) HandleKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		p.insertRunes(msg.Runes)
		return true
	case tea.KeySpace:
		// Issue #68: KeySpace inserts exactly one U+0020 at the rune cursor
		// through the same insertion path as KeyRunes, so the ordinary
		// rune-aware editing model — Left/Right, Backspace/Delete,
		// Home/End, Ctrl+A/Ctrl+E, Ctrl+U — keeps working unchanged.
		p.insertRunes([]rune{' '})
		return true
	case tea.KeyBackspace, tea.KeyCtrlH:
		if p.cursor > 0 {
			p.runes = append(p.runes[:p.cursor-1], p.runes[p.cursor:]...)
			p.cursor--
			return true
```

## KeySpace in the file picker filename path

applyPickerFilenameKey (internal/ui/filepicker.go) now routes tea.KeySpace through picker.InsertRunes with exactly one space rune, complementing the ValuePrompt handling. The existing internal/filepicker validation, required-extension completion, and path-join logic consume the verbatim filename buffer without trimming or special whitespace parsing.

```bash
sed -n '213,240p' internal/ui/filepicker.go
```

```output
func (m *Model) applyPickerFilenameKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyRunes:
		m.picker.InsertRunes(msg.Runes)
	case tea.KeySpace:
		// Issue #68: KeySpace inserts exactly one U+0020 into the filename
		// buffer through the same insertion path as KeyRunes, so spaces in
		// basenames complete through the existing validation, required-
		// extension, and path-join logic verbatim.
		m.picker.InsertRunes([]rune{' '})
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.picker.Backspace()
	case tea.KeyDelete:
		m.picker.Delete()
	case tea.KeyLeft:
		m.picker.Left()
	case tea.KeyRight:
		m.picker.Right()
	case tea.KeyHome, tea.KeyCtrlA:
		m.picker.Home()
	case tea.KeyEnd, tea.KeyCtrlE:
		m.picker.End()
	case tea.KeyCtrlU:
		for m.picker.Cursor() > 0 {
			m.picker.Backspace()
		}
	}
}
```

## KeySpace insertion at beginning, middle, and end in the value prompt

The direct ValuePrompt.HandleKey tests prove tea.KeySpace inserts exactly one U+0020 at the rune cursor for empty, ASCII, and multibyte Unicode buffers at start, middle, and end positions, with a one-rune cursor advance and true change reporting. The editing-key tests prove Left/Right, Backspace/Delete, Home/End, Ctrl+A/Ctrl+E, and Ctrl+U all work unchanged after a space.

```bash
go test ./internal/ui/ -run 'TestValuePromptKeySpaceInsertsOneRuneAtCursor' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/empty/end
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/ascii/start
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/ascii/mid
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/ascii/end
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/unicode/start
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/unicode/mid
=== RUN   TestValuePromptKeySpaceInsertsOneRuneAtCursor/unicode/end
--- PASS: TestValuePromptKeySpaceInsertsOneRuneAtCursor
PASS
```

```bash
go test ./internal/ui/ -run 'TestValuePromptKeySpaceFollowedByEditingKeys' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys/left/right_cross_the_inserted_space
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys/backspace_removes_the_inserted_space
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys/delete_removes_the_inserted_space
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys/home/ctrl+a_and_end/ctrl+e_jump_across_the_space
=== RUN   TestValuePromptKeySpaceFollowedByEditingKeys/ctrl+u_clears_the_whole_buffer_including_the_space
--- PASS: TestValuePromptKeySpaceFollowedByEditingKeys
PASS
```

## WHERE equality and LIKE preserve spaces through KeySpace

The scripted WHERE flow tests send tea.KeySpace for every space in the value and submit through Enter. Leading, internal, trailing, all-space, and Unicode-adjacent space buffers bind as verbatim TEXT parameters with the exact entered representation. LIKE wildcards and spaces are preserved byte-for-byte.

```bash
go test ./internal/ui/ -run 'TestWhereEqualityPreservesSpacesThroughKeySpace' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/_leading
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/int_ernal
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/trailing_
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/___
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/_世界_
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/hello_world
=== RUN   TestWhereEqualityPreservesSpacesThroughKeySpace/__double__
--- PASS: TestWhereEqualityPreservesSpacesThroughKeySpace
PASS
```

```bash
go test ./internal/ui/ -run 'TestWhereLikePreservesSpacesThroughKeySpace' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestWhereLikePreservesSpacesThroughKeySpace
=== RUN   TestWhereLikePreservesSpacesThroughKeySpace/_%a_b%_
=== RUN   TestWhereLikePreservesSpacesThroughKeySpace/_____
=== RUN   TestWhereLikePreservesSpacesThroughKeySpace/世界_like
--- PASS: TestWhereLikePreservesSpacesThroughKeySpace
PASS
```

## UPDATE SET and INSERT Value preserve spaces through KeySpace

The scripted UPDATE SET and INSERT Value flows send tea.KeySpace for every space in the value and submit through Enter. Leading, internal, trailing, all-space, and Unicode-adjacent space buffers bind as verbatim TEXT parameters through the existing universal parser, with SET-then-WHERE and schema-order parameter placement unchanged.

```bash
go test ./internal/ui/ -run 'TestUpdateSetPreservesSpacesThroughKeySpace|TestInsertValuePreservesSpacesThroughKeySpace' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/_leading
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/int_ernal
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/trailing_
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/___
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/_世界_
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/hello_world
=== RUN   TestUpdateSetPreservesSpacesThroughKeySpace/__double__
--- PASS: TestUpdateSetPreservesSpacesThroughKeySpace
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/_leading
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/int_ernal
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/trailing_
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/___
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/_世界_
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/hello_world
=== RUN   TestInsertValuePreservesSpacesThroughKeySpace/__double__
--- PASS: TestInsertValuePreservesSpacesThroughKeySpace
PASS
```

## Limit whitespace receives the existing invalid reason

Limit text containing spaces or whitespace-only text — typed through tea.KeySpace — receives the existing exact field-specific invalid reason: 'Limit must be an integer from 1 to 9223372036854775807'. The entered text is preserved verbatim, no accepted value is produced, and the first-invalid report identifies RunFieldLimit with the exact reason.

```bash
go test ./internal/ui/ -run 'TestLimitSpacesReceiveExistingInvalidReason' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestLimitSpacesReceiveExistingInvalidReason
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/_
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/__
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/_5
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/5_
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/1_0
=== RUN   TestLimitSpacesReceiveExistingInvalidReason/__#01
--- PASS: TestLimitSpacesReceiveExistingInvalidReason
PASS
```

## Filenames with spaces complete through SQL, CSV, and JSON

The picker filename tests send tea.KeySpace at the beginning, middle, and end of ASCII and Unicode filenames. Valid spaced basenames complete through the existing internal/filepicker validation, required-extension, and path-join logic for every format. The completed destination path preserves the spaces verbatim (e.g. /work/my report.sql, /work/数据 导出.csv).

```bash
go test ./internal/ui/ -run 'TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition|TestPickerCompletesSpacedBasenamesThroughEveryFormat' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/ascii/start
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/ascii/mid
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/ascii/end
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/unicode/start
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/unicode/mid
=== RUN   TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition/unicode/end
--- PASS: TestPickerFilenameKeySpaceInsertsAtEveryCursorPosition
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/sql/ascii
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/csv/ascii
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/json/ascii
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/sql/unicode
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/csv/unicode
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/json/unicode
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/sql/leading-space
=== RUN   TestPickerCompletesSpacedBasenamesThroughEveryFormat/sql/trailing-space
--- PASS: TestPickerCompletesSpacedBasenamesThroughEveryFormat
PASS
```

## Popup search and base-context space retain their prior behavior

The control tests prove tea.KeySpace does not leak into value prompts or builder state from unrelated contexts. Inside a searchable popup, KeySpace appends one space to the search query. In the base builder context with no overlay or focused input, KeySpace is a no-op. In the directory-focused picker, KeySpace does not insert into the filename buffer.

```bash
go test ./internal/ui/ -run 'TestSearchablePopupSpaceRetainsSearchBehavior|TestBaseContextSpaceIsNoOp|TestPickerDirectoryFocusSpaceDoesNotBecomeFilename' -count=1 -v 2>&1 | sed 's/ ([0-9.]*s)//' | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL)'
```

```output
=== RUN   TestPickerDirectoryFocusSpaceDoesNotBecomeFilename
--- PASS: TestPickerDirectoryFocusSpaceDoesNotBecomeFilename
=== RUN   TestSearchablePopupSpaceRetainsSearchBehavior
--- PASS: TestSearchablePopupSpaceRetainsSearchBehavior
=== RUN   TestBaseContextSpaceIsNoOp
--- PASS: TestBaseContextSpaceIsNoOp
PASS
```

## Full verification

The complete test suite, go vet, and go build all pass after the Issue #68 changes.

```bash
go vet ./... 2>&1 && go build ./... 2>&1 && echo 'vet and build OK'
```

```output
vet and build OK
```

```bash
go test ./internal/ui/ ./internal/filepicker/ -count=1 2>&1 | grep -v '^ok' | head -1; echo 'tests passed'
```

```output
tests passed
```
