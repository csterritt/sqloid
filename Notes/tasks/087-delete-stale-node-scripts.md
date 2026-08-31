# Tasks for #87: Delete stale Node project scripts

Parent issue: #87
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Remove the stale Node workflow scripts

**Type**: REFACTOR  
**Output**: The two foreign Node/Playwright helper scripts and active guidance references are absent, while Sqloid's Go, capability, sync, and archive workflows remain intact and green.  
**Depends on**: none

Delete exactly `scripts/run-all-tests.sh` and `scripts/set-up-for-wiki-fixes.sh`. Before deletion, search tracked repository guidance and workflow configuration for both filenames and for claims that their npm, Playwright, Wrangler, Mailpit, `src`, `tests`, or `e2e-tests` commands are supported Sqloid workflows; remove only active guidance references that would remain misleading after deletion, without rewriting historical issue, critique, PRD, task, walkthrough, or wiki ingest records that merely document prior findings. Leave `scripts/capability-suite.sh`, `scripts/pull-up-new.sh`, `scripts/tar-new.sh`, and any active task/watch helpers unchanged. Verify the deleted paths are no longer tracked, no active workflow invokes them, and the remaining scripts belong to current repository operations. Run `go test ./...`, `go build ./...`, `go vet ./...`, and `scripts/capability-suite.sh` as applicable; do not introduce a replacement Node script, alter production Go behavior, or broaden this cleanup beyond stale workflow artifacts.

---

### 2. Create the stale-script cleanup walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/087-02/code-walkthrough`.  
**Depends on**: 1

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/087-02/code-walkthrough`, with the main file named `walkthrough.md`. Show that `scripts/run-all-tests.sh` and `scripts/set-up-for-wiki-fixes.sh` are absent from tracked files and active guidance, summarize the foreign Node/Playwright infrastructure they previously targeted, and list the retained Sqloid scripts without modifying them. Present passing repository-wide test, build, and vet results plus the retained capability-suite result, and show that CI still invokes `scripts/capability-suite.sh`. Reference Issue #87 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
