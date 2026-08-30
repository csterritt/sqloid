## Issue 58: Classify permission-denied stat failures as unreadable

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Refine startup path-stat error classification so only `os.IsNotExist` failures are reported as missing. Preserve EACCES/EPERM causes from inaccessible files or denied parent-directory traversal and classify them as unreadable/permission denied, producing an actionable path-specific diagnostic rather than `no such file or directory`. Keep directory, header, read-write open, and other raw-cause classifications unchanged.

### How to verify

- **Manual**: On Linux or macOS, deny traversal on a fixture's parent directory and attempt `sqloid sqlite <path>`; confirm one stderr line names the path and says `permission denied`, then compare with a genuinely missing path that still says `no such file or directory`.
- **Automated**: Startup tests inject EACCES and EPERM from the stat boundary and assert unreadable classification, preserved causes, exact permission-denied rendering, and exit status 1; separate missing-path and unrelated-stat-error cases assert their distinct existing classifications.

### Acceptance criteria

- [ ] Given path stat fails with EACCES or EPERM, then startup classifies the input as unreadable, preserves the cause, and reports `<path>: permission denied` rather than missing.
- [ ] Given path stat fails because the path does not exist, then startup still classifies it as missing and reports the missing-file diagnostic.
- [ ] Given either startup validation failure, then exactly one actionable stderr line is emitted and the process exits 1 without creating a file.

### User stories addressed

- User story 3: Distinguish missing and unreadable startup inputs
- User story 7: Name the startup path and actionable failure reason
- The stat/readability stage is distinct from the mode=rw open-stage failures covered by User story 90.

---
