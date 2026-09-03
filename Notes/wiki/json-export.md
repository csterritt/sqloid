# Array-of-objects JSON export (Issue #51)

Issue #51 implements the JSON half of the export format boundary: a deterministic, UI-independent array-of-objects writer in `internal/export` (`json.go`) that consumes the Issue #49 immutable capture payload — shared full-set deduplicated output names and typed rows from `internal/result` — and emits exact compact UTF-8 JSON bytes. Capture metadata and Issue #49 warnings stay outside the serializer API and data path entirely, so they can never become objects, properties, keys, or wrappers. The CSV half is owned by Issue #50.

## Structure

`JSON(p export.Payload) []byte` renders exactly one top-level JSON array and nothing else — no wrapper object, no metadata properties:

- **One object per retained row** — each row serializes as one compact JSON object `{...}`, objects separated by bare commas, the array closed with `]`, and **no trailing whitespace or newline**. Zero retained rows serialize to exactly `[]`; a single row has no comma.
- **Ascending rows** — objects serialize by ascending one-based logical position, reusing the same stable index sort (`ascendingPositions`, shared with `CSV`) that never mutates `Payload.Positions`, `Rows`, names, or any BLOB byte slice; ties keep source order. Rows supplied in nonascending source order therefore serialize ascending, and repeated serialization is byte-identical (map-order independence is structural, not incidental).
- **Shared deduplicated keys in identical column order** — every object's keys are the payload's full-set deduplicated output names (Issue #22/#47 `result.DeduplicateNames` rule, shared with the grid header, CSV header, and JSON keys), emitted **directly in column order** rather than through an unordered map: `writeJSONObject` walks the names slice writing key/value pairs one at a time. No map iteration, no key reordering, no sorting — every object carries the identical left-to-right key sequence.
- **Compact byte layout** — exactly one policy: no spaces around colons or commas, keys and string values quoted, all other separators bare.

## String escaping

`writeJSONString` is one standards-compliant escaper used for both keys and string values: the double quote and reverse solidus escape with a reverse solidus; controls U+0000–U+001F use the short forms `\b`, `\f`, `\n`, `\r`, `\t` and the `\u00XX` form for the rest; the solidus is emitted verbatim (JSON does not require escaping it); every other byte passes through unchanged. TEXT is already valid UTF-8 — invalid sequences were normalized once upstream by the shared `result.DecodeText` policy (exactly one U+FFFD per maximal invalid byte sequence per Unicode Table 3-7, with corrected E0–EF and F0–F4 maximal-subpart handling and valid U+FFFD preservation per Issue #74) — and keys are driver labels or deduplicated suffixes, so the escaper never produces invalid JSON.

## Typed value policies

`writeJSONValue` converts each typed `result.Value`, reusing the shared definition sites (no private numeric, name, or UTF-8 copy — enforced by `internal/result/architecture_test.go`):

- **SQL NULL** emits the JSON literal `null` — distinctly from empty TEXT's `""`, a typed distinction CSV loses.
- **INTEGER** emits the authoritative shared `result.IntegerToken` (decimal `strconv.FormatInt`) as a **raw unquoted number token**, covering signed 64-bit boundaries (`-9223372036854775808` … `9223372036854775807`).
- **Finite REAL** emits the shared `result.RealToken` as a **raw number token**: shortest round-tripping locale-independent `'g'` token with `.0` appended when the token contains none of `.`/`e`/`E` (`1.0`, `-0.0`, `1e+20`, `5e-324`, …), identical to the grid and CSV tokens, bit-exact round-trip. No lossy generic number or map encoder is used.
- **Pre-existing non-finite REAL** cannot be a JSON number; it emits the exact **quoted** policy strings `"Inf"`, `"-Inf"`, or `"NaN"` (never strconv's `+Inf`), one string per token regardless of NaN payload.
- **TEXT** emits the escaped JSON string verbatim from the decoded string — empty and nonempty strings stay distinct, NUL and other controls are escaped, and multibyte Unicode passes through.
- **BLOB** bytes encode as a standard base64 JSON string (`encoding/base64` `StdEncoding`), the source bytes unchanged before and after — empty BLOBs become `""`, `CA FE BA BE` becomes `yv66vg==`.
- Declared SQLite types are never inspected, no locale formatting is applied, and source bytes are never mutated or re-normalized.

## Warning exclusion

`export.Payload` structurally owns only `Names`, `Positions`, and `Rows`; `Capture.Metadata` and `Completeness` are carried separately for UI warnings (Issue #49's pre-destination flow). Every completeness label combination, terminal outcome (success/cancelled/failed, with and without reason), retained range, byte-cap truncation, row-cap eviction, and invalid-UTF flag combination has been proven byte-for-byte identical to the metadata-free baseline over both TEXT-only and fully typed fixtures: no warning object, property, key, wrapper, or extra byte, and no designated warning string anywhere in the output.

## Testing

- `internal/export/json_test.go` — exact-byte structure golden: duplicate/colliding labels (a pre-suffixed `name_2` original resolving the deduplicated duplicate to `name_3`), nonascending source positions serialized ascending, zero/one/multiple rows, shared keys in identical column order, JSON metacharacters/quotes/reverse-solidus/controls/CR/LF/tab/Unicode escaping, no raw control bytes, parsed shape and key-set equivalence, no input mutation, repeated-serialization byte identity, `CaptureRows`-built equivalence, and the full Issue #49 metadata warning-combination matrix (4 completeness × 4 outcomes × 5 flag sets) with leak and array-length assertions.
- `internal/export/json_value_test.go` — the complete typed-value matrix: NULL/empty-TEXT distinction, INTEGER boundaries as raw tokens, finite REAL shared tokens (integral, negative zero, fraction, repeating, pi, exponent, subnormal, max finite, precision edge) with `result.RealToken` equality and bit-exact round-trip, non-finite quoted `"Inf"`/`"-Inf"`/`"NaN"`, BLOB standard base64 with unchanged retained bytes and decode round-trip, exact escaping of quotes/solidus/controls/short-forms/`\u00XX`/multibyte, five maximal-invalid-UTF-8 normalization cases with exact U+FFFD counts, a mixed-class golden row with parsed-type assertions, REAL-token grid/CSV equality, and the typed-data warning-leak check.

## References

- Issues #14 (typed literals, downstream of write/SQL paths), #22/#47 (shared typed result seam and output names), #23 (REAL/non-finite tokens), #33/#49 (snapshot metadata, immutable capture, warning exclusion), #50 (CSV counterpart), #51, #74 (corrected maximal-subpart decoding and valid U+FFFD preservation).
- `Notes/PRD-sqloid.md`: Numeric value parsing and rendering, Invalid UTF-8 TEXT, Export formats and values, Output names, Result export scope, Export Module Design, and Testing Decisions.
- Related pages: [csv-export.md](csv-export.md), [shared-typed-result-rendering.md](shared-typed-result-rendering.md), [immutable-export-capture.md](immutable-export-capture.md), [non-finite-real-grid.md](non-finite-real-grid.md), [first-select-result-grid.md](first-select-result-grid.md), [byte-cap-oversized-values.md](byte-cap-oversized-values.md), [snapshot-metadata.md](snapshot-metadata.md), [sql-save-targeting-serialization.md](sql-save-targeting-serialization.md).
