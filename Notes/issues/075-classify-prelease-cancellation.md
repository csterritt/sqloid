## Issue 75: Classify cancellation before lease acquisition correctly

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

At every database entry point that can fail while acquiring a dedicated lease—general requests, started SELECT requests, and writes—recognize an error matching `context.Canceled` before health and general-failure handling. Return the existing cancelled outcome (`OutcomeCancelled` or `WriteCancelled`) so cancellation has one classification whether it occurs before or after lease acquisition, without masking path-health failures or other lease errors.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Saturate the two-connection pool, start a page/count or write waiting for a lease, cancel it, and confirm the UI settles as cancelled rather than displaying an execution failure; then verify a subsequent request still succeeds.
- **Automated**: Deterministic connection tests hold all leases with synchronization barriers, cancel queued `RunRequest`, `startRequest`, and `StartWrite` calls, and assert their cancelled outcomes; wrapped `context.Canceled`, health errors, and ordinary lease failures verify precedence and unchanged non-cancellation classification.

### Acceptance criteria

- [ ] Given a general or started SELECT request is cancelled while waiting for a lease, then it returns `OutcomeCancelled` rather than `OutcomeFailed`.
- [ ] Given a write is cancelled while waiting for a lease, then it returns `WriteCancelled` and starts no transaction work.
- [ ] Given lease acquisition fails for a health or ordinary non-cancellation error, then existing health precedence and failure classification remain unchanged.
- [ ] Given pre-lease cancellation settles, then no replacement work starts before settlement and later requests remain usable.

### User stories addressed

- User story 12: Cancel only active cancellable database work
- User story 14: Prevent requests from stacking while work is pending
- User story 82: Surface cancellation consistently and preserve subsequent requests

---
