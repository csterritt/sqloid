# Tasks for #15: SELECT wildcard and COUNT(*) projection path

Parent issue: #15
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify empty-projection candidates and transitions

**Type**: RED
**Output**: Failing QueryBuilder tests cover `*` first/default, conditional `COUNT(*)`, direct sentinel addition, hiding/reappearance, and named-column continuation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create pure table-driven projection tests in `internal/querybuilder/projection_test.go`, using typed visible columns from `internal/schema` and no Bubble Tea dependencies. Define candidate identity separately from display text and require an empty SELECT projection to produce wildcard `*` first and default-highlighted, synthetic bare `COUNT(*)` second, then eligible named columns in Schema order. Require selecting `COUNT(*)` to add a dedicated sentinel directly and return a transition that reopens Column(s), without requesting named-column aggregate selection; once any projection entry exists, require the bare sentinel to be absent from candidates, and require it to reappear in second position when removal returns the projection to empty. Require named-column selection, whether the projection is empty or populated, to preserve the chosen column identity and transition to aggregate selection rather than commit immediately. Cover wildcard direct addition, empty column metadata, columns named `*` or `COUNT(*)`, and ensure no `MIN(*)`, `MAX(*)`, `AVG(*)`, or `SUM(*)` candidate can be represented. Keep this task test-only and defer general ordering/deduplication rules to Issue #16.

---

### 2. Implement wildcard and sentinel projection state

**Type**: GREEN
**Output**: Pure projection candidate and transition tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add the minimal UI-independent projection candidate and transition model to `internal/querybuilder/projection.go`, integrating its state with `internal/querybuilder/builder.go` and consuming typed column metadata from `internal/schema`. Represent wildcard, bare `COUNT(*)`, and named columns as distinct identities so display labels cannot collide with real column names. Derive deterministic candidates with `*` first/default and conditional `COUNT(*)` second only while projection is empty; make wildcard and the sentinel direct transitions, and return named columns as pending aggregate choices. Hide the sentinel after any committed entry and restore it after the projection becomes empty. Enforce unsupported wildcard aggregates by construction, avoid importing `internal/ui`, and implement only the state required by Task 1 without completing Issue #16's broad ordered-editing behavior.

---

### 3. Specify Column(s) popup flow

**Type**: RED
**Output**: Failing scripted tests cover popup ordering, direct `COUNT(*)` reopen, aggregate-popup routing, focus, and unsupported wildcard aggregates.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model tests in `internal/ui/projection_popup_test.go` and focused rendering assertions in `internal/ui/projection_view_test.go`, following Issue #12's reusable searchable/scroll-only popup contract and Issue #11's exact QueryBuilder focus ownership. Open Column(s) for an empty SELECT and assert rendered order, candidate identities, wildcard default highlight, searchable named columns, and acceptance of `*`. Select bare `COUNT(*)` and require direct commit followed by Column(s) reopening with exact Column(s) focus, reset popup search/highlight/viewport according to the reusable contract, and no sentinel in the now-nonempty candidate list. Select named columns from empty and nonempty projections and require routing to the scroll-only Value/Count/Min/Max/Avg/Sum aggregate popup, with aggregate acceptance returning to and reopening Column(s) while preserving completed entries. Remove back to empty through the existing model seam and require the sentinel's reappearance. Assert that no aggregate popup opens for wildcard or bare `COUNT(*)` and no unsupported aggregate-on-wildcard option is rendered or accepted. Keep this task test-only and do not duplicate projection rules in model tests.

---

### 4. Integrate projection state with Column(s)

**Type**: GREEN
**Output**: End-to-end popup tests pass using the reusable popup contract.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire the QueryBuilder projection transitions into the TUI through `internal/ui/projection_popup.go`, `internal/ui/model.go`, and the existing builder/view rendering seams, updating reusable popup configuration only where Issue #12's contract requires it. Populate Column(s) from `internal/querybuilder/projection.go` and refreshed `internal/schema` columns, preserve typed candidate identity, and let QueryBuilder decide whether acceptance commits wildcard or bare `COUNT(*)` directly or opens the named-column aggregate popup. Configure Column(s) as searchable and aggregate choices as scroll-only; after bare-sentinel or aggregate completion, reopen Column(s) with exact focus and deterministic reset semantics, and on cancel restore the correct opener while preserving only completed selections. Never synthesize wildcard aggregate choices in UI code, preserve overlay non-reflow and keyboard modality, and implement only enough to make Tasks 1 and 3 pass without taking over Issue #16's ordered removal and full deduplication scope.

---

### 5. Document wildcard and COUNT(*) behavior

**Type**: DOCUMENT
**Output**: Wiki documentation records the empty Column(s) sequence and sentinel visibility rules.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #15 implementation and tests from `internal/querybuilder`, `internal/ui`, and the consumed `internal/schema` metadata into the appropriate pages under `Notes/wiki`. Document the exact empty Column(s) ordering and default highlight, distinct wildcard/sentinel/named-column identities, conditional visibility of bare `COUNT(*)`, direct sentinel commit and Column(s) reopen, sentinel hiding after any entry and reappearance after returning to empty, and named-column routing through the Value/Count/Min/Max/Avg/Sum scroll-only aggregate popup. Record that wildcard and bare `COUNT(*)` never open the aggregate popup and that unsupported wildcard aggregates are absent by construction. Cross-reference Issues #12 and #15 and the Query Grammar, Builder and Display Interaction, QueryBuilder, UI, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the projection-path walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/015-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/015-06/code-walkthrough`. Demonstrate an empty Column(s) popup with `*` first and highlighted, bare `COUNT(*)` second, and named columns after both. Show wildcard acceptance, reset to empty, direct `COUNT(*)` acceptance and immediate reopen with the sentinel hidden, named-column selection entering the Value/Count/Min/Max/Avg/Sum aggregate popup, aggregate completion reopening Column(s), and removal back to empty restoring the sentinel. Include evidence that real columns with sentinel-like names retain distinct identity and that `MIN(*)`, `MAX(*)`, `AVG(*)`, and `SUM(*)` are never offered. Reference Issue #15 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
