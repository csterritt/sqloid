## Issue 46: Directory and filename picker flow

**Type**: AFK
**Blocked by**: Issue 42, Issue 43

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the shared save/export picker from **File picker**. Start at the process working directory, navigate visible and hidden directories plus `..`, keep filename entry separate, append format extensions, and retain inline-retry state on path/permission errors.

### How to verify

- **Manual**: Navigate nested/hidden/parent directories, enter valid and invalid names, omit extensions, trigger permission/path errors, and cancel.
- **Automated**: Scripted picker/filesystem tests assert starting path, ordering/navigation, `/`/NUL/empty validation, extension rules, error retention, and exact opener restoration.

### Acceptance criteria

- [ ] Given picker open, then it begins in the working directory and supports directories including hidden entries and parent `..` without directory creation.
- [ ] Given an empty or `/`/NUL-containing basename, then an inline error appears; otherwise the required missing extension is appended.
- [ ] Given path/permission failure or Esc, then retry/cancel remains safe and the exact opener state/focus is restored.

### User stories addressed

- User story 71: Pick a directory and validated filename safely

---
