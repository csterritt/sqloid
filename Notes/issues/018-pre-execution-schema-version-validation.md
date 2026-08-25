## Issue 18: Pre-execution schema-version validation

**Type**: AFK
**Blocked by**: Issue 11, Issue 17, Issue 5b

### Parent PRD

`PRD-sqloid.md`

### What to build

Insert the cancellable, no-history validation workflow between runnable Enter and execution as defined in **Execution and Result Lifecycle**. This issue owns applying the Issue 5b infrastructure to schema-validation Ctrl+W, visible `cancelling…` settlement, and late-success rejection. Reuse cached schema when unchanged; otherwise refresh and repair only dependent builder state.

### How to verify

- **Manual**: Build a runnable query, change or drop relevant schema externally, press Enter, and exercise retry/cancel.
- **Automated**: Fake and SQLite tests cover unchanged/changed versions, eligibility and identifier invalidation, stale refresh, Ctrl+W, health precedence, and post-validation DDL races.

### Acceptance criteria

- [ ] Given unchanged `schema_version`, then cached metadata is used and execution may proceed without refresh.
- [ ] Given relevant schema changes, then only dependent state is cleared and the first specific invalid reason is focused.
- [ ] Given failed or cancelled validation, then execution and both history appends are blocked with retry/cancel as applicable.
- [ ] Given Ctrl+W during validation, then `cancelling…` remains visible until settlement and any late success is discarded as cancelled.

### User stories addressed

- User story 81: Validate schema cancellably before every actual execution

---
