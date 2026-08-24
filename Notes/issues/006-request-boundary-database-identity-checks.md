## Issue 6: Request-boundary database identity checks

**Type**: AFK
**Blocked by**: Issue 5

### Parent PRD

`PRD-sqloid.md`

### What to build

Record the startup device/inode and enforce the **Session health** contract before every request and newly opened connection. Distinguish deletion from same-path replacement while allowing same-inode mutation to follow ordinary SQLite behavior.

### How to verify

- **Manual**: During a session, delete, rename away, replace, and mutate the database, then trigger the next operation.
- **Automated**: Linux/macOS integration tests cover prechecks, post-error reclassification, request races, pooled connection opening, and the single precheck for a phased write.

### Acceptance criteria

- [ ] Given the original path is absent at a request boundary, then the deletion terminal state appears before database work starts.
- [ ] Given the path has a different device/inode, then the distinct replacement terminal state appears.
- [ ] Given an in-place same-inode mutation, then it is handled as ordinary SQLite behavior rather than a replacement.

### User stories addressed

- User story 9: Detect deletion, rename-away, and replacement on the next request
- User story 84: Distinguish same-path replacement from same-inode mutation

---
