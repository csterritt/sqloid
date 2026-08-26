---
name: running-the-cli
description: How to build and run the Sqloid Go command-line and terminal application.
---

## Build and run

Run commands from the repository root.

- Build all packages with `go build ./...`.
- Keep `cmd/sqloid` as a thin process boundary and use the PRD-mandated `mow.cli` command structure for `sqlite <file>`, `d1`, help, and version handling.
- Run the CLI during development with `go run ./cmd/sqloid --help`, `go run ./cmd/sqloid --version`, `go run ./cmd/sqloid sqlite <file>`, or `go run ./cmd/sqloid d1` as appropriate.
- When validating exact streams and exit statuses, build a temporary binary and invoke it from a shell test; `go run` can add toolchain diagnostics that are not authored by Sqloid. Assert status 2 for usage failures and status 1 for startup or database-validation failures.
- Use a disposable SQLite fixture for manual checks. Never point destructive manual checks at a database that must be preserved.
- Run interactive TUI commands in a real PTY. Capture deterministic behavior in model/update/view tests rather than depending only on terminal recordings.
- Successful startup is silent. Preserve the exact help, version, diagnostics, stream ownership, and exit-status contracts in the PRD and issue tasks.
