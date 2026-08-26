# Tasks for #53: Atomic saves and overwrite protection

Parent issue: #53
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify overwrite confirmation state

**Type**: RED
**Output**: Failing model/filesystem tests cover existing destination detection, immutable payload/selection, explicit confirmation, cancel restoration, and no premature replacement.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic save-flow tests in `internal/ui` and filesystem-boundary tests in `internal/export`, extending Issue #52's shared picker and Issue #48's immutable SQL target while also exercising immutable CSV/JSON copies from Issues #49-#51. Cover new and existing destinations for `.sql`, `.csv`, and `.json`; require an existing destination to open exactly one overwrite confirmation only after final path resolution and before any destructive filesystem operation. Capture destination, format, payload, source selection, opener state, and focus at save-flow entry, mutate the live builder/result/history selection behind the confirmation, and prove confirm and cancel continue to use the captured values without re-resolving or reserializing mutable state. Require Esc/n to cancel only overwrite and return to the intact picker/save path with filename, directory, format, warnings, selection, and immutable copy preserved; require explicit Enter/y before replacement can proceed. Instrument filesystem calls to assert existing-file detection performs no truncation, removal, rename, or write and that repeated/unrelated keys neither stack confirmations nor leak into restored state. Keep this task test-only and leave temporary-file replacement to Tasks 3-4.

---

### 2. Implement overwrite confirmation flow

**Type**: GREEN
**Output**: New/existing destination and confirmation/cancel tests pass without reserializing mutable state.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the overwrite branch across `internal/ui` and the UI-independent destination boundary in `internal/export`, following Issue #52's inline retry/cancel and exact opener-restoration patterns. Freeze the resolved destination, output format, source selection, warnings, and complete immutable serialized payload before checking the destination; send a new path directly to the write stage and route an existing path into one nonstacking confirmation without opening or truncating it. On Esc/n, restore the latest intact save-flow state and exact focus without discarding the captured copy or leaking the cancellation key; on Enter/y, advance that same captured payload and destination to the write stage. Ignore duplicate confirmations and stale asynchronous destination responses by save-flow identity, retain path/permission errors inline, and ensure no branch consults the live builder, active result, or current history selection after capture. Implement only enough to make Task 1 pass without adding the temp-file-plus-rename mechanism.

---

### 3. Specify destination-local atomic replacement

**Type**: RED
**Output**: Failing injected tests cover serialization/write/temp-close/rename failures, existing-destination preservation, temp cleanup, successful replacement, and inline retry/cancel.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add injected filesystem and serializer tests in `internal/export` plus scripted completion/error tests in `internal/ui` for atomic output of SQL, CSV, and JSON. Inject serialization failure, destination-local temporary-file creation failure, partial and full write failure, temporary-file sync/close failure where applicable, and final rename failure as distinct stages; for every failure before rename, require byte-for-byte preservation of an existing destination, absence of a newly claimed destination, closure and cleanup of every temporary artifact, and one inline error that retains the immutable payload, resolved path, format, selection, and safe retry/cancel path. For rename failure, require no success message or replacement claim, preservation of whatever target state the platform reports, best-effort temp cleanup, and the same inline retry/cancel behavior. Prove the temporary file is created in the destination directory rather than a global temp location, receives the already captured immutable bytes exactly once, is closed before rename, and cannot collide with or expose a partial target. Cover successful creation of a new destination and successful replacement of an existing one with exact output bytes, no leaked temporary artifacts, one completion transition, and exact opener restoration. Use deterministic injected boundaries rather than restrictive-permission timing for automated ordering, keep this task test-only, and preserve platform behavior required on Linux and macOS.

---

### 4. Implement temp-file-plus-rename saving

**Type**: GREEN
**Output**: Atomic replacement and every pre-rename cleanup/preservation test pass; rename failure never claims success.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement atomic file output in `internal/export` using a uniquely named temporary file created in the resolved destination directory, the immutable bytes supplied by Task 2, complete write and required close handling, and a final rename over the destination. Centralize ownership of the temporary resource so every serialization, create, write, and pre-rename close failure closes what was opened, removes the temporary artifact, and returns a typed stage error without touching an existing destination. Treat rename as the sole replacement boundary; report success only after it succeeds, and on rename failure return an inline-retry error, perform safe best-effort temp cleanup, and never claim that replacement occurred. Integrate typed completion/failure messages into `internal/ui` with save-flow identity guards so retry reuses the captured destination/format/payload/selection, cancel restores the exact opener, stale or duplicate completions are ignored, and active SELECT state is not finalized. Implement only enough to make Task 3 pass, retaining Issue #52 path validation and picker ownership.

---

### 5. Document atomic file output

**Type**: DOCUMENT
**Output**: Wiki documentation records overwrite state, immutable payloads, destination-local temps, failure boundaries, cleanup, and retry.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #53 implementation and tests from `internal/export` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document existing-destination detection without premature replacement, one explicit overwrite confirmation, immutable destination/format/payload/selection capture, confirmation and cancellation behavior, and exact return to the picker/save opener. Record destination-local unique temporary files, complete write/close before rename, rename as the only replacement boundary, successful new-file and replacement behavior, and each serialization/create/write/close/rename failure boundary. Explain existing-destination preservation, temporary cleanup, inline error presentation, retry using the same immutable copy, cancel restoration, stale-completion guards, and the accepted restrictive-directory limitation without implying success after rename failure. Cross-reference Issues #48-#53, the File picker, Atomic saves, Export Module Design, Global Key Precedence, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the atomic-save walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/053-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/053-06/code-walkthrough`. Demonstrate saving each supported format to a new destination and an existing destination, show that existing bytes remain untouched before explicit Enter/y confirmation, mutate live builder/result/history state behind confirmation, and verify the captured destination, format, immutable copy, and selection remain authoritative. Cancel overwrite with Esc/n and capture exact picker/save restoration, then confirm and show destination-local temp creation, complete write/close, rename, exact final bytes, and no temp artifact. Deterministically inject serialization, temporary creation, partial/full write, close, and rename failures; for each capture destination preservation where guaranteed, cleanup, inline error, no false success, retry with the same copy, and safe cancel. Reference Issue #53 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
