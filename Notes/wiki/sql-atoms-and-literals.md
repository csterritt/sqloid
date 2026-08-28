# Universal Value Parsing and Safe SQL Atoms (Issue #14)

Issue #14 adds the UI-independent value-parsing, binding, and SQL-safety primitives to `internal/querybuilder`, implementing the **Numeric value parsing and rendering** and **SQL safety** decisions of [Notes/PRD-sqloid.md](../PRD-sqloid.md). Three files own the contract:

- `internal/querybuilder/value.go` — universal text parser (`ParseValue`), the `Value`/`ParsedKind` types, and the driver-facing bound-parameter conversion (`ParamValue`).
- `internal/querybuilder/sql_atoms.go` — schema-derived identifier quoting and closed typed choices for predicate operators, projection aggregates, and ordering directions, plus assembled `Predicate` objects.
- `internal/querybuilder/sql_literal.go` — the sole canonical standalone-SQL literal renderer (`RenderSQLLiteral` with `Literal`/`LiteralKind`).

The package still never imports `internal/ui`; parsing depends on no declared column type; Schema identities from `internal/schema` are the only source of identifiers.

## Verbatim INTEGER-first then finite-REAL grammar

`ParseValue(input)` performs deterministic classification on untrimmed input:

1. **INTEGER first**: exactly `-?[0-9]+` fitting signed int64 — leading zeros allowed, negative zero (`-0`) becomes integer `0`, and both int64 boundaries parse as INTEGER.
2. **REAL second**: any *finite* value accepted by `strconv.ParseFloat(s, 64)`, including decimal forms (`3.14`, `1.`, `.5`), exponent forms, and valid hexadecimal floating-point forms such as `0x1p2` / `0x1.8p1`. A leading `+` is rejected before both stages per the PRD's fall-through rule.
3. **TEXT fallback**: everything else is preserved verbatim — whitespace in any position, empty input, typed `NULL`, non-finite spellings (`NaN`, `Inf`, `Infinity`), float overflow (`1e400`), hexadecimal integers (`0x1A`), malformed hex floats (`0x1p`), and injection-shaped strings.

Integer overflow falls through: `9223372036854775808` classifies as REAL because ParseFloat accepts it finitely. There is no trimming or normalization anywhere; the original text is kept until classification completes, and TEXT values store it byte-for-byte.

## Binding: concrete parameter types, no SQL NULL from text

`Value.ParamValue()` returns stable concrete Go types for the SQLite driver: `int64` for INTEGER, `float64` for REAL, and the verbatim `string` for TEXT. Typed `NULL` and empty input remain strings rather than becoming SQL null — SQL NULL exists only through explicit popup/operator choices. No declared-type coercion happens in the builder; SQLite column affinity may coerce at bind time but the parser does not know or consult column types. BLOB input parsing does not exist; every user-entered value is bound via parameters, never interpolated into SQL text.

## Schema-only atom-by-atom identifier quoting

Identifiers enter query construction only through `internal/schema` object/column identity: `ObjectIdentifier(*schema.Object)` and `ColumnIdentifier(schema.Column)` each quote one atom using the shared `quoteIdentifierAtom` helper — double quotes around the whole name with each embedded double quote doubled (`tricky""name` → `"tricky""""name"`). The helper never parses schema qualification (`main.users` stays one quoted atom) and never accepts caller-authored SQL.

## Closed operator, aggregate, and direction choices

Fixed v1 choices are closed enums whose renderers emit only PRD-approved tokens and return typed errors for invalid zero or out-of-range values:

- `Operator`: `OpEq "="`, `OpNotEq "!="`, `OpLt "<"`, `OpLe "<="`, `OpGt ">"`, `OpGe ">="`, `OpIsNull "IS NULL"`, `OpIsNotNull "IS NOT NULL"`, `OpLike "LIKE"`.
- `Aggregate`: `AggCount COUNT`, `AggMin MIN`, `AggMax MAX`, `AggAvg AVG`, `AggSum SUM` (zero = unaggregated).
- `Direction`: `DirAsc ASC`, `DirDesc DESC`.

`NewPredicate(column, operator, value)` validates at construction; value-taking operators render `<quoted column> <token> ?` while IS NULL / IS NOT NULL take no value, and `Params()` keeps parsed values unchanged on the parameter list in deterministic order (SET-then-WHERE ordering belongs to future write builders).

## Canonical standalone literals

`RenderSQLLiteral(Literal)` is the one serializer available to Issues #40 and #48:

- **INTEGER**: exact decimal at boundaries (`9223372036854775807`, `-9223372036854775808`).
- **REAL**: `strconv.FormatFloat(v, 'g', -1, 64)` with `.0` appended iff the token contains none of `.`, `e`, `E` — preserving REAL identity for integral values (`4` → `4.0`), negative zero (`-0.0`, sign-bit tested), subnormals (`5e-324`), and exponent output (`1e+20`). Non-finite REAL returns a typed error instead of unsafe SQL.
- **TEXT**: single quotes with every embedded single quote doubled; empty string → `''`; whitespace, NUL bytes, and injection content survive verbatim inside the token.
- **NULL**: exactly `NULL`.
- **BLOB**: uppercase `X` with lowercase hex payload (`X'01abff'`, empty bytes → `X''`). BLOB remains renderable from typed data only — it is unsupported as universal user input.

`Value.Literal()` converts parsed values into matching literal kinds without re-parsing; a Value can never become a SQL null literal. Ordinary execution continues to use bindings; rendered literals serve destructive modal SQL and saved standalone SQL (`Query save targeting`: identifiers double-quoted, strings quote-doubled, `NULL` keyword, BLOBs as `X'hex'`, trailing semicolon added by the save flow).

## Testing

See the Issue #14 test catalog under [unit-tests.md](unit-tests.md); related contracts live in [schema-catalog.md](schema-catalog.md) (identifier provenance), [stale-schema-refresh.md](stale-schema-refresh.md) (fresh catalogs before quoting), and [early-integration-tracer.md](early-integration-tracer.md) (predecessor quoting rules now superseded by these shared atoms).
