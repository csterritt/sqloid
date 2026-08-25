## Issue 17: Authoritative runnable-state feedback

**Type**: AFK
**Blocked by**: Issue 15, Issue 16

### Parent PRD

`PRD-sqloid.md`

### What to build

Expose a UI-independent QueryBuilder runnable report and connect it to Enter. Apply the command prerequisites and common gates in **Runnable-State Contract**, focusing the first invalid field in visual order with a specific reason. This issue delivers the **general** runnable framework for all four commands (SELECT, UPDATE, DELETE, INSERT), not SELECT alone. Write-specific prerequisites (SET column uniqueness and Value/NULL completion for UPDATE, optional WHERE completion for DELETE, per-column Value/NULL/Default completion and zero-insertable-column blocking for INSERT) are part of this framework even though their end-to-end exercise occurs in Issues 33, 34, and 35.

### How to verify

- **Manual**: Attempt Enter from representative incomplete and invalid SELECT states, then correct the focused field.
- **Automated**: Table-driven runnable tests and scripted model tests assert every prerequisite, first-invalid ordering, exact focus, and absence of execution commands.

### Acceptance criteria

- [ ] Given invalid builder data, when Enter is pressed in a runnable UI context, then no request starts.
- [ ] Given multiple invalid fields, then the first in visual order receives focus and a command-specific reason appears.
- [ ] Given valid data but an invalid UI context, then that context consumes Enter rather than running the query.
- [ ] Given an UPDATE with duplicate SET columns or incomplete Value/NULL choices, then the first invalid SET field is focused with a specific reason.
- [ ] Given a DELETE with an incomplete WHERE, then the first invalid WHERE component is focused.
- [ ] Given an INSERT with incomplete per-column choices or zero insertable columns, then the specific blocking reason is focused.

### User stories addressed

- User story 32: Focus and explain the first invalid prerequisite

---
