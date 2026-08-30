## Issue 59: Percent-encode relative SQLite DSN paths

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Construct relative SQLite file URLs with an escaped URL path and no invented authority so reserved filename characters remain part of the filename instead of becoming URI query or fragment delimiters. Preserve correct handling for absolute paths and ordinary relative paths, including spaces, while retaining read-write startup options and validation behavior.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Create valid relative SQLite files whose names contain `?`, `#`, and spaces, open each through `sqloid sqlite`, and confirm Sqloid opens the intended file without creating or selecting a differently parsed path.
- **Automated**: Connection tests assert exact relative file-URL construction and successful SQLite opening for `?`, `#`, spaces, and combined reserved characters, with no `//` authority; regression cases cover ordinary relative and absolute paths plus unchanged mode/read-write behavior.

### Acceptance criteria

- [ ] Given a relative database filename contains `?` or `#`, when its SQLite DSN is built, then those characters are percent-encoded as path data and are not parsed as query or fragment separators.
- [ ] Given a relative filename contains spaces, then it is encoded and opens the intended existing database.
- [ ] Given ordinary relative or absolute paths, then URL shape, no-create read-write startup, and target selection remain correct.

### User stories addressed

- User story 1: Open the explicitly named SQLite file
- User story 7: Keep successful startup silent and failures path-specific
- User story 8: Open the selected database read-write without fallback

---
