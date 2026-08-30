## Issue 64: Distinguish unseen low endpoints from truncation

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Correct `internal/history/snapshot_classify.go` so completeness uses low-endpoint evidence consistently. A nonempty snapshot with `ReachedLow == false` is `partial` when rows below the retained range may simply be unseen; it must not fall through to `truncated` absent eviction or rows known to exceed retention. An empty logical result (`high == 0`) can be `complete` without `ReachedLow` when work finished and full retention otherwise holds. Keep `truncated` reserved for row/byte eviction or known/observed rows outside the retained range, and preserve truthful coexistence of incomplete labels where applicable.

### How to verify

- **Manual**: View/export a settled snapshot retaining positions 11–20 of known total 20 without low-end eviction and confirm it is partial, not truncated; execute an empty result and confirm it is complete.
- **Automated**: Table-driven `history.Classify` tests cover unseen low endpoint with known high, empty complete with `ReachedLow=false`, actual low/high eviction, unknown work, and mixed partial/truncated evidence; assert `complete := ... && (high == 0 || meta.ReachedLow) && fullRetention` semantics and `!meta.ReachedLow` partial classification for nonempty results.

### Acceptance criteria

- [ ] Given a nonempty settled snapshot reached the high endpoint but never observed the low endpoint and has no eviction/known overflow, then it is labeled `partial` and not `truncated`.
- [ ] Given a finished empty logical result with full retention and no contradictory evidence, then it is labeled exclusively `complete` even when `ReachedLow` is false.
- [ ] Given rows were evicted by row/byte cap or are known/observed outside the retained range, then `truncated` remains true and may coexist with `partial` when truthful.
- [ ] Given a nonempty result is labeled complete, then the low endpoint and high endpoint are both established and the full limited logical result is retained.

### User stories addressed

- User story 55: Store and export truthful completeness metadata
- User story 56: Keep unseen remainder partial unless an endpoint is established

---
