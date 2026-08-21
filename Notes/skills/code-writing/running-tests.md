---
name: running-tests
description: How to run both unit and end-to-end tests for the project.
---

## Unit Tests

To run unit tests, use the following command at the root of the project:

```bash
bun test tests
```

## End-to-End Tests

To run end-to-end tests, use the following command:

```bash
npx playwright test --reporter=line e2e-tests
```
