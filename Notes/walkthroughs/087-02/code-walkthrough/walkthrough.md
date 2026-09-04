# Issue #87 Task 2: Stale Node Script Cleanup Walkthrough

*2026-09-04T01:37:38Z by Showboat 0.6.1*
<!-- showboat-id: 9a591201-82b5-4809-9ded-b0a4b1fafb10 -->

## Overview

This walkthrough documents the cleanup performed for Issue #87 (Delete stale Node project scripts), which removes two foreign Node/Playwright helper scripts that referenced infrastructure absent from this Go repository. The cleanup ensures Sqloid's Go, capability, sync, and archive workflows remain intact and green.

References:
- Issue #87: `Notes/issues/087-delete-stale-node-scripts.md`
- PRD: `Notes/PRD-sqloid.md`
- Task spec: `Notes/tasks/087-delete-stale-node-scripts.md`

## What was deleted

Two scripts were removed from `scripts/`:

1. **`scripts/run-all-tests.sh`** — a 192-line bash script that ran Playwright e2e tests across five sign-up modes by starting `wrangler dev` and `mailpit` servers, then invoking `npx playwright test e2e-tests`. This targeted a Node/Cloudflare Worker project (`src/`, `tests/`, `e2e-tests/`, `npm`, `playwright`, `wrangler`, `mailpit`) that does not exist in this Go repository.

2. **`scripts/set-up-for-wiki-fixes.sh`** — a 6-line bash script that generated `Notes/_file-checklist.md` by listing files under `src/`, `tests/`, and `e2e-tests/` — directories that do not exist in this Go repository.

Both scripts were not invoked by any active workflow (CI or local) and would fail if run.

## Active guidance cleaned up

The wiki schema file `Notes/wiki/AGENTS.md` contained stale references inherited from a prior Node project ("expense-log"): it claimed the wiki covered an "expense-log" Cloudflare Worker app, listed `src/`, `e2e-tests/`, `tests/` as directories to ingest, referenced a non-existent `e2e-tests.md` wiki page, and mentioned `node_modules`. These were corrected to reflect the actual Sqloid Go project structure (`cmd/` and `internal/` for source, Go `*_test.go` files for tests).

Historical records (issues, critiques, PRD, tasks, walkthroughs, wiki ingest log) were left unchanged — they document prior findings and are not active guidance.

```bash
result="$(git ls-files scripts/run-all-tests.sh scripts/set-up-for-wiki-fixes.sh)"; if [ -z "$result" ]; then echo 'NOT TRACKED (expected)'; else echo "FOUND (bad): $result"; fi
```

```output
NOT TRACKED (expected)
```

```bash
ls scripts/run-all-tests.sh scripts/set-up-for-wiki-fixes.sh 2>&1 || true
```

```output
ls: cannot access 'scripts/run-all-tests.sh': No such file or directory
ls: cannot access 'scripts/set-up-for-wiki-fixes.sh': No such file or directory
```

## Retained Sqloid scripts

The remaining scripts in `scripts/` belong to current repository operations and were left unchanged:

```bash
git ls-files scripts/
```

```output
scripts/build-tasks-until-review.ts
scripts/capability-suite.sh
scripts/json-watch.ts
scripts/pull-up-new.sh
scripts/remove-non-hitl-review-blocks.ts
scripts/tar-new.sh
```

These six scripts are the retained Sqloid workflow artifacts:

- `scripts/capability-suite.sh` — the canonical release-capability suite gate (Issue #56), invoked by CI.
- `scripts/pull-up-new.sh` — sync helper for pulling up new content.
- `scripts/tar-new.sh` — archive helper for creating tarballs.
- `scripts/build-tasks-until-review.ts`, `scripts/json-watch.ts`, `scripts/remove-non-hitl-review-blocks.ts` — task/watch helpers for the Notes workflow.

None of these reference the foreign Node/Playwright infrastructure.

## No active workflow invokes the deleted scripts

The only CI workflow is `.github/workflows/capability-suite.yml`, which invokes `scripts/capability-suite.sh` — not the deleted scripts. A search of `.github/` confirms no reference to either deleted filename:

```bash
grep -rn 'run-all-tests\|set-up-for-wiki-fixes' .github/ 2>&1; echo "exit: $?"
```

```output
exit: 1
```

The grep exit code 1 confirms no matches — no CI workflow references the deleted scripts.

## CI still invokes `scripts/capability-suite.sh`

The capability-suite CI workflow (Issue #56) runs on both Linux and macOS from a clean checkout:

```bash
grep -n 'capability-suite.sh' .github/workflows/capability-suite.yml
```

```output
6:# command (scripts/capability-suite.sh) from a clean checkout with the
40:        run: scripts/capability-suite.sh
61:        run: scripts/capability-suite.sh
```

## Verification: go build, go vet, go test

Repository-wide build, vet, and test results after the cleanup:

```bash
go build ./... 2>&1 && echo 'BUILD OK'
```

```output
BUILD OK
```

```bash
go vet ./... 2>&1 && echo 'VET OK'
```

```output
VET OK
```

```bash
go test ./... 2>&1
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/070-06/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/085-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/086-02/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

## Verification: capability-suite.sh

The retained canonical capability suite (Issue #56) passes with the race detector enabled:

```bash
scripts/capability-suite.sh 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/connection	75.355s
ok  	github.com/chris/sqloid/internal/ui	6.687s
ok  	github.com/chris/sqloid/internal/history	2.235s
```

## Summary

- **Deleted**: `scripts/run-all-tests.sh` and `scripts/set-up-for-wiki-fixes.sh` — two stale Node/Playwright helper scripts referencing absent `src/`, `tests/`, `e2e-tests/`, `npm`, `playwright`, `wrangler dev`, and `mailpit` infrastructure.
- **Active guidance cleaned**: `Notes/wiki/AGENTS.md` updated to remove stale `expense-log` project references, `src/`/`e2e-tests/`/`tests/` directory listings, the non-existent `e2e-tests.md` wiki page reference, and the `node_modules` mention — replaced with accurate Sqloid Go project structure (`cmd/` and `internal/` for source, Go `*_test.go` files for tests).
- **Historical records preserved**: Issues, critiques, PRD, tasks, prior walkthroughs, and wiki ingest log entries were left unchanged — they document prior findings, not active guidance.
- **Retained scripts unchanged**: `scripts/capability-suite.sh`, `scripts/pull-up-new.sh`, `scripts/tar-new.sh`, `scripts/build-tasks-until-review.ts`, `scripts/json-watch.ts`, `scripts/remove-non-hitl-review-blocks.ts` — all belong to current repository operations.
- **CI intact**: `.github/workflows/capability-suite.yml` still invokes `scripts/capability-suite.sh` on both Linux and macOS.
- **All verification green**: `go build ./...`, `go vet ./...`, `go test ./...`, and `scripts/capability-suite.sh` all pass.
- **No replacement Node script introduced**; no production Go behavior altered; cleanup scoped strictly to stale workflow artifacts.

Issue #87: `Notes/issues/087-delete-stale-node-scripts.md`
PRD: `Notes/PRD-sqloid.md`
