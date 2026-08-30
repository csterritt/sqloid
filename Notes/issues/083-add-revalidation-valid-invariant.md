## Issue 83: Add a Revalidation payload invariant

**Type**: AFK
**Blocked by**: Issue 82 — encode the settled revalidation status mapping as an invariant

### Parent PRD

`PRD-sqloid.md`

### What to build

Add `Revalidation.Valid()` as the typed invariant guard for settled schema-revalidation values, mirroring `Attempt.Valid()`. It must encode the documented payload rules: unchanged and refreshed statuses require a catalog and no cause, refresh failure requires a cause and no catalog, terminal deletion/replacement carry neither, and zero or unknown statuses are invalid.

### How to verify

- **Manual**: Compare the method's status matrix with the `Revalidation` field contract and confirm each status permits exactly its intended catalog/cause payload.
- **Automated**: Add a table-driven truth table covering every valid status/payload combination plus missing required fields, forbidden extra fields, zero status, and unknown status; assert existing revalidation results satisfy `Valid()`.

### Acceptance criteria

- [ ] Given an unchanged or refreshed revalidation, then `Valid()` is true only with a non-nil catalog and nil cause.
- [ ] Given a refresh-failed revalidation, then `Valid()` is true only with a nil catalog and non-nil cause.
- [ ] Given a terminal deletion or replacement revalidation, then `Valid()` is true only when both catalog and cause are nil.
- [ ] Given a zero/unknown status or any contradictory payload, then `Valid()` returns false.

### User stories addressed

- User story 22: Represent stale schema refresh failure distinctly from terminal health states
- User story 81: Keep pre-execution schema-validation outcomes internally consistent

---
