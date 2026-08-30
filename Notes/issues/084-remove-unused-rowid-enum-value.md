## Issue 84: Remove the unused rowid enum value

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Align `schema.RowidCapability` with the PRD's three-value capability set and the project's other schema enums. Remove the unused `RowidApplicable` constant and type the constant block by declaring `RowidHas RowidCapability = iota + 1`, so the meaningful values carry the intended enum type and zero remains an unset sentinel. Preserve the existing string forms and catalog classification of has-rowid, without-rowid, and not-applicable objects.

### How to verify

- **Manual**: Inspect catalog output for an ordinary rowid table, a WITHOUT ROWID table, and a view, confirming only the three PRD capabilities are exposed and zero is not presented as a real capability.
- **Automated**: Update schema enum and catalog tests to assert the three nonzero values, their exact `String()` results, the zero/unknown diagnostic behavior, and unchanged classification for ordinary tables, WITHOUT ROWID tables, virtual tables, and views.

### Acceptance criteria

- [ ] Given the rowid-capability constants, then only has-rowid, without-rowid, and not-applicable are defined as meaningful values and each constant has type `RowidCapability`.
- [ ] Given a zero `RowidCapability`, then it remains an unset/unknown sentinel rather than an undocumented applicable state.
- [ ] Given schema catalog fixtures, then every object retains its existing correct capability and string representation.

### User stories addressed

- User story 20: Refresh eligible schema objects with correct capabilities
- User story 81: Revalidate rowid properties against refreshed schema metadata

---
