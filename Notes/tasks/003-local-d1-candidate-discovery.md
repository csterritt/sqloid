# Tasks for #3: Local D1 candidate discovery

Parent issue: #3
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify Wrangler candidate discovery

**Type**: RED  
**Output**: Failing filesystem tests cover the exact relative path, case-sensitive `.sqlite`, lowercase `metadata` exclusion, `-wal`/`-shm` exclusion, nested files, alternate layouts, and cardinality.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven filesystem tests in `internal/d1` for Issue #3 and the D1 discovery section of `Notes/PRD-sqloid.md`. Require discovery only in the working-directory-relative `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` directory, an exact case-sensitive `.sqlite` extension, exclusion of names containing lowercase `metadata` and `-wal` or `-shm` sidecars, no nested or alternate-layout search, and explicit zero-, one-, and multiple-candidate cardinality outcomes.

---

### 2. Implement nonrecursive D1 discovery

**Type**: GREEN  
**Output**: Discovery tests pass and return the sole candidate or typed zero/multiple outcomes.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the smallest deterministic filesystem discovery API in `internal/d1` that satisfies Issue #3 and its PRD contract. Inspect only immediate entries in the exact Wrangler directory, preserve the exact case-sensitive `.sqlite` and exclusion rules, return the sole eligible path unchanged, and represent zero and multiple candidates as typed outcomes for `internal/cli` without opening SQLite or taking ownership from `internal/connection`.

---

### 3. Specify integration with the shared opener

**Type**: RED  
**Output**: Failing CLI test proves the sole discovered path is passed to Issue 2's opener without a D1-specific opening path.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add a focused `internal/cli` integration test for the successful `sqloid d1` path, using a mixed filesystem fixture to prove the exact sole candidate from `internal/d1` is passed unchanged to Issue #2's `internal/connection` validation and read-write opener. Assert successful startup remains silent, `cmd/sqloid` remains only the entrypoint, ignored `.sqlite` sidecars and metadata files do not affect selection, and no D1-specific validation or SQLite-opening path is introduced.

---

### 4. Wire D1 discovery into CLI startup

**Type**: GREEN  
**Output**: End-to-end `sqloid d1` success opens the discovered database through the shared validation path.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire the `d1` command in `internal/cli` to request the sole path from `internal/d1` and pass it unchanged to the shared `internal/connection` pre-open validation, read-write open, and schema-probe flow from Issue #2. Keep `cmd/sqloid` as the thin executable entrypoint, preserve silent success, and avoid duplicating candidate filtering, validation, opening, probing, or diagnostic responsibilities across packages.

---

### 5. Document local D1 discovery rules

**Type**: DOCUMENT  
**Output**: Wiki documentation records the exact path and candidate/exclusion rules.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #3 implementation and tests into the appropriate pages under `Notes/wiki`. Document the exact working-directory-relative Wrangler path, nonrecursive search, case-sensitive `.sqlite` rule, lowercase `metadata` and `-wal`/`-shm` exclusions, cardinality behavior, and the handoff from `internal/d1` through `internal/cli` to `internal/connection`; update the wiki index and append-only log as required.

---

### 6. Create the D1 discovery walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough demonstrates discovery and ignored files at `Notes/walkthroughs/003-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/003-06/code-walkthrough`. Demonstrate `internal/d1` filtering and the successful `internal/cli` to `internal/connection` handoff with one exact `.sqlite` candidate plus lowercase metadata, sidecar, wrong-case, nested, and alternate-layout files that must be ignored, referencing Issue #3 and `Notes/PRD-sqloid.md`.

---
