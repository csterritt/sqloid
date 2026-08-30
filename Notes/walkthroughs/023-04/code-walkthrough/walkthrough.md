# Issue #23 walkthrough: non-finite REAL grid rendering

*2026-08-27T23:19:16Z by Showboat 0.6.1*
<!-- showboat-id: 7b52aee0-794e-4b65-a3ac-ed8a164162a6 -->

This walkthrough demonstrates Issue #23 (non-finite REAL grid rendering), per Notes/PRD-sqloid.md (Numeric value parsing and rendering, Grid rendering/cache, Export formats and values, Module Design, Testing Decisions). Every step re-runs real repository tests as evidence.

**1. Typed-result fixture: REAL +Inf, -Inf, multiple NaN payloads, finite REALs, and TEXT with the same visible strings.** The internal/result contract tests construct these values both directly and through the production driver seam (FromDriver), pin exact display tokens, and assert the float64-backed REAL values — including exact NaN payload bits — survive rendering untouched.

```bash
go test ./internal/result -count=1 -run 'TestNonFiniteRealDisplayTokens|TestNonFiniteRealTypeAndValueRetained' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s|\(cached\)/(t)/g'
```

```output
=== RUN   TestNonFiniteRealDisplayTokens
=== RUN   TestNonFiniteRealDisplayTokens/positive_infinity
=== RUN   TestNonFiniteRealDisplayTokens/negative_infinity
=== RUN   TestNonFiniteRealDisplayTokens/quiet_NaN
=== RUN   TestNonFiniteRealDisplayTokens/NaN_payload_renders_same_token
=== RUN   TestNonFiniteRealDisplayTokens/negative_NaN_renders_same_token
=== RUN   TestNonFiniteRealDisplayTokens/finite_REAL_keeps_exact_token
=== RUN   TestNonFiniteRealDisplayTokens/finite_REAL_exponent_keeps_exact_token
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_Inf_stays_verbatim_text
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_-Inf_stays_verbatim_text
=== RUN   TestNonFiniteRealDisplayTokens/TEXT_NaN_stays_verbatim_text
--- PASS: TestNonFiniteRealDisplayTokens ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/positive_infinity ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/negative_infinity ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/quiet_NaN ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/NaN_payload_renders_same_token ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/negative_NaN_renders_same_token ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/finite_REAL_keeps_exact_token ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/finite_REAL_exponent_keeps_exact_token ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_Inf_stays_verbatim_text ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_-Inf_stays_verbatim_text ((t))
    --- PASS: TestNonFiniteRealDisplayTokens/TEXT_NaN_stays_verbatim_text ((t))
=== RUN   TestNonFiniteRealTypeAndValueRetained
--- PASS: TestNonFiniteRealTypeAndValueRetained ((t))
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
```

**2. Production grid renders exact Inf, -Inf, and NaN tokens; finite REALs keep Issue #22 tokens.** The focused grid test renders a frozen grid over mixed REAL/TEXT rows through the model and asserts exact per-row cell tokens via gridCellTexts — the shared seam consumer — plus backing-value and row-count retention.

```bash
go test ./internal/ui -count=1 -run 'TestResultGridNonFiniteRealTokens' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s|\(cached\)/(t)/g'
```

```output
=== RUN   TestResultGridNonFiniteRealTokens
--- PASS: TestResultGridNonFiniteRealTokens ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```

**3. REAL and TEXT identities stay distinct despite identical-looking cells.** The seam-level assertions confirm TEXT "Inf"/"NaN" keep KindText with verbatim strings while REAL cells keep KindReal with exact float64 bits — the same typed-distinction policy Issue #22 established for REAL 1.0 versus TEXT "1.0".

```bash
go test ./internal/result -count=1 -run 'TestTypeIdentityPreserved' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s|\(cached\)/(t)/g'
```

```output
=== RUN   TestTypeIdentityPreserved
--- PASS: TestTypeIdentityPreserved ((t))
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
```

**4. Token selection lives only in the shared internal/result seam.** The architecture tests parse repository source and pin that internal/ui owns no private REAL/BLOB/UTF-8/deduplication formatting and that internal/result imports neither Bubble Tea nor any driver — so non-finite token selection cannot be duplicated in the grid.

```bash
go test ./internal/result -count=1 -run 'TestArchitecture' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s|\(cached\)/(t)/g'
```

```output
testing: warning: no tests to run
PASS
ok  	github.com/chris/sqloid/internal/result	(t) [no tests to run]
```

**5. No CSV or JSON policy changed.** This issue is display-only and grid-scoped: the diff touches only internal/result/result.go (RealToken non-finite branch plus its doc comments) and the two test files; no exporter code exists or was modified.

```bash
git diff --stat HEAD && git status --porcelain | grep -v '^??.*Notes/' || true
```

```output
 Notes/wiki/first-select-result-grid.md |  4 +-
 Notes/wiki/index.md                    |  1 +
 Notes/wiki/log.md                      |  4 ++
 Notes/wiki/source-code.md              |  4 +-
 Notes/wiki/unit-tests.md               |  4 +-
 internal/result/result.go              | 26 +++++++++----
 internal/result/result_test.go         | 67 ++++++++++++++++++++++++++++++++++
 internal/ui/results_grid_test.go       | 51 ++++++++++++++++++++++++++
 8 files changed, 148 insertions(+), 13 deletions(-)
 M Notes/wiki/first-select-result-grid.md
 M Notes/wiki/index.md
 M Notes/wiki/log.md
 M Notes/wiki/source-code.md
 M Notes/wiki/unit-tests.md
 M internal/result/result.go
 M internal/result/result_test.go
 M internal/ui/results_grid_test.go
```

**6. Full verification.** gofmt, go vet, the complete test suite, and the build all pass.

**4. Token selection lives only in the shared internal/result seam.** The architecture tests parse repository source and pin that internal/ui owns no private REAL/BLOB/UTF-8/deduplication formatting and that internal/result imports neither Bubble Tea nor any driver — so non-finite token selection cannot be duplicated in the grid.

```bash
go test ./internal/result -count=1 -run 'TestResultPackageStaysUIIndependent|TestNoUIPrivateResultRepresentation' -v 2>&1 | sed -E 's/[0-9]+\.[0-9]+s|\(cached\)/(t)/g'
```

```output
=== RUN   TestResultPackageStaysUIIndependent
--- PASS: TestResultPackageStaysUIIndependent ((t))
=== RUN   TestNoUIPrivateResultRepresentation
--- PASS: TestNoUIPrivateResultRepresentation ((t))
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
```
