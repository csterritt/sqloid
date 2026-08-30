## Issue 70: Run the full definition of done in CI

**Type**: AFK
**Blocked by**: Issue 57

### Parent PRD

`PRD-sqloid.md`

### What to build

Expand the sole CI workflow into a Linux/macOS release gate that runs the repository-wide pure-Go checks `go test ./...`, `go build ./...`, and `go vet ./...` while retaining the targeted race/capability suite for its specialized cancellation guarantees. Add a real built-binary/TUI integration test that exercises production composition rather than only package-local fakes, so regressions in the shipped command, discovery, SQL generation, serialization, schema, and UI wiring cannot merge behind a green partial workflow.

### How to verify

- **Manual**: Inspect a Linux and macOS workflow run, confirm every repository package participates in test/build/vet, and review the binary/TUI integration job's captured successful startup and end-to-end interaction.
- **Automated**: CI runs repository-wide test, build, and vet checks on both supported operating systems, preserves the targeted race/capability invocation, and executes an integration test against the built `sqloid` binary that fails when production TUI composition is absent or broken.

### Acceptance criteria

- [ ] Given a pull request on Linux or macOS CI, then `go test ./...`, `go build ./...`, and `go vet ./...` all run and any failure blocks merging.
- [ ] Given the expanded checks, then the existing targeted race/capability suite remains release-gated rather than being replaced by non-race tests.
- [ ] Given a built production binary and a SQLite fixture, then an automated TUI integration path proves the shipped command reaches and operates the real application composition.
- [ ] Given a regression in any shipped package or production wiring, then at least one required CI gate fails.

### User stories addressed

- User story 1: Open a SQLite database through the shipped command
- User story 57: Run independent page and count work through production composition
- User story 82: Preserve release-blocking cancellation guarantees

---
