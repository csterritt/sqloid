## Issue 87: Delete stale Node project scripts

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Delete `scripts/run-all-tests.sh` and `scripts/set-up-for-wiki-fixes.sh`, which target absent Node, Playwright, Wrangler, Mailpit, `src`, `tests`, and `e2e-tests` infrastructure from another project. Keep `scripts/capability-suite.sh` and any active sync or archive helpers unchanged, and ensure repository guidance does not present the deleted scripts as valid Sqloid workflows.

### How to verify

- **Manual**: List the remaining scripts and confirm each belongs to the Go Sqloid repository; confirm the two stale script paths are absent and active capability/sync/archive helpers remain available.
- **Automated**: Search tracked files and workflow references for the deleted names and stale Node/Playwright test commands, then run the repository's Go test, vet, and build commands plus the retained capability-suite entry point as applicable.

### Acceptance criteria

- [ ] Given the `scripts` directory, then `run-all-tests.sh` and `set-up-for-wiki-fixes.sh` no longer exist.
- [ ] Given active Go, capability, sync, and archive workflows, then no required helper is deleted or changed by this cleanup.
- [ ] Given repository documentation and workflows, then none references either deleted script as a supported command.
- [ ] Given the standard Go verification commands, then the project remains green after the deletion.

### User stories addressed

- No direct user story: repository maintenance removes misleading, non-Sqloid developer workflows.

---
