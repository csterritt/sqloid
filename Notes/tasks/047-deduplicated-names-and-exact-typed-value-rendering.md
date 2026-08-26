# Tasks for #47: Finalize shared typed rendering for exporters

Parent issue: #47
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify exporter-facing name and value contracts

**Type**: RED
**Output**: Failing consumer-contract tests cover full-set collisions, finite REAL precision/locale edges, typed values, and unchanged driver metadata/SQL.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend shared seam tests in `internal/result` and add consumer-contract tests at the `internal/export` boundary while retaining focused grid assertions in `internal/ui`. Require one full-set, deterministic output-name calculation for empty, duplicate, pre-suffixed, and recursively colliding labels, and prove grid and exporter-facing consumers receive exactly the same final names in column order. Cover finite REAL shortest-round-trip tokens for `1.0`, `-0.0`, `1e+20`, subnormals, adjacent precision edges, and integral-valued floats under non-default locale conditions; assert REAL identity is retained and identical-looking INTEGER/TEXT values remain typed distinctly. Verify obtaining shared names/tokens never mutates, replaces, or writes deduplicated labels back into driver column metadata, query SQL, or immutable source values. Keep this task test-only, use Issue #22's existing package-shaped implementation as the sole reference, and do not create CSV/JSON format serialization owned by Issues #50 and #51.

---

### 2. Harden shared names and finite-REAL APIs

**Type**: GREEN
**Output**: Grid and exporter consumers use the same names/tokens without moving or copying Issue 22 logic.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Harden the Issue #22 APIs in place under `internal/result` so `internal/ui` and the exporter-facing `internal/export` boundary consume the same full-set output names, typed values, and finite-REAL tokens. Expose stable UI-independent contracts without relocating algorithms or adding wrappers that reimplement collision handling or numeric formatting. Keep original driver metadata, SQL, column order, and stored values immutable; calculate consumer names separately and retain REAL type even when the canonical token requires `.0` or preserves negative zero. Migrate only private consumer call sites necessary to make Task 1 pass, and do not introduce CSV/JSON quoting, non-finite exporter policy, or a second formatting implementation.

---

### 3. Specify normalized TEXT/BLOB and warning metadata

**Type**: RED
**Output**: Failing tests cover maximal invalid sequences, U+FFFD metadata, exact BLOB bytes, NULL/empty distinction, controls, and format-policy inputs.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exact-byte and typed consumer-contract tests in `internal/result` and `internal/export`, with grid compatibility checks in `internal/ui`. Feed valid UTF-8 and maximal invalid UTF-8 sequences including isolated continuation bytes, truncated multibyte prefixes, overlong encodings, surrogate encodings, and adjacent invalid runs; require one U+FFFD per maximal invalid sequence plus structured warning metadata that exporters can aggregate without reparsing text. Require BLOBs containing the same bytes to remain byte-for-byte unchanged, including empty, NUL, high-byte, and invalid-UTF payloads. Distinguish SQL NULL, empty TEXT, and empty BLOB; retain tabs, newlines, carriage returns, NUL, and other controls in normalized TEXT while exposing both raw typed policy inputs and Issue #22's visible grid rendering. Assert no consumer infers type from display text and keep this task test-only without defining CSV or JSON escaping.

---

### 4. Finalize typed result representation

**Type**: GREEN
**Output**: Shared values/metadata expose all CSV/JSON inputs while preserving grid behavior and BLOB identity.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Finalize the shared typed representation and metadata in `internal/result`, and expose it unchanged through `internal/export` while keeping `internal/ui` on the same seam. Preserve explicit SQLite NULL, INTEGER, REAL, TEXT, and BLOB identities; retain exact BLOB byte ownership through safe immutable copies; normalize invalid TEXT with one U+FFFD per maximal invalid sequence; and carry structured warning metadata indicating replacement occurred. Make normalized text with controls, canonical finite-REAL tokens, original typed values, collision-safe names, and warning information available as format-policy inputs so later CSV/JSON code need not inspect grid strings. Preserve Issue #22's visible control-character and `[BLOB n bytes]` grid behavior and Issue #23's non-finite grid behavior, and implement only enough to make Task 3 pass without adding format-specific serialization.

---

### 5. Add architecture duplication checks

**Type**: REFACTOR
**Output**: Grid remains on the shared seam and checks prevent grid/exporter-private copies of names, REAL tokens, or UTF normalization.
**Depends on**: 4

Refactor `internal/result`, `internal/ui`, and `internal/export` without changing observable behavior so all name deduplication, finite-REAL token generation, and invalid-UTF normalization retain one definition site under the shared result seam. Remove any now-redundant consumer glue that reproduces those policies, narrow APIs around typed values and metadata, and keep format-specific decisions outside the shared module. Add focused architecture or consumer-equivalence checks that fail if the grid or exporter boundary introduces private collision suffixing, numeric token formatting, or UTF replacement logic, while allowing later CSV/JSON escaping and non-finite policies at their proper format boundaries. Run the affected package tests and preserve driver metadata, SQL, grid output, BLOB identity, and warning behavior exactly.

---

### 6. Document shared rendering APIs

**Type**: DOCUMENT
**Output**: Wiki documentation records contracts, metadata, consumers, format-specific boundaries, and definition sites.
**Depends on**: 5

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #47 implementation and tests from `internal/result`, `internal/ui`, and `internal/export` into the appropriate pages under `Notes/wiki`. Document full-set collision-safe output names, exact finite-REAL tokens and retained type, maximal-invalid-sequence U+FFFD normalization with warning metadata, exact BLOB identity, NULL/empty distinctions, and control-character policy inputs. Identify the shared `internal/result` definition sites and the grid/exporter-facing consumers, explain that driver metadata and SQL remain unchanged, and distinguish shared representation from grid display and the later CSV/JSON escaping and non-finite policies owned by Issues #50 and #51. Cross-reference Issues #22, #23, and #47 plus Numeric value parsing and rendering, Grid rendering/cache, Export formats and values, Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 7. Create the shared-rendering walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/047-07/code-walkthrough`.
**Depends on**: 6

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/047-07/code-walkthrough`. Exercise recursive full-set label collisions and show identical grid/exporter-facing names without changing driver metadata or SQL. Demonstrate finite REAL identity and exact locale-independent tokens across integral, negative-zero, exponent, subnormal, and precision-edge values alongside non-finite and identical-looking typed values. Cover SQL NULL, empty TEXT/BLOB, controls, maximal invalid UTF-8 replacement and warning metadata, and byte-for-byte BLOB preservation. Include package-level evidence that `internal/result` owns each shared policy, `internal/ui` remains a consumer, `internal/export` receives format-policy inputs, and no consumer-private copies or CSV/JSON serialization were introduced. Reference Issue #47 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 8. Review typed rendering

**Type**: REVIEW
**Output**: Human verifies duplicate labels, finite/non-finite values, NULL/empty, controls, invalid UTF-8, and BLOB identity.
**Depends on**: 7

Review the shared representation and definition sites in `internal/result`, grid consumption in `internal/ui`, exporter-facing contracts in `internal/export`, the wiki updates, and `Notes/walkthroughs/047-07/code-walkthrough` against Issue #47. Manually inspect recursive duplicate labels, finite REAL precision/locale edges, non-finite REALs, identical-looking typed values, SQL NULL versus empty TEXT/BLOB, controls, maximal invalid UTF-8 sequences and warning metadata, and exact BLOB bytes. Confirm grid behavior remains unchanged, exporter-facing consumers receive the same names/tokens and complete typed policy inputs, driver metadata and SQL are untouched, and no duplicate name, REAL, or UTF implementation exists before approving the issue.

---
