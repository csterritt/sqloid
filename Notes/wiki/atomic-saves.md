# Atomic saves and overwrite protection

Destination confirmation and atomic file output for Ctrl+S query save and Ctrl+X CSV/JSON export (Issue #53), with Issue #63 adding an actionable cause for nil-error short writes. Per the Atomic saves decision and the File picker, Export Module Design, and Testing Decisions decisions in `Notes/PRD-sqloid.md`. The UI-independent boundary lives in [internal/export](source-code.md) (`save_write.go`); `internal/ui` composes it above the [file picker](file-picker.md) for both openers. This page documents existing-destination detection, the single overwrite confirmation, the immutable save-flow capture, the destination-local temp-file-plus-rename write stage, every failure boundary (including the short-write cause), and cancel/retry restoration. The [picker](file-picker.md) still touches no file; this flow performs the only filesystem writes in the save path and starts no database work.

## Capture: the frozen save-flow identity

- Path resolution still belongs to the [picker](file-picker.md). When verification succeeds, `beginSaveFlow` mints one immutable **save capture** — the resolved destination path, the closed output format, the complete serialized payload, the source selection's provenance, and the pre-destination warnings — before the destination is inspected. Exactly one monotonic **attempt identity** is minted with it; every later asynchronous response (inspection, completion, failure) is guarded by that identity, so duplicate confirms and stale responses are inert.
- The **SQL save flow** serializes the [Issue #48 target's](sql-save-targeting-serialization.md) immutable complete query state once through `SerializeSQLQuery`; the **export flow** serializes the [Issue #49 capture's](immutable-export-capture.md) payload through the opener's closed format with [CSV](csv-export.md) or [JSON](json-export.md). Serialization happens once, before inspection; a serialization failure (typed `ErrUnsupportedQueryState`) becomes an inline retry/cancel failure with no destination check performed.
- **No branch after capture consults the live builder, the active result, or the current history selection.** Confirm, retry, and the write command all use the captured bytes; tests mutate the prepared target and live state behind the confirmation and prove the captured destination, format, payload, and selection stay authoritative.

## Destination inspection: detection without replacement

- `InspectDestination(fs SaveFS, path string)` classifies the destination as `DestinationNew` or `DestinationExisting` through the injected `SaveFS` boundary (`SaveFS.Exists` backed by `os.Stat` in production; `SaveFS` is a test seam, `OSSaveFS` the real implementation).
- Detection is issued as a `tea.Cmd` (`SaveInspectMsg` with the capture's attempt identity and path) — never run inside `Update`. It performs **no truncation, removal, rename, or write**, never opens the destination, and starts no database work; boundary-instrumented tests prove `Exists` is the only call and an existing destination keeps its bytes.
- An inspection error (path/permission) becomes the same **inline retry/cancel failure** as a write failure; a new destination advances straight to the write stage, and an existing destination opens the overwrite confirmation.

## The single overwrite confirmation

- An existing destination opens **exactly one non-stacking confirmation** — `Overwrite existing file?` over the destination path, `Enter/y overwrite · Esc/n cancel` — drawn above the **intact picker**, which stays open underneath with its directory, listing, filename, cursor, and format untouched. The confirmation consumes every key until resolved; repeated and unrelated keys (including `q`/`?`) neither stack confirmations nor leak into the picker, and Ctrl+C still opens the shared quit confirmation that suspends and restores the whole context.
- **Enter/y** advances that one captured payload and destination to the write stage. **Esc/n** cancels only the overwrite question: it closes the confirmation and returns to the intact picker with the filename, directory, format, warnings, selection, and the captured immutable copy preserved — no replacement ever started and no destructive call made. Resubmitting afterwards re-runs verification, capture, and inspection with a fresh attempt identity.
- Explicit Enter/y is required before replacement can proceed; nothing in detection or inspection ever opens or modifies an existing file.

## The write stage: destination-local temp file plus rename

- `WriteAtomic(fs SaveFS, path string, data []byte)` performs the atomic output of the already-captured immutable bytes: a **uniquely named temporary file created in the resolved destination's own directory** (never a global temp location) via `SaveFS.TempFile(dir, ".<base>.sqloid-*")` — a hidden dotfile that cannot collide with or expose a partial target under its final name.
- The temporary file **receives the captured bytes exactly once** (one write call with the full payload), is **synced**, and is **completely closed before rename**. The write command runs outside `Update` as a `tea.Cmd` and reports through typed `SaveCompletedMsg`/`SaveFailedMsg` carrying the attempt identity.
- **Issue #63: a nil-error short write is a failed write, not a silent partial save.** When `Write` returns `(n, nil)` with `n < len(payload)`, the boundary converts the absent cause to `io.ErrShortWrite` before constructing the `StageError`. The cause matches `io.ErrShortWrite` through `errors.Is`, the stage-qualified text names the short write (`save failed at write: short write`) rather than `<nil>`, and the existing pre-rename cleanup (close the temp best-effort, remove it) runs exactly as for a non-nil writer error. Sync, the final close-as-success, and rename never run; the existing destination is preserved byte-for-byte; a previously missing destination remains absent. A non-nil writer error stays the cause unchanged — the short-write conversion applies only when the writer reported no error.
- **The final rename is the sole replacement boundary.** Success is reported only after the rename succeeded: one completion transition restores the exact opener (via the [picker](file-picker.md)'s atomic snapshot) and records the completed destination in `saveCompletedPath`/`exportCompletedPath`. Successful creation of a new destination and successful replacement of an existing one produce identical exact output bytes with no leaked temporary artifacts.

## Failure boundaries and cleanup

Every pre-rename failure is a typed `StageError` naming its stage, and each leaves the same inline state: the captured payload, resolved path, format, selection, and warnings retained with the Enter/y retry and Esc/n cancel path — success is never claimed.

| Stage | Trigger | Preservation and cleanup |
| --- | --- | --- |
| Serialization | `SerializeSQLQuery` rejects an incomplete/unsupported state | No destination check, no boundary call beyond nothing; inline retry re-runs capture from the still-immutable prepared state |
| Temp-file creation | `TempFile` fails | No temporary artifact claimed; existing destination untouched; nothing to clean |
| Write | Non-nil writer error, or nil-error short write (`n < len(payload)`, Issue #63) | Existing destination byte-for-byte preserved; temp closed best-effort and removed; nil-error short write carries an `io.ErrShortWrite` cause actionable through `errors.Is` |
| Sync | `Sync` fails | Same cleanup as write |
| Close | `Close` fails | Temp removed without a second close |
| Rename | `Rename` fails | **No success message or replacement claim**; the platform-reported target state is whatever it was; best-effort temp removal; same inline retry/cancel |

- Retry (Enter/y) reuses the **same captured destination, format, payload, and selection** — the boundary receives the identical bytes again; only a serialization-stage retry re-runs the capture, against the immutable prepared state. Cancel (Esc/n) restores the exact opener fingerprint with no completed path recorded.
- Stale or duplicate completions (wrong or already-settled attempt identities) are inert: the restored opener and recorded destination never mutate after settlement, and no key leaks into the restored state.

## Accepted limitations

- Restrictive-permission directories are handled through the same typed inline failures as every other stage; automated tests use deterministic injected `SaveFS` boundaries rather than permission-timing races, and the real `OSSaveFS` surface (`os.CreateTemp` in the destination directory, `os.Rename`) keeps Linux and macOS platform behavior.
- A rename failure can never be distinguished from a replacement that an external actor completed; the flow accordingly never claims replacement on rename failure and reports it as an inline retry/cancel error.

## Testing

- `internal/export/save_write_test.go`: detection performing no destructive call with typed stat errors; new-destination and replacement success with exact bytes, one destination-local hidden temp, and no leak; every pre-rename stage (create, partial write, full write, sync, close) with byte-for-byte destination preservation, exactly one cleanup removal, and the correct `StageError`; rename failure with best-effort cleanup and no replacement claim; Issue #63 nil-error short write (`n < len(payload)`, `nil`) returning a `StageWrite` `StageError` whose cause matches `io.ErrShortWrite` through `errors.Is` with actionable text (not `<nil>`), no sync/rename, temp closed and removed, existing destination preserved byte-for-byte, missing destination remaining absent, with complete-write and non-nil-error control rows.
- `internal/ui/save_write_test.go`: exactly one confirmation above the intact picker with zero destructive calls before Enter/y; captured destination/format/payload/selection surviving prepared-target mutation; Esc/n cancel restoring the intact picker (filename, directory, format) with the captured copy retained and unrelated keys consumed; CSV export capture carrying warnings, format, and `result capture` provenance; new and replacement success with exact bytes, no temp leak, one completion transition, and exact opener fingerprints; every injected stage failure with destination preservation, temp cleanup, inline `Save failed`, no completed path, and retry writing the same copy; failure-cancel opener fingerprints; inert duplicate completions, stale inspections, and post-settlement keys; Issue #63 nil-error short write showing one inline failure with the captured destination/payload retained, actionable short-write text (not `<nil>`), no success message, destination preserved, temp cleaned up, and retry writing the same captured copy.

## Cross-references

- Issues #48 ([sql-save-targeting-serialization.md](sql-save-targeting-serialization.md)), #49 ([immutable-export-capture.md](immutable-export-capture.md)), #50 ([csv-export.md](csv-export.md)), #51 ([json-export.md](json-export.md)), #52 ([file-picker.md](file-picker.md)), #53 (this page), and #63 (nil-error short-write cause). Issue #64 builds on this persistence boundary and must preserve the demonstrated short-write behavior.
- `Notes/PRD-sqloid.md`: File picker, Atomic saves, Export Module Design, Global Key Precedence, and Testing Decisions decisions; user story 72 (overwrite confirmation and temp-file-plus-rename saves that preserve an existing destination and clean temporary files on pre-rename failure).
