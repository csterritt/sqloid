---
name: running-tests
description: How to run and diagnose Sqloid's Go tests and verification tools.
---

Run commands from the repository root.

## Focused development

- Run one package: `go test ./internal/<package>`.
- Run one test: `go test ./internal/<package> -run '^TestName$'`.
- Disable cached results while reproducing a failure: `go test -count=1 ./internal/<package> -run '^TestName$'`.
- Use `-v` only when its additional output helps diagnose the failure.

Start with the narrowest failing test. Fix the root cause, rerun that test, then broaden verification. Do not update expectations merely to accept incorrect behavior.

## Required verification

When a Go module exists, finish code changes with:

```bash
gofmt -w <changed-go-files>
go vet ./...
go test ./...
go build ./...
```

Use `gofmt -d <changed-go-files>` when checking without modifying files.

## Additional verification

- Run `go test -race ./...` for concurrency changes when cgo and a supported C toolchain are available. This verification build may use cgo even though production must use the pure-Go SQLite driver.
- Run a specific fuzz target with `go test <package> -fuzz '^FuzzName$' -fuzztime=<bounded-duration>`; ordinary `go test` still runs its seed corpus.
- Use temporary directories through `t.TempDir()` and test cleanup APIs. Do not depend on test order, shared working-tree files, sleeps, network access, or a developer's local database.
- Run project capability, integration, and release suites required by the issue or PRD in addition to `go test ./...`.

## References

- [testing package](https://pkg.go.dev/testing)
- [Go command: test flags](https://pkg.go.dev/cmd/go#hdr-Testing_flags)
- [Data race detector](https://go.dev/doc/articles/race_detector)
