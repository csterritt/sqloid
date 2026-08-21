---
name: running-the-server
description: Since this is a web development project, here are rules for running the server and running tests in this project. 
---

## server and test running

- run the server with one of the following commands, you cannot just run 'npm run dev':
  - npm run dev-open-sign-up
  - npm run dev-no-sign-up
  - npm run dev-gated-sign-up
  - npm run dev-interest-sign-up
- by default, run the server with open sign-up
- run the tests with the following command:
  - npx playwright test
  - you can add specific tests by naming them after the 'npx playwright test' command
  - you can have it stop at the first failure by adding the '-x' argument
- when running the tests, just run until the first test fails, and fix that problem.
  - if that fix applies to other tests, apply that fix to the other tests, then continue fixing one failure at a time
- when writing tests, make sure to look in the @e2e-tests/support folder for test helpers
  external references. If no input is provided, ask the user to share a
  file or diff before proceeding.
---

## server and test running

- run the server with one of the following commands, you cannot just run 'npm run dev':
  - npm run dev-open-sign-up
  - npm run dev-no-sign-up
  - npm run dev-gated-sign-up
  - npm run dev-interest-sign-up
- by default, run the server with open sign-up
- run the tests with the following command:
  - npx playwright test
  - you can add specific tests by naming them after the 'npx playwright test' command
  - you can have it stop at the first failure by adding the '-x' argument
- when running the tests, just run until the first test fails, and fix that problem.
  - if that fix applies to other tests, apply that fix to the other tests, then continue fixing one failure at a time
- when writing tests, make sure to look in the @e2e-tests/support folder for test helpers
