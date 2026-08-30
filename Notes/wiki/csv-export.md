# RFC 4180 CSV export (Issue #50)

Issue #50 implements the CSV half of the export format boundary: a deterministic, UI-independent RFC 4180 UTF-8 writer in `internal/export` (`csv.go`) that consumes the Issue #49 immutable capture payload — shared full-set deduplicated output names and typed rows from `internal/result` — and emits exact bytes. Capture metadata and Issue #49 warnings stay outside the serializer API and data path entirely, so they can never become rows, columns, comments, or prefixes. The JSON serializer is owned by Issue #51 and documented in [json-export.md](json-export.md).

## Structure

`CSV(p export.Payload) []byte` renders exactly one header record followed by one data record per row, every record CRLF-terminated:

- **One deduplicated header** — the payload's full-set deduplicated output names (Issue #22/#47 `result.DeduplicateNames` rule, shared with the grid header and future JSON keys) serialize as record one, under the same minimal-quoting rule as data fields. A zero-row capture still emits exactly one header record.
- **Ascending rows** — data records serialize by ascending one-based logical position. The serializer sorts a temporary index of row positions with a stable sort (`ascendingPositions`), never mutating `Payload.Positions`, `Rows`, names, or any BLOB byte slice; ties keep source order. Rows supplied in nonascending source order therefore serialize ascending, and repeated serialization is byte-identical.
- **CRLF after every record** — including the final record.
- **Minimal quoting with quote doubling** — a field (header or data) is quoted exactly when it contains a comma, double quote, CR, or LF; embedded quotes double; embedded commas, CR, LF, CRLF, and tabs are preserved byte-exactly. Tabs alone never trigger quoting. No other control character (NUL, `\x01`, `\x7f`, …) forces quoting — controls pass through verbatim, quoted only when the RFC-required characters are also present.

## Value policies

`csvField` converts each typed `result.Value` under one policy, reusing the shared definition sites (no private name, numeric, or UTF-8 copy — enforced by `internal/result/architecture_test.go`):

- **SQL NULL and empty TEXT** both serialize to the identical empty CSV field — the documented accepted lossy limitation: the two typed values cannot be distinguished in CSV, while surrounding delimiters and records stay exact.
- **INTEGER** uses the authoritative shared `result.IntegerToken` (decimal `strconv.FormatInt`), covering signed 64-bit boundaries (`-9223372036854775808` … `9223372036854775807`).
- **REAL** uses the shared `result.RealToken`: finite values get the shortest round-tripping locale-independent `'g'` token with `.0` appended when the token contains none of `.`/`e`/`E` (so `1.0`, `-0.0`, `100000.0`, `1e+20`, `1e-05`, `5e-324`, `0.30000000000000004` keep REAL identity), identical to grid tokens; pre-existing non-finite REALs serialize as the exact textual tokens `Inf`, `-Inf`, and `NaN` (one token for every NaN payload).
- **BLOB** bytes encode as lowercase hexadecimal (`encoding/hex`), byte-for-byte unchanged before encoding — empty BLOBs become an empty field, `CA FE BA BE` becomes `cafebabe`.
- **TEXT** is the already-decoded string preserved verbatim through the quoting layer. Invalid UTF-8 was normalized once upstream by the shared `result.DecodeText` — exactly one U+FFFD per maximal invalid byte sequence (overlong `C0 80` as two one-byte subparts, surrogate `ED A0 80` as three, truncated prefixes per the maximal-subpart rule) — with `result.UTFWarning` metadata carried outside the payload; the CSV writer renormalizes nothing.
- Declared SQLite types are never inspected, no locale formatting is applied, and source bytes are never mutated or re-normalized.

## Warning exclusion

`export.Payload` structurally owns only `Names`, `Positions`, and `Rows`; `Capture.Metadata` and `Completeness` are carried separately for UI warnings (Issue #49's pre-destination flow). Every completeness label combination, terminal outcome (success/cancelled/failed, with and without reason), retained range, byte-cap truncation, row-cap eviction, and invalid-UTF flag combination has been proven byte-for-byte identical to the metadata-free baseline: no warning row, column, prefix, comment, or altered header, and no designated warning string (`result.ByteCapWarning`, `result.UTFWarning`, completeness/outcome wording) anywhere in the bytes.

## Testing

- `internal/export/csv_test.go` — exact-byte structure golden: deduplicated header (including a pre-suffixed `name_2` original colliding with the deduplicated duplicate, resolving to `name_3`), nonascending source-order rows serialized ascending, zero/one/multiple rows, one header, CRLF records, minimal quoting with quote doubling, header quoting, tab-alone-unquoted, no input mutation, and the full Issue #49 metadata warning-combination matrix (4 completeness × 4 outcomes × 5 flag sets) with leak and record/column-count assertions.
- `internal/export/csv_value_test.go` — the complete typed-value matrix: NULL/empty-TEXT byte identity, INTEGER boundaries, finite REAL shared tokens (integral, negative zero, fraction, repeating, pi, exponent, subnormal, max finite, precision edge) with `result.RealToken` equality, non-finite `Inf`/`-Inf`/`NaN`, BLOB lowercase hex with unchanged retained bytes, empty/multibyte TEXT, NUL and controls, commas/quotes/CR/LF/CRLF, five maximal-invalid-UTF-8 normalization cases with exact U+FFFD counts, a mixed-class golden row, and the warning-leak check.

## References

- Issues #14 (typed literals, downstream of write/SQL paths), #22/#47 (shared typed result seam and output names), #23 (REAL/non-finite tokens), #33/#49 (snapshot metadata, immutable capture, warning exclusion), #50; JSON serialization remains with Issue #51.
- `Notes/PRD-sqloid.md`: Numeric value parsing and rendering, Invalid UTF-8 TEXT, Export formats and values, Output names, Result export scope, Export Module Design, and Testing Decisions.
- Related pages: [shared-typed-result-rendering.md](shared-typed-result-rendering.md), [immutable-export-capture.md](immutable-export-capture.md), [non-finite-real-grid.md](non-finite-real-grid.md), [first-select-result-grid.md](first-select-result-grid.md), [byte-cap-oversized-values.md](byte-cap-oversized-values.md), [snapshot-metadata.md](snapshot-metadata.md), [sql-save-targeting-serialization.md](sql-save-targeting-serialization.md).
