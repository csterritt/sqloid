## Issue 60: Retain page truncation and limit-failure metadata at settlement

**Type**: AFK
**Blocked by**: None — implement before Issue 61 to coordinate their shared `applySelectSettled` change

### Parent PRD

`PRD-sqloid.md`

### What to build

Preserve page limit metadata as first and later SELECT requests settle in `internal/ui/first_select.go` and `internal/ui/paging.go`. `applySelectSettled` must copy `FirstPageResult.ByteTruncated` and `LimitFailure` into `ResultView` and OR byte truncation with `viewportCache.TruncatedByByteCap()` after merging the first page. `applyPageSettled` must OR the new page/cache truncation state with prior state and prefer a newly reported non-nil `LimitFailure`, retaining an earlier failure when the new page has none. The persistent 64 MiB warning and exact page/value row-N diagnostic must survive the real settlement path without tests mutating `ResultView` directly.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Execute SELECT fixtures where the first page and then a later page independently trigger byte-cap truncation and oversized page/value failures; navigate and resize afterward and confirm each persistent warning/failure remains visible with the correct row.
- **Automated**: UI settlement tests inject `ByteTruncated` and each `LimitFailure` through `FirstPageResult` and later-page result messages, assert cache-derived truncation is ORed after merge, assert a new failure replaces the prior one while an absent new failure preserves it, and verify exact warning/diagnostic rendering without direct `ResultView` field assignment.

### Acceptance criteria

- [ ] Given a first-page result reports byte truncation or the merged cache becomes byte-truncated, then accepted settlement records it and persistently renders exactly `Result truncated: 64 MiB cache limit`.
- [ ] Given a first-page result carries a page/value `LimitFailure`, then accepted settlement stores and renders that exact failure and one-based logical row.
- [ ] Given a later page reports truncation, then prior, new, and cache truncation facts are ORed so a true value is never discarded.
- [ ] Given a later page carries a new non-nil `LimitFailure`, then it becomes the retained failure; when it carries none, any earlier failure remains available.

### User stories addressed

- User story 54: Persistently disclose byte-cap truncation while paging
- User story 55: Retain truthful snapshot metadata
- User story 89: Preserve exact oversized page/value failure positions

---
