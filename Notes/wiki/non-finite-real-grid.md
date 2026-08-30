# Issue #23 — Non-finite REAL grid rendering

Issue #23 completes the grid-facing REAL rendering policy started by Issue #22: REAL values that are not finite render as the exact display tokens `Inf`, `-Inf`, and `NaN` through the shared `internal/result` seam, while every finite REAL keeps Issue #22's shortest-round-trip token (including `.0` restoration) and every stored value keeps its original SQLite REAL or TEXT identity. See `Notes/PRD-sqloid.md` (Numeric value parsing and rendering, Grid rendering/cache, Export formats and values, Module Design, Testing Decisions), [first-select-result-grid.md](first-select-result-grid.md) (Issue #22), and [sql-atoms-and-literals.md](sql-atoms-and-literals.md). Issue #47 finalized the shared finite-REAL token, normalization, and exporter-facing contracts — see [shared-typed-result-rendering.md](shared-typed-result-rendering.md).

## Exact grid tokens

- **Positive infinity** — any REAL `+Inf` renders exactly `Inf` (never strconv's `+Inf`, never the finite `.0` suffix).
- **Negative infinity** — any REAL `-Inf` renders exactly `-Inf`.
- **NaN** — every REAL NaN bit pattern (quiet NaNs, negative NaNs, arbitrary payloads) renders exactly `NaN`. The token selection never inspects or exposes payload bits; there is one NaN token, not payload-specific text.

The policy lives in `internal/result.RealToken`, which branches on `math.IsInf`/`math.IsNaN` only after the value is already known to be a SQLite REAL (`KindReal`). Non-finite values never flow through finite formatting: the finite `strconv.FormatFloat(v, 'g', -1, 64)` path and its `.0` restoration are unreachable for non-finite inputs, and invalid-UTF handling (a TEXT-only policy in `FromDriver`/`DecodeText`) is never involved.

## Underlying value preservation

Rendering is display-only. Rows, snapshots, and metadata retain the original float64-backed REAL value and `KindReal` — including exact NaN payload bits — after rendering, and test fixtures construct these values both directly (`result.NewReal`) and through the production driver seam (`result.FromDriver`). No warning metadata, coercion, or normalization accompanies a non-finite token.

## Distinction from identical-looking TEXT

TEXT values containing the literal strings `Inf`, `-Inf`, and `NaN` render with the same visible glyphs but follow the TEXT policy verbatim (`GridText`, no REAL token logic), keeping `KindText`. A REAL and a TEXT cell that display identically remain distinguishable typed values; the render seam never reinterprets one as the other, exactly as Issue #22 established for REAL `1.0` versus TEXT `"1.0"`.

## Separation from finite, CSV, and JSON policies

The shared typed representation (`internal/result`) permits format-specific rendering without value coercion:

- **Grid (this issue)** — `Inf`, `-Inf`, `NaN` as exact tokens.
- **Finite REAL (Issue #22)** — shortest round-trip `g` token with `.0` restoration; unchanged by this issue.
- **JSON (later exporter issue)** — non-finite pre-existing REALs become quoted `"Inf"`/`"-Inf"`/`"NaN"` strings, because JSON has no infinite/NaN literals.
- **CSV (later exporter issue)** — the textual form in unquoted fields per the PRD Export formats and values decision.

No CSV or JSON behavior was implemented or changed here; the exporters will select their own forms from the same typed values in `internal/result` rather than copy grid tokens.

## Tests

- `internal/result/result_test.go` — `TestNonFiniteRealDisplayTokens` (signed infinities, quiet/payload/negative NaN patterns, finite REALs keeping their exact tokens, and TEXT `Inf`/`-Inf`/`NaN` verbatim) and `TestNonFiniteRealTypeAndValueRetained` (`FromDriver`-scanned rows retaining `KindReal` with exact float64 bits including NaN payloads, TEXT kind/strings unchanged, no invalid-UTF metadata, frozen row/column counts).
- `internal/ui/results_grid_test.go` — `TestResultGridNonFiniteRealTokens` renders a frozen grid over mixed REAL/TEXT rows and asserts exact per-row tokens via `gridCellTexts` (which consumes only `internal/result`), plus backing-value and row-count retention.