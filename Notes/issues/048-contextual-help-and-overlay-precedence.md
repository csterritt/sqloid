## Issue 48: Contextual help and overlay precedence

**Type**: AFK
**Blocked by**: Issue 7, Issue 10, Issue 46

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the non-quit portions of the authoritative **Global Key Precedence and Context/Action Matrix**: terminal/top-overlay/input/request/base ordering, literal `?` in inputs, contextual base help, and Esc cancellation of only the top overlay with exact focus restoration.

### How to verify

- **Manual**: Exercise `?`, Esc, navigation, history, save/export, and Ctrl+W across base fields, searchable/scroll-only popups, text input, help, picker, confirmation, pending, terminal, and too-small contexts.
- **Automated**: Table-driven scripted model tests cover every matrix row, key consumption, nonstacking overlays, focus restoration, and no key leakage.

### Acceptance criteria

- [ ] Given focused text/search, then `?` inserts literally; given a base context, then it opens contextual help.
- [ ] Given an overlay, then Esc cancels only the top overlay and restores its exact opener state/focus without leaking the key.
- [ ] Given overlapping key meanings, then terminal, overlay, input, request, and base precedence is applied in the documented order.

### User stories addressed

- User story 77: Make `?` context-sensitive without stealing text input
- User story 78: Cancel only the top overlay and restore exact focus

---
