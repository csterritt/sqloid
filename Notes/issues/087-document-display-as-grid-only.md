## Issue 87: Document Value.Display as grid-only

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Correct the `internal/result` package and `Value.Display()` documentation so `Display()` is explicitly the grid-facing presentation path, not a shared exporter token. Document typed `Value` fields and `Kind` as the shared export seam and format-specific serializers as responsible for preserving TEXT bytes and applying CSV/JSON NULL, BLOB, numeric, and non-finite policies. Do not change runtime rendering or serialization behavior.

### How to verify

- **Manual**: Read the rendered package and method documentation and confirm it no longer directs exporters through grid-only tab/newline symbols, `(NULL)`, or `[BLOB n bytes]` output.
- **Automated**: Run result and export tests that distinguish grid display from CSV/JSON serialization for tabs, newlines, NULL, BLOB, finite REAL, and non-finite REAL values; add or adjust documentation-oriented assertions only if the repository already enforces comment text.

### Acceptance criteria

- [ ] Given the `internal/result` package documentation, then typed values are identified as the shared representation consumed by grid and exporters.
- [ ] Given `Value.Display()` documentation, then it is described as grid-only and does not claim exporters should consume its transformed string.
- [ ] Given CSV and JSON serializers, then documentation directs them to inspect `Value.Kind` and typed payload fields for format-specific output.
- [ ] Given existing result and export tests, then all runtime output remains byte-for-byte unchanged.

### User stories addressed

- User story 73: Keep CSV and JSON output format-specific and deterministic
- User story 74: Preserve format-specific NULL, TEXT, BLOB, and REAL representations

---
