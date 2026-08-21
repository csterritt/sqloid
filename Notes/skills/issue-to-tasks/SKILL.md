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

Break the issue into ordered tasks. Each task must:

- Be completable in a single AI session (one focused prompt exchange)
- Have a clear, verifiable output (a file, a passing test, a working endpoint)
- Follow the dependency order: schema before logic, logic before API, API before UI, tests alongside or immediately after each layer

Label each task with its type:

- **RED**: create tests that must fail before any code is written to make them pass.
- **GREEN**: create or modify just enough production code to make the tests pass.
- **REFACTOR**: improve existing code without changing behavior
- **MIGRATE**: schema or data migration
- **CONFIG**: environment, tooling, or infrastructure change
- **DOCUMENT**: update docs, READMEs, the wiki in Notes/wiki (see Notes/wiki/wiki-rules.md for details) or other non-code artifacts
- **CODE WALKTHROUGH**: Using showboat (run `uvx showboat --help` for details) create a walkthrough of the implementation, making a new directory under Notes/walkthroughs named {{TASK-ID}}/code-walkthrough, and put the files it generates there.
- **REVIEW**: human decision required before proceeding

Write the DOCUMENT and CODE WALKTHROUGH tasks after all code is written, just before the final REVIEW step.

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

## Tasks

### <n>. <Task title>

**Type**: RED / GREEN / REFACTOR / MIGRATE / CONFIG / DOCUMENT / CODE WALKTHROUGH / REVIEW  
**Output**: <what exists or passes when done>  
**Depends on**: <task numbers or "none">

<For RED and GREEN tasks include a short paragraph asking the AI to read and follow the coding standards in Notes/skills/AGENTS.md>

<A short paragraph describing exactly what to do. Written as an instruction to the AI that will execute it. Include: which files to touch, which pattern to follow, which existing code to use as reference. Do NOT include code snippets — describe intent, not implementation.>

---

</task-file-template>

Do NOT modify the parent issue or the parent PRD.
