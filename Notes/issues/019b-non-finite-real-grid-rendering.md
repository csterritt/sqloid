## Issue 19b: Non-finite REAL grid rendering

**Type**: AFK
**Blocked by**: Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Define and implement grid rendering for non-finite REAL values (Inf, -Inf, NaN) that SQLite may return from pre-existing data. Issue 19 (merged with Issue 41's finite REAL token logic) covers finite REAL formatting only. The PRD states "Existing non-finite policy remains separate" and Issues 44/45 cover non-finite REAL in CSV (`Inf`/`-Inf`/`NaN` textual form) and JSON (quoted `"Inf"`/`"-Inf"`/`"NaN"`). The grid display must be consistent and unambiguous.

Render non-finite REALs as their textual token (`Inf`, `-Inf`, `NaN`) in the grid, matching the CSV textual form. This is a display-only concern; the underlying value type is not changed.

### How to verify

- **Manual**: Open a fixture containing non-finite REAL values and inspect grid display.
- **Automated**: Rendering tests assert that Inf, -Inf, and NaN each produce the exact textual token in the grid, distinct from any finite REAL or TEXT value.

### Acceptance criteria

- [ ] Given a REAL value of Inf, -Inf, or NaN returned by SQLite, then the grid displays the exact textual token `Inf`, `-Inf`, or `NaN` respectively.
- [ ] Given a non-finite REAL alongside finite REALs and TEXT, then each is rendered by its own policy without ambiguity.

### User stories addressed

- User story 74: Represent special SQLite values by documented format policies (non-finite REAL grid display)

---
