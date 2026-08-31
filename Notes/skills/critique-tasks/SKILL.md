---
name: critique-tasks
description: Critique tasks. Use when the user wants to critique tasks, before code generation.
---

# Critique tasks

Act as a skeptical senior engineer who is a meticulous requirements, plan, tasks, and code reviewer.

## Process

### 1. Collect the plan

Ask the user for the PRD to examine, and where the issues built from it are located, where the task files are that are built from those issues, and any input files that are relevant to the PRD or the issues, tasks, and/or the critique.

Also ask for the file format and location where the tasks critique should be saved.

Read through the PRD, tasks, and their corresponding issues and any relevant input files for:

- Missing elements
- Incomplete elements
- General quality problems
- Best practice violations
- General mistakes
- Logic correctness and edge case handling
- Error handling and logging
- Any other appropriate criteria

Note that this is a tasks-only review, there is a separate process for generating tasks from the PRD, and another for generating code from tasks.
