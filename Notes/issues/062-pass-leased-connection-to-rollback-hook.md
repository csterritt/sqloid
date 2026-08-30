## Issue 62: Pass the leased connection to the rollback test hook

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Make the connection-aware rollback barrier seam consistent with the begin, execute, and commit seams in the phased write path. When rollback cleanup begins, invoke `beforeWriteRollback` with the same leased `*sql.Conn` that owns the transaction rather than `nil`, without changing production transaction, cancellation, or rollback-settlement behavior.

### How to verify

- **Manual**: Trace one cancelled or failed write through rollback cleanup and confirm the rollback hook receives the transaction's leased connection while the write still settles under the existing rollback rules.
- **Automated**: Extend the barrier-based connection tests with a rollback hook that asserts its connection is non-nil and identical to the lease observed for the other phases; retain coverage for confirmed rollback, unresolved rollback, cancellation, and statement failure outcomes.

### Acceptance criteria

- [ ] Given a write reaches rollback cleanup, when `beforeWriteRollback` runs, then it receives the non-nil leased connection that owns that write transaction.
- [ ] Given begin, execute, commit, and rollback barrier hooks inspect connection identity, then all hooks for one write observe the same leased connection.
- [ ] Given rollback succeeds or fails, then the existing confirmed-rollback and outcome-unknown classifications remain unchanged.

### User stories addressed

- User story 45: Keep cancelled and failed writes transactional through confirmed rollback cleanup
- User story 82: Settle cancellable database work safely without force-closing its connection

---
