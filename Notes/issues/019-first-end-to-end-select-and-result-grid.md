## Issue 19: First end-to-end SELECT and result grid

**Type**: AFK
**Blocked by**: Issue 18

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver the first complete runnable tracer bullet: build a safe SELECT, validate schema, execute its first page, and render a bordered grid with deduplicated frozen headers, absolute range/status, and explicit empty-result treatment.

### How to verify

- **Manual**: Open a fixture, run `SELECT *` and a duplicate-label projection, then run a SELECT returning no rows.
- **Automated**: End-to-end fake/SQLite tests assert generated SQL and params, result values, header deduplication, status range, and `No rows` rendering.

### Acceptance criteria

- [ ] Given a runnable SELECT, when Enter validation succeeds, then its first rows render in the result grid.
- [ ] Given duplicate output labels, then deterministic deduplicated names appear in the frozen header.
- [ ] Given zero rows, then the result view says `No rows` rather than appearing blank.

### User stories addressed

- User story 51: Show a frozen deduplicated header, range/count status, and explicit empty result

---
