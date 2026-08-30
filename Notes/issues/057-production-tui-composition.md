## Issue 57: Production TUI composition and binary smoke path

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Add the production application-composition path missing between `cmd/sqloid/main.go`, `internal/connection/startup.go`, and `internal/ui`: retain ownership of the opened `connection.DB`, load the initial schema catalog, construct `ui.Model`, wire its database, paging/count, write, export, and file-picker executors to real connection/filesystem implementations, and run the Bubble Tea program for both `sqloid sqlite <file>` and `sqloid d1`. Map terminal completion and failure to the documented process status, complete UI/request cleanup before shutdown, and close the database only after the program has settled. Add a production-level binary test that proves a valid database enters the TUI and can traverse a baseline SELECT, write, and export path rather than silently returning after validation.

### How to verify

- **Manual**: Launch the built `sqloid` binary against a valid explicit SQLite file and through D1 discovery; confirm the full-screen builder appears, execute a SELECT and a confirmed write, save/export a result, then quit and verify cleanup and the documented exit status.
- **Automated**: A production-composition/binary integration test drives a real temporary SQLite database through startup into Bubble Tea, exercises baseline SELECT, write, and export behavior through real adapters, asserts the process does not exit immediately after validation, and verifies terminal outcomes, resource cleanup, and database close ordering.

### Acceptance criteria

- [ ] Given a valid database opened by either startup mode, when startup validation succeeds, then the shipped binary loads the catalog, constructs the production UI with every required executor, and runs the Bubble Tea program instead of returning silently.
- [ ] Given baseline SELECT, write, and export interactions in the production composition, then they reach the real connection/filesystem adapters and produce the documented visible and persisted results.
- [ ] Given accepted quit or a terminal application outcome, then pending UI/request cleanup settles before the database closes and the process returns the documented status.
- [ ] Given the production binary integration test, then removing or bypassing the TUI composition causes the test to fail even if package-level fake-seam tests still pass.

### User stories addressed

- User story 1: Open an explicit SQLite database
- User story 4: Discover and open a local Wrangler database
- User story 7: Complete successful startup silently before entering the application
- (Cross-cutting production composition makes the v1 TUI, execution, history, and export stories reachable.)

---
