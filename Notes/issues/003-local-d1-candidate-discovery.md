## Issue 3: Local D1 candidate discovery

**Type**: AFK
**Blocked by**: Issue 1, Issue 2

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement `sqloid d1` discovery exactly within the working-directory-relative Wrangler path defined in **D1 discovery**. Apply the case-sensitive candidate, metadata, and sidecar rules without recursive or alternate-layout searches. Pass the sole candidate path to Issue 2's shared validation/read-write opening path; do not implement a separate D1-specific opener.

### How to verify

- **Manual**: Run `sqloid d1` from fixtures containing one valid candidate and mixtures of ignored files.
- **Automated**: Table-driven filesystem tests cover case-sensitive extensions, metadata substrings, WAL/SHM sidecars, nested files, and exactly-one selection.

### Acceptance criteria

- [ ] Given exactly one eligible candidate in the documented directory, when `sqloid d1` runs, then that database is opened.
- [ ] Given metadata, sidecar, wrong-case, or nested files, then they are not candidates.
- [ ] Given an alternate Wrangler layout, then Sqloid does not search it.

### User stories addressed

- User story 4: Discover local Wrangler D1 state
- User story 5: Apply exact candidate and exclusion rules

---
