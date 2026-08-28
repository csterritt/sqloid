// Demonstration program for the Issue #21 code walkthrough: pre-execution
// schema-version validation. It exercises the exported UI-independent seams —
// schema.Revalidate typed outcomes, QueryBuilder.Revalidate dependent-only
// repair, and the connection-boundary ReadSchemaVersion/ReadCatalog requests
// against a real modernc.org/sqlite file — covering changed identifier,
// eligibility, insertability, and rowid fixtures plus post-validation DDL
// behavior. The Bubble Tea validation workflow itself is evidenced through
// the scripted tests in internal/ui (executed by the walkthrough).

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

func catalogV1() *schema.Catalog {
	return &schema.Catalog{Version: 7, Objects: []*schema.Object{
		{
			Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
			Columns: []schema.Column{
				{Name: "id", Insertable: true},
				{Name: "email", Insertable: true},
				{Name: "note", Insertable: true},
			},
		},
		{Name: "logs", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
	}}
}

func section(name string) { fmt.Println("== " + name + " ==") }

func main() {
	section("1. schema.Revalidate outcomes")
	{
		prior := catalogV1()
		r := schema.Revalidate(prior, 7, func() schema.Attempt {
			panic("refresh must never be invoked on an unchanged version")
		})
		fmt.Printf("unchanged: status=%v samePointer=%v\n", r.Status, r.Catalog == prior)

		refreshed := &schema.Catalog{Version: 9, Objects: prior.Objects}
		calls := 0
		r2 := schema.Revalidate(prior, 9, func() schema.Attempt {
			calls++
			return schema.NewSuccess(refreshed)
		})
		fmt.Printf("changed:   status=%v refreshCalls=%d catalog==refreshed=%v\n",
			r2.Status, calls, r2.Catalog == refreshed)

		r3 := schema.Revalidate(prior, 9, func() schema.Attempt {
			return schema.NewFailure(fmt.Errorf("database is locked"))
		})
		fmt.Printf("failure:   status=%v cause=%q cacheStands=%v\n",
			r3.Status, r3.Cause, r3.Catalog == nil)
	}

	section("2. QueryBuilder.Revalidate repair fixtures")
	{
		// 2a. Changed identifier: the note column vanishes; its projection
		// entry is removed individually while Limit and email survive.
		v2 := &schema.Catalog{Version: 9, Objects: []*schema.Object{{
			Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
			Columns: []schema.Column{{Name: "id", Insertable: true}, {Name: "email", Insertable: true}},
		}}}
		q := querybuilder.NewQuery().RefreshSchema(catalogV1()).
			SelectCommand(querybuilder.CommandSelect).SelectTable("users")
		q = q.CompleteProjectionAggregate("email", querybuilder.AggregateValue).Builder
		q = q.CompleteProjectionAggregate("note", querybuilder.AggregateValue).Builder
		q = q.SetLimitInput("5")
		repaired, report := q.Revalidate(v2)
		lim, limOK := repaired.LimitValue()
		fmt.Printf("identifier: cleared=%v entries=%d limit=(%d,%v) runnable=%v firstReason=%q\n",
			report.Cleared, len(repaired.ProjectionEntries()), lim, limOK, report.Report.Runnable, report.Report.Reason)

		// 2b. Eligibility change: users became a view, so DELETE drops the
		// table and everything downstream.
		qd := querybuilder.NewQuery().RefreshSchema(catalogV1()).
			SelectCommand(querybuilder.CommandDelete).SelectTable("users")
		vw := &schema.Catalog{Version: 9, Objects: []*schema.Object{
			{Name: "users", Kind: schema.KindView, Rowid: schema.RowidNotApplicable,
				Columns: []schema.Column{{Name: "id"}, {Name: "email"}, {Name: "note"}}},
			{Name: "logs", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
		}}
		rq, rr := qd.Revalidate(vw)
		_, hasTable := rq.SelectedTable()
		fmt.Printf("eligibility: cleared=%v tableSelected=%v runnable=%v\n",
			rr.Cleared, hasTable, rr.Report.Runnable)

		// 2c. Insertability change: note became hidden/generated, dropping
		// only its INSERT prompt.
		qi := querybuilder.NewQuery().RefreshSchema(catalogV1()).
			SelectCommand(querybuilder.CommandInsert).SelectTable("users")
		qi = qi.BeginInsertPrompts()
		qi, _ = qi.ChooseInsertColumn("note", querybuilder.InsertChoiceValue)
		qi, _ = qi.ChooseInsertColumn("id", querybuilder.InsertChoiceValue)
		hidden := &schema.Catalog{Version: 9, Objects: []*schema.Object{{
			Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
			Columns: []schema.Column{
				{Name: "id", Insertable: true},
				{Name: "email", Insertable: true},
				{Name: "note", Hidden: true, Insertable: false},
			},
		}}}
		ri, rir := qi.Revalidate(hidden)
		fmt.Printf("insertability: cleared=%v prompts=%d (note prompt dropped; id+email remain)\n",
			rir.Cleared, len(ri.InsertColumns()))

		// 2d. Rowid property change (rowid shadowing/WITHOUT ROWID): the
		// rowid-addressing ORDER BY consumer is dropped while the projection
		// stands.
		qo := querybuilder.NewQuery().RefreshSchema(catalogV1()).
			SelectCommand(querybuilder.CommandSelect).SelectTable("users")
		qo = qo.CompleteProjectionAggregate("email", querybuilder.AggregateValue).Builder
		committed := false
		for _, c := range qo.OrderByCandidates() {
			if next, ok := qo.AcceptOrderBy(c.Key); ok {
				qo, committed = next, true
				break
			}
		}
		withoutRowid := &schema.Catalog{Version: 9, Objects: []*schema.Object{{
			Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidWithout,
			Columns: catalogV1().Objects[0].Columns,
		}}}
		ro, ror := qo.Revalidate(withoutRowid)
		_, _, orderLeft := ro.OrderBySelection()
		fmt.Printf("rowid: committed=%v cleared=%v orderRemaining=%v projection=%d\n",
			committed, ror.Cleared, orderLeft, len(ro.ProjectionEntries()))
	}

	section("3. Live sqlite: version read, changed refresh, post-validation DDL")
	{
		dir, _ := os.MkdirTemp("", "demo21")
		path := filepath.Join(dir, "demo.sqlite")
		// Initialize a valid SQLite file before opening: connection.Open is
		// mode=rw with the 16-byte header check and never creates the target.
		seed, err := sql.Open("sqlite", path)
		if err != nil {
			fmt.Println("seed open failed:", err)
			os.Exit(1)
		}
		if _, err := seed.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, note TEXT)"); err != nil {
			fmt.Println("seed exec failed:", err)
			os.Exit(1)
		}
		seed.Close()
		db, err := connection.Open(path)
		if err != nil {
			fmt.Println("open failed:", err)
			os.Exit(1)
		}
		ctx := context.Background()
		if res := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "CREATE TABLE extras0 (x TEXT)") // verify writes work through the boundary
			return err
		}); res.Err != nil {
			fmt.Println("create failed:", res.Err)
			os.Exit(1)
		}

		cat, res := db.ReadCatalog(ctx)
		if res.Err != nil {
			fmt.Println("catalog failed:", res.Err)
			os.Exit(1)
		}
		ver, _ := db.ReadSchemaVersion(ctx)
		fmt.Printf("opened: version=%d catalog=%d objects=%v\n", ver, cat.Version, names(cat))

		r := schema.Revalidate(cat, ver, func() schema.Attempt {
			att, r2 := db.ReadCatalog(ctx)
			if r2.Err != nil {
				return schema.NewFailure(r2.Err)
			}
			return schema.NewSuccess(att)
		})
		fmt.Printf("unchanged: status=%v samePointer=%v (no refresh request issued)\n",
			r.Status, r.Catalog == cat)

		// External DDL changes the schema version; a changed-version
		// revalidation refreshes and reuses the fresh snapshot.
		if res := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "CREATE TABLE logs (line TEXT)")
			return err
		}); res.Err != nil {
			fmt.Println("ddl failed:", res.Err)
			os.Exit(1)
		}
		ver2, _ := db.ReadSchemaVersion(ctx)
		r2 := schema.Revalidate(cat, ver2, func() schema.Attempt {
			att, rr := db.ReadCatalog(ctx)
			if rr.Err != nil {
				return schema.NewFailure(rr.Err)
			}
			return schema.NewSuccess(att)
		})
		fmt.Printf("changed:   version=%d status=%v objects=%v\n", ver2, r2.Status, names(r2.Catalog))

		// Post-validation race: a settled unchanged outcome is immutable even
		// when DDL lands afterwards; the next request discovers the change.
		r3 := schema.Revalidate(r2.Catalog, ver2, func() schema.Attempt { panic("no refresh") })
		if res := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "CREATE TABLE extras (x TEXT)")
			return err
		}); res.Err != nil {
			fmt.Println("ddl failed:", res.Err)
			os.Exit(1)
		}
		ver3, _ := db.ReadSchemaVersion(ctx)
		fmt.Printf("post-validation race: settled=%v stillUnchanged=%v nextReadSees=%d (ordinary execution territory)\n",
			r3.Status, r3.Status == schema.RevalidateUnchanged, ver3)
		db.Close()
	}
}

func names(c *schema.Catalog) []string {
	out := []string{}
	for _, o := range c.Objects {
		out = append(out, o.Name)
	}
	return out
}
