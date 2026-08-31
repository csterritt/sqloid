# Tasks for #67: Reject stale INSERT prompts before SQL rendering

Parent issue: #67
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify stale INSERT prompt reporting

**Type**: RED  
**Output**: Failing runnable-report tests reject stored prompts that are dropped, hidden, generated, or otherwise no longer insertable with specific feedback.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/querybuilder/runnable_test.go` and focused prompt tests in `internal/querybuilder/insert_prompt_test.go`, following their catalog builders and immutable INSERT choice transitions. Begin prompts against a catalog where several columns are insertable, complete Value, NULL, and Default/Omit choices, then refresh to catalogs where a stored prompt's column is dropped, hidden, generated, or has `Insertable=false` while the table remains eligible. Require `Runnable=false`, `RunFieldInsertColumns`, and one specific stale-column reason naming or clearly identifying the stale condition before completeness checks on current prompts. Include multiple stale prompts to prove deterministic stored/schema-order handling and controls where every stored prompt remains current. Keep this task test-only and retain stale state deliberately rather than using a helper that clears dependent prompts.

---

### 2. Validate every stored prompt against insertability

**Type**: GREEN  
**Output**: The authoritative INSERT report blocks any prompt absent from the current InsertableColumns set while preserving complete current prompts.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `reportInsert` and its exact reasons in `internal/querybuilder/runnable.go` to build the current insertable-column identity set and validate every stored entry in the builder's INSERT prompt state before accepting it. A prompt whose column is no longer in `InsertableColumns` must return `RunFieldInsertColumns` with the specific stale-column reason, regardless of its former choice or submitted value. Then retain the existing zero-insertable and per-current-column choice/submission checks in visual order. Reuse `InsertableColumns`, prompt identity, and report patterns already used for stale SET/WHERE state; do not infer insertability from declared types, defaults, names, or old prompt metadata. Implement only enough to satisfy Task 1.

---

### 3. Specify stale INSERT renderer refusal and valid regressions

**Type**: RED  
**Output**: Failing INSERT SQL tests require empty SQL/params for stale prompts and unchanged order, choices, bindings, and DEFAULT VALUES for current prompts.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exact table cases to `internal/querybuilder/insert_sql_test.go`, using its mixed-choice and quoting fixtures plus the stale catalogs from Task 1. For dropped, hidden, generated, and non-insertable stored prompts, require `InsertSQL` to return an empty string and `InsertParams` to return nil so no stale identifier or former bound value escapes. Add current-state controls covering ordered Value bindings, SQL `NULL` without a parameter, Default/Omit exclusion, unusual quoted identifiers, submitted empty TEXT and typed `NULL`, and all-omit `DEFAULT VALUES` with no parameters. Assert report rejection before checking renderer output and keep the RED task production-free.

---

### 4. Enforce report-gated INSERT rendering

**Type**: GREEN  
**Output**: Stale INSERT state emits no SQL or parameters, and every valid INSERT rendering regression passes unchanged.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Harden `internal/querybuilder/insert_sql.go` only as needed so both `InsertSQL` and `InsertParams` use the strengthened authoritative report before traversing stored prompts. Preserve schema prompt order and the exact Value placeholder/binding, SQL NULL, omission, quoting, and all-omit `DEFAULT VALUES` behavior for accepted state. Do not silently filter a stale prompt and render the remainder, quote before validation, or duplicate insertability checks in the renderer; rejection must be all-or-nothing through `RunnableReport`. Run the querybuilder tests and keep connection/UI behavior outside this production change unless an existing adapter requires no-code verification.

---

### 5. Document stale INSERT prompt rejection

**Type**: DOCUMENT  
**Output**: Wiki documentation records current-insertability validation, all-or-nothing rendering, preserved choices/order, and DEFAULT VALUES behavior.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #67's implementation and tests from `internal/querybuilder/runnable.go`, `insert_sql.go`, and their focused tests into the appropriate `Notes/wiki` pages. Document that every stored prompt must still correspond to a current visible insertable column; dropped, hidden, generated, or otherwise non-insertable prompts produce typed INSERT-field stale feedback and no SQL or parameters. Record that accepted prompts preserve schema order, Value/NULL/Default choices, parameter order, omission, and all-omit `DEFAULT VALUES`. Cross-reference Issue #67 and the INSERT Query Grammar, schema metadata/revalidation, Runnable-State Contract, QueryBuilder Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update the wiki index and append the required dated log entry without rewriting prior entries.

---

### 6. Create the stale-INSERT walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/067-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/067-06/code-walkthrough`, with the main file named `walkthrough.md`. Build completed INSERT prompts, refresh each fixture so a prompted column is dropped, hidden, generated, or made non-insertable, and show the exact non-runnable INSERT-field feedback plus empty SQL and parameters. Prove no stale identifier is quoted or bound, then demonstrate unchanged mixed Value/NULL/Default order, typed parameters, quoted names, and all-omit `DEFAULT VALUES` for current prompts. Reference Issue #67 and `Notes/PRD-sqloid.md`, and place all showboat artifacts beneath the approved directory.

---
