## Issue 80: Settle malformed schema revalidation attempts

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Harden schema revalidation against an exported `schema.Attempt` carrying an unknown or zero status. Instead of returning an empty, unsettled `Revalidation`, map malformed attempts to the settled refresh-failed status with a concrete diagnostic cause and no catalog, preserving the contract that every revalidation result is actionable by its consumer.

### How to verify

- **Manual**: Inspect the revalidation status mapping and confirm every switch path, including the default path, produces one documented settled status with the permitted payload.
- **Automated**: Add table-driven schema tests that pass zero and unknown attempt statuses and assert refresh-failed status, a non-nil cause, a nil catalog, and no panic; retain existing unchanged, refreshed, ordinary-failure, deletion, and replacement cases.

### Acceptance criteria

- [ ] Given an `Attempt` with a zero or unknown status, when it is revalidated, then the result is settled as refresh failed with a concrete cause and no catalog.
- [ ] Given any constructor-produced attempt status, when it is revalidated, then its existing status and payload mapping remains unchanged.
- [ ] Given malformed internal state reaches a consumer, then the consumer does not receive another unknown or zero status to interpret.

### User stories addressed

- User story 22: Keep failed schema refreshes visibly stale and actionable
- User story 81: Settle pre-execution schema revalidation as success, failure, or terminal health state

---
