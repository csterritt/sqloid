## Issue 2: SQLite startup validation and read-write errors

**Type**: AFK
**Blocked by**: Issue 1

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement explicit-file validation and read-write opening in the Connection path, following **Startup validation and errors** in order and without creating missing targets or silently falling back to read-only access.

### How to verify

- **Manual**: Open valid, missing, unreadable, directory, invalid-header, and read-only fixtures through `sqloid sqlite`.
- **Automated**: CLI and SQLite integration tests assert the validation order, exact one-line diagnostics, exit status 1, `mode=rw`, successful probe, and no file creation.

### Acceptance criteria

- [ ] Given a valid SQLite file, when opened, then it is probed and opened read-write without changing its journal mode.
- [ ] Given a missing, unreadable, directory, non-SQLite, read-only, or failed-probe input, then startup exits 1 without creating or modifying the target.
- [ ] Given EACCES/EPERM, EROFS, or another driver cause, then the documented distinct read-write error text is emitted on one stderr line.

### User stories addressed

- User story 3: Reject invalid explicit-file inputs safely
- User story 8: Open read-write with no silent fallback
- User story 90: Classify mode=rw startup failures

---
