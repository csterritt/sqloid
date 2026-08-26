# Tasks for #48: SQL save targeting and standalone serialization

Parent issue: #48
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify ordinary and terminal save targeting

**Type**: RED
**Output**: Failing model tests cover viewed-result query, runnable builder, last execution, no-target error/no picker, terminal selected query fallback, and zero database work.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven target-resolution tests in `internal/ui` and `internal/export`, using immutable query/result associations from `internal/history` and `internal/result`. In ordinary state, require Ctrl+S to choose the query associated with the currently viewed historical result before a current runnable builder, and choose that runnable builder before the last actual execution; cover every pairwise and all-present priority combination plus absent and non-runnable builders. With no target, require exactly `no runnable query to save`, no picker, and no serialization. In deletion, replacement, and outcome-unknown terminal states, require only the Ctrl+P/N-selected immutable query when present, otherwise the last actual execution; explicitly ignore current builder and viewed-result priority there. Assert target resolution and picker preparation use immutable memory only and issue zero validation, schema, connection, or database work. Keep this task test-only and do not implement filesystem picker/save behavior owned by later issues.

---

### 2. Implement Ctrl+S target resolution

**Type**: GREEN
**Output**: Ordinary and terminal priority tests pass using immutable in-memory state.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure Ctrl+S target resolution across `internal/ui`, `internal/history`, `internal/result`, and the UI-independent save boundary in `internal/export`. Resolve ordinary targets in exact viewed-result-query, runnable-builder, last-execution order, with viewed-result association obtained from its backing immutable history entry rather than visible text. Resolve terminal targets only from the selected immutable query or last actual execution, never from builder state or database inspection. Return the exact no-target feedback and do not open a picker when resolution fails; when it succeeds, pass an immutable complete query state onward without starting validation or database work. Implement only enough to make Task 1 pass, leaving full statement assembly to Tasks 3-4 and filesystem behavior to its owning issues.

---

### 3. Specify standalone SQL assembly

**Type**: RED
**Output**: Failing exact-byte/round-trip tests cover all commands, identifiers, strings, numerics, NULL, BLOB, UPDATE/INSERT choices, and trailing semicolon through Issue 14 atoms.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exact-byte statement tests and modernc SQLite round-trip tests in `internal/export`, using complete immutable query states from `internal/history`/`internal/result` and Issue #14's canonical identifier, fixed-token, and typed-literal atoms. Cover SELECT variants, qualified and unqualified UPDATE/DELETE, and INSERT; include embedded-quote, keyword, punctuation, and empty-looking schema-derived identifiers; quote-doubled empty/control/NUL/injection-looking TEXT; signed-int64 boundaries; finite REAL integral identity, negative zero, exponent, subnormal, and precision edges; SQL NULL; and empty/nonempty BLOB bytes. Exercise UPDATE SET ordering and optional WHERE plus INSERT Value, NULL, and Default/Omit choices, proving command structure and deterministic ordering match the runnable builder. Require exactly one executable statement, exact whitespace/token bytes, no placeholders or second statement, and one trailing semicolon, then execute it against SQLite to verify semantic round trip. Assert assembly calls Issue #14 atoms rather than defining a second literal serializer, and keep this task test-only.

---

### 4. Implement full-statement serialization

**Type**: GREEN
**Output**: One executable statement is produced without a second literal serializer.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement UI-independent full-statement assembly in `internal/export` from the immutable complete query representation shared by `internal/history` and `internal/result`, and have `internal/ui` pass only the target selected by Task 2. Serialize each supported SELECT, UPDATE, DELETE, and INSERT structure in deterministic builder order, delegating every identifier, fixed SQL token, and INTEGER/REAL/TEXT/NULL/BLOB literal to Issue #14's canonical atoms. Preserve UPDATE Value/NULL assignments, optional predicates, and INSERT Value/NULL/Default-Omit choices exactly, produce one standalone executable statement with one trailing semicolon, and return typed errors for unsupported or incomplete states without opening a picker. Do not add a private literal renderer, interpolate untrusted raw SQL, query the database, or implement loading saved SQL; implement only enough to make Task 3 pass.

---

### 5. Document SQL save behavior

**Type**: DOCUMENT
**Output**: Wiki documentation records target priority, terminal behavior, no-target feedback, canonical serialization, and unsupported loading.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #48 implementation and tests from `internal/history`, `internal/ui`, `internal/export`, and `internal/result` into the appropriate pages under `Notes/wiki`. Document ordinary Ctrl+S priority from viewed historical result query to runnable builder to last actual execution; terminal-only priority from selected immutable query to last actual execution; zero database work; and exact `no runnable query to save` feedback with no picker. Record supported command assembly, deterministic structure, Issue #14's sole canonical identifier/fixed-token/literal ownership, exact typed value handling, and one trailing semicolon. State that this issue prepares one standalone statement but does not support loading saved SQL or own later filesystem picker/overwrite completion. Cross-reference Issues #14, #35, #36, #42, #45, #46, and #48 plus Query save targeting, SQL safety, and terminal context/action rules in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the SQL-save walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/048-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/048-06/code-walkthrough`. Demonstrate every ordinary priority combination among viewed-result query, runnable builder, and last execution, plus no-target exact feedback and absence of a picker. Enter deletion, replacement, and outcome-unknown terminal states and show selected-query then last-execution fallback with zero database work. Serialize and round-trip representative SELECT, qualified/unqualified UPDATE and DELETE, and INSERT statements containing difficult identifiers, strings, numeric edges, NULL, BLOB, UPDATE assignments, and INSERT Value/NULL/Default-Omit choices; inspect exact bytes and the single trailing semicolon. Include evidence that assembly uses Issue #14's atoms with no second literal serializer and that loading/filesystem completion remains unsupported here. Reference Issue #48 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review query saving

**Type**: REVIEW
**Output**: Human saves every ordinary/terminal target and verifies exact executable SQL and no-target behavior.
**Depends on**: 6

Review immutable target associations in `internal/history` and `internal/result`, Ctrl+S resolution in `internal/ui`, statement assembly in `internal/export`, the wiki updates, and `Notes/walkthroughs/048-06/code-walkthrough` against Issue #48. Manually exercise viewed-result, runnable-builder, last-execution, selected-terminal-query, and terminal last-execution targets in every priority combination, confirming terminal resolution remains in memory and starts no database work. Verify exact no-target feedback and no picker. Compare and execute saved SELECT, UPDATE, DELETE, and INSERT SQL with difficult identifiers and every typed literal category, checking UPDATE/INSERT choices, exact bytes, one statement, and trailing semicolon. Confirm Issue #14 remains the sole atom/literal serializer and no SQL-loading behavior was added before approving the issue.

---
