# Tasks for #12: Searchable popup interaction contract

Parent issue: #12
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify popup filtering and viewport behavior

**Type**: RED
**Output**: Failing model tests cover case-insensitive subsequence matching, empty/no-match states, scrolling, and deterministic highlight reset.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create pure popup-state tests in `internal/ui/popup_test.go` and focused rendering tests in `internal/ui/popup_view_test.go`, following the scripted model conventions in the UI Module Design and Testing Decisions of `Notes/PRD-sqloid.md`. Cover reusable searchable and scroll-only candidate lists: case-insensitive subsequence matching, empty search showing all candidates in source order, nonmatching search keeping the popup open with exact `no matches`, viewport scrolling at both boundaries, and deterministic highlight and viewport reset whenever search text changes. Include empty candidate data, repeated case variants, lists longer than the viewport, and search changes while scrolled. Keep this task test-only, independent of `internal/querybuilder` fields and database access, and do not prescribe future column, grouping, aggregate, or operator implementations.

---

### 2. Implement reusable popup filtering and scrolling

**Type**: GREEN
**Output**: Searchable and scroll-only popup state tests pass independently of builder fields.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement reusable popup state, matching, navigation, and viewport calculation in `internal/ui/popup.go` and presentation in `internal/ui/popup_view.go`. Keep candidate identity separate from displayed text, preserve deterministic source ordering, apply case-insensitive subsequence filtering only to searchable variants, show all items for empty search, retain an open empty/no-match state, and reset highlight and viewport predictably after each search change. Support scroll-only variants without introducing a search input. Follow the existing Issue #8 overlay and non-reflow patterns, but do not connect the component to Command, Table, or `internal/querybuilder` yet; implement only enough to pass Task 1.

---

### 3. Specify popup completion and restoration semantics

**Type**: RED
**Output**: Failing tests cover single-select Enter, multi-select add/reopen, Esc preservation, search input, and exact opener focus restoration.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui/popup_test.go` and add scripted lifecycle coverage in `internal/ui/popup_lifecycle_test.go`. Require searchable text input to update filtering without leaking into builder shortcuts, single-select Enter to accept the highlighted candidate and close, multi-select Enter to add a nonduplicate completed selection and reopen for another choice, and Esc to discard only unfinished popup work while preserving already completed multi-selections. Capture the exact field and focus context that opened the popup and require both accept and cancel paths to restore that opener rather than infer a default focus. Cover Enter with no matches, empty candidates, selection after scrolling/filtering, repeated multi-select attempts, and both searchable and scroll-only variants; keep this task test-only and independent of Table-specific production wiring.

---

### 4. Integrate popup variants with Table selection

**Type**: GREEN
**Output**: Selection/restoration tests pass, and Table is the first end-to-end searchable-popup consumer.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Complete reusable accept, reopen, cancel, and opener-restoration behavior in `internal/ui/popup.go`, then wire the Table field through `internal/ui/table_popup.go` and `internal/ui/model.go`. Populate Table candidates from refreshed Schema metadata represented in `internal/querybuilder/builder.go` and `internal/querybuilder/command_table.go`, filter according to the currently selected command's eligibility, and commit accepted object identity back through QueryBuilder transitions rather than duplicating builder rules in UI code. Make Table the first end-to-end searchable single-select consumer, preserving its exact opener focus on Enter and Esc and leaving multi-select and scroll-only behavior reusable and fully tested for later fields. Follow Issue #8 overlay non-reflow and Issue #11 focus/navigation behavior; do not implement later column, GROUP BY, ORDER BY, aggregate, or operator consumers. Make Tasks 1 and 3 pass without database access in `internal/ui`.

---

### 5. Document popup interaction contracts

**Type**: DOCUMENT
**Output**: Wiki documentation records matching, scrolling, selection variants, cancellation, and focus restoration.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #12 implementation and tests from `internal/ui` and the touched `internal/querybuilder` integration into the appropriate pages under `Notes/wiki`. Document searchable versus scroll-only variants, case-insensitive subsequence matching, empty and no-match states, deterministic highlight/viewport reset, scrolling boundaries, search input modality, single-select acceptance, multi-select add-and-reopen semantics, duplicate handling, Esc preservation of completed selections, and exact opener focus restoration. Identify Table as the first end-to-end consumer and later column/grouping/aggregate/operator flows as consumers rather than current scope. Cross-reference Issue #12 and the Builder and Display Interaction, UI Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`, update `Notes/wiki/index.md`, and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the popup-contract walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/012-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough in `Notes/walkthroughs/012-06/code-walkthrough`. Demonstrate the Table searchable popup with empty search, case-insensitive subsequence matches, `no matches`, a list longer than the viewport, scrolling, changed search while scrolled, accepted selection, and cancellation with exact Table focus restoration. Include test-backed evidence for reusable single-select, multi-select add/reopen and Esc preservation, and scroll-only variants without presenting later builder fields as implemented. Reference Issue #12 and `Notes/PRD-sqloid.md`, and place every generated showboat artifact under the approved directory.

---

### 7. Review popup behavior

**Type**: REVIEW
**Output**: Human confirms empty, matching, nonmatching, scrolled, accepted, and cancelled Table-popup flows.
**Depends on**: 6

Review `internal/ui/popup.go`, `internal/ui/popup_view.go`, `internal/ui/table_popup.go`, related model and QueryBuilder integration, tests, wiki updates, and walkthrough against Issue #12. Open Table with empty and populated candidates, type matching and nonmatching searches with case variation, navigate beyond one viewport, change search while scrolled, accept a selection, and cancel from multiple positions. Confirm deterministic highlight reset, exact `no matches`, candidate eligibility, accepted Table state, unchanged completed state on cancellation, and exact opener focus restoration; also inspect the reusable tests for single-select, multi-select, and scroll-only semantics before approving the issue.

---
