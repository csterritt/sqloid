## Issue 43: Immutable result-export capture and warnings

**Type**: AFK
**Blocked by**: Issue 30, Issue 32, Issue 41

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement Ctrl+X targeting and immutable instant capture for active or historical tabular results. Gate it during requests, preserve active SELECT lifetime/state, and present truthful metadata warnings before destination selection without adding warning data.

### How to verify

- **Manual**: Export active and historical complete/partial/truncated/cancelled/failed/invalid-UTF snapshots; cancel and complete the flow during an active SELECT.
- **Automated**: Model tests assert immutable capture timing, request gating, non-tabular rejection, unchanged active state, warning combinations, and no metadata records in serializer input.

### Acceptance criteria

- [ ] Given an idle tabular selection, then Ctrl+X captures an immutable instant copy without finalizing or changing the active SELECT.
- [ ] Given any request or non-tabular selection, then export is blocked with the documented feedback and no picker opens.
- [ ] Given incomplete or warned metadata, then truthful warnings appear outside exported CSV/JSON and cancel/complete restores unchanged state.

### User stories addressed

- User story 69: Capture immutable result export only while idle
- User story 70: Preserve active state and disclose result metadata outside data

---
