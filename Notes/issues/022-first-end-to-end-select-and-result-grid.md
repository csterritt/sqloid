## Issue 22: First end-to-end SELECT and result grid

**Type**: AFK
**Blocked by**: Issue 20, Issue 21

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver the first complete runnable tracer bullet: build a safe SELECT, validate schema, execute its first page, and render a bordered grid with deduplicated frozen headers, absolute range/status, and explicit empty-result treatment. Replace and remove the hardcoded Issue 10 production runtime path so only the builder → validation → execution lifecycle remains; reusable fixtures, helpers, and integration-test infrastructure may be retained. Implement result output-name deduplication and typed value primitives behind a UI-independent package-shaped seam that the grid consumes immediately and exporters can extend through Issue 47; no grid-private copy is allowed. The seam includes full-set collision-safe deduplication, exact finite REAL tokens (`strconv.FormatFloat(v, 'g', -1, 64)` with `.0` appended when needed), visible grid control characters (tabs/newlines as visible symbols), maximal invalid UTF-8 TEXT replacement (one U+FFFD per maximal invalid sequence with warning metadata), and exact BLOB-byte preservation with `[BLOB n bytes]` grid display. Non-finite REAL grid rendering is handled separately in Issue 23.

### How to verify

- **Manual**: Open a fixture, run `SELECT *` and a duplicate-label projection, then run a SELECT returning no rows.
- **Automated**: End-to-end fake/SQLite tests assert generated SQL and params, result values, header deduplication, status range, and `No rows` rendering; package-level tests exercise the UI-independent seam, and architecture/build checks confirm the grid has no private duplicate and no hardcoded Issue 10 production execution path remains.

### Acceptance criteria

- [ ] Given a runnable SELECT, when Enter validation succeeds, then its first rows render in the result grid.
- [ ] Given duplicate output labels, then deterministic full-set collision-safe deduplicated names appear in the frozen header, shared by grid and (later) CSV/JSON without altering driver metadata or SQL.
- [ ] Given zero rows, then the result view says `No rows` rather than appearing blank.
- [ ] Given finite REAL values, then one exact shortest-round-trip REAL-preserving token is used in the grid (e.g. `1.0`, `-0.0`, `1e+20`).
- [ ] Given invalid UTF-8 TEXT, then each maximal invalid sequence becomes one U+FFFD with warning metadata while BLOB bytes remain unchanged and display as `[BLOB n bytes]`.
- [ ] Given tabs or newlines in TEXT values, then they render as visible symbols in the grid.
- [ ] Given Issue 22 is complete, then the hardcoded Issue 10 production runtime path has been removed or fully replaced and only the builder → validation → execution path remains; reusable test helpers may remain.

### User stories addressed

- User story 51: Show a frozen deduplicated header, range/count status, and explicit empty result

---
