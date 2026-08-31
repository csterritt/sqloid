# Tasks for #77: Distinguish unseen low endpoints from truncation

Parent issue: #77
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify low-endpoint completeness semantics

**Type**: RED  
**Output**: Failing history truth-table tests distinguish unseen low endpoints from eviction, allow empty completion without ReachedLow, and preserve truthful mixed labels.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

As the first change in the shared Issue #77 → #78 → #79 → #80 `history.Classify`/`TraversalFacts` sequence, extend the table-driven matrix in `internal/history/snapshot_classify_test.go`. Add a settled nonempty retained range such as positions 11–20 with known total 20, `ReachedHigh=true`, `ReachedLow=false`, no cap eviction, and finished work; require partial only, not truncated. Add successful empty known-total and observed-short-empty cases with `high == 0`, `ReachedLow=false`, finished work, and no contradictory evidence; require exclusive complete. Cover actual low- and high-side row/byte eviction, known rows beyond the retained end, unknown count/page work, nonempty missing-low with and without independent truncation evidence, and combinations where partial and truncated truthfully coexist. Assert complete for nonempty data still requires both established endpoints and full limited-result retention. Keep this task test-only and do not edit classification logic.

---

### 2. Correct low-endpoint classification

**Type**: GREEN  
**Output**: `history.Classify` reports partial for an unseen nonempty low endpoint, complete for a fully settled empty result, and truncated only from actual overflow/eviction evidence.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update only the necessary logic and explanatory comments in `internal/history/snapshot_classify.go`, preserving the existing pure classifier and immutable inputs. Compute complete from established high evidence, finished work, no eviction/inconsistency, full limited-result retention, and the low-endpoint condition `high == 0 || meta.ReachedLow`; an empty logical result is vacuously retained without a low row to observe. For every nonempty incomplete result, treat `!meta.ReachedLow` as partial evidence that lower rows may be unseen. Keep truncated limited to row-cap or byte-cap eviction and known/observed rows outside the retained range, never using a missing low endpoint alone as truncation. Preserve count/cache contradictions without clamping, unknown-work partial behavior, exclusive complete, and truthful partial-plus-truncated coexistence. Implement only enough to make Task 1 pass, then run focused `internal/history` and dependent `internal/ui` snapshot/export tests plus established Go verification.

---

### 3. Document low-endpoint classification

**Type**: DOCUMENT  
**Output**: Wiki documentation distinguishes unseen low endpoints, empty-result completion, actual truncation evidence, and coexisting incomplete labels.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #77 implementation and truth table into the existing snapshot metadata documentation under `Notes/wiki` and any directly affected export/history pages. State that a nonempty missing low endpoint means unseen lower rows may remain and is therefore partial, not truncated by itself; empty `high == 0` results need no observed low row when work and retention facts otherwise establish completion; and nonempty complete results require both endpoints plus the full limited logical range. Enumerate the only truncation evidence—row/byte eviction or known/observed rows outside retention—and explain when partial and truncated coexist. Cross-reference Issue #77, user stories 55 and 56, and the cache and snapshot invariant in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if necessary and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the low-endpoint classification walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/077-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/077-04/code-walkthrough`, with the main file named `walkthrough.md`. Exercise the classifier with retained positions 11–20 of known total 20 and no low endpoint to show partial rather than truncated, then show known-total and observed-short empty results completing with `ReachedLow=false`. Contrast actual low/high eviction, known overflow, unknown work, and mixed missing-endpoint plus eviction evidence, including partial-plus-truncated output. Include focused passing truth-table output, reference Issue #77 and `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---
