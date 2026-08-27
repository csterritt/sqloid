# Session Health — Request-Boundary Database Identity Checks (Issue #7)

Sqloid detects deletion, rename-away, and same-path replacement of its open database file at **request boundaries**, never through background observation. This page records the semantics implemented in `internal/connection` (`health.go`, `identity_unix.go`, `identity_other.go`, plus opener/lease wiring) per Issue #7 and the *Session health* section of `Notes/PRD-sqloid.md`.

**There is no watcher, no polling loop, no continuous monitoring, and no UI dependency anywhere in this mechanism.** Health is checked only when a request happens. Terminal wording (e.g. the exact deletion/replacement session-end messages) is owned by Issue #46; this package produces typed outcomes with neutral diagnostic text only.

## Startup identity recording

When [the opener](sqlite-startup.md) completes its full ordered validation (existence → readability → header → writability → mode=rw open → probe), it immediately re-stats the validated target and records its **device and inode** on Linux/macOS as the reference identity for the session. A stat failure in that instant is classified as a read-write startup failure rather than silently leaving a zero reference.

- Linux/macOS: `statIdentity` reads device/inode from `syscall.Stat_t` via `os.Stat` (`identity_unix.go`).
- Other platforms: `identity_other.go` reports identity capture unsupported; nothing is recorded and verification trivially passes (Sqloid targets Linux/macOS).

## Boundaries: before every request and every new connection

The original path is verified immediately:

1. **Before every database request** — `DB.RunRequest` acquires a lease first, and `DB.Lease` itself performs the verification after checkout but before configuring/handing over the connection. Because all work follows `Lease`, every request begins verified.
2. **Before any newly opened or replacement pooled connection is admitted for use** — the same `Lease` guard applies whether the pool hands back a retained idle connection or opens a fresh physical one; none reaches a caller unverified.

Verified calls increment an internal counter so tests can prove exactly-one-check contracts without hooks.

## Typed outcomes

`VerifyHealth()` returns nil or a `*HealthError{Path, Kind, Cause}`:

| Situation | Kind | Meaning |
|---|---|---|
| File deleted | `HealthDeleted` | stat of the original path failed; existence cannot be confirmed |
| Renamed away | `HealthDeleted` | rename-away is absence of the path |
| Stat fails for any other reason | `HealthDeleted` | classification preserves the unwrappable cause (`Unwrap` → e.g. `fs.ErrNotExist`) |
| Same path, different device or inode | `HealthReplaced` | both identifiers are compared; either mismatch classifies as replacement |
| In-place mutation retaining device+inode | nil | ordinary SQLite behavior continues unchanged |

`Error()` text is neutral diagnostics ("database file absent at request boundary" / "database file device/inode differs from startup") — deliberately distinct from the exact terminal copy Issue #46 renders.

`RunRequest(parent, op)` returns a `RequestResult{Outcome, Err, Health}`: one complete lifecycle — boundary check (inside Lease) → cancellable op on the dedicated lease ([Issue #6](cancellation-infrastructure.md)) → settlement → post-error reclassification → lease release.

## Race behavior at boundaries

- **Raced replacement + request error**: after every failed outcome, identity is re-verified *before* ordinary SQLite error handling returns; deletion or replacement takes precedence while the request's own error cause stays preserved alongside.
- **Raced replacement + request success**: a successful result stands even if the file was replaced after its precheck; the **next** `RunRequest` boundary detects replacement before any further database work begins (its operation never starts).
- **Cancellation outcomes** are returned untouched; cancellation is not an error and does not trigger reclassification.
- There is intentionally **no automatic retry** around any of this.

## Phased write transaction

A write is one request: it receives **exactly one pre-BEGIN check** (the boundary check performed by Lease) and **none between statement execution and COMMIT** — including for rolled-back phases. Tests instrument the counter to pin this exactly-once property.

Cross-references: [sqlite-startup.md](sqlite-startup.md) · [connection-pool.md](connection-pool.md) · [cancellation-infrastructure.md](cancellation-infrastructure.md) · PRD *Session health* section.
