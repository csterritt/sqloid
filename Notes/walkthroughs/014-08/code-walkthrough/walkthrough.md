# Issue #14: Universal value parsing and safe SQL atoms — code walkthrough

*2026-08-27T07:28:05Z by Showboat 0.6.1*
<!-- showboat-id: b6817aae-7c65-4cc0-a18d-aad6ed8872a7 -->

Evidence for Issue #14 per Notes/tasks/014-universal-value-parsing-and-safe-sql-atoms.md, implementing the Numeric value parsing and rendering and SQL safety decisions of Notes/PRD-sqloid.md. Workdir: sqloid repo root; go test timing strings are normalized so every block re-executes byte-identically.

## 1. Universal parsing: int64 boundaries, signs, leading zeros

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestParseValueClassifiesVerbatimText/(max_int64|min_int64|negative_leading_zeros|negative_zero_integer|leading_zeros|plain_integer)" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestParseValueClassifiesVerbatimText
=== RUN   TestParseValueClassifiesVerbatimText/plain_integer
=== RUN   TestParseValueClassifiesVerbatimText/leading_zeros
=== RUN   TestParseValueClassifiesVerbatimText/negative_leading_zeros
=== RUN   TestParseValueClassifiesVerbatimText/negative_zero_integer
=== RUN   TestParseValueClassifiesVerbatimText/max_int64
=== RUN   TestParseValueClassifiesVerbatimText/min_int64
--- PASS: TestParseValueClassifiesVerbatimText (t)
    --- PASS: TestParseValueClassifiesVerbatimText/plain_integer (t)
    --- PASS: TestParseValueClassifiesVerbatimText/leading_zeros (t)
    --- PASS: TestParseValueClassifiesVerbatimText/negative_leading_zeros (t)
    --- PASS: TestParseValueClassifiesVerbatimText/negative_zero_integer (t)
    --- PASS: TestParseValueClassifiesVerbatimText/max_int64 (t)
    --- PASS: TestParseValueClassifiesVerbatimText/min_int64 (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 2. Decimal and hexadecimal finite REALs, plus overflow / non-finite / malformed fallbacks

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestParseValueClassifiesVerbatimText/(decimal|trailing_dot|leading_dot|hexadecimal|float_overflow|NaN|Inf)" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestParseValueClassifiesVerbatimText
=== RUN   TestParseValueClassifiesVerbatimText/decimal
=== RUN   TestParseValueClassifiesVerbatimText/trailing_dot
=== RUN   TestParseValueClassifiesVerbatimText/leading_dot
=== RUN   TestParseValueClassifiesVerbatimText/decimal_zero
=== RUN   TestParseValueClassifiesVerbatimText/negative_decimal_zero_keeps_sign_bit
=== RUN   TestParseValueClassifiesVerbatimText/hexadecimal_float
=== RUN   TestParseValueClassifiesVerbatimText/negative_hexadecimal_float
=== RUN   TestParseValueClassifiesVerbatimText/hexadecimal_float_fractional_mantissa
=== RUN   TestParseValueClassifiesVerbatimText/leading_plus_hexadecimal_float_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/float_overflow_to_+Inf_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/negative_float_overflow_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/NaN_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/Inf_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/-Inf_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/Infinity_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/hexadecimal_integer_is_TEXT
=== RUN   TestParseValueClassifiesVerbatimText/malformed_hexadecimal_float_is_TEXT
--- PASS: TestParseValueClassifiesVerbatimText (t)
    --- PASS: TestParseValueClassifiesVerbatimText/decimal (t)
    --- PASS: TestParseValueClassifiesVerbatimText/trailing_dot (t)
    --- PASS: TestParseValueClassifiesVerbatimText/leading_dot (t)
    --- PASS: TestParseValueClassifiesVerbatimText/decimal_zero (t)
    --- PASS: TestParseValueClassifiesVerbatimText/negative_decimal_zero_keeps_sign_bit (t)
    --- PASS: TestParseValueClassifiesVerbatimText/hexadecimal_float (t)
    --- PASS: TestParseValueClassifiesVerbatimText/negative_hexadecimal_float (t)
    --- PASS: TestParseValueClassifiesVerbatimText/hexadecimal_float_fractional_mantissa (t)
    --- PASS: TestParseValueClassifiesVerbatimText/leading_plus_hexadecimal_float_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/float_overflow_to_+Inf_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/negative_float_overflow_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/NaN_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/Inf_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/-Inf_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/Infinity_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/hexadecimal_integer_is_TEXT (t)
    --- PASS: TestParseValueClassifiesVerbatimText/malformed_hexadecimal_float_is_TEXT (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 3. Signs, whitespace in every position, empty text, typed NULL, and injection-looking input

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestParseValueParamValues/(leading_plus|space|tab|empty_text|typed_NULL|quote_injection|statement_injection|int64_overflow_max)" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestParseValueParamValues
=== RUN   TestParseValueParamValues/int64_overflow_max+1_becomes_REAL
=== RUN   TestParseValueParamValues/leading_plus_integer_is_TEXT
=== RUN   TestParseValueParamValues/leading_plus_float_is_TEXT
=== RUN   TestParseValueParamValues/leading_plus_hexadecimal_float_is_TEXT
=== RUN   TestParseValueParamValues/leading_space
=== RUN   TestParseValueParamValues/trailing_space
=== RUN   TestParseValueParamValues/internal_space
=== RUN   TestParseValueParamValues/space-only_input
=== RUN   TestParseValueParamValues/tab_and_newline_padding
=== RUN   TestParseValueParamValues/space_around_negative_integer
=== RUN   TestParseValueParamValues/empty_text
=== RUN   TestParseValueParamValues/typed_NULL_is_TEXT
=== RUN   TestParseValueParamValues/quote_injection
=== RUN   TestParseValueParamValues/statement_injection
--- PASS: TestParseValueParamValues (t)
    --- PASS: TestParseValueParamValues/int64_overflow_max+1_becomes_REAL (t)
    --- PASS: TestParseValueParamValues/leading_plus_integer_is_TEXT (t)
    --- PASS: TestParseValueParamValues/leading_plus_float_is_TEXT (t)
    --- PASS: TestParseValueParamValues/leading_plus_hexadecimal_float_is_TEXT (t)
    --- PASS: TestParseValueParamValues/leading_space (t)
    --- PASS: TestParseValueParamValues/trailing_space (t)
    --- PASS: TestParseValueParamValues/internal_space (t)
    --- PASS: TestParseValueParamValues/space-only_input (t)
    --- PASS: TestParseValueParamValues/tab_and_newline_padding (t)
    --- PASS: TestParseValueParamValues/space_around_negative_integer (t)
    --- PASS: TestParseValueParamValues/empty_text (t)
    --- PASS: TestParseValueParamValues/typed_NULL_is_TEXT (t)
    --- PASS: TestParseValueParamValues/quote_injection (t)
    --- PASS: TestParseValueParamValues/statement_injection (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 4. Schema-derived identifier quoting: embedded quotes, keyword/qualification-shaped, punctuation-heavy names

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestObjectIdentifierQuotesOneAtom|TestColumnIdentifierQuotesOneAtom" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestObjectIdentifierQuotesOneAtom
=== RUN   TestObjectIdentifierQuotesOneAtom/users
=== RUN   TestObjectIdentifierQuotesOneAtom/#00
=== RUN   TestObjectIdentifierQuotesOneAtom/with_space
=== RUN   TestObjectIdentifierQuotesOneAtom/select
=== RUN   TestObjectIdentifierQuotesOneAtom/FROM
=== RUN   TestObjectIdentifierQuotesOneAtom/main.users
=== RUN   TestObjectIdentifierQuotesOneAtom/tricky""name
=== RUN   TestObjectIdentifierQuotesOneAtom/o'brien
=== RUN   TestObjectIdentifierQuotesOneAtom/;_DROP_TABLE_users--
=== RUN   TestObjectIdentifierQuotesOneAtom/?
--- PASS: TestObjectIdentifierQuotesOneAtom (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/users (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/#00 (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/with_space (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/select (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/FROM (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/main.users (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/tricky""name (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/o'brien (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/;_DROP_TABLE_users-- (t)
    --- PASS: TestObjectIdentifierQuotesOneAtom/? (t)
=== RUN   TestColumnIdentifierQuotesOneAtom
=== RUN   TestColumnIdentifierQuotesOneAtom/users
=== RUN   TestColumnIdentifierQuotesOneAtom/#00
=== RUN   TestColumnIdentifierQuotesOneAtom/with_space
=== RUN   TestColumnIdentifierQuotesOneAtom/select
=== RUN   TestColumnIdentifierQuotesOneAtom/FROM
=== RUN   TestColumnIdentifierQuotesOneAtom/main.users
=== RUN   TestColumnIdentifierQuotesOneAtom/tricky""name
=== RUN   TestColumnIdentifierQuotesOneAtom/o'brien
=== RUN   TestColumnIdentifierQuotesOneAtom/;_DROP_TABLE_users--
=== RUN   TestColumnIdentifierQuotesOneAtom/?
--- PASS: TestColumnIdentifierQuotesOneAtom (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/users (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/#00 (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/with_space (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/select (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/FROM (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/main.users (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/tricky""name (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/o'brien (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/;_DROP_TABLE_users-- (t)
    --- PASS: TestColumnIdentifierQuotesOneAtom/? (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 5. Exhaustive fixed operator / aggregate / direction tokens with invalid-value rejection

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestOperatorSQLTokens|TestAggregateSQLTokens|TestDirectionSQLTokens" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestOperatorSQLTokens
--- PASS: TestOperatorSQLTokens (t)
=== RUN   TestAggregateSQLTokens
--- PASS: TestAggregateSQLTokens (t)
=== RUN   TestDirectionSQLTokens
--- PASS: TestDirectionSQLTokens (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 6. User values stay bound: '?' placeholders only, unchanged parsed types, no value leakage into SQL text

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestPredicateBindsValues|TestNullOperatorPredicatesTakeNoValue" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestPredicateBindsValues
=== RUN   TestPredicateBindsValues/injection_text
=== RUN   TestPredicateBindsValues/injection_integer_shape
=== RUN   TestPredicateBindsValues/typed_NULL_text
=== RUN   TestPredicateBindsValues/integer
=== RUN   TestPredicateBindsValues/real
--- PASS: TestPredicateBindsValues (t)
    --- PASS: TestPredicateBindsValues/injection_text (t)
    --- PASS: TestPredicateBindsValues/injection_integer_shape (t)
    --- PASS: TestPredicateBindsValues/typed_NULL_text (t)
    --- PASS: TestPredicateBindsValues/integer (t)
    --- PASS: TestPredicateBindsValues/real (t)
=== RUN   TestNullOperatorPredicatesTakeNoValue
--- PASS: TestNullOperatorPredicatesTakeNoValue (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 7. Canonical standalone literals: INTEGER boundaries, REAL identity (integral and -0.0), quote-doubled TEXT, NULL, empty/nonempty BLOB

```bash
go test ./internal/querybuilder/ -count=1 -v -run "TestRenderIntegerLiterals|TestRenderRealLiterals|TestRenderTextLiterals|TestRenderNullAndBlobLiterals|TestValueConvertsToLiteralWithoutReparsing" 2>&1 | grep -E . | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g"
```

```output
=== RUN   TestRenderIntegerLiterals
--- PASS: TestRenderIntegerLiterals (t)
=== RUN   TestRenderRealLiterals
--- PASS: TestRenderRealLiterals (t)
=== RUN   TestRenderTextLiterals
--- PASS: TestRenderTextLiterals (t)
=== RUN   TestRenderNullAndBlobLiterals
=== RUN   TestRenderNullAndBlobLiterals/null_keyword
=== RUN   TestRenderNullAndBlobLiterals/empty_blob
=== RUN   TestRenderNullAndBlobLiterals/blob_zero_byte
=== RUN   TestRenderNullAndBlobLiterals/blob_mixed_bytes
=== RUN   TestRenderNullAndBlobLiterals/invalid_kind_rejected
--- PASS: TestRenderNullAndBlobLiterals (t)
    --- PASS: TestRenderNullAndBlobLiterals/null_keyword (t)
    --- PASS: TestRenderNullAndBlobLiterals/empty_blob (t)
    --- PASS: TestRenderNullAndBlobLiterals/blob_zero_byte (t)
    --- PASS: TestRenderNullAndBlobLiterals/blob_mixed_bytes (t)
    --- PASS: TestRenderNullAndBlobLiterals/invalid_kind_rejected (t)
=== RUN   TestValueConvertsToLiteralWithoutReparsing
--- PASS: TestValueConvertsToLiteralWithoutReparsing (t)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

## 8. Full package verification and the shared-renderer ownership point

```bash
go vet ./internal/querybuilder/ && go test ./internal/querybuilder/ -count=1 | sed -E "s/\((cached|[0-9]+\.[0-9]+s)\)/(t)/g" && ls internal/querybuilder/value.go internal/querybuilder/sql_atoms.go internal/querybuilder/sql_literal.go
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
internal/querybuilder/sql_atoms.go
internal/querybuilder/sql_literal.go
internal/querybuilder/value.go
```

sql_literal.go is the one canonical serializer for standalone SQL literals; Issues #40 (destructive modal SQL) and #48 (Query save targeting) consume it rather than adding private renderers, while ordinary query execution keeps binding every user-entered value through the Task 2 ParamValue contract. Cross-references: Issue #14 tasks in Notes/tasks/014-universal-value-parsing-and-safe-sql-atoms.md and the Query Grammar / Numeric value parsing and rendering / SQL safety decisions of Notes/PRD-sqloid.md.
