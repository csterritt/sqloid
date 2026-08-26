## Issue 49: Immutable result-export capture and warnings

**Type**: AFK
**Blocked by**: Issue 31, Issue 34, Issue 36, Issue 45, Issue 46, Issue 47

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement Ctrl+X targeting and immutable instant capture for active or historical tabular results. In deletion/replacement and outcome-unknown terminal states, act only on the in-memory selected result: export a tabular snapshot without database work, or reject a non-tabular entry with the exact documented message and no picker. This issue is the definition site for `selected result has no tabular data to export`; terminal consumers reuse it. Gate export during requests, preserve active SELECT lifetime/state, and present truthful metadata warnings before destination selection without adding warning data, reusing Issue 31's byte-cap warning definition rather than duplicating it.

### How to verify

- **Manual**: Export active, historical, and terminal-selected complete/partial/truncated/cancelled/failed/invalid-UTF snapshots; cancel and complete the flow during an active SELECT.
- **Automated**: Model tests assert immutable capture timing, request gating, ordinary and terminal tabular export, exact non-tabular rejection with no picker, unchanged active state, warning combinations, and no metadata records in serializer input.

### Acceptance criteria

- [ ] Given an idle tabular selection, then Ctrl+X captures an immutable instant copy without finalizing or changing the active SELECT.
- [ ] Given any request or non-tabular selection, then export is blocked with the documented feedback and no picker opens.
- [ ] Given incomplete or warned metadata, then truthful warnings appear before destination selection and outside exported CSV/JSON, and cancel/complete restores unchanged state; `truncated-by-byte-cap` displays exactly `Result truncated: 64 MiB cache limit`.
- [ ] Given a terminal-state result selection, then a tabular snapshot exports from immutable memory without database work, while a non-tabular entry or empty selection reports `selected result has no tabular data to export` and opens no picker.

### User stories addressed

- User story 69: Capture immutable result export only while idle
- User story 70: Preserve active state and disclose result metadata outside data
- User story 80: Preserve terminal-state in-memory tabular export without database work
- User story 85: Reject non-tabular outcome-unknown export without misleading output

---
