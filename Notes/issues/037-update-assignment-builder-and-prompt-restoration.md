## Issue 37: UPDATE assignment builder and prompt restoration

**Type**: AFK
**Blocked by**: Issue 14, Issue 17, Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver UPDATE construction end to end through unique SET-column selection, ordered Value/NULL choices, universal Value entry, optional shared WHERE, runnable feedback, SQL generation, and exact prompt revisiting.

### How to verify

- **Manual**: Build an UPDATE mixing Value and NULL assignments, revisit every prompt, add WHERE, and inspect generated SQL and parameters.
- **Automated**: QueryBuilder/model tests assert uniqueness, completion, stored choices/text/types, NULL keyword omission from params, SET-then-WHERE parameter order, and history-ready state.

### Acceptance criteria

- [ ] Given selected SET columns, then each has exactly one complete Value or NULL choice and Default/Omit is never offered.
- [ ] Given mixed assignments and WHERE, then Value params follow SET order, NULL adds none, and the WHERE value is last.
- [ ] Given prompt revision, then prior choices, entered text, and bound types are restored exactly.

### User stories addressed

- User story 33: Build UPDATE assignments and optional WHERE safely
- User story 37: Revisit write prompts with exact prior state

---
