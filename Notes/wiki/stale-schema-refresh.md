# Stale Schema Refresh, Retry, and Terminal Precedence

Issue #13: refresh-before-presentation on every Table-popup open, unchanged retention of the prior typed catalog when refresh fails with exactly `Schema data is stale — retry or cancel` plus the inline `could not refresh: <cause>`, blocked acceptance/continuation/execution while stale, explicit retry/cancel lifecycle, and typed deletion/replacement health classifications overriding everything terminal-ward. Implements User story 22 and the stale-flow requirements of the Schema scope, cache, and validation and Session health sections of [`Notes/PRD-sqloid.md`](../../PRD-sqloid.md).

Related pages: [schema-catalog.md](schema-catalog.md) (typed catalog + Connection gathering), [searchable-popups.md](searchable-popups.md) (popup contract this flow wraps), [builder-command-table.md](builder-command-table.md) (eligibility), [session-health.md](session-health.md) (request-boundary identity checks), [responsive-tui-shell.md](responsive-tui-shell.md) (suspension interplay).

## Refresh-before-presentation on every open

- Pressing Enter on the focused Table field (`internal/ui/table_popup.go`, `beginTablePopup`) installs the searchable single-select popup over the current eligible candidates captured from `QueryBuilder.EligibleTables()` — preserving the Issue #12 contract that a Table popup presents candidates at open — **and** unconditionally issues one fresh main-schema catalog request through the injected Connection seam before any refreshed result can be presented.
- The seam is the narrow `ui.CatalogRefresher` interface (`RefreshCatalog() schema.Attempt`), consumed where defined in `internal/ui/schema_refresh.go`; real wiring implements it with `connection.DB.ReadCatalog` mapped onto typed attempts. It runs only inside returned `tea.Cmd` functions, never in `Update` or `View`.
- Every issued request carries a process-lifetime attempt identity (`Model.refreshAttempt`, incremented per issue). Exactly one settled result per attempt arrives as an unexported-to-consumers `SchemaRefreshSettledMsg{Attempt, Result}`; mismatches are discarded wholesale.
- A nil refresher (nothing wired) yields no command; opens present current candidates without issuing requests.

## Typed attempts (`internal/schema/refresh.go`)

- `Attempt{Status, Catalog, Cause}` settles into exactly one `RefreshStatus`: `RefreshOK` (carries the complete refreshed catalog), `RefreshFailed` (ordinary failure; carries only its cause so no consumer can perform partial replacement), and the terminal `RefreshDeleted`/`RefreshReplaced`, which map the Connection boundary's typed `connection.HealthError` kinds — never error-string matching.
- `NewSuccess`/`NewFailure`/`NewTerminal` constructors plus `Valid()` enforce the payload rules; invalid payloads are discarded like superseded results.

## Stale state on ordinary failure

A settled ordinary failure keeps the exact prior catalog untouched (no `QueryBuilder.RefreshSchema` call, retained object pointer identity preserved) and raises both indicators:

- **Persistent status**, rendered exactly once — inside the open popup's body above its candidate rows, or leading the results region once the popup closes: `Schema data is stale — retry or cancel`.
- **Inline cause**, rendered beneath the status with the failed attempt's own cause verbatim: `could not refresh: <cause>`.

Both indicators derive from model state (`schemaStale` + stored cause), so they survive ordinary updates — resize cycles, scrolling, typing — deterministically. While stale:

- Enter cannot accept the highlighted candidate; the popup stays open untouched.
- Tab/Shift+Tab/Up/Down do not advance builder focus (navigation would continue past unchanged data).
- `ContinuationBlocked()` reports true, gating downstream continuation and execution until the flow ends.
- Search editing keeps working against the stale candidate list.

## Retry, cancel, and repeated failure

Driven by explicit messages handled in `Update`: `RetrySchemaRefreshMsg` (Ctrl+R while stale) and `CancelStaleRefreshMsg` (Esc while stale).

- **Retry** routes through the same `issueRefresh()` path as the initial open: a new identified request on the same Connection seam. While an attempt is outstanding further retries are gated (duplicate requests impossible), and the unchanged stale catalog plus both exact indicators remain visible. Retry is refused after cancel and under terminal states.
- **Success** installs the whole refreshed catalog through `QueryBuilder.RefreshSchema`, clears stale status and cause atomically, continues/reopens the Table popup with the refreshed eligible set via the deterministic `Popup.ReplaceCandidates` (whole-catalog replacement only; search text preserved, highlight and viewport reset), and restores continuation. Acceptance works immediately from refreshed data with the usual Issue #12 opener restoration.
- **Repeated ordinary failure** retains the same prior catalog and candidates, replaces the inline cause with exactly that attempt's cause, and leaves retry/cancel available.
- **Cancel** closes only the stale refresh flow: the popup disappears, the captured opener focus comes back (exact pre-open state: selected table, command, catalog snapshot all unchanged), both indicators clear, and no continuation or execution starts. The attempt identity advances so an outstanding result can never mutate the restored state.

## Terminal precedence over the workflow

Typed deletion (`connection.HealthDeleted`) and replacement (`HealthReplaced`) classifications are injected as `schema.NewTerminal(RefreshDeleted|RefreshReplaced)` attempts — delivered by implementations before work begins, at each pre-request boundary check, or through post-error reclassification per [session-health.md](session-health.md):

- On settling, the model transitions immediately to the established terminal presentation: the entire shell (all regions, overlays, indicators) becomes exactly `Database file no longer exists — session ended` for deletion, or `Database file was replaced — session ended` for replacement.
- All retry/cancel affordances disappear: both messages become no-ops, no key opens popups or advances the builder, `ContinuationBlocked()` stays true, and late refresh completions — even with matching identities — are rejected on arrival so pending or superseded work cannot revive the session. In-memory retained catalogs remain immutable snapshots, not mutated in place, matching the PRD cache rules.

## Tests

See [unit-tests.md](unit-tests.md): `internal/schema/refresh_test.go`, `internal/ui/stale_schema_test.go` (Task 1–2 contracts), and `internal/ui/stale_schema_lifecycle_test.go` (retry/cancel/terminal precedence) drive everything through scripted `(model, msg) → (model, cmd)` Update flows with a deterministic fake `CatalogRefresher` — no database access, no sleeps, precedence covered independently of error strings.
