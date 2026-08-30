## Issue 65: Gate stale SELECT projections through the runnable report

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Extend the authoritative SELECT runnable report in `internal/querybuilder/runnable.go` to validate every committed named `ProjectionColumn`/`Aggregate` entry against the selected object's refreshed visible columns. Add projection validation to `reportSelect` so a missing projected identifier returns an `InvalidIssue{Field: RunFieldProjection}` with specific stale-column feedback, makes the report non-runnable, and causes invalid Enter to focus projection. Preserve valid wildcard and `COUNT(*)` sentinel behavior.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Build a SELECT containing a named projection, remove that column externally, trigger schema refresh, and press Enter; confirm execution does not start, focus moves to Column(s), and a specific stale-projection reason appears. Confirm wildcard and `COUNT(*)` still run when valid.
- **Automated**: QueryBuilder report tests remove visible columns after committing value and aggregate projections and assert `Runnable=false`, `RunFieldProjection`, and specific feedback; command tests assert Enter starts no request and focuses projection. Valid named, wildcard, and `COUNT(*)` report behavior remains unchanged.

### Acceptance criteria

- [ ] Given a committed named projection column no longer exists in the refreshed visible schema, then the authoritative report returns `Runnable=false` with `InvalidIssue.Field == RunFieldProjection` and specific stale-column feedback.
- [ ] Given a stale value or aggregate projection, then Enter starts no request and focuses the projection field for repair.
- [ ] Given valid wildcard, `COUNT(*)`, named value, or named aggregate projections, then runnable-report validation remains unchanged.

### User stories addressed

- User story 26: Build valid ordered SELECT projections
- User story 32: Focus and explain the first invalid prerequisite
- User story 81: Revalidate identifiers after schema-version refresh

---
