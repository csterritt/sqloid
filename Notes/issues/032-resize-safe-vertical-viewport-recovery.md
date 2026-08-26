## Issue 32: Resize-safe vertical viewport recovery

**Type**: AFK
**Blocked by**: Issue 25, Issue 26, Issue 30, Issue 31

### Parent PRD

`PRD-sqloid.md`

### What to build

Recompute page size on resize while preserving the exact first logical row when valid within the post-eviction dual-cap contiguous retained range. Otherwise clamp to a known retained endpoint or fetch the containing page, invalidating stale old-size work through viewport generations and applying the row- and byte-cap cache invariants.

### How to verify

- **Manual**: Resize during idle and pending paging at first, middle, and end positions with and without retained target rows.
- **Automated**: Model tests cover preservation, low/high clamp, containing-page fetch, old-generation rejection, and exact new page size.

### Acceptance criteria

- [ ] Given the prior first row is retained and valid, then resize preserves that exact logical position.
- [ ] Given it is unavailable, then the viewport clamps to a known endpoint or requests the page containing the required row.
- [ ] Given an old-size request returns, then its response is rejected and cannot overwrite the resized viewport.

### User stories addressed

- User story 53: Preserve or recover the first logical row safely on resize

---
