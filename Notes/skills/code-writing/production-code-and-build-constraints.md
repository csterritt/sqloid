---
name: production-code-and-build-constraints
description: Rules for keeping test and development behavior out of Sqloid production binaries.
---

## Production boundaries

- Do not use source-rewriting comments or cleanup scripts to transform development code into production code.
- Keep test fixtures, fakes, and test-only helpers in `_test.go` files whenever possible. Put reusable test support in clearly named internal test packages only when multiple packages need it.
- Prefer dependency injection at narrow boundaries over runtime development switches in production packages.
- Use Go build constraints only when behavior genuinely depends on platform, architecture, cgo availability, or an intentionally separate build variant. Build tags are not a substitute for ordinary configuration or test seams.
- When a build constraint is necessary, use the `//go:build` form at the top of the file, provide the complementary implementation where needed, and test every supported variant.
- Do not include permissive authentication, database seeding routes, hidden debug commands, or bypasses in the Sqloid binary. This is a local CLI/TUI, not a web server.
- Keep `cmd/sqloid` as a thin process boundary. Put behavior in testable internal packages and make production wiring explicit.
- Verify release code with `go test ./...`, `go vet ./...`, and `go build ./...`; run capability and platform-specific suites required by the PRD before release.

## References

- [Go build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints)
- [How to Write Go Code](https://go.dev/doc/code)
