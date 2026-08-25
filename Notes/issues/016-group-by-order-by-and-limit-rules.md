## Issue 16: GROUP BY, ORDER BY, and LIMIT rules

**Type**: AFK
**Blocked by**: Issue 14, Issue 15

### Parent PRD

`PRD-sqloid.md`

### What to build

Complete SELECT construction and validation for GROUP BY multi-selection, context-valid ORDER BY with ASC/DESC, and bounded LIMIT, enforcing the grouping matrix in **Query Grammar** and **Runnable-State Contract**.

### How to verify

- **Manual**: Build grouped nonaggregate, mixed aggregate, all-aggregate, wildcard, invalid-limit, and valid grouped ordering examples.
- **Automated**: Pure QueryBuilder matrix tests assert candidates, SQL/params, duplicate prevention, direction, every valid/invalid grouping combination, and LIMIT cases for empty, one, max int64, zero, negative, malformed, and overflow input.

### Acceptance criteria

- [ ] Given GROUP BY, then every nonaggregate projection is grouped and wildcard is rejected.
- [ ] Given mixed aggregate/nonaggregate projection without GROUP BY, then it is invalid; an all-aggregate projection remains valid.
- [ ] Given ORDER BY, then only context-valid expressions are offered and ASC/DESC behavior follows the documented defaults and toggle rules.
- [ ] Given empty LIMIT input, then the query is unbounded; given an integer from 1 through 9,223,372,036,854,775,807, then that LIMIT is accepted.
- [ ] Given LIMIT input that is zero, negative, malformed, or above 9,223,372,036,854,775,807, then it is invalid and Issue 17 focuses Limit with the specific reason when execution is attempted.

### User stories addressed

- User story 29: Build GROUP BY, ORDER BY, and LIMIT interactively
- User story 30: Enforce complete grouping rules
- User story 31: Permit all-aggregate projection without GROUP BY

---
