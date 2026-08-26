## Issue 31: 64 MiB cache and oversized-value handling

**Type**: AFK
**Blocked by**: Issue 5, Issue 30

### Parent PRD

`PRD-sqloid.md`

### What to build

Add exact retained-payload accounting and the independent 64 MiB cache cap, including deterministic eviction, complete-leading-row retention, persistent truncation disclosure, and distinct page/value over-limit failures at one-based logical positions. This issue is the definition site for the shared byte-cap presentation string `Result truncated: 64 MiB cache limit`; result headers and later export flows must reuse that definition rather than duplicate the literal.

### How to verify

- **Manual**: Browse fixtures that cross the byte cap and encounter a page or value exceeding the v1 envelope.
- **Automated**: Cache/SQLite tests assert TEXT/BLOB byte counts, numeric/null costs, opposite-end eviction, exact messages and row N, no partial rows, and exact BLOB retention.

### Acceptance criteria

- [ ] Given byte-cap eviction, then standard opposite-end eviction occurs, metadata records `truncated-by-byte-cap`, and the result header shows exactly `Result truncated: 64 MiB cache limit` persistently.
- [ ] Given the retained rows from one fetched page exceed 64 MiB, then only complete leading rows are retained up to the first nonfitting one-based logical position N, no partial row is retained, and the page shows exactly `result page exceeds the 64 MiB v1 limit at row N`.
- [ ] Given one row or value exceeds the connection-local 64 MiB value limit at one-based logical position N, then no partial row is retained and the page shows exactly `result value exceeds the 64 MiB v1 limit at row N`.

### User stories addressed

- User story 54: Enforce the independent byte cap while paging
- User story 89: Identify oversized page/value failures without partial rows

---
