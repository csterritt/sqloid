## Issue 76: Insert space in universal value and filename inputs

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Handle Bubble Tea `tea.KeySpace` as printable input in both universal `ValuePrompt` editing and file-picker filename entry. Insert one space at the current cursor using the same rune-aware editing behavior as ordinary text, preserving cursor movement, deletion, submission, parsing, and filename validation. This must work for WHERE/LIKE/SET values, INSERT values, Limit text, and save/export filenames.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Type and edit spaces at the beginning, middle, and end of WHERE/LIKE/SET/INSERT values, Limit input, and filenames; confirm submitted values are verbatim, whitespace Limit remains invalid as documented, and a filename with spaces can be saved.
- **Automated**: Bubble Tea key-sequence tests send `tea.KeySpace` at multiple cursor positions to `ValuePrompt` and filename input, asserting exact text/cursor results, rune-safe edits, verbatim value parsing/binding, Limit validation, and successful filename path construction without changing popup/base-context space behavior.

### Acceptance criteria

- [ ] Given universal value entry is focused, when `tea.KeySpace` is received, then one space is inserted at the cursor and surrounding text/cursor position are preserved correctly.
- [ ] Given filename entry is focused, when `tea.KeySpace` is received, then one space rune is inserted and the resulting valid basename can proceed through save/export.
- [ ] Given spaces occur in ordinary values, LIKE values, or Limit text, then existing verbatim parsing and field-specific validation apply after submission.
- [ ] Given popup search or the base context receives `tea.KeySpace`, then its existing search/toggle behavior remains unchanged and the key is not reinterpreted as value or filename input.

### User stories addressed

- User story 28: Enter WHERE and LIKE values verbatim
- User story 29: Validate bounded Limit text after universal entry
- User story 71: Enter and validate save/export filenames

---
