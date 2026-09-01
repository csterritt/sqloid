# File picker (save/export destination)

The destination picker for Ctrl+S query save and Ctrl+X result export (Issue #52), per the File picker decision and the Global Key Precedence and Context/Action Matrix in `Notes/PRD-sqloid.md`. The UI-independent model lives in [internal/filepicker](source-code.md); `internal/ui` composes it for every opener. This page documents navigation, ordering, filename entry, validation, extensions, inline errors, retry/cancel, and opener restoration. Overwrite confirmation and atomic temp-and-rename persistence are owned by later issues; the picker itself touches no file and starts no database work.

## Opening and start directory

- Every query-save and CSV/JSON-export opener begins at the **process working directory**, captured at open through the injected [FS boundary](source-code.md) (`PickerFS`/`PickerStart` are test seams; production uses `os.Getwd` and `filepicker.OSFS`).
- The first directory read is issued as a `tea.Cmd` — blocking work never runs inside `Update` or `View`. Exactly one request is outstanding at any time; while pending, navigation and submission are consumed without issuing duplicates.
- Openers: Ctrl+S opens the picker with format `sql` after [save targeting](sql-save-targeting-serialization.md) succeeds; the [export warning flow](immutable-export-capture.md) proceeds to the picker with the current closed save format (`csv`/`json`, toggled with Left/Right inside the picker). The immutable capture/prepared target is carried as opaque opener-owned state.

## Directory navigation and ordering

- Lists **only navigable directories**: visible and hidden (dot-prefixed) children alike, never regular files. Directory creation is unsupported — the FS boundary exposes only `ReadDir`.
- The parent `..` is **always listed first** whenever a parent is navigable (absent only at the filesystem root); child basenames follow in **ascending case-sensitive bytewise order equivalent to Go string comparison** (`sort.Strings`). Ordering is **locale-independent** and deliberately **not natural numeric order**: `B` sorts before `a`, `d10` before `d2`, `.conf` before `.hidden`, and non-ASCII bytes order by their bytes alone.
- Up/Down move the highlight with clamped boundary no-ops; Enter enters the highlighted entry (`..` navigates to the parent, otherwise the child directory); repeated navigation issues one request at a time. Every navigation resets the highlight to the first entry and never edits the filename buffer.
- Directory read failures (path and permission) become **typed inline errors** (`filepicker.Error`, `KindRead`) in the same picker; the current directory, highlight, filename, cursor, and format are retained untouched, and a late/stale response whose attempt identity no longer matches is inert. Retry (Enter on the error, or after correction) re-issues the same path with a fresh attempt identity and clears the error only through the user's forward action (edit, navigate, retry).

## Filename entry and validation

- Filename text is a **separate state and focus target** from directory selection; Tab switches panes. Directory navigation leaves the filename and its cursor unchanged.
- While filename focus owns the keys, **every printable key — including `?` and `q` — inserts literally**, ahead of global help/quit handling, with editing keys (Backspace, Delete, Left, Right, Home, End, Ctrl+A/E/U) affecting only the input. No action key leaks through; Ctrl+C still opens the quit confirmation, which suspends the exact picker context and restores it on Esc/n.
- Validation rejects, inline and typed (`KindValidate`), before any destination is constructed or any filesystem effect issued:
  - empty basename → `filename is empty`
  - basename containing `/` → `filename may not contain '/'`
  - basename containing NUL → `filename may not contain NUL`
- These submits issue no verification read, no serialization, no write, and no overwrite prompt; the picker stays open with state intact, and editing clears the inline error.

## Extension completion and submission

- The opener supplies a closed save-format value (`sql`, `csv`, `json`). On submission the **required extension is appended exactly once**: a name missing the exact lowercase suffix gets it; a basename already carrying the required suffix is preserved verbatim; all otherwise-valid text — dots, spaces, leading dots, multiple extensions, mixed case (`.SQL` is not `.sql` and gains the suffix), Unicode — is never rewritten.
- The validated, completed basename is joined to the selected directory **only at submission** (`dir + "/" + name`), producing the verified destination. Verification issues one read of the destination's directory (never the destination file) with its own attempt identity; success completes the picker, failure becomes a typed inline `KindVerify` error retaining selection, filename/cursor, and format for retry.

## Errors, retry, and restoration

- Path and permission failures during navigation or destination submission stay **inline in the same picker**, start no database work, and retain the current directory (when still valid), highlighted selection, filename text/cursor, save format, and the immutable query or result capture owned by the opener, so correction or retry needs no recapture. Repeated failure and corrected retry are deterministic; stale/late responses are rejected by attempt identity without duplicated picker effects.
- **Esc from directory focus, filename focus, or an inline-error state** — and **successful completion** alike — restore the exact opener atomically: mode, focus, popup/overlay status (including the intact export warning flow, from which the picker was opened), active SELECT identity/lifetime/cache/viewport, historical or terminal stable-ID selection, query builder, and the captured save/export context. Zero keys leak; zero database work runs anywhere in the flow (`picker_flow_test.go` proves zero fake-executor calls end to end).
- Quit confirmation (q/Ctrl+C from either picker focus) suspends the whole model including the picker and restores it on Esc/n.

## Testing

- `internal/filepicker`: `navigation_test.go` (fake-filesystem table: working-directory start, `..`-first bytewise listing with files excluded, root without parent row, visible/hidden child and repeated `..` navigation, boundary no-ops, typed read/permission failures with in-place retry, inert stale responses, start-failure retry) and `filename_test.go` (validation table including empty/slash/NUL, completion table with exact-once suffixes and preserved literal text, submit lifecycle with typed verify failures, stale verification inertness, editing-key separation).
- `internal/ui`: `picker_flow_test.go` scripts the composed flow through `Update` for save and CSV/JSON export openers (start directory, listing order, separate filename, literal `?`/`q`, validation, verify paths, exact-opener fingerprints after Esc and completion, stale inertness, retained state through repeated failure and corrected retry, zero database work). Existing save-targeting and export-warning suites now assert that Ctrl+S/Enter open the picker and that Esc restores the intact opener including the warning flow.

## Later ownership

- **Overwrite confirmation** of an existing destination and **atomic temp-and-rename persistence** were delivered by Issue #53; the verified destination is now frozen into the immutable save-flow capture and inspected through the save boundary before a single confirmation or the atomic write stage. Issue #64 added race-safe overwrite intent: the inspected destination state token is carried to the persistence boundary, unconfirmed saves use atomic exclusive creation (`O_CREATE|O_EXCL`), and confirmed replacement re-verifies the state before rename — race failures route the user through fresh inspection and renewed confirmation. See [atomic-saves.md](atomic-saves.md), [csv-export.md](csv-export.md), [json-export.md](json-export.md), [immutable-export-capture.md](immutable-export-capture.md), and [sql-save-targeting-serialization.md](sql-save-targeting-serialization.md) for the flows feeding the picker.
