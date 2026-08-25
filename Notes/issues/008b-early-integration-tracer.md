## Issue 8b: Early integration tracer (hardcoded SELECT *)

**Type**: AFK
**Blocked by**: Issue 5, Issue 8

### Parent PRD

`PRD-sqloid.md`

### What to build

A thin end-to-end integration milestone to de-risk the Bubble Tea ↔ Connection ↔ Schema stack before the full builder is built on top. Hardcode a `SELECT * FROM <table>` for a single chosen table (no command/table popups, no validation, no WHERE/GROUP/ORDER/LIMIT, no count request) and render the results in a minimal bordered grid. The goal is to prove the module boundaries and integration points, not to deliver user-facing functionality.

This tracer is disposable: once Issue 19 (first end-to-end SELECT with the real builder) lands, this code path is replaced. It does not need to handle paging, cancellation, history, or error recovery beyond a basic error display.

### How to verify

- **Manual**: Open a fixture database and observe that hardcoded `SELECT *` results render in a bordered grid.
- **Automated**: Integration test asserts that the Connection module executes a simple SELECT and returns rows; the UI model renders those rows in a bordered area with column headers. No builder, popup, or validation logic is exercised.

### Acceptance criteria

- [ ] Given an opened database and a chosen table, then a hardcoded `SELECT *` executes and its rows render in a minimal bordered grid.
- [ ] Given the module boundaries, then Connection, Schema, and UI compose without embedding database behavior in the UI model.
- [ ] Given a basic query error, then it is displayed without crashing the application.

### User stories addressed

- (Risk-reduction milestone; no direct user story. Supports user story 51 by proving the rendering path early.)

---
