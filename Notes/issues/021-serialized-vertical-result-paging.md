## Issue 21: Serialized vertical result paging

**Type**: AFK
**Blocked by**: Issue 19, Issue 20

### Parent PRD

`PRD-sqloid.md`

### What to build

Add LIMIT/OFFSET Page Up/Down navigation with exactly one pending page request and an exact page size derived from complete visible data rows. Keep count as the only request allowed to coexist.

### How to verify

- **Manual**: Page forward/backward through a large fixture and press repeated/opposite Page keys while loading.
- **Automated**: Fake-Connection tests assert offsets, page sizes at supported heights, request serialization, ignored keys with feedback, at-most count-plus-one-page concurrency, and selected-aggregate ORDER BY in both ASC/DESC preserving the user expression without appending `rowid`.

### Acceptance criteria

- [ ] Given an idle active SELECT, when Page Up/Down is pressed, then exactly the required adjacent page is requested.
- [ ] Given a page is pending, then repeated or opposite Page keys start no request and show loading feedback.
- [ ] Given terminal height changes fixed rows, then the next request uses the exact number of complete visible data rows.
- [ ] Given a SELECT without user ORDER BY on an ordinary rowid table without a declared rowid shadow, then the page SQL appends `ORDER BY rowid` for stable paging.
- [ ] Given a SELECT without user ORDER BY on a view, virtual table, WITHOUT ROWID table, shadowed table, or aggregate/grouped query, then no implicit ordering is appended and no stability is implied.

### User stories addressed

- User story 49: Serialize paging and ignore stacked page keys
- User story 52: Compute exact page size from visible complete rows

---
