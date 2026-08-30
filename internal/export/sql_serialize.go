// Full-statement standalone SQL serialization (Issue #48), per the Query
// save targeting decision in Notes/PRD-sqloid.md. The serializer is the
// UI-independent, database-free save boundary for the immutable complete
// query states shared by internal/history and internal/result: it renders
// every supported SELECT, UPDATE, DELETE, and INSERT structure in
// deterministic builder order, delegating every identifier, fixed SQL token,
// and INTEGER/REAL/TEXT/NULL/BLOB literal to Issue #14's canonical atoms
// (QuoteIdentifier, Operator/Aggregate/Direction SQLToken, RenderSQLLiteral)
// — never through any second literal serializer and never by interpolating
// untrusted raw SQL. The output is exactly one standalone executable
// statement with one trailing semicolon. Unsupported or incomplete states
// return typed errors; no picker is opened and the database is never
// consulted. Loading saved SQL is unsupported: this issue is one-way only.

package export

import (
	"errors"
	"strings"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// ErrUnsupportedQueryState reports a query state that cannot be serialized
// to one standalone executable statement: an unsupported command, an
// incomplete state (no table, empty SELECT projection, unsubmitted values),
// or an identity this package cannot resolve. No partial statement is ever
// returned alongside it.
var ErrUnsupportedQueryState = errors.New("query state cannot be serialized to SQL")

// SerializeSQLLiteral renders one typed literal through Issue #14's
// canonical RenderSQLLiteral atom. It exists so save flows that hold a typed
// literal directly (such as BLOB payloads that can never arrive from user
// text entry) serialize through the same single canonical atom as statement
// assembly — never a private re-implementation.
func SerializeSQLLiteral(l qb.Literal) (string, error) {
	return qb.RenderSQLLiteral(l)
}

// SerializeSQLQuery assembles exactly one standalone executable SQL
// statement from one immutable complete query state, with one trailing
// semicolon and no placeholders or bound parameters: every literal renders
// through Issue #14's atoms. Structure and ordering follow the deterministic
// builder order for each command. Incomplete states (missing table, empty
// SELECT projection, unsubmitted values, pending choices) and unsupported
// commands return ErrUnsupportedQueryState; unsafe or unresolvable pieces
// (invalid operators, unknown ORDER BY identities) return the same typed
// error rather than arbitrary text.
func SerializeSQLQuery(s qb.HistoryState) (string, error) {
	var statement string
	switch s.Command {
	case qb.CommandSelect:
		st, err := serializeSelect(s)
		if err != nil {
			return "", err
		}
		statement = st
	case qb.CommandUpdate:
		st, err := serializeUpdate(s)
		if err != nil {
			return "", err
		}
		statement = st
	case qb.CommandDelete:
		st, err := serializeDelete(s)
		if err != nil {
			return "", err
		}
		statement = st
	case qb.CommandInsert:
		st, err := serializeInsert(s)
		if err != nil {
			return "", err
		}
		statement = st
	default:
		return "", ErrUnsupportedQueryState
	}
	return statement + ";", nil
}

// renderWhereClause renders the shared optional WHERE clause from committed
// history predicate state, or empty text when none is committed. Value-taking
// operators require a submitted value; IS NULL / IS NOT NULL carry none.
// Operators render only through their closed typed tokens.
func renderWhereClause(s qb.HistoryState) (string, error) {
	if !s.WhereSet {
		return "", nil
	}
	token, err := s.WhereOperator.SQLToken()
	if err != nil {
		return "", ErrUnsupportedQueryState
	}
	clause := qb.QuoteIdentifier(s.WhereColumn) + " " + token
	if s.WhereOperator.TakesValue() {
		if !s.WhereHasValue {
			return "", ErrUnsupportedQueryState
		}
		literal, err := SerializeSQLLiteral(s.WhereValue.Literal())
		if err != nil {
			return "", err
		}
		clause += " " + literal
	}
	return "WHERE " + clause, nil
}

// renderProjectionEntries renders the committed SELECT list in commit order:
// the wildcard and COUNT(*) sentinels render their exact atoms, aggregated
// entries render TOKEN(quoted column), and plain entries render the quoted
// column atom. All identifiers quote through Issue #14's canonical atom.
func renderProjectionEntries(s qb.HistoryState) (string, error) {
	if len(s.Projection) == 0 {
		return "", ErrUnsupportedQueryState
	}
	if len(s.Projection) == 1 && s.Projection[0].Kind == qb.ProjectionWildcard {
		return "*", nil
	}
	parts := make([]string, 0, len(s.Projection))
	for _, e := range s.Projection {
		switch e.Kind {
		case qb.ProjectionWildcard:
			// A wildcard is valid only as the sole projection entry.
			return "", ErrUnsupportedQueryState
		case qb.ProjectionCountStar:
			parts = append(parts, "COUNT(*)")
		default:
			atom := qb.QuoteIdentifier(e.Column)
			if e.Aggregate > qb.AggregateValue {
				token, err := e.Aggregate.SQLToken()
				if err != nil {
					return "", ErrUnsupportedQueryState
				}
				parts = append(parts, token+"("+atom+")")
				continue
			}
			parts = append(parts, qb.QuoteIdentifier(e.Column))
		}
	}
	return strings.Join(parts, ", "), nil
}

// serializeSelect renders the complete SELECT: projection, WHERE, GROUP BY
// in commit order, the committed ORDER BY expression with its direction, and
// the accepted Limit.
func serializeSelect(s qb.HistoryState) (string, error) {
	projection, err := renderProjectionEntries(s)
	if err != nil {
		return "", err
	}
	parts := []string{"SELECT " + projection + " FROM " + qb.QuoteIdentifier(s.Table)}
	where, err := renderWhereClause(s)
	if err != nil {
		return "", err
	}
	if where != "" {
		parts = append(parts, where)
	}
	if len(s.Groups) > 0 {
		atoms := make([]string, len(s.Groups))
		for i, g := range s.Groups {
			atoms[i] = qb.QuoteIdentifier(g)
		}
		parts = append(parts, "GROUP BY "+strings.Join(atoms, ", "))
	}
	if s.OrderSet {
		expr, err := qb.HistoryOrderExpression(s.OrderExpression)
		if err != nil {
			return "", ErrUnsupportedQueryState
		}
		token, err := s.OrderDirection.SQLToken()
		if err != nil {
			return "", ErrUnsupportedQueryState
		}
		parts = append(parts, "ORDER BY "+expr+" "+token)
	}
	if s.LimitHas {
		limit, err := SerializeSQLLiteral(qb.Literal{Kind: qb.LiteralInteger, Int: s.LimitValue})
		if err != nil {
			return "", err
		}
		parts = append(parts, "LIMIT "+limit)
	}
	return strings.Join(parts, " "), nil
}

// serializeUpdate renders the deterministic builder order: SET assignments in
// SET order with preserved Value/NULL choices, then the optional predicate.
// Incomplete states return a typed error.
func serializeUpdate(s qb.HistoryState) (string, error) {
	if !s.TableSet || s.Table == "" || len(s.Sets) == 0 {
		return "", ErrUnsupportedQueryState
	}
	assignments := make([]string, 0, len(s.Sets))
	for _, assignment := range s.Sets {
		var value string
		switch assignment.Choice {
		case qb.SetChoiceNull:
			literal, err := SerializeSQLLiteral(qb.Literal{Kind: qb.LiteralNull})
			if err != nil {
				return "", err
			}
			value = literal
		case qb.SetChoiceValue:
			if !assignment.HasValue {
				return "", ErrUnsupportedQueryState
			}
			literal, err := SerializeSQLLiteral(assignment.Value.Literal())
			if err != nil {
				return "", err
			}
			value = literal
		default:
			return "", ErrUnsupportedQueryState // pending or invalid choice
		}
		assignments = append(assignments, qb.QuoteIdentifier(assignment.Column)+" = "+value)
	}
	statement := "UPDATE " + qb.QuoteIdentifier(s.Table) + " SET " + strings.Join(assignments, ", ")
	where, err := renderWhereClause(s)
	if err != nil {
		return "", err
	}
	if where != "" {
		statement += " " + where
	}
	return statement, nil
}

// serializeDelete renders the complete DELETE: one quoted table atom and,
// when a predicate is committed, its complete WHERE clause appended exactly
// once. The absent-WHERE bare form targets every row.
func serializeDelete(s qb.HistoryState) (string, error) {
	if !s.TableSet || s.Table == "" {
		return "", ErrUnsupportedQueryState
	}
	statement := "DELETE FROM " + qb.QuoteIdentifier(s.Table)
	where, err := renderWhereClause(s)
	if err != nil {
		return "", err
	}
	if where != "" {
		statement += " " + where
	}
	return statement, nil
}

// serializeInsert renders the complete INSERT in schema prompt order:
// Value columns contribute a rendered literal, NULL columns stay as the
// SQL keyword, and Default/Omit columns are absent entirely. When every
// prompted column is omitted, the exact DEFAULT VALUES form is emitted.
func serializeInsert(s qb.HistoryState) (string, error) {
	if !s.TableSet || s.Table == "" {
		return "", ErrUnsupportedQueryState
	}
	columns := make([]string, 0, len(s.Inserts))
	values := make([]string, 0, len(s.Inserts))
	for _, c := range s.Inserts {
		switch c.Choice {
		case qb.InsertChoiceValue:
			if !c.HasValue {
				return "", ErrUnsupportedQueryState
			}
			literal, err := SerializeSQLLiteral(c.Value.Literal())
			if err != nil {
				return "", err
			}
			columns = append(columns, qb.QuoteIdentifier(c.Column))
			values = append(values, literal)
		case qb.InsertChoiceNull:
			columns = append(columns, qb.QuoteIdentifier(c.Column))
			values = append(values, "NULL")
		case qb.InsertChoiceOmit:
			// Default/Omit: absent from both lists entirely.
		default:
			return "", ErrUnsupportedQueryState // incomplete choice
		}
	}
	if len(columns) == 0 {
		return "INSERT INTO " + qb.QuoteIdentifier(s.Table) + " DEFAULT VALUES", nil
	}
	return "INSERT INTO " + qb.QuoteIdentifier(s.Table) +
		" (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(values, ", ") + ")", nil
}
