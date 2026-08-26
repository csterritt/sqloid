# Tasks for #52: Directory and filename picker flow

Parent issue: #52
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify directory navigation and ordering

**Type**: RED
**Output**: Failing fake-filesystem/model tests cover working-directory start, visible/hidden children, `..`, no creation, case-sensitive bytewise order, locale independence, and no natural sorting.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create a deterministic fake-filesystem contract and table-driven navigation tests in `internal/filepicker`, plus scripted picker model tests in `internal/ui`, for Issue #52 and the File picker, Global Key Precedence and Context/Action Matrix, UI Module Design, and picker Testing Decisions in `Notes/PRD-sqloid.md`. Require every query-save and CSV/JSON-export opener to begin at the process working directory supplied through the fake boundary, list only navigable directories including dot-prefixed hidden children, and provide parent `..` without exposing directory creation. Build mixed ASCII, case, punctuation, numeric-looking, Unicode-byte, file-versus-directory, root, empty, and unreadable fixtures. Require `..` first whenever a parent is navigable, followed by child directory basenames in ascending case-sensitive bytewise order equivalent to Go string comparison, independent of process locale and never natural numeric order; files must not enter the directory list. Exercise entering visible/hidden children and `..`, repeated navigation, roots, refreshes, and selection boundaries while retaining a separate filename input. Keep this task test-only and leave filename validation and path-error retry to later tasks.

---

### 2. Implement directory picker navigation

**Type**: GREEN
**Output**: Starting path, navigation, and deterministic ordering tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the UI-independent directory model and injected filesystem boundary in `internal/filepicker`, then compose it from `internal/ui` for query-save and result-export flows. Initialize from the captured process working directory, enumerate visible and hidden child directories without filtering by leading dot, synthesize or represent navigable parent `..` first, and sort remaining basenames by direct case-sensitive bytewise Go string ordering without locale collation or natural-number parsing. Navigate only existing directories, preserve deterministic selection/boundary behavior, exclude regular files from the navigation list, and expose no create-directory action. Keep directory choice distinct from filename text state and avoid coupling to serializers in `internal/export`. Implement only enough to make Task 1 pass; validation, extension, retry, and cancellation behavior belong to later tasks.

---

### 3. Specify filename entry and extensions

**Type**: RED
**Output**: Failing tests cover separate input, empty/`/`/NUL errors, `.sql`/`.csv`/`.json` appending, existing extensions, and literal input keys.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven filename/path-completion tests in `internal/filepicker` and scripted focused-input tests in `internal/ui` for query save and CSV/JSON export openers. Prove filename text is a separate state and focus target from directory selection and remains unchanged while navigating directories unless the user edits it. Require empty basename and any basename containing `/` or NUL to remain in the picker with an inline validation error and no serialization, filesystem write, overwrite prompt, or path completion. For valid names, require the opener's missing `.sql`, `.csv`, or `.json` suffix to be appended exactly once, while a basename already carrying the required extension is preserved and other basename text is not rewritten. Cover dots, spaces, leading dots, multiple extensions, mixed case, Unicode, and control-like input that is otherwise permitted. While filename input is focused, require printable keys—including `?` and `q`—to insert literally rather than open help or quit, with editing keys affecting only input and no action-key leakage. Keep this task test-only and leave write/overwrite behavior to its owning issue.

---

### 4. Implement filename validation and format completion

**Type**: GREEN
**Output**: Filename and extension tests pass for query save and both exports.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement pure basename validation and format-aware completion in `internal/filepicker`, with a closed save-format value supplied by `internal/ui`/`internal/export` for SQL, CSV, and JSON. Maintain independent directory and filename states; reject empty, slash-containing, or NUL-containing basenames inline before constructing a destination or issuing any serializer/filesystem effect. Append the required `.sql`, `.csv`, or `.json` only when that exact required suffix is missing, preserve already-complete names and all otherwise valid literal bytes/text, and join the validated completed basename to the selected directory only at submission. Route focused printable keys to filename editing before global `?`/`q` handling according to the context matrix. Share this implementation across query save and both result formats without copying validation in `internal/ui` or serializers, and implement only enough to make Task 3 pass.

---

### 5. Specify picker errors and restoration

**Type**: RED
**Output**: Failing tests cover path/permission inline retry, retained selection/input, Esc cancellation, and exact opener state/focus restoration.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the `internal/filepicker` fake-filesystem matrix and scripted `internal/ui` tests for directory read/stat/path failures and permission-denied failures during navigation or destination submission from query save and CSV/JSON export. Require each typed failure to stay inline in the same picker, start no database work, and retain current directory when still valid, highlighted selection, filename text/cursor, save format, and the immutable query or result capture owned by the opener so correction or retry needs no recapture. Exercise repeated failure, corrected retry, navigation after error, and stale/late retry responses without duplicated picker effects. Press Esc from directory focus, filename focus, and inline-error/retry states and require exact restoration of the opener's mode, focus, popup/overlay status, active SELECT identity/lifetime/cache/viewport, historical or terminal stable-ID selection, query builder, and captured save/export context, with no key leakage. Require successful completion to use the same opener-restoration contract. Keep this task test-only; atomic writes and overwrite confirmation remain outside Issue #52.

---

### 6. Implement picker retry/cancel lifecycle

**Type**: GREEN
**Output**: Error recovery and restoration tests pass across save/export openers.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the shared retry/cancel lifecycle across `internal/filepicker` and `internal/ui`, carrying query-save or immutable export data from `internal/export` as opaque opener-owned state. Convert directory/path/permission failures into typed inline picker errors, preserve current valid navigation state, highlight, filename/cursor, format, and capture for deterministic retry, clear or replace only the error when the user edits/navigates/retries, and reject stale completion identities. Suspend an exact opener snapshot when the picker opens and restore it atomically on Esc or successful completion, including focus, overlays, builder, active SELECT lifecycle/cache/viewport, and historical/terminal selection, without leaked keys, recapture, refetch, or database commands. Share the flow across `.sql`, `.csv`, and `.json`, but leave overwrite confirmation and atomic temp-and-rename persistence to the later owning issue. Implement only enough to make Task 5 and all prior picker tests pass.

---

### 7. Document file-picker behavior

**Type**: DOCUMENT
**Output**: Wiki documentation records navigation, ordering, validation, extensions, errors, and restoration.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #52 implementation and tests from `internal/filepicker`, `internal/ui`, and its opaque save/export boundary in `internal/export` into the appropriate pages under `Notes/wiki`. Document working-directory start, visible and hidden child navigation, parent `..`, no directory creation, directory-only listing, and exact `..`-first then ascending case-sensitive bytewise Go-string order independent of locale and natural sorting. Record separate filename focus/state, literal printable-key behavior, empty/slash/NUL validation, exact `.sql`/`.csv`/`.json` completion, and preservation of already complete names. Explain inline path/permission errors, retained directory/selection/input/format/capture on retry, stale-result protection, Esc and successful-completion restoration for every query-save and active/history/terminal export opener, and zero database work. Identify overwrite confirmation and atomic persistence as later ownership, cross-reference Issues #48, #49, and #52 plus the File picker, Result export scope, Query save targeting, context/action matrix, UI/Export Module Design, and Testing Decisions in `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the file-picker walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/052-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/052-08/code-walkthrough`. From query save and active, historical, and terminal CSV/JSON export openers, show working-directory start; navigate nested visible, hidden, and parent directories; and capture `..` first plus mixed-case, punctuation, Unicode-byte, and numeric-looking children in exact bytewise non-natural order under differing locale settings. Demonstrate separate filename entry, literal `?`/`q`, valid and empty/slash/NUL names, `.sql`/`.csv`/`.json` appending, and existing required extensions. Inject path and permission failures, retain selection/input/format/immutable capture through correction and repeated retry, then cancel from each focus/error state and complete representative flows to prove exact opener focus/state restoration and zero database work. Reference Issue #52 and `Notes/PRD-sqloid.md`, distinguish overwrite/atomic-save ownership, and place every showboat-generated artifact under the approved directory.

---
