## Issue 4: Exact D1 discovery diagnostics

**Type**: AFK
**Blocked by**: Issue 3

### Parent PRD

`PRD-sqloid.md`

### What to build

Complete D1 discovery failure handling with the exact zero-candidate and multiple-candidate diagnostics specified in **D1 discovery**, including the explicit-open recovery hint only where required.

### How to verify

- **Manual**: Run `sqloid d1` against missing, unreadable, empty, and multi-candidate directories.
- **Automated**: Golden CLI tests assert exact line count, spelling, path hint presence or absence, stderr destination, and exit status 1.

### Acceptance criteria

- [ ] Given no candidate, when discovery fails, then the exact two-line expected-path and explicit-open diagnostic is emitted.
- [ ] Given multiple candidates, then only `There is more than one SQLite database in .wrangler` is emitted, without a layout hint.
- [ ] Given either failure, then startup exits 1 and does not create a database.

### User stories addressed

- User story 6: Report zero and multiple D1 candidates precisely
- User story 87: Include the expected path and explicit-open hint only for no candidate

---
