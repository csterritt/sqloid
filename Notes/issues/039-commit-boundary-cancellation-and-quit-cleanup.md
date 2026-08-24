## Issue 39: Commit-boundary cancellation and quit cleanup

**Type**: AFK
**Blocked by**: Issue 38

### Parent PRD

`PRD-sqloid.md`

### What to build

Enforce the write commit boundary and accepted-quit lifecycle. Beginning/executing remain cancellable; rollback cleanup and committing do not. Quit waits for resolution and never abandons transaction or driver work.

### How to verify

- **Manual**: Press Ctrl+W and confirm quit before and after the commit boundary and observe phase feedback and final database state.
- **Automated**: Barrier-based fake/SQLite tests assert interrupt issuance before but never after boundary, exact feedback, atomic cancellation checks, and quit settlement before exit.

### Acceptance criteria

- [ ] Given beginning/executing, then Ctrl+W requests cancellation; given rollback/commit, then it is ignored with boundary-specific feedback.
- [ ] Given accepted quit during cancellable write work, then cancellation and rollback resolution finish before exit.
- [ ] Given accepted quit while committing, then commit resolution finishes before exit and no cleanup is abandoned.

### User stories addressed

- User story 46: Respect the commit boundary and wait for cleanup on quit

---
