# Sqloid Project Overview

Sqloid is a Go terminal application for browsing and editing SQLite databases, including Cloudflare D1 local databases. It opens either an explicit SQLite file (`sqloid sqlite <file>`) or discovers the single D1 candidate under `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` (`sqloid d1`).

## Language and stack

- Go with the pure-Go/no-cgo `modernc.org/sqlite` driver (pinned exact version).
- Bubble Tea for the TUI, Lip Gloss for styling.
- `mow.cli` (PRD-mandated) for command-word/argument parsing of `sqlite <file>` and `d1`.

## Module design

- `internal/cli` — command-line shell: command routing, argument parsing, help, version, stream and exit-status contracts. Database startup stays behind injectable handlers.
- `cmd/sqloid` — thin process boundary; maps the CLI's returned status onto the process exit status.
- `internal/connection`, `internal/d1` — planned: connection pool/discovery (Issues #2–#5).

## Key contracts

- Successful startup is silent: the CLI adds no output of its own.
- Usage failures (missing or unexpected arguments, unknown commands) print an error plus usage to stderr and exit 2.
- Version output is exactly `sqloid <version>` followed by a newline, on stdout.
- Startup validation and database errors are defined in `Notes/PRD-sqloid.md` and Issues #1–#8.

See [cli-contract.md](cli-contract.md) for the full command-line contract.
