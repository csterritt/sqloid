# Tasks for #69: Remove unreachable filename arrow-key cases

Parent issue: #69
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC3 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Lock reachable picker key-routing behavior

**Type**: REFACTOR
**Output**: Passing behavioral tests lock picker-level format routing and every reachable filename edit before dead code is removed.
**Verification obligation**: The behavioral safety net is green before the refactor and remains green afterward; production behavior does not change.
**Supplemental checks**: none
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the key-routing coverage in `internal/ui/picker_flow_test.go`, following its injected picker and exact state assertions. In both directory and filename focus, send Left and Right through `Model.Update` and require them to change only the export format through `pickerToggleFormat`, preserve filename text/cursor and directory state, and never reach filename mutation. Exercise every remaining reachable filename editing route—printable runes, Backspace/Ctrl+H, Delete, Home/Ctrl+A, End/Ctrl+E, Ctrl+U—plus Tab, Enter, and Esc at the picker dispatcher, requiring unchanged edit/focus/submit/cancel behavior. Run this behavioral safety net before production edits and record that it is green; do not add an AST unit test, broad snapshots, or unrelated linter configuration.

---

### 2. Remove dead filename arrow branches

**Type**: REFACTOR  
**Output**: applyPickerFilenameKey contains no unreachable Left/Right arms and all picker routing tests pass unchanged.  
**Verification obligation**: Task 1's behavioral safety net passes unchanged; production behavior does not change.
**Supplemental checks**: Focused grep/static build evidence shows the dead Left/Right arms are absent; `go build ./...` and `go vet ./...` pass.
**Depends on**: 1

Refactor `applyPickerFilenameKey` in `internal/ui/filepicker.go` by removing only its `tea.KeyLeft` and `tea.KeyRight` cases. Preserve the earlier picker-level `msg.String()` Left/Right branch in `handlePickerKey`, which must continue toggling export format before focus-specific filename dispatch in both picker focuses. Do not add replacement cursor bindings, reorder global/pending/focus precedence, or alter any reachable filename mutation, focus, validation, submission, cancellation, opener restoration, or format behavior. Update nearby comments only where needed to state the actual reachable editing contract. Run the unchanged focused UI picker safety net and repository verification, then use a focused grep/static build check as supplemental evidence that the two dead switch arms are absent; do not encode private function shape in a unit test.

---

### 3. Document picker arrow-key ownership

**Type**: DOCUMENT  
**Output**: Wiki documentation records picker-level format ownership of Left/Right and the reachable filename editing key set.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #69's routing tests and refactor from `internal/ui/filepicker.go` and `internal/ui/picker_flow_test.go` into the appropriate `Notes/wiki` pages. Document that Left/Right are consumed at picker scope in both directory and filename focus to toggle export format and therefore are not filename cursor keys; list the remaining reachable filename editing, focus, submit, cancel, validation, and restoration behavior. Cross-reference Issue #69 and the File picker, context/action matrix, UI Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for page changes and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the picker-routing walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/069-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/069-04/code-walkthrough`, with the main file named `walkthrough.md`. Show the dispatcher ordering and the cleaned `applyPickerFilenameKey`, then exercise Left/Right from directory and filename focus to prove export-format toggling occurs with filename text/cursor unchanged. Demonstrate every remaining reachable filename edit plus Tab focus, valid/invalid Enter, and Esc restoration, and run the focused picker tests showing behavior is unchanged by the dead-code removal. Reference Issue #69 and `Notes/PRD-sqloid.md`, and keep all generated artifacts beneath the approved directory.

---
