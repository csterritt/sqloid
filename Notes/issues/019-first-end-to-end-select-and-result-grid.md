## Issue 19: First end-to-end SELECT and result grid

**Type**: AFK
**Blocked by**: Issue 18, Issue 17b

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver the first complete runnable tracer bullet: build a safe SELECT, validate schema, execute its first page, and render a bordered grid with deduplicated frozen headers, absolute range/status, and explicit empty-result treatment. Replace and remove the hardcoded Issue 8b production runtime path so only the builder → validation → execution lifecycle remains; reusable fixtures, helpers, and integration-test infrastructure may be retained. This issue also centralizes the result output-name deduplication and value rendering shared by the grid (and later by exporters per Issue 41): full-set collision-safe deduplication, exact finite REAL tokens (`strconv.FormatFloat(v, 'g', -1, 64)` with `.0` appended when needed), visible grid control characters (tabs/newlines as visible symbols), maximal invalid UTF-8 TEXT replacement (one U+FFFD per maximal invalid sequence with warning metadata), and `[BLOB n bytes]` display. Non-finite REAL grid rendering is handled separately in Issue 19b.

### How to verify

- **Manual**: Open a fixture, run `SELECT *` and a duplicate-label projection, then run a SELECT returning no rows.
- **Automated**: End-to-end fake/SQLite tests assert generated SQL and params, result values, header deduplication, status range, and `No rows` rendering; architecture/build checks confirm no hardcoded Issue 8b production execution path remains.

### Acceptance criteria

- [ ] Given a runnable SELECT, when Enter validation succeeds, then its first rows render in the result grid.
- [ ] Given duplicate output labels, then deterministic full-set collision-safe deduplicated names appear in the frozen header, shared by grid and (later) CSV/JSON without altering driver metadata or SQL.
- [ ] Given zero rows, then the result view says `No rows` rather than appearing blank.
- [ ] Given finite REAL values, then one exact shortest-round-trip REAL-preserving token is used in the grid (e.g. `1.0`, `-0.0`, `1e+20`).
- [ ] Given invalid UTF-8 TEXT, then each maximal invalid sequence becomes one U+FFFD with warning metadata while BLOB bytes remain unchanged and display as `[BLOB n bytes]`.
- [ ] Given tabs or newlines in TEXT values, then they render as visible symbols in the grid.
- [ ] Given Issue 19 is complete, then the hardcoded Issue 8b production runtime path has been removed or fully replaced and only the builder → validation → execution path remains; reusable test helpers may remain.

### User stories addressed

- User story 51: Show a frozen deduplicated header, range/count status, and explicit empty result

---
