## Issue 85: Remove unused traversal limit fields

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Remove the unused `HasLimit` and `Limit` fields from `history.TraversalFacts` and from every producer and test fixture. Keep the completeness rule explicit in documentation: known totals already count the SELECT including the user's Limit, so classification needs no separate raw Limit input. Preserve all complete, partial, truncated, endpoint, and count/cache-inconsistency outcomes.

### How to verify

- **Manual**: Review the finalization and export classification paths and confirm they pass only facts consumed by `history.Classify`, while limited-result semantics remain documented beside the known-total logic.
- **Automated**: Update the completeness truth table and UI traversal-fact producers to omit the dead fields; assert limited known-total cases classify identically to equivalent unbounded cases and run all snapshot finalization/export metadata tests.

### Acceptance criteria

- [ ] Given `TraversalFacts`, then every retained field is read by completeness classification and no separate Limit fields remain.
- [ ] Given a SELECT with a user Limit, then its known total still represents only the limited logical result and rows beyond that Limit remain irrelevant.
- [ ] Given the existing completeness matrix, then complete, partial, truncated, endpoint, and inconsistency classifications are unchanged.

### User stories addressed

- User story 55: Record truthful snapshot completeness and endpoint metadata
- User story 56: Treat observed endpoints and unknown remainders correctly after count failure
- User story 59: Count and classify the complete SELECT including its Limit

---
