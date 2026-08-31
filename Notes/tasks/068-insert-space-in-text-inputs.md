# Tasks for #68: Insert space in universal value and filename inputs

Parent issue: #68
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1 → Tasks 1–2; AC2 → Tasks 3–4; AC3–AC4 → Tasks 1–4
**Manual verification**: Task 6 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify KeySpace editing in universal value prompts

**Type**: RED  
**Output**: Failing ValuePrompt tests require one space at beginning, middle, and end with exact rune cursor behavior and unchanged editing keys.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused key-message tests beside the existing universal input coverage in `internal/ui`, targeting `ValuePrompt.HandleKey` in `internal/ui/value_input.go`. Send `tea.KeySpace` with empty, ASCII, and multibyte Unicode buffers at cursor positions zero, middle, and end; require exactly one U+0020 insertion, a one-rune cursor advance, unchanged surrounding runes, and `true` change reporting. Continue each sequence with Left/Right, Backspace/Delete, Home/End, Ctrl+A/Ctrl+E, and Ctrl+U to prove space uses the ordinary rune-aware editing model. Include `tea.KeyRunes` controls and keep the task test-only without routing base or popup space keys through the prompt.

---

### 2. Insert KeySpace through the shared value-input path

**Type**: GREEN  
**Output**: ValuePrompt handles tea.KeySpace exactly like one printable space rune.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `ValuePrompt.HandleKey` in `internal/ui/value_input.go` so `tea.KeySpace` inserts one space at the current rune cursor through the same insertion helper or logic as `tea.KeyRunes`. Preserve verbatim buffer storage, rune-index cursor invariants, all movement/deletion semantics, and Enter/Esc ownership by the model. Do not reinterpret whitespace, trim input, synthesize a runes message at unrelated contexts, or change popup/base key precedence. Make only the production change required for Task 1.

---

### 3. Specify submitted spaces and filename routing

**Type**: RED  
**Output**: Failing scripted UI tests preserve spaces across value-bearing fields and filenames while retaining Limit validation and popup/base context behavior.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the existing WHERE/value, UPDATE, INSERT, Limit, popup lifecycle, and picker flow tests under `internal/ui` with Bubble Tea key sequences that use `tea.KeySpace`, not a synthetic `KeyRunes` containing a space. Enter and edit leading, internal, trailing, and all-space value buffers, including adjacent Unicode, then submit; require WHERE and LIKE, SET, and INSERT Value choices to preserve the exact representation and existing universal parsed/bound type, while whitespace-only and space-containing Limit text receives the existing exact field-specific invalid reason. In `internal/ui/picker_flow_test.go`, send `tea.KeySpace` at the beginning, middle, and end of ASCII/Unicode filenames, require exact filename text and rune cursor, and complete valid spaced basenames through SQL, CSV, and JSON extension/path construction. Add controls proving searchable popup space retains its search behavior, base-context space retains its toggle/no-op behavior, and directory-focused picker space does not become filename input. Keep this task test-only.

---

### 4. Route KeySpace through field-specific input ownership

**Type**: GREEN  
**Output**: Value and filename spaces submit correctly while field validation and unrelated context routing remain unchanged.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `internal/ui/filepicker.go` so filename-focused `tea.KeySpace` calls `picker.InsertRunes` with exactly one space, complementing the shared `ValuePrompt` handling from Task 2. Use the existing focus-first dispatcher boundaries so submitted WHERE/LIKE/SET/INSERT and Limit flows consume the exact ValuePrompt buffer without trimming or special whitespace parsing, and valid spaced filenames continue through the established `internal/filepicker` validation, required-extension, and path-join logic. Preserve picker-level Left/Right format toggling, Tab focus, submit/cancel, all remaining editing keys, popup search, base context, and directory-focus behavior. Do not make KeySpace globally printable or duplicate parser/filename validation in the UI.

---

### 5. Document space handling in text inputs

**Type**: DOCUMENT  
**Output**: Wiki documentation records KeySpace routing, rune-aware editing, verbatim value semantics, Limit validation, filename completion, and context isolation.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #68's implementation and tests from `internal/ui/value_input.go`, `filepicker.go`, their scripted flow tests, and the `internal/filepicker` completion boundary into the appropriate pages under `Notes/wiki`. Document that Bubble Tea `tea.KeySpace` inserts one U+0020 at the rune cursor only while universal value or filename input owns focus; record verbatim WHERE/LIKE/SET/INSERT submission, ordinary Limit validation, valid filenames with spaces, and unchanged cursor/deletion behavior. Explicitly distinguish popup search and base-context space routing. Cross-reference Issue #68 and the universal parsing, Limit, file picker, context/action matrix, UI Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update the index and append the required dated wiki log entry without rewriting prior history.

---

### 6. Create the text-input space walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/068-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/068-06/code-walkthrough`, with the main file named `walkthrough.md`. Demonstrate `tea.KeySpace` insertion and rune-safe editing at beginning, middle, and end for WHERE equality/LIKE, UPDATE SET, INSERT Value, Limit, and SQL/CSV/JSON filenames. Show exact submitted text and bound values, the existing invalid Limit feedback for whitespace, and successful completed paths for filenames with spaces. Also show popup-search space and base-context space retain their prior behavior without leaking into inputs. Reference Issue #68 and `Notes/PRD-sqloid.md`, and keep every showboat artifact in the approved directory.

---
