# Tasks for #45: Outcome-unknown terminal workflow

Parent issue: #45
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify outcome-unknown result creation

**Type**: RED
**Output**: Failing fake/model tests cover unresolved commit/rollback after settlement, exactly one newest selected entry, operation/SQL/phase/error, and non-proving RowsAffected wording.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-controlled write-outcome tests in `internal/ui` and immutable non-tabular entry tests in `internal/history`, using the phased write fake and execution identities established by Issues #42 and #43. Hold unresolved commit and rollback resolution until all transaction and driver work has ended, then require exactly one newest result entry to be appended and initially selected. Cover INSERT, UPDATE, and DELETE and assert that the immutable entry retains outcome-unknown status, operation and table, exact executed SQL, commit-versus-rollback phase, driver error, and optional statement `RowsAffected()` with wording that explicitly says it does not prove persistence. Reject entry or terminal-state creation while work remains pending, duplicate/late settlement messages, multiple entries, stale identities, and any database-untouched or persistence claim. Keep this task test-only and leave terminal interaction behavior to Tasks 3-6.

---

### 2. Implement outcome-unknown terminal entry

**Type**: GREEN
**Output**: Settlement and immutable entry-selection tests pass without pending driver work.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal unresolved-write finalization across `internal/ui` and `internal/history`. Preserve the execution identity and immutable operation, table, standalone SQL, failed resolution phase, driver error, and optional actual statement row count through settlement; only after the connection lifecycle reports no pending transaction or driver work, atomically append exactly one non-tabular outcome-unknown entry, select it as the newest result, and enter the terminal result view. Make finalization idempotent against duplicate or late messages and reject stale execution identities. Label any retained `RowsAffected()` information as not proving persistence, never claim commit, rollback, or an untouched database, and do not initiate additional driver work. Implement only enough to make Task 1 pass without adding terminal navigation, help, or quit behavior.

---

### 3. Specify terminal in-memory behavior

**Type**: RED
**Output**: Failing tests cover database-work prohibition, Ctrl+P/N and Ctrl+E/Y navigation, empty-history behavior, and reduced help.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model tests in `internal/ui` backed by immutable stores in `internal/history` for the outcome-unknown terminal state. Exercise every normally database-capable action and require that none can create a Bubble Tea command, connection request, validation, estimate, execution, paging, refresh, rerun, or other database work. Require Ctrl+P/N to traverse retained query history and Ctrl+E/Y to traverse retained result history entirely in memory with deterministic boundaries, while preserving the initially selected newest outcome-unknown entry. Cover empty query history and the defensive empty-result fallback without synthetic entries or missing-backed rendering, and prove navigation is a no-op when its history is empty. Require `?` to open reduced help containing only actions actually available in this terminal state and no database suggestions. Keep this task test-only; Ctrl+S and Ctrl+X integration remain owned by Issues #48 and #49.

---

### 4. Implement terminal restrictions and navigation

**Type**: GREEN
**Output**: In-memory navigation/help tests pass and no database command can start.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the outcome-unknown terminal interaction branch in `internal/ui` using immutable query/result selection primitives from `internal/history`. Route Ctrl+P/N and Ctrl+E/Y only through local stable history state, retain deterministic empty and boundary behavior, and ensure selected result content always comes from a backing immutable entry. Suppress every database-starting action before command construction, including execution, validation, estimation, refresh, paging, and rerun. Provide reduced help limited to the terminal state's available in-memory history actions, help dismissal, separately owned save/export actions, and immediate quit keys, without implying database recovery or new work. Preserve the primary outcome-unknown view when no navigable entry exists and implement only enough to make Task 3 pass.

---

### 5. Specify immediate terminal quit

**Type**: RED
**Output**: Failing tests require q/Ctrl+C to exit immediately with status 1 and no confirmation or abandoned work.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the scripted `internal/ui` terminal-state tests for `q` and Ctrl+C from the primary outcome-unknown view, selected query/result history, and reduced help. Require each key to bypass ordinary quit confirmation and immediately produce process termination with status 1. Assert that no confirmation overlay, cancellation request, cleanup command, database request, delayed settlement, or state restoration is scheduled, because terminal entry already guarantees no transaction or driver work remains pending. Exercise repeated keys and both populated and empty histories, and keep this task test-only.

---

### 6. Implement status-1 terminal exit

**Type**: GREEN
**Output**: Immediate terminal quit tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement `q` and Ctrl+C handling for the outcome-unknown terminal state in `internal/ui` so either key exits immediately with status 1 from every terminal in-memory subview. Give this terminal rule precedence over ordinary confirmation, help, and history contexts, and do not issue cancellation, connection, cleanup, or other asynchronous commands. Preserve the invariant established by Task 2 that no transaction or driver work remains to abandon, and implement only enough to make Task 5 pass.

---

### 7. Document outcome-unknown workflow

**Type**: DOCUMENT
**Output**: Wiki documentation records entry data, non-persistence wording, restrictions, history, help, and quit.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #45 implementation and tests from `internal/history` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document settlement before terminal entry; exactly one newest, initially selected immutable non-tabular result; operation/table, executed SQL, commit-versus-rollback phase, driver error, and optional `RowsAffected()` explicitly described as not proving persistence. Record the prohibition on every database action, local Ctrl+P/N and Ctrl+E/Y behavior including empty histories, reduced-help contents, and immediate status-1 `q`/Ctrl+C with no confirmation. Identify Issues #48 and #49 as the owners of terminal save/export integration, cross-reference Issues #36, #42, #43, and #45 plus the Writes and commit boundary and terminal context/action rules in `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md` for any added or removed page, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the outcome-unknown walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/045-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/045-08/code-walkthrough`. Deterministically inject unresolved commit and rollback outcomes, show that terminal entry waits for all transaction and driver work to settle, and inspect the single newest selected immutable entry for status, operation/table, exact SQL, phase, driver error, and non-proving `RowsAffected()` wording. Browse populated and empty query/result histories with Ctrl+P/N and Ctrl+E/Y, attempt all database-capable actions and capture zero commands, open and dismiss reduced help, and demonstrate `q` and Ctrl+C exiting immediately with status 1 and no confirmation. Reference Issue #45 and `Notes/PRD-sqloid.md`, distinguish later Ctrl+S/Ctrl+X ownership, and place every showboat-generated artifact under the approved directory.

---
