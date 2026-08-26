---
name: go-rules
description: Rules for writing clear, idiomatic, and maintainable Go in this project.
---

## Go code

- Read the relevant issue, task, PRD sections, and existing package before changing code. Preserve the architecture and exact behavioral contracts they define.
- Run `gofmt` on every changed Go file. Do not hand-format around `gofmt`.
- Prefer clarity and simple control flow over cleverness or speculative abstraction. Keep packages cohesive and dependencies directed toward smaller, stable packages.
- Use short, lower-case package names without underscores. Avoid package names such as `util`, `common`, or `helpers` that obscure ownership.
- Keep exported APIs minimal. Define interfaces where they are consumed and only when more than one implementation, substitution in tests, or decoupling at a boundary makes the abstraction useful.
- Accept interfaces and return concrete types by default. Prefer useful zero values where practical.
- Pass `context.Context` explicitly as the first parameter of operations that can block or be cancelled. Do not store a context in a struct.
- Return errors instead of panicking for ordinary failures. Handle every error; wrap only when adding useful operation or identity context, using `%w` when callers must inspect the cause.
- Keep error strings lower-case and without trailing punctuation unless an exact user-facing contract requires otherwise. Use `errors.Is` or `errors.As` rather than matching error text.
- Make goroutine ownership and termination explicit. Do not start a goroutine without knowing how it exits; avoid unbounded goroutine creation and channel operations that can leak.
- Copy slices, maps, or byte buffers when retaining data that callers may mutate. Document ownership when it is not obvious.
- Use Go tooling to manage dependencies (`go get` with a specific version, then `go mod tidy`); do not edit checksums manually. Follow the PRD requirement to pin an exact vetted `modernc.org/sqlite` version.

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/guide)
