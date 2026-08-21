### Sqloid

This is a project to create a SQL editor that can connect to multiple types of database and execute queries.

### Features
- Connect to multiple types of database
- Execute queries
- View results
- Save queries
- Save results

### Implementation ideas

- Uses golang for code
- Uses mow.cli (https://github.com/jawher/mow.cli) for the command line interface using command-word/arguments style
    - sqlite <file> - run the app with the given sqlite database file
    - local-d1 - run the app with a local d1 database
- Uses bubbletea (https://github.com/charmbracelet/bubbletea) for the UI and Lipgloss (https://github.com/charmbracelet/lipgloss) for styling, with (where appropriate) components from bubbles (https://github.com/charmbracelet/bubbles)
- For the 'sqlite <file>' command, the user gives the path to a sqlite database on the command line, with an error if it's not given, does not exist, or is not a sqlite database
- For the 'local-d1' command, the program should look in the '.wrangler/state/v3/d1/miniflare-D1DatabaseObject' directory for a single sqlite database file (that is not have 'metadata' in the name) and use that. If there is no such file, or if there are multiple such files, it should error.
- The main idea is that the user can type in SQL commands.

### UI

- There are fields at the bottom of the screen. There are results at the top. The results should take up most of the area, but the fields can grow to multiple lines as bits get added.
- The fields are:
    - Command
    - Column(s)
    - Table
    - Where
    - Order By
    - Limit
    - Group By
- The UI starts in the 'Command' field, and the user can hit:
    - S - which expands to 'Select'
    - U - which expands to 'Update'
    - D - which expands to 'Delete'
    - I - which expands to 'Insert'
- After the command is selected, the UI moves to the 'Table' field
- Then the UI pops up a list of all the tables in the given database, and the user can select a table from the list, either by scrolling or fuzzy search
- After the table is selected, the UI moves to the 'Column(s)' field, again with a list of columns in that table, with '*' at the top, selected by default. selecting one with 'enter' should add it to the list of columns
- After a column is selected, the user is given a pop-up list of 'Value/Count/Min/Max/Avg/Sum' to choose from. 'Value' is selected by default
- After the user selects an option, the UI shows the pop-up column list again so they can select more than one column
- After one or more columns are selected, the user can hit Enter to run the query, or tab/arrow keys to move to the next field
- arrows and tab/shift-tab move between fields
- Page up/down page through results 
- Shift page up/down scroll results left/right 
- Header shows result total row count, range currently displayed
- Ctrl p/n to scroll through previous queries 
- Ctrl shift p/n to scroll through previous results 
- If the user modifies a query, it should be added to the history at the bottom

### Decisions (from PRD interview, see Notes/PRD-sqloid.md)
- v1 supports only sqlite files and local d1; other databases are out of scope
- Database connections are read-write; use the pure-Go sqlite driver (modernc.org/sqlite)
- local-d1: ignore files with 'metadata' in the name and -shm/-wal files; error with a single message if more than one candidate; error if none
- The program errors out if the db file is deleted mid-session
- 'Edit results' is removed from the feature list
- Ctrl+S saves the query as plain .sql; Ctrl+Shift+S saves the result as csv or json; file picker for directory + text entry for name; confirm overwrite; saving is one-way (no loading) in v1
- Command field: single keypress S/U/D/I expands immediately; returning to the field lets another keypress replace the choice
- UPDATE/DELETE flows end with a confirmation modal before running; INSERT skips AUTO-INCREMENT columns and shows row add count; UPDATE/DELETE show rows affected plus the query
- Where/Order By/Limit are assisted: column popup then typed value entry; numbers-only for numeric columns, text otherwise
- Group By is an assisted multi-select column popup; if any aggregate column is picked, the query won't run until Group By is non-empty
- No Limit means a default cap of one terminal page of rows; total count comes from wrapping the query in 'Select count(*) from (...inner query...)'
- Results are fetched in pages; page size computed from terminal height and recomputed on resize
- NULLs render as empty cells in grid/CSV, 'null' keyword in JSON
- All user values bound as parameters (no string-concatenated SQL)
- Grid has a frozen header row; header shows total row count and displayed range
- Empty SELECT results show a message; errors replace the results view
- Ctrl+P/N repopulates all builder fields from a previous query; only genuine changes add new history entries; Ctrl+Shift+P/N scrolls result history including write-statement summaries
- Quit: 'q' quits without confirmation from the Command field only; Ctrl+C elsewhere shows a quit confirmation modal

### Future ideas, won't be implemented in version 1

- Full screen for nested query or flat text entry 
- Freeze column(s) for left/right scroll
