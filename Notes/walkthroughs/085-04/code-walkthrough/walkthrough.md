# Issue #085 Code Walkthrough: Centralize Finite REAL Token Generation

*2026-09-03T23:59:39Z by Showboat 0.6.1*
<!-- showboat-id: 59b9ad63-ba7e-444d-b27f-a00abcc5cebd -->

Issue #85 (Notes/tasks/085-centralize-finite-real-tokens.md, Notes/PRD-sqloid.md Numeric value parsing and rendering and Export formats and values decisions) centralizes finite REAL token generation into the single canonical internal/result.RealToken implementation. Before the edit, internal/querybuilder carried a duplicate realToken formatter (strconv.FormatFloat plus the .0 suffix rule) alongside the canonical one in internal/result. The edit removes the duplicate and makes RenderSQLLiteral's finite REAL branch delegate to result.RealToken, so the formatting-plus-suffix algorithm has exactly one implementation in the repository, shared by grid, CSV, JSON, and query/saved-SQL literals. Querybuilder keeps its typed non-finite rejection before the shared call, because result.RealToken intentionally returns display/export tokens (Inf, -Inf, NaN) for non-finite database values rather than rejecting them. Reference: Issue #85 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/085-04/code-walkthrough/.

## The single canonical finite REAL token implementation

The formatting-plus-suffix algorithm lives in exactly one place: internal/result/result.go. RealToken branches on math.IsInf/math.IsNaN for non-finite display tokens (Inf, -Inf, NaN per Issue #23), then for finite values runs strconv.FormatFloat(v, 'g', -1, 64) and appends .0 iff the token contains none of '.', 'e', or 'E', preserving REAL identity for integral values, negative zero, subnormals, and exponent output. The token is locale-independent by construction of strconv.

```bash
sed -n '/\/\/ RealToken returns the exact PRD REAL token/,/^}/p' /home/chris/sqloid/internal/result/result.go
```

```output
// RealToken returns the exact PRD REAL token. For positive infinity,
// negative infinity, and any NaN payload it is exactly `Inf`, `-Inf`, and
// `NaN` respectively (Issue #23), never strconv's `+Inf` and never a
// payload-specific form. Finite values use the shortest round-tripping
// 'g'-format float64 representation, with ".0" appended exactly when the
// token contains none of '.', 'e', or 'E' so REAL identity survives for
// values such as 1.0, -0.0, and 1e+20. The token is locale-independent by
// construction of strconv.
func RealToken(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	token := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(token, ".eE") {
		token += ".0"
	}
	return token
}
```

## Query-literal delegation to the canonical token

RenderSQLLiteral in internal/querybuilder/sql_literal.go now delegates finite REAL formatting to result.RealToken. The typed non-finite rejection stays before the shared call: isNonFinite(l.Real) returns a typed error and empty token for Inf/-Inf/NaN, because result.RealToken would return display/export tokens for those values rather than rejecting them. The package imports internal/result (one-way: internal/result imports neither internal/querybuilder nor any UI/driver package).

```bash
sed -n '/\/\/ RenderSQLLiteral renders one typed literal/,/^}/p' /home/chris/sqloid/internal/querybuilder/sql_literal.go
```

```output
// RenderSQLLiteral renders one typed literal into its exact standalone SQL
// token per Notes/PRD-sqloid.md: INTEGER as canonical decimal, REAL via the
// locale-independent shortest round-trip token with `.0` appended when it
// contains none of `.`, `e`, or `E`, TEXT double-quoted inside single quotes,
// exactly `NULL`, and BLOB as `X'hex'`. Finite REAL tokens delegate to the
// single canonical result.RealToken shared by grid, CSV, and JSON, so there
// is one finite REAL formatting implementation across consumers. Non-finite
// REAL values return a typed error rather than producing unsafe or
// nonportable SQL; this rejection happens before the shared call because
// result.RealToken intentionally returns display/export tokens (Inf, -Inf,
// NaN) for non-finite database values rather than rejecting them.
func RenderSQLLiteral(l Literal) (string, error) {
	switch l.Kind {
	case LiteralNull:
		return "NULL", nil
	case LiteralText:
		return quoteTextLiteral(l.Text), nil
	case LiteralInteger:
		return strconv.FormatInt(l.Int, 10), nil
	case LiteralReal:
		if isNonFinite(l.Real) {
			return "", fmt.Errorf("cannot render non-finite REAL %v", l.Real)
		}
		return result.RealToken(l.Real), nil
	case LiteralBlob:
		var b strings.Builder
		b.Grow(4 + 2*len(l.Blob))
		b.WriteString("X'")
		for _, c := range l.Blob {
			const hexdigits = "0123456789abcdef"
			b.WriteByte(hexdigits[c>>4])
			b.WriteByte(hexdigits[c&0x0f])
		}
		b.WriteByte('\'')
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported literal kind %d", int(l.Kind))
	}
}
```

## The duplicate formatter is gone from value.go

The duplicate realToken function was removed from internal/querybuilder/value.go. The file now owns only the universal parser, the bound-parameter conversion, the isNonFinite helper, and quoteTextLiteral. There is no second FormatFloat-plus-suffix implementation in the querybuilder package.

```bash
grep -n 'func realToken\|FormatFloat\|ContainsAny' /home/chris/sqloid/internal/querybuilder/value.go; echo 'exit: '$?
```

```output
exit: 1
```

## Supplemental static evidence: one formatting-plus-suffix implementation

A focused grep over production (non-test) Go source confirms exactly one strconv.FormatFloat call and one ContainsAny(".eE") suffix rule remain in the repository, both in internal/result/result.go. No duplicate lives in internal/querybuilder, internal/export, internal/ui, or cmd/.

```bash
cd /home/chris/sqloid && echo '=== FormatFloat in production source ===' && grep -rn 'FormatFloat' --include='*.go' --exclude='*_test.go' internal/ cmd/ && echo '=== ContainsAny(".eE") suffix rule in production source ===' && grep -rn 'ContainsAny.*\.eE' --include='*.go' --exclude='*_test.go' internal/ cmd/
```

```output
=== FormatFloat in production source ===
internal/result/result.go:154:	token := strconv.FormatFloat(v, 'g', -1, 64)
=== ContainsAny(".eE") suffix rule in production source ===
internal/result/result.go:155:	if !strings.ContainsAny(token, ".eE") {
```

## Representative finite values traced through every consumer

A small Go program renders the representative finite values (1.0, -0.0, 1e+20, the smallest subnormal, and a precision-edge value) through every consumer that now shares the canonical token: result.RealToken (grid/CSV/JSON), and querybuilder.RenderSQLLiteral (query/saved-SQL literals). Every consumer produces the identical token for each value, proving the single shared call path.

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/085-04/code-walkthrough/
```

```output
=== Finite REAL tokens: identical across result.RealToken and RenderSQLLiteral ===
value                                  result.RealToken             RenderSQLLiteral             match
1.0 (integral REAL)                    1.0                          1.0                          OK
-0.0 (negative zero)                   -0.0                         -0.0                         OK
1e+20 (exponent)                       1e+20                        1e+20                        OK
5e-324 (smallest subnormal)            5e-324                       5e-324                       OK
1.7976931348623157e+308 (max finite)   1.7976931348623157e+308      1.7976931348623157e+308      OK
0.30000000000000004 (precision edge)   0.3                          0.3                          OK
nextafter(1, 0) (adjacent)             0.9999999999999999           0.9999999999999999           OK
nextafter(1, 2) (adjacent)             1.0000000000000002           1.0000000000000002           OK

=== Non-finite REAL: query literals reject, result formatting retains tokens ===
value                  result.RealToken   SQL token    query policy
positive infinity      Inf                ""           rejects (err=cannot render non-finite REAL +Inf)
negative infinity      -Inf               ""           rejects (err=cannot render non-finite REAL -Inf)
not a number           NaN                ""           rejects (err=cannot render non-finite REAL NaN)
```

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/085-04/code-walkthrough/
```

```output
=== Finite REAL tokens: identical across result.RealToken and RenderSQLLiteral ===
value                                  result.RealToken             RenderSQLLiteral             match
1.0 (integral REAL)                    1.0                          1.0                          OK
-0.0 (negative zero)                   -0.0                         -0.0                         OK
1e+20 (exponent)                       1e+20                        1e+20                        OK
5e-324 (smallest subnormal)            5e-324                       5e-324                       OK
1.7976931348623157e+308 (max finite)   1.7976931348623157e+308      1.7976931348623157e+308      OK
0.30000000000000004 (precision edge)   0.3                          0.3                          OK
nextafter(1, 0) (adjacent)             0.9999999999999999           0.9999999999999999           OK
nextafter(1, 2) (adjacent)             1.0000000000000002           1.0000000000000002           OK

=== Non-finite REAL: query literals reject, result formatting retains tokens ===
value                  result.RealToken   SQL token    query policy
positive infinity      Inf                ""           rejects (err=cannot render non-finite REAL +Inf)
negative infinity      -Inf               ""           rejects (err=cannot render non-finite REAL -Inf)
not a number           NaN                ""           rejects (err=cannot render non-finite REAL NaN)
```

## Behavioral parity tests pass unchanged

The Issue #85 Task 1 cross-package contract tests in internal/querybuilder/sql_literal_test.go lock identical finite REAL tokens across consumers before the implementation deduplication, and they pass unchanged after the delegation. TestRenderRealLiteralsMatchCanonicalRealToken compares RenderSQLLiteral output to result.RealToken for integral, negative-zero, exponent, subnormal, max-finite, and adjacent/precision-edge values with round-trip, locale-independence, and REAL-identity assertions. TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy pins that query literals reject Inf/-Inf/NaN while result.RealToken retains its non-finite display tokens. The pre-existing TestRenderRealLiterals and TestValueConvertsToLiteralWithoutReparsing also remain green.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/querybuilder/ -run 'TestRenderRealLiteralsMatchCanonicalRealToken|TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy|TestRenderRealLiterals|TestValueConvertsToLiteralWithoutReparsing' -v 2>&1 | sed 's/	0\.[0-9]*s/	TIMEs/g; s/0\.[0-9]*s/TIMEs/g'
```

```output
=== RUN   TestRenderRealLiterals
--- PASS: TestRenderRealLiterals (TIMEs)
=== RUN   TestValueConvertsToLiteralWithoutReparsing
--- PASS: TestValueConvertsToLiteralWithoutReparsing (TIMEs)
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/integral_REAL_1.0
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/negative_zero
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/exponent_1e+20
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/smallest_subnormal
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/maximum_finite_float
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/precision_edge_0.1+0.2
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_below_1
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_above_1
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_below_MaxFloat64
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/negative_integral
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/fractional
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/large_integral
=== RUN   TestRenderRealLiteralsMatchCanonicalRealToken/negative_exponent
--- PASS: TestRenderRealLiteralsMatchCanonicalRealToken (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/integral_REAL_1.0 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/negative_zero (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/exponent_1e+20 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/smallest_subnormal (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/maximum_finite_float (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/precision_edge_0.1+0.2 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_below_1 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_above_1 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/nextafter_below_MaxFloat64 (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/negative_integral (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/fractional (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/large_integral (TIMEs)
    --- PASS: TestRenderRealLiteralsMatchCanonicalRealToken/negative_exponent (TIMEs)
=== RUN   TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy
=== RUN   TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/positive_infinity
=== RUN   TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/negative_infinity
=== RUN   TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/not_a_number
--- PASS: TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy (TIMEs)
    --- PASS: TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/positive_infinity (TIMEs)
    --- PASS: TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/negative_infinity (TIMEs)
    --- PASS: TestRenderRealLiteralsRejectNonFiniteWhileResultRetainsPolicy/not_a_number (TIMEs)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	TIMEs
```

## Focused and repository-wide verification

The focused internal/querybuilder, internal/result, and internal/export behavioral tests pass unchanged, confirming no consumer contract changed. Repository-wide go vet, go test, and go build all pass.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/querybuilder/ ./internal/result/ ./internal/export/ 2>&1 | sed 's/0\.[0-9]*s/TIMEs/g'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	TIMEs
ok  	github.com/chris/sqloid/internal/result	TIMEs
ok  	github.com/chris/sqloid/internal/export	TIMEs
```

```bash
cd /home/chris/sqloid && go vet ./... && go build ./... && echo 'vet+build OK'
```

```output
vet+build OK
```

```bash
cd /home/chris/sqloid && go test ./... 2>&1 | tail -20 | sed 's/0\.[0-9]*s/TIMEs/g'
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/070-06/code-walkthrough	[no test files]
?   	github.com/chris/sqloid/Notes/walkthroughs/085-04/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

## Shipped-binary/TUI evidence is dependent on Issue #57

Package-level evidence (cross-package behavioral parity tests, focused grep/static build check, repository-wide go test/go vet/go build) is valid before Issue #57. Shipped-TUI manual and end-to-end comparison must be rerun after Issue #57 Phase A lands, because the production composition root that bridges internal/connection and internal/ui is delivered by Issue #57. Reference: Issue #85 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/085-04/code-walkthrough/.
