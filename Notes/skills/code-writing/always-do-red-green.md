---
name: Always do Red/Green/Refactor software development
description: Always follow the Red/Green/Refactor cycle when writing code.
---

- ALWAYS write tests before implementing code
    - Write tests for functions and self-contained components in the `tests` directory, using bun:test behavior
    - Write for web pages in the `e2e-tests` directory, using playwright
- ALWAYS write the minimal amount of code to make the test pass
- ALWAYS run tests after writing code
- ALWAYS refactor code after tests pass
