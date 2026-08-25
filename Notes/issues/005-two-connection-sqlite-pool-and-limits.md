## Issue 5: Two-connection SQLite pool and limits

**Type**: AFK
**Blocked by**: Issue 2

### Parent PRD

`PRD-sqloid.md`

### What to build

Establish the Connection foundation from **Connection pool, limits, and busy handling**: an exact-two pool, dedicated leases, five-second busy handling, and a 64 MiB SQLite length limit on every connection, without journal-mode mutation.

### How to verify

- **Manual**: Open an explicit and a discovered database and execute harmless requests in WAL and rollback-journal modes.
- **Automated**: SQLite integration tests inspect pool limits, distinct leases, per-connection configuration, unchanged journal mode, and length-limit installation.

### Acceptance criteria

- [ ] Given an opened target, then the pool maintains at least and at most two appropriate connections.
- [ ] Given any leased connection, then its busy timeout is five seconds and its SQLite length limit is exactly 64 MiB.
- [ ] Given WAL or rollback-journal input, then opening the pool does not change journal mode.

### User stories addressed

- User story 8: Use a writable SQLite connection
- User story 89: Limit every connection to 64 MiB SQLite values

---
