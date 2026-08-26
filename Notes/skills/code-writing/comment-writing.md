---
name: comment-writing
description: Rules for Go doc comments and explanatory comments in this project.
---

- Keep existing comments unless they are wrong or stale; update them when behavior changes.
- Every package should have one package comment. Every exported declaration should have a doc comment unless the declaration is self-documenting within a grouped declaration.
- Start a declaration's doc comment with the declared name and write complete sentences ending in punctuation.
- Document contracts that callers need: ownership, mutation, cancellation, concurrency safety, zero-value behavior, units, limits, and non-obvious error conditions.
- Use comments to explain why a constraint or unusual choice exists, not to narrate code that names and control flow already make clear.
- Document invariants around request identities, transaction phases, terminal state, cache ownership, and layout arithmetic where the code alone cannot make them obvious.
- Do not leave commented-out code, TODOs without actionable context, or foreign-language documentation annotations in Go code.
- Keep exact user-facing behavior in tests and project documentation rather than relying on comments as the only specification.

Reference: [Go Doc Comments](https://go.dev/doc/comment).
