## Issue 13: SELECT wildcard and COUNT(*) projection path

**Type**: AFK
**Blocked by**: Issue 9, Issue 10

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the empty SELECT Column(s) flow from **Query Grammar**: default-highlighted `*`, conditional bare `COUNT(*)`, and named-column continuation into the aggregate popup.

### How to verify

- **Manual**: Open empty Column(s), select `*`, reset, select `COUNT(*)`, then add and remove named entries.
- **Automated**: QueryBuilder and scripted popup tests assert ordering, visibility conditions, focus/reopen behavior, and that no unsupported aggregate-on-wildcard options appear.

### Acceptance criteria

- [ ] Given an empty projection, then `*` is first and default-selected and `COUNT(*)` is immediately second.
- [ ] Given `COUNT(*)` is selected, then it is added directly and Column(s) reopens without a named-column aggregate prompt; because the projection is now nonempty, the sentinel is hidden from the reopened popup.
- [ ] Given the projection becomes empty again, then `COUNT(*)` reappears; named columns always continue to aggregate selection.

### User stories addressed

- User story 23: Make wildcard the fastest SELECT projection
- User story 24: Offer conditional bare COUNT(*)
- User story 25: Restore COUNT(*) and route named columns through aggregates

---
