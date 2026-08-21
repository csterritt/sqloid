---
name: database-access
description: Since this is a web development project with a database that runs on a remote server, here are rules for writing code that accesses the database in this project.
---

- Always use the `withRetry`/`toResult` functions from `src/lib/db-helpers.ts` to access the database.
