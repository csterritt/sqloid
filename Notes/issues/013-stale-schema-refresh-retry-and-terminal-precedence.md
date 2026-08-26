## Issue 13: Stale schema refresh, retry, and terminal precedence

**Type**: AFK
**Blocked by**: Issue 9, Issue 12

### Parent PRD

`PRD-sqloid.md`

### What to build

Refresh the table catalog whenever its popup opens and handle refresh failures as specified in **Schema scope, cache, and validation**: retain stale data, block unsafe continuation, and offer retry/cancel while health failures take terminal precedence. While stale data is displayed, show persistent status `Schema data is stale — retry or cancel` together with inline `could not refresh: <cause>` until retry succeeds, the user cancels, or deletion/replacement takes terminal precedence.

### How to verify

- **Manual**: Open Table, change or lock the schema, reopen it, and exercise retry/cancel and deletion paths.
- **Automated**: Fake-Connection model/rendering tests assert refresh timing, exact persistent stale status and inline cause, unchanged lists, retry/cancel lifecycle, successful-retry clearing, and deletion/replacement override.

### Acceptance criteria

- [ ] Given Table opens, then its catalog is refreshed before presenting current candidates.
- [ ] Given refresh fails, then the unchanged prior list remains visible with persistent `Schema data is stale — retry or cancel` and inline `could not refresh: <cause>`; retry/cancel remains available and both indicators persist until retry succeeds, cancel closes the flow, or terminal precedence applies.
- [ ] Given deletion or replacement during refresh, then the appropriate terminal state overrides the stale workflow.

### User stories addressed

- User story 22: Retain stale schema visibly with retry/cancel and terminal precedence

---
