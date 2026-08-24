## Issue 28: 64 MiB cache and oversized-value handling

**Type**: AFK
**Blocked by**: Issue 5, Issue 27

### Parent PRD

`PRD-sqloid.md`

### What to build

Add exact retained-payload accounting and the independent 64 MiB cache cap, including deterministic eviction, complete-leading-row retention, persistent truncation disclosure, and distinct page/value over-limit failures at one-based logical positions.

### How to verify

- **Manual**: Browse fixtures that cross the byte cap and encounter a page or value exceeding the v1 envelope.
- **Automated**: Cache/SQLite tests assert TEXT/BLOB byte counts, numeric/null costs, opposite-end eviction, exact messages and row N, no partial rows, and exact BLOB retention.

### Acceptance criteria

- [ ] Given retained payload reaches 64 MiB, then standard opposite-end eviction occurs and the exact persistent truncation warning is recorded.
- [ ] Given a fetched page cannot fit, then only complete leading rows are retained up to the first nonfitting logical position.
- [ ] Given a page or connection-local value limit failure, then the correct distinct message identifies one-based row N and no partial row is retained.

### User stories addressed

- User story 54: Enforce the independent byte cap while paging
- User story 89: Identify oversized page/value failures without partial rows

---
