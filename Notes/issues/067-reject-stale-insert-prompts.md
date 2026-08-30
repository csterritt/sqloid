## Issue 67: Reject stale INSERT prompts before SQL rendering

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Strengthen the authoritative INSERT runnable report so every stored prompt column must still exist in the current insertable-column set. A prompt made stale because its column was hidden, generated, dropped, or otherwise became non-insertable must block execution with a specific stale-column reason, and `InsertSQL` must never quote or bind it. Preserve complete current prompts, their order, Value/NULL/Default choices, and the all-omit `DEFAULT VALUES` path.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Build INSERT prompts, refresh schema so a prompted column becomes hidden/generated or is dropped, and confirm execution is blocked with a specific reason and no stale identifier appears in generated SQL; verify an unchanged all-omit INSERT still renders `DEFAULT VALUES`.
- **Automated**: QueryBuilder tests inject stale prompts for dropped, hidden, generated, and otherwise non-insertable columns and assert a non-runnable report plus empty INSERT output; valid prompt sets assert unchanged column order, parameter order, NULL/omit behavior, and `DEFAULT VALUES`.

### Acceptance criteria

- [ ] Given a stored INSERT prompt whose column is absent from current `InsertableColumns`, when runnable state is checked, then INSERT is rejected with a stale-column reason.
- [ ] Given stale INSERT state, when SQL rendering is requested, then no SQL or parameters containing the stale identifier are emitted.
- [ ] Given every prompt corresponds to a current insertable column and is complete, then existing Value, NULL, omission, ordering, and all-omit behavior remains valid.

### User stories addressed

- User story 35: Prompt only current insertable columns with exact choices
- User story 36: Preserve zero-insertable and `DEFAULT VALUES` behavior
- User story 81: Revalidate insertability after schema changes

---
