## Issue 65: Gate stale SELECT projections through the runnable report

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Extend the authoritative SELECT runnable report in `internal/querybuilder/runnable.go` to validate every committed named `ProjectionColumn`/`Aggregate` entry against the selected object's refreshed visible columns. A missing projected identifier must return an `InvalidIssue{Field: RunFieldProjection}` with specific stale-column feedback so invalid Enter focuses projection. Gate `SelectSQL`, `PageSQL`, and therefore `CountSQL` through that authoritative report and the SELECT command, ensuring `renderSelectCore` cannot emit SQL such as `SELECT "vanished_col" …` after schema refresh invalidates the projection. Preserve valid wildcard and `COUNT(*)` sentinel behavior.

### How to verify

- **Manual**: Build a SELECT containing a named projection, remove that column externally, trigger schema refresh, and press Enter; confirm execution does not start, focus moves to Column(s), and a specific stale-projection reason appears. Confirm wildcard and `COUNT(*)` still run when valid.
- **Automated**: QueryBuilder tests remove visible columns after committing value and aggregate projections and assert `Runnable=false`, `RunFieldProjection`, specific feedback, and empty `SelectSQL`/`PageSQL`/`CountSQL`; renderer tests also reject every other invalid SELECT validity class while valid named, wildcard, and `COUNT(*)` projections retain exact SQL.

### Acceptance criteria

- [ ] Given a committed named projection column no longer exists in the refreshed visible schema, then the authoritative report returns `Runnable=false` with `InvalidIssue.Field == RunFieldProjection` and specific stale-column feedback.
- [ ] Given a stale value or aggregate projection, then Enter starts no request and focuses the projection field for repair.
- [ ] Given the authoritative SELECT report rejects stale identifiers, grouping, incomplete values, or limit state, then `SelectSQL`, `PageSQL`, and `CountSQL` emit no executable SQL.
- [ ] Given valid wildcard, `COUNT(*)`, named value, or named aggregate projections, then validation and exact SQL rendering remain unchanged.

### User stories addressed

- User story 26: Build valid ordered SELECT projections
- User story 32: Focus and explain the first invalid prerequisite
- User story 81: Revalidate identifiers after schema-version refresh

---
