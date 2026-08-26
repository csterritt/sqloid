## Issue 7: Request-boundary database identity checks

**Type**: AFK
**Blocked by**: Issue 5

### Parent PRD

`PRD-sqloid.md`

### What to build

Record the startup device/inode and enforce the **Session health** contract before every request and newly opened connection. Return typed deletion and same-path-replacement classifications while allowing same-inode mutation to follow ordinary SQLite behavior; do not embed terminal UI strings, which Issue 46 owns.

### How to verify

- **Manual**: During a session, delete, rename away, replace, and mutate the database, then trigger the next operation.
- **Automated**: Linux/macOS integration tests cover prechecks, post-error reclassification, request races, pooled connection opening, and the single precheck for a phased write.

### Acceptance criteria

- [ ] Given the original path is absent at a request boundary, then the deletion terminal state appears before database work starts.
- [ ] Given the path has a different device/inode, then the distinct replacement terminal state appears.
- [ ] Given an in-place same-inode mutation, then it is handled as ordinary SQLite behavior rather than a replacement.
- [ ] Given a raced replacement followed by a request error, then the error is classified terminal immediately rather than as an ordinary request error.
- [ ] Given a raced replacement followed by a request success, then the success stands for that request and the replacement is detected at the next request boundary before more work begins.
- [ ] Given a phased write transaction, then there is exactly one identity precheck before BEGIN and none between statement execution and COMMIT.

### User stories addressed

- User story 9: Detect deletion, rename-away, and replacement on the next request
- User story 84: Distinguish same-path replacement from same-inode mutation

---
