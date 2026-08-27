# Local D1 Discovery

How `sqloid d1` finds the local Wrangler D1 database (Issue #3 and the D1 discovery section of `Notes/PRD-sqloid.md`). Opening, validation, and failure diagnostics for the discovered path are governed by [sqlite-startup.md](sqlite-startup.md).

## The exact directory

Discovery inspects exactly one directory, relative to the process working directory:

```
.wrangler/state/v3/d1/miniflare-D1DatabaseObject
```

There is deliberately **no** recursive search into subdirectories and **no** probing of alternate Wrangler layouts: an absent or empty directory simply yields zero candidates. Every other location under `.wrangler` is ignored.

## Candidate rules

An immediate entry of that directory is a candidate when **all** rules hold:

| Rule | Effect |
| --- | --- |
| Case-sensitive `.sqlite` extension | `ABC.SQLITE` and `db.SQLite` are not candidates; only an exact lowercase `.sqlite` suffix qualifies. |
| No lowercase `metadata` substring | `state-metadata.sqlite` is excluded. The rule is case-sensitive by design: `B-Metadata.sqlite` with uppercase `M` remains a candidate. |
| Not a `-wal` / `-shm` sidecar | `abc123.sqlite-wal` and `abc123.sqlite-shm` are never candidates even though they contain `.sqlite`. |

## Cardinality outcomes

| Candidates found | Behavior |
| --- | --- |
| Exactly one | Discovery returns its joined path (`<dir>/<name>`) unchanged and startup proceeds through the shared opener below. Successful open is silent (exit 0). |
| Zero (directory absent, empty, unreadable, or all entries excluded) | Typed `ErrNoCandidate` outcome surfaced to `internal/cli`; exit 1 with exactly **two** stderr lines: `no candidate database found in .wrangler`, then `Expected .wrangler/state/v3/d1/miniflare-D1DatabaseObject; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.` |
| More than one | Typed `ErrMultipleCandidates` outcome surfaced to `internal/cli`; exit 1 with only the exact single stderr line `There is more than one SQLite database in .wrangler` — deliberately no expected-path or explicit-open layout hint. |

## Discovery-failure diagnostics (Issue #4)

`internal/cli` owns the mapping from typed outcomes to process-facing diagnostics; `internal/d1` carries no message text beyond its sentinel errors, and neither maps to any open attempt:

- **Zero candidates** — covering a missing, unreadable, empty, or candidate-free directory — produce exactly two stderr lines: the typed message plus the hint above naming the working-directory-relative expected path and explicit-open recovery (`sqloid sqlite <file>`). Note that `d1.Discover` cannot distinguish missing from unreadable from candidate-free: every such case collapses onto the same zero-candidate diagnostic by design.
- **Multiple candidates** produce exactly one stderr line with no hint.
- Every discovery failure exits status 1, writes nothing to stdout, bypasses `internal/connection` entirely, and creates no database target anywhere in the working directory (golden tests snapshot the whole working tree before/after).
- The single-line `internal/connection` rendering convention does not apply here: discovery failures are the PRD's explicitly defined multi-line exception, printed verbatim by `Main` as one newline-bearing handler error.

## Handoff chain

1. `internal/d1.Discover()` performs the filesystem scan above. It never opens SQLite and owns nothing beyond path strings; it returns typed sentinel outcomes (`ErrNoCandidate`, `ErrMultipleCandidates`) so `internal/cli` can render diagnostics without this package duplicating them.
2. `internal/cli.RunD1` receives the sole candidate path and passes it **unchanged** to `internal/connection.Session` — the same pre-open validation, read-write mode=rw open, and schema-probe flow as `sqloid sqlite <file>`. There is no D1-specific validation or SQLite-opening path anywhere.
3. `cmd/sqloid/main.go` wires `Handlers{SQLite: connection.Session, D1: cli.RunD1}` and stays a thin executable entrypoint.

## Relative-path URI detail

Because discovery returns working-directory-relative paths, `internal/connection.mustFileURL` renders relative paths as opaque `file:<path>` URIs. Without this, `net/url` invents a `//` authority from the leading path segment (`file://.wrangler/...`) and the SQLite URI parser rejects it with `invalid uri authority: .wrangler`. Absolute paths keep the ordinary `file:/abs/path?mode=rw` form.

Cross-references: [cli-contract.md](cli-contract.md), [sqlite-startup.md](sqlite-startup.md), [source-code.md](source-code.md).
