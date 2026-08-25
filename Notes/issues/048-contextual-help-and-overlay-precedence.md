## Issue 48: Contextual help and overlay precedence

**Type**: AFK
**Blocked by**: Issue 7, Issue 10, Issue 15, Issue 20

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the non-quit portions of the authoritative **Global Key Precedence and Context/Action Matrix**: terminal/top-overlay/input/request/base ordering, literal `?` in inputs, contextual base help and its context-specific content, and Esc cancellation of only the top overlay with exact focus restoration. WHERE help must explain SQL-NULL semantics, result help must explain independent complete-limited-result count semantics, and reduced terminal help must expose only actions available without starting database work.

### How to verify

- **Manual**: Exercise `?`, Esc, navigation, history, save/export, and Ctrl+W across base fields, searchable/scroll-only popups, text input, help, picker, confirmation, pending, terminal, and too-small contexts; inspect WHERE, result-count, and reduced terminal help content.
- **Automated**: Table-driven scripted model tests cover every matrix row, key consumption, nonstacking overlays, focus restoration, no key leakage, and the required context-specific help disclosures.

### Acceptance criteria

- [ ] Given focused text/search, then `?` inserts literally; given a base context, then it opens contextual help.
- [ ] Given WHERE context, then help says typed `NULL` is TEXT, directs SQL-null intent to `IS NULL`/`IS NOT NULL`, and explains that ordinary comparisons and LIKE do not match actual NULL values.
- [ ] Given a result count, then help explains that it counts the complete SELECT including Limit rather than table or pre-limit rows, is an independent autocommit read that may drift, and never clamps pages.
- [ ] Given a deletion, replacement, or outcome-unknown terminal context, then reduced help lists only available in-memory actions and immediate status-1 quit without suggesting any database work.
- [ ] Given an overlay, then Esc cancels only the top overlay and restores its exact opener state/focus without leaking the key.
- [ ] Given overlapping key meanings, then terminal, overlay, input, request, and base precedence is applied in the documented order.

### User stories addressed

- User story 28: Explain typed TEXT `NULL` and explicit SQL-null operators
- User story 59: Explain complete limited-result count semantics and independent drift
- User story 77: Make `?` context-sensitive without stealing text input
- User story 78: Cancel only the top overlay and restore exact focus

---
