# Tasks for #88: Run the full definition of done in CI

Parent issue: #88
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Expand CI into the full cross-platform release gate

**Type**: CONFIG  
**Output**: Linux and macOS CI run repository-wide test, build, and vet checks, the retained race/capability suite, and Issue #57's unattended built-binary/TUI integration test against production composition.  
**Depends on**: none

Begin only after Issues #57 through #87 have landed, and inspect their final code and verification entry points before editing CI. Update the sole workflow at `.github/workflows/capability-suite.yml` so both `ubuntu-latest` and `macos-latest` jobs perform clean checkout and Go setup, then fail closed on `go test ./...`, `go build ./...`, and `go vet ./...`. Retain the exact modernc pin check and `scripts/capability-suite.sh` invocation from Issue #56 as a separate targeted race/cancellation capability gate rather than replacing it with ordinary repository tests. Locate the production built-binary/TUI integration test delivered by Issue #57 and invoke that existing test or canonical harness on both platforms after building the actual `sqloid` binary; configure its pseudo-terminal or Bubble Tea harness for unattended headless execution, real temporary SQLite fixtures, deterministic timeouts, captured diagnostics, and nonzero failure on absent or bypassed production composition. Do not create a duplicate integration test, substitute package-local fakes, skip supported platforms, use `continue-on-error`, or weaken the targeted capability suite. Ensure the workflow exercises every shipped package and that a test, build, vet, capability, binary-integration, setup, timeout, or cleanup failure blocks merging.

---

### 2. Document the complete CI definition of done

**Type**: DOCUMENT  
**Output**: The wiki records every Linux/macOS release-gate command, the retained specialized suite, the Issue #57 production-composition path, and failure/diagnostic expectations.  
**Depends on**: 1

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #88 workflow and the final Issue #57 integration-test entry point into the appropriate release and testing pages under `Notes/wiki`. Document both supported runner jobs, exact repository-wide `go test ./...`, `go build ./...`, and `go vet ./...` commands, the unchanged exact-pin check and targeted `scripts/capability-suite.sh` race guarantees, and the command that builds and drives the production `sqloid` binary headlessly through its real SQLite/TUI composition. Record ordering, timeouts, captured diagnostics, temporary-fixture cleanup, and fail-closed semantics, and explain which partial-package regression classes the expanded gate now catches. Cross-reference Issues #56-#57 and #81-#88, the Language and stack, Connection pool, Session health, History, Module Design, Testing Decisions, and Acceptance Criteria sections of `Notes/PRD-sqloid.md`, and the existing release-capability documentation. Update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 3. Create the full CI gate walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/088-03/code-walkthrough`.  
**Depends on**: 2

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/088-03/code-walkthrough`, with the main file named `walkthrough.md`. Present the final Linux and macOS job definitions and retained exact modernc pin check, then show evidence for repository-wide test, build, and vet commands, the unchanged targeted race/capability suite, and Issue #57's existing built-binary/TUI integration path running unattended against a real SQLite fixture through production adapters. Demonstrate from retained CI output that all shipped packages participate, both platforms run equivalent required gates, useful diagnostics are captured, and no allowed-failure or skip path hides failure. Include one controlled or clearly explained negative demonstration showing that bypassing production composition or failing any required command makes the workflow non-green. Reference Issues #56-#57 and #88 plus `Notes/PRD-sqloid.md`, and keep every showboat-generated artifact under the approved directory.

---
