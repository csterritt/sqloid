---
name: always-do-red-green
description: Follow the Red/Green/Refactor cycle when changing Sqloid's Go code.
---

## Red

- Before changing behavior, add the smallest failing test that demonstrates the required contract or reproduces the bug.
- Place tests beside the package under test in `_test.go` files. Prefer same-package tests for internal behavior and external-package tests when validating only the public API.
- Use table-driven tests with descriptive case names when several inputs exercise the same contract. Call `t.Helper()` in test helpers and make failure messages identify input, got, and want.
- Test observable behavior rather than private implementation details. Avoid mocks when a small fake, temporary file, in-memory model, or narrow injected function is clearer.
- For Bubble Tea behavior, send messages through `Update` and assert model transitions, returned commands, and rendered output without sleeps.

Run the focused test and confirm it fails for the expected reason before implementing the change.

## Green

- Write the minimum production code that makes the focused test pass while preserving PRD and issue contracts.
- Run the focused test after each meaningful change, then run `go test ./...` once it passes.

## Refactor

- Remove duplication and improve names or boundaries only while tests remain green. Do not add speculative abstractions.
- Finish with `gofmt`, `go vet ./...`, `go test ./...`, and `go build ./...` when the module exists.
- Run `go test -race ./...` for concurrency changes when the environment supports the race detector. The race detector requires cgo even though the production SQLite build is pure Go/no-cgo.
- Add focused fuzz tests for parsers, quoting, serialization, or other input-heavy boundaries when useful; keep deterministic regression cases for every discovered failure.

## References

- [Go testing package](https://pkg.go.dev/testing)
- [Go test comments](https://go.dev/wiki/TestComments)
- [Go race detector](https://go.dev/doc/articles/race_detector)
- [Go fuzzing](https://go.dev/doc/security/fuzz/)
