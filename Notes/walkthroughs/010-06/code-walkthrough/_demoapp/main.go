// Demo application for the Issue #10 walkthrough (disposable, lives outside
// the production source tree): drives one fixture table through Schema,
// Connection, and UI exactly as the tests do, printing observable state at
// each boundary so the tracer flow is visible end to end.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	schema "github.com/chris/sqloid/internal/schema"
	"github.com/chris/sqloid/internal/ui"
)

const fixtureSQL = `
CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT);
INSERT INTO albums VALUES (1, 'one'), (2, NULL), (3, 'three');
CREATE TABLE "odd ""name" (a INTEGER); INSERT INTO "odd ""name" VALUES (7);
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func create(path string) {
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		fatal(err)
	}
	defer writer.Close()
	for _, stmt := range strings.Split(fixtureSQL, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := writer.Exec(stmt); err != nil {
			fatal(fmt.Errorf("creating fixture (%s): %w", strings.TrimSpace(stmt), err))
		}
	}
}

func open(path string) *connection.DB {
	db, err := connection.Open(path)
	if err != nil {
		fatal(err)
	}
	return db
}

// traceExecutor returns the injected Schema/Connection-facing executor the UI
// would be wired with: catalog choice translated inside the seam, typed
// completion translated into UI types. No database code exists in ui.
func traceExecutor(db *connection.DB, cat *schema.Catalog, name string) func(context.Context) ui.TraceResult {
	obj, _ := schema.ChooseTracerTarget(cat, name) // demo pre-validates composition
	return func(ctx context.Context) ui.TraceResult {
		out, tres := db.RunTraceSelectAll(ctx, obj)
		if tres.Err != nil {
			return ui.TraceResult{Err: tres.Err.Error()}
		}
		rows := make([][]string, len(out.Rows))
		for i, row := range out.Rows {
			cells := make([]string, len(row))
			for j, v := range row {
				switch v.(type) {
				case nil:
					cells[j] = ""
				default:
					cells[j] = fmt.Sprintf("%v", v)
				}
			}
			rows[i] = cells
		}
		return ui.TraceResult{Grid: &ui.TraceGrid{Headers: out.Columns, Rows: rows}}
	}
}

// driveRender starts one trace through Update only (executor runs inside the
// returned command) and prints the settled 80x24 view.
func driveRender(m ui.Model, execute func(context.Context) ui.TraceResult, title string) {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(ui.Model)
	start, cmd := m.Update(ui.StartTraceMsg{Execute: execute})
	m = start.(ui.Model)
	msg := cmd() // blocks until the executor settles; returns traceSettledMsg
	settled, _ := m.Update(msg)
	fmt.Println(title)
	fmt.Println(settled.(ui.Model).View())
}

func show(db *connection.DB, path string) {
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != connection.OutcomeSuccess {
		fatal(res.Err)
	}
	fmt.Printf("catalog version %d, objects:", cat.Version)
	for _, o := range cat.Objects {
		fmt.Printf(" %s(%s)", o.Name, o.Kind)
	}
	fmt.Println()

	for _, name := range []string{"albums", `odd "name`} {
		obj, err := schema.ChooseTracerTarget(cat, name)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("\nselected %q -> SQL: %s\n", obj.Name, schema.SelectAllSQL(obj))
		out, tres := db.RunTraceSelectAll(context.Background(), obj)
		if tres.Outcome != connection.OutcomeSuccess {
			fatal(tres.Err)
		}
		fmt.Println("columns:", out.Columns)
		for i, row := range out.Rows {
			rendered := make([]string, len(row))
			for j, v := range row {
				switch v.(type) {
				case nil:
					rendered[j] = "NULL(nil)"
				case int64:
					rendered[j] = fmt.Sprintf("%v(int64)", v)
				case string:
					rendered[j] = fmt.Sprintf("%q(string)", v)
				default:
					rendered[j] = fmt.Sprintf("%#v(%T)", v, v)
				}
			}
			fmt.Printf("row %d: [%s]\n", i, strings.Join(rendered, ", "))
		}
	}

	// Render success and error through the UI model without any live terminal:
	// messages in via Update, command executed, settled message back in.
	driveRender(ui.New(), traceExecutor(db, cat, "albums"), "\n=== rendered tracer grid (80x24 view) ===")

	obj, err := schema.ChooseTracerTarget(cat, "albums")
	if err != nil {
		fatal(err)
	}
	w, err := sql.Open("sqlite", path)
	if err != nil {
		fatal(err)
	}
	defer w.Close()
	if _, err := w.Exec(`DROP TABLE albums`); err != nil {
		fatal(err)
	}
	out, tres := db.RunTraceSelectAll(context.Background(), obj)
	fmt.Printf("\nfailed trace: outcome=%v result=%v\nerr=%v\n", tres.Outcome, out, tres.Err)

	failExecutor := func(ctx context.Context) ui.TraceResult {
		_, tres := db.RunTraceSelectAll(ctx, obj)
		return ui.TraceResult{Err: tres.Err.Error()}
	}
	driveRender(ui.New(), failExecutor, "\n=== rendered tracer error (80x24 view) ===")

	db.Close()
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: _demoapp create|show PATH")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "create":
		create(os.Args[2])
	case "show":
		show(open(os.Args[2]), os.Args[2])
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}
