# Issue #47 — Shared typed result rendering for exporters

Issue #47 finalizes the shared typed rendering seam started by Issues #22 and #23: the full-set collision-safe output names, the exact finite-REAL tokens, the maximal-invalid-UTF-8 TEXT normalization with warning metadata, the exact BLOB byte identity, and the NULL/empty-TEXT/empty-BLOB distinctions all retain one definition site under `internal/result`, consumed unchanged by `internal/ui`'s frozen grid and by the new exporter-facing `internal/export` boundary. See `Notes/PRD-sqloid.md` (Numeric value parsing and rendering, Grid rendering/cache, Invalid UTF-8 TEXT, Export formats and values, Export warnings, Module Design, Testing Decisions), and cross-references to [first-select-result-grid.md](first-select-result-grid.md) (Issue #22) and [non-finite-real-grid.md](non-finite-real-grid.md) (Issue #23).

## Full-set collision-safe output names

`internal/result.DeduplicateNames` remains the sole definition of the PRD Output names rule: walking the original label set left to right, the first occurrence keeps its name and each later duplicate receives the lowest `_2`, `_3`, … suffix colliding with neither any already-final name nor any original name. This resolves empty labels, duplicates, pre-suffixed labels, and recursively colliding chains deterministically. `Page.HeaderNames` exposes the calculation per page; both `internal/ui` (frozen header, horizontal scrolling bindings) and `internal/export.OutputNames` call exactly that method, so grid and exporters receive identical final names in column order.

Obtaining the names never mutates anything: the original driver labels in `Page.Columns`, the generated SQL, and every stored value stay exactly as the driver returned them; deduplication produces a fresh slice that applies to display and export only.

## Exact finite-REAL tokens with retained type

`internal/result.RealToken` is the sole numeric token site: finite values use the shortest round-tripping `strconv.FormatFloat(v, 'g', -1, 64)` token with `.0` appended exactly when the token contains none of `.`, `e`, or `E`, keeping REAL identity for `1.0`, `-0.0` (sign preserved), `1e+20`, subnormals such as `5e-324`, and adjacent precision edges (`math.Nextafter` neighbors of 1 and `MaxFloat64`). The token is locale-independent by construction of strconv — never a decimal comma or grouped digits. Non-finite tokens (`Inf`, `-Inf`, `NaN`, one token for every NaN payload) come from the same function per Issue #23. `Value.Display` routes REALs through `RealToken` and INTEGERs through `strconv.FormatInt`; a value's `Kind` is never inferred from or coerced by its token, so INTEGER `1`, REAL `1.0`, and TEXT `"1.0"` remain three distinct typed values even where tokens look alike.

## Maximal invalid UTF-8 normalization and warning metadata

`internal/result.DecodeText` is the sole normalization site: each maximal invalid byte sequence (per Unicode's maximal-subpart rule — isolated continuation bytes, truncated multibyte prefixes, overlong encodings such as `C0 80`, surrogate encodings such as `ED A0 80`, and adjacent invalid runs) becomes exactly one U+FFFD. `FromDriver` applies it once to TEXT only and sets the structured `Page.InvalidUTF` metadata (surfaced as the persistent `result.UTFWarning` status text); row and column counts are unchanged and exporters can aggregate the warning without reparsing text.

- **BLOBs** holding the same bytes stay byte-for-byte unchanged — including empty payloads, NUL, high bytes, and invalid UTF-8 — and never set the warning. `Value.Bytes` are safe immutable copies; mutating caller storage never affects the retained payload.
- **NULL/empty distinctions**: SQL NULL (`KindNull`, grid `(NULL)`), empty TEXT (`KindText`, `Str == ""`, exports as an empty field), and empty BLOB (`KindBlob`, zero bytes, `[BLOB 0 bytes]`) are three distinct typed values.
- **Controls**: normalized TEXT retains tabs, newlines, carriage returns, NUL, and every other control verbatim. Issue #22's visible grid rendering (`GridText`: `⇥`/`⏎` symbols, `[BLOB n bytes]` placeholders, `(NULL)`) is display-only; exporters receive the raw typed policy inputs and must not infer type from display text.

## The exporter-facing boundary (`internal/export`)

`internal/export` is a thin consumer of the shared seam, not a second representation: `OutputNames(page)` delegates to `Page.HeaderNames` and `CellToken(value)` delegates to `Value.Display`. It owns no collision suffixing, numeric formatting, UTF-8 replacement, or format serialization — CSV quoting and JSON encoding plus their non-finite policies are owned by Issues #50/#51, which will select per-format forms from the same typed values rather than copy grid tokens.

## Definition sites and architecture checks

Single definition sites (all in `internal/result`): name deduplication (`DeduplicateNames`), REAL tokens (`RealToken`), TEXT normalization (`DecodeText`/`maximalSubpart`), grid display (`Display`/`GridText`), and driver conversion (`FromDriver`). `internal/result/architecture_test.go` enforces this by parsed-source assertions: the result package imports neither Bubble Tea nor any driver; `internal/ui` owns no private representation (`FormatFloat`, `FormatInt`, `[BLOB `, `RuneError`); and `internal/export` owns none either, and imports no `strconv`, `unicode/utf8`, `encoding/csv`, or `encoding/json`.

## Tests

- `internal/export/export_test.go` — full-set collision cases with metadata-mutation checks, exporter/grid name equivalence in column order, exact finite-REAL tokens with bit-exact round-trip and locale-independence assertions, REAL identity retention versus identical-looking INTEGER/TEXT, maximal-invalid-UTF replacement with warning metadata, exact BLOB bytes, controls in normalized TEXT, and NULL/empty-TEXT/empty-BLOB distinctions.
- `internal/ui/export_seam_test.go` — grid/exporter name equivalence rendered through the real model route with driver metadata unchanged.
- `internal/result/result_test.go`, `internal/result/architecture_test.go`, and `internal/ui/results_grid_test.go` — the pre-existing Issue #22/#23 contract pins, retained.
