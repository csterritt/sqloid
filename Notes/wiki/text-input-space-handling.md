# Text-input space handling (Issue #68)

Bubble Tea `tea.KeySpace` inserts exactly one U+0020 at the rune cursor only while a universal value prompt or the file picker's filename input owns focus. Every other context retains its prior space behavior: searchable popup search appends a space to the query, the base builder context consumes space as a no-op, and the directory-focused picker does not insert into the filename buffer. This page documents the routing, rune-aware editing model, verbatim value semantics, Limit validation, filename completion, and context isolation introduced by Issue #68, per the [Global Key Precedence and Context/Action Matrix](../PRD-sqloid.md), the [UI Module Design](../PRD-sqloid.md), and the [Testing Decisions](../PRD-sqloid.md) in `Notes/PRD-sqloid.md`.

## KeySpace routing

- **Universal value prompt** (`ValuePrompt.HandleKey` in `internal/ui/value_input.go`): `tea.KeySpace` calls the same `insertRunes` helper as `tea.KeyRunes`, inserting exactly one U+0020 at the current rune cursor, advancing the cursor by one rune, and reporting `true`. The shared insertion path preserves the verbatim buffer storage and rune-index cursor invariants; no whitespace reinterpretation, trimming, or synthetic `KeyRunes` message is constructed.
- **File picker filename** (`applyPickerFilenameKey` in `internal/ui/filepicker.go`): `tea.KeySpace` calls `picker.InsertRunes` with exactly one space rune, complementing the `ValuePrompt` handling. The existing `internal/filepicker` validation, required-extension completion, and path-join logic consume the verbatim filename buffer without trimming or special whitespace parsing.
- **Searchable popup search**: `tea.KeySpace` appends one space to the popup search query through `Popup.AppendSearchRune(' ')`, unchanged from prior behavior. Space never leaks into a value prompt or builder state while a popup is open.
- **Base builder context**: `tea.KeySpace` is a no-op — it does not open a popup, move focus, mutate builder state, or open a value prompt. The base context owns no focused text input, so space has no target.
- **Directory-focused picker**: `tea.KeySpace` is consumed as a no-op — it does not insert into the filename buffer, navigate the listing, or change the directory. Only filename focus routes space into the buffer.

## Rune-aware editing model

A space inserted by `KeySpace` uses the ordinary rune-aware editing model shared with `KeyRunes`:

- **Left/Right** (value prompt) move the cursor by one rune across the inserted space.
- **Backspace/Delete** remove exactly the space rune when targeted.
- **Home/End, Ctrl+A/Ctrl+E** jump to the buffer boundaries across the space.
- **Ctrl+U** clears the whole buffer including the space.

The file picker filename buffer supports the same editing keys through `picker.Left`/`Right`/`Backspace`/`Delete`/`Home`/`End` and Ctrl+U clearing, with the picker-level exception that Left/Right toggle the export save format (CSV/JSON) rather than moving the filename cursor — Home/End/Ctrl+A/Ctrl+E position the cursor at the boundaries.

## Verbatim value submission

WHERE/LIKE/SET/INSERT Value choices preserve the exact entered representation when spaces are typed through `tea.KeySpace`:

- **WHERE equality (`=`)**: leading, internal, trailing, all-space, and Unicode-adjacent space buffers bind as verbatim TEXT parameters through the [universal parser](sql-atoms-and-literals.md). The committed SQL and parameter order are unchanged.
- **WHERE LIKE**: spaces and `%`/`_` wildcards are preserved byte-for-byte in the bound TEXT parameter.
- **UPDATE SET Value**: the exact entered text binds as verbatim TEXT through the [UPDATE assignment builder](update-assignment-builder.md), with SET-then-WHERE parameter order unchanged.
- **INSERT Value**: the exact entered text binds as verbatim TEXT through the [INSERT builder](insert-builder.md), with schema-order parameter placement unchanged.

No trimming, normalization, or whitespace reinterpretation occurs at the UI layer; parsing and type classification remain owned by `QueryBuilder`'s universal parser per Issue #14.

## Limit validation

Limit text containing spaces or whitespace-only text — typed through `tea.KeySpace` — receives the existing exact field-specific invalid reason: `Limit must be an integer from 1 to 9223375807`. The entered text is preserved verbatim in `LimitInput`, no accepted value is produced, and the first-invalid report identifies `RunFieldLimit` with the exact reason. See [group-order-limit.md](group-order-limit.md) and [runnable-state-feedback.md](runnable-state-feedback.md).

## Filename completion with spaces

Valid filenames containing spaces — typed through `tea.KeySpace` — complete through the established [file picker](file-picker.md) validation, required-extension, and path-join logic:

- Spaces at the beginning, middle, and end of ASCII and Unicode basenames are preserved verbatim.
- The required `.sql`/`.csv`/`.json` extension is appended exactly once when missing.
- The completed basename is joined to the selected directory at submission, producing the verified destination (e.g. `/work/my report.sql`, `/work/数据 导出.csv`).
- Empty basenames and basenames containing `/` or NUL are rejected inline before any destination is constructed, unchanged from prior behavior.

## Context isolation

- **Popup search space** retains its search behavior: `tea.KeySpace` appends to the popup search query and does not open or leak into a value prompt.
- **Base-context space** retains its no-op behavior: `tea.KeySpace` does not open a popup, move focus, or mutate builder state.
- **Directory-focused picker space** does not become filename input: `tea.KeySpace` is consumed without inserting into the filename buffer, navigating the listing, or changing the directory.
- **Ctrl+C** still opens the quit confirmation from any focused input or picker focus, suspending the exact context and restoring it on Esc/n.

## Testing

- `internal/ui/value_input_space_test.go` (Issue #68 Task 1): `ValuePrompt.HandleKey` direct coverage of `tea.KeySpace` at empty, ASCII, and multibyte Unicode buffers with cursor positions at start, middle, and end; exact one-U+0020 insertion, one-rune cursor advance, unchanged surrounding runes, and `true` change reporting; `KeySpace` matches `KeyRunes` carrying one space; Left/Right, Backspace/Delete, Home/End, Ctrl+A/Ctrl+E, and Ctrl+U editing after a space; `KeyRunes` control.
- `internal/ui/space_submission_test.go` (Issue #68 Task 3): scripted `Update` coverage of WHERE equality, WHERE LIKE, UPDATE SET, INSERT Value, and Limit flows using `tea.KeySpace` for every space; verbatim TEXT preservation for leading, internal, trailing, all-space, and Unicode-adjacent buffers; existing Limit invalid reason for whitespace-only and space-containing text; searchable popup space retains search; base-context space is a no-op.
- `internal/ui/picker_flow_test.go` (Issue #68 Task 3): `tea.KeySpace` at the beginning, middle, and end of ASCII/Unicode filenames; `KeySpace` matches `KeyRunes` carrying one space; valid spaced basenames complete through SQL, CSV, and JSON extension/path construction; directory-focus space does not become filename input.

## Cross-references

- [Issue #68](../issues/) — Insert space in universal value and filename inputs
- [sql-atoms-and-literals.md](sql-atoms-and-literals.md) — universal value parsing
- [group-order-limit.md](group-order-limit.md) — Limit validation
- [file-picker.md](file-picker.md) — file picker filename completion
- [contextual-help-and-overlay-precedence.md](contextual-help-and-overlay-precedence.md) — context/action matrix and key precedence
- [where-guided-predicates.md](where-guided-predicates.md) — WHERE guided flow
- [update-assignment-builder.md](update-assignment-builder.md) — UPDATE SET flow
- [insert-builder.md](insert-builder.md) — INSERT Value flow
- [searchable-popups.md](searchable-popups.md) — popup search behavior
- `Notes/PRD-sqloid.md` — Global Key Precedence and Context/Action Matrix, UI Module Design, Testing Decisions
