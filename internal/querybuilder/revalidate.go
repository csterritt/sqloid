// Immutable pre-execution revalidation transition for builder state (Issue
// #21), per the Builder lifecycle and Runnable-State Contract decisions in
// Notes/PRD-sqloid.md. When pre-execution schema-version validation observed
// a changed PRAGMA schema_version, the UI installs the refreshed catalog
// here; the transition revalidates object identity, command eligibility,
// every referenced identifier, INSERT column insertability, and the selected
// object's rowid capability and declared-rowid shadowing, then clears only
// the state transitively dependent on an invalidated prerequisite while
// preserving unrelated completed builder state. The authoritative runnable
// report of the repaired snapshot identifies the first specific invalid
// field and reason deterministically; this transition itself performs no
// validation workflow, history effect, or execution.
package querybuilder

import (
	"strings"

	"github.com/chris/sqloid/internal/schema"
)

// RevalidateReport summarizes one revalidation transition: whether any
// dependent builder state was cleared, and the authoritative post-repair
// runnable report that names the first invalid field and reason when the
// repaired snapshot is not runnable.
type RevalidateReport struct {
	Cleared bool           // true when any dependent builder state was dropped
	Report  RunnableReport // authoritative runnable evaluation of the repaired snapshot
}

// Revalidate installs the refreshed catalog snapshot as the builder's object
// identities and capabilities and repairs dependent state. A nil catalog is
// an unchanged no-op returning the authoritative report of the receiver.
// Revalidation rules:
//
//   - A selected object that vanished or lost eligibility for the current
//     command drops the table selection and everything downstream of it,
//     exactly like a vanished-table catalog refresh.
//   - A change in the selected object's rowid capability or declared-rowid
//     shadowing invalidates the committed ORDER BY expression, the only v1
//     state that consumes rowid addressing.
//   - Every referenced identifier is revalidated against the refreshed
//     columns: vanished projection entries, GROUP BY names, committed WHERE
//     predicates (and WHERE drafts), and ORDER BY column expressions are
//     removed individually; UPDATE SET assignments for vanished columns and
//     INSERT prompts for columns that became hidden/non-insertable are
//     removed individually.
//
// Unrelated completed state — Limit, surviving projection/group/prompt
// entries, surviving assignments — is always preserved, and focus follows
// the established clearing rule only through the removed states.
func (q QueryBuilder) Revalidate(c *schema.Catalog) (QueryBuilder, RevalidateReport) {
	if c == nil {
		return q, RevalidateReport{Report: q.RunnableReport()}
	}

	next := q
	next.objects = c.Objects
	cleared := false

	if q.tableSet {
		obj := next.findObjectIn(c.Objects, q.table)
		eligible := obj != nil && (obj.WriteEligible || q.command == CommandSelect)
		if !eligible {
			// The invalidated object drops the table and every downstream
			// state that referenced it.
			next.table, next.tableSet = "", false
			next.discardSelectors()
			next.downstreamG++
			return next, RevalidateReport{Cleared: true, Report: next.RunnableReport()}
		}
		if old := q.findObjectIn(q.objects, q.table); old != nil &&
			(old.Rowid != obj.Rowid || old.RowidShadowed != obj.RowidShadowed) && next.orderSet {
			// The rowid property changed: drop the rowid-addressing consumer.
			next = next.ClearOrderBy()
			cleared = true
		}
	}

	visible := visibleNames(next.findObjectIn(c.Objects, next.table))

	// Projection entries naming vanished columns are removed individually.
	if len(next.projection) > 0 {
		kept := make([]ProjectionEntry, 0, len(next.projection))
		for _, e := range next.projection {
			if e.Kind == ProjectionColumn && !visible[e.Column] {
				cleared = true
				continue
			}
			kept = append(kept, e)
		}
		next.projection = kept
	}

	// GROUP BY names that vanished are removed individually.
	if len(next.groups) > 0 {
		kept := make([]string, 0, len(next.groups))
		for _, g := range next.groups {
			if !visible[g] {
				cleared = true
				continue
			}
			kept = append(kept, g)
		}
		next.groups = kept
	}

	// A committed WHERE predicate or open draft naming a vanished column is
	// cleared whole: its predicate depends entirely on that identifier.
	if next.whereSet {
		if name, ok := next.where.Column(); ok && !visible[name.Name] {
			next.discardWhere()
			cleared = true
		}
	}
	if next.whereDrafting {
		if name, ok := next.whereDraft.Column(); ok && !visible[name.Name] {
			next.whereDrafting = false
			next.whereDraft = AbsentWhere()
			cleared = true
		}
	}

	// An ORDER BY expression over a vanished column is removed.
	if next.orderSet && next.orderKey != "order-count-star" {
		name, known := orderKeyColumn(next.orderKey)
		if !known || !visible[name] {
			next = next.ClearOrderBy()
			cleared = true
		}
	}

	// UPDATE SET assignments for vanished columns are removed individually.
	if len(next.sets) > 0 {
		kept := make([]SetAssignment, 0, len(next.sets))
		for _, a := range next.sets {
			if !visible[a.Column] {
				cleared = true
				continue
			}
			kept = append(kept, a)
		}
		next.sets = kept
	}

	// INSERT prompts for columns that are no longer insertable — dropped, or
	// newly hidden/generated — are removed individually.
	if len(next.inserts) > 0 {
		insertable := insertableNames(next.findObjectIn(c.Objects, next.table))
		kept := make([]InsertColumn, 0, len(next.inserts))
		for _, p := range next.inserts {
			if !insertable[p.Column] {
				cleared = true
				continue
			}
			kept = append(kept, p)
		}
		next.inserts = kept
	}

	if cleared {
		next.downstreamG++
	}
	return next, RevalidateReport{Cleared: cleared, Report: next.RunnableReport()}
}

// visibleNames indexes the visible (non-hidden) column names of one object;
// a nil object yields no names.
func visibleNames(obj *schema.Object) map[string]bool {
	out := make(map[string]bool)
	if obj == nil {
		return out
	}
	for _, col := range obj.Columns {
		if !col.Hidden {
			out[col.Name] = true
		}
	}
	return out
}

// insertableNames indexes the explicitly insertable column names of one
// object; a nil object yields no names.
func insertableNames(obj *schema.Object) map[string]bool {
	out := make(map[string]bool)
	if obj == nil {
		return out
	}
	for _, col := range obj.Columns {
		if col.Insertable {
			out[col.Name] = true
		}
	}
	return out
}

// orderKeyColumn extracts the declared column name behind an ORDER BY
// identity key. The count-star sentinel reports false (it references no
// column); aggregate keys split on the last separator so the column part is
// recovered from the reserved-prefix encoding.
func orderKeyColumn(key string) (string, bool) {
	const columnPrefix = "order-column:"
	const aggregatePrefix = "order-aggregate:"
	switch {
	case strings.HasPrefix(key, columnPrefix):
		return key[len(columnPrefix):], true
	case strings.HasPrefix(key, aggregatePrefix):
		rest := key[len(aggregatePrefix):]
		if i := strings.LastIndex(rest, ":"); i >= 0 {
			return rest[:i], true
		}
		return "", false
	default:
		return "", false
	}
}
