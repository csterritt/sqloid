# Tasks for #74: Correct invalid UTF-8 maximal-subpart decoding

Parent issue: #74
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify Unicode maximal-subpart decoding cases

**Type**: RED  
**Output**: Failing result-decoder tests pin valid U+FFFD preservation, exact E0–EF and F0–F4 maximal subparts, adjacent malformed sequences, metadata, and unchanged BLOB bytes.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the table-driven decoder coverage in `internal/result/result_test.go`, following `TestDecodeTextMaximalInvalidSequences`, `TestPageInvalidUTFMetadataWithoutRowChange`, and the existing BLOB identity tests. Include a valid encoded U+FFFD before and after unrelated malformed bytes and require the valid rune to remain unchanged. Exhaust the E0–EF lead-byte classes with valid constrained second bytes followed by invalid or missing third bytes, and the F0–F4 classes with one, two, or three valid continuation-prefix bytes followed by an invalid or missing later byte; include boundary second-byte constraints, adjacent malformed subparts, malformed sequences followed by valid text, and fully valid controls. Assert exact decoded bytes and replacement count by expected output, require `DecodeText`/`FromDriver` invalid-UTF metadata only when malformed input was replaced, and prove `[]byte` BLOB payloads remain byte-for-byte unchanged and never set text warnings. Keep this task test-only and do not alter `maximalSubpart` yet.

---

### 2. Correct the shared TEXT decoder

**Type**: GREEN  
**Output**: The shared decoder emits one U+FFFD per maximal invalid sequence, preserves valid U+FFFD, and keeps grid/export metadata and BLOB handling unchanged.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Correct `DecodeText` and `maximalSubpart` in `internal/result/result.go` using the existing single shared conversion boundary consumed by grid, CSV, and JSON. Make the helper recognize the full valid continuation prefix of three-byte E0–EF and four-byte F0–F4 sequences, returning the maximal ill-formed subpart length when a later byte is invalid or missing while preserving Unicode lead/second-byte constraints against overlong encodings, surrogates, and code points above U+10FFFF. Do not post-process replacement-rune bytes or confuse a valid encoded U+FFFD with decoder-inserted U+FFFD; retain the boolean replacement signal only for malformed input. Leave `FromDriver` TEXT routing, `Page.InvalidUTF`, serializers, grid rendering, and BLOB copying unchanged except as they consume corrected decoded text. Implement only enough to make Task 1 pass, then run focused `internal/result`, `internal/export`, and `internal/ui` tests plus the established Go verification command.

---

### 3. Document corrected UTF-8 decoding

**Type**: DOCUMENT  
**Output**: Wiki documentation defines maximal invalid subparts, valid U+FFFD preservation, shared consumer behavior, warning metadata, and BLOB exclusion.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #74 decoder and tests into the appropriate existing shared typed-result and export pages under `Notes/wiki`. Document the Unicode maximal-subpart rule for constrained E0–EF and F0–F4 prefixes, including invalid or missing later continuation bytes; distinguish a valid encoded U+FFFD from replacement inserted for malformed input; and state that grid, CSV, and JSON consume the same corrected TEXT value and invalid-UTF metadata. Explicitly record that BLOB bytes are neither decoded nor changed and that warning metadata remains outside exported records. Cross-reference Issue #74, user story 75, and the cache/rendering and export requirements in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` only if needed and append the required dated ingest to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the UTF-8 decoder walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/074-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/074-04/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate a valid encoded U+FFFD surviving beside malformed input, representative and boundary E0–EF and F0–F4 prefixes with invalid and missing later bytes, adjacent invalid subparts, and valid controls. Show exact shared grid, CSV, and JSON text plus the invalid-UTF signal, then contrast an identical malformed byte pattern stored as BLOB and prove its bytes are untouched. Include focused passing test output, reference Issue #74 and `Notes/PRD-sqloid.md`, and keep every generated artifact under the approved directory.

---
