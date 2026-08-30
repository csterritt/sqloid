# Issue #49: immutable result-export capture and warnings

*2026-08-29T18:40:40Z by Showboat 0.6.1*
<!-- showboat-id: 480f0dc8-ce37-4f8c-9d23-a6ca83cc98d6 -->

Issue #49 (Notes/PRD-sqloid.md 'Result export scope', 'Export warnings', the 'Cache and snapshot invariant', and the 'Global Key Precedence and Context/Action Matrix') implements the Ctrl+X result-export path: pure in-memory targeting, the immutable instant capture, request gating, tabular eligibility with one shared rejection, and the pre-destination metadata warning flow. First: pure internal/export eligibility and the immutable capture boundary — deep-copied deduplicated names, ascending positions, typed cells with exact BLOB bytes, and metadata carried separately from the serializer-visible payload, immune to post-capture mutation.

```bash
cd /home/chris/sqloid && go test ./internal/export -count=1 -run 'TestCaptureRowsImmutableSnapshot|TestCaptureRowsDefaultPositions|TestExportEligibility|TestCapturePayloadExcludesMetadata' -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestCaptureRowsImmutableSnapshot
--- PASS: TestCaptureRowsImmutableSnapshot (Ts)
=== RUN   TestCaptureRowsDefaultPositions
--- PASS: TestCaptureRowsDefaultPositions (Ts)
=== RUN   TestExportEligibility
=== RUN   TestExportEligibility/backed_tabular_selection
=== RUN   TestExportEligibility/empty_or_missing-backed_selection
=== RUN   TestExportEligibility/error_view
=== RUN   TestExportEligibility/write_summary
=== RUN   TestExportEligibility/outcome-unknown_entry
=== RUN   TestExportEligibility/cancelled-before-rows_marker
--- PASS: TestExportEligibility (Ts)
    --- PASS: TestExportEligibility/backed_tabular_selection (Ts)
    --- PASS: TestExportEligibility/empty_or_missing-backed_selection (Ts)
    --- PASS: TestExportEligibility/error_view (Ts)
    --- PASS: TestExportEligibility/write_summary (Ts)
    --- PASS: TestExportEligibility/outcome-unknown_entry (Ts)
    --- PASS: TestExportEligibility/cancelled-before-rows_marker (Ts)
=== RUN   TestCapturePayloadExcludesMetadata
--- PASS: TestCapturePayloadExcludesMetadata (Ts)
PASS
ok  	github.com/chris/sqloid/internal/export
```

At the model level, Ctrl+X from an idle active SELECT captures synchronously and preserves every active-SELECT/request/generation state with zero executor calls; mutating the live rows, BLOB sources, and page slices after capture leaves the export copy byte-identical and ascending, and historical capture of the selected retained snapshot does not follow later selection changes.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -run 'TestIdleActiveExportCapture|TestExportCaptureImmuneToLiveMutation|TestHistorySelectionExportCapture' -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestIdleActiveExportCapture
--- PASS: TestIdleActiveExportCapture (Ts)
=== RUN   TestExportCaptureImmuneToLiveMutation
--- PASS: TestExportCaptureImmuneToLiveMutation (Ts)
=== RUN   TestHistorySelectionExportCapture
--- PASS: TestHistorySelectionExportCapture (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Terminal targeting follows the current in-memory selection in all three terminal workflows: in the deletion and replacement states Ctrl+X captures the initially selected newest snapshot, then Ctrl+E retargets the older retained snapshot on the next press; in the outcome-unknown terminal the initially selected outcome-unknown entry is non-tabular and Ctrl+Y reaches a retained tabular snapshot — all without consulting the database.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -run 'TestTerminalSelectionExportCapture|TestOutcomeUnknownTerminalExportTargetsSelection' -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestTerminalSelectionExportCapture
=== RUN   TestTerminalSelectionExportCapture/deletion
=== RUN   TestTerminalSelectionExportCapture/replacement
--- PASS: TestTerminalSelectionExportCapture (Ts)
    --- PASS: TestTerminalSelectionExportCapture/deletion (Ts)
    --- PASS: TestTerminalSelectionExportCapture/replacement (Ts)
=== RUN   TestOutcomeUnknownTerminalExportTargetsSelection
--- PASS: TestOutcomeUnknownTerminalExportTargetsSelection (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Every request-bearing state routes Ctrl+X through the existing generic pending gate: schema validation and refresh, estimate, SELECT first/later page, count-only work, cancellation settlement, and write beginning/executing/rollback-cleanup/committing all consume the key with the exact shared feedback, no capture, no picker, and unchanged fake call counts. At idle, every non-tabular selection reports exactly the one shared Issue #49 definition `selected result has no tabular data to export` — while retained-row cancelled/failed snapshots and zero-row tabular SELECT snapshots stay exportable.

The metadata-to-warning matrix presents one deterministic order — completeness state first, truncation details (reusing Issue #31's shared exact `Result truncated: 64 MiB cache limit` definition), terminal-outcome information next, and invalid-UTF disclosure last — for exclusive complete, partial, truncated, partial-plus-truncated, row-cap and byte-cap truncation, cancelled/failed outcomes with reasons and failure positions, invalid UTF, and all truthful combinations; absent facts add no warning.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -run 'TestExportWarningMatrix' -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/' | grep -E '^(--- (PASS|FAIL)|ok|FAIL)'
```

```output
--- PASS: TestExportWarningMatrix (Ts)
ok  	github.com/chris/sqloid/internal/ui
```

The pre-destination flow drives Ctrl+X from active, historical, and terminal openers: warnings render before any destination selection or confirmation, the serializer-visible payload carries no warning records or properties, and both Esc cancel and successful completion restore the exact opener (mode, focus, selection, viewport, builder, active SELECT identity/lifetime, terminal state) with zero database work and the captured data stable.

```bash
cd /home/chris/sqloid && go test ./internal/ui -count=1 -run 'TestExportWarningFlowBeforeDestinationSelection' -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestExportWarningFlowBeforeDestinationSelection
=== RUN   TestExportWarningFlowBeforeDestinationSelection/active_opener_with_partial_truncation
=== RUN   TestExportWarningFlowBeforeDestinationSelection/historical_opener_with_failed_outcome
=== RUN   TestExportWarningFlowBeforeDestinationSelection/terminal_opener_with_byte-cap_truncation
--- PASS: TestExportWarningFlowBeforeDestinationSelection (Ts)
    --- PASS: TestExportWarningFlowBeforeDestinationSelection/active_opener_with_partial_truncation (Ts)
    --- PASS: TestExportWarningFlowBeforeDestinationSelection/historical_opener_with_failed_outcome (Ts)
    --- PASS: TestExportWarningFlowBeforeDestinationSelection/terminal_opener_with_byte-cap_truncation (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Finally, the complete Issue #49 verification suite: every export capture, gating, eligibility, and warning test across internal/export and internal/ui passes, alongside the full module build and vet.

```bash
cd /home/chris/sqloid && go test ./internal/export ./internal/ui -count=1 2>&1 | sed -E 's/[0-9.]+s$//' && gofmt -l internal/export internal/ui && go vet ./internal/export ./internal/ui && go build ./... && echo VET_BUILD_OK
```

```output
ok  	github.com/chris/sqloid/internal/export	
ok  	github.com/chris/sqloid/internal/ui	
VET_BUILD_OK
```
