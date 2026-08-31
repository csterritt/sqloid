# Tasks for #59: Percent-encode relative SQLite DSN paths

Parent issue: #59
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify escaped relative file-URL construction

**Type**: RED  
**Output**: Failing DSN and open tests cover relative `?`, `#`, spaces, combined reserved characters, ordinary relative paths, and absolute paths.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven tests in `internal/connection` for `mustFileURL`, `dsn`, and real `Open` behavior using working-directory-relative SQLite filenames containing `?`, `#`, spaces, and combined reserved characters. Require reserved characters to appear percent-encoded as URL path data, never as query/fragment delimiters, and require relative file URLs to have no invented `//` authority while preserving the real `_busy_timeout` and `mode=rw` query options. Pin ordinary relative paths, dot-prefixed Wrangler-style paths, and absolute paths as regressions. For each real file fixture, assert the intended existing database opens read-write, a known row is selected, a write reaches that same file, and no differently parsed or newly created path appears. Keep this task test-only and follow `internal/connection/opener_test.go`, `startup_test.go`, and the current `dsn`/`mustFileURL` boundary.

---

### 2. Encode relative paths without inventing authority

**Type**: GREEN  
**Output**: Relative reserved-character filenames produce valid escaped mode=rw DSNs and open the intended existing database.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update URL construction in `internal/connection/startup.go` so relative filesystem paths are serialized as escaped file-URL path data without becoming a URL authority and without letting `?`, `#`, or spaces alter URL structure. Preserve `filepath.IsAbs` handling for absolute paths, the caller's relative target selection, and query construction through `url.Values`; keep `mode=rw`, the five-second busy timeout, startup validation order, diagnostics, and no-create behavior unchanged. Use Go's URL representation deliberately rather than pre-escaping text in a way that double-encodes it. Implement only enough to make Task 1 pass, then run focused connection tests and the established Go verification command.

---

### 3. Document relative SQLite DSN escaping

**Type**: DOCUMENT  
**Output**: Wiki documentation records relative/absolute file-URL shapes, path escaping, no-authority behavior, and unchanged startup options.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #59 implementation and tests from `internal/connection` into the startup/connection pages under `Notes/wiki`. Explain how relative paths are represented without an authority, how `?`, `#`, spaces, and combined reserved characters remain percent-encoded filename data, and how absolute and ordinary relative paths retain their intended targets. Record that `mode=rw`, busy timeout, validation, no-create semantics, and path-specific diagnostics are unchanged. Cross-reference Issue #59, user stories 1, 7, and 8, and the Startup validation and errors and Connection Module Design sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the relative-DSN walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/059-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/059-04/code-walkthrough`, with the main file named `walkthrough.md`. Display exact file-URL and DSN construction for relative names containing `?`, `#`, spaces, combined characters, an ordinary Wrangler-style relative path, and an absolute path; verify escaped path data, absent relative authority, and retained mode/read-write options. Run focused automated opening tests, then use the shipped TUI path made executable by Issue #57 to open and query each intended file and prove no alternate file is created. Reference Issues #57 and #59 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
