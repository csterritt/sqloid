## Issue 59: Preserve absolute logical positions for later-page failures

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Carry the requested logical `OFFSET` through the later-page execution boundary in `internal/connection/started_request.go`. `StartPage` must receive the same offset used to render `PageSQL` and pass it to `runFirstPage` (or the shared scanner) so oversized value/page scan failures report the one-based absolute logical result position `offset + page-relative row index + 1`. Update every production adapter, interface, fake, and capability test to use one consistent offset contract.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Page beyond the first result page in a fixture whose first oversized value is on that later page; confirm the error identifies its absolute logical row rather than row 1 or another page-relative row.
- **Automated**: Connection and UI-adapter tests request multiple nonzero offsets and inject oversized value/page failures at known page-relative indexes, asserting the exact absolute row N in `result value exceeds the 64 MiB v1 limit at row N` or `result page exceeds the 64 MiB v1 limit at row N`; contract tests assert the execution offset equals the `PageSQL` offset.

### Acceptance criteria

- [ ] Given a page rendered with nonzero SQL `OFFSET`, when that page is started, then the identical logical offset reaches the row scanner.
- [ ] Given an oversized value at page-relative index i on a request with offset O, then the visible failure is exactly `result value exceeds the 64 MiB v1 limit at row N` where `N = O + i + 1`.
- [ ] Given a page-cap failure at page-relative index i on a request with offset O, then the visible failure is exactly `result page exceeds the 64 MiB v1 limit at row N` where `N = O + i + 1`, with no partial row retained.
- [ ] Given first-page offset zero, then its existing one-based diagnostics remain unchanged.

### User stories addressed

- User story 48: Fetch and browse vertical result pages
- User story 89: Identify oversized page/value failures at the first absolute logical position

---
