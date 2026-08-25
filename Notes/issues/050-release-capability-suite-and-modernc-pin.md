## Issue 50: Integrated release-capability suite and modernc pin/upgrade gate

**Type**: HITL
**Blocked by**: Issue 5, Issue 6, Issue 20, Issue 24, Issue 38, Issue 39

### Parent PRD

`PRD-sqloid.md`

### What to build

Own the integrated Linux/macOS release-capability suite and the modernc pin/upgrade gate that the PRD designates as release- and dependency-upgrade-blocking. While individual issues (5, 6, 20, 24, 38, 39) contain per-issue release-blocking tests, no single issue owns the assembled suite that must pass on every release and every modernc dependency upgrade.

Scope:
- Pin an exact vetted `modernc.org/sqlite` version in `go.mod`. A version that fails any capability test must be changed, never silently accepted as best-effort.
- Assemble and maintain the integrated capability suite that runs on both Linux and macOS:
  - Journal overlap: rollback-journal and WAL count/page overlap on two distinct leases.
  - Pool configuration: exact pool size two, per-connection five-second busy timeout, 64 MiB length limit, unchanged journal mode.
  - Cancellation: interrupt long CPU page and count independently, cancel one without affecting the other or the next request, discard late success, settle CPU cancellation within one second, settle lock wait by five seconds.
  - Identity: deletion, rename-away, same-path replacement terminal messages, raced replacement plus request error classified terminal immediately, raced replacement plus request success detected at next boundary.
  - Transaction: pre-COMMIT cancellation rollback, post-boundary Ctrl+W issues no interrupt, commit boundary enforcement.
- Define the CI gate that fails on a bad modernc upgrade.

Use synchronization barriers rather than sleeps except for explicit latency assertions.

### How to verify

- **Manual**: Review the integrated suite on Linux and macOS for every release and any modernc dependency upgrade.
- **Automated**: The assembled suite runs as a single gated test pass/fail on both platforms. Failure blocks release and requires changing the pinned version or implementation, not weakening behavior.

### Acceptance criteria

- [ ] Given a modernc version is pinned, then the integrated capability suite passes on both Linux and macOS.
- [ ] Given a modernc dependency upgrade, then the CI gate runs the suite and fails the upgrade if any capability test fails.
- [ ] Given a failing capability test, then the release is blocked until the pinned version or implementation is changed, never silently accepted.

### User stories addressed

- User story 82: Settle cancellation visibly, safely, and within required bounds (integrated evidence)
- (Cross-cutting release gate; supports all user stories backed by mandatory capability tests.)

---
