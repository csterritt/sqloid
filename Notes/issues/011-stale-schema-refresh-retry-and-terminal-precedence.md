## Issue 11: Stale schema refresh, retry, and terminal precedence

**Type**: AFK
**Blocked by**: Issue 8, Issue 10

### Parent PRD

`PRD-sqloid.md`

### What to build

Refresh the table catalog whenever its popup opens and handle refresh failures as specified in **Schema scope, cache, and validation**: retain visibly stale data, block unsafe continuation, and offer retry/cancel while health failures take terminal precedence.

### How to verify

- **Manual**: Open Table, change or lock the schema, reopen it, and exercise retry/cancel and deletion paths.
- **Automated**: Fake-Connection model tests assert refresh timing, stale notices, unchanged lists, retry/cancel state, and deletion/replacement override.

### Acceptance criteria

- [ ] Given Table opens, then its catalog is refreshed before presenting current candidates.
- [ ] Given refresh fails, then the prior list remains visibly stale and retry/cancel is available rather than silently using it.
- [ ] Given deletion or replacement during refresh, then the appropriate terminal state overrides the stale workflow.

### User stories addressed

- User story 22: Retain stale schema visibly with retry/cancel and terminal precedence

---
