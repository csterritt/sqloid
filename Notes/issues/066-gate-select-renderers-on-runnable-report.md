## Issue 66: Gate SELECT renderers on the authoritative runnable report

**Type**: AFK
**Blocked by**: Issue 65

### Parent PRD

`PRD-sqloid.md`

### What to build

Make `SelectSQL`, `PageSQL`, and therefore `CountSQL` refuse to emit SQL or parameters unless the builder's authoritative runnable report accepts the selected SELECT command. This issue owns all SELECT-family renderer gating after Issue 65 adds stale-projection validation to that report. Preserve valid SELECT, paging, count, LIMIT/OFFSET, and parameter-order behavior while closing every rejected class: invalid grouping, stale identifiers including projections, incomplete value state, invalid Limit state, missing prerequisites, and non-SELECT commands.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Exercise valid and invalid SELECT builder states, including schema-stale columns, incomplete WHERE input, bad grouping, and bad Limit; confirm only the valid state can produce executable SELECT/page/count SQL.
- **Automated**: QueryBuilder table tests enumerate every SELECT validity class rejected by `RunnableReport` and assert empty SQL and no parameters from all three renderers; valid cases assert unchanged SQL, paging/count wrappers, and binding order.

### Acceptance criteria

- [ ] Given any SELECT state rejected by the authoritative runnable report, when SELECT, page, or count rendering is requested, then no SQL or parameters are emitted.
- [ ] Given a builder whose selected command is not SELECT, then SELECT-family renderers return empty output even if component fields are otherwise formattable.
- [ ] Given a runnable SELECT, then SELECT, page, and count SQL remain correctly quoted and parameterized with existing LIMIT/OFFSET and count semantics.

### User stories addressed

- User story 32: Reject invalid query state with an authoritative reason
- User story 76: Bind values and quote only schema-selected identifiers
- User story 81: Block schema-invalidated state before execution

---
