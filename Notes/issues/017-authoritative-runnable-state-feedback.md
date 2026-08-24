## Issue 17: Authoritative runnable-state feedback

**Type**: AFK
**Blocked by**: Issue 15, Issue 16

### Parent PRD

`PRD-sqloid.md`

### What to build

Expose a UI-independent QueryBuilder runnable report and connect it to Enter. Apply the command prerequisites and common gates in **Runnable-State Contract**, focusing the first invalid field in visual order with a specific reason.

### How to verify

- **Manual**: Attempt Enter from representative incomplete and invalid SELECT states, then correct the focused field.
- **Automated**: Table-driven runnable tests and scripted model tests assert every prerequisite, first-invalid ordering, exact focus, and absence of execution commands.

### Acceptance criteria

- [ ] Given invalid builder data, when Enter is pressed in a runnable UI context, then no request starts.
- [ ] Given multiple invalid fields, then the first in visual order receives focus and a command-specific reason appears.
- [ ] Given valid data but an invalid UI context, then that context consumes Enter rather than running the query.

### User stories addressed

- User story 32: Focus and explain the first invalid prerequisite

---
