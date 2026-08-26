# Tasks for #7: Request-boundary database identity checks

Parent issue: #7
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify startup identity and boundary classification

**Type**: RED
**Output**: Failing Linux/macOS tests cover recorded device/inode, absence, replacement, same-inode mutation, and checks before requests/new connections.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add Linux/macOS filesystem and SQLite integration tests in `internal/connection` for Issue #7 and the Session health and Connection module contracts in `Notes/PRD-sqloid.md`. Extend the Issue #2 opener tests to require recording the validated target's device and inode at startup, and use the Issue #5 pool and leases to prove the original path is checked immediately before every request and before a newly opened or replacement physical connection is used. Cover deletion, rename-away absence, same-path replacement with a different device or inode, and in-place mutation that retains both identifiers; require typed absence and replacement outcomes, allow same-inode changes to follow ordinary SQLite behavior, and avoid asserting terminal UI wording owned by Issue #46.

---

### 2. Implement typed database identity checks

**Type**: GREEN
**Output**: Startup recording and pre-request/new-connection classification tests pass without embedding UI strings.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement startup identity capture and reusable request-boundary verification in `internal/connection`, following the existing ordered validation and shared opener patterns from Issue #2 and dedicated pool ownership from Issue #5. Record the original Linux/macOS device and inode only for the successfully validated target, stat the original path before database work and before any new physical connection is admitted for use, and return typed deletion and replacement classifications that preserve underlying causes without containing terminal-facing copy. Treat rename-away as absence, compare both identifiers for same-path replacement, permit same-inode mutation to continue into ordinary SQLite behavior, and do not add a watcher, polling loop, or UI dependency.

---

### 3. Specify races, post-error reclassification, and write boundaries

**Type**: RED
**Output**: Failing tests cover replacement followed by request error/success and exactly one pre-BEGIN check for a phased write.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-driven tests in `internal/connection` for the race and request-boundary semantics in Issue #7 and the PRD's Session health high-risk coverage. Replace the original path after a successful precheck and control separate requests that then fail or succeed: require a request error to trigger immediate post-error identity reclassification with deletion or replacement taking precedence, while a successful request result stands and the next request detects replacement before further database work. Exercise schema-style reads, count/page-style pooled calls, estimates, and a phased write boundary as reusable Connection requests, and instrument identity checks to prove the whole write receives exactly one precheck before BEGIN with none between statement execution and COMMIT. Use barriers rather than sleeps and keep this task test-only.

---

### 4. Apply health checks to every request boundary

**Type**: GREEN
**Output**: Race classification, pooled-connection, post-error, and phased-write tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Route every reusable database request boundary in `internal/connection` through the typed health check from Task 2, including schema version and refresh work, estimates, count and page operations, and the entire phased write transaction. Recheck identity after every request error before returning ordinary SQLite, cancellation, or transaction outcome handling so deletion or replacement wins the required races; do not discard a successful current request merely because replacement occurred after its precheck, but ensure the next boundary blocks before more work. Apply the same guard to newly opened pooled connections, preserve one pre-BEGIN check for a write with no checks inside its statement-to-COMMIT sequence, and retain the absence of continuous watching and UI strings.

---

### 5. Document session-health boundaries

**Type**: DOCUMENT
**Output**: Wiki documentation records device/inode semantics, race behavior, typed outcomes, and the absence of continuous watching.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #7 implementation and Linux/macOS tests into the appropriate pages under `Notes/wiki`. Document startup device/inode recording, checks before every request and newly opened connection, typed deletion and replacement outcomes, rename-away behavior, same-inode mutation as ordinary SQLite behavior, post-error reclassification precedence, successful-request race handling, and exactly one pre-BEGIN check for a phased write. State explicitly that health is request-boundary based with no watcher or continuous monitoring and that Issue #46 owns terminal copy; cross-reference Issue #7 and the relevant PRD sections, update `Notes/wiki/index.md`, and append the required dated ingest record to `Notes/wiki/log.md` without modifying prior entries.

---

### 6. Create the database-identity walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/007-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/007-06/code-walkthrough`. Demonstrate `internal/connection` startup identity recording and the next-request behavior for deletion, rename-away, same-path replacement, and in-place same-inode mutation, along with controlled replacement races followed by request error and success. Include evidence for checks on newly opened pooled connections, post-error terminal classification, and exactly one pre-BEGIN check for an entire phased write, reference Issue #7 and the Session health requirements in `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---

### 7. Review identity behavior

**Type**: REVIEW
**Output**: Human confirms deletion, rename-away, replacement, and same-inode mutation behavior.
**Depends on**: 6

Review the completed `internal/connection` behavior, Linux/macOS integration tests, wiki updates, and walkthrough against Issue #7 and the Session health, Connection module, and high-risk testing requirements in `Notes/PRD-sqloid.md`. During a session, delete the target, rename it away, replace the same path with a different file, and mutate it in place while retaining its inode, then trigger the next operation and confirm the correct typed behavior. Also confirm race precedence after errors, successful-current-request handling, checks before new pooled connections, one pre-BEGIN write check, no embedded UI strings, and no continuous watcher before approving the issue.

---
