## Issue 1: CLI shell and usage contracts

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Create the Go application entry point and command surface described in **Implementation Decisions / Language and stack** and **CLI behavior**. Support `sqlite <file>`, `d1`, help, version, and strict unexpected-argument handling while keeping successful startup silent.

### How to verify

- **Manual**: Run help, version, a missing `sqlite` argument, and an unexpected argument; inspect output streams and exit statuses.
- **Automated**: CLI tests assert exact routing, stderr/stdout use, exit status 2 for usage errors, and no success output.

### Acceptance criteria

- [ ] Given `sqloid sqlite` without a file, when invoked, then usage is written to stderr and the process exits 2.
- [ ] Given help or version flags, when invoked, then the documented information is printed successfully.
- [ ] Given a valid command that reaches startup, then the CLI adds no success message of its own.

### User stories addressed

- User story 1: Open an explicit SQLite file
- User story 2: Report a missing SQLite argument
- User story 7: Keep success silent and startup diagnostics precise

---
