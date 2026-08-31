# Tasks for #86: Document Value.Display as grid-only

Parent issue: #86
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Correct the result presentation documentation

**Type**: DOCUMENT  
**Output**: `internal/result` documentation defines `Value.Display()` as grid-only, directs exporters to typed values, and leaves runtime output byte-for-byte unchanged.  
**Depends on**: none

Read the comment-writing guidance referenced by `Notes/skills/AGENTS.md`, then update documentation comments only in `internal/result/result.go`. Revise the package comment to describe `result.Value`, `Value.Kind`, and the kind-specific fields as the shared UI-independent representation consumed by both the grid and exporters, while identifying visible tab/newline transformation, `(NULL)`, and `[BLOB n bytes]` as grid presentation policy rather than shared export tokens. Rewrite the `Value.Display()` comment so it explicitly serves grid-facing rendering and does not direct CSV or JSON serializers through transformed display strings. Point format-specific serializers to inspect `Kind` and typed payload fields so TEXT bytes and CSV/JSON NULL, BLOB, numeric, and non-finite policies remain under `internal/export`. Do not change function bodies, constants, imports, value conversion, serialization, or tests except an existing documentation assertion if the repository already enforces exact comment text. Run `go doc` for the package/method and the focused `internal/result` and `internal/export` tests covering tabs, newlines, NULL, BLOB, finite REAL, and non-finite REAL to prove behavior is unchanged. Then ingest the corrected source documentation into the relevant pages under `Notes/wiki`, following `Notes/wiki/wiki-rules.md` and `Notes/wiki/AGENTS.md`; update `Notes/wiki/index.md` if needed and append the required dated entry to `Notes/wiki/log.md`.

---

### 2. Create the grid-only Display walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/086-02/code-walkthrough`.  
**Depends on**: 1

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/086-02/code-walkthrough`, with the main file named `walkthrough.md`. Render the updated package and `Value.Display()` documentation and trace one typed matrix containing TEXT with tabs/newlines, NULL, BLOB, finite REAL, and non-finite REAL through grid display and CSV/JSON serialization. Show that the grid alone uses visible control symbols, `(NULL)`, and `[BLOB n bytes]`, while exporters inspect `Kind` and typed fields for their format-specific bytes. Include focused test output proving runtime rendering and serialization remain byte-for-byte unchanged. Reference Issue #86 and the Export formats and values and Grid rendering/cache decisions in `Notes/PRD-sqloid.md`, and keep all generated artifacts in the approved directory.

---
