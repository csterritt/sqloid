## Issue 18: Pre-execution schema-version validation

**Type**: AFK
**Blocked by**: Issue 11, Issue 17

### Parent PRD

`PRD-sqloid.md`

### What to build

Insert the cancellable, no-history validation workflow between runnable Enter and execution as defined in **Execution and Result Lifecycle**. Reuse cached schema when unchanged; otherwise refresh and repair only dependent builder state.

### How to verify

- **Manual**: Build a runnable query, change or drop relevant schema externally, press Enter, and exercise retry/cancel.
- **Automated**: Fake and SQLite tests cover unchanged/changed versions, eligibility and identifier invalidation, stale refresh, Ctrl+W, health precedence, and post-validation DDL races.

### Acceptance criteria

- [ ] Given unchanged `schema_version`, then cached metadata is used and execution may proceed without refresh.
- [ ] Given relevant schema changes, then only dependent state is cleared and the first specific invalid reason is focused.
- [ ] Given failed or cancelled validation, then execution and both history appends are blocked with retry/cancel as applicable.

### User stories addressed

- User story 81: Validate schema cancellably before every actual execution

---
