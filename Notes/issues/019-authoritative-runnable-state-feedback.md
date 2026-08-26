## Issue 19: Authoritative runnable-state feedback

**Type**: AFK
**Blocked by**: Issue 17, Issue 18

### Parent PRD

`PRD-sqloid.md`

### What to build

Expose a UI-independent QueryBuilder runnable report and connect it to Enter. Apply the command prerequisites and common gates in **Runnable-State Contract**, focusing the first invalid field in visual order with a specific reason. Complete the general builder-editing contract by making Backspace/Delete clear a focused completed whole-value field, including WHERE values and Limit, while empty fields remain unchanged; UPDATE/INSERT Value fields use the same behavior when their end-to-end flows land. This issue delivers the **general** runnable framework for all four commands (SELECT, UPDATE, DELETE, INSERT), not SELECT alone. Write-specific prerequisites (SET column uniqueness and Value/NULL completion for UPDATE, optional WHERE completion for DELETE, per-column Value/NULL/Default completion and zero-insertable-column blocking for INSERT) are part of this framework even though their end-to-end exercise occurs in Issues 37, 38, and 39.

### How to verify

- **Manual**: Clear completed WHERE and Limit values with Backspace/Delete, including empty no-ops, then attempt Enter from representative incomplete and invalid SELECT states and correct the focused field.
- **Automated**: Table-driven runnable and scripted model tests assert whole-value clearing, empty no-ops, every prerequisite, first-invalid ordering, exact focus, and absence of execution commands; Issues 37 and 39 exercise the same clearing behavior for UPDATE/INSERT Value fields.

### Acceptance criteria

- [ ] Given a focused completed whole-value builder field such as WHERE value or Limit, when Backspace/Delete is pressed, then the entire value is cleared; an already empty field is unchanged and no execution starts.
- [ ] Given UPDATE/INSERT Value fields when those flows are integrated, then they follow the same whole-value clearing and empty no-op behavior.
- [ ] Given a nonempty invalid Limit, when Enter is pressed in a runnable UI context, then Limit is focused with exactly `Limit must be an integer from 1 to 9223372036854775807` and no request starts.
- [ ] Given invalid builder data, when Enter is pressed in a runnable UI context, then no request starts.
- [ ] Given multiple invalid fields, then the first in visual order receives focus and a command-specific reason appears.
- [ ] Given valid data but an invalid UI context, then that context consumes Enter rather than running the query.
- [ ] Given an UPDATE with duplicate SET columns or incomplete Value/NULL choices, then the first invalid SET field is focused with a specific reason.
- [ ] Given a DELETE with an incomplete WHERE, then the first invalid WHERE component is focused.
- [ ] Given an INSERT with incomplete per-column choices or zero insertable columns, then the specific blocking reason is focused.

### User stories addressed

- User story 27 (whole-value-field portion): Clear an appropriate focused whole field safely
- User story 32: Focus and explain the first invalid prerequisite

---
