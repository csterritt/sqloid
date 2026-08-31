---
name: issues-to-tasks
description: Break an issue file into concrete, ordered, AI-executable tasks. Use when the user wants to implement an issue, start work on a ticket, or break down an issue into smaller steps.
---

# Issues to Tasks

Break a single vertical-slice issue into concrete, ordered tasks that can each be completed in one focused AI session.

## Process

### 1. Locate the issue

Ask the user for the issue. If not provided, ask for the issue number or URL or file.
Read the parent PRD issue referenced in the "Parent PRD" field.

### 2. Explore the codebase

Explore the parts of the codebase touched by this issue. Focus on:

- Files and modules that will be created or modified
- Existing patterns to follow (naming conventions, error handling, test structure)
- Any interfaces or contracts this issue must respect

### 3. Draft the task list

Break the issue into ordered tasks. Copy the parent issue's cross-issue prerequisites into the structured `Blocked by issues` header; `Depends on` is reserved for sibling task ordinals. Add an explicit AC-to-task map and name the task that owns manual verification so schedulers do not have to infer either relationship from prose. Standardize repeated gate wording as `Begin only after Issue #X is complete.`

Each task must:

- Be completable in a single AI session (one focused prompt exchange)
- Have a clear, verifiable output (a file, a passing test, a working endpoint)
- Follow the dependency order: schema before logic, logic before API, API before UI, tests alongside or immediately after each layer
- Preserve cross-issue ordering on shared files or functions; explicitly sequence otherwise-independent issues that edit the same seam

Label each task with exactly one of these types and satisfy its verification obligation:

- **RED**: add an externally observable contract test that fails for the intended reason before production changes. Do not use private source/AST shape as the primary RED gate; when behavior cannot expose a cleanup requirement, specify a separate lint/build check instead.
- **GREEN**: create or modify only enough production code to make its preceding RED contract pass, then run the focused and repository-standard verification.
- **REFACTOR**: improve code without changing behavior. Begin from an already-green behavioral safety net, run it unchanged after the edit, and use lint/build/source checks only as supplemental evidence for structural cleanup.
- **MIGRATE**: apply a schema or data migration and verify both migration correctness and the supported upgrade/rollback contract.
- **CONFIG**: change environment, tooling, or infrastructure; validate syntax/configuration, execute the affected gate where possible, and include a fail-closed negative check when the configuration enforces a release requirement.
- **DOCUMENT**: update docs, READMEs, the wiki in `Notes/wiki` (see `Notes/wiki/wiki-rules.md`), or other non-code artifacts; verify links, terminology, and claimed commands against the implementation.
- **CODE WALKTHROUGH**: use showboat (run `uvx showboat --help` for details) to create evidence under `Notes/walkthroughs/{{TASK-ID}}/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate the acceptance contracts and retain verification output. For a documentation-only or mechanical no-behavior-change issue, a lightweight walkthrough containing the changed artifact plus focused verification output is sufficient; do not require a full interactive product demonstration.
- **REVIEW**: require a recorded human decision before proceeding; use only for HITL issues.

Write DOCUMENT and CODE WALKTHROUGH tasks after implementation, just before the final REVIEW step (if any). RED must pair with GREEN when production behavior changes. A REFACTOR instead starts from existing behavioral coverage and must not be presented as the GREEN half of a structural-test-only RED/REFACTOR cycle.

### 4. Quiz the user

Present the proposed task list as a numbered list. For each task show:

- **Title**: short imperative description (e.g. "Add `user_id` column to `sessions` table")
- **Type**: RED / GREEN / REFACTOR / MIGRATE / CONFIG / DOCUMENT / CODE WALKTHROUGH / REVIEW
- **Output**: what exists or passes when this task is done
- **Depends on**: task numbers that must complete first

Ask the user:

- Does the order feel right?
- Are any tasks too large to complete in one session?
- Are any tasks so small they should be merged?
- Are all REVIEW tasks correctly identified?

Iterate until the user approves the list.

### 5. Write the task file

Save the approved task list to a file in the format and location specified by the user.

Use the task file template below.

<task-file-template>
# Tasks for #<issue-number>: <issue-title>

Parent issue: #<issue-number>
Parent PRD: #<prd-issue-number>
**Blocked by issues**: <issue IDs or "none">
**Acceptance criteria**: <AC-to-task map>
**Manual verification**: <state which task owns it; for unattended work, normally the final walkthrough after the production composition prerequisite is available>

## Tasks

### <n>. <Task title>

**Type**: RED / GREEN / REFACTOR / MIGRATE / CONFIG / DOCUMENT / CODE WALKTHROUGH / REVIEW  
**Output**: <what exists or passes when done>  
**Depends on**: <sibling task numbers or "none">

<For RED and GREEN tasks include a short paragraph asking the AI to read and follow the coding standards in Notes/skills/AGENTS.md>

<A short paragraph describing exactly what to do. Written as an instruction to the AI that will execute it. Include: which files to touch, which pattern to follow, which existing code to use as reference. Do NOT include code snippets — describe intent, not implementation.>

---

</task-file-template>

Do NOT modify the parent issue or the parent PRD.
