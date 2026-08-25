## Issue 38: Transactional write execution and summaries

**Type**: AFK
**Blocked by**: Issue 24, Issue 35, Issue 37

### Parent PRD

`PRD-sqloid.md`

### What to build

Execute confirmed UPDATE/DELETE and runnable INSERT as sole actual writes on a leased connection. This issue owns applying the Issue 5b infrastructure and scoped Ctrl+W to cancellable beginning/executing phases. Implement the atomic pre-COMMIT cancellation check, confirmed rollback cleanup, commit, and exactly one non-tabular summary/history result.

### How to verify

- **Manual**: Run INSERT and confirmed qualified/unqualified UPDATE/DELETE; cancel or fail writes before commit and inspect persisted rows and summaries.
- **Automated**: SQLite/fake tests cover constraints/triggers, cancellation flag winning after statement success, rollback confirmation, query append timing, single result entry, and `RowsAffected()` labels.

### Acceptance criteria

- [ ] Given confirmation or runnable INSERT, then exactly one actual execution begins and query history appends at that point only.
- [ ] Given cancellation or statement failure before commit, then rollback cleanup completes before any untouched guarantee is shown.
- [ ] Given success, then exactly one result shows executed SQL and actual UPDATE/DELETE rows affected or INSERT rows added.

### User stories addressed

- User story 44: Produce exactly one result per actual write
- User story 45: Make writes transactional with confirmed rollback guarantees
- User story 66: Show operation-appropriate write summaries and executed SQL

---
